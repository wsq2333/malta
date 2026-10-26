#!/usr/bin/env python3
"""Black-box skill/CLI contract acceptance; does not invoke an LLM or read the DB."""
import argparse,json,os,pathlib,subprocess,time,urllib.request
p=argparse.ArgumentParser(description=__doc__);p.add_argument('--malta',required=True);p.add_argument('--fio',required=True);p.add_argument('--output',required=True);a=p.parse_args()
root=pathlib.Path(__file__).resolve().parents[2];out=pathlib.Path(a.output).resolve();out.mkdir(parents=True,exist_ok=False);(out/'data').mkdir()
url='http://127.0.0.1:19484';log=(out/'server.log').open('w')
def cli(*args):
 r=subprocess.run([a.malta,*args],capture_output=True,text=True);return r.returncode,json.loads(r.stdout)
script=root/'skills/malta-benchmark/scripts/session.py';session=out/'session.json'
def helper(action,*args):
 r=subprocess.run(['python3',str(script),action,'--malta',a.malta,'--session',str(session),*args],capture_output=True,text=True);return r.returncode,json.loads(r.stdout)
server=subprocess.Popen([a.malta,'serve','--listen','127.0.0.1:19484','--allow-target',str(out/'data'),'--data-dir',str(out/'store'),'--fio',a.fio,'--run-timeout','1m'],stdout=log,stderr=log)
try:
 for _ in range(100):
  try:
   urllib.request.urlopen(url+'/api/v1/health',timeout=1).close();break
  except OSError:time.sleep(.1)
 else:raise AssertionError('server not ready')
 code,caps=cli('--server',url,'capabilities');assert code==0 and caps['data']['executors']==['local']
 code,plan=cli('plan','--spec',str(root/'benchmarks/data/smoke-read.yaml'),'--environment',str(root/'environments/local.yaml'),'--version',str(root/'environments/version-example.yaml'),'--target',str(out/'data'),'--out',str(out/'plan.json'));assert code==0,plan
 code,start=helper('start','--server',url,'--plan',str(out/'plan.json'),'--request-id','skill-contract-001','--executor','local');assert code==0,start
 run=start['data']['id']
 # Simulate a conversation losing the returned run ID after successful submission.
 saved=json.loads(session.read_text());saved['run_id']=None;session.write_text(json.dumps(saved))
 code,resumed=helper('resume');assert code==0 and resumed['data']['id']==run,resumed
 code,retried=helper('start');assert code==0 and retried['data']['id']==run,retried
 code,conflict=helper('start','--executor','kubernetes');assert code==6,conflict
 code,wait=cli('--server',url,'run','wait',run,'--timeout','1ms');assert code==4,wait
 code,resumed=helper('resume');assert code==0 and resumed['data']['id']==run
 code,completed=cli('--server',url,'run','wait',run,'--timeout','55s');assert code==0,completed
 code,comparison=helper('compare','--baseline',run);assert code==0 and comparison['data']['comparable'],comparison
 saved=json.loads(session.read_text());saved['request_id']='never-submitted';saved['run_id']=None;session.write_text(json.dumps(saved))
 code,missing=helper('resume');assert code==3 and missing['error']['code']=='not_found',missing
 code,history=cli('--server',url,'run','list');assert code==0 and len(history['data'])==1,history
 evidence={'passed':True,'run_id':run,'capabilities':caps['data'],'comparison':comparison['data'],'tests':['lost-response recovery','stable request','conflict','wait timeout','resume never submits'],'model_invoked':False}
 (out/'acceptance.json').write_text(json.dumps(evidence,indent=2)+'\n');print('PASS: public CLI/skill session recovery, conflict, wait timeout, comparison and no duplicate I/O')
finally:
 server.terminate();server.wait(timeout=20);log.close()
