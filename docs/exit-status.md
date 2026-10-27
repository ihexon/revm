# Exit status

Invalid invocations return 2 and one diagnostic on stderr; runtime failures
handled by revm return 1. Explicit help writes
to stdout and succeeds. Invalid options never panic or create a session.

Guest exit codes are preserved by run and exec, including 126 (not executable)
and 127 (not found). Ordinary guest nonzero exits do not add revm diagnostics
to stderr. Startup and transport errors handled by revm report their cause.
Failed temporary runs retain logs and results; revm failures include commands
for inspecting them. Startup failures also include a bounded guest log tail
when available. Signal termination is recorded by name and mapped to 128
plus the Linux signal number, independently of the host platform.

libkrun can end the process directly with `_exit`. revm does not intercept it:
the shell receives libkrun's native status, which may differ from the command's
status. Without a command result, the session is `unknown` and `revm wait`
returns 1. Its retained guest log contains boot/service diagnostics.

Cleanup happens in a separate process after the VM process exits. Cleanup
failures are recorded before releasing the session lock; `revm wait` observes
them, but they cannot change the exit status already returned to the shell.
A nonzero guest result takes precedence over a cleanup failure; otherwise a
cleanup failure makes the recorded status 1.

The default VM allocation is up to four host CPUs and one quarter of available
memory, bounded to 512–2048 MiB. Explicit memory values below 512 MiB are errors.

The first host cancellation returns `128+signal` immediately, even during output
draining. A downstream pipe closing while execution is active returns 141
(SIGPIPE). There is no VM shutdown, escalation timer or automatic filesystem
sync; applications own flushing their data. Joined error diagnostics are
deduplicated onto one stderr line;
human wording is not a parsing contract. Use exit status, `ls --json`, `inspect`,
and `ctl --list-port --json` for automation.
