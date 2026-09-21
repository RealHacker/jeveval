from __future__ import annotations

import json
from pathlib import Path
from typing import Any


def _display(value: Any, digits: int = 4) -> str:
    if value is None:
        return "n/a"
    if isinstance(value, float):
        return f"{value:.{digits}f}"
    return str(value)


def write_summary(output_dir: Path, summaries: list[dict[str, Any]], slices: list[dict[str, Any]]) -> None:
    output_dir.mkdir(parents=True, exist_ok=True)
    payload = {"results": summaries, "slices": slices}
    (output_dir / "summary.json").write_text(json.dumps(payload, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    lines = [
        "# JevEval Summary", "",
        "| Model | Benchmark | Accuracy | Correct/Total | ECE | Brier | Mean latency (s) | p95 (s) | Cost USD | Failed |",
        "|---|---|---:|---:|---:|---:|---:|---:|---:|---:|",
    ]
    for item in summaries:
        lines.append(
            f"| {item['model']} | {item['benchmark']} | {_display(item['accuracy'])} | "
            f"{item['accurate']}/{item['total']} | {_display(item['ece_10'])} | {_display(item['brier'])} | "
            f"{_display(item['latency_mean_seconds'])} | {_display(item['latency_p95_seconds'])} | "
            f"{_display(item['cost_usd'], 6)} | {item['failed']} |"
        )
    if slices:
        lines += ["", "## Metadata slices", "", "Detailed slice metrics are available in `summary.json`."]
    (output_dir / "summary.md").write_text("\n".join(lines) + "\n", encoding="utf-8")

