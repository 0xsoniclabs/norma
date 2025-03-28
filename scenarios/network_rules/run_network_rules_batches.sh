#!/bin/bash

# Number of jobs to run in parallel
max_jobs=1

# Path to the directory containing YAML files
yaml_dir="./scenarios/network_rules/"

# Find all YAML files in the directory
yaml_files=("$yaml_dir"/*.yml)

# Function to run a single YAML file
run_yaml() {
  local yaml_file=$1
  go run ./driver/norma run "$yaml_file"
  echo "$? $yaml_file" > "$yaml_file.exitcode"
  rm "$yaml_file.job"
}

for yaml_file in "${yaml_files[@]}"; do
    touch "$yaml_file.job"

    # Run the YAML file scenario in the background
    run_yaml "${yaml_file}" &

    # Check if the number of running jobs has reached the limit
    while [  "$(ls -1 ${yaml_dir}/*.job 2>/dev/null | wc -l)" -ge "$max_jobs" ]; do
      sleep 10s
    done
done

# wait for all remaining jobs to finish
wait

# Check exit codes
echo "Exit codes for all scenarios:"
cat ${yaml_dir}/*.exitcode | sort
rm ${yaml_dir}/*.exitcode