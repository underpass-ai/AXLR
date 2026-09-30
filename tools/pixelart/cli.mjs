#!/usr/bin/env node
import { readFileSync, writeFileSync, mkdirSync } from "node:fs";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";
import {
  validateManifest,
  buildScene,
  renderSVG,
  renderPNG,
  pngPixelContent,
} from "./render.mjs";

const help = `Usage: node tools/pixelart/cli.mjs [options]
  --manifest FILE  Logo definitions (default: logos.json beside this script)
  --out DIR        Output directory (default: docs/assets/brand)
  --logo ID        Render one logo (default: all logos)
  --format LIST    svg,png or either format (default: svg,png)
  --scale INTEGER  Export scale, 1–8 (default: 1; PNG pixel count is bounded)
  --check          Verify existing exports without writing files
  --help          Show this help
`;

function main(args) {
  const options = {
    manifest: fileURLToPath(new URL("logos.json", import.meta.url)),
    out: "docs/assets/brand",
    format: "svg,png",
    scale: "1",
  };
  for (let i = 0; i < args.length; i++) {
    const name = args[i];
    if (name === "--help") {
      process.stdout.write(help);
      return;
    }
    if (name === "--check") {
      options.check = true;
      continue;
    }
    if (
      !["--manifest", "--out", "--logo", "--format", "--scale"].includes(
        name,
      ) ||
      !args[i + 1] ||
      args[i + 1].startsWith("--")
    )
      throw new Error(`Invalid option: ${name}`);
    options[name.slice(2)] = args[++i];
  }
  const scale = Number(options.scale);
  if (!Number.isInteger(scale) || scale < 1 || scale > 8)
    throw new Error("Scale must be an integer from 1 to 8");
  const formats = [...new Set(options.format.split(","))];
  if (formats.some((format) => !["svg", "png"].includes(format)))
    throw new Error("Format must be svg, png or svg,png");
  const manifest = validateManifest(
    JSON.parse(readFileSync(options.manifest, "utf8")),
  );
  const logos = options.logo
    ? manifest.logos.filter((logo) => logo.id === options.logo)
    : manifest.logos;
  if (!logos.length) throw new Error(`Unknown logo: ${options.logo}`);
  // Render and check everything before touching the output directory.
  const exports = logos.flatMap((logo) => {
    const scene = buildScene(manifest, logo);
    return formats.map((format) => ({
      path: resolve(options.out, `${logo.id}-spectrum.${format}`),
      format,
      bytes:
        format === "svg" ? renderSVG(scene, scale) : renderPNG(scene, scale),
    }));
  });
  if (options.check) {
    for (const asset of exports) {
      let actual;
      try {
        actual = readFileSync(asset.path);
      } catch {
        throw new Error(`Missing export: ${asset.path}`);
      }
      const equal =
        asset.format === "png"
          ? pngPixelContent(actual).equals(pngPixelContent(asset.bytes))
          : actual.equals(asset.bytes);
      if (!equal) throw new Error(`Outdated export: ${asset.path}`);
    }
    process.stdout.write(`Verified ${exports.length} exports.\n`);
    return;
  }
  mkdirSync(options.out, { recursive: true });
  for (const asset of exports) {
    writeFileSync(asset.path, asset.bytes);
    process.stdout.write(`${asset.path}\n`);
  }
}

try {
  main(process.argv.slice(2));
} catch (error) {
  process.stderr.write(`pixelart: ${error.message}\n`);
  process.exitCode = 1;
}
