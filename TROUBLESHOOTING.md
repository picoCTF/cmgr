# Troubleshooting

## Getting Docker working with `firewalld` (Fedora/CentOS/RHEL)
```
firewall-cmd --zone=public --add-masquerade --permanent
firewall-cmd --zone=trusted --change-interface=docker 0 --permanent
firewall-cmd --reload
```

## Receiving Docker network errors

**Symptom:**  Starting of challenge instances fails with the message
"ERROR:  could not create challenge network (cmgr-...): Error response from
daemon:  could not find an available, non-overlapping IPv4 address pool
among the defaults to assign to the network.

**Cause:** Docker has exhausted all available subnets that it has been
assigned and cannot create anymore.  By default, Docker only reserves 31
distinct subnets which constrains `cmgr` to no more than that number of running
challenge instances (each instance gets a network).

**Solution:** Choose a sufficiently large region of RFC 1918 address space
and update the Docker daemon's configuration (`/etc/docker/daemon.json`) to
allot more default networks.  It is important to ensure that these addresses
are not in use by another network segment and that the individual subnets are
large enough to handle any multi-host challenges (to include a solver host and
the default gateway).

An example configuration which carves the address space into \~2 million subnets
is:
```json
{
  "default-address-pools":
    [
      {"base":"10.0.0.0/8", "size":29}
    ]
}
```

**Note:** You will need to restart the daemon after changing its configuration.

## 404 errors when stopping instances (`unknown instance id`)

**Symptom:** An external service (e.g., a Celery task) that periodically calls
`DELETE /instances/<id>` receives a 404 / "unknown instance id" error, even
though the instance was recently active.

**Cause:** `cmgr` automatically prunes on-demand instances from its database
when they are older than `CMGR_PRUNE_AGE` (default: `1h`). If the prune age
is shorter than your external stop interval the DB row is removed before the
scheduled stop request arrives. This is harmless at the container layer (the
running containers are also reaped externally), but the REST call will fail.

**Solution:** Set `CMGR_PRUNE_AGE` to a value strictly greater than your
external stop interval. For example, if your Celery task stops instances every
15 minutes, a value of `1h` (the default) or greater is safe.

```sh
export CMGR_PRUNE_AGE=2h
```

Alternatively, set `CMGR_PRUNE_AGE=0` to disable automatic pruning entirely
and rely solely on explicit stop calls to clean up instances.
