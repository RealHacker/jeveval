from __future__ import annotations

import argparse
import asyncio
import json
import sys
from pathlib import Path
from typing import Any

from .adapters import ModelError
from .benchmarks import build_benchmarks, resolve_benchmark_names
from .config import build_adapters, load_config, resolve_model_names
from .runner import BenchmarkRunner, RunSettings
from .subsets import load_mini, make_all_minis


def _parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="jeveval", description="Benchmark Jev against reference models")
    subparsers = parser.add_subparsers(dest="command", required=True)

    listing = subparsers.add_parser("list", help="List available benchmarks and configured models")
    listing.add_argument("--dataset-root", type=Path, default=Path("datasets"))
    listing.add_argument("--config", type=Path)

    mini = subparsers.add_parser("make-mini", help="Regenerate random mini datasets")
    mini.add_argument("--dataset-root", type=Path, default=Path("datasets"))
    mini.add_argument("--mini-root", type=Path)
    mini.add_argument("--seed", type=int, default=42)
    mini.add_argument("--size", type=int, default=100)

    run = subparsers.add_parser("run", help="Run selected model/benchmark pairs")
    run.add_argument("--config", type=Path)
    run.add_argument("--benchmarks")
    run.add_argument("--models")
    run.add_argument("--mode", help="mini (default), full, or a positive integer sample cap")
    run.add_argument("--dataset-root", type=Path)
    run.add_argument("--mini-root", type=Path)
    run.add_argument("--output-dir", type=Path)
    run.add_argument("--cache-dir", type=Path)
    run.add_argument("--concurrency", type=int)
    run.add_argument("--max-retries", type=int)
    return parser


def _setting(arguments: argparse.Namespace, config: dict[str, Any], name: str, default: Any) -> Any:
    value = getattr(arguments, name, None)
    return value if value is not None else config.get(name.replace("_", "-"), config.get(name, default))


def _examples(mode: str, names: list[str], loaders: dict, mini_root: Path) -> dict[str, list]:
    if mode == "mini":
        return {name: load_mini(name, mini_root) for name in names}
    if mode == "full":
        return {name: list(loaders[name].load()) for name in names}
    try:
        count = int(mode)
    except ValueError as exc:
        raise ValueError("mode must be 'mini', 'full', or a positive integer") from exc
    if count < 1:
        raise ValueError("numeric mode must be positive")
    return {name: list(loaders[name].load(count)) for name in names}


async def _run(arguments: argparse.Namespace) -> int:
    config = load_config(arguments.config)
    dataset_root = Path(_setting(arguments, config, "dataset_root", "datasets"))
    mini_root = Path(_setting(arguments, config, "mini_root", dataset_root / "mini"))
    output_dir = Path(_setting(arguments, config, "output_dir", "outputs"))
    cache_dir = Path(_setting(arguments, config, "cache_dir", output_dir / "cache"))
    loaders = build_benchmarks(dataset_root)
    adapters = build_adapters(config)
    benchmark_selection = _setting(arguments, config, "benchmarks", "all")
    model_selection = _setting(arguments, config, "models_selection", config.get("run", {}).get("models", "all"))
    if arguments.models is not None:
        model_selection = arguments.models
    if arguments.benchmarks is None and isinstance(config.get("run"), dict):
        benchmark_selection = config["run"].get("benchmarks", benchmark_selection)
    mode = str(_setting(arguments, config, "mode", config.get("run", {}).get("mode", "mini")))
    benchmark_names = resolve_benchmark_names(str(benchmark_selection), loaders)
    model_names = resolve_model_names(str(model_selection), adapters)
    examples = _examples(mode, benchmark_names, loaders, mini_root)
    settings = RunSettings(
        concurrency=int(_setting(arguments, config.get("run", {}), "concurrency", 8)),
        max_retries=int(_setting(arguments, config.get("run", {}), "max_retries", 4)),
    )
    runner = BenchmarkRunner(output_dir, cache_dir, settings)
    summaries, _ = await runner.run([adapters[name] for name in model_names], examples)
    print(json.dumps(summaries, indent=2))
    print(f"Wrote reports to {output_dir.resolve()}")
    return 0 if all(item["failed"] == 0 for item in summaries) else 2


def main(argv: list[str] | None = None) -> int:
    arguments = _parser().parse_args(argv)
    try:
        if arguments.command == "list":
            loaders = build_benchmarks(arguments.dataset_root)
            adapters = build_adapters(load_config(arguments.config))
            print("Benchmarks:")
            for name in sorted(loaders):
                print(f"  {name}")
            print("Models:")
            for name in adapters:
                print(f"  {name}")
            return 0
        if arguments.command == "make-mini":
            root = arguments.mini_root or arguments.dataset_root / "mini"
            manifest = make_all_minis(build_benchmarks(arguments.dataset_root), root, seed=arguments.seed, size=arguments.size)
            for item in manifest:
                print(f"{item['benchmark']}: {item['mini_count']}/{item['source_count']} -> {item['path']}")
            return 0
        if arguments.command == "run":
            return asyncio.run(_run(arguments))
    except (OSError, ValueError, ModelError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2
    return 2


if __name__ == "__main__":
    raise SystemExit(main())

