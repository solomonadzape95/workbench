# Workbench website

Static site for the project. No build step and no JavaScript framework.

```sh
make site
# open http://localhost:8080
```

`index.html`, `styles.css`, `dither.js`, `keys.js`, `tui.js`, `reveal.js`, and
the Maple Mono webfonts are the whole site. The canvas shader paints a
Bayer-dithered amber field; pressing `i`, `d`, `g`, `r`, `a`, or `?` follows the
matching link. Internal commands open a scrollable TUI-style panel inside the
page; only GitHub and release downloads leave the site. It is deployed to
GitHub Pages by `.github/workflows/pages.yml` on changes under `site/`.

The visual language follows the TUI's default OpenCode theme: near-black
panels, a single amber accent, mono type, rounded borders, and the dither mark.

## Domain

The site is published at `https://workbench.thesolenoid.space`. The Pages site
has a `CNAME` for that host; the DNS provider needs one record:

```text
Type   Name        Value
CNAME  workbench   solomonadzape95.github.io
```

Once DNS resolves, enable HTTPS in the repository's Pages settings.
