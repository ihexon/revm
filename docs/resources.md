# Resource ownership

On both Linux and macOS, revm keeps its managed data under `~/.revm`. Set
`REVM_HOME` to an absolute path to select a different root. XDG variables do not
change this layout:

| Kind | Default | Removed by |
| --- | --- | --- |
| Runtime sockets | `$REVM_HOME/run/<instance>` | instance exit, `cache prune` |
| Private writable rootfs | `$REVM_HOME/cache/instances/<instance>` | instance exit, `cache prune` |
| Session config, results and logs | `$REVM_HOME/state/sessions/<id>` | `rm` |
| Persistent volumes | `$REVM_HOME/data/volumes/<name>` | `volume rm NAME` |

Here `$REVM_HOME` means the selected home, defaulting to `~/.revm`. Session and
volume lock files live under `state/locks` and `data/locks` respectively; build dependency caches use
`cache/build/v1`. Build outputs remain in the requested checkout under `out`.

Resource subdirectories must stay inside the selected home and must not overlap,
including through symbolic links. If the home is too long for Unix sockets,
startup fails with a request to choose a shorter `REVM_HOME`; it never falls back
to another directory. Resource directories are private. Each
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
locks cover both VM execution and cleanup. External disks are locked by inode,
so another home or hard link cannot bypass the lock. Session and volume lock
files are stable and never unlinked; the helper inherits them and releases them after cleanup. Use
`revm wait ID` to wait for that release before reusing named sessions or disks.
`cache prune` skips
active instances and only removes resources with matching ownership records.
Unclassified legacy directories and all persistent data are preserved.

The VM process sets its host `TMPDIR` to the instance runtime directory, so
native-library temporary sockets are removed by the same cleanup.

Old XDG and cache layouts are not automatically imported or used. Stop old
instances before switching versions or data homes. They remain untouched. Select an
existing external disk explicitly with `--container-disk`; revm does not maintain
migration records or reconnect disks through legacy session names.
