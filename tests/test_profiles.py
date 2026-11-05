"""Testy dwóch osobnych kont Synergia bez prawdziwych haseł i bez sieci."""
import asyncio
import sys
import types

import pytest
from fastapi.testclient import TestClient

from app import app, SESSIONS


@pytest.fixture
def fake_librus(monkeypatch):
    class AuthError(Exception): pass
    class CaptchaError(Exception): pass
    class LibrusError(Exception): pass
    class FakeLibrus:
        connections = []
        def __init__(self, login, password, **kwargs):
            self.login, self.password = login, password
            self.closed = False
            self.__class__.connections.append(self)
        async def me(self):
            if self.password != 'ok': raise AuthError('invalid')
            return types.SimpleNamespace(display_name=f'Dziecko {self.login}')
        async def close(self): self.closed = True
        async def grades(self):
            return [types.SimpleNamespace(value={'ania':'5','ola':'2','zuzia':'4'}.get(self.login,'3'),
                    subject_id=1,category_id=1,add_date='2026-11-03',comments=[], semester=1)]
        async def subjects(self): return {1: f'Przedmiot {self.login}'}
        async def grade_categories(self): return {1: types.SimpleNamespace(name='Kartkówka',weight=1)}
        async def messages(self, mailbox, limit=30):
            return [types.SimpleNamespace(id=f'msg-{self.login}',sender_name='Sekretariat',
                    receiver_name='Uczennica',topic=f'Wiadomość {self.login}',content='Treść',
                    send_date='2026-11-03',read_date=None)]
        async def unread_messages(self):return {'inbox':1}
        async def message(self,message_id,mailbox):
            if message_id!=f'msg-{self.login}':return None
            return types.SimpleNamespace(id=message_id,topic=f'Skrzynka {self.login}',sender_name='Nauczyciel',
                                         send_date='2026-11-03',content=f'Sekret dla {self.login}',attachments=[])
    fake = types.ModuleType('librus_synergia')
    errors = types.ModuleType('librus_synergia.exceptions')
    errors.LibrusInvalidCredentialsError=AuthError
    errors.LibrusCaptchaRequiredError=CaptchaError
    errors.LibrusError=LibrusError
    fake.Librus=FakeLibrus
    monkeypatch.setitem(sys.modules,'librus_synergia',fake)
    monkeypatch.setitem(sys.modules,'librus_synergia.exceptions',errors)
    yield FakeLibrus
    SESSIONS.clear()


def post(client,path,body): return client.post(path,json=body)


def test_profile_flow_isolation_and_cleanup(fake_librus):
    with TestClient(app) as c:
        one=post(c,'/api/login',{'login':'ania','password':'ok','label':'Hania'})
        assert one.status_code==200,one.text
        a=one.json()['active_profile_id']
        assert one.json()['name']=='Hania'
        assert len(one.json()['profiles'])==1
        assert 'password' not in one.json()
        assert 'login' not in one.json()
        assert c.get('/api/section/grades').json()['subjects'][0]['grades'][0]['value']=='5'
        assert c.get('/api/message/msg-ania').json()['body']=='Sekret dla ania'
        two=post(c,'/api/profiles/add',{'login':'ola','password':'ok','label':'Martyna'})
        assert two.status_code==200,two.text
        b=two.json()['active_profile_id']; assert b!=a
        assert two.json()['name']=='Martyna'
        assert len(two.json()['profiles'])==2
        # Po dodaniu drugiego profilu sesja jest odnawiana, ale jej token się nie zmienia.
        assert two.headers.get('set-cookie')
        assert 'max-age=31536000' in two.headers['set-cookie'].lower()
        assert one.cookies.get('synergia_session') == two.cookies.get('synergia_session')
        assert c.get('/api/section/grades').json()['subjects'][0]['grades'][0]['value']=='2'
        assert c.get('/api/message/msg-ania').status_code==404
        assert c.get('/api/message/msg-ola').json()['body']=='Sekret dla ola'
        assert c.get('/api/section/messages').json()['items'][0]['id']=='msg-ola'
        assert post(c,'/api/profiles/switch',{'profile_id':a}).json()['name']=='Hania'
        assert c.get('/api/section/messages').json()['items'][0]['id']=='msg-ania'
        assert c.get('/api/section/grades').json()['subjects'][0]['grades'][0]['value']=='5'
        assert post(c,'/api/profiles/switch',{'profile_id':b}).status_code==200
        assert c.get('/api/section/grades').json()['subjects'][0]['grades'][0]['value']=='2'
        assert c.get('/api/session').json()['active_profile_id']==b
        assert post(c,'/api/profiles/rename',{'profile_id':b,'label':'  Druga córka  '}).json()['name']=='Druga córka'
        assert post(c,'/api/profiles/rename',{'profile_id':a,'label':'Pierwsza córka'}).json()['name']=='Druga córka'
        assert post(c,'/api/profiles/add',{'login':'zuzia','password':'ok'}).status_code==409
        assert post(c,'/api/profiles/switch',{'profile_id':'nieistnieje'}).status_code==404
        assert post(c,'/api/profiles/remove',{'profile_id':'nieistnieje'}).status_code==404
        assert post(c,'/api/profiles/remove',{'profile_id':b}).json()['name']=='Pierwsza córka'
        assert fake_librus.connections[1].closed
        assert not fake_librus.connections[0].closed
        assert c.get('/api/section/grades').json()['subjects'][0]['grades'][0]['value']=='5'
        assert post(c,'/api/profiles/remove',{'profile_id':a}).status_code==409
        assert c.post('/api/logout').status_code==200
        assert fake_librus.connections[0].closed
        assert c.get('/api/section/grades').status_code==401


def test_add_invalid_duplicate_and_csrf_do_not_destroy_first_login(fake_librus):
    with TestClient(app) as c:
        assert post(c,'/api/login',{'login':'ania','password':'ok'}).status_code==200
        assert post(c,'/api/profiles/add',{'login':'ania','password':'ok'}).status_code==409
        assert post(c,'/api/profiles/add',{'login':'ola','password':'wrong'}).status_code==401
        assert fake_librus.connections[-1].closed
        assert len(c.get('/api/profiles').json()['profiles'])==1
        for endpoint,body in [('/api/profiles/add',{'login':'ola','password':'ok'}),
                              ('/api/profiles/switch',{'profile_id':'x'}),
                              ('/api/profiles/remove',{'profile_id':'x'}),
                              ('/api/profiles/rename',{'profile_id':'x','label':'x'})]:
            r=c.post(endpoint,json=body,headers={'origin':'https://not-our-site.invalid'})
            assert r.status_code==403,(endpoint,r.text)
        assert c.post('/api/profiles/add',json={'login':'ola','password':'ok'},headers={'sec-fetch-site':'cross-site'}).status_code==403
        assert len(c.get('/api/profiles').json()['profiles'])==1
        assert c.get('/api/section/grades').json()['count']==1


def test_demo_and_other_browser_cannot_mutate_profiles(fake_librus):
    with TestClient(app) as a, TestClient(app) as b:
        assert a.post('/api/demo').status_code==200
        assert post(a,'/api/profiles/add',{'login':'ania','password':'ok'}).status_code==409
        assert len(a.get('/api/profiles').json()['profiles'])==1
        assert b.get('/api/profiles').status_code==401
        assert post(b,'/api/profiles/switch',{'profile_id':a.get('/api/session').json()['active_profile_id']}).status_code==401
        assert post(b,'/api/login',{'login':'ola','password':'ok'}).status_code==200
        assert b.get('/api/section/grades').json()['subjects'][0]['grades'][0]['value']=='2'
        assert a.get('/api/section/grades').json()['count']!=None
        assert a.post('/api/logout').status_code==200
        assert b.get('/api/session').json()['authenticated']
        assert b.post('/api/logout').status_code==200


def test_html_and_javascript_profile_controls(fake_librus):
    with TestClient(app) as c:
        html=c.get('/').text
        js=c.get('/static/app.js').text
        css=c.get('/static/style.css').text
        for id in ('profile-trigger','profile-options','add-profile-btn','profile-dialog','profile-form','profile-login','profile-password'):
            assert f'id="{id}"' in html
        for endpoint in ('/api/profiles/add','/api/profiles/switch','/api/profiles/remove','/api/profiles/rename'):
            assert endpoint in js
        assert 'profile-menu' in css
        assert 'localStorage' not in js
        assert 'sessionStorage' not in js
