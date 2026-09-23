# Pickskill implementation plan

## Contract

- Discover only `<skillsdir>/<skill-name>/SKILL.md`; the immediate directory name is the returned skill name.
- Extract only the leading YAML frontmatter (between the opening and closing `---` lines). Missing, empty, or malformed frontmatter is an input error.
- Send the task prompt and a batch of `{name, frontmatter}` records as Jev `state`.
- Ask one independent `noul` question per skill so every skill receives an absolute relevance probability and multiple skills can be selected.
- Filter by a configurable probability threshold, rank by probability descending with skill name as a deterministic tie-break, and apply an optional maximum result count.

## Implementation

1. Create an isolated Go module and a small standard-library-only CLI.
2. Add deterministic top-level skill discovery and strict frontmatter extraction.
3. Build Jev requests for `POST /v1/systemone`, authenticated from a configurable environment variable.
4. Greedily batch requests under both a maximum skill count and an exact serialized JSON byte limit. Reject a single skill that cannot fit instead of silently truncating it.
5. Validate Jev response types, question coverage, and probabilities; retry transient HTTP failures with bounded backoff and `Retry-After` support.
6. Keep stdout machine-readable as a JSON array of skill names; send diagnostics only to stderr.

## Verification

- Unit-test discovery/frontmatter edge cases, batching, thresholding, ranking, result caps, request shape, and API failures with an in-process HTTP server.
- Run `go test ./...`, `go vet ./...`, and `go build ./...`.
- Document installation, all flags, environment configuration, output format, batching behavior, and a local example.
