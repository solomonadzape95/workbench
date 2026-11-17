# Measuring the funnel

Workbench ships a local-first binary: the privacy row of `SPEC.md` §10 says no
network except explicit model downloads, and **no telemetry**. That constraint is
deliberate, so this document covers what we can measure without breaking it, how
to read those numbers, and what counting real users would cost.

The short version: **downloads and installs are public and free; users are not.**
Approximate users from the funnel, and only reach for in-binary reporting if a
specific decision needs it.

## What is measurable today

| Signal | Source | Answers |
|---|---|---|
| Release downloads | GitHub releases API, `assets[].download_count` | How many binary fetches, per platform and version |
| Homebrew installs | Homebrew's public analytics API (`formulae.brew.sh`), also shown by `brew info` | Install events through the tap (30/90/365 days) |
| Repo traffic | GitHub → Insights → Traffic | Clones, views, and referring sites (14 days, owner only) |
| Site visits | `workbench.thesolenoid.space` via GoatCounter | How people find the page and which docs they open |
| Install intent | GoatCounter events `install-curl`, `install-brew` | Clicks on copy in the install panel |

None of these touch a user's machine, and none require a privacy policy change.

## Run it

```sh
make funnel
```

```text
Workbench install funnel

releases                  3
downloads                23
  Linux / amd64           9
  macOS / arm64           6
  macOS / amd64           4
  Linux / arm64           4
homebrew 30d              0
homebrew 90d              0
homebrew 365d             0

Binary archives only; checksums, signatures, and metadata are excluded.
Downloads count every fetch: updates, CI, and mirrors included.
They are a trend line, not a headcount.
```

That is the real output on 2026-11-17. `python3 scripts/funnel.py --json` prints
the same data for scripts. The GitHub API allows 60 unauthenticated requests per
hour per IP; export `GITHUB_TOKEN` if you hit the limit.

The Releases panel on the site shows the same download total, read directly from
the GitHub API at page load (`site/funnel.js`).

There is also an unlinked dashboard at `/admin.html` with the same numbers as
dithered charts. It is gated by a client-side password, which hides it from
casual visitors only — the data is public either way. See
[`site/README.md`](../site/README.md#stats-page) for the password rotation step.

## Reading the numbers

A download is one asset fetch, not one person:

- Updating from `v0.1.1` to `v0.1.2` counts again.
- CI jobs, mirrors, and archive tools count.
- One person on macOS with two machines counts twice.
- Homebrew only lists formulas installed at least once in the window, and users
  who set `HOMEBREW_NO_ANALYTICS` are invisible; brew counts are a floor.
- Anything installed from source, `go install`, or a package manager we do not
  own is invisible.

Treat the total as a trend line. What matters is the shape over weeks — flat,
rising, or spiking after a post — not the absolute value. Record it somewhere
weekly; a single reading answers nothing.

## Turning on site analytics

The site is static HTML on GitHub Pages, so there are no server logs. Page counts
need a third-party script. [GoatCounter](https://www.goatcounter.com) is the best
fit here: cookieless, no personal data, no consent banner, free for small sites,
and it respects Do Not Track.

1. Create a GoatCounter account and pick a code, for example `workbench`.
2. In [`site/index.html`](../site/index.html), set:

   ```html
   <meta name="wb-analytics" content="workbench">
   ```

   A full URL also works if you self-host:

   ```html
   <meta name="wb-analytics" content="https://count.example.com">
   ```

3. Push. `site/analytics.js` picks up the tag and loads the counter.

Leaving `content=""` disables analytics entirely — no request is made, so the
site stays as it is today. The only events sent are page views and the two
install-copy events from `site/copy.js`; no text from the clipboard is ever
transmitted, only a fixed event name.

If the domain is proxied through Cloudflare, the dashboard also shows request
counts per path. Requests to `/i` approximate install attempts from
`curl … | sh`, which nothing else measures.

## When we need real user counts

Downloads and visits cannot tell you activation or retention: how many installs
produce a second run, which operations people actually use, or whether v0.2
keeps its users. Only the binary knows that.

If a decision needs those numbers, the smallest honest version is:

- A random install ID written once beside `state.json`, never derived from the machine.
- One `POST` on first run, then at most once per day: `{install_id, version, os, arch, event}`.
- Operation **counts** only. No paths, no filenames, no parameters, and nothing from `history.jsonl` — file paths are the sensitive part of this product.
- Opt-in and default **off**, with a line on the first-run onboarding screen, a `[telemetry]` config section, and a way to turn it off afterwards.
- A `docs/privacy.md` page, and edits to the privacy row of `SPEC.md` §10 plus the
  "no accounts, no uploads" claims in the README.

Shipping that is a product decision, not an implementation detail. Until a real
question requires it, the funnel above is the answer.
