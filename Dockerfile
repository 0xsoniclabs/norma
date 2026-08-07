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
# GO_VERSION selects the Go toolchain (the golang base image tag) the client
# is built with. GOTOOLCHAIN=local makes that tag authoritative: without it,
# Go silently downloads a newer toolchain whenever the client's go.mod asks
# for one, and the base image tag becomes decorative. A test in driver/docker
# guards that the default here stays at or above the sonic go.mod directive.
ARG GO_VERSION=1.26.3
FROM golang:${GO_VERSION} AS client-build
ENV GOTOOLCHAIN=local

WORKDIR /client

# Download expected Client version from the outside defined location.
# The 'client-src' parameter is passed as '--build-context' to the docker build command.

# Download Sonic dependencies first to cache them.
COPY --from=client-src go.mod .
RUN go mod download

# Copy the rest of the client source code to build it.
COPY --from=client-src . .

# Build the client. The build cache is keyed by toolchain so that builds
# with different Go versions do not thrash a shared cache.
ARG GO_VERSION
RUN --mount=type=cache,id=go-build-${GO_VERSION},target=/root/.cache/go-build make sonicd sonictool

#
# Stage 2: Build the final image
#
# It contains the client binaries only. Norma does not run the client as
# the container's entrypoint: it keeps the container idle and starts,
# stops, kills and restarts the client inside it via `docker exec`, so a
# node survives its client process (see driver/node/node_actions.go).
#
FROM debian:trixie

ARG GO_VERSION
LABEL io.norma.go-version="${GO_VERSION}"

RUN apt-get update && \
    apt-get install iproute2 iputils-ping -y

COPY --from=client-build /client/build/sonicd /client/build/sonictool ./

# Defaults for the client processes exec'd into this container; they are
# inherited from the container environment. GOMEMLIMIT keeps a node's heap
# small enough to run many of them on one host.
ENV STATE_DB_IMPL="geth"
ENV VM_IMPL="geth"
ENV LD_LIBRARY_PATH=./
ENV GOMEMLIMIT=1GiB

EXPOSE 5050
EXPOSE 6060
EXPOSE 18545
EXPOSE 18546

# Simple check that the binaries are built correctly
RUN ./sonictool --version
RUN ./sonicd version

# Idle by default so the container can be driven via `docker exec`. Norma
# sets the same entrypoint explicitly rather than relying on this default.
CMD ["sleep", "infinity"]
