"""Regresje ciągłości sesji; wszystkie konta i ciasteczka są fikcyjne."""
import json
import sys
import time
import types
from types import SimpleNamespace as N

from cryptography.fernet import Fernet
from fastapi.testclient import TestClient

import app as module
from session_store import RedisSessionStore, configured_store


class FakeRedis:
    def __init__(self):
        self.data = {}
        self.expiration = {}
    async def set(self, key, value, ex):
        self.data[key] = value
        self.expiration[key] = ex
    async def get(self, key):
        return self.data.get(key)
    async def delete(self, key):
        self.data.pop(key, None)
    async def aclose(self):
        pass


def install_mock_librus(monkeypatch):
    mock = types.ModuleType("librus_synergia")
    errors = types.ModuleType("librus_synergia.exceptions")
    class LibrusAuthError(Exception):
        pass
    errors.LibrusAuthError = LibrusAuthError
    errors.LibrusInvalidCredentialsError = LibrusAuthError
    errors.LibrusCaptchaRequiredError = LibrusAuthError
    errors.LibrusError = LibrusAuthError

    class LibrusSessionData:
        def __init__(self, cookies=None, logged_in_at=0.0):
            self.cookies = cookies or []
            self.logged_in_at = logged_in_at

    class FakeLibrus:
        def __init__(self, username, password, session_data=None, **_):
            self.username, self.password = username, password
            self._data = session_data or LibrusSessionData(
                cookies=[{"name": "oauth_token", "value": "fake-" + username,
                          "domain": "synergia.librus.pl"}],
                logged_in_at=time.time())
            self.closed = False
            self.login_calls = 0
        @property
        def session_data(self):
            return self._data
        async def login(self):
            self.login_calls += 1
        async def me(self):
            if self.password == "bad":
                raise LibrusAuthError()
            return N(display_name="Uczeń " + self.username)
        async def grades(self):
            return [N(value={"h": "5", "m": "3"}.get(self.username,"4"),
                      subject_id=1, category_id=1,add_date="2026-11-03",comments=[],semester=1)]
        async def subjects(self):
            return {1: "Test"}
        async def grade_categories(self):
            return {1: N(name="Sprawdzian", weight=1)}
        async def close(self):
            self.closed = True

    mock.Librus = FakeLibrus
    mock.LibrusSessionData = LibrusSessionData
    monkeypatch.setitem(sys.modules, "librus_synergia", mock)
    monkeypatch.setitem(sys.modules, "librus_synergia.exceptions", errors)
    return FakeLibrus


def test_fernet_server_storage_roundtrip_and_no_plaintext():
    import asyncio
    db = FakeRedis()
    store = RedisSessionStore(db, Fernet.generate_key())
    token = "random-browser-session-token"
    record = {"version": 1, "profiles": [{"username": "h", "name": "Hania",
              "session_data": {"cookies": [{"value": "fake-cookie-secret"}]}}]}
    asyncio.run(store.save(token, record, 4321))
    key = next(iter(db.data))
    ciphertext = db.data[key]
    assert token not in key
    assert b"fake-cookie-secret" not in ciphertext
    assert b"Hania" not in ciphertext
    assert db.expiration[key] == 4321
    assert asyncio.run(store.get(token)) == record
    db.data[key] = ciphertext[:-2] + b"xx"
    assert asyncio.run(store.get(token)) is None
    asyncio.run(store.delete(token))
    assert not db.data


def test_config_requires_tls_and_both_env_variables(monkeypatch):
    import pytest
    monkeypatch.delenv("SYNERGIA_REDIS_URL", raising=False)
    monkeypatch.delenv("SYNERGIA_SESSION_FERNET_KEY", raising=False)
    assert configured_store() is None
    monkeypatch.setenv("SYNERGIA_REDIS_URL", "redis://localhost:6379")
    with pytest.raises(RuntimeError):
        configured_store()
    monkeypatch.setenv("SYNERGIA_SESSION_FERNET_KEY", Fernet.generate_key().decode())
    with pytest.raises(RuntimeError):
        configured_store()
    monkeypatch.setenv("SYNERGIA_REDIS_URL", "redis://red-verified123:6379")
    monkeypatch.delenv("RENDER", raising=False)
    with pytest.raises(RuntimeError):
        configured_store()
    monkeypatch.setenv("RENDER", "true")
    store = configured_store()
    assert store is not None
    import asyncio
    asyncio.run(store.close())
    monkeypatch.setenv("SYNERGIA_REDIS_URL", "redis://example.com:6379")
    with pytest.raises(RuntimeError):
        configured_store()


def test_two_profiles_survive_backend_restart_without_saved_password(monkeypatch):
    install_mock_librus(monkeypatch)
    db = FakeRedis()
    store = RedisSessionStore(db, Fernet.generate_key())
    monkeypatch.setattr(module, "STORE", store)
    module.SESSIONS.clear()
    with TestClient(module.app) as c:
        r = c.post("/api/login", json={"login": "h", "password": "ok", "label": "Hania"})
        assert r.status_code == 200, r.text
        h = r.json()["active_profile_id"]
        assert "httponly" in r.headers["set-cookie"].lower()
        r = c.post("/api/profiles/add", json={"login": "m", "password": "ok", "label": "Martyna"})
        assert r.status_code == 200, r.text
        m = r.json()["active_profile_id"]
        assert h != m
        assert len(db.data) == 1
        ciphertext = next(iter(db.data.values()))
        assert b'"password"' not in ciphertext
        assert b'"username"' not in ciphertext
        assert c.get("/api/session").status_code == 200
        assert c.get("/api/section/grades").json()["subjects"][0]["grades"][0]["value"] == "3"

        # Symulacja restartu procesu: znika RAM, Redis i ciasteczko zostają.
        module.SESSIONS.clear()
        restored = c.get("/api/session")
        assert restored.status_code == 200, restored.text
        assert restored.json()["authenticated"] is True
        assert len(restored.json()["profiles"]) == 2
        assert "max-age=604800" in restored.headers["set-cookie"].lower()
        active = next(iter(module.SESSIONS.values())).active
        assert active.librus.password == ""  # hasło NIE jest zapisywane do Redis
        assert active.librus.login_calls == 1  # odświeżenie przy otwarciu
        assert c.get("/api/section/grades").json()["subjects"][0]["grades"][0]["value"] == "3"
        switched = c.post("/api/profiles/switch", json={"profile_id": h})
        assert switched.status_code == 200
        assert c.get("/api/section/grades").json()["subjects"][0]["grades"][0]["value"] == "5"
        assert c.post("/api/logout").status_code == 200
        assert not db.data
        module.SESSIONS.clear()
        assert c.get("/api/session").json() == {"authenticated": False}
    module.SESSIONS.clear()


def test_without_redis_restart_cannot_restore_session(monkeypatch):
    monkeypatch.setattr(module, "STORE", None)
    with TestClient(module.app) as c:
        assert c.post("/api/demo").status_code == 200
        assert c.get("/api/session").json()["authenticated"]
        module.SESSIONS.clear()
        assert c.get("/api/session").json() == {"authenticated": False}


def test_session_idle_rejected_and_persistent_record_deleted(monkeypatch):
    install_mock_librus(monkeypatch)
    db = FakeRedis()
    store = RedisSessionStore(db, Fernet.generate_key())
    monkeypatch.setattr(module, "STORE", store)
    with TestClient(module.app) as c:
        assert c.post("/api/login", json={"login": "h", "password": "ok"}).status_code == 200
        assert db.data
        token, session = next(iter(module.SESSIONS.items()))
        session.last_seen -= module.SESSION_IDLE + 2
        assert c.get("/api/session").json() == {"authenticated": False}
        assert token not in module.SESSIONS
        assert not db.data
    module.SESSIONS.clear()

def test_real_librus_session_import_export_without_network():
    """Weryfikuje kontrakt prawdziwej biblioteki 0.3.30, bez kontaktu z Librusem."""
    import asyncio
    from librus_synergia import Librus, LibrusSessionData
    async def check():
        initial = LibrusSessionData(
            cookies=[{"name": "oauth_token", "value": "totally-fake-test-cookie",
                      "domain": "synergia.librus.pl"}],
            logged_in_at=time.time())
        client = Librus("dummy-login", "", session_data=initial)
        try:
            assert client.session_data.logged_in_at == initial.logged_in_at
            assert client.client.is_session_valid()
            exported = client.session_data
            assert any(c["name"] == "oauth_token"
                       and c["value"] == "totally-fake-test-cookie" for c in exported.cookies)
        finally:
            await client.close()
    asyncio.run(check())
