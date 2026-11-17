#!/usr/bin/env python3
"""Print the public install funnel for Workbench.

Only public data is read: GitHub release asset download counts and Homebrew
install counts. Nothing here touches a user's machine, and the Workbench binary
still makes no network calls.

    make funnel
    python3 scripts/funnel.py --json

Unauthenticated GitHub API calls are limited to 60 per hour per IP. Export
GITHUB_TOKEN to raise the limit; it is used only for this request.

Reading the numbers: a download is one binary archive fetch. Updates, CI runs,
mirrors, and repeat installs all count, so downloads overstate people. Treat the
totals as a trend line, not a headcount.
"""

from __future__ import annotations

import argparse
import json
import os
import sys
import urllib.request

API = "https://api.github.com"
ANALYTICS = "https://formulae.brew.sh/api/analytics/install/{window}.json"
TAP_FORMULA = "solomonadzape95/tap/workbench"
WINDOWS = ("30d", "90d", "365d")


def fetch(url: str):
    request = urllib.request.Request(
        url,
        headers={
            "Accept": "application/vnd.github+json",
            "User-Agent": "workbench-funnel",
        },
    )
    token = os.environ.get("GITHUB_TOKEN", "").strip()
    if token:
        request.add_header("Authorization", f"Bearer {token}")
    with urllib.request.urlopen(request, timeout=20) as response:
        return json.load(response)


def platform(name: str) -> str:
    lower = (name or "").lower()
    if "arm64" in lower:
        arch = "arm64"
    elif "amd64" in lower:
        arch = "amd64"
    else:
        return ""
    if "darwin" in lower:
        return f"macOS / {arch}"
    if "linux" in lower:
        return f"Linux / {arch}"
    return ""


def is_binary(name: str) -> bool:
    """True for release archives, so checksums and metadata do not inflate counts."""
    lower = (name or "").lower()
    return lower.endswith((".tar.gz", ".tgz", ".zip", ".deb", ".rpm", ".dmg", ".exe"))


def github_downloads(repo: str, max_pages: int = 5) -> dict:
    totals = {"releases": 0, "downloads": 0, "platforms": {}}
    for page in range(1, max_pages + 1):
        url = f"{API}/repos/{repo}/releases?per_page=100&page={page}"
        batch = fetch(url)
        if not batch:
            break
        for release in batch:
            if release.get("draft"):
                continue
            totals["releases"] += 1
            for asset in release.get("assets") or []:
                name = asset.get("name", "")
                if not is_binary(name):
                    continue
                count = int(asset.get("download_count") or 0)
                totals["downloads"] += count
                label = platform(name)
                if label:
                    totals["platforms"][label] = totals["platforms"].get(label, 0) + count
        if len(batch) < 100:
            break
    totals["platforms"] = dict(
        sorted(totals["platforms"].items(), key=lambda item: item[1], reverse=True)
    )
    return totals


def homebrew_installs(formula: str = TAP_FORMULA) -> dict:
    """Install counts for the tap formula from Homebrew's public analytics.

    Homebrew publishes one entry per formula installed at least once in the
    window, so a missing entry means zero rather than unknown. Users who set
    HOMEBREW_NO_ANALYTICS are not counted.
    """
    result = {}
    for window in WINDOWS:
        try:
            data = fetch(ANALYTICS.format(window=window))
        except Exception:  # noqa: BLE001 - a failed window should not hide the rest
            continue
        result[window] = next(
            (
                int(item.get("count") or 0)
                for item in data.get("formulae") or []
                if item.get("formula") == formula
            ),
            0,
        )
    return result


def render(downloads: dict, brew: dict) -> str:
    rows = [
        ("releases", str(downloads["releases"])),
        ("downloads", f"{downloads['downloads']:,}"),
    ]
    for label, count in downloads["platforms"].items():
        rows.append(("  " + label, f"{count:,}"))
    for window in WINDOWS:
        if window in brew:
            rows.append((f"homebrew {window}", f"{brew[window]:,}"))

    width = max(len(label) for label, _ in rows)
    lines = ["Workbench install funnel", ""]
    lines += [f"{label:<{width}}  {value:>10}" for label, value in rows]
    if not brew:
        lines += ["", "homebrew installs: unavailable (analytics request failed)"]
    lines += [
        "",
        "Binary archives only; checksums, signatures, and metadata are excluded.",
        "Downloads count every fetch: updates, CI, and mirrors included.",
        "They are a trend line, not a headcount.",
    ]
    return "\n".join(lines)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--repo", default="solomonadzape95/workbench")
    parser.add_argument("--json", action="store_true", help="machine-readable output")
    args = parser.parse_args()

    try:
        downloads = github_downloads(args.repo)
    except Exception as error:  # noqa: BLE001 - any network/API failure is fatal here
        print(f"funnel: could not read releases for {args.repo}: {error}", file=sys.stderr)
        print("funnel: a GITHUB_TOKEN raises the 60/hour unauthenticated limit", file=sys.stderr)
        return 1

    brew = homebrew_installs()

    if args.json:
        payload = {"repo": args.repo, "github": downloads, "homebrew": brew}
        print(json.dumps(payload, indent=2))
    else:
        print(render(downloads, brew))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
