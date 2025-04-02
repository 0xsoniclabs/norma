#!/bin/bash

yaml_dir="./scenarios/network_rules"

# Path to the input YAML file
input_yaml="${yaml_dir}/network_rules_constraints.yml"

# Path to the output directory
output_dir="${yaml_dir}/"

# Extract keys from rules_config.go
keys=(
  "MAX_BLOCK_GAS: 123"
#  "MAX_EMPTY_BLOCK_SKIP_PERIOD"
#  "MAX_EPOCH_GAS"
  "MAX_EPOCH_DURATION: 0s"
  "EMITTER_INTERVAL: 60s"
  "EMITTER_STALL_THRESHOLD: 0s"
  "EMITTER_STALLED_INTERVAL: 0s"
  "UPGRADES_BERLIN: false"
  "UPGRADES_LONDON: false"
  "UPGRADES_LLR: true"
  "UPGRADES_SONIC: false"
  "UPGRADES_ALLEGRO: false"
  "MIN_GAS_PRICE: 0"
  "MIN_GAS_PRICE: 1000000000000000"
  "MIN_BASE_FEE: 0"
  "MIN_BASE_FEE: 1000000000000000"
#  "BLOCK_MISSED_SLACK"
  "MAX_EVENT_GAS: 0"
#  "EVENT_GAS"
#  "PARENT_GAS"
#  "EXTRA_DATA_GAS"
#  "BLOCK_VOTES_BASE_GAS"
#  "BLOCK_VOTE_GAS"
#  "EPOCH_VOTE_GAS"
#  "MISBEHAVIOUR_PROOF_GAS"
  "SHORT_ALLOC_PER_SEC: 0"
  "SHORT_MAX_ALLOC_PERIOD: 0s"
  "SHORT_STARTUP_ALLOC_PERIOD: 0s"
#  "SHORT_MIN_STARTUP_GAS"
  "LONG_ALLOC_PER_SEC: 0"
  "LONG_MAX_ALLOC_PERIOD: 0s"
  "LONG_STARTUP_ALLOC_PERIOD: 0s"
#  "LONG_MIN_STARTUP_GAS"
  "MAX_PARENTS: 1"
  "MAX_FREE_PARENTS: 1"
  "MAX_EXTRA_DATA: 100000000000"
)

# Iterate over each key and generate a new YAML file
for key in "${keys[@]}"; do
  sanitized_key=$(echo "$key" | sed 's/[^a-zA-Z0-9_-]/_/g')
  # Replace the string "MAX_PARENTS: 0" with the current key
  sed "s/MAX_PARENTS:.*/$key/" "$input_yaml" > "$output_dir/network_rules_constraints_$sanitized_key.yml"
done

echo "YAML files generated in $output_dir"
