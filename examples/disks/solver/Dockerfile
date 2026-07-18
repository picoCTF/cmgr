# Share the prebuilt libguestfs base image with the challenge instead of
# re-installing libguestfs + a kernel here. Build it first:
#   docker build -t cmgr/examples-guestfish-base -f ci/guestfish-base.Dockerfile ci
FROM cmgr/examples-guestfish-base

COPY . /solve
WORKDIR /solve

CMD python3 solve.py
