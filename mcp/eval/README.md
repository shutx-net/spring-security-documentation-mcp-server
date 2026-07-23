# Retrieval evaluation datasets

This directory holds the fixtures the `eval` CLI (`spring-security-docs-mcp eval
run` / `eval score`) uses to measure search quality.

```
datasets/<name>/topics.jsonl   information needs (the queries)
datasets/<name>/qrels.jsonl    relevance judgments (the answer key)
samples/                        tiny fake fixtures for CLI smoke tests / --help
```

The daily `retrieval-eval.yml` workflow uploads `datasets/spring-security-6.5.x/`
to S3, runs the live index against `topics.jsonl`, and scores the results
against `qrels.jsonl`.

## Fixture schema

**topics.jsonl** — one information need per line:

```json
{"topicId":"SS-001","query":"How do I configure SecurityFilterChain?","ref":"6.5.x","area":"servlet"}
```

`ref`/`area` are optional retrieval filters. Note `area` is the indexer's
deepest-match section (`servlet`, `oauth2`, `architecture`, …), not `servlet`
vs `reactive` — over-narrow filters silently drop relevant chunks.

**qrels.jsonl** — one judgment (topic × chunk → grade 0–3) per line. Identify
the judged chunk by a **stable key**, not the chunkId:

```json
{"topicId":"SS-001","canonicalUrl":"https://docs.spring.io/.../servlet/configuration/java.html","headingPath":["Java Configuration","HttpSecurity"],"grade":3}
```

**run.jsonl** — produced by `eval run`; carries `chunkId` **and** the stable-key
fields (`canonicalUrl`, `headingPath`) so scoring can match either way.

## Why stable keys (and not chunkId)

`chunkId = sha256(ref : commitSha : canonicalUrl : headingPath)`. Because the
hash includes `commitSha`, **every chunkId changes when the docs are
re-indexed**, even if the content is identical. Judgments pinned to chunkId go
stale on the next reindex and silently score 0.

A judgment keyed on `(canonicalUrl, headingPath)` — the semantic location of the
section — keeps matching across reindexing as long as the page and heading still
exist. `eval score` matches on the stable key when present and falls back to
`chunkId` for legacy fixtures, so both formats work; prefer the stable key for
anything new.

## Guardrails (fail loud, not silent-zero)

`eval` fails the run instead of reporting a green 0.000 when a fixture or the
pipeline is broken:

- `eval run --fail-on-empty` (default on): no results from any topic → the
  search pipeline is broken (auth/filter error).
- `eval score --fail-on-zero-judged` (default on): results were retrieved but
  **none** are in the qrels pool → the fixture is almost certainly stale;
  regenerate it against the current index.

The score report also prints `Judged coverage: <judged>/<retrieved>` so drift is
visible before it reaches zero.

## Regenerating / migrating qrels

To (re)build judgments so they stay in sync:

1. Run `eval run` against the current index — the output already contains
   `canonicalUrl` + `headingPath` for every retrieved chunk.
2. Pool the retrieved chunks per topic and assign grades (0–3), writing each
   judgment with `canonicalUrl` + `headingPath` (not `chunkId`).

Migrating an existing chunkId-based `qrels.jsonl` to stable keys requires
resolving each chunkId → `canonicalUrl`/`headingPath` via the chunks table
(`DocChunksTable`), which needs index access; do it as a one-off with AWS
credentials, then keep the stable-key file going forward.
