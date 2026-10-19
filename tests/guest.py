#!/usr/bin/env python3
"""Run Linux-only guest service and supervisor tests inside the built VM."""
import os
from pathlib import Path
import platform
import subprocess
import tempfile

repo = Path(__file__).resolve().parents[1]
binary = repo / 'out/revm/bin/revm'
arch = {'aarch64': 'arm64', 'arm64': 'arm64', 'x86_64': 'amd64'}[platform.machine()]
with tempfile.TemporaryDirectory(prefix='revm-guest-tests-') as root:
    root = Path(root)
    fixtures = root / 'tests'
    fixtures.mkdir()
    build_env = dict(os.environ, CGO_ENABLED='0', GOOS='linux', GOARCH=arch)
    for package in ('service', 'supervisor', 'vsock'):
        subprocess.run(['go', 'test', '-c', '-o', str(fixtures / (package + '.test')),
                        './pkg/' + package], cwd=repo / 'cmd/guest-agent',
                       env=build_env, check=True, timeout=120)
    env = dict(os.environ, XDG_STATE_HOME=str(root / 'state'),
               XDG_CACHE_HOME=str(root / 'cache'), XDG_DATA_HOME=str(root / 'data'))
    try:
        subprocess.run([str(binary), 'run', '--cpus', '1', '--memory', '512',
                        '--mount', str(fixtures) + ':/tests,ro', '--', 'sh', '-c',
                        'set -e; for test in /tests/*.test; do "$test" -test.v; done'],
                       env=env, check=True, timeout=45)
    finally:
        subprocess.run([str(binary), 'cache', 'prune'], env=env, timeout=10,
                       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
print('PASS Linux guest service ownership, supervisor and host-only transport')
