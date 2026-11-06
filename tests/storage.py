#!/usr/bin/env python3
"""All managed files stay in the default or explicitly selected revm home."""
from support import prune_after_cleanup
import json
import os
from pathlib import Path
import select
import subprocess
import tempfile

binary = str(Path('out/revm/bin/revm').resolve())

for custom in (False, True):
    with tempfile.TemporaryDirectory(prefix='rv-', dir='/var/tmp') as temp:
        root = Path(temp).resolve()
        user_home = root / 'user'
        user_home.mkdir()
        data_home = root / 'custom' if custom else user_home / '.revm'
        env = dict(os.environ, HOME=str(user_home), TMPDIR=str(root / 'tmp'))
        env.pop('REVM_HOME', None)
        if custom:
            env['REVM_HOME'] = str(data_home)
        for key in ('XDG_STATE_HOME', 'XDG_CACHE_HOME', 'XDG_DATA_HOME', 'XDG_RUNTIME_DIR'):
            env[key] = str(root / key)

        def cli(args, code=0):
            p = subprocess.run([binary] + args, env=env, capture_output=True, timeout=30)
            assert p.returncode == code, (args, p.returncode, p.stdout, p.stderr)
            return p

        owner = None
        try:
            cli(['volume', 'create', 'data'])
            volume = json.loads(cli(['volume', 'ls', '--json']).stdout)[0]
            assert Path(volume['path']) == data_home / 'data/volumes/data/disk.ext4', volume
            owner = subprocess.Popen(
                [binary, 'run', '--id', 'storage-test', '--cpus', '1', '--memory', '512',
                 '--', 'sh', '-c', 'echo READY; exec sleep 300'],
                env=env, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
            assert select.select([owner.stdout], [], [], 20)[0], 'VM not ready'
            assert owner.stdout.readline() == b'READY\n'
            view = json.loads(cli(['inspect', 'storage-test']).stdout)
            for path in (view['logDir'], view['lastRun']['runtimeDir'], view['lastRun']['cacheDir']):
                assert Path(path).is_relative_to(data_home), path
                assert Path(path).is_dir(), path
            assert (Path(view['logDir']) / 'revm.log').is_file()
            assert (Path(view['logDir']) / 'vm.log').is_file()
            runtime = Path(view['lastRun']['runtimeDir'])
            assert list(runtime.glob('krun-net-*.sock')), runtime
            cli(['exec', '--id', 'storage-test', '--', 'true'])
            cli(['logs', 'storage-test'])
            cli(['logs', '--guest', 'storage-test'])
            cli(['stop', 'storage-test'])
            owner.communicate(timeout=10)
            cli(['cache', 'prune'])
            assert not runtime.exists()
            assert not Path(view['lastRun']['cacheDir']).exists()
            assert Path(view['logDir']).is_dir()
            cli(['rm', 'storage-test'])
            assert Path(volume['path']).is_file(), 'session removal deleted persistent data'
            cli(['volume', 'rm', 'data'])
            for key in ('XDG_STATE_HOME', 'XDG_CACHE_HOME', 'XDG_DATA_HOME', 'XDG_RUNTIME_DIR', 'TMPDIR'):
                assert not Path(env[key]).exists(), (key, env[key])
            assert not (user_home / '.cache').exists()
            assert not (user_home / '.local').exists()
            print('PASS single data directory: ' + ('REVM_HOME' if custom else '~/.revm'), flush=True)
        finally:
            if owner is not None and owner.poll() is None:
                owner.kill()
                owner.communicate(timeout=5)
            prune_after_cleanup(binary, env)
