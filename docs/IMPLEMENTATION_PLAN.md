# JevEval Implementation Plan

## 1. Establish the package and contracts

- Create a Python 3.11+ package with a `jeveval` console entry point.
- Define typed domain objects for benchmark examples, model predictions, usage, per-example run records, and aggregate metrics.
- Define pluggable `BenchmarkLoader` and asynchronous `ModelAdapter` interfaces; benchmark-specific prompt/schema construction stays in loaders, while transport and response normalization stay in adapters.
- Use stable example IDs and validate label spaces/probability distributions at module boundaries.

## 2. Normalize the supplied datasets

- Implement local-file loaders for FOLIO, ProofWriter, WinoGrande, CommonsenseQA, MuSR, CLadder, and JevBench.
- Preserve benchmark metadata for slicing (`depth`, `subtask`, `rung`, `family`, `tier`, and related source fields).
- Normalize each record into a common representation containing state/context, instructions, choices/criteria, gold label, and metadata.
- Treat source-data mismatches explicitly:
  - the supplied WinoGrande `data.jsonl` contains labeled examples using `1`/`2` labels for `option1`/`option2`;
  - the supplied CommonsenseQA file is the classic five-choice dataset, not CommonsenseQA 2.0, so expose and document it as five-way `commonsenseqa`;
  - the supplied ProofWriter translation export has binary `True`/`False` labels embedded in marker strings, so parse that exact format and derive best-effort proof depth metadata;
  - MuSR source files are JSON arrays with nested questions, so flatten questions while retaining story/subtask identity;
  - JevBench has per-example schemas, so retain each example's labels and typed question definition rather than imposing one global schema.

## 3. Generate reproducible mini datasets

- Add a standalone command/script that samples up to 100 normalized problems from every source containing more than 100 scoreable problems.
- Accept a seed so the same subset is reproducible and changing the seed selects a new batch.
- Save mini subsets under `datasets/mini/` as normalized JSONL plus a manifest containing source path, source fingerprint, seed, and counts.
- Make `mode=mini` the default, support `mode=full`, and accept a positive integer mode as an additional run-time cap.

## 4. Implement model adapters

- Implement a Jev adapter against `POST /v1/systemone`, mapping binary questions to `noul` and multiclass questions to `choice`, and normalize returned probabilities into the benchmark labels.
- Implement an OpenAI-compatible adapter with configurable base URL, API key environment variable, model ID, and endpoint mode.
- In System One wrapper mode, request a strict JSON probability map over the exact per-example labels; validate and optionally normalize it, with bounded repair/retry behavior.
- Capture resolved model name, latency, token usage, raw response, and optional cost. Keep prices configuration-driven rather than hardcoded into run logic.

## 5. Build the execution engine

- Resolve selected benchmark/model registries from CLI or YAML/JSON config without hardcoded execution branches.
- Run asynchronously with configurable concurrency, request timeouts, exponential backoff for transient errors, and rate-limit handling.
- Cache successful raw model results by adapter configuration, benchmark, example ID, and prompt/schema fingerprint so stale prompt or model changes cannot reuse incompatible entries.
- Write append-safe per-example JSONL records and continue independent examples after bounded failures; include failure counts in summaries.

## 6. Metrics and reports

- Compute accurate count/total count, top-1 accuracy, multiclass Brier score, 10-bin top-label ECE, mean/p50/p95 latency, token totals, total cost, and cost per 1,000 examples.
- Aggregate by model and benchmark, and emit optional metadata slices when fields such as depth, rung, subtask, family, or tier exist.
- Produce machine-readable JSON plus a human-readable Markdown summary table alongside raw prediction JSONL files.

## 7. CLI, configuration, and documentation

- Implement `run`, `make-mini`, and `list` commands; support the required `--benchmarks`, `--models`, `--mode`, `--output-dir`, and config-file options.
- Provide an example configuration with environment-variable references only; never persist API keys.
- Document installation, dataset caveats, credentials, commands, outputs, adding adapters/loaders, cache semantics, probability/calibration definitions, and pricing configuration.

## 8. Verification

- Add unit tests for every dataset parser, mini sampling, probability validation, cache keys, metrics, retry decisions, config merging, and report generation.
- Add mocked transport tests for Jev and OpenAI-compatible structured outputs, including malformed and transient responses.
- Run formatting/static checks and the full test suite.
- Execute an offline end-to-end sample using a deterministic test adapter over one or two benchmarks. Live Jev/OpenAI verification will be reported separately and only performed when the corresponding API keys are available.
