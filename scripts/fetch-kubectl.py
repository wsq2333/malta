#!/usr/bin/env python3
"""Download pinned kubectl binaries and verify committed official SHA-256 values."""
import hashlib,json,pathlib,urllib.request
root=pathlib.Path(__file__).resolve().parents[1]
config=json.loads((root/'build/docker/kubectl-checksums.json').read_text())
out=root/'build/output';out.mkdir(parents=True,exist_ok=True)
for arch,digest in config['sha256'].items():
    dest=out/('kubectl-linux-'+arch)
    if not dest.exists() or hashlib.sha256(dest.read_bytes()).hexdigest()!=digest:
        with urllib.request.urlopen('https://dl.k8s.io/release/'+config['version']+'/bin/linux/'+arch+'/kubectl',timeout=90) as response: data=response.read()
        if hashlib.sha256(data).hexdigest()!=digest:raise SystemExit('kubectl checksum mismatch: '+arch)
        temp=dest.with_suffix('.tmp');temp.write_bytes(data);temp.replace(dest)
    dest.chmod(0o755)
    print('verified kubectl',config['version'],arch,digest)
