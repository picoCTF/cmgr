FROM ubuntu:24.04 AS base
ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y \
    build-essential
RUN install -d -m 0700 /challenge
# End of shared layers for all flag-only challenges

COPY Dockerfile packages.txt* ./
RUN if [ -f packages.txt ]; then apt-get update && xargs -a packages.txt apt-get install -y; fi

COPY . /app
WORKDIR /app

# End of shared layers for all builds of the same flag-only challenge
FROM base AS challenge

ARG FLAG_FORMAT
ARG FLAG
ARG SEED

# The build's only product is metadata.json: the flag plus any lookup values
# (e.g. shuffled multiple-choice options) referenced by the challenge text.
# A Makefile with a 'metadata.json' target is optional; without one, the
# templated flag is used as-is.  No artifacts are produced and no instance is
# ever launched, so there is intentionally no CMD.
RUN if [ -f Makefile ]; then \
        make metadata.json && mv metadata.json /challenge; \
    else \
        echo "{\"flag\":\"$FLAG\"}" > /challenge/metadata.json; \
    fi
