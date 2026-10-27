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
    return start(['run', '--id', sid, '--cpus', '1', '--memory', '512']+list(extra)+['--']+argv, **kw), sid

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
    assert p.returncode == 42 and out == data and err == b'stderr', (p.returncode, len(out), err)
    print('PASS binary streaming and exit 42', flush=True)

    p, _ = run(['sh', '-c', 'printf "%s" "$0"; exit 42', '--generate-shell-completion'])
    assert finish(p, 42)[0] == b'--generate-shell-completion'
    print('PASS completion marker is passed unchanged to the guest', flush=True)

    p, _ = run(['cat'], stdin=subprocess.PIPE)
    p.stdin.write(b'before EOF\n'); p.stdin.flush()
    assert line(p) == b'before EOF\n'
    p.stdin.close(); p.stdin = None
    finish(p)
    print('PASS output before stdin EOF', flush=True)

    p, _ = run(['head', '-c', '1048576', '/dev/zero'])
    assert select.select([p.stdout], [], [], 20)[0]
    time.sleep(4)
    assert len(finish(p)[0]) == 1048576
    print('PASS slow output consumer without truncation', flush=True)

    fd = os.open(root.name, os.O_RDONLY)
    try:
        p, _ = run(['cat'], stdin=fd)
        out, err = p.communicate(timeout=20)
        assert p.returncode != 0 and b'read command input' in err, (p.returncode, err)
    finally:
        os.close(fd)
    print('PASS stdin read failure is reported', flush=True)

    p, _ = run(['sh','-c','printf "%s:%s" "$CHECK" "$PWD"'], extra=['--envs','CHECK=propagated','--workdir','/tmp'])
    assert finish(p)[0] == b'propagated:/tmp'
    print('PASS execution environment and working directory', flush=True)

    for argv, code in [(['revm-command-does-not-exist'],127), (['sh','-c','kill -TERM $$'],143), (['sh','-c','kill -34 $$'],162)]:
        p, _ = run(argv); finish(p, code)
    with tempfile.TemporaryDirectory(prefix='revm-exec-format-', dir='/tmp') as fixture:
        script=fixture+'/script'
        with open(script,'w') as f: f.write('echo missing interpreter header\n')
        for mode in [0o644,0o755]:
            os.chmod(script,mode)
            p,_=run(['/work/script'],extra=['--mount',fixture+':/work'])
            finish(p,126)
        p,_=run(['sh','-c','test -f /work/script && ! touch /work/new-file'],extra=['--mount',fixture+':/work,ro'])
        finish(p)
        assert not os.path.exists(fixture+'/new-file')
    print('PASS read-only mounts, exec failure, permission/format status 126 and guest signals', flush=True)

    owner, sid = run(['sh','-c','echo ready; sleep 100'])
    assert line(owner) == b'ready\n'
    attached = start(['exec','--id',sid,'--','sh','-c',
                      'setsid sh -c "exec sleep 123" & echo ready; wait'])
    assert line(attached) == b'ready\n'
    attached.send_signal(signal.SIGTERM); finish(attached,143)
    assert owner.poll() is None
    probe = start(['exec','--id',sid,'--','sh','-c',"! ps -o args | grep '[s]leep 123'"])
    finish(probe)
    probe = start(['exec','--id',sid,'--tty=always','--','sh','-c','test -t 0 && printf pty-ok'])
    assert finish(probe)[0] == b'pty-ok'
    owner.send_signal(signal.SIGTERM); finish(owner,143)
    print('PASS exec cancellation, setsid cleanup, PTY argv, owner survival', flush=True)

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
        assert len(children) == 1, children
        command = subprocess.check_output(['ps','-p',str(children[0]),'-o','command=']).decode()
        assert command.rstrip().endswith(' _cleanup'), command
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
    print('PASS VM runs in the main process; cleanup child exits after SIGKILL', flush=True)
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
