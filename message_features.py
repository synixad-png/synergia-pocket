"""Synergia Pocket: secure sending, replying and staff recipient directory."""
from __future__ import annotations
import logging
import re
import time
from collections import defaultdict
from typing import Any
from fastapi import FastAPI, HTTPException, Request
from pydantic import BaseModel, Field

LOG=logging.getLogger("synergia_pocket.messages")
ATTEMPTS: dict[tuple[str,str],dict[str,float]]=defaultdict(dict)
MAX_WRITES=10

class ComposeBody(BaseModel):
    profile_id: str=Field(min_length=1,max_length=80)
    recipient_id: str=Field(min_length=1,max_length=160)
    subject: str=Field(min_length=1,max_length=200)
    text: str=Field(min_length=1,max_length=10000)
    nonce: str=Field(min_length=16,max_length=100)

class ReplyBody(BaseModel):
    profile_id: str=Field(min_length=1,max_length=80)
    message_id: str=Field(min_length=1,max_length=160)
    text: str=Field(min_length=1,max_length=10000)
    nonce: str=Field(min_length=16,max_length=100)

def clean(value:Any)->str:
    return str(value).strip() if value is not None else ""

def contact(item:Any,klass:str)->dict:
    group=clean(getattr(item,"group",""))
    return {
        "id":clean(getattr(item,"account_id","")),
        "name":clean(getattr(item,"name","")),
        "group":group,
        "role":clean(getattr(item,"group_name","")) or group,
        "subjects":[clean(s) for s in (getattr(item,"subjects",[]) or []) if clean(s)],
        "class_name":klass if group=="tutors" else "",
        "available":getattr(item,"available",None),
    }

def throttle(request:Request,profile_id:str,nonce:str)->None:
    if not re.fullmatch(r"[A-Za-z0-9_-]{16,100}",nonce):
        raise HTTPException(400,"Nieprawidłowy identyfikator wysyłki.")
    key=(request.cookies.get("synergia_session",""),profile_id)
    now=time.monotonic()
    d=ATTEMPTS[key]
    for k,t in list(d.items()):
        if now-t>3600: del d[k]
    if nonce in d:
        raise HTTPException(409,"Ta wysyłka była już podjęta. Sprawdź Wysłane.")
    if len(d)>=MAX_WRITES:
        raise HTTPException(429,"Osiągnięto limit 10 wysyłek na godzinę dla tego konta.")
    d[nonce]=now
    if len(ATTEMPTS)>2000:
        for k,entries in list(ATTEMPTS.items()):
            if not entries or all(now-t>3600 for t in entries.values()): del ATTEMPTS[k]

def mapped_error(exc:Exception)->HTTPException:
    try:
        from librus_synergia.exceptions import LibrusMessageSendError,LibrusAuthError
    except ImportError:
        LibrusMessageSendError=type("DummySendError",(Exception,),{})
        LibrusAuthError=type("DummyAuthError",(Exception,),{})
    if isinstance(exc,LibrusMessageSendError):
        if exc.sent is None:
            return HTTPException(409,"Nie wiadomo, czy Librus wysłał wiadomość. Najpierw sprawdź Wysłane, nie ponawiaj automatycznie.")
        return HTTPException(422,"Librus odmówił wysłania wiadomości. Sprawdź odbiorcę i uprawnienia.")
    if isinstance(exc,LibrusAuthError):
        return HTTPException(401,"Librus wymaga ponownego logowania do konta.")
    if isinstance(exc,ValueError):
        return HTTPException(422,"Nieprawidłowy temat lub treść wiadomości.")
    LOG.warning("Problem z API wiadomości: %s",type(exc).__name__)
    return HTTPException(502,"Brak potwierdzenia z Librusa. Przed ponowieniem sprawdź Wysłane.")

def install_message_routes(app:FastAPI,get_session,require_origin,assert_session_current)->None:
    def selected(session,profile_id):
        if session.active_profile_id!=profile_id:
            raise HTTPException(409,"Zmieniono profil dziecka. Zamknij formularz i otwórz go ponownie.")
        p=session.active
        if p.mode!="live" or p.librus is None:
            raise HTTPException(403,"Wiadomości można wysyłać tylko z prawdziwego konta Synergia.")
        return p

    async def directory(p):
        cached=p.cache.get("message-directory")
        if cached and time.monotonic()-cached[0]<300:
            return cached[1]
        recipients=await p.librus.message_recipients()
        try: klass=await p.librus.school_class()
        except Exception: klass=None
        class_name=clean(getattr(klass,"display_name",""))
        items=[contact(r,class_name) for r in recipients]
        items=[r for r in items if r["name"] and r["id"]]
        rank={"tutors":0,"teachers":1,"headteachers":2,"secretary":3}
        items.sort(key=lambda r:(rank.get(r["group"],4),r["name"].casefold()))
        data={"profile_id":p.id,"class_name":class_name,"items":items}
        p.cache["message-directory"]=(time.monotonic(),data)
        return data

    @app.get("/api/contacts")
    async def contacts(request:Request):
        s=await get_session(request)
        async with s.lock:
            assert_session_current(request,s)
            p=s.active
            if p.mode!="live":
                return {"profile_id":p.id,"class_name":"","items":[],"demo":True}
            try: return await directory(p)
            except Exception as exc: raise mapped_error(exc) from None

    @app.post("/api/send-message")
    async def send_message(body:ComposeBody,request:Request):
        require_origin(request)
        s=await get_session(request)
        async with s.lock:
            assert_session_current(request,s)
            p=selected(s,body.profile_id)
            try: recipients=await directory(p)
            except Exception as exc: raise mapped_error(exc) from None
            if body.recipient_id not in {r["id"] for r in recipients["items"]}:
                raise HTTPException(400,"Wybranego pracownika nie ma na liście dostępnych odbiorców.")
            if not body.subject.strip() or not body.text.strip():
                raise HTTPException(422,"Wpisz temat oraz treść.")
            throttle(request,p.id,body.nonce)
            try: receipt=await p.librus.send_message([body.recipient_id],body.subject.strip(),body.text.strip())
            except Exception as exc: raise mapped_error(exc) from None
            p.cache.pop("messages::outbox",None)
            p.cache.pop("messages::inbox",None)
            return {"sent":True,"message_id":getattr(receipt,"message_id",None),"profile_id":p.id}

    @app.post("/api/reply-message")
    async def reply_message(body:ReplyBody,request:Request):
        require_origin(request)
        s=await get_session(request)
        async with s.lock:
            assert_session_current(request,s)
            p=selected(s,body.profile_id)
            if not body.text.strip(): raise HTTPException(422,"Wpisz treść odpowiedzi.")
            throttle(request,p.id,body.nonce)
            try: receipt=await p.librus.reply_to_message(body.message_id,body.text.strip(),mailbox="inbox")
            except Exception as exc: raise mapped_error(exc) from None
            p.cache.pop("messages::outbox",None)
            p.cache.pop("messages::inbox",None)
            return {"sent":True,"message_id":getattr(receipt,"message_id",None),"profile_id":p.id}
