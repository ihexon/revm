#!/usr/bin/env python3
"""Real VM checks: python3 tests/smoke.py out/revm/bin/revm (macOS arm64/Linux)."""
import os
import select
import shutil
import signal
import subprocess
import sys
import tempfile
import time
import uuid

binary = os.path.abspath(sys.argv[1] if len(sys.argv) > 1 else 'out/revm/bin/revm')
processes, sessions = [], []
root = tempfile.TemporaryDirectory(prefix='revm-smoke-')
env = dict(os.environ, XDG_STATE_HOME=root.name+'/state', XDG_CACHE_HOME=root.name+'/cache',
           XDG_DATA_HOME=root.name+'/data', XDG_RUNTIME_DIR=root.name+'/runtime')
os.mkdir(env['XDG_RUNTIME_DIR'], 0o700)

def start(argv, stdin=subprocess.DEVNULL):
    p = subprocess.Popen([binary]+argv, stdin=stdin, stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=env)
    processes.append(p)
    return p

def run(argv, extra=(), **kw):
    sid = 'audit-'+uuid.uuid4().hex[:10]
    sessions.append(sid)
    return start(['run', '--id', sid, '--cpus', '1', '--memory', '512', '--stop-timeout', '1s']+list(extra)+['--']+argv, **kw), sid

def line(p):
    assert select.select([p.stdout], [], [], 20)[0], 'timed out waiting for output'
    got = p.stdout.readline()
    if not got:
        raise AssertionError(p.communicate(timeout=5))
    return got

def finish(p, code=0):
    out, err = p.communicate(timeout=20)
    assert p.returncode == code, (p.returncode, out, err)
    return out, err

try:
    p, _ = run(['sh', '-c', 'cat; printf stderr >&2; exit 42'], stdin=subprocess.PIPE)
    data = bytes(range(256))*8192
    out, err = p.communicate(data, timeout=30)
    assert p.returncode == 42 and out == data and err.startswith(b'stderr'), (p.returncode, len(out), err)
    print('PASS binary streaming and exit 42', flush=True)

    p, _ = run(['cat'], stdin=subprocess.PIPE)
    p.stdin.write(b'before EOF\n'); p.stdin.flush()
    assert line(p) == b'before EOF\n'
    p.stdin.close(); p.stdin = None
    finish(p)
    print('PASS output before stdin EOF', flush=True)

    for argv, code in [(['revm-command-does-not-exist'],127), (['sh','-c','kill -TERM $$'],143), (['sh','-c','kill -34 $$'],162)]:
        p, _ = run(argv); finish(p, code)
    with tempfile.TemporaryDirectory(prefix='revm-exec-format-', dir='/tmp') as fixture:
        script=fixture+'/script'
        with open(script,'w') as f: f.write('echo missing interpreter header\n')
        for mode in [0o644,0o755]:
            os.chmod(script,mode)
            p,_=run(['/work/script'],extra=['--mount',fixture+':/work'])
            finish(p,126)
    print('PASS exec failure, permission/format status 126 and guest signals', flush=True)

    owner, sid = run(['sh','-c','echo ready; sleep 100'])
    assert line(owner) == b'ready\n'
    attached = start(['attach','--id',sid,'--stop-timeout','1s','--','sh','-c',
                      'setsid sh -c "exec sleep 123" & echo ready; wait'])
    assert line(attached) == b'ready\n'
    attached.send_signal(signal.SIGTERM); finish(attached,143)
    assert owner.poll() is None
    probe = start(['attach','--id',sid,'--','sh','-c',"! ps -o args | grep '[s]leep 123'"])
    finish(probe)
    probe = start(['attach','--id',sid,'--tty=always','--','sh','-c','test -t 0 && printf pty-ok'])
    assert finish(probe)[0] == b'pty-ok'
    owner.send_signal(signal.SIGTERM); finish(owner,143)
    print('PASS attach cancellation, setsid cleanup, PTY argv, owner survival', flush=True)

    p, _ = run(['sh','-c','trap "" TERM; echo ready; while :; do sleep 1; done'])
    assert line(p) == b'ready\n'
    at = time.monotonic(); p.send_signal(signal.SIGTERM); finish(p,143)
    assert time.monotonic()-at < 5
    print('PASS bounded cancellation for ignored TERM', flush=True)

    for phase in ['startup','running']:
        p, _ = run(['sh','-c','echo ready; sleep 100'])
        if phase == 'running': assert line(p) == b'ready\n'
        children, deadline = [], time.monotonic()+10
        while not children and time.monotonic() < deadline:
            found = subprocess.run(['pgrep','-P',str(p.pid)], stdout=subprocess.PIPE)
            children = [int(x) for x in found.stdout.split()]
            if not children: time.sleep(.005)
        assert children, phase
        p.kill(); p.wait(timeout=5)
        deadline = time.monotonic()+5
        while time.monotonic() < deadline:
            alive = []
            for child in children:
                try: os.kill(child,0); alive.append(child)
                except ProcessLookupError: pass
            if not alive: break
            time.sleep(.02)
        assert not alive, (phase, alive)
    print('PASS no orphan VMM after owner SIGKILL during startup/running', flush=True)
finally:
    for p in processes:
        if p.poll() is None: p.kill()
        try: p.communicate(timeout=5)
        except subprocess.TimeoutExpired: pass
    # Before the XDG layout upgrade, these uniquely named sessions live here.
    for sid in sessions:
        shutil.rmtree(os.path.expanduser('~/.cache/revm/'+sid), ignore_errors=True)
    subprocess.run([binary, "cache", "prune"], env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=10)
    root.cleanup()
