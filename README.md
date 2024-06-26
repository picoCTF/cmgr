# cmgr

## Notice

This repo is a fork of [ArmyCyberInstitute/cmgr](https://github.com/ArmyCyberInstitute/cmgr) with
customizations to support our continued usage of cmgr at [picoCTF](https://picoctf.org/).

For specific details regarding changes from the upstream project, see the [release notes](https://github.com/picoCTF/cmgr/releases).

## Introduction

**cmgr** is a new backend designed to simplify challenge development and
management for Jeopardy-style CTFs.  It provides a CLI (`cmgr`) intended for
development and managing available challenges on a back-end challenge server
as well as a REST server (`cmgrd`) which exposes the minimal set of commands
necessary for a front-end web interface to leverage it to host a competition
or training platform.

## Quickstart

Assuming you already have Docker installed, the following code snippet will
download example challenges and the **cmgr** binaries, initialize a
database file that tracks the metadata for those challenges, and then run the
test suite to ensure a working system.  The test suite can take several minutes
to run and is not required to start working.  However, running the suite can
identify permissions and other errors and is highly recommended for the first
time you use `cmgr` on a system.

```sh
wget https://github.com/picoCTF/cmgr/releases/latest/download/examples.tar.gz
wget https://github.com/picoCTF/cmgr/releases/latest/download/cmgr_`uname -s | tr '[:upper:]' '[:lower:]'`_amd64.tar.gz
tar xzvf examples.tar.gz
cd examples
tar xzvf ../cmgr_`uname -s | tr '[:upper:]' '[:lower:]'`_amd64.tar.gz
./cmgr update
CMGR_LOGGING=info ./cmgr test --require-solve
```

**NOTE:** If you are running this on an ARM-based computer, you will need to change `amd64` in the cmgr tarball to `arm64`.

At this point, you can start checking out problems by finding the challenge ID
of one you would like to play and running `./cmgr playtest <challenge>`.  This
will build and start the challenge and run a minimal webserver (`localhost:4200`
by default) that you can use to view and interact with the content.  You could
also launch the REST server on port 4200 with `./cmgrd` or launch all of the
examples from the CLI with `./cmgr test --no-solve` which will launch an
instance of each example challenge and print the associated port information.

## Configuration

**cmgr** is configured using environment variables.  In particular, it
currently uses the following variables:

- *CMGR\_DB*: path to cmgr's database file (defaults to 'cmgr.db')

- *CMGR\_DIR*: directory containing all challenges (defaults to '.')

- *CMGR\_ARTIFACT\_DIR*: directory for storing artifact bundles (defaults to '.')

- *CMGR\_LOGGING*: logging verbosity for command clients (defaults to
'disabled' for `cmgr` and 'warn' for `cmgrd`; valid options are `debug`,
`info`, `warn`, `error`, and `disabled`)

- *CMGR\_INTERFACE*: the host interface/address to which published challenge
ports should be bound (defaults to '0.0.0.0') (_Note_: if the specified
address is not bound to the host running the Docker daemon, this value gets
silently ignored by Docker and the exposed ports will be bound to the loopback
interface.)

- *CMGR\_PORTS*: the range of ports that are dedicated for serving challenges;
cmgr will assume that it fully owns these ports and nothing else will try
to use them (i.e., not in ephemeral range or overlapping with a service
running on the host); format is '1000-1000'.  Ephemeral ports on a Linux host
can be enumerated with `cat /proc/sys/net/ipv4/ip_local_port_range` and adjusted
with `sysctl`.  Some programs (e.g., `docker`) will need to be restarted after
adjusting the kernel parameter.

- *CMGR\_ENABLE\_DISK\_QUOTAS*: enables the [disk
  quota](examples/specification.md#challenge-options) container option when set. Disk quotas
  are only functional when using the `overlay2` Docker storage driver and
  [pquota-enabled](https://access.redhat.com/documentation/en-us/red_hat_enterprise_linux/7/html/storage_administration_guide/xfsquota)
  XFS backing storage. Otherwise, the creation of containers with disk quotas will fail at runtime.
  When unset, any specified quotas are ignored.

Additionally, we rely on the Docker SDK's ability to self-configure base off
environment variables.  The documentation for those variables can be found at
[https://docs.docker.com/engine/reference/commandline/cli/](https://docs.docker.com/engine/reference/commandline/cli/).

## Developing...

### Challenges

One of our design goals is to make developing challenges for CTFs as simple as
possible so that developers can focus on the content and not quirks of the
platform.  We have specific challenge types that make it as easy as possible to
create new challenges of a particular flavor, and the documentation for each
type and how to use them are in the [examples](examples/) directory.

Additionally, we have a simple interface for creating automated solvers for
your challenges.  It is as simple as creating a directory named `solver` with
a Python script called `solve.py`.  This script will get its own Docker
container on the same network as the instance it is checking and start with
all of the artifact files and additional information provided to competitors in
its working directory.  Once it solves the challenge, it just needs to write
the flag value to a file named `flag` in its current working directory and
**cmgr** will validate the answer and report it back to the user.

In both the challenge and solver cases, we support challenge authors using
custom Dockerfiles to support creative challenges that go beyond the most
common types of challenges.  In order to support the other automation aspects
of the system, there are some requirements for certain files to be created
during the build phase of the Docker image and are documented in the `custom`
challenge type example.

Testing challenges is meant to be as easy as executing `cmgr test` from the
directory of an individual challenge or the directory containing all of the
challenges for an event.  This is intended to support quick feedback cycles
for developers as well as enabling automated quality control during the
preparation for an event.

### Front-Ends

Another design of this project is to make it easier for custom front-end
interfaces for CTFs to reuse existing content/challenges rather than forcing
organizers to port between systems.  To make this possible, `cmgrd` exposes a
very simple REST API which allows a front-end to manage all of the important
tasks of running a competition or training environment.  The OpenAPI specification
can be found [here](cmd/cmgrd/swagger.yaml).

### Back-End

If you're interested in contributing, modifying, or extending **cmgr**, the
core functionality of the project is implemented in a single Go library under
the `cmgr` directory.
Additionally, the _SQLite3_ database is intended to function as a read-only
API and its schema can be found [here](cmgr/database.go).

In order to work on the back-end, you will need to have _Go_ installed and
_cgo_ enabled for at least the initial build where the _sqlite3_ driver is
built and installed.  To get started, you can run:

```sh
git clone https://github.com/picoCTF/cmgr
cd cmgr
go get -v -t -d ./...
mkdir bin
go build -v -o bin ./...
go test -v ./...
```

## Acknowledgments

This project is heavily inspired by the
[picoCTF](https://github.com/picoCTF/picoCTF) platform and seeks to be a next
generation implementation of the _hacksport_ back-end for CTF platforms built
in its style.

## Contributing

Please carefully read the [NOTICE](Notice), [CONTRIBUTING](CONTRIBUTING.md),
[DISCLAIMER](DISCLAIMER.md), and [LICENSE](LICENSE) files for details on how
to contribute as well as the copyright and licensing situations when
contributing to the project.
