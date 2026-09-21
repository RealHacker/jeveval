from __future__ import annotations

import hashlib
import json
import os
import time
from typing import Any

import httpx

from ..models import Example, Prediction, Usage
from .base import MalformedModelError, ModelAdapter, ModelError, normalize_probabilities
from .http import post_json


class OpenAICompatibleAdapter(ModelAdapter):
    def __init__(
        self,
        *,
        name: str,
        model: str,
        base_url: str,
        api_key_env: str = "OPENAI_API_KEY",
        endpoint_path: str = "/chat/completions",
        timeout: float = 120.0,
        structured_outputs: bool = True,
        reasoning_effort: str = "none",
        input_price_per_million: float | None = None,
        output_price_per_million: float | None = None,
        extra_headers: dict[str, str] | None = None,
    ):
        self.name = name
        self.model = model
        self.base_url = base_url.rstrip("/")
        self.endpoint_path = "/" + endpoint_path.lstrip("/")
        self.api_key_env = api_key_env
        self.structured_outputs = structured_outputs
        if reasoning_effort != "none":
            raise ValueError("JevEval reference models require reasoning_effort='none' for a fair System One comparison")
        self.reasoning_effort = reasoning_effort
        self.input_price_per_million = input_price_per_million
        self.output_price_per_million = output_price_per_million
        self.extra_headers = extra_headers or {}
        self.client = httpx.AsyncClient(timeout=timeout)

    @property
    def fingerprint(self) -> str:
        value = json.dumps({
            "type": "openai_compatible", "model": self.model, "base_url": self.base_url,
            "endpoint_path": self.endpoint_path, "structured_outputs": self.structured_outputs,
            "reasoning_effort": self.reasoning_effort,
        }, sort_keys=True)
        return hashlib.sha256(value.encode()).hexdigest()

    @staticmethod
    def _schema(labels: list[str]) -> dict[str, Any]:
        probability_properties = {label: {"type": "number", "minimum": 0, "maximum": 1} for label in labels}
        return {
            "name": "system_one_decision",
            "strict": True,
            "schema": {
                "type": "object",
                "properties": {
                    "label": {"type": "string", "enum": labels},
                    "probabilities": {
                        "type": "object", "properties": probability_properties,
                        "required": labels, "additionalProperties": False,
                    },
                },
                "required": ["label", "probabilities"],
                "additionalProperties": False,
            },
        }

    @staticmethod
    def _content(message: dict[str, Any]) -> str:
        content = message.get("content", "")
        if isinstance(content, str):
            return content
        if isinstance(content, list):
            return "".join(str(part.get("text", "")) for part in content if isinstance(part, dict))
        return str(content)

    @staticmethod
    def _parse_probabilities(data: dict[str, Any], labels: list[str]) -> tuple[str, dict[str, float]]:
        try:
            content = OpenAICompatibleAdapter._content(data["choices"][0]["message"])
            decision = json.loads(content)
            if not isinstance(decision, dict):
                raise TypeError("decision must be a JSON object")
            probabilities = normalize_probabilities(decision["probabilities"], labels)
            label = decision["label"]
            if label not in labels:
                raise ValueError(f"label {label!r} is not in the allowed labels")
            return content, probabilities
        except (KeyError, IndexError, TypeError, ValueError, json.JSONDecodeError, ModelError) as exc:
            try:
                content = OpenAICompatibleAdapter._content(data["choices"][0]["message"])
            except (KeyError, IndexError, TypeError):
                content = "<missing assistant content>"
            snippet = content[:500].replace("\n", " ")
            raise MalformedModelError(
                "response did not match the required object with top-level 'label' and 'probabilities' fields; "
                f"content={snippet!r}"
            ) from exc

    @staticmethod
    def _usage(data: dict[str, Any]) -> tuple[int, int]:
        usage = data.get("usage") or {}
        return (
            int(usage.get("prompt_tokens", usage.get("input_tokens", 0))),
            int(usage.get("completion_tokens", usage.get("output_tokens", 0))),
        )

    async def predict(self, example: Example) -> Prediction:
        key = os.environ.get(self.api_key_env)
        if not key:
            raise ModelError(f"Environment variable {self.api_key_env} is required for model {self.name!r}")
        schema = self._schema(example.labels)
        schema_text = json.dumps(schema["schema"], ensure_ascii=False, separators=(",", ":"))
        system = (
            "You are a calibrated classification system. Return exactly one JSON object and no other text. "
            "Assign a probability to every label; probabilities must sum to 1. "
            f"The exact required JSON Schema is: {schema_text}"
        )
        user = json.dumps({
            "state": example.state, "instructions": example.instructions,
            "criteria": example.choices, "allowed_labels": example.labels,
        }, ensure_ascii=False)
        messages: list[dict[str, str]] = [
            {"role": "system", "content": system},
            {"role": "user", "content": user},
        ]
        payload: dict[str, Any] = {
            "model": self.model,
            "messages": messages,
            "temperature": 0,
            "reasoning_effort": self.reasoning_effort,
        }
        if self.structured_outputs:
            payload["response_format"] = {"type": "json_schema", "json_schema": schema}
        else:
            payload["response_format"] = {"type": "json_object"}
        headers = {"Authorization": f"Bearer {key}", "Content-Type": "application/json", **self.extra_headers}
        started = time.perf_counter()
        attempts: list[dict[str, Any]] = []
        probabilities: dict[str, float] | None = None
        for format_attempt in range(2):
            payload["messages"] = messages
            data = await post_json(self.client, self.base_url + self.endpoint_path, headers=headers, payload=payload)
            attempts.append(data)
            try:
                _, probabilities = self._parse_probabilities(data, example.labels)
                break
            except MalformedModelError as exc:
                if format_attempt == 1:
                    raise MalformedModelError(
                        f"model {self.name!r} returned malformed structured output after one corrective retry: {exc}"
                    ) from exc
                previous_content = self._content(data.get("choices", [{}])[0].get("message", {}))
                correction = (
                    "Your previous response did not match the required schema. Return only a JSON object with exactly "
                    "two top-level fields: 'label' and 'probabilities'. Do not return a flat label-to-probability map. "
                    f"The exact JSON Schema is: {schema_text}"
                )
                messages = [
                    *messages,
                    {"role": "assistant", "content": previous_content},
                    {"role": "user", "content": correction},
                ]
        if probabilities is None:  # pragma: no cover
            raise MalformedModelError("structured-output correction loop exhausted")
        latency = time.perf_counter() - started
        label = max(probabilities, key=probabilities.get)  # type: ignore[arg-type]
        usage_counts = [self._usage(attempt) for attempt in attempts]
        input_tokens = sum(item[0] for item in usage_counts)
        output_tokens = sum(item[1] for item in usage_counts)
        cost: float | None = None
        if self.input_price_per_million is not None or self.output_price_per_million is not None:
            cost = input_tokens * (self.input_price_per_million or 0) / 1_000_000
            cost += output_tokens * (self.output_price_per_million or 0) / 1_000_000
        raw_response: Any = data if len(attempts) == 1 else {"attempts": attempts}
        return Prediction(label, probabilities, latency, str(data.get("model", self.model)), Usage(input_tokens, output_tokens, cost), raw_response)

    async def aclose(self) -> None:
        await self.client.aclose()

