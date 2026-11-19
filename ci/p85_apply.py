import base64,gzip,hashlib,json,subprocess,sys,zipfile
from pathlib import Path
root=Path(sys.argv[1])
ci=Path(__file__).parent
hashes=json.loads((ci/'p85_hashes.json').read_text())
def digest(p):return hashlib.sha256(p.read_bytes()).hexdigest()
for path, expected in hashes.items():
 p=root/path
 if expected['before'] is None:
  assert not p.exists(), 'unexpected existing file: '+path
 else: assert digest(p)==expected['before'], 'unexpected input source: '+path
patch=gzip.decompress(base64.b64decode((ci/'p85_session.patch.gz.b64').read_text()))
subprocess.run(['patch','--batch','--fuzz=0','-p1'],input=patch,cwd=root,check=True)
for path, expected in hashes.items():
 assert digest(root/path)==expected['after'], 'wrong patched source: '+path
assets=ci/'p85_assets.zip'
assert digest(assets)=='d9009c8073a634f669c7534ea7b72c65ec703282d36815621d7766f8569ea0c2'
with zipfile.ZipFile(assets) as z:
 assert all(p.startswith('Sources/Assets.xcassets/') and '..' not in Path(p).parts for p in z.namelist())
 z.extractall(root)
(root/'Tests/DirectSessionCookieSnapshotTests.swift').write_text((ci/'p85_existing_snapshot_tests.swift').read_text())
print('P85 verified session source and app icon: PASS')
