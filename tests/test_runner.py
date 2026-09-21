from pathlib import Path

import pytest

from jeveval.adapters.base import MalformedModelError, ModelAdapter, ModelError
from jeveval.models import Example, Prediction
from jeveval.runner import BenchmarkRunner, RunSettings


class CountingAdapter(ModelAdapter):
    name = "counting"

    def __init__(self, malformed: bool = False, provider_error: bool = False):
        self.calls = 0
        self.malformed = malformed
        self.provider_error = provider_error

    @property
    def fingerprint(self) -> str:
        return "counting-v1"

    async def predict(self, example: Example) -> Prediction:
        self.calls += 1
        if self.malformed:
            raise MalformedModelError("bad shape")
        if self.provider_error:
            raise ModelError("provider rejected reasoning_effort")
        return Prediction("yes", {"yes": 1.0, "no": 0.0}, 0.01, self.name)


@pytest.mark.asyncio
async def test_unscored_pair_does_not_call_model(tmp_path: Path):
    adapter = CountingAdapter()
    runner = BenchmarkRunner(tmp_path / "out", tmp_path / "cache", RunSettings())
    example = Example("1", "winogrande", "state", "question", {"1": None, "2": None}, None)
    records = await runner.run_pair(adapter, [example])
    assert adapter.calls == 0
    assert records[0].error and "no gold labels" in records[0].error


@pytest.mark.asyncio
async def test_malformed_preflight_aborts_before_batch(tmp_path: Path):
    adapter = CountingAdapter(malformed=True)
    runner = BenchmarkRunner(tmp_path / "out", tmp_path / "cache", RunSettings(max_retries=4, retry_base_seconds=0))
    examples = [
        Example(str(index), "binary", "state", "question", {"yes": None, "no": None}, "yes")
        for index in range(100)
    ]
    with pytest.raises(MalformedModelError, match="remaining 99 examples were not started"):
        await runner.run_pair(adapter, examples)
    assert adapter.calls == 1


@pytest.mark.asyncio
async def test_provider_error_preflight_aborts_before_batch(tmp_path: Path):
    adapter = CountingAdapter(provider_error=True)
    runner = BenchmarkRunner(tmp_path / "out", tmp_path / "cache", RunSettings(max_retries=4, retry_base_seconds=0))
    examples = [
        Example(str(index), "binary", "state", "question", {"yes": None, "no": None}, "yes")
        for index in range(100)
    ]
    with pytest.raises(ModelError, match="remaining 99 examples were not started"):
        await runner.run_pair(adapter, examples)
    assert adapter.calls == 1


@pytest.mark.asyncio
async def test_successful_preflight_is_reused(tmp_path: Path):
    adapter = CountingAdapter()
    runner = BenchmarkRunner(tmp_path / "out", tmp_path / "cache", RunSettings())
    examples = [
        Example(str(index), "binary", "state", "question", {"yes": None, "no": None}, "yes")
        for index in range(3)
    ]
    records = await runner.run_pair(adapter, examples)
    assert adapter.calls == 3
    assert len(records) == 3
