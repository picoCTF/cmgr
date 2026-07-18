# Challenges

## problem.md

This file specifies a challenge's metadata (name, description, etc.) For a detailed breakdown of the format, see the [specification](specification.md).

## Supported Challenge Types

### custom

In this challenge type, the author must supply a complete Dockerfile.  The Dockerfile will be supplied with three build arguments when the challenge is built: `FLAG`, `SEED`, and `FLAG_FORMAT`.

The Dockerfile is responsible for using these inputs to build the templated challenge and format the image appropriately for _cmgr_ to retrieve the artifacts and build metadata.  In particular, any artifacts competitors should see **must** be in a GZIP-ed tar archive located at `/challenge/artifacts.tar.gz`.  Additionally, there **must** be a `/challenge/metadata.json` file that has a field for the flag (named `flag`) as well as any other lookup values the challenge references in its details and hints. These files must be present in either the final build stage, or in a stage explicitly named `builder`.

You can find an example [here](custom/).  The ["multi"](multi/) challenge example demonstrates the full range of customization you can leverage by demonstrating multi-container challenges and custom per-build lookup values.

#### Publishing ports

Docker has a distinction between "exposed" ports and "published" ports. `cmgr` detects which exposed
ports should be published by requiring a comment of the form `# PUBLISH {port} AS {name}` (case
sensitive) to occur in the Dockerfile after `EXPOSE` directives.  This allows challenge authors
to bring in base images that already expose ports in Docker (e.g., the PostgreSQL image) without
neccessarily exposing those ports to competitors.

#### Launching more than one container

In order to support challenges that launch multiple containers for a
challenge, `cmgr` introduces a comment of the form `# LAUNCH {build_stage} ...`
which will launch an instance of each listed stage with the stage name as
its Docker DNS name and place them on the same overlay network.  For a
specific example of this, see the [multi example](./multi).  When using
multiple containers, it is important that each `# PUBLISH` comment (described above)
appears in the same build stage as the `EXPOSE` directive it is referencing.

### Compiled Challenges

In the `remote-make` and `static-make` challenge types, the build process will call `make main`, `make artifacts.tar.gz`, and `make metadata.json` in that order to build the challenge and necessary components.  Additionally, challenges with a network component will have `make run` called to start as the entrypoint.

#### remote-make

The `remote-make` challenge type will take a program that uses stdin/stdout to communicate and connect it to a port so that every new TCP connection gets forked into a new process with stdin/stdout piped to the network.

#### static-make

The `static-make` challenge type has no network component and should be solvable solely by using the `artifacts.tar.gz` and `metadata.json` created during the build process.

## Delivery Types

Independent of the challenge type above (which controls how a challenge is *built*), `cmgr` derives a `delivery_type` for every challenge that describes what competitors actually receive:

- **`service`** — the challenge publishes at least one port (via `# PUBLISH`); a running container serves competitors.  Whether it runs as a shared persistent instance or on-demand per user is decided by the schema's `instance_count`, not by the challenge.
- **`artifact_only`** — the challenge publishes no ports; the `artifacts.tar.gz` produced at build time is the entire challenge and no running container is needed.  All `static-make` challenges are artifact-only, as is any `custom` challenge without a `# PUBLISH` directive.
- **`flag_only`** — reserved for an upcoming challenge type for bare flag-submission challenges (no ports, no artifacts); not yet available.

This value is derived from the Dockerfile — authors never write it, and it is reported through the `cmgrd` API so front-ends can distinguish these cases explicitly.

Two things follow from this that challenge authors should know:

- A challenge that publishes no ports **should** produce a non-empty `artifacts.tar.gz`; if it produces neither, the build logs a warning since this usually indicates a forgotten `# PUBLISH` directive (description-only challenges that build solely to generate a flag are the exception, and will eventually declare this via a dedicated challenge type).  `cmgr update` also tags each non-service challenge with its delivery type and prints a summary, so an unexpectedly artifact-only challenge is visible before deployment.
- Solvers for artifact-only challenges run against the build's artifacts directly (on Docker's default network — no `challenge` host, outbound access preserved) rather than alongside a running instance, and `cmgr test` skips the start/stop steps for them.
- **Keep a placeholder entrypoint (e.g. `CMD tail -f /dev/null`) in custom artifact-only Dockerfiles for now.**  cmgr currently still launches schema-managed instances for artifact-only challenges, and a container whose image has no `CMD` fails to launch.  A future release will stop launching instances for them entirely, after which the placeholder becomes unnecessary.

## Schemas

"Schemas" are a mechanism for declaratively specifying the desired state for a set of builds and instances.  Builds and the associated instances that are created by a schema are locked out from manual control and should be the preferred way to manage a large number of builds and instances for events.  However, they are still event agnostic and can be used for managing other groupings of resources as appropriate.  An example schema can be found [here](./schema.yaml).  It is worth noting that a `-1` for instance count specifies that instances are manually controlled and allows the CLI or `cmgrd` to dynamically increase or decrease the number of running instances (useful for mapping instances uniquely to end-users without having a large number of unused containers).  `instance_count` is only meaningful for challenges with a `service` delivery type; artifact-only challenges listed in a schema get their builds (one per seed, with artifacts and flags), and a future release will stop launching the placeholder instances that are currently still created for them.
