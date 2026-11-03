#!/usr/bin/env python3
"""Prepare Linux/amd64 Python 3.11 dependencies for the Docker COPY recipe."""
import argparse
import hashlib
import importlib.metadata
import json
from pathlib import Path
import platform
import shutil
import subprocess
import sys

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--output', required=True, type=Path)
parser.add_argument('--ca-bundle', required=True, type=Path)
args = parser.parse_args()
if sys.version_info[:2] != (3, 11) or platform.system() != 'Linux' or platform.machine() not in ('x86_64', 'amd64'):
    parser.error('run with Linux/amd64 Python 3.11 to match the Runner base image')
lock = Path(__file__).with_name('requirements.lock')
args.output.mkdir(parents=True, exist_ok=False)
deps = args.output / 'deps'
subprocess.run([sys.executable, '-m', 'pip', 'install', '--no-cache-dir', '--target', str(deps), '-r', str(lock)], check=True)
shutil.copyfile(lock, args.output / 'requirements.lock')
shutil.copyfile(args.ca_bundle, args.output / 'ca-certificates.crt')
files = {str(f.relative_to(deps)): hashlib.sha256(f.read_bytes()).hexdigest() for f in sorted(deps.rglob('*'))
         if f.is_file() and '__pycache__' not in f.parts and f.suffix != '.pyc'}
versions = {d.metadata['Name']: d.version for d in importlib.metadata.distributions(path=[str(deps)])}
(args.output / 'dependencies.json').write_text(json.dumps({'packages': versions, 'installed_files_sha256': files}, indent=2) + '\n')
