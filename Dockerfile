# syntax=docker/dockerfile:1

# This is a multi stages Dockerfile, which builds go-opera
# from the client/ directory first, and runs the binary then.
#
# This Dockerfile requires running installation of Docker,
# and then the image is build by typing
# > docker build . -t <image-name>
#

# The build is done in independent stages, to allow for
# caching of the intermediate results.

#
# Stage 1a: Build Client
#
# It prepeares an image with dependencies for the client.
# Its caches the dependencies first, so that the build is faster.
#
# It checks out the required version of the client, and builds it.
#
FROM golang:1.24 AS client-build

WORKDIR /client

# Download expected Client version from the outside defined location.
# The 'client-src' parameter is passed as '--build-context' to the docker build command.

# Download Sonic dependencies first to cache them.
COPY --from=client-src go.mod .
RUN go mod download

# Copy the rest of the client source code to build it.
COPY --from=client-src . .

# Build the client
RUN --mount=type=cache,target=/root/.cache/go-build make sonicd sonictool

#
# Stage 1b: Build Norma related tools supporting Client runs.
#
# It prepares an image with dependencies for the norma.
# Its caches the dependencies first, so that the build is faster.
#
# It checks out the local version of the norma, and builds it.
#
FROM golang:1.24 AS norma-build

# Download dependencies supporting Sonic run first to cache them for faster build when Norma changes.
WORKDIR /
COPY genesis/go.mod go.mod
RUN go mod download

# Build norma itself
WORKDIR /genesistools
COPY /genesis/ ./
RUN --mount=type=cache,target=/root/.cache/go-build make genesistools

#
# Stage 2: Build the final image
# It consists of the client binaries and the norma tools supporting runtime of the client.
#
FROM debian:bookworm

RUN apt-get update && \
    apt-get install iproute2 iputils-ping -y

COPY --from=client-build /client/build/sonicd /client/build/sonictool ./
COPY --from=norma-build /genesistools/build/genesistools ./

ENV STATE_DB_IMPL="geth"
ENV VM_IMPL="geth"
ENV LD_LIBRARY_PATH=./
ENV TINI_KILL_PROCESS_GROUP=1

EXPOSE 5050
EXPOSE 6060
EXPOSE 18545
EXPOSE 18546

COPY scripts/run_sonic_privatenet.sh ./run_sonic.sh

CMD ["./run_sonic.sh"]
