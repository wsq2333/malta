#!/usr/bin/env python3
"""Render a persistent management-only service; never contacts Kubernetes."""
import argparse,json,re
p=argparse.ArgumentParser(description=__doc__)
p.add_argument('--namespace',required=True)
p.add_argument('--server-image',required=True)
p.add_argument('--storage-class',required=True)
p.add_argument('--state-size',default='20Gi')
p.add_argument('--image-pull-secret')
p.add_argument('--alibaba-internal-lb',action='store_true')
g=p.add_mutually_exclusive_group(required=True)
g.add_argument('--allow-unauthenticated',action='store_true')
g.add_argument('--token-secret')
a=p.parse_args()
if not re.fullmatch(r'[^\s]+@sha256:[a-f0-9]{64}',a.server_image):p.error('server image must be pinned by sha256 digest')
for value in [a.namespace,a.storage_class,a.image_pull_secret,a.token_secret]:
 if value and not re.fullmatch(r'[a-z0-9]([-a-z0-9.]*[a-z0-9])?',value):p.error('invalid resource name')
if not re.fullmatch(r'[1-9][0-9]*(Mi|Gi|Ti)',a.state_size):p.error('state size must be a positive Mi/Gi/Ti quantity')
labels={'app':'malta-server'}
def obj(api,kind,name,**fields):return {'apiVersion':api,'kind':kind,'metadata':{'name':name,'namespace':a.namespace},**fields}
args=['serve','--listen','0.0.0.0:8080','--data-dir','/var/lib/malta','--management-only']
if a.allow_unauthenticated:args+=['--allow-unauthenticated']
container={'name':'server','image':a.server_image,'command':['/usr/local/bin/malta'],'args':args,
 'ports':[{'name':'http','containerPort':8080}],
 'resources':{'requests':{'cpu':'250m','memory':'256Mi'},'limits':{'cpu':'1','memory':'1Gi'}},
 'securityContext':{'allowPrivilegeEscalation':False,'readOnlyRootFilesystem':True,'capabilities':{'drop':['ALL']}},
 'volumeMounts':[{'name':'state','mountPath':'/var/lib/malta'},{'name':'tmp','mountPath':'/tmp'}],
 'startupProbe':{'httpGet':{'path':'/api/v1/health','port':'http'},'periodSeconds':5,'failureThreshold':60},
 'readinessProbe':{'httpGet':{'path':'/api/v1/health','port':'http'},'periodSeconds':5},
 'livenessProbe':{'httpGet':{'path':'/api/v1/health','port':'http'},'periodSeconds':10,'failureThreshold':3}}
if a.token_secret:container['env']=[{'name':'MALTA_TOKEN','valueFrom':{'secretKeyRef':{'name':a.token_secret,'key':'token'}}}]
pod={'serviceAccountName':'malta-server','automountServiceAccountToken':False,'nodeSelector':{'kubernetes.io/os':'linux','kubernetes.io/arch':'amd64'},
 'securityContext':{'runAsNonRoot':True,'runAsUser':10001,'runAsGroup':10001,'fsGroup':10001,'fsGroupChangePolicy':'OnRootMismatch','seccompProfile':{'type':'RuntimeDefault'}},
 'terminationGracePeriodSeconds':30,'containers':[container],
 'volumes':[{'name':'state','persistentVolumeClaim':{'claimName':'malta-state'}},{'name':'tmp','emptyDir':{'sizeLimit':'64Mi'}}]}
if a.image_pull_secret:pod['imagePullSecrets']=[{'name':a.image_pull_secret}]
service=obj('v1','Service','malta',spec={'type':'LoadBalancer' if a.alibaba_internal_lb else 'ClusterIP','selector':labels,'ports':[{'name':'http','port':80,'targetPort':'http'}]})
if a.alibaba_internal_lb:service['metadata']['annotations']={'service.beta.kubernetes.io/alibaba-cloud-loadbalancer-address-type':'intranet'}
items=[{'apiVersion':'v1','kind':'Namespace','metadata':{'name':a.namespace}},
 obj('v1','ServiceAccount','malta-server',automountServiceAccountToken=False),
 obj('v1','PersistentVolumeClaim','malta-state',spec={'accessModes':['ReadWriteOnce'],'storageClassName':a.storage_class,'resources':{'requests':{'storage':a.state_size}}}),
 obj('apps/v1','Deployment','malta',spec={'replicas':1,'strategy':{'type':'Recreate'},'selector':{'matchLabels':labels},'template':{'metadata':{'labels':labels},'spec':pod}}),service]
print(json.dumps({'apiVersion':'v1','kind':'List','items':items},indent=2))
