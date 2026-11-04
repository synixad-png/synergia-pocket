"""Tests for messaging routing without real Librus credentials or real sends."""
import secrets
from types import SimpleNamespace as NS
from fastapi.testclient import TestClient
from app import app, SESSIONS, Session, make_profile
from message_features import ATTEMPTS

class Stub:
    def __init__(self, label):
        self.label=label;self.sends=[];self.replies=[]
    async def message_recipients(self):
        return [NS(account_id=self.label+"-tutor",name="Anna Nowak",group="tutors",group_name="Wychowawcy",subjects=["Matematyka"],available=True)]
    async def school_class(self):return NS(display_name="5b")
    async def send_message(self,ids,topic,text):
        self.sends.append((ids,topic,text))
        return NS(message_id=8)
    async def reply_to_message(self,msg,text,mailbox="inbox"):
        self.replies.append((msg,text,mailbox))
        return NS(message_id=None)

def test_directory_send_and_profile_lock():
    a,b=Stub("a"),Stub("b");p1=make_profile("live","A",a);p2=make_profile("live","B",b)
    s=Session();s.profiles={p1.id:p1,p2.id:p2};s.active_profile_id=p1.id
    token=secrets.token_urlsafe(32);SESSIONS[token]=s;ATTEMPTS.clear()
    try:
        with TestClient(app) as c:
            c.cookies.set("synergia_session",token)
            d=c.get("/api/contacts").json()
            assert d["items"][0]["class_name"]=="5b"
            body={"profile_id":p1.id,"recipient_id":"a-tutor","subject":"T","text":"Dzień dobry","nonce":"abcdefgh12345678901234"}
            assert c.post("/api/send-message",json=body).status_code==200
            assert c.post("/api/send-message",json=body).status_code==409
            assert len(a.sends)==1
            assert c.post("/api/send-message",json={**body,"recipient_id":"b-tutor","nonce":"abcdefgh12345678901235"}).status_code==400
            s.active_profile_id=p2.id
            assert c.post("/api/send-message",json={**body,"nonce":"abcdefgh12345678901236"}).status_code==409
            assert not b.sends
            reply={"profile_id":p2.id,"message_id":"123","text":"Tak","nonce":"abcdefgh12345678901237"}
            assert c.post("/api/reply-message",json=reply).status_code==200
            assert b.replies==[("123","Tak","inbox")]
    finally:
        SESSIONS.pop(token,None);ATTEMPTS.clear()
