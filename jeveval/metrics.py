from __future__ import annotations

import math
from statistics import mean
from typing import Any

from .models import RunRecord


def percentile(values: list[float], quantile: float) -> float | None:
    if not values:
        return None
    ordered = sorted(values)
    position = (len(ordered) - 1) * quantile
    lower, upper = math.floor(position), math.ceil(position)
    if lower == upper:
        return ordered[lower]
    return ordered[lower] * (upper - position) + ordered[upper] * (position - lower)


def expected_calibration_error(records: list[RunRecord], bins: int = 10) -> float | None:
    scored = [record for record in records if record.error is None and record.gold is not None and record.prediction is not None]
    if not scored:
        return None
    total = len(scored)
    result = 0.0
    for bin_index in range(bins):
        low, high = bin_index / bins, (bin_index + 1) / bins
        bucket: list[tuple[float, float]] = []
        for record in scored:
            confidence = record.probabilities.get(record.prediction or "", 0.0)
            if low <= confidence < high or (bin_index == bins - 1 and confidence == 1.0):
                bucket.append((confidence, float(record.prediction == record.gold)))
        if bucket:
            result += len(bucket) / total * abs(mean(item[1] for item in bucket) - mean(item[0] for item in bucket))
    return result


def multiclass_brier(records: list[RunRecord]) -> float | None:
    values: list[float] = []
    for record in records:
        if record.error is not None or record.gold is None:
            continue
        values.append(sum((probability - float(label == record.gold)) ** 2 for label, probability in record.probabilities.items()))
    return mean(values) if values else None


def summarize(records: list[RunRecord]) -> dict[str, Any]:
    scored = [record for record in records if record.error is None and record.gold is not None and record.prediction is not None]
    successful = [record for record in records if record.error is None and record.prediction is not None]
    accurate = sum(record.prediction == record.gold for record in scored)
    latencies = [record.latency_seconds for record in successful if record.latency_seconds is not None]
    known_costs = [record.usage.cost_usd for record in successful if record.usage.cost_usd is not None]
    total_cost = sum(known_costs) if known_costs else None
    return {
        "model": records[0].model if records else None,
        "benchmark": records[0].benchmark if records else None,
        "accurate": accurate,
        "total": len(scored),
        "accuracy": accurate / len(scored) if scored else None,
        "ece_10": expected_calibration_error(records, 10),
        "brier": multiclass_brier(records),
        "latency_mean_seconds": mean(latencies) if latencies else None,
        "latency_p50_seconds": percentile(latencies, 0.50),
        "latency_p95_seconds": percentile(latencies, 0.95),
        "input_tokens": sum(record.usage.input_tokens for record in successful),
        "output_tokens": sum(record.usage.output_tokens for record in successful),
        "cost_usd": total_cost,
        "cost_per_1k_examples_usd": total_cost / len(successful) * 1000 if total_cost is not None and successful else None,
        "successful": len(successful),
        "failed": sum(record.error is not None for record in records),
        "unscored": sum(record.error is None and record.gold is None for record in records),
        "cached": sum(record.cached for record in successful),
    }


def metadata_slices(records: list[RunRecord], fields: tuple[str, ...] = ("depth", "rung", "subtask", "family", "tier")) -> list[dict[str, Any]]:
    slices: list[dict[str, Any]] = []
    for field in fields:
        values = sorted({str(record.metadata[field]) for record in records if record.metadata.get(field) is not None})
        for value in values:
            selected = [record for record in records if str(record.metadata.get(field)) == value]
            summary = summarize(selected)
            summary.update({"slice_field": field, "slice_value": value})
            slices.append(summary)
    return slices

