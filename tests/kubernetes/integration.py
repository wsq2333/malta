#!/usr/bin/env python3
"""Destructive fault tests ONLY in a disposable malta-test* namespace/cluster."""
import argparse,json,os,pathlib,subprocess,time,urllib.request,urllib.error,uuid
p=argparse.ArgumentParser(description=__doc__)
p.add_argument('--kubeconfig',required=True);p.add_argument('--namespace',required=True)
p.add_argument('--malta',required=True);p.add_argument('--token-file',required=True)
p.add_argument('--output',required=True);p.add_argument('--cluster',required=True);p.add_argument('--node',required=True)
a=p.parse_args()
if not a.namespace.startswith('malta-test'): p.error('requires a disposable malta-test* namespace')
out=pathlib.Path(a.output);out.mkdir(parents=True,exist_ok=False)
k=['kubectl','--kubeconfig',a.kubeconfig,'-n',a.namespace]
def kubectl(*args): return subprocess.check_output(k+list(args),stderr=subprocess.STDOUT).decode()
token=pathlib.Path(a.token_file).read_text().strip();pf=None
log=(out/'port-forward.log').open('w')
def api(method,path,data=None):
 global pf
 for _ in range(90):
  if pf is None or pf.poll() is not None:
   pf=subprocess.Popen(k+['port-forward','service/malta','19483:8080'],stdout=log,stderr=log);time.sleep(.3)
  req=urllib.request.Request('http://127.0.0.1:19483/api/v1/'+path,data=None if data is None else json.dumps(data).encode(),method=method,headers={'Authorization':'Bearer '+token,'Content-Type':'application/json'})
  try:
   with urllib.request.urlopen(req,timeout=10) as r:return json.load(r)['data']
  except urllib.error.HTTPError as e: raise RuntimeError(e.read().decode())
  except (urllib.error.URLError,TimeoutError,ConnectionError):time.sleep(1)
 raise RuntimeError('server unavailable')
def save(name,data): (out/(name+'.json')).write_text(json.dumps(data,indent=2)+'\n')
def wait(r,states=('succeeded','failed','cancelled'),seconds=150):
 end=time.monotonic()+seconds
 while time.monotonic()<end:
  v=api('GET','runs/'+r['id'])
  if v['status'] in states and v['attempt']['result_state']!='collecting': return v
  time.sleep(.4)
 raise RuntimeError('run wait deadline: '+r['id'])
def running(r):
 end=time.monotonic()+60
 while time.monotonic()<end:
  v=api('GET','runs/'+r['id']);job=v['attempt'].get('job_name')
  if job:
   pods=json.loads(kubectl('get','pods','-l','job-name='+job,'-o','json'))['items']
   if any(pod['status']['phase']=='Running' for pod in pods):return v
  if v['status'] in ('failed','succeeded'):raise RuntimeError('run finished too early: '+json.dumps(v))
  time.sleep(.3)
 raise RuntimeError('Pod did not start')
def start(plan,label):
 request={'plan':plan,'executor':'kubernetes','source':'ci','request_id':label+'-'+uuid.uuid4().hex[:12]}
 r=api('POST','runs',request);save(label+'-start',r);return r,request
root=pathlib.Path(__file__).resolve().parents[2]
env={'schema_version':'1.0','id':'kind-fixture-v1','name':'Isolated kind hostPath fixture','kind':'kubernetes','provider':'local-docker','region':'local','machines':[{'role':'client','model':'Docker Desktop VM','count':1,'cpu':'arm64','memory_bytes':0,'storage':'hostPath functional fixture','network':'kind bridge'}],'os':'Linux','config':{'cluster':a.cluster,'node':a.node,'benchmark_pvc':'bench','purpose':'functional-smoke-only'}}
(out/'environment.json').write_text(json.dumps(env))
subprocess.run([a.malta,'plan','--spec',str(root/'benchmarks/data/smoke-read.yaml'),'--environment',str(out/'environment.json'),'--version',str(root/'environments/version-example.yaml'),'--target','/bench','--out',str(out/'plan.json')],check=True,stdout=subprocess.DEVNULL)
plan=json.loads((out/'plan.json').read_text())
# Generate a long plan using only public CLI, preserving canonical plan hashing.
spec=plan['spec'].copy();spec['workload']=dict(spec['workload'],runtime_seconds=10);(out/'long-spec.json').write_text(json.dumps(spec))
subprocess.run([a.malta,'plan','--spec',str(out/'long-spec.json'),'--environment',str(out/'environment.json'),'--version',str(root/'environments/version-example.yaml'),'--target','/bench','--out',str(out/'long-plan.json')],check=True,stdout=subprocess.DEVNULL)
long_plan=json.loads((out/'long-plan.json').read_text())
try:
 api('GET','health')
 r,q=start(plan,'success');assert api('POST','runs',q)['id']==r['id']
 final=wait(r);save('success-final',final);assert final['status']=='succeeded',final
 bundle=api('GET','runs/'+r['id']+'/bundle');save('success-bundle',bundle)
 assert final['attempt']['job_uid'] and final['attempt']['pod_uids'] and final['attempt']['exit_code']==0
 assert any(f['path']=='environment-observed.json' for f in bundle['files'])
 for _ in range(40):
  jobs=json.loads(kubectl('get','jobs','-o','json'))['items']
  if not any(j['metadata']['name']==final['attempt']['job_name'] for j in jobs):break
  time.sleep(.3)
 else:raise AssertionError('successful Job not cleaned')
 assert api('GET','runs/'+r['id']+'/bundle')['run']['id']==r['id']
 print('PASS successful Job, evidence, identity, cleanup, duplicate request',flush=True)
 r,q=start(long_plan,'cancel');running(r);api('POST','runs/'+r['id']+'/cancel',{});final=wait(r);save('cancel-final',final);assert final['status']=='cancelled',final
 print('PASS cancellation after Pod starts',flush=True)
 r,q=start(long_plan,'restart');before=running(r);kubectl('rollout','restart','deployment/malta');kubectl('rollout','status','deployment/malta','--timeout=90s');final=wait(r);save('restart-final',final);assert final['status']=='succeeded',final
 assert api('POST','runs',q)['id']==r['id'] and final['attempt']['job_uid']==before['attempt']['job_uid']
 print('PASS service restart retains same Job and request',flush=True)
 r,q=start(long_plan,'pod-kill');v=running(r);kubectl('delete','pod','-l','job-name='+v['attempt']['job_name'],'--wait=true','--timeout=60s');final=wait(r);save('pod-kill-final',final);assert final['status']=='failed',final
 print('PASS Pod loss does not produce successful replay',flush=True)
 # Make only the server's final artifact store unwritable; work/results remain writable.
 kubectl('exec','deployment/malta','--','chmod','0500','/var/lib/malta/artifacts')
 try:
  r,q=start(plan,'persistence-failure');final=wait(r);save('persistence-failure-final',final);assert final['status']=='failed' and final['attempt']['result_state']=='failed',final
  job=json.loads(kubectl('get','job',final['attempt']['job_name'],'-o','json'));assert job['metadata']['uid']==final['attempt']['job_uid']
 finally:kubectl('exec','deployment/malta','--','chmod','0755','/var/lib/malta/artifacts')
 print('PASS persistence failure retains Job',flush=True)
 helper=root/'skills/malta-benchmark/scripts/session.py'
 session=out/'agent-session.json'
 def agent(action,*args):
  raw=subprocess.check_output(['python3',str(helper),action,'--malta',a.malta,'--session',str(session),*args],env=dict(os.environ,MALTA_TOKEN=token));return json.loads(raw)['data']
 r=agent('start','--server','http://127.0.0.1:19483','--plan',str(out/'plan.json'),'--request-id','agent-'+uuid.uuid4().hex[:12],'--executor','kubernetes')
 saved=json.loads(session.read_text());saved['run_id']=None;session.write_text(json.dumps(saved))
 assert agent('resume')['id']==r['id']
 final=wait(r);assert final['status']=='succeeded',final
 assert agent('compare','--baseline',r['id'])['comparable']
 save('agent-final',final)
 print('PASS public Agent session helper on Kubernetes',flush=True)

 save('summary',{'passed':True,'cluster':a.cluster,'node':a.node,'storage':'single-node hostPath fixture; not CSI certification'})
finally:
 if pf is not None:pf.terminate();pf.wait(timeout=10)
 log.close()
