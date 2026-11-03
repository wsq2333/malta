#!/usr/bin/env python3
"""Generate portable UCE EP and Malta case examples; contains no live configuration."""
import copy
import json
from pathlib import Path
import uuid
from tasks import DEFAULTS, RANGES

ROOT = Path(__file__).resolve().parents[2]
DEST = ROOT / 'docs/examples/workflow'
NS = uuid.UUID('d17dfc49-6d42-46e6-a6e8-896be281bb4a')
def uid(name): return str(uuid.uuid5(NS, name))
def write(name, data): (DEST / name).write_text(json.dumps(data, indent=2, ensure_ascii=False) + '\n')

inputs = [{'id': 'in-' + name, 'name': name, 'type': 'integer' if type(value) is int else 'string', 'required': True}
          for name, value in DEFAULTS.items()]
inputs.append({'id': 'in-context', 'name': 'malta_context', 'type': 'json', 'required': True})
stages = [('upload', 'dataset'), ('warmup', 'warmup'), ('fio-read', None)]
nodes, ops = [], []
for i, (action, artifact) in enumerate(stages):
    env = [{'name': 'MAREA_TEST_PARAMS', 'value': '{{ toJson .task_params }}'},
           {'name': 'RUN_ID', 'value': '{{ .pending_task.run_id }}'},
           {'name': 'STAGE_ID', 'value': '{{ .pending_task.stagerun_id }}'},
           {'name': 'ATTEMPT', 'value': '{{ .pending_task.attempt }}'},
           {'name': 'EVICTION_COUNT', 'value': '{{ .pending_task.eviction_count }}'},
           {'name': 'NODE_ID', 'value': uid(action + '-node')},
           {'name': 'RESULTS_ROOT', 'value': '/results'},
           {'name': 'BENCH_ROOT', 'value': '/bench'}]
    volumes = [{'name': 'results', 'persistentVolumeClaim': {'claimName': 'replace-shared-results-pvc'}},
               {'name': 'scratch', 'emptyDir': {'sizeLimit': '1Gi'}}]
    mounts = [{'name': 'results', 'mountPath': '/results'}, {'name': 'scratch', 'mountPath': '/tmp'}]
    if action == 'upload':
        env += [{'name': 'OSS_ENDPOINT', 'value': 'https://oss-region.example.invalid'},
                {'name': 'OSS_BUCKET', 'value': 'replace-bucket'}]
        for key in ('OSS_ACCESS_KEY_ID', 'OSS_ACCESS_KEY_SECRET', 'OSS_SESSION_TOKEN'):
            env.append({'name': key, 'valueFrom': {'secretKeyRef': {
                'name': 'replace-oss-secret', 'key': key, 'optional': key == 'OSS_SESSION_TOKEN'}}})
    else:
        volumes.append({'name': 'benchmark', 'persistentVolumeClaim': {'claimName': 'replace-curvine-pvc'}})
        mounts.append({'name': 'benchmark', 'mountPath': '/bench', 'readOnly': True})
    c = {'name': 'main', 'image': 'registry.example.invalid/marea-test-tools:replace',
         'command': ['/usr/local/bin/marea-test-tools'], 'args': [action], 'env': env,
         'resources': {'requests': {'cpu': '1', 'memory': '512Mi'}, 'limits': {'cpu': '1', 'memory': '1Gi'}},
         'volumeMounts': mounts}
    output_names = ([artifact] if artifact else []) + ['evidence']
    output_path = [{'name': n, 'path': '${output_root}/${run_id}/${stage_name}/${attempt}/${eviction_count}/' + n + '.json'}
                   for n in output_names]
    op = {'operator_id': uid(action + '-operator'), 'operator_version_id': uid(action + '-v1'),
          'name': action, 'version': '0.1.0', 'resource_pool': 'replace-resource-pool',
          'inputs': [{k: v for k, v in x.items() if k in ('name', 'type')} for x in inputs],
          'outputs': [], 'output_path': output_path, 'execution': {'retryLimit': 0},
          'manifest': {'apiVersion': 'v1', 'kind': 'Pod', 'metadata': {'name': action},
                       'spec': {'restartPolicy': 'Never', 'containers': [c], 'volumes': volumes,
                                'nodeSelector': {'kubernetes.io/hostname': 'replace-client-node'}}}}
    ops.append(op)
    nodes.append({'definition_node_id': uid(action + '-node'), 'name': action, 'kind': 'operator',
                  'target_version_id': op['operator_version_id'],
                  'input_bindings': {x['name']: {'kind': 'dag_input', 'input_id': x['id']} for x in inputs},
                  'input_path': [] if i == 0 else [{'from_node': uid(stages[i-1][0] + '-node'), 'output': stages[i-1][1]}]})
ep = {'schema_version': '0.1.0', 'artifact_contract': 'paths-v1', 'root_dag_version_id': uid('dag-v1'),
      'dag_definitions': [{'dag_id': uid('dag'), 'dag_version_id': uid('dag-v1'), 'name': 'oss-fuse-fio',
                           'version': '0.1.0', 'inputs': inputs, 'outputs': [], 'nodes': nodes, 'output_bindings': {}}],
      'operator_definitions': ops}
write('oss-fuse-fio.ep.example.json', ep)
properties = {}
for name, value in DEFAULTS.items():
    properties[name] = {'type': 'integer' if type(value) is int else 'string', 'default': value}
    if name in RANGES:
        properties[name].update(minimum=RANGES[name][0], maximum=RANGES[name][1])
properties['dataset_prefix'].update(pattern='^[A-Za-z0-9_-]+(/[A-Za-z0-9_-]+)*$', maxLength=160)
properties['fio_ioengine']['enum'] = ['psync', 'libaio']
rules = []
for action, names in [('upload', ['objects_verified', 'dataset_complete']),
                      ('warmup', ['full_read_completed', 'dataset_sizes_verified']),
                      ('fio-read', ['warmup_completed', 'dataset_sizes_verified'])]:
    rule = {'node_id': uid(action + '-node'), 'output': 'evidence', 'required': True,
            'parser': 'fio-3.33' if action == 'fio-read' else 'assertions-v1', 'assertions': names}
    if action == 'fio-read':
        rule.update(expected_options={'rw': 'read', 'allow_file_create': '0', 'time_based': '1'},
                    option_parameters={'bs': 'fio_bs_bytes', 'numjobs': 'fio_numjobs', 'iodepth': 'fio_iodepth',
                                       'ioengine': 'fio_ioengine', 'direct': 'fio_direct', 'runtime': 'fio_runtime_seconds'})
    rules.append(rule)
body = {'schema_version': 'workflow/v1alpha1', 'backend': 'uce', 'template': ep,
        'parameters': {'type': 'object', 'additionalProperties': False, 'properties': properties, 'required': list(DEFAULTS)},
        'defaults': DEFAULTS, 'timeout_seconds': 12000,
        'dataset': {'file_size_parameter': 'file_size_bytes', 'file_count_parameter': 'file_count'}, 'results': rules}
write('oss-fuse-fio.case.example.json', {'name': 'OSS 上传 / FUSE 全量预热 / fio 顺序读',
      'description': '参数化三阶段用例。需替换镜像、路由、OSS、PVC 和节点占位值；完整读取预热不证明缓存命中率。',
      'base_revision': 0, 'change_note': '首版工具链', 'body': body})
write('oss-fuse-fio.parameters.example.json', DEFAULTS)
large = copy.deepcopy(DEFAULTS);large['file_count'] = 65536;large['warmup_timeout_seconds'] = 7200
write('oss-fuse-fio.parameters-256gib.example.json', large)
print('Generated EP, Malta case, 1 GiB and 256 GiB parameter examples')
