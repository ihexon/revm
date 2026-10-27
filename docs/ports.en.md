# revm ports

```sh
revm ports SESSION
revm ports --json SESSION
```

List current TCP mappings, including startup forwards and ports published by Podman.

```sh
# Terminal 1: HOST_PORT:GUEST_PORT, listening on 127.0.0.1 by default
revm run --id demo --port-export 8080:8000 -- nc -lk -p 8000 -e cat

# Terminal 2
revm ports demo
printf 'hello\n' | nc -w 1 127.0.0.1 8080
```

Repeat `--port-export` for multiple mappings. `HOST_IP:HOST_PORT:GUEST_PORT` is also accepted;
only TCP and IPv4 are supported. Restart the session to change startup forwards.
Publish container ports with `podman run -p HOST_PORT:CONTAINER_PORT`.
