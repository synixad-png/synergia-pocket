"""Smoke testy bez połączenia z kontem dziecka."""
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from fastapi.testclient import TestClient
from app import app, SESSIONS


def test_home_and_security_headers():
    with TestClient(app) as client:
        res = client.get('/')
        assert res.status_code == 200
        assert 'Synergia Pocket' in res.text
        assert 'script-src' in res.headers['Content-Security-Policy']
        assert res.headers['Cache-Control'] == 'no-store'
        assert client.get('/static/app.js').status_code == 200


def test_demo_login_and_sections():
    with TestClient(app) as client:
        assert client.get('/api/session').json()['authenticated'] is False
        res = client.post('/api/demo')
        assert res.status_code == 200
        assert res.json()['mode'] == 'demo'
        assert 'HttpOnly' in res.headers['Set-Cookie']
        assert 'SameSite=strict' in res.headers['Set-Cookie']
        for section in ('grades','timetable','attendance','agenda','homework','messages','notes','announcements'):
            response = client.get('/api/section/'+section)
            assert response.status_code == 200, (section,response.text)
            data = response.json()
            assert isinstance(data,dict)
            assert data
        assert len(client.get('/api/section/timetable?week=2026-11-03').json()['days'])==5
        assert client.get('/api/section/messages?mailbox=outbox').json()['items']
        msg = client.get('/api/message/demo-1').json()
        assert msg['id'] == 'demo-1'
        assert 'fikcyjna' in msg['body']
        assert client.post('/api/logout').status_code==200
        assert client.get('/api/section/grades').status_code==401
    SESSIONS.clear()


def test_invalid_inputs_and_cross_site_origin_rejected():
    with TestClient(app) as client:
        assert client.post('/api/demo', headers={'Origin':'https://stranger.test'}).status_code == 403
        client.post('/api/demo')
        assert client.get('/api/section/unknown').status_code == 404
        assert client.get('/api/section/timetable?week=2026-13-99').status_code == 400
        assert client.get('/api/section/timetable?week=2010-01-01').status_code == 400
        assert client.get('/api/section/messages?mailbox=abc').status_code == 400
        assert client.get('/api/message/nonexistent').status_code == 404
    SESSIONS.clear()
