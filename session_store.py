"""Opcjonalne trwałe sesje serwerowe. Bez przechowywania haseł i bez sejfu na telefonie.

Redis trzyma wyłącznie zaszyfrowane migawki ciasteczek Librusa i metadanych
profilu. Klucz Fernet należy trzymać jako sekret środowiskowy na hostingu.
Brak konfiguracji => aplikacja działa jak dotychczas tylko w RAM.
"""
from __future__ import annotations

import hashlib
import json
import os
from typing import Any
from urllib.parse import urlsplit

from cryptography.fernet import Fernet, InvalidToken
from redis.asyncio import Redis


class RedisSessionStore:
    def __init__(self, redis: Any, key: str | bytes):
        self.redis = redis
        self.cipher = Fernet(key.encode("ascii") if isinstance(key, str) else key)

    @staticmethod
    def _key(token: str) -> str:
        # Token przeglądarki nie występuje jawnie w nazwie klucza Redis.
        return "synergia:pocket:session:" + hashlib.sha256(token.encode("ascii")).hexdigest()

    async def get(self, token: str) -> dict[str, Any] | None:
        if not token or len(token) > 256:
            return None
        ciphertext = await self.redis.get(self._key(token))
        if ciphertext is None:
            return None
        try:
            data = json.loads(self.cipher.decrypt(ciphertext))
        except (InvalidToken, ValueError, TypeError, UnicodeDecodeError):
            return None
        return data if isinstance(data, dict) and data.get("version") == 1 else None

    async def save(self, token: str, data: dict[str, Any], ttl: int) -> None:
        if ttl <= 0:
            raise ValueError("TTL must be positive")
        plaintext = json.dumps(data, separators=(",", ":"), ensure_ascii=False).encode("utf-8")
        await self.redis.set(self._key(token), self.cipher.encrypt(plaintext), ex=ttl)

    async def delete(self, token: str) -> None:
        if token:
            await self.redis.delete(self._key(token))

    async def close(self) -> None:
        await self.redis.aclose()


def configured_store() -> RedisSessionStore | None:
    url = os.getenv("SYNERGIA_REDIS_URL", "").strip()
    key = os.getenv("SYNERGIA_SESSION_FERNET_KEY", "").strip()
    if not url and not key:
        return None
    if not url or not key:
        raise RuntimeError(
            "Trwałe sesje wymagają obu sekretów: SYNERGIA_REDIS_URL i SYNERGIA_SESSION_FERNET_KEY."
        )
    parsed = urlsplit(url)
    private_render = (
        parsed.scheme == "redis"
        and parsed.hostname is not None
        and parsed.hostname.startswith("red-")
        and parsed.hostname[4:].isalnum()
        and parsed.port == 6379
        and parsed.username is None
        and parsed.password is None
        and parsed.path in ("", "/")
        and not parsed.query
        and not parsed.fragment
        # Używamy niezabezpieczonego TLS-em transportu wyłącznie na
        # prywatnej sieci Render. Zawartość sesji i tak jest szyfrowana Fernet.
        and os.getenv("RENDER", "") == "true"
    )
    if parsed.scheme != "rediss" and not private_render:
        raise RuntimeError("Redis wymaga rediss:// lub prywatnej sieci Render redis://red-*:6379.")
    return RedisSessionStore(Redis.from_url(url, decode_responses=False), key)
