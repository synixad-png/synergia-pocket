"""Create a reproducible source ZIP; exclude generated Python caches."""
from pathlib import Path
from zipfile import ZipFile, ZIP_DEFLATED

project = Path(__file__).resolve().parents[1]
archive = project.parent / "SYNERGIA_API_INDEPENDENT_v0_2_ZRODLA.zip"
excluded = {"__pycache__", ".pytest_cache", ".venv", "build", "dist"}
files = sorted(
    path for path in project.rglob("*")
    if path.is_file()
    and not any(part in excluded or part.endswith(".egg-info") for part in path.parts)
    and path.suffix not in {".pyc", ".pyo"}
)
assert files
with ZipFile(archive, "w", compression=ZIP_DEFLATED, compresslevel=9) as z:
    for path in files:
        z.write(path, arcname=path.relative_to(project.parent).as_posix())
with ZipFile(archive) as z:
    corrupt = z.testzip()
    assert corrupt is None, f"CRC mismatch: {corrupt}"
    assert len(z.namelist()) == len(files)
print(f"PASS ZIP CRC, {len(files)} files: {archive}")
