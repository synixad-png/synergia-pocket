"""Niezależny moduł Dokumenty szkoły — tylko odczyt i świadome pobieranie.

Nie dotyka poczty, ocen, planu ani mechanizmów Push. Każda operacja
odczytuje dokumenty wyłącznie z bieżącego profilu użytkownika.
"""
from __future__ import annotations

import io
import logging
import mimetypes
import re
import unicodedata
import zipfile
from urllib.parse import quote, urlsplit

from fastapi import HTTPException, Request
from fastapi.responses import Response

LOG = logging.getLogger("synergia_pocket.school_files")
MAX_DOWNLOAD_BYTES = 20 * 1024 * 1024
MAX_FILES_LISTED = 500
DEMO_ID = "demo-szkolny-dokument"
DEMO_CONTENT = "SYNERGIA POCKET - PLIK DEMONSTRACYJNY\nTo nie jest dokument szkolny.\n".encode("utf-8")


def _field(obj, key: str, default=None):
    return obj.get(key, default) if isinstance(obj, dict) else getattr(obj, key, default)


def _safe_source_path(value: str | None) -> bool:
    """Nie pozwalaj klientowi lub wadliwej odpowiedzi API wyznaczyć obcego hosta."""
    if not isinstance(value, str) or not value:
        return False
    try:
        u = urlsplit(value)
        if u.scheme or u.netloc:
            if u.scheme != "https" or u.netloc.lower() != "synergia.librus.pl":
                return False
        if u.query or u.fragment or u.username or u.password:
            return False
        return re.fullmatch(r"/pliki_szkoly/pobierz/[a-zA-Z0-9_-]+", u.path) is not None
    except ValueError:
        return False


def _document(item):
    identifier = str(_field(item, "id", "") or "").strip()
    return {
        "id": identifier,
        "name": str(_field(item, "name", "") or "Dokument szkolny").strip()[:260],
        "added": _field(item, "added"),
        "can_download": _safe_source_path(_field(item, "download_path")),
    }


def _clean_filename(value: str, fallback="dokument_szkolny") -> str:
    value = str(value or "").replace("\\", "/").split("/")[-1]
    value = re.sub(r'[\x00-\x1f\x7f<>:"/\\|?*]', "_", value).strip(" .")
    if not value or value.startswith("."):
        value = fallback
    if len(value) > 160:
        root, dot, ext = value.rpartition(".")
        value = (root[:130] + "." + ext[:22]) if dot and len(ext) <= 22 else value[:160]
    return value


def _file_extension(content: bytes, content_type: str) -> str:
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
                names = archive.namelist()[:1000]
                if "word/document.xml" in names:
                    return ".docx"
                if "xl/workbook.xml" in names:
                    return ".xlsx"
                if "ppt/presentation.xml" in names:
                    return ".pptx"
        except (OSError, ValueError, zipfile.BadZipFile):
            pass
        return ".zip"
    type_to_ext = {
        "application/pdf": ".pdf", "application/msword": ".doc",
        "application/vnd.openxmlformats-officedocument.wordprocessingml.document": ".docx",
        "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet": ".xlsx",
        "application/vnd.openxmlformats-officedocument.presentationml.presentation": ".pptx",
        "application/vnd.ms-excel": ".xls", "application/rtf": ".rtf",
        "image/jpeg": ".jpg", "image/png": ".png", "image/gif": ".gif",
        "text/plain": ".txt", "text/csv": ".csv",
        "application/zip": ".zip",
    }
    return type_to_ext.get(content_type.split(";", 1)[0].lower().strip(), ".bin")


def _attachment_name(attachment, record: dict) -> str:
    suggested = str(_field(attachment, "filename", "") or "").strip()
    if not suggested or re.fullmatch(r"school-file-[a-zA-Z0-9_-]+", suggested):
        suggested = record["name"]
    name = _clean_filename(suggested)
    # Nie nadpisuj już istniejącego rozszerzenia, nawet jeśli nie znamy formatu.
    last = name.rsplit(".", 1)
    if (len(last) == 1 or not 1 <= len(last[-1]) <= 12 or
            not re.fullmatch(r"[A-Za-z0-9]+", last[-1]) or last[-1].isdigit()):
        name += _file_extension(bytes(_field(attachment, "content", b"") or b""),
                                str(_field(attachment, "content_type", "") or ""))
    return name


def _attachment_response(attachment, record: dict) -> Response:
    content = _field(attachment, "content")
    if not isinstance(content, bytes):
        raise HTTPException(502, "Librus zwrócił nieprawidłowy format dokumentu.")
    if len(content) > MAX_DOWNLOAD_BYTES:
        raise HTTPException(413, "Dokument przekracza limit 20 MiB.")
    filename = _attachment_name(attachment, record)
    ascii_name = unicodedata.normalize("NFKD", filename).encode("ascii", "ignore").decode("ascii")
    ascii_name = _clean_filename(ascii_name or "dokument_szkolny")
    disposition = f'attachment; filename="{ascii_name}"; filename*=UTF-8\x27\x27{quote(filename, safe="")}'
    content_type = str(_field(attachment, "content_type", "") or "").split(";")[0].lower().strip()
    if not content_type or content_type == "text/html":
        content_type = "application/octet-stream"
    return Response(content=content, media_type=content_type, headers={
        "Content-Disposition": disposition,
        "Cache-Control": "no-store",
        "X-Content-Type-Options": "nosniff",
    })


def _upstream_error(exc: Exception, action: str) -> HTTPException:
    try:
        from librus_synergia.exceptions import LibrusAuthError
        if isinstance(exc, LibrusAuthError):
            return HTTPException(401, "Librus wymaga ponownego logowania do tego profilu.")
    except ImportError:
        pass
    LOG.warning("Niepowodzenie %s: %s", action, type(exc).__name__)
    return HTTPException(502, f"Nie udało się {action}. Możesz spróbować ponownie.")


def install_school_file_routes(app, get_session, assert_session_current):
    @app.get("/api/school-files")
    async def list_school_files(request: Request):
        session = await get_session(request)
        async with session.lock:
            assert_session_current(request, session)
            profile = session.active
            if profile.mode == "demo":
                return {"items": [{
                    "id": DEMO_ID, "name": "Przykładowy dokument szkolny.txt",
                    "added": "2026-11-04", "can_download": True,
                }], "demo": True, "truncated": False}
            try:
                entries = await profile.librus.school_files()
            except Exception as exc:
                raise _upstream_error(exc, "pobrać listy dokumentów") from exc
            items = [_document(x) for x in entries if _field(x, "id") is not None]
            items.sort(key=lambda x: str(x["added"] or ""), reverse=True)
            return {"items": items[:MAX_FILES_LISTED], "demo": False,
                    "truncated": len(items) > MAX_FILES_LISTED}

    @app.get("/api/school-files/{document_id}/download")
    async def download_school_file(document_id: str, request: Request):
        if not re.fullmatch(r"[a-zA-Z0-9_-]{1,128}", document_id):
            raise HTTPException(400, "Nieprawidłowy identyfikator dokumentu.")
        session = await get_session(request)
        async with session.lock:
            assert_session_current(request, session)
            profile = session.active
            profile_id = profile.id
            if profile.mode == "demo":
                if document_id != DEMO_ID:
                    raise HTTPException(404, "Nie znaleziono pliku demonstracyjnego.")
                return _attachment_response({
                    "filename": "Przykladowy_dokument_szkolny.txt",
                    "content_type": "text/plain", "content": DEMO_CONTENT,
                }, {"name": "Przykładowy dokument szkolny.txt"})
            try:
                entries = await profile.librus.school_files()
            except Exception as exc:
                raise _upstream_error(exc, "sprawdzić dokumentu") from exc
            matched = next((x for x in entries if str(_field(x, "id", "")) == document_id), None)
            if matched is None:
                raise HTTPException(404, "Nie znaleziono dokumentu w aktywnym profilu.")
            record = _document(matched)
            path = _field(matched, "download_path")
            if not record["can_download"] or not _safe_source_path(path):
                raise HTTPException(422, "Brak bezpiecznego adresu pobierania tego dokumentu.")
            bound_client = profile.librus

        # Pobieranie może trwać długo. Nie trzymaj blokady całej sesji,
        # aby inne zakładki działały podczas transferu.
        try:
            attachment = await bound_client.download_school_file(path)
        except Exception as exc:
            raise _upstream_error(exc, "pobrać dokumentu") from exc

        # Nie zwracaj danych ze starego profilu, jeśli w trakcie zmieniono konto.
        async with session.lock:
            assert_session_current(request, session)
            if session.active_profile_id != profile_id or session.active.librus is not bound_client:
                raise HTTPException(409, "W trakcie pobierania zmieniono profil. Spróbuj ponownie.")
        return _attachment_response(attachment, record)
