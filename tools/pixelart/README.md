# Pixel wordmark renderer

This standalone Node.js tool generates transparent SVG and RGBA PNG logos from terminal-art rows. It uses only Node's standard library: no fonts, browser, image editor, network access or npm dependencies. Node 22 or newer is supported.

From the AXLR repository root:

```bash
node tools/pixelart/cli.mjs
node tools/pixelart/cli.mjs --check
node --test tools/pixelart/render.test.mjs
```

The default command generates the six assets in `docs/assets/brand/`. `--check` performs no writes and rejects missing or outdated exports. PNG comparison uses pixels rather than compressed bytes so zlib version differences do not cause false failures.

```bash
# One logo, double-resolution raster and SVG display dimensions.
node tools/pixelart/cli.mjs --logo axlr --scale 2 --out /tmp/axlr-brand

# A site can render directly into its public asset directory.
node tools/pixelart/cli.mjs --out /absolute/site/public/brands
```

## Edit a logo

[`logos.json`](logos.json) is the source of truth. Each definition has a canvas, origin, cell size, terminal-art rows, ink rules and optional Spectrum placement. Each product has its own four-color palette; all three share the diagonal bar geometry. Row ink gives KMP its existing sweep, using copper twice, gold once, brown once and sand twice. Column runs assign the four colors in order to MADE and AXLR's letters. Spaces count as columns. Use only `█`, `╔`, `╗`, `╚`, `╝`, `║`, `═` and space; the renderer expands them into explicit rectangles, independently of installed fonts.

`spectrum.layout: "square"` stacks the four slanted bars vertically in a 64 × 64 unit block, scaled by `spectrum.scale`. Each bar is 13 units high, with 4-unit transparent gaps. All three products use this block below the wordmark, aligned with its right edge. Omitting `layout`, or setting `"horizontal"`, preserves the original 175 × 40 unit arrangement for other uses.

`spectrum.colors` specifies exactly four palette keys, in top-to-bottom order for the square and left-to-right order for the horizontal arrangement. Those keys can differ between logos without changing their geometry. Older manifests that omit this field use `red`, `yellow`, `green` and `cyan`, which must exist in their palette. The current product palettes are listed in the [asset pack](../../docs/assets/brand/README.md).

KMP's rows and geometry match its current public SVG and CLI mark. MADE's rows and geometry match its distinct current public SVG, with secondary text removed. AXLR's rows reconstruct the supplied raster's letters, cell proportions and stepped outlines as vector shapes. The original raster stays in `docs/assets/axlr-wordmark.png` as a visual reference. Exports intentionally have solid pixel edges and transparent gaps; no dark matte or embedded bitmap is included.

To reuse the tool elsewhere, copy this directory, add a logo definition and run `cli.mjs --manifest /path/logos.json --out /path/public/brands`. An unknown glyph, palette reference, unsafe ID or drawing outside the canvas fails before any export is written. PNG scale is an integer from 1 to 8, with a 64 million pixel limit.

## Use the renderer in a build

```js
import { readFileSync, writeFileSync } from "node:fs";
import { validateManifest, buildScene, renderSVG, renderPNG } from "./tools/pixelart/render.mjs";

const pack = validateManifest(JSON.parse(readFileSync("./tools/pixelart/logos.json", "utf8")));
const scene = buildScene(pack, pack.logos.find(logo => logo.id === "axlr"));
writeFileSync("./public/axlr.svg", renderSVG(scene));
writeFileSync("./public/axlr.png", renderPNG(scene, 2));
```

SVG dimensions and PNG geometry come from the same scene. The SVG contains vector rectangles and polygons with `shape-rendering="crispEdges"`; the PNG uses pixel-center coverage without smoothing. Keep the aspect ratio and complete wordmark when embedding either format.
