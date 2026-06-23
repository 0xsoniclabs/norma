#!/bin/bash

LOGDIR="logs/sequential_$(date +%Y%m%d_%H%M%S)"
mkdir -p "$LOGDIR"

echo "Logs will be written to $LOGDIR"

for scenario in scenarios/new_syntax/*.yml; do
    name=$(basename "$scenario" .yml)
    logfile="$LOGDIR/${name}.log"
    echo -n "Running $name ... "
    if go run ./driver/norma/ run "$scenario" > "$logfile" 2>&1; then
        echo "OK"
    else
        echo "FAILED (exit code $?), see $logfile"
    fi
done

echo "Done. All logs in $LOGDIR"
