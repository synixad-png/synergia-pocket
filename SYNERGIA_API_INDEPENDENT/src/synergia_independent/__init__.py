"""SYNERGIA API — independent experimental client, not an official SDK."""
from .client import (
    SynergiaClient, SynergiaError, AuthenticationRequired, CaptchaRequired,
    MessagesUnavailable, HTTPError, NetworkError, ProtocolError, MessageWriteUncertain,
)
from .models import Grade, Lesson, MessagePreview, Contact, SentMessage

__all__ = [
    "SynergiaClient", "SynergiaError", "AuthenticationRequired",
    "CaptchaRequired", "MessagesUnavailable", "HTTPError",
    "NetworkError", "ProtocolError", "MessageWriteUncertain", "Grade", "Lesson",
    "MessagePreview", "Contact", "SentMessage",
]
