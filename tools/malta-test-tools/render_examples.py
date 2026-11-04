#!/usr/bin/env python3
"""Generate portable Argo and Malta case examples; never embeds site configuration."""
import copy
import json
from pathlib import Path
from tasks import DEFAULTS, RANGES
ROOT = Path(__file__).resolve().parents[2]
DEST = ROOT / 'docs/examples/workflow'
def write(name, value): (DEST / name).write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n')
def secret(key): return {'name': key, 'valueFrom': {'secretKeyRef': {'name':'replace-oss-secret', 'key':key, 'optional':key=='OSS_SESSION_TOKEN'}}}
stages = [('upload','dataset'),('warmup','warmup'),('fio-read',None)]
templates=[]; dag=[]
for i,(action,artifact) in enumerate(stages):
    env=[{'name':'MALTA_PARAM_'+name.upper(),'value':'{{workflow.parameters.'+name+'}}'} for name in DEFAULTS]
    env += [{'name':'MALTA_CONTEXT','value':'{{workflow.parameters.malta_context}}'},
            {'name':'WORKFLOW_NAME','value':'{{workflow.name}}'}, {'name':'WORKFLOW_UID','value':'{{workflow.uid}}'},
            {'name':'ARGO_NODE_ID','valueFrom':{'fieldRef':{'fieldPath':"metadata.annotations['workflows.argoproj.io/node-id']"}}},
            {'name':'RESULTS_ROOT','value':'/results'}, {'name':'BENCH_ROOT','value':'/bench'}]
    mounts=[{'name':'results','mountPath':'/results'},{'name':'scratch','mountPath':'/tmp'}]
    task={'name':action,'template':action}
    template={'name':action}
    if i:
        prev,output=stages[i-1]
        task['dependencies']=[prev]
        task['arguments']={'parameters':[{'name':'input-path','value':'{{tasks.'+prev+'.outputs.parameters.'+output+'}}'}]}
        template['inputs']={'parameters':[{'name':'input-path'}]}
        env.append({'name':'MALTA_INPUT_PATH','value':'{{inputs.parameters.input-path}}'})
        mounts.append({'name':'benchmark','mountPath':'/bench','readOnly':True})
    else:
        env += [{'name':'OSS_ENDPOINT','value':'https://oss-region.example.invalid'}, {'name':'OSS_BUCKET','value':'replace-bucket'}]
        env += [secret(k) for k in ['OSS_ACCESS_KEY_ID','OSS_ACCESS_KEY_SECRET','OSS_SESSION_TOKEN']]
    names=([artifact] if artifact else [])+['evidence']
    template['outputs']={'parameters':[{'name':n,'valueFrom':{'path':'/tmp/malta-'+n+'-path'}} for n in names]}
    template['container']={'image':'registry.example.invalid/malta-test-tools:replace','command':['/usr/local/bin/malta-test-tools'],'args':[action],
       'env':env,'volumeMounts':mounts,'resources':{'requests':{'cpu':'1','memory':'512Mi'},'limits':{'cpu':'2','memory':'1Gi'}},
       'securityContext':{'allowPrivilegeEscalation':False,'readOnlyRootFilesystem':True,'capabilities':{'drop':['ALL']}}}
    templates.append(template);dag.append(task)
workflow={'apiVersion':'argoproj.io/v1alpha1','kind':'WorkflowTemplate','metadata':{'name':'malta-oss-fuse-fio'},'spec':{
 'entrypoint':'main','serviceAccountName':'replace-workflow-service-account','activeDeadlineSeconds':1200,
 'securityContext':{'runAsNonRoot':True,'runAsUser':10001,'runAsGroup':10001,'fsGroup':10001,'fsGroupChangePolicy':'OnRootMismatch','seccompProfile':{'type':'RuntimeDefault'}},
 'nodeSelector':{'kubernetes.io/hostname':'replace-client-node'},'imagePullSecrets':[{'name':'replace-registry-secret'}],
 'arguments':{'parameters':[{'name':k,'value':str(v)} for k,v in DEFAULTS.items()]+[{'name':'malta_context'}]},
 'volumes':[{'name':'results','persistentVolumeClaim':{'claimName':'replace-shared-results-pvc'}},
            {'name':'benchmark','persistentVolumeClaim':{'claimName':'replace-curvine-pvc','readOnly':True}},
            {'name':'scratch','emptyDir':{'sizeLimit':'1Gi'}}],
 'templates':[{'name':'main','dag':{'tasks':dag}}]+templates}}
properties={}
for name,value in DEFAULTS.items():
    properties[name]={'type':'integer' if type(value) is int else 'string','default':value}
    if name in RANGES:properties[name].update(minimum=RANGES[name][0],maximum=RANGES[name][1])
properties['dataset_prefix'].update(pattern='^[A-Za-z0-9_-]+(/[A-Za-z0-9_-]+)*$',maxLength=160)
properties['fio_ioengine']['enum']=['psync','libaio']
rules=[]
for action,assertions in [('upload',['objects_verified','dataset_complete']),('warmup',['full_read_completed','dataset_sizes_verified']),('fio-read',['warmup_completed','dataset_sizes_verified'])]:
    rule={'node_id':action,'output':'evidence','required':True,'parser':'fio-3.33' if action=='fio-read' else 'assertions-v1','assertions':assertions}
    if action=='fio-read':rule.update(expected_options={'rw':'read','allow_file_create':'0','time_based':'1'},option_parameters={'bs':'fio_bs_bytes','numjobs':'fio_numjobs','iodepth':'fio_iodepth','ioengine':'fio_ioengine','direct':'fio_direct','runtime':'fio_runtime_seconds'})
    rules.append(rule)
body={'schema_version':'workflow/v1alpha1','backend':'argo','template':workflow,'parameters':{'type':'object','additionalProperties':False,'properties':properties,'required':list(DEFAULTS)},'defaults':DEFAULTS,'timeout_seconds':1200,'dataset':{'file_size_parameter':'file_size_bytes','file_count_parameter':'file_count'},'results':rules}
write('oss-fuse-fio.argo.example.json',workflow)
write('oss-fuse-fio.argo-case.example.json',{'name':'OSS 上传 / FUSE 全读 / fio 顺序读（Argo）','description':'1 GiB 参数化三阶段用例；使用前绑定镜像 digest、Secret、PVC、ServiceAccount 和节点。全读不证明缓存命中。','base_revision':0,'change_note':'Argo 工具首版','body':body})
write('oss-fuse-fio.argo-parameters.example.json',DEFAULTS)
print('Generated native Argo template, Malta case and 1 GiB parameters')
