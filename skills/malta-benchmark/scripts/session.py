#!/usr/bin/env python3
"""Provider-neutral session checkpointing over the public Malta CLI only."""
import argparse,fcntl,json,os,pathlib,subprocess,sys,tempfile

class Failure(Exception):
    def __init__(self,code,message,exit_code=3): self.code,self.message,self.exit_code=code,message,exit_code

def invoke(binary,server,*args):
    try: r=subprocess.run([binary,'--format','json','--non-interactive','--server',server,*args],capture_output=True,text=True,timeout=35)
    except subprocess.TimeoutExpired: raise Failure('timeout','CLI timed out; resume this session and look up the same request ID',4)
    try: value=json.loads(r.stdout)
    except ValueError: raise Failure('protocol','CLI did not return a JSON envelope')
    if value.get('schema_version')!='1.0': raise Failure('unsupported','Unsupported CLI envelope schema',2)
    if r.returncode or 'error' in value:
        error=value.get('error',{});raise Failure(error.get('code','execution'),error.get('message','CLI failed'),r.returncode or 3)
    return value['data']

def persist(path,state):
    fd,tmp=tempfile.mkstemp(prefix='.malta-session-',dir=path.parent)
    try:
        with os.fdopen(fd,'w') as f: json.dump(state,f,indent=2);f.write('\n');f.flush();os.fsync(f.fileno())
        os.replace(tmp,path)
        directory=os.open(path.parent,os.O_RDONLY)
        try: os.fsync(directory)
        finally: os.close(directory)
    finally:
        if os.path.exists(tmp):os.unlink(tmp)

def run(a):
    path=pathlib.Path(a.session).resolve();path.parent.mkdir(parents=True,exist_ok=True)
    with open(str(path)+'.lock','a') as lock:
        os.chmod(lock.name,0o600)
        try:fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
        except BlockingIOError:raise Failure('conflict','Another process is using this session',6)
        if path.exists():
            state=json.loads(path.read_text())
            if state.get('schema_version')!='1.0':raise Failure('unsupported','Unknown session schema',2)
            for key,val in [('server',a.server),('request_id',a.request_id),('executor',a.executor)]:
                if val is not None and val!=state[key]:raise Failure('conflict','Session '+key+' differs',6)
            if a.plan and json.loads(pathlib.Path(a.plan).read_text())!=state['plan']:raise Failure('conflict','Session plan differs',6)
        else:
            if a.action!='start' or not all((a.server,a.plan,a.request_id,a.executor)):raise Failure('invalid','New sessions require start, server, plan, request-id and executor',2)
            state={'schema_version':'1.0','server':a.server,'request_id':a.request_id,'executor':a.executor,'plan':json.loads(pathlib.Path(a.plan).read_text()),'run_id':None}
            persist(path,state) # Durable before the first network side effect.
        def cli(*args):return invoke(a.malta,state['server'],*args)
        caps=cli('capabilities')
        if caps['schema_version']!='1.0' or not caps['version'].startswith('0.1.') or not caps['features'].get('request_lookup'):raise Failure('unsupported','This skill requires Malta 0.1.x and request lookup',2)
        try: result=cli('run','lookup','--request-id',state['request_id'])
        except Failure as e:
            if e.code!='not_found':raise
            if a.action!='start':raise Failure('not_found','No run recorded; resume never starts work. Explicit start can submit the saved request.')
            if state['executor'] not in caps['executors']:raise Failure('unsupported','Executor is not enabled on this server',2)
            fd,plan=tempfile.mkstemp(prefix='.malta-plan-',suffix='.json',dir=path.parent)
            try:
                with os.fdopen(fd,'w') as f:json.dump(state['plan'],f)
                result=cli('run','start','--plan',plan,'--request-id',state['request_id'],'--source','agent','--executor',state['executor'])
            finally:os.unlink(plan)
        if result['plan']['hash']!=state['plan']['hash'] or result['source']!='agent' or result['attempt']['executor']!=state['executor'] or (state['run_id'] and state['run_id']!=result['id']):raise Failure('conflict','Request resolves to a different run identity',6)
        state['run_id']=result['id'];persist(path,state)
        if a.action=='cancel':result=cli('run','cancel',state['run_id'])
        if a.action=='compare':
            if not a.baseline:raise Failure('invalid','compare requires --baseline RUN_ID',2)
            result=cli('compare','--baseline',a.baseline,'--candidate',state['run_id'])
        return result

def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('action',choices=['start','resume','cancel','compare'])
    p.add_argument('--malta',default='malta');p.add_argument('--session',required=True)
    p.add_argument('--server');p.add_argument('--plan');p.add_argument('--request-id')
    p.add_argument('--executor',choices=['local','kubernetes']);p.add_argument('--baseline')
    a=p.parse_args()
    try:print(json.dumps({'schema_version':'1.0','data':run(a)}));return 0
    except Failure as e:print(json.dumps({'schema_version':'1.0','error':{'code':e.code,'message':e.message}}));return e.exit_code
    except (OSError,ValueError,KeyError) as e:print(json.dumps({'schema_version':'1.0','error':{'code':'invalid','message':str(e)}}));return 2
if __name__=='__main__':sys.exit(main())
