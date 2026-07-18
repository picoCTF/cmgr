FROM ubuntu:24.04 AS base
ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y \
    python3-pip \
    socat && \
    rm -rf /var/lib/apt/lists/*

RUN apt-get update && apt-get install -y \
    openssl && \
    rm -rf /var/lib/apt/lists/*

# Every challenge must place files into this directory
RUN mkdir /challenge
RUN date > /time.txt

FROM base AS challenge

ARG FLAG

# The "flag" field in this json object is mandatory, the rest are lookup fields.
RUN echo "{\"flag\":\"$FLAG\"}" > /challenge/metadata.json
RUN echo "$FLAG" | openssl aes-256-cbc -k unguessable -pbkdf2 -out /secret.enc

# These "artifacts" are available to competitors for download
RUN tar czvf /challenge/artifacts.tar.gz -C / secret.enc time.txt

# The service just hands out the decryption command; competitors (and the
# solver) run it against the downloaded secret.enc in their own directory, so
# the '-in' path is relative to wherever they extracted the artifact.
CMD socat TCP4-LISTEN:4242,reuseaddr,fork exec:'echo -n openssl aes-256-cbc -d -k unguessable -pbkdf2 -in secret.enc'

EXPOSE 4242
# PUBLISH 4242 AS socat
