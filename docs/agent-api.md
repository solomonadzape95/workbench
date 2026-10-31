# Workbench agent API

The API is a local HTTP/JSON surface over the same operation registry and job
runner as the TUI. Use it from an agent, a shell script, or a small integration.

```sh
./bin/wb serve
# Workbench API listening on http://127.0.0.1:8765 · no token (local-only recommended)
```

Bind to localhost by default. If other processes or people can reach the port,
set a token:

```sh
./bin/wb serve --addr 127.0.0.1:8765 --token "$WORKBENCH_API_TOKEN"
```

Send `Authorization: Bearer <token>`. The token can also come from
`WORKBENCH_API_TOKEN`; `--token` wins. Binding to a non-loopback address exposes
file operations and output paths to the network: use a token and a firewall,
and do not expose the port to the public internet.

## Discover tools

```sh
curl -s http://127.0.0.1:8765/v1/operations
curl -s http://127.0.0.1:8765/v1/operations/video.transcript
curl -s http://127.0.0.1:8765/v1/engines
curl -s http://127.0.0.1:8765/v1/pipelines
```

Each operation includes its `source` (`files` or `urls`), `params`, `engines`,
and output format. This is the same schema the TUI form uses, so an agent can
build a job without hard-coding a separate tool registry.

## Start a job

Single operation:

```sh
curl -s -X POST http://127.0.0.1:8765/v1/jobs \
  -H 'Content-Type: application/json' \
  -d '{
    "operation": "video.transcript",
    "inputs": ["https://youtu.be/0vFErGxD2QY"],
    "params": {"language": "en", "auto": true, "timestamps": false},
    "target": {"dir": "/tmp/workbench-output"},
    "name": "youtube transcript"
  }'
```

A file operation takes paths, globs, directories, or aliases in `inputs`; a URL
operation takes full `http(s)` URLs. `target.dir` is optional and defaults to
the configured output directory. `target.template` and `target.collision`
(`suffix`, `overwrite`, `skip`, `fail`) are also optional.

Humanize a Markdown document with a free OpenCode model:

```sh
curl -s -X POST http://127.0.0.1:8765/v1/jobs \
  -H 'Content-Type: application/json' \
  -d '{
    "operation": "doc.humanize",
    "inputs": ["~/Documents/article.md"],
    "params": {
      "style": "professional",
      "model": "auto",
      "chunk_words": 900,
      "workers": 3
    },
    "target": {"dir": "/tmp/humanized"}
  }'
```

The job item metadata records the style, model, chunk count, and
`fallback_chunks`. DOCX inputs are also supported; the output DOCX keeps its
figures, tables, headers, and styles, and only body paragraphs are replaced.
See [`docs/humanizer.md`](./humanizer.md) for the rules, model fallback, and
the protected blocks (code, tables, quotes, links).

Detect AI-written prose and return a Markdown report plus JSON:

```sh
curl -s -X POST http://127.0.0.1:8765/v1/jobs \
  -H 'Content-Type: application/json' \
  -d '{
    "operation": "doc.detect_ai",
    "inputs": ["~/Documents/report.docx"],
    "params": {"threshold": 0.55, "json": true},
    "target": {"dir": "/tmp/detected"}
  }'
```

The Markdown artifact metadata carries `ai_percent`, `human_percent`,
`verdict`, `confidence`, `top_model`, and report paths. The JSON sidecar is the
stable structured schema. See [`docs/ai-detector.md`](./ai-detector.md) for the
signals, calibration, model-attribution limits, and references.

Saved pipeline by name:

```sh
curl -s -X POST http://127.0.0.1:8765/v1/jobs \
  -H 'Content-Type: application/json' \
  -d '{
    "pipeline": "convert-image-chain",
    "inputs": ["~/Pictures/shots/*.png"],
    "target": {"dir": "/tmp/workbench-output"}
  }'
```

Ad-hoc chain:

```sh
curl -s -X POST http://127.0.0.1:8765/v1/jobs \
  -H 'Content-Type: application/json' \
  -d '{
    "inputs": ["https://youtu.be/0vFErGxD2QY"],
    "steps": [
      {"operation": "video.download", "params": {"quality": "720p", "container": "mp4"}},
      {"operation": "video.compress", "params": {"crf": 28, "preset": "fast"}}
    ],
    "target": {"dir": "/tmp/workbench-output"}
  }'
```

A successful create returns `202 Accepted` with `{"job": {...}}`. Job IDs look
like `job-179...-001`.

## Wait for completion

Poll with a long-poll timeout in seconds:

```sh
curl -s 'http://127.0.0.1:8765/v1/jobs/job-179...-001?wait=30'
```

The request returns when the job reaches `done`, `partial`, `failed`, or
`canceled`, or after the timeout. Without `wait` it returns the current snapshot
immediately. List recent jobs with `GET /v1/jobs` or `?limit=50`. The list
merges the live runner with `history.jsonl`, so completed jobs remain visible
after a restart; `GET /v1/jobs/{id}`, retry, and rerun also work for historical
IDs.

Job snapshots include:

- `status`, `percent`, `counts`, timestamps, and duration
- `items[].input`, `items[].outputs` (paths, sizes, formats)
- `error` with `code`, `message`, `hint`, and `path`
- `logs` for diagnosis
- `metrics` (bytes in/out, duration, engine, model for model-backed operations)
- `outputs[].meta` for operation-specific metadata. For example,
  `doc.detect_ai` returns `ai_percent`, `human_percent`, `verdict`,
  `confidence`, `profile`, and `top_model` on the Markdown artifact.

## Stream, fetch, and callback

```sh
curl -N http://127.0.0.1:8765/v1/jobs/<id>/events
curl -s http://127.0.0.1:8765/v1/jobs/<id>/artifacts
curl -OJ http://127.0.0.1:8765/v1/jobs/<id>/artifacts/1
```

`events` is a Server-Sent Events stream of `job` events. It sends the current
snapshot immediately and then every 300 ms until the job is terminal. Artifact
indices are 1-based and follow `items[].outputs` order; the endpoint serves the
file with its normal name.

Add `callback_url` to a create request to receive one JSON POST when the job
finishes:

```json
{
  "operation": "image.convert",
  "inputs": ["/tmp/in.png"],
  "params": {"format": "jpeg"},
  "callback_url": "http://127.0.0.1:9000/workbench"
}
```

The callback body is the same job record used by the webhook integration: id,
name, status, duration, bytes in/out, output paths, and error when present.
Callbacks are best-effort and require an `http` or `https` URL.

## Control jobs

```sh
curl -s -X POST http://127.0.0.1:8765/v1/jobs/<id>/cancel
curl -s -X POST http://127.0.0.1:8765/v1/jobs/<id>/retry
curl -s -X POST http://127.0.0.1:8765/v1/jobs/<id>/rerun
```

`cancel` is cooperative and leaves no partial output. `retry` starts a new job
with only failed/cancelled items; `rerun` starts a new job with every original
input. Both return `202` with the new job.

## Errors

Errors use the same shape everywhere:

```json
{
  "error": {
    "code": "engine_missing",
    "message": "yt-dlp is not installed",
    "hint": "brew install yt-dlp",
    "operation": "video.download"
  }
}
```

HTTP mapping: `400` usage/input, `401` auth, `404` unknown job/operation,
`409` engine missing or job state conflict, `422` output problem, `502` engine
failure, `500` internal.

## Scope and rationale

The API is a good fit for **local automation**: an agent can discover the same
operations the TUI uses, run a batch, and read structured results without
scraping a terminal. It is not a multi-user service, a remote job queue, or a
replacement for the TUI.

Keep it narrow: catalog, create, inspect, cancel, retry/rerun. Because it shares
`runner.Runner`, it cannot drift on atomic output, cancellation, naming, or
engine limits. The MCP adapter (`wb mcp`) exposes the same operation catalog and
runner over stdio for agent hosts that prefer MCP to raw HTTP.

## Notes

- The API is local and headless; it does not open the TUI.
- `wb serve` uses the same config and saved pipelines as the TUI.
- No partial files are committed: outputs appear atomically at completion.
- Polling is the stable interface; `GET /v1/jobs/{id}/events` adds Server-Sent
  Events for hosts that want live updates.
- The OpenAPI schema is [`openapi.yaml`](./openapi.yaml); `pkg/apiclient` is the
  typed Go client.
