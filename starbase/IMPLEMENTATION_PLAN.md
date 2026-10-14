# StarBase implementation plan

## Objective

Build a standalone Go application in this directory with `fetch`, `tag`, and `serve` subcommands. It collects the authenticated user's GitHub stars and enrichment data into SQLite, applies Jev-derived topical tags, and serves a local paginated dashboard.

## Settled behavior

- GitHub credentials come from `GH_TOKEN` by default; a CLI option can select a different environment variable. Never persist or log tokens.
- `fetch` replaces the current dataset. It lists `/user/starred` with `application/vnd.github.star+json`, preserving the `starred_at` timestamp, then gathers repository metadata, commit activity for the last 60 days, GitHub weekly star history, and LOC details.
- Activity score is the number of commits returned in the 60-day window. Fewer than 10 means inactive. Commit records are transient and not stored.
- Default dashboard order is starred-at descending. Other sorts are stars, activity score, and LOC.
- StarBase's tags are the existing `tags.txt`. `tag` classifies each repository with Jev using description, languages, and up to the first 1,024 README words. Keep up to six tags whose probability is at least 0.1.
- `fetch` and `tag` replace their respective dataset/tag assignments. `refresh` compares current starred repositories using GitHub repository IDs, enriches and Jev-tags only additions, deletes removed repositories (cascading tags), and preserves all other rows unchanged in one transaction. `serve` requires fetch and tag stages to have completed.

## Source investigation

- GitHub REST supports authenticated-user starred repos and embeds star timestamps using the `application/vnd.github.star+json` Accept media type. Use page size 100 and follow pagination links.
- GitHub's REST `GET /repos/{owner}/{repo}/stargazers/history` is the structured weekly star-history source used by the Star History project; it avoids HTML parsing and returns week, total, and daily counts.
- `ghloc-web`'s public badge route returns only a badge label/value. Its source calls `https://ghloc.ifels.dev/{owner}/{repo}/{branch}?pretty=false` for the actual structured LOC response (`loc`, per-language counts, and nested children). Use that JSON endpoint and retain total and per-language counts; do not parse rendered HTML.
- GitHub responses include rate-limit headers. Requests must observe `Retry-After` and reset headers on throttling, and avoid concurrency that overwhelms the authenticated quota.

## Architecture and database

- Standalone module, with SQLite through a pure-Go driver for Windows portability.
- A `repositories` table stores GitHub identity (`github_id`) and canonical URLs, description, counts, language map, license, fork/archived/private flags, topics, timestamps, default branch, README excerpt, star history, LOC summary, activity score, active state, and stage completion state. Repeated structured fields use JSON columns; tags use a normalized relation table with probability. Existing databases receive the `github_id` column automatically and are matched by name once to backfill it.
- `fetch` stages data before replacing the database contents in a transaction. Per-repository enrichment errors are recorded as unavailable/error status without silently inventing metric values; authentication/list failures abort without destroying the prior successful dataset.
- `tag` requires a completed fetch, clears previous tags, evaluates every repository, and marks the tag stage ready only after all evaluations succeed. Jev API defaults follow `notetag` (`https://api.typesafe.ai/v1/systemone`, `jev-latest`, `TYPESAFE_API_KEY`) with configurable flags and timeout/retry behavior.
- `serve` validates fetch and tag stage markers before starting a loopback-only server by default. Server options include bind address and page size. All filtering/sorting/paging is done with parameterized SQL; HTML is rendered with Go templates and proper escaping. Tag/language/active filters and repository detail modal cards expose history, LOC breakdown, and activity.

## Commands

```text
starbase fetch [--db starbase.db] [--github-token-env GH_TOKEN] [--github-api https://api.github.com]
starbase tag [--db starbase.db] [--tagsfile tags.txt] [--endpoint https://api.typesafe.ai/v1/systemone] [--model jev-latest] [--api-key-env TYPESAFE_API_KEY]
starbase serve [--db starbase.db] [--addr 127.0.0.1:8080] [--page-size 25]\nstarbase refresh [--db starbase.db] [--github-token-env GH_TOKEN] [--api-key-env TYPESAFE_API_KEY]
```

Flags and defaults will be documented in `README.md`; database path defaults relative to the working directory.

## Implementation slices

1. Add the standalone module, CLI, schema, fetch/API clients, pagination and rate-limit handling.
2. Add LOC and star-history clients, README retrieval/excerpt, and transactional dataset replacement.
3. Add Jev Choice client and atomic tag refresh.
4. Add the paginated/filterable/sortable UI and modal repository detail views.
5. Add incremental refresh with staged enrichment and Jev tagging, then document setup and operation. Build the application; no live authenticated GitHub fetch or Jev call.

## Risks and limits

- The exact `ghloc.ifels.dev` contract is owned by a third party and may change; treat missing/invalid data as unavailable and surface it in the detail view.
- GitHub API v2026-03-10 star-history pagination is capped at 100 pages; report history beyond the 100-page cap as unavailable rather than presenting partial history as complete.
- GitHub's per-repository commit endpoint caps its response at 100 per page; follow pagination to count the full 60-day window (the score is explicitly a recent-commit count).


