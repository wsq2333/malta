#!/usr/bin/env python3
"""Render deployment manifests from explicit inputs; never contacts a cluster."""
import argparse, json, pathlib, re
p=argparse.ArgumentParser(description=__doc__)
p.add_argument('--config',required=True)
p.add_argument('--server-image',required=True)
p.add_argument('--state-pvc',required=True)
p.add_argument('--token-secret',required=True)
a=p.parse_args()
c=json.loads(pathlib.Path(a.config).read_text())
if c.get('context'): p.error('in-cluster deployment requires an empty context')
if not re.fullmatch(r'[^\s]+@sha256:[a-f0-9]{64}',a.server_image): p.error('server image must use a sha256 digest')
if a.state_pvc in (c['benchmark_pvc'],c['results_pvc']): p.error('SQLite state requires a separate PVC')
values={'NAMESPACE':c['namespace'],'SERVER_IMAGE':a.server_image,'STATE_PVC':a.state_pvc,'TOKEN_SECRET':a.token_secret,'RESULTS_PVC':c['results_pvc'],'RESULTS_DIR':c['results_dir'],'RUNNER_SA':c['service_account'],'BENCH_PVC':c['benchmark_pvc'],'NODE':c['node']}
def render(x):
    if isinstance(x,str):
        for k,v in values.items(): x=x.replace('${'+k+'}',v)
        if '${' in x: p.error('unresolved template parameter')
    elif isinstance(x,list): x=[render(v) for v in x]
    elif isinstance(x,dict): x={k:render(v) for k,v in x.items()}
    return x
items=[]
for path in sorted((pathlib.Path(__file__).resolve().parents[1]/'deploy/kubernetes/base').glob('*.json')):
    obj=render(json.loads(path.read_text()))
    items.extend(obj['items'] if obj['kind']=='List' else [obj])
items.append({'apiVersion':'v1','kind':'ConfigMap','metadata':{'name':'malta-executor','namespace':c['namespace']},'data':{'executor.json':json.dumps(c)}})
print(json.dumps({'apiVersion':'v1','kind':'List','items':items},indent=2))
