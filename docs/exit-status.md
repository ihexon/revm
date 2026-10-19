# Exit status

Only the CLI entry point exits the host process. Invalid invocations return 2
and one diagnostic on stderr; runtime failures return 1. Explicit help writes
to stdout and succeeds. Invalid options never panic or create a session.

Guest exit codes are preserved by run and exec, including 126 (not executable)
and 127 (not found). Signal termination is recorded by name and mapped to 128
plus the Linux signal number, independently of the host platform. A missing
guest result is an error, never an inferred success. A nonzero guest result
takes precedence over a simultaneous cleanup failure; successful execution with
failed cleanup returns nonzero.

The default VM allocation is up to four host CPUs and one quarter of available
memory, bounded to 512–2048 MiB. Explicit memory values below 512 MiB are errors.

The first host cancellation takes precedence over signals used later for cleanup.
A downstream pipe closing while execution is active returns 141 (SIGPIPE), after
bounded cleanup. Late signals accelerate cleanup without replacing a known
command result. Joined error diagnostics are deduplicated onto one stderr line;
human wording is not a parsing contract. Use exit status, `ls --json`, `inspect`,
and `ctl --list-port --json` for automation.
