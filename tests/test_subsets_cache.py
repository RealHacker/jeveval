from collections.abc import Iterator
from pathlib import Path

from jeveval.adapters.deterministic import DeterministicAdapter
from jeveval.benchmarks.base import BenchmarkLoader
from jeveval.cache import PredictionCache
from jeveval.models import Example, Prediction
from jeveval.subsets import load_mini, make_mini


class FakeLoader(BenchmarkLoader):
    name = "fake"

    def __init__(self, source_path: Path):
        self.source_path = source_path

    @property
    def label_space(self) -> list[str]:
        return ["a", "b"]

    def load(self, n: int | None = None) -> Iterator[Example]:
        limit = 120 if n is None else min(n, 120)
        for index in range(limit):
            yield Example(str(index), self.name, "state", "question", {"a": None, "b": None}, "a")


def test_mini_is_reproducible(tmp_path):
    source = tmp_path / "source.jsonl"
    source.write_text("fixture", encoding="utf-8")
    loader = FakeLoader(source)
    first = tmp_path / "first"
    second = tmp_path / "second"
    make_mini(loader, first, seed=7)
    make_mini(loader, second, seed=7)
    assert [item.id for item in load_mini("fake", first)] == [item.id for item in load_mini("fake", second)]
    assert len(load_mini("fake", first)) == 100


def test_cache_key_changes_with_prompt(tmp_path):
    adapter = DeterministicAdapter()
    cache = PredictionCache(tmp_path)
    first = Example("1", "b", "s", "q1", {"a": None, "b": None}, "a")
    second = Example("1", "b", "s", "q2", {"a": None, "b": None}, "a")
    prediction = Prediction("a", {"a": 1, "b": 0}, 0.1, "deterministic")
    cache.put(adapter, first, prediction)
    assert cache.get(adapter, first) is not None
    assert cache.get(adapter, second) is None

