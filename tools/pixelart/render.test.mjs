import assert from "node:assert/strict";
import { test } from "node:test";
import { readFileSync, mkdtempSync, writeFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";
import {
  validateManifest,
  buildScene,
  renderSVG,
  rasterize,
  renderPNG,
  pngPixelContent,
} from "./render.mjs";

const manifest = JSON.parse(
  readFileSync(new URL("logos.json", import.meta.url), "utf8"),
);
const copy = () => structuredClone(manifest);
const brandColors = {
  kmp: ["#C96B2C", "#D89A3D", "#7A5230", "#D8C3A5"],
  made: ["#B6633A", "#D98545", "#5A3E2B", "#BFA27A"],
  axlr: ["#B85C38", "#D97B2D", "#6B4A2E", "#C9B08C"],
};
const rgba = (hex) => [...Buffer.from(hex.slice(1), "hex"), 255];

test("source glyphs preserve terminal columns, per-letter ink and transparent counters", () => {
  validateManifest(manifest);
  for (const logo of manifest.logos) {
    const scene = buildScene(manifest, logo);
    const { width, pixels } = rasterize(scene);
    const pixel = (x, y) =>
      Array.from(pixels.subarray((y * width + x) * 4, (y * width + x) * 4 + 4));
    assert.deepEqual(pixel(0, 0), [0, 0, 0, 0]);
    const copperSample = { kmp: [45, 21], made: [49, 53], axlr: [128, 38] }[
      logo.id
    ];
    assert.deepEqual(pixel(...copperSample), rgba(brandColors[logo.id][0]));
    assert.deepEqual(pixel(logo.width - 1, logo.height - 1), [0, 0, 0, 0]);
    assert.equal(
      scene.shapes.filter((shape) => shape.type === "polygon").length,
      4,
    );
    assert.deepEqual(
      scene.shapes.slice(-4).map((shape) => shape.color),
      brandColors[logo.id],
    );
    assert.deepEqual(
      [...new Set(scene.shapes.map((shape) => shape.color))],
      brandColors[logo.id],
      "lettering and bars use only the four product colors in order",
    );
    for (let i = 3; i < pixels.length; i += 4)
      assert.ok(pixels[i] === 0 || pixels[i] === 255);
  }
  const axlr = buildScene(
    manifest,
    manifest.logos.find((logo) => logo.id === "axlr"),
  );
  const blocks = axlr.shapes.filter(
    (shape) =>
      shape.type === "rect" && shape.y === 37 && shape.width === 44.875,
  );
  assert.equal(blocks.filter((shape) => shape.color === "#D97B2D").length, 4);
  assert.equal(blocks.find((shape) => shape.color === "#6B4A2E").x, 800);
  assert.equal(blocks.find((shape) => shape.color === "#C9B08C").x, 1159);
  const made = buildScene(
    manifest,
    manifest.logos.find((logo) => logo.id === "made"),
  );
  const { pixels, width } = rasterize(made);
  assert.equal(
    pixels[(140 * width + 590) * 4 + 3],
    0,
    "D's counter stays transparent",
  );
});

test("older manifests retain default Spectrum colors when no bar ink is specified", () => {
  const legacy = copy();
  Object.assign(legacy.palette, {
    red: "#f04445",
    yellow: "#ffda36",
    green: "#32cd76",
    cyan: "#42d4ed",
  });
  delete legacy.logos[0].spectrum.colors;
  validateManifest(legacy);
  assert.deepEqual(
    buildScene(legacy, legacy.logos[0])
      .shapes.slice(-4)
      .map((shape) => shape.color),
    ["#f04445", "#ffda36", "#32cd76", "#42d4ed"],
  );
});

test("SVG escapes labels, has vector shapes only and shares the PNG's dimensions", () => {
  const scene = buildScene(manifest, manifest.logos[0]);
  scene.title = 'KMP <test> & "logo"';
  const svg = renderSVG(scene, 2).toString();
  assert.match(svg, /width="1424" height="600" viewBox="0 0 712 300"/);
  assert.match(svg, /KMP &lt;test&gt; &amp; &quot;logo&quot;/);
  assert.doesNotMatch(svg, /<image|<text|#101114|by Underpass/);
  assert.equal((svg.match(/<polygon /g) ?? []).length, 4);
});

test("RGBA PNG round-trips, scales crisply and checks pixels independently of compression", () => {
  const scene = {
    width: 4,
    height: 3,
    title: "test",
    shapes: [
      { type: "rect", x: 1, y: 1, width: 1, height: 1, color: "#f04445" },
    ],
  };
  const png = renderPNG(scene, 2);
  assert.equal(png.readUInt32BE(16), 8);
  assert.equal(png.readUInt32BE(20), 6);
  const content = pngPixelContent(png);
  assert.equal(content.subarray(0, 4).toString(), "8x6:");
  assert.equal(pngPixelContent(renderPNG(scene, 2)).equals(content), true);
  const scaled = rasterize(scene, 2);
  const pixel = (x, y) =>
    Array.from(
      scaled.pixels.subarray(
        (y * scaled.width + x) * 4,
        (y * scaled.width + x) * 4 + 4,
      ),
    );
  assert.deepEqual(pixel(2, 2), [240, 68, 69, 255]);
  assert.deepEqual(pixel(3, 3), [240, 68, 69, 255]);
  assert.deepEqual(pixel(1, 2), [0, 0, 0, 0]);
  assert.deepEqual(pixel(4, 3), [0, 0, 0, 0]);
  assert.throws(() => rasterize(scene, 0), /Scale/);
  assert.throws(() => rasterize(scene, 1.5), /Scale/);
  assert.throws(() => renderSVG(scene, -1), /Scale/);
  const corrupted = Buffer.from(png);
  corrupted[45] ^= 1;
  assert.throws(() => pngPixelContent(corrupted), /checksum/);
  assert.throws(() => pngPixelContent(png.subarray(0, 24)), /Truncated/);
  assert.throws(() => pngPixelContent(Buffer.from("not a png")), /signature/);
});

test("bad definitions fail before rendering or allocating image buffers", () => {
  const cases = [
    [(m) => (m.version = 2), /version/],
    [(m) => (m.logos[0].id = "../escape"), /ID/],
    [(m) => (m.logos[1].id = "kmp"), /duplicate/],
    [(m) => (m.logos[0].width = 0), /canvas/],
    [(m) => (m.palette.red = "url(secret)"), /palette/],
    [(m) => (m.logos[0].rows[0] += "A"), /glyph/],
    [(m) => (m.logos[0].ink.rows[0] = "missing"), /ink/],
    [(m) => m.logos[0].ink.rows.pop(), /count/],
    [(m) => (m.logos[1].ink.runs[0].columns = 1), /insufficient/],
    [(m) => (m.logos[1].ink.runs[0].columns = 1000000), /column run/],
    [(m) => (m.logos[0].origin[0] = 700), /outside/],
    [(m) => (m.logos[0].spectrum.scale = 100), /outside/],
    [(m) => (m.logos[0].spectrum.colors[0] = "missing"), /Spectrum/],
    [(m) => m.logos[0].spectrum.colors.pop(), /Spectrum/],
    [(m) => (m.logos[0].spectrum.colors = "kmp-copper"), /Spectrum/],
  ];
  for (const [mutate, expected] of cases) {
    const value = copy();
    mutate(value);
    assert.throws(() => validateManifest(value), expected);
  }
});

test("CLI exports every vector/raster, detects drift and never writes in check mode", () => {
  const dir = mkdtempSync(join(tmpdir(), "axlr-pixelart-test-"));
  const cli = fileURLToPath(new URL("cli.mjs", import.meta.url));
  const run = (args) =>
    spawnSync(process.execPath, [cli, ...args], { encoding: "utf8" });
  try {
    assert.equal(run(["--out", dir]).status, 0);
    assert.equal(run(["--out", dir, "--check"]).status, 0);
    const svg = join(dir, "axlr-spectrum.svg");
    writeFileSync(svg, "changed");
    const check = run(["--out", dir, "--check"]);
    assert.equal(check.status, 1);
    assert.match(check.stderr, /Outdated export/);
    assert.equal(readFileSync(svg, "utf8"), "changed");
    assert.equal(run(["--logo", "absent", "--out", dir]).status, 1);
    assert.equal(run(["--scale", "-1", "--out", dir]).status, 1);
    assert.equal(run(["--format", "gif", "--out", dir]).status, 1);
    assert.equal(run(["--unknown"]).status, 1);
    assert.equal(run(["--help"]).status, 0);
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});
