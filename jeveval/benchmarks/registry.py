from __future__ import annotations

from pathlib import Path

from .base import BenchmarkLoader
from .loaders import (
    CladderLoader,
    CommonsenseQALoader,
    FolioLoader,
    JevBenchLoader,
    MusrLoader,
    ProofWriterLoader,
    WinograndeLoader,
)


def build_benchmarks(dataset_root: Path) -> dict[str, BenchmarkLoader]:
    loaders: list[BenchmarkLoader] = [
        FolioLoader(dataset_root / "folio" / "folio-validation.jsonl"),
        ProofWriterLoader(dataset_root / "proofwriter" / "data-test.jsonl"),
        WinograndeLoader(dataset_root / "winogrande" / "data.jsonl"),
        CommonsenseQALoader(dataset_root / "commonsenseQA" / "dev_rand_split.jsonl"),
        MusrLoader(dataset_root / "musr" / "object_placements.json", "object_placements"),
        MusrLoader(dataset_root / "musr" / "team_allocation.json", "team_allocation"),
        CladderLoader(dataset_root / "cladder" / "cladder-v1-q-easy.json", "easy"),
        CladderLoader(dataset_root / "cladder" / "cladder-v1-q-balanced.json", "balanced"),
        CladderLoader(dataset_root / "cladder" / "cladder-v1-q-hard.json", "hard"),
        JevBenchLoader(dataset_root / "jevbench" / "easy.jsonl", "easy"),
        JevBenchLoader(dataset_root / "jevbench" / "original.jsonl", "original"),
        JevBenchLoader(dataset_root / "jevbench" / "hard.jsonl", "hard"),
    ]
    return {loader.name: loader for loader in loaders}


BENCHMARK_GROUPS = {
    "musr": ["musr-object-placements", "musr-team-allocation"],
    "cladder": ["cladder-easy", "cladder-balanced", "cladder-hard"],
    "jevbench": ["jevbench-easy", "jevbench-original", "jevbench-hard"],
}


def resolve_benchmark_names(selection: str, available: dict[str, BenchmarkLoader]) -> list[str]:
    requested = [item.strip().lower() for item in selection.split(",") if item.strip()]
    if not requested:
        raise ValueError("At least one benchmark must be selected")
    if "all" in requested:
        return sorted(available)
    resolved: list[str] = []
    for name in requested:
        candidates = BENCHMARK_GROUPS.get(name, [name])
        for candidate in candidates:
            if candidate not in available:
                raise ValueError(f"Unknown benchmark {name!r}; available: {', '.join(sorted(available))}")
            if candidate not in resolved:
                resolved.append(candidate)
    return resolved

