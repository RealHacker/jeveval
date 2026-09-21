from __future__ import annotations

import hashlib
import time

from ..models import Example, Prediction
from .base import ModelAdapter


class DeterministicAdapter(ModelAdapter):
    """Offline smoke-test adapter; it is not intended as a benchmark baseline."""

    def __init__(self, name: str = "deterministic"):
        self.name = name

    @property
    def fingerprint(self) -> str:
        return hashlib.sha256(b"deterministic-v1").hexdigest()

    async def predict(self, example: Example) -> Prediction:
        started = time.perf_counter()
        index = int(hashlib.sha256(example.id.encode()).hexdigest(), 16) % len(example.labels)
        selected = example.labels[index]
        probabilities = {label: 0.0 for label in example.labels}
        probabilities[selected] = 1.0
        return Prediction(selected, probabilities, time.perf_counter() - started, self.name)

