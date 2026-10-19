# Exit status

Only the CLI entry point exits the host process. Invalid invocations return 2
and one diagnostic on stderr; runtime failures return 1. Explicit help writes
to stdout and succeeds. Invalid options never panic or create a session.

The default VM allocation is up to four host CPUs and one quarter of available
memory, bounded to 512–2048 MiB. Explicit memory values below 512 MiB are errors.
