"""Shared cleanup for isolated real-VM test homes."""
import fcntl
from pathlib import Path
import subprocess
import time


def prune_after_cleanup(binary, env):
    # Call after every VM owner has exited. Helpers inherit the session locks
    # and may still write records; cache prune intentionally skips those runs.
    home = Path(env.get('REVM_HOME') or Path(env['HOME']) / '.revm')
    deadline = time.monotonic() + 15
    for path in (home / 'state/locks').glob('session-*.lock'):
        with path.open('rb') as lock:
            while True:
                try:
                    fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
                    break
                except BlockingIOError:
                    assert time.monotonic() < deadline, ('cleanup still owns session', path)
                    time.sleep(.02)
    subprocess.run([str(binary), 'cache', 'prune'], env=env, capture_output=True,
                   check=True, timeout=10)
