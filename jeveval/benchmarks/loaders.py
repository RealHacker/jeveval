from __future__ import annotations

import json
import re
from collections.abc import Iterator
from pathlib import Path
from typing import Any

from ..models import Example
from .base import BenchmarkLoader, take


def _jsonl(path: Path) -> Iterator[dict[str, Any]]:
    with path.open("r", encoding="utf-8") as handle:
        for line_number, line in enumerate(handle, 1):
            if line.strip():
                try:
                    yield json.loads(line)
                except json.JSONDecodeError as exc:
                    raise ValueError(f"{path}:{line_number}: invalid JSON: {exc}") from exc


class FolioLoader(BenchmarkLoader):
    name = "folio"

    def __init__(self, path: Path):
        self.source_path = path

    @property
    def label_space(self) -> list[str]:
        return ["True", "False", "Uncertain"]

    def load(self, n: int | None = None) -> Iterator[Example]:
        def rows() -> Iterator[Example]:
            for index, row in enumerate(_jsonl(self.source_path)):
                premises = row["premises"]
                yield Example(
                    id=f"folio-{index}", benchmark=self.name,
                    state={"premises": premises},
                    instructions={"question": row["conclusion"], "task": "Is the conclusion true, false, or uncertain from the premises?"},
                    choices={"True": "The conclusion follows.", "False": "The conclusion is contradicted.", "Uncertain": "Neither follows nor is contradicted."},
                    gold_label=str(row["label"]),
                    metadata={"source_index": index},
                )
        return take(rows(), n)


class ProofWriterLoader(BenchmarkLoader):
    name = "proofwriter"
    _english = re.compile(r"^\$answer\$\s*;\s*\$proof\$\s*;\s*\$question\$\s*=\s*(.*?)\s*;\s*\$context\$\s*=\s*(.*)$", re.S)
    _answer = re.compile(r"\$answer\$\s*=\s*([^;]+)")

    def __init__(self, path: Path):
        self.source_path = path

    @property
    def label_space(self) -> list[str]:
        return ["True", "False"]

    @staticmethod
    def _depth(proof: str) -> int:
        return proof.count("#")

    def load(self, n: int | None = None) -> Iterator[Example]:
        def rows() -> Iterator[Example]:
            for index, row in enumerate(_jsonl(self.source_path)):
                translation = row.get("translation", {})
                en, annotated = translation.get("en", ""), translation.get("ro", "")
                parsed, answer = self._english.match(en), self._answer.search(annotated)
                if not parsed or not answer:
                    raise ValueError(f"{self.source_path}:{index + 1}: unrecognized ProofWriter translation record")
                question, context = parsed.groups()
                proof = annotated.split("$proof$ =", 1)[1].strip() if "$proof$ =" in annotated else ""
                yield Example(
                    id=f"proofwriter-{index}", benchmark=self.name,
                    state=context,
                    instructions={"question": question, "task": "Is the statement true or false under the supplied facts and rules?"},
                    choices={"True": "The statement is entailed.", "False": "The statement is not entailed."},
                    gold_label=answer.group(1).strip(),
                    metadata={"source_index": index, "depth": self._depth(proof)},
                )
        return take(rows(), n)


class WinograndeLoader(BenchmarkLoader):
    name = "winogrande"

    def __init__(self, path: Path):
        self.source_path = path

    @property
    def label_space(self) -> list[str]:
        return ["1", "2"]

    def load(self, n: int | None = None) -> Iterator[Example]:
        def rows() -> Iterator[Example]:
            for index, row in enumerate(_jsonl(self.source_path)):
                answer = row.get("answer", row.get("label"))
                yield Example(
                    id=str(row.get("qID", f"winogrande-{index}")), benchmark=self.name,
                    state=row["sentence"],
                    instructions="Which option correctly fills the underscore in the sentence?",
                    choices={"1": row["option1"], "2": row["option2"]},
                    gold_label=str(answer) if answer is not None else None,
                    metadata={"source_index": index, "unscored_reason": "The supplied WinoGrande record has no answer field."} if answer is None else {"source_index": index},
                )
        return take(rows(), n)


class CommonsenseQALoader(BenchmarkLoader):
    name = "commonsenseqa"

    def __init__(self, path: Path):
        self.source_path = path

    @property
    def label_space(self) -> list[str]:
        return ["A", "B", "C", "D", "E"]

    def load(self, n: int | None = None) -> Iterator[Example]:
        def rows() -> Iterator[Example]:
            for index, row in enumerate(_jsonl(self.source_path)):
                question = row["question"]
                yield Example(
                    id=str(row.get("id", f"commonsenseqa-{index}")), benchmark=self.name,
                    state=question["stem"], instructions="Choose the best commonsense answer.",
                    choices={str(item["label"]): item["text"] for item in question["choices"]},
                    gold_label=str(row["answerKey"]),
                    metadata={"source_index": index, "question_concept": question.get("question_concept", "")},
                )
        return take(rows(), n)


class MusrLoader(BenchmarkLoader):
    def __init__(self, path: Path, subtask: str):
        self.source_path = path
        self.subtask = subtask
        self.name = f"musr-{subtask.replace('_', '-')}"

    @property
    def label_space(self) -> list[str]:
        rows = json.loads(self.source_path.read_text(encoding="utf-8"))
        maximum = max(len(question["choices"]) for row in rows for question in row["questions"])
        return [str(index) for index in range(maximum)]

    def load(self, n: int | None = None) -> Iterator[Example]:
        def rows() -> Iterator[Example]:
            data = json.loads(self.source_path.read_text(encoding="utf-8"))
            for story_index, row in enumerate(data):
                for question_index, question in enumerate(row["questions"]):
                    choices = {str(i): text for i, text in enumerate(question["choices"])}
                    depths = [tree.get("depth") for tree in question.get("intermediate_trees", []) if isinstance(tree, dict) and isinstance(tree.get("depth"), int)]
                    yield Example(
                        id=f"{self.name}-{story_index}-{question_index}", benchmark=self.name,
                        state=row["context"], instructions=question["question"], choices=choices,
                        gold_label=str(question["answer"]),
                        metadata={"source_index": story_index, "question_index": question_index, "subtask": self.subtask, "depth": max(depths) if depths else None},
                    )
        return take(rows(), n)


class CladderLoader(BenchmarkLoader):
    def __init__(self, path: Path, tier: str):
        self.source_path = path
        self.tier = tier
        self.name = f"cladder-{tier}"

    @property
    def label_space(self) -> list[str]:
        return ["yes", "no"]

    def load(self, n: int | None = None) -> Iterator[Example]:
        def rows() -> Iterator[Example]:
            for index, row in enumerate(json.loads(self.source_path.read_text(encoding="utf-8"))):
                meta = row.get("meta", {})
                yield Example(
                    id=str(row.get("question_id", f"{self.name}-{index}")), benchmark=self.name,
                    state=row["given_info"], instructions=row["question"],
                    choices={"yes": "Yes", "no": "No"}, gold_label=row["answer"],
                    metadata={"source_index": index, "tier": self.tier, "rung": meta.get("rung"), "query_type": meta.get("query_type"), "story_id": meta.get("story_id")},
                    question_type="noul",
                )
        return take(rows(), n)


class JevBenchLoader(BenchmarkLoader):
    def __init__(self, path: Path, tier: str):
        self.source_path = path
        self.tier = tier
        self.name = f"jevbench-{tier}"

    @property
    def label_space(self) -> list[str]:
        labels: set[str] = set()
        for row in _jsonl(self.source_path):
            labels.update(map(str, row["labels"]))
        return sorted(labels)

    def load(self, n: int | None = None) -> Iterator[Example]:
        def rows() -> Iterator[Example]:
            for index, row in enumerate(_jsonl(self.source_path)):
                question = row["question"]
                labels = [str(label) for label in row["labels"]]
                criteria = question.get("criteria") or {}
                qtype = question.get("type", "choice")
                if isinstance(criteria, list):
                    choices = {label: criteria[position] if position < len(criteria) else None for position, label in enumerate(labels)}
                elif qtype == "noul":
                    choices = {
                        label: criteria.get("true" if label.casefold() in {"yes", "true"} else "false")
                        for label in labels
                    }
                else:
                    choices = {label: criteria.get(label) for label in labels}
                yield Example(
                    id=str(row["id"]), benchmark=self.name, state=row["state"],
                    instructions=question.get("instructions", "Choose the best answer."),
                    choices=choices, gold_label=str(row["expected"]),
                    metadata={"source_index": index, "tier": self.tier, "family": row.get("family"), "group": row.get("group"), "provenance": row.get("provenance")},
                    question_type=qtype,
                )
        return take(rows(), n)

