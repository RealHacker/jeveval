from __future__ import annotations

from dataclasses import asdict, dataclass, field
from typing import Any


JsonValue = None | bool | int | float | str | list["JsonValue"] | dict[str, "JsonValue"]


@dataclass(slots=True)
class Example:
    id: str
    benchmark: str
    state: JsonValue
    instructions: JsonValue
    choices: dict[str, JsonValue]
    gold_label: str | None
    metadata: dict[str, JsonValue] = field(default_factory=dict)
    question_type: str = "choice"

    @property
    def labels(self) -> list[str]:
        return list(self.choices)

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)

    @classmethod
    def from_dict(cls, value: dict[str, Any]) -> "Example":
        return cls(**value)


@dataclass(slots=True)
class Usage:
    input_tokens: int = 0
    output_tokens: int = 0
    cost_usd: float | None = None


@dataclass(slots=True)
class Prediction:
    label: str
    probabilities: dict[str, float]
    latency_seconds: float
    model: str
    usage: Usage = field(default_factory=Usage)
    raw_response: JsonValue = None
    cached: bool = False

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)

    @classmethod
    def from_dict(cls, value: dict[str, Any]) -> "Prediction":
        data = dict(value)
        usage = data.get("usage", {})
        data["usage"] = usage if isinstance(usage, Usage) else Usage(**usage)
        return cls(**data)


@dataclass(slots=True)
class RunRecord:
    benchmark: str
    model: str
    example_id: str
    question: JsonValue
    gold: str | None
    prediction: str | None
    probabilities: dict[str, float]
    latency_seconds: float | None
    usage: Usage = field(default_factory=Usage)
    metadata: dict[str, JsonValue] = field(default_factory=dict)
    cached: bool = False
    error: str | None = None

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)

