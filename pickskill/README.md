# pickskill

`pickskill` is a small Go CLI that asks [TypeSafe AI's Jev model](https://docs.typesafe.ai/api) which skills are relevant to a task. It reads only each skill's YAML frontmatter, sends that metadata to Jev as structured `state`, and returns a JSON list of skill directory names.

## Skill repository layout

Only immediate child directories are inspected:

```text
skills/
├── documents/
│   └── SKILL.md
├── presentations/
│   └── SKILL.md
└── spreadsheets/
    └── SKILL.md
```

Every `SKILL.md` must start with non-empty YAML frontmatter:

```markdown
---
name: documents
description: Create and edit Word documents.
---

# Documents
...
```

The returned name is the immediate directory name (`documents` above). The Markdown body is never read into Jev's state.

## Build

Go 1.24 or newer is required.

```console
cd pickskill
go build -o pickskill .
```

On Windows this creates `pickskill.exe`.

## Configure and run

Set the API key in the default environment variable:

PowerShell:

```powershell
$env:TYPESAFE_API_KEY = "your-key"
./pickskill.exe --skillsdir C:\path\to\skills --prompt "Create a quarterly report as an Excel workbook"
```

macOS/Linux:

```sh
export TYPESAFE_API_KEY="your-key"
./pickskill --skillsdir /path/to/skills --prompt "Create a quarterly report as an Excel workbook"
```

Stdout is always a JSON array, ordered from highest to lowest relevance:

```json
["spreadsheets"]
```

No match is successful and prints `[]`. Errors produce a non-zero exit code. `--verbose` writes request and score diagnostics to stderr without changing stdout.

## Options

```text
--skillsdir DIR          Skills repository (required)
--prompt TEXT            Current task prompt (required)
--threshold FLOAT        Minimum relevance probability, 0..1 (default 0.5)
--max-skills INT         Maximum returned names; 0 is unlimited (default 5)
--batch-max-skills INT   Maximum candidates per Jev request (default 100)
--batch-max-bytes INT    Maximum serialized request size (default 24576)
--model NAME             Jev model or alias (default jev-latest)
--endpoint URL           System One endpoint
--api-key-env NAME       API-key environment variable (default TYPESAFE_API_KEY)
--timeout DURATION       Per-request HTTP timeout (default 1m)
--retries INT            Transient retry count (default 2)
--verbose                Print diagnostics and relevance scores to stderr
```

Go duration syntax is accepted by `--timeout`, such as `30s` or `2m`.

## Selection and batching

Each skill gets its own Jev `noul` question. A `noul` response is the probability that the answer is yes, so scores are independent: multiple skills can exceed the threshold. Results are filtered, sorted by score (then name for stable ties), and capped by `--max-skills`.

Jev currently documents a 64k-token request context and a 32k-token limit for `state` plus the longest question. `pickskill` greedily makes batches constrained by both `--batch-max-skills` and the exact serialized JSON size in `--batch-max-bytes`. The conservative 24 KiB default also stays below 32k even under byte-level tokenization. You can raise it for fewer requests after measuring your metadata. If one prompt plus one skill cannot fit the configured byte ceiling, the CLI fails instead of truncating frontmatter.

Batch scores are directly comparable because every candidate is asked the same absolute yes/no relevance question. Batches run sequentially to avoid creating rate-limit spikes.

## Examples

Return every skill with at least 70% relevance, without a hard result cap:

```console
pickskill --skillsdir ./skills --prompt "Review this pull request for security issues" --threshold 0.70 --max-skills 0
```

Use a pinned model and smaller batches:

```console
pickskill --skillsdir ./skills --prompt "Build a slide deck" --model jev-1.13.0 --batch-max-skills 25 --batch-max-bytes 65536
```

Use a differently named key variable or a compatible test endpoint:

```console
pickskill --skillsdir ./skills --prompt "Summarize this PDF" --api-key-env JEV_API_KEY --endpoint http://127.0.0.1:8080/v1/systemone
```

## Development

The test suite uses local in-process HTTP servers and does not make paid API calls:

```console
go test ./...
go vet ./...
```
