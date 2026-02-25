# Management API

The management API is served over a Unix domain socket (`vmCTLAddress` in the VM config). It provides host-side control of a running VM.

**Base URL:** `unix://<workspace>/vm-ctl.sock`

All responses use `Content-Type: application/json` unless noted otherwise.

---

## Endpoints

### `GET /healthz`

Returns 200 if the server is running.

**Response:** `200 OK`, empty body.

**Example:**
```sh
curl --unix-socket /tmp/.revm-xxx/vm-ctl.sock http://localhost/healthz
```

---

### `GET /stop`

Requests a graceful VM shutdown. Returns immediately; the VM stops asynchronously.

**Response:** `200 OK`, empty body.

**Example:**
```sh
curl --unix-socket /tmp/.revm-xxx/vm-ctl.sock http://localhost/stop
```

---

### `POST /exec`

Executes a command inside the guest VM and streams its output via [Server-Sent Events (SSE)](https://developer.mozilla.org/en-US/docs/Web/API/Server-sent_events).

**Request body:** `application/json`

| Field  | Type       | Required | Description                          |
|--------|------------|----------|--------------------------------------|
| `bin`  | `string`   | Yes      | Absolute path to the executable      |
| `args` | `string[]` | No       | Command-line arguments               |
| `env`  | `string[]` | No       | Extra environment variables (`KEY=VALUE`) |

**Response:** `200 OK`, `Content-Type: text/event-stream`

The response is an SSE stream. Each event has one of the following types:

| Event type | Meaning                                              |
|------------|------------------------------------------------------|
| `out`      | A line from the command's stdout                     |
| `error`    | A line from stderr, or an execution error message    |
| `done`     | Command exited successfully; stream ends after this  |

If the command fails to start, a single `error` event is sent and the stream closes.

**Example — run `uname -a` in the guest:**
```sh
curl --unix-socket /tmp/.revm-xxx/vm-ctl.sock \
  -X POST http://localhost/exec \
  -H 'Content-Type: application/json' \
  -d '{"bin":"/bin/uname","args":["-a"]}' \
  --no-buffer
```

**Example response stream:**
```
event: out
data: Linux (none) 6.1.0 #1 SMP PREEMPT Mon Jan 1 00:00:00 UTC 2024 aarch64 GNU/Linux

event: done
data: done
```

**Example — command that writes to stderr:**
```
event: error
data: /bin/sh: unknown option: -z

event: error
data: wait: exit status 2
```

---

### `GET /info` *(OVMode only)*

Returns connection info for Podman and SSH proxies. Only available when the VM is running in OVMode (`runMode: "oomol-studio"`).

**Response:** `200 OK`

```json
{
  "podmanSocketPath": "/tmp/.revm-abc123/podman.sock",
  "sshPort": 6123,
  "sshUser": "root",
  "hostEndpoint": "host.containers.internal"
}
```

| Field             | Description                                              |
|-------------------|----------------------------------------------------------|
| `podmanSocketPath`| Unix socket path for the Podman API proxy on the host    |
| `sshPort`         | Host port forwarding SSH into the guest                  |
| `sshUser`         | Default SSH user inside the guest                        |
| `hostEndpoint`    | DNS name resolving to the host from within the VM (gvisor mode only) |

---

## Error responses

All endpoints return a JSON error body on failure:

```json
{ "error": "<message>" }
```

| Status | Meaning                        |
|--------|--------------------------------|
| `405`  | Method not allowed             |
| `400`  | Invalid request body (`/exec`) |
| `500`  | Internal server error          |
