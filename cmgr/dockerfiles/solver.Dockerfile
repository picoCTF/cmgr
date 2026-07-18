FROM ubuntu:24.04 AS base
ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y \
    python3-pip \
    socat && \
    rm -rf /var/lib/apt/lists/*

RUN apt-get update && apt-get install -y \
    build-essential && \
    rm -rf /var/lib/apt/lists/*

COPY Dockerfile packages.txt* ./
RUN if [ -f packages.txt ]; then apt-get update && xargs -a packages.txt apt-get install -y && rm -rf /var/lib/apt/lists/*; fi

COPY Dockerfile requirements.txt* ./
RUN if [ -f requirements.txt ]; then pip3 install --break-system-packages -r requirements.txt; fi

COPY . /solve
WORKDIR /solve

CMD python3 solve.py
