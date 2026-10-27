#!/usr/bin/env python3
"""Real CLI/API scenario acceptance. Only writes inside a new output directory."""
import argparse
import json
import os
from pathlib import Path
import socket
import subprocess
import time
import urllib.request

p = argparse.ArgumentParser()
p.add_argument('--malta', required=True)
p.add_argument('--fio', required=True)
p.add_argument('--out', required=True)
a = p.parse_args()
root = Path(__file__).resolve().parents[1]
base = Path(a.out).resolve()
base.mkdir(parents=True, exist_ok=False)
target = base / 'data'
target.mkdir()
malta = str(Path(a.malta).resolve())

def cli(*args, server=False):
    command = [malta]
    if server:
        command += ['--server', url]
    proc = subprocess.run(command + [str(x) for x in args], capture_output=True, text=True, timeout=75)
    if proc.returncode:
        raise RuntimeError(proc.stdout + proc.stderr)
    return json.loads(proc.stdout)['data']

def plan(name, mode=None, duration=2):
    initial = cli('plan', '--scenario', root / 'benchmarks/scenarios' / (name + '.yaml'),
                  '--environment', root / 'environments/local.yaml', '--version', root / 'environments/version-example.yaml',
                  '--target', target, '--out', base / 'initial.json')
    spec = initial['spec']['scenario']
    spec['repeats'] = 1
    for stage in spec['stages']:
        stage['runtime_seconds'] = duration
        stage['warmup_seconds'] = 0
        if mode:
            stage['mode'] = mode
    filename = base / (name + '-' + str(mode) + '-' + str(duration) + '.json')
    filename.write_text(json.dumps(spec))
    destination = filename.with_suffix('.plan.json')
    cli('plan', '--scenario', filename, '--environment', root / 'environments/local.yaml',
        '--version', root / 'environments/version-example.yaml', '--target', target, '--out', destination)
    return destination

with socket.socket() as sock:
    sock.bind(('127.0.0.1', 0))
    port = sock.getsockname()[1]
url = 'http://127.0.0.1:' + str(port)
log = (base / 'server.log').open('w')
process = subprocess.Popen([malta, 'serve', '--listen', '127.0.0.1:' + str(port), '--data-dir', str(base / 'store'),
                            '--allow-target', str(target), '--fio', a.fio, '--run-timeout', '1m'], stdout=log, stderr=log)
summary = []
try:
    for _ in range(100):
        try:
            with urllib.request.urlopen(url + '/api/v1/capabilities', timeout=1):
                break
        except OSError:
            time.sleep(.1)
    else:
        raise RuntimeError('server did not become ready')
    for name, mode in [('mixed-data', 'sequential'), ('mixed-data', 'parallel'), ('weighted-data', None), ('weighted-metadata', None), ('pipeline', None)]:
        filename = plan(name, mode)
        request = name + '-' + str(mode)
        r = cli('run', 'start', '--plan', filename, '--request-id', request, '--source', 'agent', server=True)
        duplicate = cli('run', 'start', '--plan', filename, '--request-id', request, '--source', 'agent', server=True)
        assert duplicate['id'] == r['id']
        r = cli('run', 'wait', r['id'], '--timeout', '45s', server=True)
        assert r['status'] == 'succeeded' and r['attempt']['result_state'] == 'complete'
        destination = base / request
        cli('result', 'export', r['id'], '--out', destination, server=True)
        cli('--data-dir', base / 'imported', 'result', 'import', '--bundle', destination)
        cli('baseline', 'set', '--name', request, '--run', r['id'], server=True)
        comparison = cli('compare', '--baseline', r['id'], '--candidate', r['id'], server=True)
        assert comparison['comparable'] and all(x['baseline']['count'] == 1 for x in comparison['metrics'])
        counts = {t['point']['workload_id']: t.get('activity', {}).get('submitted') for t in r['trials']}
        if name.startswith('weighted'):
            weights = r['plan']['spec']['scenario']['stages'][0]['mix']['weights']
            total = sum(counts.values())
            assert total > 100
            for key, weight in weights.items():
                assert abs(counts[key] / total - weight / sum(weights.values())) < .02
        summary.append({'name': request, 'run': r['id'], 'metrics': len(comparison['metrics']), 'submitted': counts})
        print(json.dumps(summary[-1]), flush=True)
    filename = plan('weighted-data', duration=30)
    r = cli('run', 'start', '--plan', filename, '--request-id', 'cancel-weighted', server=True)
    for _ in range(200):
        commands = subprocess.check_output(['ps', '-eo', 'args'], text=True)
        if any('internal-native' in line and r['id'] in line for line in commands.splitlines()):
            break
        time.sleep(.05)
    else:
        raise AssertionError('native child did not start')
    time.sleep(.3)
    cli('run', 'cancel', r['id'], server=True)
    for _ in range(100):
        state = cli('run', 'get', r['id'], server=True)
        if state['status'] == 'cancelled' and state['attempt']['result_state'] == 'complete':
            break
        time.sleep(.05)
    else:
        raise AssertionError(state)
    commands = subprocess.check_output(['ps', '-eo', 'args'], text=True)
    assert not any('internal-native' in line and r['id'] in line for line in commands.splitlines())
    summary.append({'name': 'native cancellation', 'status': state['status'], 'no_child_remaining': True})
    (base / 'summary.json').write_text(json.dumps(summary, indent=2))
    print('PASS: evidence retained at ' + str(base), flush=True)
finally:
    process.terminate()
    try:
        process.wait(timeout=10)
    except subprocess.TimeoutExpired:
        process.kill()
        process.wait()
    log.close()
