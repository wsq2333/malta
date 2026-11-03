import contextlib
import copy
import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import importlib.util
import io
import json
import os
from pathlib import Path
import tempfile
import threading
import unittest
from unittest.mock import patch
from urllib.parse import unquote

SPEC = importlib.util.spec_from_file_location('tasks', Path(__file__).resolve().parents[1] / 'tasks.py')
t = importlib.util.module_from_spec(SPEC); SPEC.loader.exec_module(t)


class Parameters(unittest.TestCase):
    def test_budget_and_independent_concurrency(self):
        p = t.parameters({'file_count': 65536})
        self.assertEqual(p['file_count'] * p['file_size_bytes'], 256 * 1024**3)
        self.assertEqual(p['fio_numjobs'], 1)
        for bad in ({'file_count': True}, {'dataset_prefix': '../escape'}, {'dataset_prefix': 'x\nrw=write'},
                    {'file_count': 65537}, {'fio_iodepth': 2}, {'fio_bs_bytes': 3},
                    {'file_size_bytes': 1073741824, 'upload_concurrency': 4}):
            with self.subTest(bad=bad), self.assertRaises(t.TaskError): t.parameters(bad)

    def test_no_escape_or_symlink(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / 'link').symlink_to('/tmp')
            for p in (root / '../escape', root / 'link/report.json', Path('/elsewhere/report.json')):
                with self.assertRaises(t.TaskError): t.safe_path(root, p)


class Pipeline(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='tools-validation-')
        self.root = Path(self.temp.name)
        self.results = self.root / 'results'; self.results.mkdir()
        self.bench = self.root / 'bench'; self.bench.mkdir()
        self.objects = {}
        self.puts = 0
        owner = self
        class OSS(BaseHTTPRequestHandler):
            def log_message(self, *args): pass
            def key(self):
                path = unquote(self.path.split('?', 1)[0])
                if not path.startswith('/test-bucket/'):
                    raise AssertionError('unexpected bucket')
                return path[len('/test-bucket/'):]
            def do_PUT(self):
                key = self.key()
                body = self.rfile.read(int(self.headers['Content-Length']))
                if key in owner.objects:
                    raw = b'<Error><Code>FileAlreadyExists</Code><Message>exists</Message></Error>'
                    self.send_response(409); self.send_header('Content-Length', str(len(raw))); self.end_headers();self.wfile.write(raw); return
                owner.puts += 1
                owner.objects[key] = (body, {k.lower(): v for k, v in self.headers.items() if k.lower().startswith('x-oss-meta-')})
                target = owner.bench / key;target.parent.mkdir(parents=True, exist_ok=True);target.write_bytes(body)
                self.send_response(200);self.send_header('ETag', hashlib.md5(body).hexdigest());self.send_header('Content-Length', '0');self.end_headers()
            def do_HEAD(self):
                key = self.key()
                if key not in owner.objects:self.send_response(404);self.end_headers();return
                body, headers = owner.objects[key]
                self.send_response(200);self.send_header('Content-Length', str(len(body)))
                for k, v in headers.items():self.send_header(k, v)
                self.end_headers()
        self.server = ThreadingHTTPServer(('127.0.0.1', 0), OSS)
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True);self.thread.start()
        self.params = dict(t.DEFAULTS, file_count=4, fio_numjobs=2, fio_runtime_seconds=1)
        self.context = {'workflow_version_id': 'version-test', 'snapshot_hash': 'snapshot-test',
                        'environment_id': 'env-test', 'software_version_id': 'software-test'}
        self.env = {'MAREA_TEST_PARAMS': json.dumps(dict(self.params, malta_context=self.context)),
                    'RUN_ID': 'run-test', 'STAGE_ID': 'upload-attempt', 'NODE_ID': 'upload-node',
                    'ATTEMPT': '0', 'EVICTION_COUNT': '0', 'RESULTS_ROOT': str(self.results),
                    'BENCH_ROOT': str(self.bench), 'OSS_ENDPOINT': 'http://127.0.0.1:' + str(self.server.server_port),
                    'OSS_BUCKET': 'test-bucket', 'OSS_ACCESS_KEY_ID': 'isolated-test-id',
                    'OSS_ACCESS_KEY_SECRET': 'isolated-test-secret', 'OSS_SESSION_TOKEN': ''}
        self.patch = patch.dict(os.environ, self.env);self.patch.start()
        self.published = {}

    def tearDown(self):
        self.patch.stop();self.server.shutdown();self.server.server_close();self.thread.join();self.temp.cleanup()

    def task(self, action, input_path=None):
        artifact = {'upload': 'dataset', 'warmup': 'warmup', 'fio-read': None}[action]
        folder = self.results / 'run-test' / action / '0' / '0'
        outputs = ([folder / (artifact + '.json')] if artifact else []) + [folder / 'evidence.json']
        os.environ.update(MAREA_OUTPUT_PATHS=json.dumps(list(map(str, outputs))),
                          MAREA_INPUT_PATHS=json.dumps([str(input_path)] if input_path else []),
                          STAGE_ID=action + '-attempt', NODE_ID=action + '-node')
        return t.Task(action), outputs

    def test_three_stages_real_sdk_and_fio(self):
        upload, up = self.task('upload');upload.upload()
        upload.upload()  # Conditional writes must safely recognize the same attempt's objects.
        self.assertEqual(self.puts, 4)
        dataset = t.read_json(up[0])
        self.assertEqual(len({f['sha256'] for f in dataset['files']}), 4)
        warmup, wp = self.task('warmup', up[0]);warmup.warmup()
        warm = t.read_json(wp[0]);self.assertFalse(warm['cache_hit_verified'])
        self.assertEqual(warm['read_bytes'], 4 * 4194304)
        read, rp = self.task('fio-read', wp[0]);read.read()
        report = t.read_json(read.folder / 'fio.json')
        opts = report['jobs'][0]['job options']
        for k, v in {'rw': 'read', 'bs': '1048576', 'numjobs': '2', 'iodepth': '1', 'ioengine': 'psync',
                     'direct': '1', 'runtime': '1', 'allow_file_create': '0', 'time_based': '1'}.items():
            self.assertEqual(opts[k], v, k)
        for paths in (up, wp, rp):
            evidence = t.read_json(paths[-1]);self.assertTrue(all(evidence['assertions'].values()))
            for f in evidence['files']:
                path = paths[-1].parent / f['path'];self.assertEqual(t.sha(path), f['sha256']);self.assertEqual(path.stat().st_size, f['size'])
        self.assertEqual(len(list(self.bench.rglob('*.bin'))), 4)
        output = os.environ.get('TOOLS_VALIDATION_EXPORT')
        if output:
            import shutil
            shutil.copytree(self.results, output, dirs_exist_ok=True)
        # A persisted checksum prevents reusing a changed upload list after warmup.
        changed = copy.deepcopy(dataset);changed['files'][0]['sha256'] = '0' * 64;t.atomic(up[0], changed)
        with self.assertRaises(t.TaskError):read.read()

    def test_missing_dataset_cannot_pass(self):
        upload, up = self.task('upload');upload.upload()
        next(self.bench.rglob('*.bin')).unlink()
        warmup, paths = self.task('warmup', up[0])
        with self.assertRaises(t.TaskError):warmup.warmup()
        self.assertFalse(paths[-1].exists())

    def test_sdk_errors_do_not_leak_credentials(self):
        self.task('upload')
        with patch.object(t, 'oss_bucket', side_effect=RuntimeError('isolated-test-secret')), \
             patch('sys.argv', ['tasks.py', 'upload']), contextlib.redirect_stdout(io.StringIO()) as out:
            self.assertEqual(t.main(), 1)
        self.assertNotIn('isolated-test-secret', out.getvalue())


if __name__ == '__main__':
    unittest.main()
