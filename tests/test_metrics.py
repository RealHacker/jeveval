import pytest

from jeveval.metrics import expected_calibration_error, multiclass_brier, percentile, summarize
from jeveval.models import RunRecord


def record(gold: str, prediction: str, probabilities: dict[str, float]) -> RunRecord:
    return RunRecord("bench", "model", gold + prediction, "q", gold, prediction, probabilities, 1.0)


def test_metrics_for_perfect_predictions():
    records = [
        record("A", "A", {"A": 1.0, "B": 0.0}),
        record("B", "B", {"A": 0.0, "B": 1.0}),
    ]
    result = summarize(records)
    assert result["accuracy"] == 1.0
    assert result["accurate"] == 2
    assert expected_calibration_error(records) == 0.0
    assert multiclass_brier(records) == 0.0


def test_brier_and_percentile():
    records = [record("A", "A", {"A": 0.75, "B": 0.25})]
    assert multiclass_brier(records) == pytest.approx(0.125)
    assert percentile([1.0, 2.0, 3.0], 0.95) == pytest.approx(2.9)

