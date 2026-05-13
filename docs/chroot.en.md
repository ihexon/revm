# chroot mode

`chroot` mode starts an isolated Linux command environment quickly. It feels like a local command-line workflow, but gives builds, tests, scripts, and debugging tasks a cleaner runtime.

## Who It Is For

- Developers who want to run builds and tests in a clean Linux environment.
- Engineers who need a disposable Linux shell for quick checks.
- Tool builders who need to inspect rootfs content, validate scripts, or reproduce user environments.
- Teams building CI helpers, local developer platforms, or sandboxed command execution.

## Common Scenarios

### Scenario 1: Keep Build Dependencies Off The Host

Mount the current project directory and run tests inside Linux:

```bash
./chroot --id build \
  --mount "$PWD:/workspace" \
  --workdir /workspace \
  -- sh -c 'make test'
```

This helps when:

- Host dependencies make build results unstable.
- Team members use different systems and get different results.
- A task needs a clean environment that can be discarded after use.

### Scenario 2: Open A Temporary Linux Shell

Start a shell:

```bash
./chroot --id debug -- sh
```

Use it to verify commands, run scripts, or check Linux behavior without maintaining a long-lived VM.

### Scenario 3: Reproduce An Environment With Your Own rootfs

If you already have a rootfs, pass it directly:

```bash
./chroot --id ubuntu --rootfs ~/ubuntu-rootfs -- bash
```

This is useful for reproducing a specific distribution, dependency set, or production-like environment.

### Scenario 4: Run Tasks In Automation

Use it inside scripts or CI helpers:

```bash
./chroot --id ci \
  --cpus 4 \
  --memory 4096 \
  --mount "$PWD:/src,ro" \
  --workdir /src \
  -- sh -c './ci/test.sh'
```

This turns environment setup and task execution into one repeatable command.

## Quick Start

Run a command with the built-in Linux environment:

```bash
./chroot --id quick -- sh -c 'uname -a && cat /etc/os-release'
```

Open an interactive shell:

```bash
./chroot --id shell -- sh
```

Mount the current project directory:

```bash
./chroot --id dev \
  --mount "$PWD:/workspace" \
  --workdir /workspace \
  -- sh
```

## Core Capabilities

- Use the built-in Linux environment or switch to your team's own rootfs.
- Mount host project directories and run builds or tests inside the isolated environment.
- Set work directories, environment variables, and resource limits per project.
- Reuse host proxy settings when downloading dependencies.
- Emit logs for scripts, CI systems, and internal platforms.

## Recommended Usage

For quick checks, use the built-in environment:

```bash
./chroot --id test -- sh
```

For team builds or tests, pin the rootfs, work directory, and resource settings:

```bash
./chroot --id project-test \
  --rootfs ~/project-rootfs \
  --cpus 4 \
  --memory 4096 \
  --mount "$PWD:/workspace" \
  --workdir /workspace \
  -- sh -c 'make test'
```

This gives everyone a more consistent command and runtime.
