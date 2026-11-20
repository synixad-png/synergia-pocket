import base64,gzip,hashlib,json,subprocess,sys
from pathlib import Path
root=Path(sys.argv[1]);ci=Path(__file__).parent
hashes=json.loads((ci/'p86_hashes.json').read_text())
def digest(p): return hashlib.sha256(p.read_bytes()).hexdigest()
for path,h in hashes.items():
 p=root/path
 assert (digest(p) if p.exists() else None)==h['before'], 'Wrong base: '+path
patch=gzip.decompress(base64.b64decode((ci/'p86.patch.gz.b64').read_text()))
subprocess.run(['patch','--batch','--fuzz=0','-p1'],cwd=root,input=patch,check=True)
for path,h in hashes.items(): assert digest(root/path)==h['after'], 'Wrong result: '+path
print('P86 verified source overlay PASS')
