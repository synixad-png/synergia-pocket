"""SYNERGIA API — independent experimental client, not an official SDK."""
from .client import (
    SynergiaClient, SynergiaError, AuthenticationRequired, CaptchaRequired,
    MessagesUnavailable, HTTPError, NetworkError, ProtocolError,
)
from .models import Grade, Lesson, MessagePreview

__all__ = [
    "SynergiaClient", "SynergiaError", "AuthenticationRequired",
    "CaptchaRequired", "MessagesUnavailable", "HTTPError",
    "NetworkError", "ProtocolError", "Grade", "Lesson", "MessagePreview",
]
