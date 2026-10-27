#!/usr/bin/env python3
"""Session CLI integration checks against a real VM."""
import json, os, select, signal, socket, subprocess, tempfile, time, uuid
binary=os.path.abspath('out/revm/bin/revm')
root=tempfile.TemporaryDirectory(prefix='revm-sessions-')
env=dict(os.environ,XDG_STATE_HOME=root.name+'/state',XDG_CACHE_HOME=root.name+'/cache',XDG_DATA_HOME=root.name+'/data')
processes=[]
def cli(args,code=0):
 p=subprocess.run([binary]+args,env=env,stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=25)
 assert p.returncode==code,(args,p.returncode,p.stdout,p.stderr)
 return p
try:
 # Query commands must exit even when their output pipe is full.
 iid=str(uuid.uuid4())
 session=root.name+'/state/revm/sessions/blocked-logs'
 run=session+'/runs/'+iid
 os.makedirs(run+'/logs')
 with open(session+'/session.json','w') as f:
  json.dump(dict(version=1,id='blocked-logs',instanceID=iid,config={}),f)
 with open(run+'/result.json','w') as f:
  json.dump(dict(version=1,sessionID='blocked-logs',instanceID=iid,status='stopped',exitCode=0),f)
 with open(run+'/logs/revm.log','wb') as f:f.write(b'log line\n'*1000000)
 for sig in [signal.SIGINT,signal.SIGTERM]:
  reader=subprocess.Popen([binary,'logs','blocked-logs'],env=env,stdout=subprocess.PIPE,stderr=subprocess.PIPE)
  processes.append(reader)
  assert select.select([reader.stdout],[],[],5)[0]
  time.sleep(.1)
  reader.send_signal(sig)
  assert reader.wait(timeout=3)==128+sig
  reader.communicate()
 cli(['rm','blocked-logs'])
 print('PASS blocked log output exits on the first termination signal',flush=True)

 os.makedirs(root.name+'/state/revm/sessions/broken/runs/incomplete')
 views=json.loads(cli(['ls','--json']).stdout)
 assert views[0]['id']=='broken' and views[0]['status']=='broken' and views[0]['error']
 cli(['rm','broken'])
 print('PASS incomplete session listing and removal',flush=True)
 sid='audit-'+uuid.uuid4().hex[:10]
 with socket.socket() as listener:
  listener.bind(('127.0.0.1',0))
  port=str(listener.getsockname()[1])
 owner=subprocess.Popen([binary,'run','--id',sid,'--cpus','1','--memory','512','--port-export',port+':8080','--','sh','-c','nc -lk -p 8080 -e cat & echo ready; wait'],env=env,stdout=subprocess.PIPE,stderr=subprocess.PIPE);processes.append(owner)
 assert owner.stdout.readline()==b'ready\n'
 cli(['wait','--ready','--timeout','5s',sid])
 active=json.loads(cli(['inspect',sid]).stdout)
 assert active['active'] is True and active['endpoints']['management'].startswith('unix:///')
 assert os.path.isdir(active['logDir'])
 assert sid in cli(['ls','--json']).stdout.decode()
 cli(['rm',sid],1)
 cli(['logs',sid]);cli(['logs','--guest',sid])
 assert cli(['logs','--tail','0',sid]).stdout==b''
 lines=cli(['logs','--tail','2',sid]).stdout.splitlines()
 assert len(lines)<=2
 ports=json.loads(cli(['ports','--json',sid]).stdout)
 assert any(p['local']=='127.0.0.1:'+port and p['remote'].endswith(':8080') for p in ports),ports
 with socket.create_connection(('127.0.0.1',int(port)),timeout=5) as connection:
  connection.sendall(b'port-forward-ok\n')
  with connection.makefile('rb') as response:
   assert response.readline()==b'port-forward-ok\n'
 cli(['stop','--timeout','6s',sid]);owner.communicate(timeout=5)
 view=json.loads(cli(['inspect',sid]).stdout)
 assert not view['active'] and view['lastRun']['finishedAt'] and 'endpoints' not in view
 cli(['stop',sid]);cli(['rm',sid]);cli(['inspect',sid],1)
 print('PASS list/inspect/readiness/logs/ports/stop/remove',flush=True)

 sid='audit-'+uuid.uuid4().hex[:10]
 cli(['run','--id',sid,'--cpus','1','--memory','512','--','sh','-c','exit 42'],42)
 cli(['wait',sid],42)
 assert json.loads(cli(['inspect',sid]).stdout)['lastRun']['exitCode']==42
 cli(['rm',sid])
 print('PASS persisted exit status and wait',flush=True)

 for code in [0,42]:
  cli(['run','--cpus','1','--memory','512','--','sh','-c','exit '+str(code)],code)
  views=json.loads(cli(['ls','--json']).stdout)
  if code==0:
   end=time.monotonic()+5
   while views:
    assert time.monotonic()<end,views
    time.sleep(.05)
    views=json.loads(cli(['ls','--json']).stdout)
   assert views==[]
  else:
   assert len(views)==1 and views[0]['lastRun']['exitCode']==code
   sid=views[0]['id']
   cli(['wait',sid],code)
   cli(['logs',sid]);cli(['logs','--guest',sid]);cli(['cache','prune'])
   assert len(json.loads(cli(['ls','--json']).stdout))==1
   cli(['rm',sid])
 print('PASS successful temporary runs removed; failed runs retained',flush=True)

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

 owner=subprocess.Popen([binary,'run','--cpus','1','--memory','512','--','sh','-c','echo ready; sleep 100'],env=env,stdout=subprocess.PIPE,stderr=subprocess.PIPE);processes.append(owner)
 assert owner.stdout.readline()==b'ready\n'
 view=json.loads(cli(['ls','--json']).stdout)[0]
 owner.kill();owner.communicate(timeout=5)
 end=time.monotonic()+5
 while json.loads(cli(['inspect',view['id']]).stdout)['active']:
  assert time.monotonic()<end
  time.sleep(.05)
 cli(['cache','prune'])
 assert json.loads(cli(['inspect',view['id']]).stdout)['status']=='unknown'
 cli(['logs','--guest',view['id']]);cli(['rm',view['id']])
 print('PASS pruning runtime resources preserves crash diagnostics',flush=True)
finally:
 for p in processes:
  if p.poll() is None:p.kill();p.communicate(timeout=5)
 subprocess.run([binary, "cache", "prune"], env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=10)
 root.cleanup()
