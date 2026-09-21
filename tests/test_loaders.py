from pathlib import Path

from jeveval.benchmarks import build_benchmarks


ROOT = Path(__file__).parents[1]


def test_every_loader_normalizes_an_example():
    loaders = build_benchmarks(ROOT / "datasets")
    assert len(loaders) == 12
    for name, loader in loaders.items():
        example = next(loader.load(1))
        assert example.benchmark == name
        assert example.id
        assert example.choices
        assert set(example.choices).issubset(set(loader.label_space))
        assert example.gold_label in example.choices


def test_known_source_contracts():
    loaders = build_benchmarks(ROOT / "datasets")
    proof = next(loaders["proofwriter"].load(1))
    assert proof.gold_label == "True"
    assert proof.metadata["depth"] == 0
    winogrande = next(loaders["winogrande"].load(1))
    assert winogrande.gold_label == "2"
    assert winogrande.gold_label in winogrande.choices
    commonsense = next(loaders["commonsenseqa"].load(1))
    assert commonsense.labels == ["A", "B", "C", "D", "E"]


def test_jevbench_handles_noul_and_score_criteria():
    loader = build_benchmarks(ROOT / "datasets")["jevbench-original"]
    examples = list(loader.load())
    assert {example.question_type for example in examples} >= {"noul", "choice", "score"}
    assert all(set(example.labels) == set(example.choices) for example in examples)

