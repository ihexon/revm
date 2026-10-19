#!/usr/bin/env python3
"""Migrate a disposable legacy sparse disk; verify the original is retained."""
import json, os, shutil, subprocess, tempfile, uuid
binary=os.path.abspath('out/revm/bin/revm')
root=tempfile.TemporaryDirectory(prefix='revm-migration-')
sid='audit-'+uuid.uuid4().hex[:10]
legacy=os.path.expanduser('~/.cache/revm/'+sid)
env=dict(os.environ,XDG_STATE_HOME=root.name+'/state',XDG_CACHE_HOME=root.name+'/cache',XDG_DATA_HOME=root.name+'/data')
def cli(args,code=0):
 p=subprocess.run([binary]+args,env=env,stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=30)
 assert p.returncode==code,(args,p.returncode,p.stdout,p.stderr)
 return p
try:
 os.makedirs(legacy+'/raw-disk');os.makedirs(legacy+'/logs')
 subprocess.run(['tar','-xf','pkg/static_resources/raw_disks/ext4.raw.tar','-C',legacy+'/raw-disk'],check=True)
 source=legacy+'/raw-disk/container-storage.ext4';os.rename(legacy+'/raw-disk/ext4.raw',source)
 with open(legacy+'/logs/revm.log','w') as f:f.write('legacy log\n')
 cli(['run','--id',sid,'--','true'],1)
 s=json.loads(cli(['migrate',sid]).stdout)
 disk=root.name+'/data/revm/volumes/'+s['volumeID']+'/disk.ext4'
 assert os.stat(source).st_size==os.stat(disk).st_size
 assert os.stat(disk).st_blocks*512 < 1024**3, 'sparse disk expanded'
 assert open(root.name+'/state/revm/sessions/'+sid+'/legacy-logs/revm.log').read()=='legacy log\n'
 cli(['migrate',sid],1)
 cli(['rm',sid]);cli(['cache','prune'])
 cli(['run','--id',sid,'--cpus','1','--memory','512','--','true'])
 assert json.loads(cli(['inspect',sid]).stdout)['volumeID']==s['volumeID']
 cli(['rm',sid])
 assert os.path.exists(source) and os.path.exists(disk)
 cli(['volume','rm','podman-'+sid]);assert os.path.exists(source)
 print('PASS verified sparse migration, logs, conflict retry and source retention')
finally:
 shutil.rmtree(legacy,ignore_errors=True)
 subprocess.run([binary, "cache", "prune"], env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=10)
 root.cleanup()
