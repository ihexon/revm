# Resource ownership

revm uses XDG paths on both Linux and macOS:

| Kind | Default | Removed by |
| --- | --- | --- |
| Runtime sockets | `$XDG_RUNTIME_DIR/revm/<instance>`; private `/tmp/revm-<uid>` fallback | instance exit, `cache prune` |
| Private writable rootfs | `${XDG_CACHE_HOME:-~/.cache}/revm/instances/<instance>` | instance exit, `cache prune` |
| Session config, results and logs | `${XDG_STATE_HOME:-~/.local/state}/revm/sessions/<id>` | `rm` |
| Persistent volumes | `${XDG_DATA_HOME:-~/.local/share}/revm/volumes/<name>` | `volume rm NAME` |

Roots must be absolute and must not overlap. Long runtime paths use the short
fallback so Unix sockets fit on macOS. Resource directories are private. Each
start gets a new instance ID and a freshly extracted writable rootfs; changes to
guest system directories never carry into another run. Bind mounts and external
disks retain their normal host persistence.

Create storage with `revm volume create data`, then select it with
`revm podman --volume data`. Every launch requires `--volume NAME` or an external
`--container-disk PATH`. A session records the volume used by its latest run for
inspection; that record never selects or recreates storage. Volumes have no
session association. `volume ls` shows the disk path, whether it is in use,
logical capacity and host-allocated space. `volume ls --json` reports `path`,
`inUse`, `sizeBytes` and `allocatedBytes`; unknown sizes are null with an `error`.
Allocated space is the host file's allocated blocks, not guest filesystem usage
or reclaimable space (shared/cloned blocks may still be counted). `volume rm NAME`
deletes an unused volume. Deleting a session preserves all volumes.

Each volume is stored under its name. A damaged volume is listed with an error
and does not block other volumes; use `volume rm NAME` to explicitly delete it.
Earlier UUID-based volume directories are not converted automatically. Their
disk files remain usable through `--container-disk PATH`.

Omit `--id` to remove session records after successful execution and cleanup.
Failures, interruptions and crashes retain logs and results until `rm`. Supply
`--id NAME` to retain records even after success. `cache prune` removes only
inactive runtime/cache resources, preserving all diagnostic records.

`--raw-disk` and `--container-disk` paths remain user-owned even when revm creates
them. Existing images are inspected; invalid filesystems return an error without
replacing data. New disks are published without
replacing an existing target. Session and volume cleanup never delete external
disks or bind mount contents.

The main revm process hosts libkrun. Its exit releases the VM; a detached cleanup
helper then removes owned runtime/cache files. Session, volume and external-disk
locks cover both VM execution and cleanup. Lock files are stable and never
unlinked; the helper inherits them and releases them after cleanup. Use
`revm wait ID` to wait for that release before reusing named sessions or disks.
`cache prune` skips
active instances and only removes resources with matching ownership records.
Unclassified legacy directories and all persistent data are preserved.

The VM process sets its host `TMPDIR` to the instance runtime directory, so
native-library temporary sockets are removed by the same cleanup.

Old cache layouts are not imported or used. They remain untouched. Select an
existing external disk explicitly with `--container-disk`; revm does not maintain
migration records or reconnect disks through legacy session names.
