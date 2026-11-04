"""Regresje Dokumentów szkoły bez rzeczywistych kont Librusa."""
import io
import zipfile
from types import SimpleNamespace as N
from urllib.parse import unquote

import pytest
from fastapi.testclient import TestClient

from app import app, SESSIONS, make_profile, new_session
from school_files import (
    MAX_DOWNLOAD_BYTES,
    _attachment_name,
    _attachment_response,
    _clean_filename,
    _safe_source_path,
)


class FakeFiles:
    def __init__(self, identity="konto1"):
        self.identity = identity
        self.downloads = []
        self.documents = [N(id=identity, name=f"Plan szkoły {identity}",
                            added="2026-11-04", download_path=f"/pliki_szkoly/pobierz/{identity}")]
        self.payload = N(filename="school-file-" + identity,
                         content_type="application/octet-stream", content=b"%PDF-1.4\nmock")
    async def school_files(self):
        return self.documents
    async def download_school_file(self, path):
        self.downloads.append(path)
        return self.payload
    async def close(self):
        pass


def attach_profiles(client, profiles):
    token, session = new_session("live", "Pierwsze dziecko", profiles[0])
    for index, lib in enumerate(profiles[1:]):
        profile = make_profile("live", f"Dziecko {index+2}", lib)
        session.profiles[profile.id] = profile
    client.cookies.set("synergia_session", token)
    return token, session


def test_demo_school_files_and_download_are_clearly_fake():
    with TestClient(app) as client:
        assert client.get("/api/school-files").status_code == 401
        assert client.get("/api/school-files/demo-szkolny-dokument/download").status_code == 401
        client.post("/api/demo")
        data = client.get("/api/school-files")
        assert data.status_code == 200
        assert data.json()["demo"] is True
        item = data.json()["items"][0]
        assert item["can_download"] is True
        result = client.get(f"/api/school-files/{item['id']}/download")
        assert result.status_code == 200
        assert result.headers["content-disposition"].endswith(
            "Przykladowy_dokument_szkolny.txt"
        ) or "Przykladowy_dokument_szkolny.txt" in result.headers["content-disposition"]
        assert b"PLIK DEMONSTRACYJNY" in result.content
        assert "attachment" in result.headers["content-disposition"]
        assert client.get("/api/school-files/nie-ma/download").status_code == 404


def test_school_files_isolation_for_two_accounts():
    first, second = FakeFiles("konto1"), FakeFiles("konto2")
    with TestClient(app) as client:
        token, session = attach_profiles(client, [first, second])
        try:
            data = client.get("/api/school-files").json()
            assert len(data["items"]) == 1
            assert data["items"][0]["id"] == "konto1"
            assert "konto2" not in str(data)
            r = client.get("/api/school-files/konto1/download")
            assert r.status_code == 200
            assert "Plan%20szko" in r.headers["content-disposition"]
            assert first.downloads == ["/pliki_szkoly/pobierz/konto1"]
            assert second.downloads == []
            other_id = next(x for x in session.profiles if x != session.active_profile_id)
            assert client.post("/api/profiles/switch", json={"profile_id": other_id}).status_code == 200
            assert client.get("/api/school-files/konto1/download").status_code == 404
            assert first.downloads == ["/pliki_szkoly/pobierz/konto1"]
            assert client.get("/api/school-files").json()["items"][0]["id"] == "konto2"
            assert client.get("/api/school-files/konto2/download").status_code == 200
            assert second.downloads == ["/pliki_szkoly/pobierz/konto2"]
        finally:
            SESSIONS.pop(token, None)


@pytest.mark.parametrize("path,allowed", [
    ("/pliki_szkoly/pobierz/22", True),
    ("https://synergia.librus.pl/pliki_szkoly/pobierz/22", True),
    ("https://evil.example/pliki_szkoly/pobierz/22", False),
    ("http://synergia.librus.pl/pliki_szkoly/pobierz/22", False),
    ("/pliki_szkoly/pobierz/22?redirect=evil", False),
    ("/pliki_szkoly/pobierz/../secrets", False),
    ("//evil.example/pliki_szkoly/pobierz/22", False),
    ("javascript:alert(1)", False),
    ("/api/login", False),
])
def test_source_path_allowlist(path, allowed):
    assert _safe_source_path(path) is allowed


def test_bad_school_document_path_is_not_downloaded():
    fake = FakeFiles()
    fake.documents[0].download_path = "https://evil.example/other"
    with TestClient(app) as client:
        token, _ = attach_profiles(client, [fake])
        try:
            assert client.get("/api/school-files").json()["items"][0]["can_download"] is False
            assert client.get("/api/school-files/konto1/download").status_code == 422
            assert fake.downloads == []
        finally:
            SESSIONS.pop(token, None)


def test_does_not_make_up_documents_when_school_has_none():
    fake = FakeFiles()
    fake.documents = []
    with TestClient(app) as client:
        token, _ = attach_profiles(client, [fake])
        try:
            r = client.get("/api/school-files")
            assert r.status_code == 200
            assert r.json() == {"items": [], "demo": False, "truncated": False}
        finally:
            SESSIONS.pop(token, None)


def test_does_not_fetch_unrelated_modules_during_document_request():
    class Strict(FakeFiles):
        def __getattr__(self, name):
            raise AssertionError(f"moduł Dokumentów szkoły próbował użyć {name}")
    with TestClient(app) as client:
        token, _ = attach_profiles(client, [Strict()])
        try:
            assert client.get("/api/school-files").status_code == 200
            assert client.get("/api/school-files/konto1/download").status_code == 200
        finally:
            SESSIONS.pop(token, None)


def test_file_names_extensions_polish_chars_and_no_html():
    file = N(filename="school-file-22", content_type="application/octet-stream", content=b"%PDF-1.5\nmock")
    name = _attachment_name(file, {"name": "Załącznik szkolny"})
    assert name == "Załącznik szkolny.pdf"
    resp = _attachment_response(file, {"name": "Załącznik szkolny"})
    assert "filename*=UTF-8''" in resp.headers["content-disposition"]
    assert unquote(resp.headers["content-disposition"].split("filename*=UTF-8''", 1)[1]) == name
    assert resp.headers["x-content-type-options"] == "nosniff"
    assert resp.headers["cache-control"] == "no-store"
    assert _clean_filename("../bad\r\nx.pdf") == "bad__x.pdf"
    assert _attachment_name(N(filename="Raport szkolny.doc", content_type="application/octet-stream",
                              content=b"%PDF-1.4"), {"name": "A"}) == "Raport szkolny.doc"
    assert _attachment_name(N(filename="school-file-22", content_type="application/pdf", content=b"%PDF-1.4"),
                              {"name": "Plan 2026.11"}) == "Plan 2026.11.pdf"
    assert _attachment_name(N(filename="school-file-22", content_type="blob/unknown",
                              content=b"\x00\x01"), {"name": "Nieznany dokument"}) == "Nieznany dokument.bin"


@pytest.mark.parametrize("magic,ext", [
    (b"\x89PNG\r\n\x1a\npng", ".png"),
    (b"\xff\xd8\xffjpeg", ".jpg"),
    (b"GIF89a gif", ".gif"),
    (b"%PDF-1.4 pdf", ".pdf"),
])
def test_magic_byte_extensions(magic, ext):
    assert _attachment_name(N(filename="school-file-22", content_type="application/octet-stream",
                              content=magic), {"name": "Dokument"}) == "Dokument" + ext


def test_ooxml_extension():
    contents = io.BytesIO()
    with zipfile.ZipFile(contents, "w") as zf:
        zf.writestr("word/document.xml", "<document/>")
    assert _attachment_name(
        N(filename="school-file-22", content_type="application/octet-stream", content=contents.getvalue()),
        {"name": "Sprawozdanie"}
    ) == "Sprawozdanie.docx"


def test_limit_does_not_return_large_file():
    data = b"a" * (MAX_DOWNLOAD_BYTES + 1)
    from fastapi import HTTPException
    with pytest.raises(HTTPException) as exc:
        _attachment_response(N(filename="a.bin", content_type="application/octet-stream", content=data),
                             {"name": "a.bin"})
    assert exc.value.status_code == 413


def test_unknown_file_and_invalid_id_are_rejected():
    fake = FakeFiles()
    with TestClient(app) as client:
        token, _ = attach_profiles(client, [fake])
        try:
            assert client.get("/api/school-files/unknown/download").status_code == 404
            assert client.get("/api/school-files/%C5%BC%C3%B3%C5%82ty/download").status_code == 400
            assert fake.downloads == []
        finally:
            SESSIONS.pop(token, None)


def test_production_dockerfile_copies_new_module():
    from pathlib import Path
    docker = (Path(__file__).resolve().parents[1] / 'Dockerfile').read_text()
    assert any('school_files.py' in line and line.startswith('COPY ') for line in docker.splitlines())
