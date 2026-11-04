"""Typed data returned by the stand-alone client."""
from __future__ import annotations
from dataclasses import dataclass
import base64
import html
import re
from typing import Any

_CDATA = re.compile(r"<Content>\s*<!\[CDATA\[(.*?)\]\]>", re.I | re.S)
_TAG = re.compile(r"<[^>]*>")


@dataclass(frozen=True, slots=True)
class Grade:
    id: str
    value: str
    subject: str
    category: str
    date: str | None
    semester: int | None


@dataclass(frozen=True, slots=True)
class MessagePreview:
    id: str
    mailbox: str
    sender: str
    subject: str
    preview: str
    date: str | None
    unread: bool


@dataclass(frozen=True, slots=True)
class Lesson:
    day: str
    number: int | None
    start: str | None
    end: str | None
    subject: str
    teacher: str
    room: str
    canceled: bool
    substitution: bool
    original_subject: str
    note: str | None


def decode_preview(value: Any) -> str:
    """A message preview may be truncated upstream, including mid UTF-8."""
    if not isinstance(value, str):
        return ""
    try:
        decoded = base64.b64decode(value, validate=True)
        text = decoded.decode("utf-8", errors="ignore")
        if not text and value:
            return value
    except (ValueError, UnicodeError):
        return value
    match = _CDATA.search(text)
    if match:
        text = match.group(1)
    text = re.sub(r"<br\s*/?>", "\n", text, flags=re.I)
    text = _TAG.sub("", text)
    return html.unescape(text).strip()


def reference_names(payload: dict[str, Any], key: str, *, people: bool = False) -> dict[str, str]:
    """Read reference names without dropping string-valued identifiers."""
    array = payload.get(key)
    if not isinstance(array, list):
        return {}
    out: dict[str, str] = {}
    for entry in array:
        if not isinstance(entry, dict) or entry.get("Id") is None:
            continue
        if people:
            name = " ".join(str(entry.get(k) or "") for k in ("FirstName", "LastName")).strip()
        else:
            name = str(entry.get("Name") or entry.get("Symbol") or "")
        out[str(entry["Id"])] = name
    return out
