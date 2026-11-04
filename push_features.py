"""Opt-in Web Push for Synergia Pocket (two profiles, unread inbox only).

Registrations are encrypted with the existing server-side Fernet key in Redis.
No passwords, message content or teacher names are stored in push records.
The health-check keep-warm request schedules a bounded, asynchronous poll.
"""
from __future__ import annotations

import asyncio
import base64
import hashlib
import json
import logging
import os
import time
from urllib.parse import urlsplit
from typing import Any, Callable

from fastapi import FastAPI, HTTPException, Request
from pydantic import BaseModel, Field

LOG = logging.getLogger("synergia_pocket.push")
PREFIX = "synergia:pocket:push:reg:"
PERIOD = 9 * 60
MAX_DEVICES = 20
MAX_MESSAGES = 30
MAX_SEEN = 120
TTL = 7 * 24 * 3600
ALLOWED_PUSH_HOSTS = {
    "web.push.apple.com", "fcm.googleapis.com",
    "updates.push.services.mozilla.com", "push.services.mozilla.com",
}


class SubscriptionBody(BaseModel):
    endpoint: str = Field(min_length=20, max_length=2048)
    keys: dict[str, str]


class EndpointBody(BaseModel):
    endpoint: str = Field(min_length=20, max_length=2048)


def validate_subscription(endpoint: str, keys: dict[str, str]) -> dict[str, Any]:
    """Reject SSRF/private URLs and malformed Web Push keys."""
    try:
        url = urlsplit(endpoint)
        hostname = (url.hostname or "").lower()
        allowed = hostname in ALLOWED_PUSH_HOSTS or hostname.endswith(".notify.windows.com")
        if (url.scheme != "https" or not allowed or url.port not in (None, 443)
                or url.username or url.password or url.fragment or not url.path):
            raise ValueError("Unknown push endpoint")
        p256dh, auth = keys["p256dh"], keys["auth"]
        if not isinstance(p256dh, str) or not isinstance(auth, str):
            raise ValueError("Invalid key types")
        if len(p256dh) > 160 or len(auth) > 100:
            raise ValueError("Invalid key length")
        def decode(value: str) -> bytes:
            if not value or not all(ch.isalnum() or ch in "_-" for ch in value):
                raise ValueError("Invalid key encoding")
            return base64.urlsafe_b64decode(value + "=" * (-len(value) % 4))
        public, secret = decode(p256dh), decode(auth)
        if len(public) != 65 or public[0] != 4 or len(secret) != 16:
            raise ValueError("Invalid Web Push key size")
    except (ValueError, KeyError, TypeError, OverflowError) as exc:
        raise HTTPException(400, "Nieprawidłowa subskrypcja Push.") from exc
    return {"endpoint": endpoint, "keys": {"p256dh": p256dh, "auth": auth}}


def endpoint_key(endpoint: str) -> str:
    return PREFIX + hashlib.sha256(endpoint.encode("utf-8")).hexdigest()


class PushService:
    def __init__(self, store: Any, sessions: dict, restore: Callable, get_session: Callable,
                 origin_check: Callable, session_check: Callable):
        self.store = store
        self.sessions = sessions
        self.restore = restore
        self.get_session = get_session
        self.origin_check = origin_check
        self.session_check = session_check
        self.private_key = os.getenv("SYNERGIA_VAPID_PRIVATE_KEY", "").strip()
        self.public_key = os.getenv("SYNERGIA_VAPID_PUBLIC_KEY", "").strip()
        self.ready = bool(store is not None and self.private_key and self.public_key)
        self._poll_lock = asyncio.Lock()
        self._last_poll = 0.0
        self._task: asyncio.Task | None = None

    def config(self):
        return {"available": self.ready, "public_key": self.public_key if self.ready else "",
                "interval_minutes": 10, "scope": "unread_messages"}

    def schedule(self):
        if not self.ready or self._poll_lock.locked():
            return
        if self._task and not self._task.done():
            return
        if time.monotonic() - self._last_poll < PERIOD:
            return
        self._task = asyncio.create_task(self.poll())

    async def close(self):
        task = self._task
        if task and not task.done():
            task.cancel()
            try:
                await task
            except asyncio.CancelledError:
                pass

    async def _read(self, key: str):
        encrypted = await self.store.redis.get(key)
        if not encrypted:
            return None
        try:
            value = json.loads(self.store.cipher.decrypt(encrypted))
            return value if isinstance(value, dict) and value.get("version") == 1 else None
        except (ValueError, TypeError, UnicodeError, Exception) as exc:
            # Deliberately do not log contents (contains a browser session token).
            LOG.warning("Uszkodzony rekord Push: %s", type(exc).__name__)
            return None

    async def _write(self, key: str, record: dict, fresh: bool):
        data = json.dumps(record, ensure_ascii=False, separators=(",", ":")).encode()
        encrypted = self.store.cipher.encrypt(data)
        if fresh:
            await self.store.redis.set(key, encrypted, ex=TTL)
        else:
            # Polling must NOT prolong the validity of a private session indefinitely.
            await self.store.redis.set(key, encrypted, keepttl=True)

    @staticmethod
    async def _inbox(client):
        messages = await client.messages("inbox", limit=MAX_MESSAGES)
        return [m for m in (messages or []) if getattr(m, "id", None) is not None]

    async def subscribe(self, request: Request, body: SubscriptionBody):
        self.origin_check(request)
        if not self.ready:
            raise HTTPException(503, "Powiadomienia wymagają aktywnego magazynu i kluczy Push.")
        subscription = validate_subscription(body.endpoint, body.keys)
        session = await self.get_session(request)
        token = request.cookies.get("synergia_session", "")
        key = endpoint_key(body.endpoint)
        async with session.lock:
            self.session_check(request, session)
            profiles = [p for p in session.profiles.values() if p.mode == "live"]
            if not profiles:
                raise HTTPException(403, "Powiadomienia są dostępne tylko dla kont Synergia.")
            existing = await self._read(key)
            if existing and existing.get("token") == token:
                seen = existing.get("seen", {})
            else:
                # Baseline first: old unread messages never cause retroactive notifications.
                seen = {}
            for profile in profiles:
                if profile.id not in seen:
                    try:
                        seen[profile.id] = [str(m.id) for m in await self._inbox(profile.librus)]
                    except Exception as exc:
                        LOG.warning("Nie udało się ustalić punktu startowego powiadomień: %s", type(exc).__name__)
                        raise HTTPException(503, "Nie można teraz pobrać wiadomości z Synergii.") from exc
            record = {"version": 1, "token": token, "subscription": subscription,
                      "seen": {p.id: list(seen.get(p.id, []))[-MAX_SEEN:] for p in profiles},
                      "last_test": (existing or {}).get("last_test", 0) if existing and existing.get("token") == token else 0}
            if existing is None:
                n = 0
                async for _ in self.store.redis.scan_iter(match=PREFIX + "*", count=30):
                    n += 1
                    if n >= MAX_DEVICES:
                        raise HTTPException(429, "Osiągnięto limit urządzeń z powiadomieniami.")
            await self._write(key, record, True)
            return {"ok": True, "profiles": [p.name for p in profiles]}

    async def unsubscribe(self, request: Request, body: EndpointBody):
        self.origin_check(request)
        if not self.ready:
            return {"ok": True}
        session = await self.get_session(request)
        token = request.cookies.get("synergia_session", "")
        async with session.lock:
            self.session_check(request, session)
            key = endpoint_key(body.endpoint)
            old = await self._read(key)
            if old and old.get("token") == token:
                await self.store.redis.delete(key)
        return {"ok": True}

    async def _send(self, subscription: dict, payload: dict) -> None:
        from pywebpush import webpush_async
        # No teacher names, message bodies, grades or credentials go to push gateways.
        await webpush_async(
            subscription_info=subscription,
            data=json.dumps(payload, ensure_ascii=False),
            vapid_private_key=self.private_key,
            vapid_claims={"sub": "https://synergia-pocket.onrender.com"},
            ttl=3600, timeout=12,
        )

    async def send_test(self, request: Request, body: EndpointBody):
        self.origin_check(request)
        if not self.ready:
            raise HTTPException(503, "Powiadomienia nie są jeszcze skonfigurowane.")
        session = await self.get_session(request)
        token = request.cookies.get("synergia_session", "")
        key = endpoint_key(body.endpoint)
        async with session.lock:
            self.session_check(request, session)
            old = await self._read(key)
            if not old or old.get("token") != token:
                raise HTTPException(404, "To urządzenie nie ma aktywnej subskrypcji.")
            if time.time() - float(old.get("last_test", 0)) < 90:
                raise HTTPException(429, "Test można powtórzyć po 90 sekundach.")
            old["last_test"] = time.time()
            await self._write(key, old, False)
            subscription = old["subscription"]
        try:
            await self._send(subscription, {"kind": "test", "title": "Synergia Pocket",
                                           "body": "Powiadomienia na tym urządzeniu działają.",
                                           "tag": "synergia-test"})
        except Exception as exc:
            LOG.warning("Błąd wysyłki testowego Push: %s", type(exc).__name__)
            raise HTTPException(502, "Serwer Push nie potwierdził przyjęcia testu.") from exc
        return {"ok": True, "accepted": True}

    async def forget_token(self, token: str):
        if not self.ready or not token:
            return
        async for key in self.store.redis.scan_iter(match=PREFIX + "*", count=30):
            item = await self._read(key)
            if item and item.get("token") == token:
                await self.store.redis.delete(key)

    async def poll(self):
        if not self.ready:
            return
        async with self._poll_lock:
            self._last_poll = time.monotonic()
            try:
                n = 0
                async for key in self.store.redis.scan_iter(match=PREFIX + "*", count=30):
                    if n >= MAX_DEVICES:
                        break
                    n += 1
                    try:
                        await self._poll_one(key)
                    except asyncio.CancelledError:
                        raise
                    except Exception as exc:
                        LOG.warning("Błąd sprawdzania Push: %s", type(exc).__name__)
            except asyncio.CancelledError:
                raise
            except Exception as exc:
                LOG.warning("Błąd kolejki Push: %s", type(exc).__name__)

    async def _poll_one(self, key: bytes | str):
        record = await self._read(key)
        if not record:
            return
        token = record.get("token", "")
        if not isinstance(token, str) or not token:
            await self.store.redis.delete(key)
            return
        # The Redis session has its own expiry and is the authority.
        snapshot = await self.store.get(token)
        if snapshot is None:
            await self.store.redis.delete(key)
            return
        session = self.sessions.get(token)
        if session is None:
            try:
                session = await self.restore(token)
            except Exception as exc:
                LOG.warning("Sesja chwilowo nieodtwarzalna: %s", type(exc).__name__)
                return
        if session is None:
            # Provider auth can expire; stop background access without password retry.
            await self.store.redis.delete(key)
            return
        seen = record.get("seen", {})
        if not isinstance(seen, dict):
            seen = {}
        changed = False
        async with session.lock:
            for profile in list(session.profiles.values())[:2]:
                if profile.mode != "live" or not profile.librus:
                    continue
                try:
                    messages = await self._inbox(profile.librus)
                except Exception as exc:
                    LOG.info("Pominięto chwilowo konto bez dostępu do poczty: %s", type(exc).__name__)
                    continue
                ids = [str(m.id) for m in messages]
                known = set(map(str, seen.get(profile.id, [])))
                if profile.id not in seen:
                    seen[profile.id] = ids
                    changed = True
                    continue
                fresh = [m for m in messages if str(m.id) not in known and getattr(m, "read_date", None) is None]
                if fresh:
                    newest = fresh[0]
                    payload = {"kind": "message", "title": "Synergia Pocket",
                               "body": f"Nowa wiadomość — {profile.name}",
                               "profile": profile.id, "message": str(newest.id),
                               "tag": "synergia-" + hashlib.sha256((profile.id + str(newest.id)).encode()).hexdigest()[:24]}
                    try:
                        await self._send(record["subscription"], payload)
                    except Exception as exc:
                        LOG.warning("Push niepotwierdzony: %s", type(exc).__name__)
                        response = getattr(exc, "response", None)
                        status = getattr(response, "status", getattr(response, "status_code", None))
                        if status in (404, 410):
                            await self.store.redis.delete(key)
                            return
                        # Do not record unseen messages when sending failed.
                        continue
                seen[profile.id] = list(dict.fromkeys(ids + list(known)))[:MAX_SEEN]
                changed = True
        if changed:
            record["seen"] = seen
            await self._write(key, record, False)


def install_push_routes(app: FastAPI, store: Any, sessions: dict, restore: Callable,
                        get_session: Callable, origin_check: Callable,
                        session_check: Callable) -> PushService:
    service = PushService(store, sessions, restore, get_session, origin_check, session_check)

    @app.get("/api/push/config")
    async def push_config():
        return service.config()

    @app.post("/api/push/subscribe")
    async def push_subscribe(body: SubscriptionBody, request: Request):
        return await service.subscribe(request, body)

    @app.post("/api/push/unsubscribe")
    async def push_unsubscribe(body: EndpointBody, request: Request):
        return await service.unsubscribe(request, body)

    @app.post("/api/push/test")
    async def push_test(body: EndpointBody, request: Request):
        return await service.send_test(request, body)

    return service
