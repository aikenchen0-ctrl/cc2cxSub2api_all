import base64
import hashlib
import hmac
import json
import time

import pytest
from fastapi import HTTPException

import auth


def _encode(value: bytes) -> str:
    return base64.urlsafe_b64encode(value).decode().rstrip("=")


def _ticket(secret: str, **overrides: object) -> str:
    payload = {
        "iss": "sub2api",
        "aud": "screen2code",
        "sub": "42",
        "jti": "nonce-1",
        "iat": int(time.time()),
        "exp": int(time.time()) + 120,
        "email": "user@example.com",
    }
    payload.update(overrides)
    encoded = _encode(json.dumps(payload, separators=(",", ":")).encode())
    signature = _encode(hmac.new(secret.encode(), encoded.encode(), hashlib.sha256).digest())
    return f"{encoded}.{signature}"


def test_ticket_is_verified_and_replay_is_rejected(monkeypatch, tmp_path):
    secret = "s" * 32
    monkeypatch.setenv("SUB2API_SSO_SECRET", secret)
    monkeypatch.setenv("SCREEN2CODE_AUTH_DB", str(tmp_path / "auth.db"))

    payload = auth.verify_ticket(_ticket(secret))
    identity = auth.consume_ticket(payload)
    assert identity.subject == "42"
    assert identity.email == "user@example.com"

    with pytest.raises(HTTPException) as exc_info:
        auth.consume_ticket(payload)
    assert exc_info.value.status_code == 400
    assert exc_info.value.detail == "SSO ticket already used"


def test_identity_is_reused_for_same_issuer_and_subject(monkeypatch, tmp_path):
    secret = "s" * 32
    monkeypatch.setenv("SUB2API_SSO_SECRET", secret)
    monkeypatch.setenv("SCREEN2CODE_AUTH_DB", str(tmp_path / "auth.db"))

    first = auth.consume_ticket(auth.verify_ticket(_ticket(secret, jti="one", displayName="First")))
    second = auth.consume_ticket(auth.verify_ticket(_ticket(secret, jti="two", displayName="Second")))
    assert first.user_id == second.user_id
    assert second.display_name == "Second"


@pytest.mark.parametrize("value", ["/", "/editor?tab=code", "//evil.example", "https://evil.example", r"/a\\b"])
def test_safe_next_rejects_external_targets(value):
    result = auth._safe_next(value)
    if value.startswith("/") and not value.startswith("//") and "\\" not in value:
        assert result == value
    else:
        assert result == "/"
