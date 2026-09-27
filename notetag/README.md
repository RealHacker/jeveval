# notetag

Two-phase Markdown tagging with Jev. Requires Go 1.24+; no external dependencies.

## Build and run (PowerShell)

    cd E:\Toy\JevEval\notetag
    go build -o notetag.exe .
    $env:TYPESAFE_API_KEY = "your-api-key"
    .\notetag.exe tag --dir "C:\notes" --tagsfile .\tags.txt
    .\notetag.exe apply --dir "C:\notes"

The tag command sends each document's first 1000 whitespace-delimited words, capped at 3000 Unicode characters (whichever limit comes first) and the supplied tag choices to Jev. Markdown and YAML are retained in that excerpt. Recursive .md and .markdown discovery is case-insensitive; symlinks, .git and .tags are skipped.

Tags are UTF-8, one per line, without # or whitespace. Empty lines are ignored and duplicate tags are deduplicated. A Choice supports at most 255 unique tags. The existing tags.txt is preserved.

Uses [Jev's documented Choice API](https://docs.typesafe.ai/api): POST /v1/systemone, model jev-latest, Bearer authentication, and answers.tags.probabilities. Choice probabilities are expected to sum to one across all tags (totals from 0.95 to 1.05 are accepted for rounding; scores are not rescaled); they are relative topic probabilities, not independent relevance scores.

## Tag options

- --threshold 0.1: select probabilities strictly greater than this value; take at most six, highest probability first, alphabetical tie-breaking.
- --endpoint https://api.typesafe.ai/v1/systemone
- --model jev-latest
- --api-key-env TYPESAFE_API_KEY
- --timeout 60s

The apply command needs only --dir and makes no API calls. Use tag --help or apply --help for options. HTTP errors include up to 2000 characters of the API response for diagnosis. Requests are sequential with no automatic retries; rerun after resolving a failure.

## State and resuming

- .tags/results.json: relative document paths, source SHA-256, selected tags, full probabilities, original modification time, and application recovery hash.
- .tags/applied.json: successfully applied documents.
- .tags/lock: excludes simultaneous commands. After a forced termination, remove this lock only after confirming no command is running.

Classification saves after each response. Rerunning tag skips saved classifications and applied documents. Changing threshold or tag list affects only unprocessed documents. To reclassify an unapplied document, remove its entry from results.json and rerun tag. Applied documents are skipped by path even if subsequently edited.

The apply command sorts pending documents oldest to newest, uses relative paths for ties, and writes sequentially. Original modification times are restored so ordering also survives partial runs. It inserts #tag1 #tag2 before the body, below YAML frontmatter when present, followed by a blank line. UTF-8 BOM, LF/CRLF style and permission bits are retained. No selected tags means no content change, but the document is recorded as processed.

Changed content since classification is rejected. Unterminated YAML frontmatter is rejected when tags need inserting. Each expected output hash is saved before replacement; reruns recognize already-written output and finish checkpointing without duplicating the tag line. Replacement uses a synced temporary sibling and rename; abrupt OS/power failure guarantees depend on the filesystem. Avoid editing notes while apply is running. Existing manually written hashtags are retained.

## Verification

    go test ./...
    go vet ./...

Tests use a local HTTP server (no paid API requests), covering the request contract, truncation, thresholds, cap, formatting, reruns, stale files, oldest-first application, modification times and recovery.



