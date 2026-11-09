#!/usr/bin/env python3
"""Real discard regression, using only disposable VM storage and raw images.

Requires the Podman remote CLI and e2fsprogs. No network/image pull is needed.
"""
from support import prune_after_cleanup
import json
import os
from pathlib import Path
import shutil
import subprocess
import tarfile
import tempfile
import time

binary = str(Path('out/revm/bin/revm').resolve())
podman = os.environ.get('PODMAN', 'podman')
mkfs = shutil.which('mkfs.ext4')
if mkfs is None:
    prefix = subprocess.check_output(['brew', '--prefix', 'e2fsprogs'], text=True).strip()
    mkfs = str(Path(prefix) / 'sbin/mkfs.ext4')
size, block = 128 * 1024 * 1024, 4096

with tempfile.TemporaryDirectory(prefix='rv-discard-', dir='/var/tmp') as temp:
    root = Path(temp)
    env = dict(os.environ, REVM_HOME=str(root / 'home'))
    owner = None

    def checked(args, **kwargs):
        result = subprocess.run(args, env=env, capture_output=True, timeout=60, **kwargs)
        assert result.returncode == 0, (args, result.returncode, result.stdout, result.stderr)
        return result.stdout

    def image(name, length=size):
        path = root / (name + '.ext4')
        with path.open('wb') as stream:
            stream.truncate(length)
        checked([mkfs, '-q', '-F', '-E', 'nodiscard,lazy_itable_init=0', str(path)])
        return path

    storage = image('containers', 1024 * 1024 * 1024)
    disks = {name: image(name) for name in ('filesystem', 'discard', 'write-zeroes')}
    for name in ('discard', 'write-zeroes'):
        with disks[name].open('r+b') as stream:
            stream.seek(size - 2 * block)
            stream.write(b'G' * block + b'X' * block)

    def start():
        global owner
        args = [binary, 'podman', '--id', 'discard-test', '--cpus', '1', '--memory', '512',
                '--container-disk', str(storage)]
        for name, path in disks.items():
            args += ['--raw-disk', f'{path},mnt=/mnt/{name},sync=full']
        owner = subprocess.Popen(args, env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        deadline = time.monotonic() + 30
        while True:
            assert owner.poll() is None, owner.communicate()
            probe = subprocess.run([binary, 'inspect', 'discard-test'], env=env,
                                   capture_output=True, timeout=10)
            if probe.returncode == 0:
                view = json.loads(probe.stdout)
                endpoint = view.get('endpoints', {}).get('podman')
                if endpoint:
                    env['CONTAINER_HOST'] = endpoint
                    probe = subprocess.run([podman, 'info'], env=env, capture_output=True, timeout=10)
                    if probe.returncode == 0:
                        return view
            assert time.monotonic() < deadline, probe.stderr
            time.sleep(.1)

    def stop():
        if owner is not None and owner.poll() is None:
            checked([binary, 'stop', 'discard-test'])
            owner.communicate(timeout=15)
        prune_after_cleanup(binary, env)

    def guest(script):
        # Normal revm exec deliberately drops CAP_SYS_ADMIN. Use the supported
        # Podman admin API in this disposable VM, keeping production isolation.
        return checked([podman, 'run', '--rm', '--privileged', '--pid=host',
                        '--network=none', '--http-proxy=false', 'discard-tools',
                        'nsenter', '--target', '1', '--mount', '--root', '--wd',
                        '--', 'sh', '-ceu', script])

    try:
        view = start()
        # Reuse the running VM's Alpine utilities instead of pulling an image.
        archive = root / 'tools.tar'
        with tarfile.open(archive, 'w') as bundle:
            bundle.add(Path(view['lastRun']['cacheDir']) / 'rootfs', arcname='.')
        checked([podman, 'import', str(archive), 'discard-tools'])
        guest('printf retained > /mnt/filesystem/keep; sync; fstrim -v /mnt/filesystem; sync')
        assert disks['filesystem'].stat().st_size == size, 'fstrim truncated raw disk'
        stop()
        checked([binary, 'rm', 'discard-test'])
        start()
        assert guest('cat /mnt/filesystem/keep') == b'retained'
        print('PASS fstrim preserves raw disk length and filesystem data across reboot', flush=True)

        # Force operations ending exactly at EOF, independent of ext4's free
        # space layout. Unmount before discarding these disposable block devices.
        for name, option in [('discard', ''), ('write-zeroes', '--zeroout')]:
            guest(f'''
                device=$(findmnt -nr -o SOURCE /mnt/{name})
                umount /mnt/{name}
                blkdiscard {option} --offset {size - block} --length {block} "$device"
                sync
            ''')
            assert disks[name].stat().st_size == size, (name, 'raw disk truncated')
            with disks[name].open('rb') as stream:
                stream.seek(size - 2 * block)
                assert stream.read(block) == b'G' * block, (name, 'adjacent data modified')
                if name == 'write-zeroes':
                    assert stream.read(block) == bytes(block), 'tail was not zeroed'
            print(f'PASS tail {name} preserves raw disk length and adjacent data', flush=True)
    finally:
        stop()
