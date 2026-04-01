# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What is Norma

Norma is a system testing infrastructure for the Sonic blockchain. It orchestrates Docker-based Sonic validator networks, generates transaction load, collects metrics, and validates network consistency — all driven by YAML scenario files.

## Requirements

- Go 1.24+
- Docker 23.0+ with buildx
- R with packages: `rmarkdown`, `tidyverse`, `lubridate`, `slider` (for report generation)
- Optional: `solc` ≥0.8.29, `abigen` (for contract ABI generation), `jq` (for EOF contract compilation), `mockgen` (for mocks)

## Common Commands

```bash
# Build
make norma                          # Build the norma binary to build/norma
make -j                             # Build everything (norma + Docker images)
make build-sonic-docker-image-main  # Build Sonic Docker image from upstream
make build-sonic-docker-image-local # Build Sonic Docker image from local sonic/ submodule

# Test (requires Docker images to be available)
make test                           # Run all tests
go test ./...                       # Run all tests via Go directly
go test ./driver/executor/... -v -run TestRunScenario  # Run a single test

# Code generation
make generate-abi    # Regenerate Solidity ABIs (requires solc + abigen; EOF contracts also need jq, see scripts/compile_eof.sh)
make generate-mocks  # Regenerate mocks (requires mockgen)

# Run scenarios
build/norma run scenarios/small.yml   # Run a specific scenario
build/norma run scenarios/            # Run all scenarios in a directory
build/norma check scenarios/small.yml # Validate a scenario file
build/norma render <csv-dir>          # Generate analysis report from collected metrics
```

## Architecture

### Execution Flow

1. **CLI** (`driver/norma/`) parses commands and invokes the executor.
2. **Parser** (`driver/parser/`) reads a YAML scenario into typed Go structs (`Scenario`, `Validator`, `Node`, `Application`, etc.).
3. **Executor** (`driver/executor/`) walks the scenario timeline, emitting events at the right wall-clock times (node start/stop, epoch advances, network rule updates, consistency checks).
4. **Network** (`driver/network/local/`) manages the Docker-based Sonic network: creating containers, configuring genesis, managing RPC connections.
5. **Node** (`driver/node/`) wraps individual Sonic Docker containers and exposes a lifecycle interface.
6. **Load** (`load/`) deploys smart contract applications and drives transaction traffic according to configured shapers.
7. **Monitoring** (`driver/monitoring/`) collects node and application metrics (block height, gas rate, CPU, memory, Prometheus scrapes) and writes CSVs.
8. **Checking** (`driver/checking/`) runs consistency validators (block hash agreement, gas rate bounds) at scheduled scenario times.

### Key Packages

| Package | Role |
|---|---|
| `driver/executor/` | Scenario timeline execution engine |
| `driver/network/local/` | Docker network backend (node creation, genesis, RPC) |
| `driver/node/` | Sonic node container lifecycle |
| `driver/parser/` | YAML scenario → Go structs |
| `driver/monitoring/` | Metric collection (app, node, Prometheus) → CSV |
| `driver/checking/` | Network consistency validators |
| `driver/rpc/` | Ethereum-compatible RPC client for blockchain queries |
| `driver/docker/` | Low-level Docker API wrapper |
| `load/app/` | Transaction source apps: Counter, ERC20, UniswapV2, SmartAccount, Store, Transient, OsakaCounter, SelfDestructor, InstantSelfDestructor |
| `load/shaper/` | Traffic rate patterns: constant, slope, wave, auto |
| `load/controller/` | Routes transactions to nodes, enforces rate limits |
| `load/contracts/` | Solidity contracts (compiled ABIs checked in) |
| `analysis/report/` | R-based report generation from CSV metrics |
| `genesis/` | Validator key generation and network genesis config |
| `scenarios/` | YAML scenario definitions (test/, eval/, load_tests/, etc.) |

### Scenario YAML Structure

```yaml
name: my-scenario
duration: 120          # seconds
validators:
  - name: validator
    instances: 3
nodes:
  - name: rpc-node
    start: 10
    end: 90
applications:
  - name: load
    type: ERC20
    start: 20
    end: 100
    rate: 100          # tx/s
network_rules:
  genesis:
    MaxGasPerBlock: "1000000"
  updates:
    - time: 30
      rules:
        MaxGasPerBlock: "2000000"
advance_epoch:
  - time: 15
    epochs: 2
checks:
  - time: 60
    check: BlockHeight
```

### Sonic Submodule

`sonic/` is a Git submodule pointing to the Sonic blockchain client. The Docker image built from it is the node binary used in all tests. When updating the submodule, rebuild the Docker image with `make build-sonic-docker-image-local`.

## Testing Notes

- Most integration tests spin up real Docker containers and require Docker images to be present.
- The default Docker image tag is defined in the Makefile (`SONIC_IMAGE`); override with `NORMA_IMAGE` env var or scenario `imageName` field.
- Unit tests that don't need Docker can be run selectively with standard `go test ./pkg/...` patterns.
- Mocks are generated with `go.uber.org/mock/mockgen` and live alongside the interfaces they mock.