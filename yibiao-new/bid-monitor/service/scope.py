from __future__ import annotations

import re
from pathlib import Path


_USER_ID_PATTERN = re.compile(r"^[1-9][0-9]{0,18}$")


def safe_user_id(value: int | str) -> str:
    """把外部用户标识限制为正整数目录名。"""
    if isinstance(value, bool):
        raise ValueError("user id must be a positive integer")
    normalized = str(value).strip()
    if not _USER_ID_PATTERN.fullmatch(normalized):
        raise ValueError("user id must be a positive integer")
    return normalized


def _safe_child_path(root: Path, user_id: int | str) -> Path:
    """构造并校验用户目录，阻止绝对路径和路径穿越。"""
    root_path = Path(root).expanduser().resolve()
    user_path = (root_path / safe_user_id(user_id)).resolve()
    if not user_path.is_relative_to(root_path):
        raise ValueError("user data path escapes data root")
    user_path.mkdir(parents=True, exist_ok=True)
    return user_path


def user_data_path(root: str | Path, user_id: int | str) -> Path:
    """返回指定用户的持久化目录。"""
    return _safe_child_path(Path(root), user_id)


def user_db_path(root: str | Path, user_id: int | str) -> Path:
    """返回指定用户的 SQLite 数据库路径。"""
    return user_data_path(root, user_id) / "bids.db"
