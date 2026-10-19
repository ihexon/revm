#!/usr/bin/env python3
"""Real terminal and early pipe-close checks (requires a VM-capable host)."""
import os, pty, select, signal, subprocess, tempfile, termios, time, uuid
binary=os.path.abspath('out/revm/bin/revm')
root=tempfile.TemporaryDirectory(prefix='revm-terminal-')
env=dict(os.environ,XDG_STATE_HOME=root.name+'/state',XDG_CACHE_HOME=root.name+'/cache',XDG_DATA_HOME=root.name+'/data')
processes=[];child=None

def argv(command):
 return [binary,'run','--id','audit-'+uuid.uuid4().hex[:10],'--cpus','1','--memory','512','--stop-timeout','1s','--']+command
try:
 p=subprocess.Popen(argv(['yes']),env=env,stdout=subprocess.PIPE,stderr=subprocess.PIPE,stdin=subprocess.DEVNULL);processes.append(p)
 assert p.stdout.readline();p.stdout.close();p.stdout=None
 _,err=p.communicate(timeout=8)
 assert p.returncode == 141, (p.returncode,err)

 p=subprocess.Popen(argv(['yes']),env=env,stdout=subprocess.PIPE,stderr=subprocess.PIPE,stdin=subprocess.DEVNULL);processes.append(p)
 assert select.select([p.stdout],[],[],20)[0]
 time.sleep(.5);at=time.monotonic();p.send_signal(signal.SIGTERM)
 # Do not drain stdout while waiting: cancellation must not depend on output flow.
 p.wait(timeout=6);assert time.monotonic()-at<6
 p.stdout.close();p.stdout=None;p.communicate(timeout=2)
 assert p.returncode==143
 print('PASS early pipe close and cancellation with blocked stdout',flush=True)

 child,master=pty.fork()
 if child==0:os.execve(binary,argv(['sh','-c','echo READY; sleep 100']),env)
 initial=termios.tcgetattr(master)
 end=time.monotonic()+20;data=b''
 while b'READY' not in data:
  assert time.monotonic()<end
  if select.select([master],[],[],.1)[0]:data+=os.read(master,4096)
 assert not termios.tcgetattr(master)[3]&termios.ICANON
 os.kill(child,signal.SIGTSTP)
 end=time.monotonic()+3
 while True:
  pid,status=os.waitpid(child,os.WNOHANG|os.WUNTRACED)
  if pid:assert os.WIFSTOPPED(status);break
  assert time.monotonic()<end;time.sleep(.02)
 assert termios.tcgetattr(master)==initial,'terminal not restored on suspend'
 os.kill(child,signal.SIGCONT)
 end=time.monotonic()+3
 while termios.tcgetattr(master)[3]&termios.ICANON:
  assert time.monotonic()<end;time.sleep(.02)
 os.kill(child,signal.SIGTERM)
 end=time.monotonic()+6
 while True:
  pid,status=os.waitpid(child,os.WNOHANG)
  if pid:break
  assert time.monotonic()<end;time.sleep(.02)
 child=None
 assert os.waitstatus_to_exitcode(status)==143,status
 assert termios.tcgetattr(master)==initial,'terminal not restored on exit'
 os.close(master)
 print('PASS PTY auto, suspend/resume and terminal restoration',flush=True)
finally:
 for p in processes:
  if p.poll() is None:p.kill();p.communicate(timeout=5)
 if child:
  try:os.kill(child,signal.SIGKILL);os.waitpid(child,0)
  except ProcessLookupError:pass
 root.cleanup()
