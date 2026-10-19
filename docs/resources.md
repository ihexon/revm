# Resource ownership

revm uses XDG paths on both Linux and macOS:

| Kind | Default | Removed by |
| --- | --- | --- |
| Runtime sockets | `$XDG_RUNTIME_DIR/revm/<instance>`; private `/tmp/revm-<uid>` fallback | instance exit, `cache prune` |
| Private writable rootfs | `${XDG_CACHE_HOME:-~/.cache}/revm/instances/<instance>` | instance exit, `cache prune` |
| Session config, results and logs | `${XDG_STATE_HOME:-~/.local/state}/revm/sessions/<id>` | `rm` |
| Persistent volumes | `${XDG_DATA_HOME:-~/.local/share}/revm/volumes/<uuid>` | `volume rm NAME` |

Roots must be absolute and must not overlap. Long runtime paths use the short
fallback so Unix sockets fit on macOS. Resource directories are private. Each
start gets a new instance ID and a freshly extracted writable rootfs; changes to
guest system directories never carry into another run. Bind mounts and external
disks retain their normal host persistence.

`revm podman --id dev` reuses the independent `podman-dev` volume. Deleting the
session does not delete the volume or its association with `dev`. `volume ls`
(and `--json`) lists volumes; `volume rm podman-dev` removes only that unused
managed volume. Existing session references then report missing data, rather
than silently constructing an empty replacement.

`--raw-disk` and `--container-disk` paths remain user-owned even when revm creates
them. Existing images are inspected; mismatched version labels return an error.
Missing labels never authorize deletion. New disks are published without
replacing an existing target. Session and volume cleanup never delete external
disks or bind mount contents.

Session, volume and external-disk locks cover the entire VMM lifetime. Lock
files are stable and never unlinked; the VMM inherits the open locks, so a killed
CLI cannot release storage still being used by its child. `cache prune` skips
active instances and only removes resources with matching ownership records.
Unclassified legacy directories and all persistent data are preserved.

Old cache layouts are not imported or used. They remain untouched. Select an
existing external disk explicitly with `--container-disk`; revm does not maintain
migration records or reconnect disks through legacy session names.
