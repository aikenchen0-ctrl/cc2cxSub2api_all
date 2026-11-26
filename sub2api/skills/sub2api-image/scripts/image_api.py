#!/usr/bin/env python3
"""Standalone Sub2API Images API client; no changes to the text provider."""

import argparse
import base64
import ctypes
import getpass
import ipaddress
import json
import os
from pathlib import Path
import sys
import tempfile
import urllib.error
import urllib.parse
import urllib.request
import uuid

DEFAULT_BASE = "https://api.cc2.cx"
MAX_FILE = 20 * 1024 * 1024
MAX_UPLOAD = 64 * 1024 * 1024
MAX_RESPONSE = 128 * 1024 * 1024


class ImageError(Exception):
    pass


def normalize_base(value):
    value = value.strip().rstrip("/")
    parsed = urllib.parse.urlsplit(value)
    local = parsed.hostname in ("localhost", "127.0.0.1", "::1")
    if (parsed.scheme != "https" and not (parsed.scheme == "http" and local)):
        raise ImageError("Use HTTPS for the image API (HTTP is allowed only on localhost).")
    if not parsed.hostname or parsed.username or parsed.password or parsed.query or parsed.fragment:
        raise ImageError("Invalid image base URL; omit credentials, query strings, and fragments.")
    if parsed.path.endswith(("/images/generations", "/images/edits")):
        raise ImageError("Set the base URL ending in /v1, not a specific image endpoint.")
    return value


def credential_path():
    return Path(os.environ.get("CODEX_HOME") or Path.home() / ".codex") / "credentials" / "sub2api-image.json"


def windows_protect(data, decrypt=False):
    """Use Windows' current-user data protection; never print the plaintext."""
    from ctypes import wintypes

    class Blob(ctypes.Structure):
        _fields_ = [("size", wintypes.DWORD), ("data", ctypes.POINTER(ctypes.c_byte))]

    buffer = ctypes.create_string_buffer(data)
    source = Blob(len(data), ctypes.cast(buffer, ctypes.POINTER(ctypes.c_byte)))
    target = Blob()
    crypto = ctypes.WinDLL("crypt32", use_last_error=True)
    kernel = ctypes.WinDLL("kernel32", use_last_error=True)
    kernel.LocalFree.argtypes = [ctypes.c_void_p]
    kernel.LocalFree.restype = ctypes.c_void_p
    # CRYPTPROTECT_UI_FORBIDDEN, with default current-user scope.
    if decrypt:
        ok = crypto.CryptUnprotectData(ctypes.byref(source), None, None, None, None, 1, ctypes.byref(target))
    else:
        ok = crypto.CryptProtectData(ctypes.byref(source), None, None, None, None, 1, ctypes.byref(target))
    if not ok:
        raise ImageError("Windows credential protection failed for the current user.")
    try:
        return ctypes.string_at(target.data, target.size)
    finally:
        kernel.LocalFree(target.data)


def load_credentials(decrypt_key=True):
    path = credential_path()
    if not path.exists():
        return {}
    try:
        config = json.loads(path.read_text(encoding="utf-8"))
        if not isinstance(config, dict):
            raise ValueError()
        if decrypt_key and "key_dpapi" in config:
            if os.name != "nt":
                raise ImageError("This credential was encrypted on Windows; configure a key on this computer.")
            config["key"] = windows_protect(base64.b64decode(config["key_dpapi"], validate=True), True).decode()
        return config
    except (ValueError, KeyError, UnicodeError, OSError):
        raise ImageError("Image credential file is unreadable; run configure again.") from None


def clean_key(key):
    if not isinstance(key, str) or not key.strip() or any(c.isspace() for c in key.strip()):
        raise ImageError("未配置生图密钥，请先配置生图密钥。")
    return key.strip()


def configure(args):
    base = normalize_base(args.base_url or os.environ.get("SUB2API_IMAGE_BASE_URL") or DEFAULT_BASE)
    key = clean_key(sys.stdin.read() if args.key_stdin else getpass.getpass("请输入生图密钥："))
    config = {"base_url": base}
    if os.name == "nt":
        config["key_dpapi"] = base64.b64encode(windows_protect(key.encode())).decode()
    else:
        config["key"] = key
    path = credential_path()
    path.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    temporary = None
    try:
        with tempfile.NamedTemporaryFile(mode="w", encoding="utf-8", dir=path.parent, delete=False) as stream:
            temporary = Path(stream.name)
            os.chmod(temporary, 0o600)
            json.dump(config, stream)
        os.replace(temporary, path)
    finally:
        if temporary and temporary.exists():
            temporary.unlink()
    print(json.dumps({"configured": True, "base_url": base}))


def provider(base_override=None):
    env_key = os.environ.get("SUB2API_IMAGE_API_KEY", "").strip()
    # A complete explicit override must work even if another machine's saved file is unusable.
    explicit_base = base_override or os.environ.get("SUB2API_IMAGE_BASE_URL")
    config = {} if env_key and explicit_base else load_credentials(decrypt_key=not bool(env_key))
    saved_base = normalize_base(config.get("base_url", DEFAULT_BASE))
    base = normalize_base(explicit_base or saved_base)
    if not env_key and config.get("key") and base != saved_base:
        raise ImageError("Image base URL changed. Supply SUB2API_IMAGE_API_KEY or configure a key for this endpoint.")
    return base, clean_key(env_key or config.get("key", ""))


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        raise ImageError("Redirect rejected. Check the image endpoint; no request was automatically resubmitted.")


def bounded_read(response, maximum):
    data = response.read(maximum + 1)
    if len(data) > maximum:
        raise ImageError("Provider response exceeds the local size limit.")
    return data


def request_bytes(url, *, data=None, content_type=None, key=None, timeout=180, maximum=MAX_RESPONSE):
    headers = {"Accept": "application/json" if data is not None else "image/*",
               "User-Agent": "Sub2API-Image-Skill/1.0"}
    if key:
        headers["Authorization"] = "Bearer " + key
    if content_type:
        headers["Content-Type"] = content_type
    request = urllib.request.Request(url, data=data, headers=headers)
    try:
        with urllib.request.build_opener(NoRedirect()).open(request, timeout=timeout) as response:
            return bounded_read(response, maximum)
    except urllib.error.HTTPError as exc:
        detail = ""
        try:
            body = json.loads(exc.read(8192))
            error = body.get("error", {})
            detail = str(error.get("message", "")) if isinstance(error, dict) else str(error)
        except (ValueError, AttributeError):
            pass
        if key:
            detail = detail.replace(key, "[redacted]")
        detail = " ".join(detail.split())[:500]
        raise ImageError(f"Image service HTTP {exc.code}: {detail or 'request rejected'}") from None
    except (urllib.error.URLError, TimeoutError, ConnectionError, OSError):
        raise ImageError("Connection failed or timed out. Payment/result may be uncertain; check usage before retrying.") from None


def image_type(data):
    if data.startswith(b"\x89PNG\r\n\x1a\n"):
        return "png", "image/png"
    if data.startswith(b"\xff\xd8\xff"):
        return "jpeg", "image/jpeg"
    if data.startswith(b"RIFF") and data[8:12] == b"WEBP":
        return "webp", "image/webp"
    raise ImageError("Expected a PNG, JPEG, or WebP image.")


def read_image(path):
    path = Path(path)
    if not path.is_file() or path.stat().st_size > MAX_FILE:
        raise ImageError("Input image is missing or exceeds 20 MiB.")
    with path.open("rb") as stream:
        data = bounded_read(stream, MAX_FILE)
    return data, image_type(data)[1]


def multipart(fields, images, mask):
    boundary = "sub2api-" + uuid.uuid4().hex
    parts = []
    for name, value in fields.items():
        parts.append(f'--{boundary}\r\nContent-Disposition: form-data; name="{name}"\r\n\r\n{value}\r\n'.encode())
    total = 0
    for index, (name, path) in enumerate([("image[]", path) for path in images] + ([("mask", mask)] if mask else [])):
        data, mime = read_image(path)
        total += len(data)
        if total > MAX_UPLOAD:
            raise ImageError("Combined input images exceed 64 MiB.")
        if name == "mask" and mime != "image/png":
            raise ImageError("Masks must be PNG files with a transparent edit region.")
        extension = mime.split("/")[1]
        parts.append(f'--{boundary}\r\nContent-Disposition: form-data; name="{name}"; filename="input-{index}.{extension}"\r\nContent-Type: {mime}\r\n\r\n'.encode() + data + b"\r\n")
    parts.append(f"--{boundary}--\r\n".encode())
    return b"".join(parts), f"multipart/form-data; boundary={boundary}"


def download_image(url, timeout):
    parsed = urllib.parse.urlsplit(url)
    if parsed.scheme != "https" or not parsed.hostname or parsed.username or parsed.password:
        raise ImageError("Provider image URLs must use HTTPS without embedded credentials.")
    if parsed.hostname == "localhost":
        raise ImageError("Provider returned a local image URL.")
    try:
        address = ipaddress.ip_address(parsed.hostname)
    except ValueError:
        address = None
    if address and not address.is_global:
        raise ImageError("Provider returned a private image URL.")
    # Deliberately no API key, cookies, or other credentials on CDN downloads.
    return request_bytes(url, timeout=timeout, maximum=MAX_FILE * 4)


def output_paths(output, count):
    return [output] if count == 1 else [output.with_name(f"{output.stem}-{i+1}{output.suffix}") for i in range(count)]


def run_image(args):
    base, key = provider(args.base_url)
    prompt = Path(args.prompt_file).read_text(encoding="utf-8-sig") if args.prompt_file else args.prompt
    if not prompt or not prompt.strip():
        raise ImageError("Image prompt is empty.")
    if len(prompt.encode("utf-8")) > 256 * 1024:
        raise ImageError("Image prompt exceeds 256 KiB.")
    output = Path(args.out or f"outputs/sub2api-{uuid.uuid4().hex[:12]}.{args.output_format}")
    candidates = output_paths(output, args.n)
    for path in candidates:
        for extension in (".png", ".jpeg", ".jpg", ".webp"):
            if path.with_suffix(extension).exists():
                raise ImageError("Output already exists. Choose a new --out filename.")
    output.parent.mkdir(parents=True, exist_ok=True)
    fields = {"model": args.model, "prompt": prompt, "size": args.size,
              "quality": args.quality, "n": args.n, "output_format": args.output_format}
    if args.command == "edit":
        data, content_type = multipart(fields, args.image, args.mask)
        endpoint = "/images/edits"
    else:
        data, content_type = json.dumps(fields).encode(), "application/json"
        endpoint = "/images/generations"
    raw = request_bytes(base + endpoint, data=data, content_type=content_type, key=key, timeout=args.timeout)
    try:
        response = json.loads(raw)
        items = response["data"]
        if not isinstance(items, list) or not items or len(items) > args.n:
            raise ValueError()
    except (ValueError, TypeError, KeyError):
        raise ImageError("Provider returned no valid image list. Check usage before resubmitting.") from None
    saved = []
    try:
        for path, item in zip(candidates, items):
            if not isinstance(item, dict):
                raise ImageError("Invalid image response entry.")
            if item.get("b64_json"):
                try:
                    image = base64.b64decode(item["b64_json"], validate=True)
                except (ValueError, TypeError):
                    raise ImageError("Provider returned invalid base64 image data.") from None
            elif item.get("url"):
                image = download_image(item["url"], args.timeout)
            else:
                raise ImageError("Image response contains neither base64 nor a URL.")
            extension, _ = image_type(image)
            path = path.with_suffix("." + extension)
            with path.open("xb") as stream:
                stream.write(image)
            saved.append(str(path.resolve()))
    except Exception:
        if saved:
            print(json.dumps({"partial_files": saved, "resubmit": False}), file=sys.stderr)
        raise
    print(json.dumps({"files": saved, "requested": args.n, "received": len(saved),
                      "model": args.model, "size_requested": args.size}))


def parser():
    root = argparse.ArgumentParser(description=__doc__)
    commands = root.add_subparsers(dest="command", required=True)
    config = commands.add_parser("configure")
    config.add_argument("--base-url")
    config.add_argument("--key-stdin", action="store_true")
    status = commands.add_parser("status")
    status.add_argument("--base-url")
    for command in ("generate", "edit"):
        sub = commands.add_parser(command)
        prompt = sub.add_mutually_exclusive_group(required=True)
        prompt.add_argument("--prompt")
        prompt.add_argument("--prompt-file")
        sub.add_argument("--base-url")
        sub.add_argument("--model", default="gpt-image-2.6-flare")
        sub.add_argument("--size", default="1024x1024")
        sub.add_argument("--quality", choices=("auto", "low", "medium", "high"), default="auto")
        sub.add_argument("--n", type=int, choices=range(1, 5), default=1)
        sub.add_argument("--output-format", choices=("png", "jpeg", "webp"), default="png")
        sub.add_argument("--out")
        sub.add_argument("--timeout", type=int, choices=range(1, 601), default=180, metavar="1..600")
        if command == "edit":
            sub.add_argument("--image", action="append", required=True)
            sub.add_argument("--mask")
    return root


def main():
    args = parser().parse_args()
    try:
        if args.command == "configure":
            configure(args)
        elif args.command == "status":
            base, _ = provider(args.base_url)
            print(json.dumps({"configured": True, "base_url": base}))
        else:
            run_image(args)
        return 0
    except (ImageError, OSError, ValueError) as exc:
        message = str(exc) if isinstance(exc, ImageError) else "Local file or response error. Check paths and usage before retrying."
        print(json.dumps({"error": message}, ensure_ascii=False), file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
