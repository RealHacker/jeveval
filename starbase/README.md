# StarBase

StarBase is a standalone Go app for collecting, tagging, and browsing your starred GitHub repositories.

## Setup

- Go 1.24 or newer.
- Set `GH_TOKEN` to a GitHub token that can read your starred repositories and repository metadata. Fine-grained tokens need Starring (read) access; metadata for private repositories may also require repository metadata access.
- Set `TYPESAFE_API_KEY` before running `tag`.
- The SQLite database defaults to `starbase.db` in the current directory. Keep the `tags.txt` file beside the command working directory, or pass `--tagsfile` explicitly.

## Commands

Run from this directory:

```powershell
$env:GH_TOKEN = "..."
go run . fetch
$env:TYPESAFE_API_KEY = "..."
go run . tag
 go run . serve
```

`fetch` collects the user's starred repositories, repository metadata, language breakdowns, README excerpt, 60-day commit count, weekly star history and LOC details. It replaces the prior repository dataset only after collecting the current list. GitHub API calls are serial, paginated, and wait on rate-limit reset information. The activity score is the commit count in the last 60 days; fewer than 10 commits is inactive. Commit records are discarded.

GitHub's API star-history endpoint is used instead of parsing Star History HTML. The LOC breakdown comes from the structured `ghloc.ifels.dev` endpoint used by `ghloc-web`; it includes total LOC, per-language LOC and directory data. `ghloc-web`'s own public badge route exposes only a badge value.

`tag` reads each stored description, language map, and README excerpt (at most 1,024 words), sends one Jev Choice question using the tags in `tags.txt`, and stores up to six tags with probability >= 0.1. Rerunning it replaces the old tag assignments. Options: `--db`, `--tagsfile`, `--endpoint`, `--model`, `--api-key-env`, and `--timeout`.

`serve` requires both fetch and tag stages to be ready and starts the dashboard at `http://127.0.0.1:8080`. The UI supports paging, language/activity/tag filters, and sorting by starred time (default), stars, activity score, or LOC. Repository detail cards show star history, LOC, metadata and the README excerpt. Options: `--db`, `--addr`, and `--page-size`.

For GitHub Enterprise or a compatible API, `fetch` accepts `--github-api` and `--github-token-env`. To point any command at another database, use `--db`.

## Data and failure behavior

Repository metadata and metrics are stored in SQLite; commit data and API credentials are not. Per-repository enrichment failures are recorded as unavailable with an error summary, while failure to list the user's stars leaves the last successful database intact. `tag` marks the stage incomplete until every repository has been evaluated successfully, and `serve` refuses incomplete datasets.


