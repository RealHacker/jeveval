from __future__ import annotations

import hashlib
import json
import random
from pathlib import Path

from .benchmarks.base import BenchmarkLoader
from .models import Example


def _fingerprint(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def mini_path(mini_root: Path, benchmark: str) -> Path:
    return mini_root / f"{benchmark}.jsonl"


def make_mini(loader: BenchmarkLoader, mini_root: Path, *, seed: int, size: int = 100) -> dict[str, object]:
    examples = list(loader.load())
    rng = random.Random(f"{seed}:{loader.name}")
    selected = rng.sample(examples, size) if len(examples) > size else examples
    mini_root.mkdir(parents=True, exist_ok=True)
    target = mini_path(mini_root, loader.name)
    with target.open("w", encoding="utf-8", newline="\n") as handle:
        for example in selected:
            handle.write(json.dumps(example.to_dict(), ensure_ascii=False, separators=(",", ":")) + "\n")
    return {
        "benchmark": loader.name,
        "source": str(loader.source_path),
        "source_sha256": _fingerprint(loader.source_path),
        "source_count": len(examples),
        "mini_count": len(selected),
        "seed": seed,
        "path": str(target),
    }


def make_all_minis(loaders: dict[str, BenchmarkLoader], mini_root: Path, *, seed: int, size: int = 100) -> list[dict[str, object]]:
    manifest = [make_mini(loader, mini_root, seed=seed, size=size) for loader in loaders.values()]
    (mini_root / "manifest.json").write_text(json.dumps(manifest, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    return manifest


def load_mini(benchmark: str, mini_root: Path, n: int | None = None) -> list[Example]:
    target = mini_path(mini_root, benchmark)
    if not target.exists():
        raise FileNotFoundError(f"Mini dataset is missing for {benchmark!r}; run `jeveval make-mini` first")
    examples: list[Example] = []
    with target.open("r", encoding="utf-8") as handle:
        for line in handle:
            if line.strip():
                examples.append(Example.from_dict(json.loads(line)))
                if n is not None and len(examples) >= n:
                    break
    return examples

