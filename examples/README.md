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

## Schemas

"Schemas" are a mechanism for declaratively specifying the desired state for a set of builds and instances.  Builds and the associated instances that are created by a schema are locked out from manual control and should be the preferred way to manage a large number of builds and instances for events.  However, they are still event agnostic and can be used for managing other groupings of resources as appropriate.  An example schema can be found [here](./schema.yaml).  It is worth noting that a `-1` for instance count specifies that instances are manually controlled and allows the CLI or `cmgrd` to dynamically increase or decrease the number of running instances (useful for mapping instances uniquely to end-users without having a large number of unused containers).
