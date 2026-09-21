from __future__ import annotations

from abc import ABC, abstractmethod
from collections.abc import Iterator
from pathlib import Path

from ..models import Example


class BenchmarkDataError(ValueError):
    """Raised when supplied benchmark data cannot support a valid scored run."""


class BenchmarkLoader(ABC):
    name: str
    source_path: Path

    @property
    @abstractmethod
    def label_space(self) -> list[str]:
        """All labels valid across this benchmark."""

    @abstractmethod
    def load(self, n: int | None = None) -> Iterator[Example]:
        """Yield normalized examples, optionally capped at n."""


def take(iterator: Iterator[Example], n: int | None) -> Iterator[Example]:
    if n is not None and n < 0:
        raise ValueError("n must be non-negative or None")
    for index, example in enumerate(iterator):
        if n is not None and index >= n:
            break
        yield example

