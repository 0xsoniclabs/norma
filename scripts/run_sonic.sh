#!/bin/bash
set -euo pipefail # fail if anything fails

echo "Sonic binary checksum: $(sha256sum   /sonicd | cut -d ' ' -f 1 )"

# Get the local node's IP, waiting for the network interface to be ready.
until external_ip=$(hostname -I | awk '{print $1}') && [[ -n "$external_ip" ]]; do
  sleep 1
done

echo "Sonic is going to export its services on ${external_ip}"
echo "val id=${VALIDATOR_ID}"
echo "genesis validator count=${VALIDATORS_COUNT}"

datadir=$STATE_DB_DATADIR
# Initialize datadir
if [[ ! -d "${datadir}/chaindata" ]]; then
  mkdir -p "${datadir}"
  ./sonictool \
    --datadir "${datadir}" \
    --statedb.livecache 1 \
    genesis json --experimental /genesis.json
fi

# Create password file for validator keystore decryption.
echo password > password.txt
VALIDATOR_PASSWORD="password.txt"

# If validator, initialize here
val_flag=""
if [[ $VALIDATOR_ID -ne 0 ]]
then
	echo "Sonic is now running as validator"
  if [[ -z "${VALIDATOR_PUBKEY:-}" || -z "${VALIDATOR_ADDRESS:-}" ]]; then
    echo "VALIDATOR_PUBKEY and VALIDATOR_ADDRESS must be set for validators"
    exit 1
  fi
	echo "val.id=${VALIDATOR_ID}"
	echo "pubkey=${VALIDATOR_PUBKEY}"
	echo "address=${VALIDATOR_ADDRESS}"
	val_flag="--validator.id ${VALIDATOR_ID} --validator.pubkey ${VALIDATOR_PUBKEY} --validator.password ${VALIDATOR_PASSWORD} --mode rpc"
else
	echo "Sonic is now running as an observer"
fi

# The Sonic consensus-chain engine currently requires fakenet mode: it derives
# validator keys (including the BLS attestation key) from the fake-key table.
# In fakenet mode the validator key is provided by the client itself, so the
# --validator.* flags must not be passed alongside a non-zero fakenet ID.
# The mesh listens on all interfaces on a fixed port so that peers in other
# containers can dial it; the port is private to the container's namespace.
if [[ "${CONSENSUS_CHAIN:-}" == "true" ]]; then
  echo "Consensus-chain engine enabled; running in fakenet mode"
  fakenet_denominator=${VALIDATORS_COUNT}
  if [[ $VALIDATOR_ID -gt $fakenet_denominator ]]; then
    fakenet_denominator=$VALIDATOR_ID
  fi
  val_flag="--fakenet ${VALIDATOR_ID}/${fakenet_denominator}"
  if [[ $VALIDATOR_ID -ne 0 ]]; then
    val_flag="${val_flag} --mode rpc"
  fi
  export SONIC_CONSENSUSCHAIN_LISTEN_ADDRS="/ip4/0.0.0.0/udp/5052/quic-v1,/ip4/0.0.0.0/tcp/5052"
fi

# Create config.toml
# when network starts with only one genesis validator, then he will not wait to start emitting
# if there are two or more validators at genesis they have to wait 5 seconds after connecting to the network
# if another validator connects to the network during run it will wait also 5 seconds to start emitting
echo '[Emitter.EmitIntervals]' >> config.toml
if [[ $VALIDATORS_COUNT == 1 && $VALIDATOR_ID == 1 ]]
then
  echo DoublesignProtection = 0 >> config.toml
else
#  5 seconds in golang time 5*10^9 nanoseconds
  echo DoublesignProtection = 5000000000 >> config.toml
fi

# Add network latency between nodes. The following command delays
# each out-going package by the given latency. To get a given
# round-trip time, the one-way latency has to be half of it.
# To check, run `docker exec <src-container-id> ping <dst-container-id>` on host.
echo "NETWORK_LATENCY=${NETWORK_LATENCY}"
if [[ -n "${NETWORK_LATENCY}" ]]; then
  echo "Adding network latency .."
  tc qdisc add dev eth0 root netem delay "${NETWORK_LATENCY}"
  if ip link show eth1 &>/dev/null; then # if eth1 exists
    tc qdisc add dev eth1 root netem delay "${NETWORK_LATENCY}"
  fi
fi


# Enable the test-only API when the client supports it. It provides the
# bootstrap RPCs used to seed the consensus-chain p2p mesh; older clients do
# not know the flag. Unknown API namespaces in --http.api are ignored.
test_api_flag=""
if ./sonicd --help 2>/dev/null | grep -q "enable-test-only-api"; then
  test_api_flag="--enable-test-only-api"
fi

# Start sonic as part of a fake net with RPC service.
export GOMEMLIMIT="1GiB"
# shellcheck disable=SC2086
./sonicd \
    --datadir="${datadir}" \
    ${val_flag} \
    ${test_api_flag} \
    --http --http.addr 0.0.0.0 --http.port 18545 --http.api admin,eth,sonic,txpool,test \
    --ws --ws.addr 0.0.0.0 --ws.port 18546 --ws.api admin,eth,sonic,txpool,test \
    --pprof --pprof.addr 0.0.0.0 \
    --nat="extip:${external_ip}" \
    --metrics \
    --metrics.expensive \
    --config config.toml \
    --datadir.minfreedisk 0 \
    --statedb.livecache 1 \
    $EXTRA_ARGUMENTS

# docker runs by default with root user, so any files or folders created by
# it would not have permissions to be deleted by non-root users doing cleanup
# once the test is done. So we change the  datadir folder permissions to enable
# all users to run clean up.
chmod -R 777 "${datadir}"
