# Task: Build a benchmark harness comparing TypeSafe's Jev (System One model) against reference LLMs

## Objective
Build a Python CLI/library that runs a selected set of reasoning benchmarks against a
selected set of models (Jev plus one or more comparison LLMs), scores them, and reports
accuracy + calibration metrics. The user should be able to pick which benchmark(s) and
which model(s) to run via config or CLI flags — no benchmark or model should be hardcoded
into the run logic.

To learn about Jev: https://typesafe.ai/blog/introducing-system-one-models-and-jev 

## Models to support (pluggable via a `ModelAdapter` interface)
1. **Jev** (TypeSafe AI) — called via their System One API (https://docs.typesafe.ai/api).
   Output is a typed/structured decision with a calibrated probability per class, not free
   text. Adapter must define the schema (allowed labels) per-benchmark and parse the
   returned probability distribution directly — no answer-extraction/parsing needed since
   output is already structured.
2. **Reference LLMs** — at minimum, adapters for the OpenAI Compliant API (use
   official SDK / raw REST as appropriate), with customizable BaseURL Endpoint and model identifier. Support a "System One LLM wrapper" mode
   matching TypeSafe's own methodology: constrain the LLM's response format to the same
   fixed label schema (e.g., function calling / structured JSON output with logprobs or a
   self-reported confidence field) so comparisons are apples-to-apples with Jev's typed
   output.

## Benchmarks to support (pluggable via a `BenchmarkLoader` interface)
Implement loaders for the following, each yielding `(question, choices, gold_label)`
triples with a small fixed label space (binary or small multiple-choice) — pull data via
HuggingFace `datasets` where available:

- **FOLIO** — natural language + logic, True/False/Uncertain classification.
  (`datasets`: "tasksource/folio" or similar; confirm current HF path at build time.)
- **ProofWriter** (or RuleTaker) — synthetic rule-based entailment, True/False/Unknown.
  Include a `depth` field if present, so results can be sliced by reasoning-chain length.
- **Winogrande** — binary pronoun resolution. (`datasets`: "winogrande")
- **CommonsenseQA 2.0** — yes/no commonsense questions. If not on HF, note the fallback
  source and flag as optional.
- **MuSR** — multi-step narrative reasoning MCQ (murder mysteries, team allocation, object
  placement). (`datasets`: "TAUR-Lab/MuSR" or similar; confirm exact path.)
- **CLadder** — yes/no causal inference questions.
- **JevBench** - a recent benchmark for Jev-like modles.

The dataset of each of the benchmarks have been downloaded to /datasets, all in jsonl format. The data files should be self-explanatory, if there is something unclear about the benchmark, do web search to learn about it.
For each jsonl file, if it contains more than 100 problems, please use a script to randomly pick 100 problems from it, and save it as a mini version of the original. When executing a benchmark, by default use the mini version, and only use the full dataset when the mode parameter is 'full'. The selection script can be re-run to pick a different batch.

Each `BenchmarkLoader` should expose: `name`, `label_space` (list of valid answer strings),
`load(n: int | None) -> Iterator[Example]`, and optional metadata fields (e.g. reasoning
depth, subtask name) for later slicing.

## CLI / config
- `--benchmarks` (comma-separated names or `all`)
- `--models` (comma-separated names or `all`)
- `--mode` (sample size for a benchmark, default to mini subset, or 'full' for full dataset)
- `--output-dir` (where to write raw predictions + summary report)
- Config file (YAML/JSON) as an alternative to flags, listing API keys/env var names per
  model adapter.

## Metrics to compute per (model, benchmark) pair
- **Accuracy** (top-1 label match), accurate and total problem count.
- **Calibration**: Expected Calibration Error (ECE, e.g. 10-bin) and Brier score, using the
  model's reported probability for its predicted (or gold) label
- **Latency**: mean/p50/p95 per-call latency
- **Cost**: if the adapter can report tokens/pricing, compute $ per 1k examples

## Output
- Raw per-example predictions saved as JSONL (question, gold, prediction, probs, latency)
- A summary table (model x benchmark) with accuracy/ECE/Brier/latency/cost

## Engineering notes
- Rate-limit and retry API calls; cache raw responses to disk keyed by
  (model, benchmark, example_id) so re-runs/failed runs don't re-spend budget.
- Keep API keys out of code — read from environment variables, document required var names
  in a README.
- Structure as a proper package (e.g. `jeveval/{adapters,benchmarks,metrics,cli}.py`)
  rather than a single script, since benchmarks/models will likely be added over time.

## Deliverables
1. The package/CLI as described above
2. A README explaining setup (API keys, install), how to add a new benchmark or model
   adapter, and example invocation commands
3. A sample run against 1-2 benchmarks with a small `n` to confirm end-to-end correctness