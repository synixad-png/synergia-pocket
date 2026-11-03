import asyncio
from datetime import date
from fastapi.testclient import TestClient
from app import app, SESSIONS, live_section


def test_demo_session_and_sections():
    with TestClient(app) as client:
        assert client.get('/api/session').json()['authenticated'] is False
        response = client.post('/api/demo')
        assert response.status_code == 200
        assert response.json()['mode'] == 'demo'
        assert 'httponly' in response.headers['set-cookie'].lower()
        assert 'samesite=strict' in response.headers['set-cookie'].lower()
        assert client.get('/api/session').json()['authenticated'] is True
        for name in ('grades','timetable','attendance','agenda','homework','messages','notes','announcements'):
            response = client.get('/api/section/'+name)
            assert response.status_code == 200, (name, response.text)
            assert isinstance(response.json(), dict)
        assert len(client.get('/api/section/timetable?week=2026-11-02').json()['days']) == 5
        assert client.get('/api/section/messages?mailbox=outbox').status_code == 200
        assert client.get('/api/message/demo-1').json()['body']
        assert client.get('/api/message/missing').status_code == 404
        assert client.get('/api/section/no-such-section').status_code == 404
        assert client.get('/api/section/timetable?week=invalid').status_code == 400
        assert client.get('/api/section/messages?mailbox=invalid').status_code == 400
        assert client.post('/api/logout').json()['ok'] is True
        assert client.get('/api/section/grades').status_code == 401
    SESSIONS.clear()


def test_csrf_guard():
    with TestClient(app) as client:
        response = client.post('/api/demo', headers={'origin': 'https://evil.invalid'})
        assert response.status_code == 403


def test_local_html_assets_security():
    with TestClient(app) as client:
        index = client.get('/')
        assert index.status_code == 200
        assert 'Synergia Pocket' in index.text
        assert "script-src 'self'" in index.headers['content-security-policy']
        assert client.get('/static/app.js').status_code == 200
        assert client.get('/static/style.css').status_code == 200


class Dummy:
    async def grades(self):
        from types import SimpleNamespace as N
        return [N(value='5', subject_id=1, category_id=3, add_date='2026-11-02', comments=['Dobra praca'],
                  is_semester=False, is_final=False,is_semester_proposition=False,is_final_proposition=False, semester=1)]
    async def subjects(self):return {1:'Matematyka'}
    async def grade_categories(self):
        from types import SimpleNamespace as N
        return {3:N(name='Sprawdzian', weight=3)}
    async def timetable(self, week):
        from types import SimpleNamespace as N
        return {date(2026,11,2):[N(lesson_no=1,hour_from='08:00',hour_to='08:45',subject_id=1,teacher_id=9,classroom_id=2,is_canceled=False,is_substitution=False,original=None,substitution_note=None)]}
    async def classrooms(self):return {2:'102'}
    async def teachers(self):return {9:'A. Nowak'}
    async def homework(self):return []
    async def agenda(self):return []
    async def agenda_categories(self):return {}
    async def messages(self, mailbox, limit=30):return []
    async def unread_messages(self):return {'inbox':2}
    async def notes(self):return []
    async def note_categories(self):return {}
    async def announcements(self):return []
    async def attendances(self):return []
    async def attendance_types(self):return {}


def test_live_normalizers_without_credential():
    dummy=Dummy()
    grades=asyncio.run(live_section(dummy,'grades',None,'inbox'))
    assert grades['subjects'][0]['name']=='Matematyka'
    assert grades['subjects'][0]['grades'][0]['weight']==3
    schedule=asyncio.run(live_section(dummy,'timetable',date(2026,11,2),'inbox'))
    assert schedule['days'][0]['lessons'][0]['subject']=='Matematyka'
    assert schedule['days'][0]['lessons'][0]['room']=='102'
    for name in ('homework','agenda','messages','notes','announcements','attendance'):
        result=asyncio.run(live_section(dummy,name,None,'inbox'))
        assert isinstance(result,dict)


def test_mock_live_login_without_contacting_librus(monkeypatch):
    """Sprawdza całą ścieżkę sesji na sztucznym kliencie, nie na kontach uczniów."""
    import sys
    import types
    from types import SimpleNamespace
    from app import SESSIONS

    class FakeLibrus(Dummy):
        def __init__(self, login, password, **kwargs):
            self.credentials=(login,password)
            self.closed=False
        async def me(self):
            return SimpleNamespace(display_name='Uczeń Testowy')
        async def close(self):
            self.closed=True
    fake=types.ModuleType('librus_synergia')
    errors=types.ModuleType('librus_synergia.exceptions')
    errors.LibrusInvalidCredentialsError=type('LibrusInvalidCredentialsError',(Exception,),{})
    errors.LibrusCaptchaRequiredError=type('LibrusCaptchaRequiredError',(Exception,),{})
    errors.LibrusError=type('LibrusError',(Exception,),{})
    fake.Librus=FakeLibrus
    monkeypatch.setitem(sys.modules,'librus_synergia',fake)
    monkeypatch.setitem(sys.modules,'librus_synergia.exceptions',errors)

    with TestClient(app) as client:
        r=client.post('/api/login',json={'login':'user123','password':'dummy-secret'})
        assert r.status_code==200, r.text
        assert r.json()['name']=='Uczeń Testowy'
        user=next(iter(SESSIONS.values()))
        client_to_close = user.librus
        assert client_to_close.credentials==('user123','dummy-secret')
        assert client.get('/api/section/grades').json()['count']==1
        assert client.get('/api/section/messages').json()['unread']==2
        assert client.post('/api/logout').json()['ok']
        assert client_to_close.closed
        assert client.get('/api/session').json()['authenticated'] is False
