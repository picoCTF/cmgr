# Custom build so the challenge can share the prebuilt libguestfs base image
# (cmgr/examples-guestfish-base) with the solver instead of installing
# libguestfs + a kernel a second time. Build that base first:
#   docker build -t cmgr/examples-guestfish-base -f ci/guestfish-base.Dockerfile ci
FROM cmgr/examples-guestfish-base AS base

RUN install -d -m 0700 /challenge
COPY . /app
WORKDIR /app

FROM base AS challenge

ARG FLAG_FORMAT
ARG FLAG
ARG SEED

RUN make main
RUN make artifacts.tar.gz && mv artifacts.tar.gz /challenge
RUN make metadata.json && mv metadata.json /challenge
