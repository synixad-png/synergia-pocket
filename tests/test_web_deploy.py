from fastapi.testclient import TestClient
import app as module
from fastapi import HTTPException
from pathlib import Path


def test_https_reverse_proxy_origin_and_cookie(monkeypatch):
    monkeypatch.setattr(module, 'FORCE_HTTPS', True)
    with TestClient(module.app, base_url="https://testserver") as client:
        assert client.get('/api/health').json()['status'] == 'ok'
        rejected = client.post('/api/demo', headers={'origin':'https://evil.example'})
        assert rejected.status_code == 403
        response = client.post('/api/demo', headers={'origin':'https://testserver'})
        assert response.status_code == 200
        assert 'secure' in response.headers['set-cookie'].lower()
        assert 'httponly' in response.headers['set-cookie'].lower()
        assert client.get('/api/section/grades').status_code == 200
        assert client.post('/api/logout', headers={'origin':'https://testserver'}).status_code == 200
    module.SESSIONS.clear()


def test_login_throttle_per_identifier(monkeypatch):
    module.LOGIN_TRIES.clear()
    for _ in range(module.LOGIN_MAX_ATTEMPTS):
        module.throttle_login('dummyaccount')
    try:
        module.throttle_login('dummyaccount')
    except HTTPException as ex:
        assert ex.status_code == 429
    else:
        assert False, 'Oczekiwano HTTP 429'
    module.throttle_login('different-dummy-account')
    module.LOGIN_TRIES.clear()


def test_offline_standalone_contract():
    html = (Path(__file__).resolve().parents[1] / 'SYNERGIA_POCKET_WEB_v3_DEMO.html').read_text()
    assert 'let offlineLoggedIn=true;' in html
    assert 'class="demo-disabled-form"' in html
    for name in ('grades','timetable','attendance','homework','messages'):
        assert 'data-page="' + name + '"' in html
