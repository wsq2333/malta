#!/usr/bin/env python3
"""Package prepared dependencies and scripts over a verified Runner OCI layout. Never pushes."""
import argparse
import datetime
import gzip
import hashlib
import io
import json
from pathlib import Path
import shutil
import tarfile

p = argparse.ArgumentParser(description=__doc__)
p.add_argument('--base', required=True, type=Path, help='existing Runner directory with oci and image-metadata.json')
p.add_argument('--prepared-root', required=True, type=Path)
p.add_argument('--source', required=True, type=Path, help='repository source root')
p.add_argument('--output', required=True, type=Path)
p.add_argument('--tag', required=True)
p.add_argument('--revision', required=True)
a = p.parse_args()
def encode(v):return json.dumps(v, sort_keys=True, separators=(',', ':')).encode()
def digest(b):return 'sha256:' + hashlib.sha256(b).hexdigest()
def blob(layout, d):
 b = (layout / 'blobs/sha256' / d['digest'].split(':')[1]).read_bytes()
 assert digest(b) == d['digest'] and len(b) == d['size'];return b

base = a.base / 'oci';index = json.loads((base / 'index.json').read_text())
descriptor = index['manifests'][0];meta = json.loads((a.base / 'image-metadata.json').read_text())
assert descriptor['digest'] == meta['oci_manifest_digest'] and meta['role'] == 'runner'
manifest = json.loads(blob(base, descriptor));config = json.loads(blob(base, manifest['config']))
assert config['architecture'] == 'amd64' and config['os'] == 'linux' and config['config']['User'] == '10001:10001'
a.output.mkdir(parents=True, exist_ok=False)
layer = a.output / 'tools-layer.tar'
entries = []
for src, target in [(a.prepared_root / 'opt/marea-test-tools/deps', 'opt/marea-test-tools/deps'),
                    (a.prepared_root / 'opt/marea-test-tools/requirements.lock', 'opt/marea-test-tools/requirements.lock'),
                    (a.prepared_root / 'opt/marea-test-tools/dependencies.json', 'opt/marea-test-tools/dependencies.json'),
                    (a.prepared_root / 'etc/ssl/certs/ca-certificates.crt', 'etc/ssl/certs/ca-certificates.crt'),
                    (a.source / 'tools/marea-test-tools/tasks.py', 'opt/marea-test-tools/tasks.py'),
                    (a.source / 'tools/marea-test-tools/marea-test-tools', 'usr/local/bin/marea-test-tools')]:
 for f in [src, *sorted(src.rglob('*'))] if src.is_dir() else [src]:
  if '__pycache__' in f.parts or f.suffix == '.pyc':continue
  entries.append((f, target + ('/' + f.relative_to(src).as_posix() if f != src else '')))
with tarfile.open(layer, 'w') as tar:
 for src, name in entries:
  assert not src.is_symlink()
  info = tar.gettarinfo(str(src), name);info.uid = info.gid = info.mtime = 0;info.uname = info.gname = ''
  info.mode = 0o755 if src.is_dir() or name == 'usr/local/bin/marea-test-tools' else 0o644
  if src.is_file():
   with src.open('rb') as f:tar.addfile(info, f)
  else:tar.addfile(info)
raw = layer.read_bytes();compressed = gzip.compress(raw, mtime=0)
config['rootfs']['diff_ids'].append(digest(raw))
created = datetime.datetime.now(datetime.timezone.utc).isoformat()
config['created'] = created;config.setdefault('history', []).append({'created': created, 'created_by': 'COPY pinned SDK, CA bundle and marea task scripts'})
c = config['config'];c['Entrypoint'] = ['/usr/local/bin/marea-test-tools'];c['Cmd'] = ['version']
c['Env'] = [e for e in c.get('Env', []) if not e.startswith(('PYTHONPATH=', 'PYTHONDONTWRITEBYTECODE='))] + ['PYTHONPATH=/opt/marea-test-tools/deps', 'PYTHONDONTWRITEBYTECODE=1']
c.setdefault('Labels', {}).update({'org.opencontainers.image.title': 'marea-test-tools',
                                  'org.opencontainers.image.version': '0.1.0', 'org.opencontainers.image.revision': a.revision,
                                  'org.opencontainers.image.base.digest': descriptor['digest']})
cb = encode(config)
new = {'schemaVersion': 2, 'mediaType': 'application/vnd.oci.image.manifest.v1+json',
       'config': {'mediaType': 'application/vnd.oci.image.config.v1+json', 'digest': digest(cb), 'size': len(cb)},
       'layers': manifest['layers'] + [{'mediaType': 'application/vnd.oci.image.layer.v1.tar+gzip', 'digest': digest(compressed), 'size': len(compressed)}]}
mb = encode(new);oci = a.output / 'oci';blobs = oci / 'blobs/sha256';blobs.mkdir(parents=True)
for d in manifest['layers']:
 blob(base, d);shutil.copyfile(base / 'blobs/sha256' / d['digest'].split(':')[1], blobs / d['digest'].split(':')[1])
for b in (cb, mb, compressed):(blobs / digest(b).split(':')[1]).write_bytes(b)
(oci / 'oci-layout').write_text('{"imageLayoutVersion":"1.0.0"}')
(oci / 'index.json').write_bytes(encode({'schemaVersion': 2, 'manifests': [{'mediaType': new['mediaType'], 'digest': digest(mb), 'size': len(mb), 'annotations': {'org.opencontainers.image.ref.name': a.tag}, 'platform': {'os': 'linux', 'architecture': 'amd64'}}]}))
archive = a.output / 'marea-test-tools.tar'
with tarfile.open(archive, 'w') as tar:
 names = []
 for i, d in enumerate(new['layers']):
  b = blob(oci, d);r = gzip.decompress(b) if b[:2] == b'\x1f\x8b' else b
  assert digest(r) == config['rootfs']['diff_ids'][i]
  name = config['rootfs']['diff_ids'][i].split(':')[1] + '/layer.tar';names.append(name)
  info = tarfile.TarInfo(name);info.size = len(r);tar.addfile(info, io.BytesIO(r))
 cfgname = digest(cb).split(':')[1] + '.json'
 for name, b in [(cfgname, cb), ('manifest.json', encode([{'Config': cfgname, 'RepoTags': [a.tag], 'Layers': names}]))]:
  info = tarfile.TarInfo(name);info.size = len(b);tar.addfile(info, io.BytesIO(b))
report = {'image': a.tag, 'revision': a.revision, 'platform': 'linux/amd64', 'base_digest': descriptor['digest'],
          'oci_manifest_digest': digest(mb), 'archive': str(archive), 'archive_sha256': hashlib.sha256(archive.read_bytes()).hexdigest(),
          'tasks_sha256': hashlib.sha256((a.source / 'tools/marea-test-tools/tasks.py').read_bytes()).hexdigest(),
          'build_mode': 'OCI application layer; development host without Docker daemon', 'pushed': False}
(a.output / 'image-metadata.json').write_text(json.dumps(report, indent=2) + '\n')
print(json.dumps(report))
