#!/usr/bin/env python3
"""Real terminal and early pipe-close checks (requires a VM-capable host)."""
import fcntl, os, pty, select, signal, subprocess, tempfile, termios, time, uuid
binary=os.path.abspath('out/revm/bin/revm')
root=tempfile.TemporaryDirectory(prefix='revm-terminal-')
env=dict(os.environ,XDG_STATE_HOME=root.name+'/state',XDG_CACHE_HOME=root.name+'/cache',XDG_DATA_HOME=root.name+'/data')
processes=[];child=None

def argv(command):
 return [binary,'run','--id','audit-'+uuid.uuid4().hex[:10],'--cpus','1','--memory','512','--']+command

def terminal_state(fd):
 state=termios.tcgetattr(fd)
 # The kernel may set PENDIN when canonical input is restored. It is pending
 # input bookkeeping, not a changed terminal setting.
 state[3]&=~getattr(termios,'PENDIN',0)
 return state
try:
 p=subprocess.Popen(argv(['yes']),env=env,stdout=subprocess.PIPE,stderr=subprocess.PIPE,stdin=subprocess.DEVNULL);processes.append(p)
 assert p.stdout.readline();p.stdout.close();p.stdout=None
 _,err=p.communicate(timeout=8)
 assert p.returncode == 141, (p.returncode,err)

 for output in ['stdout','stderr']:
  read,write=os.pipe()
  streams={output:write, ('stderr' if output=='stdout' else 'stdout'):subprocess.DEVNULL}
  command=['yes'] if output=='stdout' else ['sh','-c','exec yes >&2']
  p=subprocess.Popen(argv(command),env=env,stdin=subprocess.DEVNULL,**streams);processes.append(p)
  assert select.select([read],[],[],20)[0]
  time.sleep(.3)
  assert not fcntl.fcntl(write,fcntl.F_GETFL)&os.O_NONBLOCK
  at=time.monotonic();p.send_signal(signal.SIGTERM)
  # Neither output pipe is drained: even diagnostics must not block cancellation.
  p.wait(timeout=2);assert time.monotonic()-at<2
  assert p.returncode==143
  assert not fcntl.fcntl(write,fcntl.F_GETFL)&os.O_NONBLOCK
  os.close(write);os.close(read)
 print('PASS broken pipe, blocked stdout/stderr cancellation and unchanged descriptor flags',flush=True)

 # Keep the controlling session alive like an interactive shell. On macOS,
 # killing the session leader revokes every slave descriptor (there is then
 # no terminal for the helper to restore).
 if os.getsid(0)!=os.getpid():os.setsid()
 signal.signal(signal.SIGHUP,signal.SIG_IGN)
 signal.signal(signal.SIGTTOU,signal.SIG_IGN)
 master,slave=pty.openpty()
 fcntl.ioctl(slave,termios.TIOCSCTTY,0)
 initial=terminal_state(master)
 for death in ['Ctrl-C','SIGKILL']:
  child=os.fork()
  if child==0:
   os.setpgid(0,0)
   os.dup2(slave,0);os.dup2(slave,1);os.dup2(slave,2)
   os.close(master);os.close(slave)
   os.execve(binary,argv(['sh','-c','trap "" INT TERM; echo READY; sleep 100']),env)
  os.setpgid(child,child)
  os.tcsetpgrp(slave,child)
  end=time.monotonic()+20;data=b''
  while b'READY' not in data:
   assert time.monotonic()<end
   if select.select([master],[],[],.1)[0]:data+=os.read(master,4096)
  assert not termios.tcgetattr(master)[3]&termios.ICANON
  assert termios.tcgetattr(master)[3]&termios.ISIG
  os.kill(child,signal.SIGTSTP)
  end=time.monotonic()+3
  while True:
   pid,status=os.waitpid(child,os.WNOHANG|os.WUNTRACED)
   if pid:assert os.WIFSTOPPED(status);break
   assert time.monotonic()<end;time.sleep(.02)
  assert terminal_state(master)==initial,'terminal not restored on suspend'
  os.kill(child,signal.SIGCONT)
  end=time.monotonic()+3
  while termios.tcgetattr(master)[3]&termios.ICANON:
   assert time.monotonic()<end;time.sleep(.02)
  if death=='Ctrl-C':os.write(master,b'\x03')
  else:os.kill(child,signal.SIGKILL)
  end=time.monotonic()+2
  while True:
   pid,status=os.waitpid(child,os.WNOHANG)
   if pid:break
   assert time.monotonic()<end;time.sleep(.02)
  child=None
  assert os.waitstatus_to_exitcode(status)==(130 if death=='Ctrl-C' else -9),status
  end=time.monotonic()+5
  while terminal_state(master)!=initial:
   assert time.monotonic()<end,'cleanup helper did not restore terminal'
   time.sleep(.02)
  assert terminal_state(master)==initial,'terminal not restored on exit'
  os.tcsetpgrp(slave,os.getpgrp())
  print('PASS PTY signals, suspend/resume and terminal restoration after '+death,flush=True)
 os.close(slave);os.close(master)
finally:
 for p in processes:
  if p.poll() is None:p.kill();p.communicate(timeout=5)
 if child:
  try:os.kill(child,signal.SIGKILL);os.waitpid(child,0)
  except ProcessLookupError:pass
 subprocess.run([binary, "cache", "prune"], env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=10)
 root.cleanup()
