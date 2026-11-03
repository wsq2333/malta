#!/usr/bin/env python3
"""UCE paths-v1 tasks: OSS upload -> FUSE read-through warmup -> fio evidence."""
import argparse
import concurrent.futures
import hashlib
import ipaddress
import json
import os
from pathlib import Path
import random
import re
import signal
import subprocess
import tempfile
import time
from urllib.parse import urlsplit

VERSION = '0.1.0'
DEFAULTS = {
    'file_size_bytes': 4194304, 'file_count': 256,
    'dataset_prefix': 'malta-read-cache', 'data_seed': 1,
    'upload_concurrency': 4, 'warmup_timeout_seconds': 600,
    'fio_bs_bytes': 1048576, 'fio_numjobs': 1, 'fio_iodepth': 1,
    'fio_ioengine': 'psync', 'fio_direct': 1, 'fio_runtime_seconds': 60,
}
RANGES = {
    'file_size_bytes': (4096, 1073741824), 'file_count': (1, 65536),
    'data_seed': (0, 2147483647), 'upload_concurrency': (1, 16),
    'warmup_timeout_seconds': (1, 21600), 'fio_bs_bytes': (4096, 16777216),
    'fio_numjobs': (1, 16), 'fio_iodepth': (1, 128),
    'fio_direct': (0, 1), 'fio_runtime_seconds': (1, 3600),
}


class TaskError(Exception):
    """Only predefined, non-secret messages may be exposed to operators."""


def require(condition, message):
    if not condition:
        raise TaskError(message)


def encode(value):
    return json.dumps(value, sort_keys=True, separators=(',', ':')).encode()


def sha(path):
    h = hashlib.sha256()
    with Path(path).open('rb') as f:
        for block in iter(lambda: f.read(1 << 20), b''):
            h.update(block)
    return h.hexdigest()


def parameters(raw):
    require(isinstance(raw, dict), 'task_params must be an object')
    p = {k: raw.get(k, v) for k, v in DEFAULTS.items()}
    for k, (lo, hi) in RANGES.items():
        require(type(p[k]) is int and lo <= p[k] <= hi, 'invalid parameter: ' + k)
    require(isinstance(p['dataset_prefix'], str) and
            re.fullmatch(r'[A-Za-z0-9_-]+(?:/[A-Za-z0-9_-]+)*', p['dataset_prefix']) and
            len(p['dataset_prefix']) <= 160, 'unsafe dataset_prefix')
    require(p['fio_ioengine'] in ('psync', 'libaio'), 'unsupported fio_ioengine')
    require(p['fio_ioengine'] != 'psync' or p['fio_iodepth'] == 1,
            'psync requires fio_iodepth=1')
    require(p['file_size_bytes'] % 4096 == 0 and p['fio_bs_bytes'] % 4096 == 0,
            'file size and block size must be 4096-byte aligned')
    require(p['fio_bs_bytes'] <= p['file_size_bytes'], 'block size exceeds file size')
    require(p['file_size_bytes'] * p['file_count'] <= 256 * 1024**3,
            'dataset exceeds 256 GiB tool budget')
    require(p['file_size_bytes'] * p['upload_concurrency'] <= 1024**3, 'upload scratch exceeds 1 GiB')
    return p


def safe_path(root, value):
    root, path = Path(root).absolute(), Path(value)
    require(path.is_absolute() and '..' not in path.parts, 'unsafe artifact path')
    try:
        relative = path.relative_to(root)
    except ValueError:
        raise TaskError('artifact outside approved root') from None
    current = root
    require(not current.is_symlink(), 'symlink root rejected')
    for part in relative.parts:
        current /= part
        require(not current.is_symlink(), 'symlink artifact rejected')
    return path


def read_json(path, limit=24 * 1024**2):
    require(path.stat().st_size <= limit, 'JSON exceeds size limit')
    with path.open('rb') as f:
        return json.load(f)


def atomic(path, value):
    path.parent.mkdir(parents=True, exist_ok=True)
    fd, temporary = tempfile.mkstemp(prefix='.publish-', dir=path.parent)
    try:
        with os.fdopen(fd, 'wb') as f:
            f.write(encode(value)); f.flush(); os.fsync(f.fileno())
        os.replace(temporary, path)
    finally:
        Path(temporary).unlink(missing_ok=True)


class Task:
    def __init__(self, action):
        self.action = action
        raw = json.loads(os.environ.get('MAREA_TEST_PARAMS', '{}'))
        self.p = parameters(raw)
        self.context = raw.get('malta_context', {})
        for k in ('workflow_version_id', 'snapshot_hash', 'environment_id', 'software_version_id'):
            require(isinstance(self.context.get(k), str) and bool(self.context[k]),
                    'malta_context missing: ' + k)
        self.run = os.environ['RUN_ID']
        self.stage = os.environ['STAGE_ID']
        self.node = os.environ['NODE_ID']
        for value in (self.run, self.stage, self.node):
            require(re.fullmatch(r'[A-Za-z0-9_-]{1,128}', value), 'invalid runtime identity')
        self.attempt = int(os.environ['ATTEMPT'])
        self.eviction = int(os.environ['EVICTION_COUNT'])
        require(self.attempt >= 0 and self.eviction >= 0, 'invalid attempt identity')
        self.root = Path(os.environ.get('RESULTS_ROOT', '/results'))
        self.bench = Path(os.environ.get('BENCH_ROOT', '/bench'))
        self.key_prefix = self.p['dataset_prefix'] + '/' + self.run
        self.outputs = [safe_path(self.root, p) for p in json.loads(os.environ['MAREA_OUTPUT_PATHS'])]
        self.inputs = [safe_path(self.root, p) for p in json.loads(os.environ.get('MAREA_INPUT_PATHS', '[]'))]
        require(len(self.outputs) == (2 if action != 'fio-read' else 1), 'unexpected output count')
        require(len(set(self.outputs)) == len(self.outputs), 'duplicate output path')
        require(len(self.inputs) == (0 if action == 'upload' else 1), 'unexpected input count')
        for path in self.outputs + self.inputs:
            require(self.run in path.relative_to(self.root).parts, 'artifact belongs to another run')
        require(len({p.parent for p in self.outputs}) == 1, 'outputs must share attempt directory')
        self.folder = self.outputs[0].parent
        self.folder.mkdir(parents=True, exist_ok=True)
        self.dataset_dir = safe_path(self.bench, self.bench / self.key_prefix)

    def evidence(self, parser, assertions, files):
        manifest = {'schema_version': 'malta.evidence/v1',
                    'workflow_version_id': self.context['workflow_version_id'],
                    'run_id': self.run, 'node_id': self.node, 'stage_run_id': self.stage,
                    'attempt': self.attempt, 'eviction_count': self.eviction,
                    'snapshot_hash': self.context['snapshot_hash'],
                    'environment_id': self.context['environment_id'],
                    'software_version_id': self.context['software_version_id'],
                    'parser': parser, 'assertions': assertions, 'files': []}
        for path in files:
            require(path.parent == self.folder, 'report must be beside manifest')
            manifest['files'].append({'path': path.name, 'size': path.stat().st_size, 'sha256': sha(path)})
        atomic(self.outputs[-1], manifest)

    def check_dataset(self, d):
        require(d.get('schema_version') == 'marea.test.dataset/v1' and
                d.get('run_id') == self.run and d.get('parameters') == self.p and
                d.get('key_prefix') == self.key_prefix, 'dataset identity or parameters differ')
        files = d.get('files', [])
        require(len(files) == self.p['file_count'], 'dataset count differs')
        for i, f in enumerate(files):
            require(f['name'] == f'part-{i}.bin' and f['size'] == self.p['file_size_bytes'] and
                    re.fullmatch('[a-f0-9]{64}', f['sha256']), 'invalid dataset entry')
        return d

    def check_visible(self, d):
        self.check_dataset(d)
        for f in d['files']:
            path = safe_path(self.bench, self.dataset_dir / f['name'])
            require(path.is_file() and path.stat().st_size == f['size'],
                    'FUSE dataset missing or size mismatch; verify OSS mount mapping')

    def fio(self, warmup=False):
        p = self.p
        report = self.folder / ('warmup-fio.json' if warmup else 'fio.json')
        config = self.folder / ('warmup.fio' if warmup else 'read.fio')
        # Every job reads the SAME uploaded file set. numjobs never multiplies prepared bytes.
        opts = {'directory': str(self.dataset_dir), 'filename_format': 'part-$filenum.bin',
                'nrfiles': p['file_count'], 'filesize': p['file_size_bytes'],
                'size': p['file_count'] * p['file_size_bytes'], 'openfiles': min(64, p['file_count']),
                'rw': 'read', 'bs': min(1048576, p['file_size_bytes']) if warmup else p['fio_bs_bytes'],
                'numjobs': 1 if warmup else p['fio_numjobs'],
                'iodepth': 1 if warmup else p['fio_iodepth'],
                'ioengine': 'psync' if warmup else p['fio_ioengine'], 'direct': p['fio_direct'],
                'allow_file_create': 0, 'group_reporting': 1, 'file_service_type': 'sequential',
                'invalidate': 1, 'runtime': 0 if warmup else p['fio_runtime_seconds']}
        if not warmup:
            opts['time_based'] = 1
        require(all('\n' not in str(v) and '\r' not in str(v) for v in opts.values()), 'unsafe fio option')
        config.write_text('[read]\n' + ''.join(f'{k}={v}\n' for k, v in opts.items()))
        timeout = p['warmup_timeout_seconds'] if warmup else p['fio_runtime_seconds'] + 120
        fio = os.environ.get('FIO_BIN', '/usr/bin/fio')
        require(subprocess.check_output([fio, '--version'], text=True).strip() == 'fio-3.33', 'fio 3.33 required')
        proc = subprocess.Popen([fio, '--readonly', '--output-format=json', '--output=' + str(report), str(config)],
                                stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, start_new_session=True)
        try:
            rc = proc.wait(timeout=timeout)
        except subprocess.TimeoutExpired:
            os.killpg(proc.pid, signal.SIGKILL); proc.wait()
            raise TaskError('fio exceeded task deadline') from None
        require(rc == 0, 'fio failed; inspect mounted target and saved report')
        result = read_json(report)
        jobs = result.get('jobs', [])
        require(result.get('fio version') == 'fio-3.33' and len(jobs) == 1 and jobs[0].get('error') == 0,
                'fio report is not one successful aggregate')
        require(jobs[0]['read']['io_bytes'] > 0, 'fio read no data')
        if warmup:
            require(jobs[0]['read']['io_bytes'] == p['file_count'] * p['file_size_bytes'],
                    'warmup did not read the complete dataset')
        return report, config

    def upload(self):
        bucket = oss_bucket()
        p = self.p
        def one(i):
            name = f'part-{i}.bin'
            rng = random.Random(f"{p['data_seed']}:{i}")
            with tempfile.TemporaryDirectory(prefix='marea-upload-') as tmp:
                file = Path(tmp) / name
                with file.open('wb') as f:
                    left = p['file_size_bytes']
                    while left:
                        block = rng.randbytes(min(left, 1 << 20)); f.write(block); left -= len(block)
                checksum = sha(file)
                key = self.key_prefix + '/' + name
                try:
                    bucket.put_object_from_file(key, str(file), headers={
                        'x-oss-forbid-overwrite': 'true', 'x-oss-meta-sha256': checksum,
                        'x-oss-meta-malta-run': self.run})
                except Exception as e:
                    # A replay may only reuse byte-identical objects belonging to this Run.
                    if getattr(e, 'status', None) != 409:
                        raise TaskError('OSS upload failed; check credentials, endpoint and permissions') from None
                try:
                    h = bucket.head_object(key)
                    require(h.content_length == p['file_size_bytes'] and
                            h.headers.get('x-oss-meta-sha256') == checksum and
                            h.headers.get('x-oss-meta-malta-run') == self.run, 'OSS object verification failed')
                except TaskError:
                    raise
                except Exception:
                    raise TaskError('OSS object HEAD failed') from None
                return {'name': name, 'size': p['file_size_bytes'], 'sha256': checksum}
        # Bounded batches avoid keeping 65K queued futures/tempfiles in memory.
        files = []
        with concurrent.futures.ThreadPoolExecutor(max_workers=p['upload_concurrency']) as pool:
            for start in range(0, p['file_count'], p['upload_concurrency']):
                files.extend(pool.map(one, range(start, min(start + p['upload_concurrency'], p['file_count']))))
        atomic(self.outputs[0], {'schema_version': 'marea.test.dataset/v1', 'run_id': self.run,
                               'parameters': p, 'key_prefix': self.key_prefix, 'files': files})
        self.evidence('assertions-v1', {'objects_verified': True, 'dataset_complete': True}, [self.outputs[0]])

    def warmup(self):
        d = read_json(self.inputs[0]); self.check_visible(d)
        report, config = self.fio(warmup=True)
        atomic(self.outputs[0], {'schema_version': 'marea.test.warmup/v1', 'run_id': self.run,
                               'dataset_path': str(self.inputs[0]), 'dataset_sha256': sha(self.inputs[0]),
                               'method': 'fuse-full-read', 'cache_hit_verified': False,
                               'read_bytes': self.p['file_count'] * self.p['file_size_bytes']})
        self.evidence('assertions-v1', {'full_read_completed': True, 'dataset_sizes_verified': True},
                      [self.outputs[0], report, config])

    def read(self):
        w = read_json(self.inputs[0])
        require(w.get('schema_version') == 'marea.test.warmup/v1' and w.get('run_id') == self.run and
                w.get('method') == 'fuse-full-read', 'warmup identity mismatch')
        dataset = safe_path(self.root, w['dataset_path'])
        require(self.run in dataset.relative_to(self.root).parts and sha(dataset) == w['dataset_sha256'],
                'dataset changed after warmup')
        d = read_json(dataset); self.check_visible(d)
        require(w.get('read_bytes') == self.p['file_count'] * self.p['file_size_bytes'], 'warmup byte count differs')
        report, config = self.fio()
        self.evidence('fio-3.33', {'warmup_completed': True, 'dataset_sizes_verified': True}, [report, config])


def oss_bucket():
    import oss2
    endpoint = os.environ.get('OSS_ENDPOINT', '')
    u = urlsplit(endpoint)
    try:
        loopback = ipaddress.ip_address(u.hostname or '').is_loopback
    except ValueError:
        loopback = False
    require(u.hostname and not u.username and not u.password and not u.query and not u.fragment and
            u.path in ('', '/') and (u.scheme == 'https' or (u.scheme == 'http' and loopback)),
            'OSS endpoint must use HTTPS (HTTP loopback only for isolated tests)')
    bucket = os.environ.get('OSS_BUCKET', '')
    require(re.fullmatch('[a-z0-9][a-z0-9-]{1,61}[a-z0-9]', bucket), 'invalid OSS bucket')
    key, secret, token = (os.environ.get(k, '') for k in ('OSS_ACCESS_KEY_ID', 'OSS_ACCESS_KEY_SECRET', 'OSS_SESSION_TOKEN'))
    require(key and secret, 'OSS credentials missing')
    auth = oss2.StsAuth(key, secret, token) if token else oss2.Auth(key, secret)
    return oss2.Bucket(auth, endpoint, bucket, connect_timeout=30, is_path_style=loopback)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=['upload', 'warmup', 'fio-read', 'version'])
    args = parser.parse_args()
    if args.action == 'version':
        import oss2
        print(json.dumps({'tool': 'marea-test-tools', 'version': VERSION, 'oss_sdk': oss2.__version__,
                          'fio': subprocess.check_output(['/usr/bin/fio', '--version'], text=True).strip()}))
        return 0
    try:
        task = Task(args.action)
        {'upload': task.upload, 'warmup': task.warmup, 'fio-read': task.read}[args.action]()
        print(json.dumps({'task': args.action, 'status': 'succeeded', 'run_id': task.run,
                          'prepared_bytes': task.p['file_size_bytes'] * task.p['file_count']}))
        return 0
    except TaskError as e:
        print(json.dumps({'task': args.action, 'status': 'failed', 'error': str(e)}))
    except Exception as e:
        # SDK exceptions can contain signed URLs or credentials: never print their body/traceback.
        print(json.dumps({'task': args.action, 'status': 'failed', 'error_type': type(e).__name__}))
    return 1


if __name__ == '__main__':
    raise SystemExit(main())
