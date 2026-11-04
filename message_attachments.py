"""Izolowany moduł pobierania ZAŁĄCZNIKÓW WIADOMOŚCI.

Każde żądanie potwierdza właściciela wiadomości, identyfikator załącznika,
skrzynkę oraz aktywny profil. Nie zmienia funkcji wysyłania ani poczty.
"""
from __future__ import annotations

import io
import logging
import re
import unicodedata
import zipfile
from urllib.parse import quote

from fastapi import HTTPException, Request
from fastapi.responses import Response

LOG = logging.getLogger("synergia_pocket.message_attachments")
MAX_ATTACHMENT_BYTES = 20 * 1024 * 1024
IDENTIFIER_RE = re.compile(r"[A-Za-z0-9_-]{1,160}\Z")
FILENAME_FALLBACK = "zalacznik"


def _field(obj, name, default=None):
    return obj.get(name, default) if isinstance(obj, dict) else getattr(obj, name, default)


def _filename(value: str, default=FILENAME_FALLBACK) -> str:
    name = str(value or "").replace("\\", "/").split("/")[-1]
    name = re.sub(r'[\x00-\x1f\x7f<>:"/\\|?*]', "_", name).strip(" .")
    if not name or name.startswith("."):
        name = default
    if len(name) > 160:
        root, dot, ext = name.rpartition(".")
        name = root[:126] + "." + ext[:24] if dot and len(ext) < 25 else name[:160]
    return name


def _ext_from_data(content: bytes, content_type: str) -> str:
    if content.startswith(b"%PDF-"):
        return ".pdf"
    if content.startswith(b"\x89PNG\r\n\x1a\n"):
        return ".png"
    if content.startswith(b"\xff\xd8\xff"):
        return ".jpg"
    if content.startswith((b"GIF87a", b"GIF89a")):
        return ".gif"
    if content.startswith(b"{\\rtf"):
        return ".rtf"
    if content.startswith(b"PK\x03\x04"):
        try:
            with zipfile.ZipFile(io.BytesIO(content)) as archive:
                names = set(archive.namelist()[:1000])
            if "word/document.xml" in names:
                return ".docx"
            if "xl/workbook.xml" in names:
                return ".xlsx"
            if "ppt/presentation.xml" in names:
                return ".pptx"
        except (OSError, ValueError, zipfile.BadZipFile):
            pass
        return ".zip"
    types = {
        "application/pdf": ".pdf",
        "application/msword": ".doc",
        "application/vnd.openxmlformats-officedocument.wordprocessingml.document": ".docx",
        "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": ".xlsx",
        "application/vnd.openxmlformats-officedocument.presentationml.presentation": ".pptx",
        "application/vnd.ms-excel": ".xls",
        "text/plain": ".txt",
        "text/csv": ".csv",
        "image/png": ".png",
        "image/jpeg": ".jpg",
        "image/gif": ".gif",
        "application/zip": ".zip",
    }
    return types.get(content_type.split(";", 1)[0].strip().lower(), ".bin")


def _download_name(attachment, record) -> str:
    source_name = str(_field(record, "filename", "") or "").strip()
    delivered_name = str(_field(attachment, "filename", "") or "").strip()
    # Nazwa widoczna w wiadomości ma priorytet nad technicznym "attachment-123".
    proposed = source_name or delivered_name
    if (not proposed or re.fullmatch(r"attachment-[A-Za-z0-9_-]+", proposed)):
        proposed = delivered_name if delivered_name and not re.fullmatch(r"attachment-[A-Za-z0-9_-]+", delivered_name) else FILENAME_FALLBACK
    name = _filename(proposed)
    last = name.rsplit(".", 1)
    if (len(last) < 2 or not 1 <= len(last[1]) <= 12
            or not re.fullmatch(r"[A-Za-z0-9]+", last[1]) or last[1].isdigit()):
        content = _field(attachment, "content", b"") or b""
        typ = str(_field(attachment, "content_type", "") or "")
        name += _ext_from_data(bytes(content), typ)
    return name


def _file_response(attachment, record) -> Response:
    content = _field(attachment, "content")
    if not isinstance(content, bytes):
        raise HTTPException(502, "Librus zwrócił nieprawidłowy format załącznika.")
    if len(content) > MAX_ATTACHMENT_BYTES:
        raise HTTPException(413, "Załącznik przekracza limit 20 MiB.")
    name = _download_name(attachment, record)
    ascii_name = unicodedata.normalize("NFKD", name).encode("ascii", "ignore").decode("ascii")
    ascii_name = _filename(ascii_name, default="zalacznik.bin")
    disposition = f'attachment; filename="{ascii_name}"; filename*=UTF-8\x27\x27{quote(name, safe="")}'
    media_type = str(_field(attachment, "content_type", "") or "").split(";", 1)[0].strip().lower()
    if not media_type or media_type in ("text/html", "application/xhtml+xml"):
        media_type = "application/octet-stream"
    return Response(
        content=content, media_type=media_type,
        headers={
            "Content-Disposition": disposition,
            "Cache-Control": "no-store",
            "X-Content-Type-Options": "nosniff",
        },
    )


def _upstream_error(exc: Exception) -> HTTPException:
    try:
        from librus_synergia.exceptions import LibrusAuthError
        if isinstance(exc, LibrusAuthError):
            return HTTPException(401, "Librus wymaga ponownego logowania do tego profilu.")
    except ImportError:
        pass
    LOG.warning("Pobieranie załącznika wiadomości nie powiodło się: %s", type(exc).__name__)
    return HTTPException(502, "Nie udało się pobrać załącznika wiadomości z Librusa.")


def install_message_attachment_routes(app, get_session, assert_session_current):
    @app.get("/api/message-attachments/{message_id}/{attachment_id}/download")
    async def download_message_attachment(
        message_id: str, attachment_id: str, request: Request,
        profile_id: str, mailbox: str = "inbox",
    ):
        if not IDENTIFIER_RE.fullmatch(message_id) or not IDENTIFIER_RE.fullmatch(attachment_id):
            raise HTTPException(400, "Nieprawidłowy identyfikator wiadomości lub załącznika.")
        if mailbox not in ("inbox", "outbox"):
            raise HTTPException(400, "Nieznana skrzynka wiadomości.")
        if not IDENTIFIER_RE.fullmatch(profile_id):
            raise HTTPException(400, "Nieprawidłowy identyfikator profilu.")

        session = await get_session(request)
        async with session.lock:
            assert_session_current(request, session)
            if session.active_profile_id != profile_id:
                raise HTTPException(409, "Zmieniono profil dziecka. Otwórz wiadomość ponownie.")
            profile = session.active
            if profile.mode != "live" or profile.librus is None:
                raise HTTPException(403, "Załączniki są dostępne tylko z konta Synergia.")
            bound_client = profile.librus
            try:
                # Ważne: żądany plik MUSI być wymieniony w tej konkretnej
                # wiadomości na tym koncie i w tej skrzynce.
                message = await bound_client.message(message_id, mailbox)
            except Exception as exc:
                raise _upstream_error(exc) from exc
            if message is None:
                raise HTTPException(404, "Nie znaleziono wiadomości w tym profilu.")
            record = next((
                a for a in (_field(message, "attachments", []) or [])
                if str(_field(a, "id", "")) == attachment_id
            ), None)
            if record is None:
                raise HTTPException(404, "Załącznik nie należy do tej wiadomości.")

        # Nie trzymamy blokady sesji podczas potencjalnie długiego transferu.
        try:
            file_data = await bound_client.download_attachment(attachment_id, message_id)
        except Exception as exc:
            raise _upstream_error(exc) from exc
        if file_data is None:
            raise HTTPException(404, "Załącznik nie jest dostępny na tym koncie.")

        async with session.lock:
            assert_session_current(request, session)
            if session.active_profile_id != profile_id or session.active.librus is not bound_client:
                raise HTTPException(409, "Zmieniono profil w trakcie pobierania. Ponów operację.")
        return _file_response(file_data, record)
