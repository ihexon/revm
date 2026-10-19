#!/usr/bin/env python3
"""Session CLI integration checks against a real VM."""
import json, os, signal, subprocess, tempfile, time, uuid
binary=os.path.abspath('out/revm/bin/revm')
root=tempfile.TemporaryDirectory(prefix='revm-sessions-')
env=dict(os.environ,XDG_STATE_HOME=root.name+'/state',XDG_CACHE_HOME=root.name+'/cache',XDG_DATA_HOME=root.name+'/data')
processes=[]
def cli(args,code=0):
 p=subprocess.run([binary]+args,env=env,stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=25)
 assert p.returncode==code,(args,p.returncode,p.stdout,p.stderr)
 return p
try:
 sid='audit-'+uuid.uuid4().hex[:10]
 owner=subprocess.Popen([binary,'run','--id',sid,'--cpus','1','--memory','512','--stop-timeout','1s','--','sh','-c','echo ready; sleep 100'],env=env,stdout=subprocess.PIPE,stderr=subprocess.PIPE);processes.append(owner)
 assert owner.stdout.readline()==b'ready\n'
 cli(['wait','--ready','--timeout','5s',sid])
 assert json.loads(cli(['inspect',sid]).stdout)['active'] is True
 assert sid in cli(['ls','--json']).stdout.decode()
 cli(['rm',sid],1)
 cli(['logs',sid]);cli(['logs','--guest',sid])
 cli(['ctl','--id',sid,'--list-port','--json'])
 cli(['stop','--timeout','6s',sid]);owner.communicate(timeout=5)
 view=json.loads(cli(['inspect',sid]).stdout)
 assert not view['active'] and view['lastRun']['finishedAt']
 cli(['stop',sid]);cli(['rm',sid]);cli(['inspect',sid],1)
 print('PASS list/inspect/readiness/logs/ports/stop/remove',flush=True)

 sid='audit-'+uuid.uuid4().hex[:10]
 cli(['run','--id',sid,'--cpus','1','--memory','512','--','sh','-c','exit 42'],42)
 cli(['wait',sid],42)
 assert json.loads(cli(['inspect',sid]).stdout)['lastRun']['exitCode']==42
 cli(['rm',sid])
 print('PASS persisted exit status and wait',flush=True)

 for code in [0,42]:
  sid='audit-'+uuid.uuid4().hex[:10]
  cli(['run','--rm','--id',sid,'--cpus','1','--memory','512','--','sh','-c','exit '+str(code)],code)
  cli(['inspect',sid],1)
 print('PASS --rm for successful and failed commands',flush=True)

 sid='audit-'+uuid.uuid4().hex[:10]
 owner=subprocess.Popen([binary,'run','--id',sid,'--cpus','1','--memory','512','--','sh','-c','echo ready; sleep 100'],env=env,stdout=subprocess.PIPE,stderr=subprocess.PIPE);processes.append(owner)
 assert owner.stdout.readline()==b'ready\n'
 owner.kill();owner.communicate(timeout=5)
 end=time.monotonic()+5
 while True:
  view=json.loads(cli(['inspect',sid]).stdout)
  if not view['active']:break
  assert time.monotonic()<end
  time.sleep(.05)
 assert view['status']=='unknown';cli(['wait',sid],1);cli(['cache','prune']);cli(['rm',sid])
 print('PASS crash is unknown and stale resources are reclaimable',flush=True)
finally:
 for p in processes:
  if p.poll() is None:p.kill();p.communicate(timeout=5)
 subprocess.run([binary, "cache", "prune"], env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=10)
 root.cleanup()
