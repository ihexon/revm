#!/usr/bin/env python3
"""Process death, cleanup ownership and persistence on a VM-capable host."""
import json
import os
from pathlib import Path
import signal
import subprocess
import tempfile
import time

binary = os.path.abspath('out/revm/bin/revm')
root = tempfile.TemporaryDirectory(prefix='revm-lifecycle-')
env = dict(os.environ, XDG_STATE_HOME=root.name+'/state',
           XDG_CACHE_HOME=root.name+'/cache', XDG_DATA_HOME=root.name+'/data')
processes = []


def cli(args, code=0):
    p = subprocess.run([binary]+args, env=env, capture_output=True, timeout=25)
    assert p.returncode == code, (args, p.returncode, p.stdout, p.stderr)
    return p


def run_args(sid, command, extra=()):
    return ['run', '--id', sid, '--cpus', '1', '--memory', '512', *extra, '--', *command]


def inspect(sid):
    return json.loads(cli(['inspect', sid]).stdout)


def assert_clean(view):
    assert not view['active'], view
    for key in ['runtimeDir', 'cacheDir']:
        assert not os.path.exists(view['lastRun'][key]), view
    assert os.path.isdir(view['logDir']), view


try:
    # This failure occurs inside the guest before the execution service is ready.
    at = time.monotonic()
    p = cli(run_args('boot-failure', ['true'],
                    ['--mount', root.name+':/dev/null/subdir']), 1)
    assert time.monotonic()-at < 10, 'native exit waited for readiness timeout'
    cli(['wait', 'boot-failure'], 1)
    view = inspect('boot-failure')
    assert view['status'] == 'unknown' and view['lastRun'].get('exitCode') is None, view
    assert_clean(view)
    assert b'not a directory' in cli(['logs', '--guest', 'boot-failure']).stdout
    print('PASS native startup failure exits promptly, preserves diagnostics and cleans resources', flush=True)

    # The command owns flushing; revm only preserves the external disk file.
    disk = root.name+'/persistent.ext4'
    extra = ['--raw-disk', disk+',mnt=/data']
    cli(run_args('write', ['sh', '-c', 'echo keep-me > /data/marker; sync'], extra))
    cli(['wait', 'write'])
    assert_clean(inspect('write'))
    assert os.path.isfile(disk)
    assert cli(run_args('read', ['cat', '/data/marker'], extra)).stdout == b'keep-me\n'
    cli(['wait', 'read'])
    cli(['rm', 'write'])
    cli(['rm', 'read'])
    assert os.path.isfile(disk)
    print('PASS explicitly synced data survives; session cleanup preserves external disks', flush=True)

    # Losing the only cleanup owner must abort the VM, not leave it unowned.
    owner = subprocess.Popen([binary]+run_args('helper-failure', ['sh', '-c', 'echo ready; sleep 100']),
                             env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    processes.append(owner)
    assert owner.stdout.readline() == b'ready\n'
    runtime = inspect('helper-failure')['lastRun']['runtimeDir']
    assert list(Path(runtime).glob('krun-net-*.sock')), runtime
    children = subprocess.check_output(['pgrep', '-P', str(owner.pid)]).split()
    assert len(children) == 1, children
    child = int(children[0])
    command = subprocess.check_output(['ps', '-p', str(child), '-o', 'command=']).decode()
    assert command.rstrip().endswith(' _cleanup'), command
    at = time.monotonic()
    os.kill(child, signal.SIGKILL)
    _, error = owner.communicate(timeout=3)
    assert owner.returncode == 1 and b'cleanup helper' in error, (owner.returncode, error)
    assert time.monotonic()-at < 3
    cli(['wait', 'helper-failure'], 1)
    cli(['cache', 'prune'])
    assert_clean(inspect('helper-failure'))
    print('PASS cleanup helper failure aborts the VM; cache prune recovers temporary resources', flush=True)
finally:
    for p in processes:
        if p.poll() is None:
            p.kill()
            p.communicate(timeout=5)
    subprocess.run([binary, 'cache', 'prune'], env=env, capture_output=True, timeout=10)
    root.cleanup()
