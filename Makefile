# Copyright 2024 Fantom Foundation
# This file is part of Norma System Testing Infrastructure for Sonic.
#
# Norma is free software: you can redistribute it and/or modify
# it under the terms of the GNU Lesser General Public License as published by
# the Free Software Foundation, either version 3 of the License, or
# (at your option) any later version.
#
# Norma is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
# GNU lesser General Public License for more details.
#
# You should have received a copy of the GNU Lesser General Public License
# along with Norma. If not, see <http://www.gnu.org/licenses/>.

BUILD_DIR := $(CURDIR)/build

.PHONY: all test clean

# Define a list of client versions
CLIENT_VERSIONS := \
	v2.0 v2.0.0 v2.0.1 v2.0.2 v2.0.3 \
	v2.1 v2.1.0 v2.1.1 v2.1.2
CLIENT_URL=https://github.com/0xsoniclabs/sonic.git

all: \
    norma \
    pull-hello-world-image \
    pull-alpine-image \
    pull-prometheus-image \
    build-sonic-docker-image-main \
    build-sonic-docker-image-local \
    $(foreach version, $(CLIENT_VERSIONS), build-sonic-docker-image-$(version)) \

pull-hello-world-image:
	DOCKER_BUILDKIT=1 docker image pull hello-world

pull-alpine-image:
	DOCKER_BUILDKIT=1 docker image pull alpine

pull-prometheus-image:
	DOCKER_BUILDKIT=1 docker image pull prom/prometheus:v2.44.0

build-sonic-docker-image-main:
	DOCKER_BUILDKIT=1 docker build --build-context client-src=$(CLIENT_URL) . -t sonic

build-sonic-docker-image-local:
	DOCKER_BUILDKIT=1 docker build --build-context client-src=sonic . -t sonic:local

# Build various client versions
$(foreach version, $(CLIENT_VERSIONS), build-sonic-docker-image-$(version)):
	DOCKER_BUILDKIT=1 docker build --build-context client-src=$(CLIENT_URL)\#$(subst build-sonic-docker-image-,,$@) . -t sonic:$(subst build-sonic-docker-image-,,$@)

generate-abi: load/contracts/abi/Counter.abi load/contracts/abi/ERC20.abi load/contracts/abi/Store.abi load/contracts/abi/UniswapV2Pair.abi load/contracts/abi/UniswapRouter.abi load/contracts/abi/Helper.abi load/contracts/abi/SmartAccount.abi load/contracts/abi/EntryPoint.abi load/contracts/abi/TransientCounter.abi load/contracts/abi/SelfDestructor.abi load/contracts/abi/InstantSelfDestructor.abi # requires installed solc and Ethereum abigen - check README.md

load/contracts/abi/Counter.abi: load/contracts/Counter.sol
	solc --evm-version london -o ./load/contracts/abi --overwrite --pretty-json --optimize --optimize-runs 200 --abi --bin ./load/contracts/Counter.sol
	abigen --type Counter --pkg abi --abi load/contracts/abi/Counter.abi --bin load/contracts/abi/Counter.bin --out load/contracts/abi/Counter.go

load/contracts/abi/ERC20.abi: load/contracts/ERC20.sol
	solc --evm-version london -o ./load/contracts/abi --overwrite --pretty-json --optimize --optimize-runs 200 --abi --bin ./load/contracts/ERC20.sol
	abigen --type ERC20 --pkg abi --abi load/contracts/abi/ERC20.abi --bin load/contracts/abi/ERC20.bin --out load/contracts/abi/ERC20.go

load/contracts/abi/Store.abi: load/contracts/Store.sol
	solc --evm-version london -o ./load/contracts/abi --overwrite --pretty-json --optimize --optimize-runs 200 --abi --bin ./load/contracts/Store.sol
	abigen --type Store --pkg abi --abi load/contracts/abi/Store.abi --bin load/contracts/abi/Store.bin --out load/contracts/abi/Store.go

load/contracts/abi/UniswapV2Pair.abi: load/contracts/UniswapV2Pair.sol
	solc --evm-version london -o ./load/contracts/abi --overwrite --pretty-json --optimize --optimize-runs 200 --abi --bin ./load/contracts/UniswapV2Pair.sol
	abigen --type UniswapV2Pair --pkg abi --abi load/contracts/abi/UniswapV2Pair.abi --bin load/contracts/abi/UniswapV2Pair.bin --out load/contracts/abi/UniswapV2Pair.go

load/contracts/abi/UniswapRouter.abi: load/contracts/UniswapRouter.sol
	solc --evm-version london -o ./load/contracts/abi --overwrite --pretty-json --optimize --optimize-runs 200 --abi --bin ./load/contracts/UniswapRouter.sol
	abigen --type UniswapRouter --pkg abi --abi load/contracts/abi/UniswapRouter.abi --bin load/contracts/abi/UniswapRouter.bin --out load/contracts/abi/UniswapRouter.go

load/contracts/abi/Helper.abi: load/contracts/Helper.sol
	solc --evm-version london -o ./load/contracts/abi --overwrite --pretty-json --optimize --optimize-runs 200 --abi --bin ./load/contracts/Helper.sol
	abigen --type Helper --pkg abi --abi load/contracts/abi/Helper.abi --bin load/contracts/abi/Helper.bin --out load/contracts/abi/Helper.go

load/contracts/abi/SmartAccount.abi: load/contracts/SmartAccount.sol
	solc --evm-version london -o ./load/contracts/abi --overwrite --pretty-json --optimize --optimize-runs 200 --abi --bin ./load/contracts/SmartAccount.sol
	abigen --type SmartAccount --pkg abi --abi load/contracts/abi/SmartAccount.abi --bin load/contracts/abi/SmartAccount.bin --out load/contracts/abi/SmartAccount.go

load/contracts/abi/EntryPoint.abi: load/contracts/EntryPoint.sol
	solc --evm-version london -o ./load/contracts/abi --overwrite --pretty-json --optimize --optimize-runs 200 --abi --bin ./load/contracts/EntryPoint.sol
	abigen --type EntryPoint --pkg abi --abi load/contracts/abi/EntryPoint.abi --bin load/contracts/abi/EntryPoint.bin --out load/contracts/abi/EntryPoint.go

load/contracts/abi/SelfDestructor.abi: load/contracts/SelfDestructor.sol
	solc --evm-version london -o ./load/contracts/abi --overwrite --pretty-json --optimize --optimize-runs 200 --abi --bin ./load/contracts/SelfDestructor.sol
	abigen --type SelfDestructorFactory --pkg abi --abi load/contracts/abi/SelfDestructorFactory.abi --bin load/contracts/abi/SelfDestructorFactory.bin --out load/contracts/abi/SelfDestructor.go

load/contracts/abi/InstantSelfDestructor.abi: load/contracts/InstantSelfDestructor.sol
	solc --evm-version london -o ./load/contracts/abi --overwrite --pretty-json --optimize --optimize-runs 200 --abi --bin ./load/contracts/InstantSelfDestructor.sol
	abigen --type InstantSelfDestructorFactory --pkg abi --abi load/contracts/abi/InstantSelfDestructorFactory.abi --bin load/contracts/abi/InstantSelfDestructorFactory.bin --out load/contracts/abi/InstantSelfDestructor.go

# TransientCounter uses EIP-1153 transient storage (Cancun) – requires solc >=0.8.24 and --evm-version cancun
load/contracts/abi/TransientCounter.abi: load/contracts/TransientCounter.sol
	solc --evm-version cancun -o ./load/contracts/abi --overwrite --pretty-json --optimize --optimize-runs 200 --abi --bin ./load/contracts/TransientCounter.sol
	abigen --type TransientCounter --pkg abi --abi load/contracts/abi/TransientCounter.abi --bin load/contracts/abi/TransientCounter.bin --out load/contracts/abi/TransientCounter.go

generate-mocks: # requires installed mockgen
	go generate ./...

norma: pull-prometheus-image build-sonic-docker-image-main
	go build -o $(BUILD_DIR)/norma ./driver/norma

test: pull-hello-world-image pull-alpine-image pull-prometheus-image build-sonic-docker-image-main
	go test ./... -v

clean:
	rm -rvf $(CURDIR)/build
