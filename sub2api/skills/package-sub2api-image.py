#!/usr/bin/env python3
"""Package the sub2api-image skill into the archive served by the API key page."""

from __future__ import annotations

import argparse
import hashlib
import io
import sys
import zipfile
from pathlib import Path

SKILL_NAME = "sub2api-image"
REPO_ROOT = Path(__file__).resolve().parent.parent
SOURCE_DIR = REPO_ROOT / "skills" / SKILL_NAME
DEFAULT_OUTPUT = REPO_ROOT / "frontend" / "public" / "downloads" / "genimg-skill.zip"

# Explicit allowlist: tests, caches, generated images and credentials never ship.
FILES = (
    "SKILL.md",
    "INSTALL.md",
    "agents/openai.yaml",
    "scripts/image_api.py",
)
FIXED_TIME = (2020, 1, 1, 0, 0, 0)
FILE_MODE = 0o644


def normalize_newlines(data: bytes) -> bytes:
    """Keep the archive byte-stable when Git checks files out with CRLF."""
    return data.replace(b"\r\n", b"\n")


def read_sources() -> dict[str, bytes]:
    missing = [rel for rel in FILES if not (SOURCE_DIR / rel).is_file()]
    if missing:
        raise SystemExit("missing skill source file(s): " + ", ".join(missing))
    return {rel: normalize_newlines((SOURCE_DIR / rel).read_bytes()) for rel in FILES}


def build_archive(files: dict[str, bytes]) -> bytes:
    buffer = io.BytesIO()
    with zipfile.ZipFile(buffer, "w", zipfile.ZIP_DEFLATED, compresslevel=9) as archive:
        for rel in FILES:
            info = zipfile.ZipInfo(f"{SKILL_NAME}/{rel}", date_time=FIXED_TIME)
            info.compress_type = zipfile.ZIP_DEFLATED
            info.external_attr = FILE_MODE << 16
            archive.writestr(info, files[rel])
    return buffer.getvalue()


def read_archive(path: Path) -> dict[str, bytes]:
    with zipfile.ZipFile(path) as archive:
        names = [name for name in archive.namelist() if not name.endswith("/")]
        return {name: archive.read(name) for name in names}


def describe_drift(expected: dict[str, bytes], actual: dict[str, bytes]) -> list[str]:
    problems: list[str] = []
    for name in sorted(expected.keys() - actual.keys()):
        problems.append(f"missing in archive: {name}")
    for name in sorted(actual.keys() - expected.keys()):
        problems.append(f"unexpected file in archive: {name}")
    for name in sorted(expected.keys() & actual.keys()):
        if expected[name] != actual[name]:
            problems.append(f"content differs: {name}")
    return problems


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--check",
        action="store_true",
        help="verify the archive matches the sources instead of writing it",
    )
    parser.add_argument(
        "--out",
        type=Path,
        default=DEFAULT_OUTPUT,
        help="archive path (default: %(default)s)",
    )
    args = parser.parse_args()

    sources = read_sources()
    expected = {f"{SKILL_NAME}/{rel}": data for rel, data in sources.items()}

    if args.check:
        if not args.out.is_file():
            print(f"missing archive: {args.out}", file=sys.stderr)
            print("run: python skills/package-sub2api-image.py", file=sys.stderr)
            return 1
        try:
            actual = read_archive(args.out)
        except (zipfile.BadZipFile, OSError) as exc:
            print(f"unreadable archive {args.out}: {exc}", file=sys.stderr)
            print("run: python skills/package-sub2api-image.py", file=sys.stderr)
            return 1
        problems = describe_drift(expected, actual)
        if problems:
            print(f"{args.out} is out of date:", file=sys.stderr)
            for problem in problems:
                print(f"  - {problem}", file=sys.stderr)
            print("run: python skills/package-sub2api-image.py", file=sys.stderr)
            return 1
        print(f"ok: {args.out} matches skills/{SKILL_NAME}")
        return 0

    payload = build_archive(sources)
    args.out.parent.mkdir(parents=True, exist_ok=True)
    args.out.write_bytes(payload)
    digest = hashlib.sha256(payload).hexdigest()
    print(f"wrote {args.out} ({len(payload)} bytes, sha256 {digest})")
    return 0


if __name__ == "__main__":
    sys.exit(main())
