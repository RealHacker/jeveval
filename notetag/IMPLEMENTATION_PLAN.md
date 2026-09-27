# Implementation plan

Build a standalone standard-library Go CLI matching pickskill API conventions. Recursively classify Markdown using one Choice question over all tags and at most 1000 words and 3000 Unicode characters. Select up to six probabilities strictly above the configurable threshold. Persist individual results in .tags/results.json and applied records in .tags/applied.json. Apply sequentially oldest first, preserve mtimes, frontmatter, BOM and newline style. Journal output hashes to recover interrupted writes without duplicate hashtags; reject stale inputs. Validate through local HTTP integration tests, formatting/recovery tests, go test, go vet and build.


