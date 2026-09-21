from __future__ import annotations

import asyncio
import json
import random
from dataclasses import dataclass
from pathlib import Path

from .adapters import MalformedModelError, ModelAdapter, ModelError, TransientModelError
from .cache import PredictionCache
from .metrics import metadata_slices, summarize
from .models import Example, RunRecord, Usage
from .reporting import write_summary


@dataclass(slots=True)
class RunSettings:
    concurrency: int = 8
    max_retries: int = 4
    retry_base_seconds: float = 0.5


class BenchmarkRunner:
    def __init__(self, output_dir: Path, cache_dir: Path, settings: RunSettings):
        if settings.concurrency < 1:
            raise ValueError("concurrency must be at least 1")
        self.output_dir = output_dir
        self.cache = PredictionCache(cache_dir)
        self.settings = settings

    async def _predict(self, adapter: ModelAdapter, example: Example, *, fail_fast_malformed: bool = False) -> RunRecord:
        cached = self.cache.get(adapter, example)
        if cached is not None:
            prediction = cached
        else:
            for attempt in range(self.settings.max_retries + 1):
                try:
                    prediction = await adapter.predict(example)
                    self.cache.put(adapter, example, prediction)
                    break
                except TransientModelError as exc:
                    if attempt >= self.settings.max_retries:
                        return self._error(adapter, example, str(exc))
                    delay = exc.retry_after
                    if delay is None:
                        delay = self.settings.retry_base_seconds * (2**attempt) * random.uniform(0.8, 1.2)
                    await asyncio.sleep(delay)
                except MalformedModelError as exc:
                    if fail_fast_malformed:
                        raise
                    return self._error(adapter, example, str(exc))
                except Exception as exc:
                    return self._error(adapter, example, f"{type(exc).__name__}: {exc}")
            else:  # pragma: no cover
                return self._error(adapter, example, "retry loop exhausted")
        return RunRecord(
            benchmark=example.benchmark, model=adapter.name, example_id=example.id,
            question={"state": example.state, "instructions": example.instructions, "choices": example.choices}, gold=example.gold_label,
            prediction=prediction.label, probabilities=prediction.probabilities,
            latency_seconds=prediction.latency_seconds, usage=prediction.usage,
            metadata=example.metadata, cached=prediction.cached,
        )

    @staticmethod
    def _error(adapter: ModelAdapter, example: Example, error: str) -> RunRecord:
        return RunRecord(
            benchmark=example.benchmark, model=adapter.name, example_id=example.id,
            question={"state": example.state, "instructions": example.instructions, "choices": example.choices}, gold=example.gold_label,
            prediction=None, probabilities={}, latency_seconds=None,
            usage=Usage(), metadata=example.metadata, error=error,
        )

    async def run_pair(self, adapter: ModelAdapter, examples: list[Example]) -> list[RunRecord]:
        if not examples:
            raise ValueError("Benchmark yielded no examples")
        if all(example.gold_label is None for example in examples):
            diagnostic = "Benchmark cannot be scored because the supplied split contains no gold labels."
            records = [self._error(adapter, example, diagnostic) for example in examples]
            raw_dir = self.output_dir / "raw"
            raw_dir.mkdir(parents=True, exist_ok=True)
            path = raw_dir / f"{adapter.name}__{examples[0].benchmark}.jsonl"
            with path.open("w", encoding="utf-8", newline="\n") as handle:
                for record in records:
                    handle.write(json.dumps(record.to_dict(), ensure_ascii=False, separators=(",", ":")) + "\n")
            return records
        semaphore = asyncio.Semaphore(self.settings.concurrency)
        preflight_index = next(
            (index for index, example in enumerate(examples) if self.cache.get(adapter, example) is None),
            None,
        )
        records_by_index: dict[int, RunRecord] = {}
        if preflight_index is not None:
            try:
                records_by_index[preflight_index] = await self._predict(
                    adapter, examples[preflight_index], fail_fast_malformed=True
                )
                if records_by_index[preflight_index].error:
                    raise ModelError(
                        f"Preflight failed for model {adapter.name!r} on benchmark {examples[0].benchmark!r}; "
                        f"the remaining {len(examples) - 1} examples were not started. "
                        f"{records_by_index[preflight_index].error}"
                    )
            except MalformedModelError as exc:
                raise MalformedModelError(
                    f"Preflight failed for model {adapter.name!r} on benchmark {examples[0].benchmark!r}; "
                    f"the remaining {len(examples) - 1} examples were not started. {exc}"
                ) from exc

        async def limited(index: int, example: Example) -> tuple[int, RunRecord]:
            async with semaphore:
                return index, await self._predict(adapter, example)

        remaining = await asyncio.gather(*(
            limited(index, example)
            for index, example in enumerate(examples)
            if index != preflight_index
        ))
        records_by_index.update(remaining)
        records = [records_by_index[index] for index in range(len(examples))]
        raw_dir = self.output_dir / "raw"
        raw_dir.mkdir(parents=True, exist_ok=True)
        path = raw_dir / f"{adapter.name}__{examples[0].benchmark}.jsonl"
        with path.open("w", encoding="utf-8", newline="\n") as handle:
            for record in records:
                handle.write(json.dumps(record.to_dict(), ensure_ascii=False, separators=(",", ":")) + "\n")
        return records

    async def run(self, adapters: list[ModelAdapter], benchmark_examples: dict[str, list[Example]]) -> tuple[list[dict], list[dict]]:
        summaries: list[dict] = []
        slices: list[dict] = []
        try:
            for adapter in adapters:
                for examples in benchmark_examples.values():
                    records = await self.run_pair(adapter, examples)
                    summaries.append(summarize(records))
                    slices.extend(metadata_slices(records))
                    write_summary(self.output_dir, summaries, slices)
        finally:
            await asyncio.gather(*(adapter.aclose() for adapter in adapters), return_exceptions=True)
        return summaries, slices

