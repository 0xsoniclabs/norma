#!/usr/bin/env bash
# Runs one scenario on a short-lived GCP VM and deletes everything the run
# created, however it ends. See "Running in GCP" in README.md.
#
#   GCP_PROJECT=<project> scripts/cloud-run.sh <scenario.yml> [norma run flags]
#
# Optional: GCP_ZONE, MACHINE_TYPE, TIMEOUT (e.g. 30m), MAX_COST (USD, default 5).
set -euo pipefail

scenario=${1:?usage: cloud-run.sh <scenario.yml> [norma run flags]}
shift
project=${GCP_PROJECT:?set GCP_PROJECT}
zone=${GCP_ZONE:-us-central1-a}
max_cost=${MAX_COST:-5}
root=$(cd "$(dirname "$0")/.." && pwd)
started=$SECONDS

run_id=$(printf '%s-%s-%s' "${USER:-norma}" "$(date +%Y%m%d-%H%M%S)" "$RANDOM" | tr '[:upper:]' '[:lower:]' | tr -c 'a-z0-9\n' '-')
name=norma-run-$run_id
out=$root/cloud-runs/$run_id
tf=(terraform -chdir="$out/tf")

scenario=$(realpath "$scenario")
estimate=$(cd "$root" && go run ./driver/norma estimate --env \
    ${MACHINE_TYPE:+--machine-type "$MACHINE_TYPE"} ${TIMEOUT:+--timeout "$TIMEOUT"} "$scenario")
eval "$(grep -E '^[A-Z_]+=[A-Za-z0-9.-]+$' <<<"$estimate")"
if awk -v w="$WORST_CASE_COST" -v m="$max_cost" 'BEGIN { exit !(w > m) }'; then
    echo "worst-case cost \$$WORST_CASE_COST exceeds MAX_COST=\$$max_cost; refusing to start" >&2
    exit 1
fi

needed=(iap.tunnelInstances.accessViaIAP compute.instances.create compute.networks.create
    compute.firewalls.create compute.routers.create)
granted=$(curl -fsS -X POST "https://cloudresourcemanager.googleapis.com/v1/projects/$project:testIamPermissions" \
    -H "Authorization: Bearer $(gcloud auth print-access-token)" -H 'Content-Type: application/json' \
    -d "{\"permissions\": [$(printf '"%s",' "${needed[@]}" | sed 's/,$//')]}")
missing=()
for permission in "${needed[@]}"; do
    grep -q "\"$permission\"" <<<"$granted" || missing+=("$permission")
done
if ((${#missing[@]})); then
    echo "missing permissions in project $project: ${missing[*]}" >&2
    echo "(iap.tunnelInstances.accessViaIAP comes with roles/iap.tunnelResourceAccessor)" >&2
    exit 1
fi

ssh_vm() {
    local cmd=$1
    shift
    gcloud compute ssh "$name" --project "$project" --zone "$zone" \
        --tunnel-through-iap --quiet --command "$cmd" "$@"
}

leftovers() {
    local kind filter
    for kind in instances disks firewall-rules routers "networks subnets" networks; do
        filter="name=$name"
        [[ $kind == instances || $kind == disks ]] && filter="labels.norma-run=$run_id"
        # shellcheck disable=SC2086 # "networks subnets" is two words
        gcloud compute $kind list --project "$project" --filter="$filter" --format="value(name)" \
            --verbosity=error |
            sed "s/^/$kind /" || return 1
    done
}

teardown() {
    local rc=$?
    trap 'echo "interrupted: skipping ahead, resources are still deleted" >&2' INT TERM
    set +e

    if [[ -n ${ran:-} ]]; then
        echo "collecting results into $out/results (Ctrl-C skips)" >&2
        mkdir -p "$out/results"
        ssh_vm 'sudo tar -C /opt/norma/out -czf - .' | tar -C "$out/results" -xzf -
        ssh_vm 'df -h / /mnt/ssd; sudo du -sh /var/lib/docker /var/lib/containerd /opt/norma; sudo docker system df' >"$out/disk.txt"
    fi
    if [[ -d $out/tf ]]; then
        echo "deleting the VM and its network (log: $out/tf/destroy.log)" >&2
        for attempt in 1 2 3; do
            "${tf[@]}" destroy -auto-approve -input=false -no-color >>"$out/tf/destroy.log" 2>&1 && break
            echo "terraform destroy failed (attempt $attempt):" >&2
            tail -5 "$out/tf/destroy.log" >&2
        done
    fi

    local left
    if ! left=$(leftovers); then
        echo "could not list the resources of run $run_id; check project $project by hand" >&2
        exit 1
    fi
    if [[ -n $left ]]; then
        printf 'LEFTOVER resources of run %s; delete them with\n  terraform -chdir=%s destroy\nor by hand in project %s:\n%s\n' \
            "$run_id" "$out/tf" "$project" "$left" >&2
        exit 1
    fi
    echo "run $run_id: all resources deleted; took $((SECONDS - started))s, estimated ${EXPECTED_SECONDS:-?}s" >&2
    exit "$rc"
}
trap teardown EXIT
trap 'exit 130' INT TERM

mkdir -p "$out/tf" "${TF_PLUGIN_CACHE_DIR:=$HOME/.terraform.d/plugin-cache}"
export TF_PLUGIN_CACHE_DIR
cp "$root/infra/gcp/main.tf" "$out/tf/"
cat >"$out/tf/terraform.tfvars" <<EOF
project      = "$project"
zone         = "$zone"
run_id       = "$run_id"
machine_type = "$MACHINE_TYPE"
disk_gb      = $DISK_GB
ttl_minutes  = $TTL_MINUTES
EOF
"${tf[@]}" init -input=false >/dev/null
echo "creating $name and its network (log: $out/tf/apply.log)" >&2
if ! "${tf[@]}" apply -auto-approve -input=false -no-color >"$out/tf/apply.log" 2>&1; then
    tail -20 "$out/tf/apply.log" >&2
    exit 1
fi

echo "waiting for the VM to install Docker" >&2
deadline=$((SECONDS + 600))
until err=$(ssh_vm 'test -f /ready' 2>&1); do
    if ((SECONDS > deadline)); then
        printf '%s\nthe VM was not ready after 10 minutes\n' "$err" >&2
        exit 1
    fi
    sleep 10
done

tar -C "$root" --exclude=./.git --exclude=./sonic/.git --exclude=./sonic/build \
    --exclude=./build --exclude=./cloud-runs -czf - . |
    ssh_vm 'sudo mkdir -p /opt/norma/out && sudo tar -C /opt/norma -xzf -'
ssh_vm 'sudo tee /opt/norma/scenario.yml >/dev/null' <"$scenario"

go_image=$(grep -m1 -oE 'golang:[0-9.]+' "$root/Dockerfile")-trixie
echo "building norma on the VM (log: $out/build.log)" >&2
if ! ssh_vm "sudo docker run --rm -v /opt/norma:/src -w /src $go_image go build -o build/norma ./driver/norma" \
    >"$out/build.log" 2>&1; then
    tail -20 "$out/build.log" >&2
    exit 1
fi

flags=""
(($#)) && flags=$(printf '%q ' "$@")
ran=1
ssh_vm "cd /opt/norma && sudo ./build/norma run --skip-report-rendering -o /opt/norma/out --timeout $TIMEOUT $flags /opt/norma/scenario.yml" -- -t
