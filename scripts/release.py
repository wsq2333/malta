#!/usr/bin/env python3
"""Build deterministic RC archives locally; does not publish or push anything."""
import argparse,gzip,hashlib,io,json,os,pathlib,re,shutil,subprocess,tarfile,tempfile
p=argparse.ArgumentParser(description=__doc__)
p.add_argument('--version',default='0.1.0-rc.1');p.add_argument('--revision')
p.add_argument('--output',default='release');p.add_argument('--with-kubectl',action='store_true')
a=p.parse_args();root=pathlib.Path(__file__).resolve().parents[1];os.chdir(root)
if not re.fullmatch(r'0\.1\.\d+(?:-[A-Za-z0-9.]+)?',a.version):p.error('version must be a 0.1.x release')
def command(*args,**kw):return subprocess.check_output(args,text=True,**kw).strip()
def run(*args,**kw):subprocess.run(args,check=True,**kw)
versions={'go':command('go','version'),'node':command('node','--version'),'pnpm':command('pnpm','--version')}
if 'go1.26.5 ' not in versions['go'] or versions['node']!='v24.19.0' or versions['pnpm']!='11.25.0':p.error('require Go 1.26.5, Node 24.19.0, pnpm 11.25.0')
revision=a.revision
if revision is None:
    revision=command('git','rev-parse','HEAD')
    if command('git','status','--porcelain'):revision+='-dirty'
if not re.fullmatch(r'[A-Za-z0-9._-]+',revision):p.error('invalid revision')
output=pathlib.Path(a.output).resolve();output.mkdir(parents=True,exist_ok=True)
build=root/'build/output';build.mkdir(parents=True,exist_ok=True)
run('pnpm','--dir','web','install','--frozen-lockfile')
run('pnpm','--dir','web','build');run('pnpm','--dir','web','test')
def tree_hash(path):return {str(f.relative_to(path)):hashlib.sha256(f.read_bytes()).hexdigest() for f in sorted(path.rglob('*')) if f.is_file()}
ui_hash=tree_hash(root/'web/dist');run('pnpm','--dir','web','build')
if ui_hash!=tree_hash(root/'web/dist'):raise SystemExit('frontend build is not reproducible')
# Collect dependency licenses, including transitive modules and frontend packages.
licenses=build/'licenses'
if licenses.exists():shutil.rmtree(licenses)
licenses.mkdir()
def copy_notices(source,dest):
    found=False
    for file in sorted(source.iterdir()):
        if file.is_file() and any(file.name.upper().startswith(v) for v in ('LICENSE','LICENCE','COPYING','NOTICE')):
            dest.mkdir(parents=True,exist_ok=True);shutil.copyfile(file,dest/file.name);found=True
    return found
raw=command('go','list','-m','-json','all');decoder=json.JSONDecoder();modules=[]
while raw.strip():v,n=decoder.raw_decode(raw.lstrip());raw=raw.lstrip()[n:];modules.append(v)
used_modules=set(command('go','list','-deps','-f','{{if .Module}}{{.Module.Path}}{{end}}','./cmd/malta').splitlines())
for module in modules:
    if module.get('Main') or module['Path'] not in used_modules:continue
    if not module.get('Dir') or not copy_notices(pathlib.Path(module['Dir']),licenses/'go'/(module['Path'].replace('/','_')+'@'+module['Version'])):raise SystemExit('Missing dependency notice: '+module['Path'])
copy_notices(pathlib.Path(command('go','env','GOROOT')),licenses/'go-runtime')
runtime=['react','react-dom','scheduler','echarts','zrender','tslib','lucide-react']
for name in runtime:
    paths=list((root/'web/node_modules/.pnpm').glob(name+'@*/node_modules/'+name))
    if not paths:raise SystemExit('Missing frontend package: '+name)
    for path in paths:
        version=json.loads((path/'package.json').read_text())['version']
        if not copy_notices(path,licenses/'frontend'/(name+'@'+version)):raise SystemExit('Missing frontend license: '+name)
if a.with_kubectl:run('python3','scripts/fetch-kubectl.py')
targets=[('linux','amd64'),('linux','arm64'),('darwin','arm64')]
manifest={'version':a.version,'revision':revision,'schema_version':'1.0','toolchains':versions,'ui_sha256':ui_hash,'targets':{},'reproducibility':'two clean binary outputs and two frontend builds compared; container OS packages are not byte-reproducible'}
for system,arch in targets:
    env=dict(os.environ,CGO_ENABLED='0',GOOS=system,GOARCH=arch,GOTOOLCHAIN='local')
    binary=build/('malta-'+system+'-'+arch)
    flags='-s -w -buildid= -X github.com/wsq2333/malta/internal/domain.Version='+a.version+' -X github.com/wsq2333/malta/internal/domain.Revision='+revision
    run('go','build','-mod=readonly','-trimpath','-buildvcs=false','-ldflags',flags,'-o',str(binary),'./cmd/malta',env=env)
    second=binary.with_suffix('.verify')
    try:
        run('go','build','-mod=readonly','-trimpath','-buildvcs=false','-ldflags',flags,'-o',str(second),'./cmd/malta',env=env)
        if binary.read_bytes()!=second.read_bytes():raise SystemExit('binary is not reproducible: '+binary.name)
    finally:second.unlink(missing_ok=True)
    manifest['targets'][system+'/'+arch]=hashlib.sha256(binary.read_bytes()).hexdigest()
(build/'build-manifest.json').write_text(json.dumps(manifest,indent=2)+'\n')
# Deterministic archives: sorted paths, stable modes/owners/timestamps and gzip header.
common=['README.md','LICENSE','NOTICE','THIRD_PARTY_NOTICES.md','docs','schemas','benchmarks','environments','skills','deploy','api','scripts/smoke.sh','scripts/render-kubernetes.py','tests/agent/acceptance.py']
for system,arch in targets:
    with tempfile.TemporaryDirectory(prefix='malta-package-') as tmp:
        folder=pathlib.Path(tmp)/('malta-'+a.version+'-'+system+'-'+arch);folder.mkdir()
        for item in common:
            src=root/item;dest=folder/item;dest.parent.mkdir(parents=True,exist_ok=True)
            if src.is_dir():shutil.copytree(src,dest,ignore=shutil.ignore_patterns('__pycache__','*.pyc'))
            else:shutil.copyfile(src,dest)
        shutil.copyfile(build/('malta-'+system+'-'+arch),folder/'malta');(folder/'malta').chmod(0o755)
        shutil.copytree(licenses,folder/'licenses');shutil.copyfile(build/'build-manifest.json',folder/'build-manifest.json')
        archive=output/(folder.name+'.tar.gz')
        with archive.open('wb') as rawout,gzip.GzipFile(filename='',mode='wb',fileobj=rawout,mtime=0) as gz,tarfile.open(fileobj=gz,mode='w') as tar:
            for file in [folder,*sorted(folder.rglob('*'))]:
                info=tar.gettarinfo(str(file),str(file.relative_to(folder.parent)));info.uid=info.gid=0;info.uname=info.gname='';info.mtime=0;info.mode=0o755 if file.is_dir() or file.name=='malta' else 0o644
                if file.is_file():
                    with file.open('rb') as data:tar.addfile(info,data)
                else:tar.addfile(info)
checks=[]
for file in sorted(output.glob('malta-'+a.version+'-*.tar.gz')):checks.append(hashlib.sha256(file.read_bytes()).hexdigest()+'  '+file.name)
(output/'SHA256SUMS').write_text('\n'.join(checks)+'\n');shutil.copyfile(build/'build-manifest.json',output/'build-manifest.json')
print('Release candidates written to',output)
