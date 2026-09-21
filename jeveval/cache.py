from __future__ import annotations

import hashlib
import json
import os
from pathlib import Path

from .adapters.base import ModelAdapter
from .models import Example, Prediction


class PredictionCache:
    def __init__(self, root: Path):
        self.root = root

    @staticmethod
    def key(adapter: ModelAdapter, example: Example) -> str:
        identity = {
            "version": 1,
            "adapter": adapter.fingerprint,
            "benchmark": example.benchmark,
            "example_id": example.id,
            "state": example.state,
            "instructions": example.instructions,
            "choices": example.choices,
            "question_type": example.question_type,
        }
        encoded = json.dumps(identity, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode()
        return hashlib.sha256(encoded).hexdigest()

    def _path(self, key: str) -> Path:
        return self.root / key[:2] / f"{key}.json"

    def get(self, adapter: ModelAdapter, example: Example) -> Prediction | None:
        path = self._path(self.key(adapter, example))
        if not path.exists():
            return None
        prediction = Prediction.from_dict(json.loads(path.read_text(encoding="utf-8")))
        prediction.cached = True
        return prediction

    def put(self, adapter: ModelAdapter, example: Example, prediction: Prediction) -> None:
        path = self._path(self.key(adapter, example))
        path.parent.mkdir(parents=True, exist_ok=True)
        temporary = path.with_suffix(f".{os.getpid()}.tmp")
        temporary.write_text(json.dumps(prediction.to_dict(), ensure_ascii=False, separators=(",", ":")), encoding="utf-8")
        os.replace(temporary, path)

