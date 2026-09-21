from __future__ import annotations

from abc import ABC, abstractmethod
from dataclasses import dataclass

from ..models import Example, Prediction


class ModelError(RuntimeError):
    pass


class MalformedModelError(ModelError):
    """Raised when a provider response cannot satisfy the requested label schema."""


@dataclass(slots=True)
class TransientModelError(ModelError):
    message: str
    retry_after: float | None = None

    def __str__(self) -> str:
        return self.message


class ModelAdapter(ABC):
    name: str

    @property
    @abstractmethod
    def fingerprint(self) -> str:
        """Stable non-secret adapter configuration fingerprint."""

    @abstractmethod
    async def predict(self, example: Example) -> Prediction:
        pass

    async def aclose(self) -> None:
        pass


def normalize_probabilities(probabilities: dict[str, float], labels: list[str]) -> dict[str, float]:
    if set(probabilities) != set(labels):
        missing = sorted(set(labels) - set(probabilities))
        extra = sorted(set(probabilities) - set(labels))
        raise ModelError(f"Probability labels do not match schema (missing={missing}, extra={extra})")
    try:
        values = {label: float(probabilities[label]) for label in labels}
    except (TypeError, ValueError) as exc:
        raise ModelError("Probabilities must be numeric") from exc
    if any(value < 0 or value != value for value in values.values()):
        raise ModelError("Probabilities must be finite and non-negative")
    total = sum(values.values())
    if total <= 0:
        raise ModelError("Probability sum must be positive")
    return {label: value / total for label, value in values.items()}

