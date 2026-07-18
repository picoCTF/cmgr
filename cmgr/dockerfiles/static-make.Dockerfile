FROM ubuntu:24.04 AS base
ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y \
    python3-pip \
    socat && \
    rm -rf /var/lib/apt/lists/*

RUN apt-get update && apt-get install -y \
    build-essential && \
    rm -rf /var/lib/apt/lists/*

RUN install -d -m 0700 /challenge
# End of shared layers for all static-make challenges

COPY Dockerfile packages.txt* ./
RUN if [ -f packages.txt ]; then apt-get update && xargs -a packages.txt apt-get install -y && rm -rf /var/lib/apt/lists/*; fi

COPY . /app
WORKDIR /app

# End of shared layers for all builds of the same static-make challenge
FROM base AS challenge

ARG FLAG_FORMAT
ARG FLAG
ARG SEED

RUN make main
RUN make artifacts.tar.gz && mv artifacts.tar.gz /challenge || true
RUN make metadata.json && mv metadata.json /challenge
