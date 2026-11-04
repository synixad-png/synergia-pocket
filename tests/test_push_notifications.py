"""Push regression: no real school account or push requests are used."""
import asyncio
import base64
import os
from pathlib import Path
from types import SimpleNamespace as NS
from cryptography.fernet import Fernet
from fastapi import HTTPException
from starlette.requests import Request
from push_features import PushService, SubscriptionBody, validate_subscription

ROOT = Path(__file__).resolve().parents[1]
APPLE_ENDPOINT = 'https://web.push.apple.com/mock-not-a-real-device'
B64 = lambda value: base64.urlsafe_b64encode(value).decode().rstrip('=')
KEYS = {'p256dh': B64(b'\x04' + b'A'*64), 'auth': B64(b'B'*16)}

class FakeRedis:
    def __init__(self):
        self.data={}
    async def set(self,key,value,ex=None,keepttl=False):
        self.data[key]=value
    async def get(self,key):
        return self.data.get(key)
    async def delete(self,key):
        self.data.pop(key,None)
    async def scan_iter(self,match,count):
        prefix=match.replace('*','')
        for key in list(self.data):
            if str(key).startswith(prefix):
                yield key

class FakeStore:
    def __init__(self):
        self.redis=FakeRedis()
        self.cipher=Fernet(Fernet.generate_key())
    async def get(self, token):
        return {'version':1} if token=='synthetic-token' else None

class FakeLibrus:
    def __init__(self, name):
        self.name=name
        self.list=[NS(id='old-'+name,read_date=None)]
    async def messages(self, mailbox,limit):
        assert mailbox=='inbox' and limit==30
        return self.list

class FakeSession:
    def __init__(self):
        self.lock=asyncio.Lock()
        self.profiles={
            'profile-h':NS(id='profile-h',name='Hania',mode='live',librus=FakeLibrus('h')),
            'profile-m':NS(id='profile-m',name='Martyna',mode='live',librus=FakeLibrus('m'))
        }

def request():
    return Request({'type':'http','method':'POST','headers':[(b'cookie',b'synergia_session=synthetic-token')]})

def make_service(monkeypatch):
    monkeypatch.setenv('SYNERGIA_VAPID_PRIVATE_KEY','synthetic-private')
    monkeypatch.setenv('SYNERGIA_VAPID_PUBLIC_KEY','synthetic-public')
    store=FakeStore()
    session=FakeSession()
    async def get_session(req): return session
    async def restore(token): return session
    service=PushService(store, {'synthetic-token':session}, restore,get_session,lambda _:None,lambda a,b:None)
    return service,session,store

def test_endpoint_validation_blocks_ssrf():
    assert validate_subscription(APPLE_ENDPOINT,KEYS)['endpoint']==APPLE_ENDPOINT
    for host in ['https://localhost/test','https://127.0.0.1/test',
                 'https://evil.com/test','http://web.push.apple.com/test',
                 'https://web.push.apple.com.evil.com/test',
                 'https://web.push.apple.com:4444/test']:
        try:
            validate_subscription(host,KEYS)
        except HTTPException as exc:
            assert exc.status_code==400
        else:
            raise AssertionError('SSRF host passed: '+host)
    try:
        validate_subscription(APPLE_ENDPOINT,{'p256dh':'invalid','auth':'bad'})
    except HTTPException as exc: assert exc.status_code==400
    else: raise AssertionError('Malformed key passed')

def test_two_profiles_new_unread_only_and_dedupe(monkeypatch):
    service,session,store=make_service(monkeypatch)
    notifications=[]
    async def fake_send(subscription,payload):
        notifications.append(payload)
    service._send=fake_send
    async def check():
        await service.subscribe(request(),SubscriptionBody(endpoint=APPLE_ENDPOINT,keys=KEYS))
        stored=list(store.redis.data.values())
        assert stored and b'synthetic-token' not in stored[0]
        key=next(iter(store.redis.data))
        record=await service._read(key)
        assert set(record['seen'])=={'profile-h','profile-m'}
        await service.poll()
        assert not notifications
        session.profiles['profile-h'].librus.list.insert(0,NS(id='new-h',read_date=None))
        session.profiles['profile-m'].librus.list.insert(0,NS(id='read-m',read_date='2026-11-04'))
        await service.poll()
        assert len(notifications)==1 and notifications[0]['profile']=='profile-h'
        assert notifications[0]['body'].endswith('Hania')
        assert 'new-h' in notifications[0]['message']
        await service.poll()
        assert len(notifications)==1
        session.profiles['profile-m'].librus.list.insert(0,NS(id='new-m',read_date=None))
        await service.poll()
        assert len(notifications)==2 and notifications[1]['profile']=='profile-m'
        await service.forget_token('synthetic-token')
        assert store.redis.data=={}
    asyncio.run(check())

def test_push_file_links_and_security():
    app=(ROOT/'app.py').read_text()
    html=(ROOT/'static/index.html').read_text()
    js=(ROOT/'static/app.js').read_text()
    sw=(ROOT/'static/sw.js').read_text()
    pushjs=(ROOT/'static/push.js').read_text()
    assert '@app.get("/sw.js")' in app
    assert 'PUSH_SERVICE.schedule()' in app
    assert 'PUSH_SERVICE.forget_token(token)' in app
    assert 'push_features.py' in (ROOT/'Dockerfile').read_text()
    assert 'pywebpush' in (ROOT/'requirements.txt').read_text()
    assert 'static/push.js' in html
    assert 'id="push-enable"' in js
    assert 'Notification.requestPermission()' in pushjs
    assert 'pushManager.subscribe(' in pushjs
    assert "self.registration.showNotification" in sw
    assert "'notificationclick'" in sw
    assert "'fetch'" not in sw
