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
# It checks out the required version of the client, and builds it.
#
FROM golang:1.26.3 AS client-build

WORKDIR /client

# The 'client-src' parameter is passed as '--build-context' to the docker build command.
# The whole source is copied before any module is resolved: its go.mod may
# `replace` a dependency with a directory inside it, such as a private fork
# cloned there by CI. The module cache mount keeps dependencies cached instead.
COPY --from=client-src . .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    make sonicd sonictool

#
# Stage 2: Build the final image
#
# It contains the client binaries only. Norma does not run the client as
# the container's entrypoint: it keeps the container idle and starts,
# stops, kills and restarts the client inside it via `docker exec`, so a
# node survives its client process (see driver/node/node_actions.go).
#
FROM debian:trixie

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
