# Shared base image for the disks (recovery-and-identification) example.
#
# libguestfs-tools pulls in qemu and a full kernel (linux-image-virtual), which
# is slow to install and was previously downloaded twice per run: once in the
# challenge build and again in the solver build. Baking it into one image that
# both the challenge Dockerfile and the solver Dockerfile FROM lets the install
# happen a single time (and, with a build cache, be reused across runs).
#
# Build before running the example, e.g.:
#   docker build -t cmgr/examples-guestfish-base -f ci/guestfish-base.Dockerfile ci
FROM ubuntu:24.04
ENV DEBIAN_FRONTEND=noninteractive

# build-essential provides make/gcc for the challenge Makefile; python3 runs the
# solver; libguestfs-tools + a kernel provide guestfish for both.
RUN apt-get update && apt-get install -y \
    build-essential \
    python3 \
    libguestfs-tools \
    linux-image-virtual && \
    rm -rf /var/lib/apt/lists/*
