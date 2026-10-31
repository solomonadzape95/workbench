# Workbench website

Static site for the project. No build step and no JavaScript framework.

```sh
make site
# open http://localhost:8080
```

`index.html`, `styles.css`, and the dither logo are the whole site. It is
deployed to GitHub Pages by `.github/workflows/pages.yml` on changes under
`site/`.

The visual language follows the TUI's default OpenCode theme: near-black
panels, a single amber accent, mono type, rounded borders, and the dither mark.
