# revm ctl

revm ctl controls the host-side network control plane of a running session. It does not start a VM, execute a guest command, or modify a rootfs.

The only operations are:

- --list-port: list current gvisor port mappings.
- --port-export: expose a guest TCP port on the host.
- --port-unexport: remove a host port exposure.

Use [revm attach](attach.en.md) for guest commands. Rootfs is outside ctl's scope; rootfs import, export, and --rootfs were removed.

## Syntax

~~~text
revm ctl --id <session-id> --list-port
revm ctl --id <session-id> --port-export <spec> [--port-export <spec>...]
revm ctl --id <session-id> --port-unexport <spec> [--port-unexport <spec>...]
~~~

--id must reference a running session using gvisor networking. One invocation selects either list or port updates; one update invocation may contain both exports and unexports.

## List ports

~~~bash
revm ctl --id web --list-port
~~~

Example output:

~~~text
PROTOCOL  HOST            GUEST
tcp       127.0.0.1:6123  192.168.127.2:22
tcp       127.0.0.1:8080  192.168.127.2:8000
~~~

The list can include:

- revm's internal SSH forwarding used by attach;
- ports published by Podman containers with -p;
- ports created manually with ctl.

The list comes from gvproxy and fails when the gvisor session is not running.

## Export and unexport

Expose guest port 8000 as host port 8080:

~~~bash
revm ctl --id web --port-export 127.0.0.1:8080:8000
curl http://127.0.0.1:8080
~~~

When the host IP is omitted, 127.0.0.1 is used:

~~~bash
revm ctl --id web --port-export 8080:8000
~~~

Remove an exposure with its host endpoint:

~~~bash
revm ctl --id web --port-unexport 127.0.0.1:8080
~~~

Update several ports in one call:

~~~bash
revm ctl --id web \
  --port-export 127.0.0.1:8080:8000 \
  --port-export 127.0.0.1:8443:8443 \
  --port-unexport 127.0.0.1:9000
~~~

Formats:

~~~text
--port-export [tcp:]<host-port>:<guest-port>
--port-export [tcp:]<host-ip>:<host-port>:<guest-port>
--port-unexport [tcp:]<host-port>
--port-unexport [tcp:]<host-ip>:<host-port>
~~~

Only TCP and IPv4 are accepted today. Ports must be 1 through 65535. A host port cannot conflict with revm's internal SSH forwarding or be duplicated in one update batch.

## How it works

ctl reads the VM view from the session management API:

~~~text
~/.cache/revm/<session-id>/socks/vmctl.sock
~~~

The management API returns the gvproxy control endpoint. ctl then calls gvproxy's expose or unexpose forwarder API. No guest command is needed.

## Invalid usage

No operation is an error:

~~~bash
revm ctl --id dev
~~~

ctl does not execute guest commands:

~~~bash
revm ctl --id dev -- sh
~~~

Use:

~~~bash
revm attach --id dev -- sh
~~~

A tsi session does not support list or port updates:

~~~bash
revm run --id light --network tsi -- sh
revm ctl --id light --list-port
~~~

## Options

| Option | Description |
| --- | --- |
| --id | Required session name. |
| --list-port | List gvisor mappings. |
| --port-export | Create a TCP/IPv4 mapping; repeatable. |
| --port-unexport | Remove a TCP/IPv4 mapping; repeatable. |
| --log-level | Log level. |
