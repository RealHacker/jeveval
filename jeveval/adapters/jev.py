from __future__ import annotations

import hashlib
import json
import os
import time
from typing import Any

import httpx

from ..models import Example, Prediction, Usage
from .base import ModelAdapter, ModelError, normalize_probabilities
from .http import post_json


class JevAdapter(ModelAdapter):
    def __init__(
        self,
        *,
        name: str = "jev",
        model: str = "jev-latest",
        base_url: str = "https://api.typesafe.ai",
        api_key_env: str = "TYPESAFE_API_KEY",
        timeout: float = 60.0,
        input_price_per_million: float = 0.042,
    ):
        self.name = name
        self.model = model
        self.base_url = base_url.rstrip("/")
        self.api_key_env = api_key_env
        self.input_price_per_million = input_price_per_million
        self.client = httpx.AsyncClient(timeout=timeout)

    @property
    def fingerprint(self) -> str:
        value = json.dumps({"type": "jev", "model": self.model, "base_url": self.base_url}, sort_keys=True)
        return hashlib.sha256(value.encode()).hexdigest()

    @staticmethod
    def _noul_labels(example: Example) -> tuple[str, str] | None:
        positive = next((label for label in example.labels if label.casefold() in {"yes", "true"}), None)
        negative = next((label for label in example.labels if label.casefold() in {"no", "false"}), None)
        return (positive, negative) if positive and negative and len(example.labels) == 2 else None

    def _question(self, example: Example) -> tuple[dict[str, Any], tuple[str, str] | None]:
        noul_labels = self._noul_labels(example) if example.question_type == "noul" else None
        if noul_labels:
            positive, negative = noul_labels
            return {
                "type": "noul",
                "instructions": example.instructions,
                "criteria": {"true": example.choices.get(positive), "false": example.choices.get(negative)},
            }, noul_labels
        return {
            "type": "choice",
            "instructions": example.instructions,
            "criteria": example.choices,
        }, None

    async def predict(self, example: Example) -> Prediction:
        key = os.environ.get(self.api_key_env)
        if not key:
            raise ModelError(f"Environment variable {self.api_key_env} is required for model {self.name!r}")
        question, noul_labels = self._question(example)
        payload = {"state": example.state, "model": self.model, "questions": {"answer": question}}
        started = time.perf_counter()
        data = await post_json(
            self.client,
            f"{self.base_url}/v1/systemone",
            headers={"Authorization": f"Bearer {key}", "Content-Type": "application/json"},
            payload=payload,
        )
        latency = time.perf_counter() - started
        try:
            answer = data["answers"]["answer"]
            if noul_labels:
                positive, negative = noul_labels
                yes_probability = float(answer["noul"])
                probabilities = {positive: yes_probability, negative: 1.0 - yes_probability}
            else:
                probabilities = answer["probabilities"]
            probabilities = normalize_probabilities(probabilities, example.labels)
        except (KeyError, TypeError, ValueError) as exc:
            raise ModelError(f"Unexpected Jev response shape: {data}") from exc
        label = max(probabilities, key=probabilities.get)  # type: ignore[arg-type]
        usage_data = data.get("usage") or {}
        input_tokens = int(usage_data.get("input_tokens", 0))
        usage = Usage(
            input_tokens=input_tokens,
            output_tokens=int(usage_data.get("output_tokens", 0)),
            cost_usd=input_tokens * self.input_price_per_million / 1_000_000,
        )
        return Prediction(label, probabilities, latency, str(data.get("model", self.model)), usage, data)

    async def aclose(self) -> None:
        await self.client.aclose()

