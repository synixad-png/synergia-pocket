"""Testy pobierania załączników Wiadomości — syntetyczne konta, zero realnych loginów."""
import asyncio
import io
import zipfile
from types import SimpleNamespace as N
from urllib.parse import unquote

from fastapi.testclient import TestClient
import httpx
import pytest

from app import app, SESSIONS, new_session, make_profile
from message_attachments import MAX_ATTACHMENT_BYTES, _download_name, _file_response


class FakeMessages:
    def __init__(self, who):
        self.who = who
        self.reads = []
        self.downloads = []
        self.messages = {
            ("m1", "inbox"): N(id="m1", topic="Testowa wiadomość", sender_name="Test",
                send_date="2026-11-04", content="Treść testowa", no_reply=False,
                attachments=[N(id="a1", filename=f"Załącznik {who}")]),
            ("m2", "outbox"): N(id="m2", topic="Wysłane", sender_name="Użytkownik",
                send_date="2026-11-04", content="Wysłane testowe",
                attachments=[N(id="a2", filename=f"Rachunek {who}.pdf")]),
        }

    async def message(self, message_id, mailbox):
        self.reads.append((message_id, mailbox))
        return self.messages.get((message_id, mailbox))

    async def download_attachment(self, attachment_id, message_id):
        self.downloads.append((attachment_id, message_id))
        return N(filename="attachment-" + attachment_id,
                 content_type="application/octet-stream",
                 content=b"%PDF-1.7\nTest" + self.who.encode("ascii"))

    async def close(self):
        pass


def attach(client, *accounts):
    token, session = new_session("live", "Profil A", accounts[0])
    for account in accounts[1:]:
        other = make_profile("live", "Profil B", account)
        session.profiles[other.id] = other
    client.cookies.set("synergia_session", token)
    return token, session


def endpoint(message="m1", attachment="a1", profile_id=None, mailbox="inbox"):
    return f"/api/message-attachments/{message}/{attachment}/download?profile_id={profile_id}&mailbox={mailbox}"


def test_requires_login_and_real_account():
    with TestClient(app) as c:
        assert c.get(endpoint(profile_id="fake")).status_code == 401
        assert c.post("/api/demo").status_code == 200
        pid = c.get("/api/session").json()["active_profile_id"]
        assert c.get(endpoint(profile_id=pid)).status_code == 403
        assert c.get(endpoint(profile_id=pid, mailbox="other")).status_code == 400


def test_download_from_own_inbox_and_outbox_with_correct_filename():
    fake = FakeMessages("Hania")
    with TestClient(app) as c:
        token, session = attach(c, fake)
        pid = session.active_profile_id
        try:
            detail = c.get("/api/message/m1?mailbox=inbox")
            assert detail.status_code == 200
            d = detail.json()
            assert d["attachments"] == ["Załącznik Hania"]   # stary kontrakt nietknięty
            assert d["attachment_files"] == [{"id": "a1", "filename": "Załącznik Hania"}]
            r = c.get(endpoint(profile_id=pid))
            assert r.status_code == 200, r.text
            assert r.content.startswith(b"%PDF-1.7")
            assert r.headers["cache-control"] == "no-store"
            assert r.headers["x-content-type-options"] == "nosniff"
            assert unquote(r.headers["content-disposition"].split("filename*=UTF-8''")[1]) == "Załącznik Hania.pdf"
            assert fake.downloads == [("a1", "m1")]
            outbox = c.get(endpoint("m2", "a2", pid, "outbox"))
            assert outbox.status_code == 200
            assert unquote(outbox.headers["content-disposition"].split("filename*=UTF-8''")[1]) == "Rachunek Hania.pdf"
        finally:
            SESSIONS.pop(token, None)


def test_rejects_wrong_attachment_wrong_mailbox_and_wrong_message():
    fake = FakeMessages("Hania")
    with TestClient(app) as c:
        token, session = attach(c, fake)
        pid = session.active_profile_id
        try:
            assert c.get(endpoint("m1", "a2", pid)).status_code == 404
            assert c.get(endpoint("m1", "a1", pid, "outbox")).status_code == 404
            assert c.get(endpoint("m9", "a1", pid)).status_code == 404
            assert c.get(endpoint("m1", "a1", pid, "deleted")).status_code == 400
            assert c.get(endpoint("m1", "%20", pid)).status_code == 400
            assert c.get(endpoint("m1", "a1", "evil")).status_code == 409
            assert fake.downloads == []
        finally:
            SESSIONS.pop(token, None)


def test_two_profiles_never_share_attachments():
    a, b = FakeMessages("Hania"), FakeMessages("Martyna")
    with TestClient(app) as c:
        token, session = attach(c, a, b)
        p1 = session.active_profile_id
        p2 = next(k for k in session.profiles if k != p1)
        try:
            first = c.get(endpoint(profile_id=p1))
            assert first.status_code == 200
            assert first.content.endswith(b"Hania")
            assert c.post("/api/profiles/switch", json={"profile_id": p2}).status_code == 200
            assert c.get(endpoint(profile_id=p1)).status_code == 409
            second = c.get(endpoint(profile_id=p2))
            assert second.status_code == 200
            assert second.content.endswith(b"Martyna")
            assert a.downloads == [("a1", "m1")]
            assert b.downloads == [("a1", "m1")]
        finally:
            SESSIONS.pop(token, None)


def test_no_unrelated_functions_are_called():
    class Strict(FakeMessages):
        def __getattr__(self, attr):
            raise AssertionError("Załączniki nie mogą korzystać z innego modułu: " + attr)
    fake = Strict("Hania")
    with TestClient(app) as c:
        token, session = attach(c, fake)
        try:
            assert c.get(endpoint(profile_id=session.active_profile_id)).status_code == 200
        finally:
            SESSIONS.pop(token, None)


def test_filename_pdf_and_unknown_mime_and_unicode():
    file = N(content=b"%PDF-1.5\nmock", content_type="application/octet-stream",
             filename="attachment-123")
    name = _download_name(file, N(filename="Zgoda na wycieczkę"))
    assert name == "Zgoda na wycieczkę.pdf"
    response = _file_response(file, N(filename="Zgoda na wycieczkę"))
    assert unquote(response.headers["content-disposition"].split("filename*=UTF-8''")[1]) == name
    assert _download_name(N(filename="attachment-123", content=b"\0\1",
                            content_type="application/octet-stream"),
                          N(filename="Nieznany")) == "Nieznany.bin"
    assert _download_name(file, N(filename="Raport 2026.11")) == "Raport 2026.11.pdf"
    assert _download_name(file, N(filename="../secret.pdf")) == "secret.pdf"


def test_docx_type_from_zip_structure():
    data = io.BytesIO()
    with zipfile.ZipFile(data, "w") as archive:
        archive.writestr("word/document.xml", "<document/>")
    file = N(content=data.getvalue(), filename="attachment-991",
             content_type="application/octet-stream")
    assert _download_name(file, N(filename="Regulamin")) == "Regulamin.docx"


def test_reject_large_file_and_invalid_content():
    from fastapi import HTTPException
    with pytest.raises(HTTPException) as e:
        _file_response(N(content=b"0"*(MAX_ATTACHMENT_BYTES+1), filename="x.bin",
                         content_type="application/octet-stream"),N(filename="x.bin"))
    assert e.value.status_code == 413
    with pytest.raises(HTTPException) as e:
        _file_response(N(content="not bytes", filename="x", content_type="text/plain"),N(filename="x"))
    assert e.value.status_code == 502


def test_session_switch_during_slow_download_does_not_leak():
    async def scenario():
        started = asyncio.Event()
        release = asyncio.Event()
        class Slow(FakeMessages):
            async def download_attachment(self, attachment_id, message_id):
                started.set()
                await asyncio.wait_for(release.wait(), timeout=10)
                return await super().download_attachment(attachment_id,message_id)
        a = Slow("Hania")
        b = FakeMessages("Martyna")
        token, session = new_session("live", "Hania", a)
        other = make_profile("live", "Martyna", b)
        session.profiles[other.id] = other
        pid = session.active_profile_id
        try:
            async with httpx.AsyncClient(transport=httpx.ASGITransport(app=app),
                                         base_url="http://testserver",
                                         cookies={"synergia_session":token}) as client:
                download = asyncio.create_task(client.get(endpoint(profile_id=pid)))
                await asyncio.wait_for(started.wait(), timeout=10)
                switch = await client.post("/api/profiles/switch",json={"profile_id":other.id})
                assert switch.status_code == 200
                release.set()
                response = await download
                assert response.status_code == 409
        finally:
            release.set()
            SESSIONS.pop(token, None)
    asyncio.run(scenario())


def test_production_dockerfile_has_module():
    from pathlib import Path
    docker = (Path(__file__).resolve().parents[1] / "Dockerfile").read_text()
    assert "message_attachments.py" in docker
