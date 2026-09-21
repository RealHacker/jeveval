# JevEval

JevEval is a Python CLI/library for comparing TypeSafe's Jev System One model with OpenAI-compatible reference models on fixed-label reasoning benchmarks. It writes raw predictions, accuracy and calibration metrics, latency percentiles, token use, and configurable cost estimates.

The harness uses the datasets already stored under `datasets/`; it does not download data during a run.

## Install

Python 3.11 or newer is required.

```powershell
python -m pip install -e .
```

Set credentials only in environment variables:

```powershell
$env:TYPESAFE_API_KEY = "..."
$env:OPENAI_API_KEY = "..."
```

Copy `config.example.yaml` and set the OpenAI-compatible base URL/model and, if desired, per-million-token prices. Secrets are referenced by environment-variable name and are never placed in config or cache keys.

## Commands

```powershell
# Inspect the local registry
jeveval list --config config.yaml

# Re-pick each mini subset reproducibly (change the seed for a different batch)
jeveval make-mini --seed 42

# Default mini run
jeveval run --config config.yaml --benchmarks folio,proofwriter --models jev,reference

# Full source files
jeveval run --config config.yaml --benchmarks cladder --models jev --mode full

# First N normalized examples from each selected full source
jeveval run --config config.yaml --benchmarks jevbench --models reference --mode 20
```

`--benchmarks` accepts individual registry names, `all`, or the groups `musr`, `cladder`, and `jevbench`. `--models` accepts configured model names or `all`. CLI values override the equivalent config values. The required options are available as `--benchmarks`, `--models`, `--mode`, and `--output-dir`.

## Model adapters

`JevAdapter` sends typed requests to `POST /v1/systemone`. Binary yes/no tasks use TypeSafe's `noul` question where appropriate; other tasks use `choice`. It consumes the returned probability directly. This follows the current [TypeSafe API reference](https://docs.typesafe.ai/api).

`OpenAICompatibleAdapter` calls a configurable Chat Completions endpoint and requests a strict JSON schema containing one label and a complete per-label probability distribution. Every request, including the one corrective structured-output request, sets `reasoning_effort: "none"` so reference models are evaluated without deliberate reasoning. Non-`none` configuration values are rejected. Set `structured_outputs: false` for providers that support JSON mode but not `json_schema`. Because provider pricing changes, prices are optional config values rather than code constants.

The default no-config registry also includes `deterministic`, an offline plumbing test adapter. It is not a meaningful benchmark baseline.

To add a model, subclass `jeveval.adapters.base.ModelAdapter`, return a normalized `Prediction`, and register its config type in `jeveval.config.build_adapters`.

## Benchmark adapters and local-data notes

Every loader implements `name`, `label_space`, and `load(n)`, and yields normalized `Example` objects. Add new loaders in `jeveval/benchmarks/loaders.py` and register them in `jeveval/benchmarks/registry.py`.

The supplied data differs from the names/formats in the initial design in several important ways:

- `winogrande/data.jsonl` is the labeled local WinoGrande source. Answers use the source labels `1` and `2`, matching `option1` and `option2` respectively.
- `commonsenseQA/dev_rand_split.jsonl` is classic five-option CommonsenseQA, not yes/no CommonsenseQA 2.0. It is exposed as `commonsenseqa` with labels A-E.
- `proofwriter/data-test.jsonl` is a translation export whose binary label is embedded in `translation.ro`; it does not contain Unknown examples. The loader parses its marker format and records a best-effort rule-depth value from the proof expression.
- MuSR files are JSON arrays containing nested questions. The loader flattens each question and retains story index, subtask, and depth metadata.
- JevBench has per-example labels and typed question schemas. The normalized example therefore carries its own label/criteria map; `label_space` is the union used for discovery only.

`jeveval make-mini` samples normalized problems, writes up to 100 per benchmark under `datasets/mini/`, and stores a manifest with the seed and SHA-256 of every source. Mini files with 100 or fewer source problems contain the complete benchmark.

## Outputs and metrics

Each run writes:

- `raw/<model>__<benchmark>.jsonl`: question, gold label, prediction, probabilities, latency, usage, metadata, cache flag, and any error;
- `summary.json`: full aggregate and metadata-slice metrics;
- `summary.md`: compact model-by-benchmark table;
- `cache/`: successful normalized predictions including the raw provider response.

Accuracy is top-1 over examples with gold labels. ECE uses ten equal-width bins over the predicted label's probability. Multiclass Brier score is the mean sum of squared error across the complete label distribution (lower is better). Latency reports mean/p50/p95. Cost is emitted only when the adapter has configured prices; `cost_per_1k_examples_usd` scales the observed successful-call cost.

Cache keys include the non-secret adapter configuration, model, benchmark/example ID, state, instructions, criteria, and question type. Changing a prompt/schema/model invalidates the old entry. Successful cached results preserve their original measured latency and usage and are marked `cached` in raw output.

## Reliability behavior

Before launching a model/benchmark batch, JevEval runs one uncached example as a preflight gate. If structured output is malformed, the OpenAI-compatible adapter sends one corrective request containing the exact JSON Schema. If that response is still malformed, or if the provider rejects the request (for example, because it does not support `reasoning_effort: "none"`), the run exits immediately without starting the remaining examples. Network failures and HTTP 408/409/425/429/5xx/529 responses still receive bounded retries with exponential backoff and jitter, honoring numeric `Retry-After` when present. Independent failures after a successful preflight are recorded without aborting other in-flight examples. Reports are refreshed after every completed model/benchmark pair.

## Verification

```powershell
python -m compileall -q jeveval tests
pytest -q
python -m jeveval run --benchmarks folio,jevbench-easy --models deterministic --mode 3 --output-dir sample-output
```

The last command is an offline end-to-end smoke run. Live verification requires valid provider credentials and may incur cost.
