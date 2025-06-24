#!/bin/bash

export CGO_CFLAGS="-O2 -D__BLST_PORTABLE__"
export CGO_CFLAGS_ALLOW="-O2 -D__BLST_PORTABLE__"

echo "Sonic binary checksum: $(sha256sum   /sonicd | cut -d ' ' -f 1 )"

# Get the local node's IP.
list=`hostname -I`
array=($list)
external_ip=${array[0]}

echo "Sonic is going to export its services on ${external_ip}"
echo "val id=${VALIDATOR_ID}"
echo "genesis validator count=${VALIDATORS_COUNT}"

# Export genesis.json
./genesistools genesis export genesis.json

datadir=$STATE_DB_DATADIR
# Initialize datadir
mkdir -p ${datadir}
./sonictool --datadir ${datadir} genesis json --experimental /genesis.json

##
## if $VALIDATOR_ID is set, it is a validator
##
if [[ $VALIDATOR_ID -ne 0 ]]
then
	cmd=`./genesistools validator from -id ${VALIDATOR_ID} -d ${datadir}`
	res=($cmd)
	VALIDATOR_PUBKEY=${res[0]}
	VALIDATOR_ADDRESS=${res[1]}
fi

# Create password file - "password" is default norma/genesistools accounts password
echo password >> password.txt
VALIDATOR_PASSWORD="password.txt"

# If validator, initialize here
val_flag=""
if [[ $VALIDATOR_ID -ne 0 ]]
then
	echo "Sonic is now running as validator"
	echo "val.id=${VALIDATOR_ID}"
	echo "pubkey=${VALIDATOR_PUBKEY}"
	echo "address=${VALIDATOR_ADDRESS}"
	val_flag="--validator.id ${VALIDATOR_ID} --validator.pubkey ${VALIDATOR_PUBKEY} --validator.password ${VALIDATOR_PASSWORD} --mode rpc"
else
	echo "Sonic is now running as an observer"
fi

# Create config.toml
# when network starts with only one genesis validator, then he will not wait to start emitting
# if there are two or more validators at genesis they have to wait 5 seconds after connecting to the network
# if another validator connects to the network during run it will wait also 5 seconds to start emitting
echo [Emitter.EmitIntervals] >> config.toml
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
  tc qdisc add dev eth0 root netem delay $NETWORK_LATENCY
  tc qdisc add dev eth1 root netem delay $NETWORK_LATENCY
fi


# Start sonic as part of a fake net with RPC service.
export GOMEMLIMIT="1GiB"
./sonicd \
    --datadir=${datadir} \
    ${val_flag} \
    --http --http.addr 0.0.0.0 --http.port 18545 --http.api admin,eth,ftm \
    --ws --ws.addr 0.0.0.0 --ws.port 18546 --ws.api admin,eth,ftm \
    --pprof --pprof.addr 0.0.0.0 \
    --nat=extip:${external_ip} \
    --metrics \
    --metrics.expensive \
    --config config.toml \
    --datadir.minfreedisk 0
