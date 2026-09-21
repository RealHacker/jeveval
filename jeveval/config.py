from __future__ import annotations

import json
import re
from pathlib import Path
from typing import Any

import yaml

from .adapters import DeterministicAdapter, JevAdapter, ModelAdapter, OpenAICompatibleAdapter


def load_config(path: Path | None) -> dict[str, Any]:
    if path is None:
        return {}
    text = path.read_text(encoding="utf-8")
    if path.suffix.lower() == ".json":
        value = json.loads(text)
    else:
        value = yaml.safe_load(text)
    if value is None:
        return {}
    if not isinstance(value, dict):
        raise ValueError("Config root must be an object")
    return value


def build_adapters(config: dict[str, Any]) -> dict[str, ModelAdapter]:
    definitions = config.get("models")
    if definitions is None:
        definitions = {
            "jev": {"type": "jev"},
            "deterministic": {"type": "deterministic"},
        }
    if not isinstance(definitions, dict):
        raise ValueError("config.models must be an object keyed by model name")
    adapters: dict[str, ModelAdapter] = {}
    for name, raw in definitions.items():
        if not re.fullmatch(r"[A-Za-z0-9_.-]+", str(name)):
            raise ValueError(f"Model name {name!r} may contain only letters, digits, dots, underscores, and hyphens")
        if not isinstance(raw, dict):
            raise ValueError(f"Model config for {name!r} must be an object")
        values = dict(raw)
        adapter_type = str(values.pop("type", "openai_compatible")).lower()
        values["name"] = name
        if adapter_type == "jev":
            adapter = JevAdapter(**values)
        elif adapter_type in {"openai", "openai_compatible"}:
            adapter = OpenAICompatibleAdapter(**values)
        elif adapter_type == "deterministic":
            adapter = DeterministicAdapter(**values)
        else:
            raise ValueError(f"Unknown adapter type {adapter_type!r} for model {name!r}")
        adapters[name] = adapter
    return adapters


def resolve_model_names(selection: str, available: dict[str, ModelAdapter]) -> list[str]:
    names = [part.strip() for part in selection.split(",") if part.strip()]
    if not names:
        raise ValueError("At least one model must be selected")
    if "all" in names:
        return list(available)
    unknown = [name for name in names if name not in available]
    if unknown:
        raise ValueError(f"Unknown model(s) {', '.join(unknown)}; available: {', '.join(available)}")
    return list(dict.fromkeys(names))

