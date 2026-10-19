import os,subprocess,tempfile,time,json,signal,uuid
b=os.path.abspath('out/revm/bin/revm');root=tempfile.TemporaryDirectory(prefix='revm-volume-');sid='audit-'+uuid.uuid4().hex[:10];owner=None
env=dict(os.environ,XDG_STATE_HOME=root.name+'/state',XDG_CACHE_HOME=root.name+'/cache',XDG_DATA_HOME=root.name+'/data')
def cli(args,ok=True):
 r=subprocess.run([b]+args,env=env,stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=40)
 if ok: assert r.returncode==0,(args,r.returncode,r.stdout,r.stderr)
 return r
def start():
 global owner
 meta=root.name+'/state/revm/sessions/'+sid+'/session.json'
 previous=json.load(open(meta))['instanceID'] if os.path.exists(meta) else None
 owner=subprocess.Popen([b,'podman','--volume','data','--id',sid,'--cpus','1','--memory','512','--stop-timeout','2s'],env=env,stdout=subprocess.PIPE,stderr=subprocess.PIPE)
 meta=root.name+'/state/revm/sessions/'+sid+'/session.json'
 end=time.monotonic()+20
 while not os.path.exists(meta) or json.load(open(meta))['instanceID']==previous:
  if owner.poll() is not None: raise AssertionError(owner.communicate())
  assert time.monotonic()<end
  time.sleep(.05)
 # Wait until the host management service starts, then exec waits for guest readiness.
 while True:
  probe=cli(['exec','--id',sid,'--','true'],False)
  if probe.returncode==0: break
  assert owner.poll() is None,(probe.stderr,owner.communicate())
  assert time.monotonic()<end,probe.stderr
  time.sleep(.1)
 return meta
try:
 cli(['volume','create','data'])
 assert cli(['volume','create','data'],False).returncode==1
 meta=start();vol=json.load(open(meta))['volumeID']
 cli(['exec','--id',sid,'--','podman','ps'])
 cli(['exec','--id',sid,'--','sh','-c','echo keep-me > /var/lib/containers/revm-smoke-data'])
 assert cli(['volume','rm','data'],False).returncode==1
 owner.send_signal(signal.SIGTERM);owner.communicate(timeout=10)
 # Remove only the test session's records, retaining the independently owned volume.
 cli(['rm',sid])
 sid='audit-'+uuid.uuid4().hex[:10]
 meta=start();assert json.load(open(meta))['volumeID']==vol
 assert cli(['exec','--id',sid,'--','cat','/var/lib/containers/revm-smoke-data']).stdout==b'keep-me\n'
 owner.send_signal(signal.SIGTERM);owner.communicate(timeout=10)
 cli(['cache','prune']);cli(['volume','rm','data'])
 assert cli(['podman','--volume','data','--id',sid,'--cpus','1','--memory','512'],False).returncode==1
 cli(['volume','create','data'])
 meta=start();assert json.load(open(meta))['volumeID']!=vol
 assert cli(['exec','--id',sid,'--','sh','-c','test ! -e /var/lib/containers/revm-smoke-data']).returncode==0
 owner.send_signal(signal.SIGTERM);owner.communicate(timeout=10)
 cli(['volume','rm','data'])
 print('PASS real Podman volume reuse, active deletion refusal, preserved contents and missing references')
finally:
 if owner and owner.poll() is None: owner.kill();owner.communicate(timeout=5)
 subprocess.run([b, "cache", "prune"], env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=10)
 root.cleanup()
