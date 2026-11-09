"""P84 native grades UI: apply verified no-IPA source fix to a P83 tree.
Only Sources/DirectPocketModuleScreen.swift is modified.
Usage: python3 ci/p84_apply.py [synergia-direct-ios]
"""
import base64
import hashlib
from pathlib import Path
import subprocess
import sys
import zlib

root = Path(sys.argv[1]) if len(sys.argv) > 1 else Path("synergia-direct-ios")
page = root / "Sources/DirectPocketModuleScreen.swift"
def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()

before = "09f889dd21b6a92cdd1e0788fa9865f0728fbb1f049c5645fa680a7d2ba6b7b7"
after = "2633872581c5c7bf12e9d8eca131aeffc4c9b8dabebb2ce7dae962d2ba84d7f6"
actual = digest(page)
if actual != before:
    raise SystemExit("P84 refuses unexpected source version: " + actual)

payload = Path(__file__).with_name("p84_grade_color.patch.zlib.b64").read_text().strip()
patch = zlib.decompress(base64.b64decode(payload))
result = subprocess.run(
    ["patch", "--batch", "--fuzz=0", "-p1"],
    input=patch, cwd=root, capture_output=True, check=True,
)
if digest(page) != after:
    raise SystemExit("P84 patched result does not match verified SHA256")
print("P84 read grade tiles use neutral background: PASS")
