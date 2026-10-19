import os,subprocess,tempfile,time,json,signal,uuid,shutil
b=os.path.abspath('out/revm/bin/revm');root=tempfile.TemporaryDirectory(prefix='revm-volume-');sid='audit-'+uuid.uuid4().hex[:10];owner=None
env=dict(os.environ,XDG_STATE_HOME=root.name+'/state',XDG_CACHE_HOME=root.name+'/cache',XDG_DATA_HOME=root.name+'/data')
def cli(args,ok=True):
 r=subprocess.run([b]+args,env=env,stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=40)
 if ok: assert r.returncode==0,(args,r.returncode,r.stdout,r.stderr)
 return r
def start():
 global owner
 owner=subprocess.Popen([b,'podman','--id',sid,'--cpus','1','--memory','512','--stop-timeout','2s'],env=env,stdout=subprocess.PIPE,stderr=subprocess.PIPE)
 meta=root.name+'/state/revm/sessions/'+sid+'/session.json'
 end=time.monotonic()+20
 while not os.path.exists(meta):
  if owner.poll() is not None: raise AssertionError(owner.communicate())
  assert time.monotonic()<end
  time.sleep(.05)
 # Wait until the host management service starts, then attach waits for guest readiness.
 while True:
  probe=cli(['attach','--id',sid,'--','true'],False)
  if probe.returncode==0: break
  assert owner.poll() is None,(probe.stderr,owner.communicate())
  assert time.monotonic()<end,probe.stderr
  time.sleep(.1)
 return meta
try:
 meta=start();vol=json.load(open(meta))['volumeID']
 cli(['attach','--id',sid,'--','sh','-c','echo keep-me > /var/lib/containers/revm-smoke-data'])
 assert cli(['volume','rm','podman-'+sid],False).returncode==1
 owner.send_signal(signal.SIGTERM);owner.communicate(timeout=10)
 # Remove only the test session's records, retaining the independently owned volume.
 shutil.rmtree(os.path.dirname(meta))
 start();assert json.load(open(meta))['volumeID']==vol
 assert cli(['attach','--id',sid,'--','cat','/var/lib/containers/revm-smoke-data']).stdout==b'keep-me\n'
 owner.send_signal(signal.SIGTERM);owner.communicate(timeout=10)
 cli(['cache','prune']);cli(['volume','rm','podman-'+sid])
 assert cli(['podman','--id',sid,'--cpus','1','--memory','512'],False).returncode==1
 print('PASS real Podman volume reuse, active deletion refusal, preserved contents and missing references')
finally:
 if owner and owner.poll() is None: owner.kill();owner.communicate(timeout=5)
 root.cleanup()
