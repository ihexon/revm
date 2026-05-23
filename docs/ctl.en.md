# revm ctl

`revm ctl` controls the control plane of an existing session. It never starts a new VM and never executes guest commands.

Supported control operations:

- `--list-port`: list current gvproxy port mappings.
- `--port-export`: expose a guest TCP port on the host.
- `--port-unexport`: remove a host port exposure.
- `--export-rootfs`: export the rootfs from the session directory to a host tar.zst file.

Use [`revm attach`](./attach.en.md) to connect to the guest or execute commands.

## Usage

```bash
revm ctl --id <session-id> --list-port
revm ctl --id <session-id> --port-export <spec>
revm ctl --id <session-id> --port-unexport <spec>
revm ctl --id <session-id> --export-rootfs <path.tar.zst>
```

`--id` must reference a running session.

## Export Rootfs

Export the rootfs from the session directory to a tar.zst file:

```bash
revm ctl --id web --export-rootfs ./rootfs.tar.zst
```

The archive is rooted at the rootfs contents and does not add an extra `rootfs/` directory. The output path cannot be inside that session rootfs directory.

## List Ports

List every current port mapping:

```bash
revm ctl --id web --list-port
```

The output includes revm's internal SSH forward, container port publishing, and manually exposed ports:

```text
PROTOCOL  HOST            GUEST
tcp       127.0.0.1:6123  192.168.127.2:22
tcp       127.0.0.1:8080  192.168.127.2:8000
```

## Expose Ports

Expose a guest port:

```bash
revm ctl --id web --port-export 127.0.0.1:8080:8000
curl http://127.0.0.1:8080
```

Remove the exposure:

```bash
revm ctl --id web --port-unexport 127.0.0.1:8080
```

Update multiple ports at once:

```bash
revm ctl --id web \
  --port-export 127.0.0.1:8080:8000 \
  --port-export 127.0.0.1:8443:8443
```

Port formats:

```text
--port-export [tcp:]<host-port>:<guest-port>
--port-export [tcp:]<host-ip>:<host-port>:<guest-port>
--port-unexport [tcp:]<host-port>
--port-unexport [tcp:]<host-ip>:<host-port>
```

When host IP is omitted, `127.0.0.1` is used. Only TCP and IPv4 are currently supported.

## How It Works

`revm ctl` first reads VM metadata from the session management API:

```text
~/.cache/revm/<session-id>/socks/vmctl.sock
```

Port updates read the gvproxy control endpoint from the management API, then call the gvproxy forwarder API:

```text
/services/forwarder/expose
/services/forwarder/unexpose
```

Because of that, port updates require gvisor networking. `tsi` networking does not support `--port-export` or `--port-unexport`.

## Invalid Usage

This fails because no control operation is selected:

```bash
revm ctl --id dev
```

This fails because `ctl` does not execute guest commands:

```bash
revm ctl --id dev -- sh
```

Use this instead:

```bash
revm attach --id dev -- sh
```
