import { deflateSync, inflateSync } from "node:zlib";

// Cell geometry is expressed in a 24 × 36 terminal cell. The outlines are
// positive shapes, so counters and gaps remain transparent on every background.
const GLYPHS = {
  " ": [],
  "█": [[0, 0, 24, 36]],
  "╗": [
    [0, 14, 18, 4],
    [14, 14, 4, 22],
    [0, 22, 10, 4],
    [6, 22, 4, 14],
  ],
  "╔": [
    [6, 14, 18, 4],
    [6, 14, 4, 22],
    [14, 22, 10, 4],
    [14, 22, 4, 14],
  ],
  "╝": [
    [14, 0, 4, 26],
    [0, 22, 18, 4],
    [6, 0, 4, 18],
    [0, 14, 10, 4],
  ],
  "╚": [
    [6, 0, 4, 26],
    [6, 22, 18, 4],
    [14, 0, 4, 18],
    [14, 14, 10, 4],
  ],
  "║": [
    [6, 0, 4, 36],
    [14, 0, 4, 36],
  ],
  "═": [
    [0, 14, 24, 4],
    [0, 22, 24, 4],
  ],
};

const BARS = [
  [
    [0, 40],
    [36, 0],
    [67, 0],
    [31, 40],
  ],
  [
    [36, 40],
    [72, 0],
    [103, 0],
    [67, 40],
  ],
  [
    [72, 40],
    [108, 0],
    [139, 0],
    [103, 40],
  ],
  [
    [108, 40],
    [144, 0],
    [175, 0],
    [139, 40],
  ],
];
// Four straight stripes, 13 units high with 4-unit gaps, occupy a 64 × 64 square.
const SQUARE_BARS = Array.from({ length: 4 }, (_, i) => [
  [0, i * 17 + 13],
  [0, i * 17],
  [64, i * 17],
  [64, i * 17 + 13],
]);
// Legacy manifests without explicit bar colors keep the original Spectrum ink.
const DEFAULT_BAR_COLORS = ["red", "yellow", "green", "cyan"];
const number = (value) => Number(value.toFixed(6)).toString();
const escapeXML = (value) =>
  value.replace(
    /[<>&"']/g,
    (c) =>
      ({
        "<": "&lt;",
        ">": "&gt;",
        "&": "&amp;",
        '"': "&quot;",
        "'": "&apos;",
      })[c],
  );

function requireValue(condition, message) {
  if (!condition) throw new Error(message);
}

function pair(value, name, positive = false) {
  requireValue(
    Array.isArray(value) &&
      value.length === 2 &&
      value.every((v) => Number.isFinite(v) && (positive ? v > 0 : v >= 0)),
    `Invalid ${name}`,
  );
}

export function validateManifest(manifest) {
  requireValue(manifest?.version === 1, "Unsupported manifest version");
  requireValue(
    manifest.palette && typeof manifest.palette === "object",
    "Missing palette",
  );
  for (const [name, color] of Object.entries(manifest.palette)) {
    requireValue(
      /^[a-z][a-z0-9_-]*$/.test(name) && /^#[0-9a-f]{6}$/i.test(color),
      `Invalid palette color: ${name}`,
    );
  }
  requireValue(
    Array.isArray(manifest.logos) && manifest.logos.length > 0,
    "Missing logos",
  );
  const ids = new Set();
  for (const logo of manifest.logos) {
    requireValue(
      typeof logo.id === "string" &&
        /^[a-z][a-z0-9_-]*$/.test(logo.id) &&
        !ids.has(logo.id),
      "Invalid or duplicate logo ID",
    );
    ids.add(logo.id);
    requireValue(
      typeof logo.title === "string" &&
        logo.title.length > 0 &&
        logo.title.length <= 256,
      `${logo.id}: invalid title`,
    );
    requireValue(
      [logo.width, logo.height].every(
        (v) => Number.isInteger(v) && v > 0 && v <= 8192,
      ) && logo.width * logo.height <= 32_000_000,
      `${logo.id}: invalid canvas size`,
    );
    pair(logo.origin, `${logo.id} origin`);
    pair(logo.cell, `${logo.id} cell`, true);
    requireValue(
      Array.isArray(logo.rows) &&
        logo.rows.length > 0 &&
        logo.rows.length <= 256,
      `${logo.id}: missing rows`,
    );
    for (const row of logo.rows) {
      requireValue(
        typeof row === "string" && Array.from(row).length <= 256,
        `${logo.id}: invalid row`,
      );
      for (const glyph of row)
        requireValue(
          Object.hasOwn(GLYPHS, glyph),
          `${logo.id}: unsupported glyph ${glyph}`,
        );
    }
    const rowInk = logo.ink?.rows;
    const runs = logo.ink?.runs;
    requireValue(
      Boolean(rowInk) !== Boolean(runs),
      `${logo.id}: choose row ink or column runs`,
    );
    const colors = rowInk ?? runs?.map((run) => run.color);
    requireValue(
      Array.isArray(colors) &&
        colors.length > 0 &&
        colors.every((c) => Object.hasOwn(manifest.palette, c)),
      `${logo.id}: unknown ink color`,
    );
    if (rowInk)
      requireValue(
        rowInk.length === logo.rows.length,
        `${logo.id}: row ink count mismatch`,
      );
    if (runs) {
      requireValue(
        runs.every(
          (run) =>
            Number.isInteger(run.columns) &&
            run.columns > 0 &&
            run.columns <= 256,
        ),
        `${logo.id}: invalid column run`,
      );
      const columns = runs.reduce((sum, run) => sum + run.columns, 0);
      requireValue(columns <= 256, `${logo.id}: too many column runs`);
      requireValue(
        logo.rows.every((row) => Array.from(row).length <= columns),
        `${logo.id}: insufficient column ink`,
      );
    }
    if (logo.spectrum) {
      requireValue(
        ["horizontal", "square"].includes(logo.spectrum.layout ?? "horizontal"),
        `${logo.id}: invalid Spectrum layout`,
      );
      pair(logo.spectrum.origin, `${logo.id} Spectrum origin`);
      requireValue(
        Number.isFinite(logo.spectrum.scale) && logo.spectrum.scale > 0,
        `${logo.id}: invalid Spectrum scale`,
      );
      requireValue(
        Array.isArray(logo.spectrum.colors ?? DEFAULT_BAR_COLORS) &&
          (logo.spectrum.colors ?? DEFAULT_BAR_COLORS).length === BARS.length &&
          (logo.spectrum.colors ?? DEFAULT_BAR_COLORS).every((c) =>
            Object.hasOwn(manifest.palette, c),
          ),
        `${logo.id}: Spectrum needs four known palette colors`,
      );
    }
    // Compile once during validation to reject clipped drawings before export.
    const scene = buildScene(manifest, logo);
    for (const shape of scene.shapes) {
      const points =
        shape.type === "rect"
          ? [
              [shape.x, shape.y],
              [shape.x + shape.width, shape.y + shape.height],
            ]
          : shape.points;
      requireValue(
        points.every(
          ([x, y]) => x >= 0 && y >= 0 && x <= logo.width && y <= logo.height,
        ),
        `${logo.id}: drawing outside canvas`,
      );
    }
  }
  return manifest;
}

export function buildScene(manifest, logo) {
  const shapes = [];
  const columnColors = logo.ink.runs?.flatMap((run) =>
    Array(run.columns).fill(run.color),
  );
  logo.rows.forEach((row, y) => {
    Array.from(row).forEach((glyph, x) => {
      const color = manifest.palette[logo.ink.rows?.[y] ?? columnColors[x]];
      for (const [gx, gy, width, height] of GLYPHS[glyph]) {
        shapes.push({
          type: "rect",
          x: logo.origin[0] + (x + gx / 24) * logo.cell[0],
          y: logo.origin[1] + (y + gy / 36) * logo.cell[1],
          width: (width / 24) * logo.cell[0],
          height: (height / 36) * logo.cell[1],
          color,
        });
      }
    });
  });
  if (logo.spectrum) {
    const bars = logo.spectrum.layout === "square" ? SQUARE_BARS : BARS;
    bars.forEach((points, i) =>
      shapes.push({
        type: "polygon",
        color:
          manifest.palette[(logo.spectrum.colors ?? DEFAULT_BAR_COLORS)[i]],
        points: points.map(([x, y]) => [
          logo.spectrum.origin[0] + x * logo.spectrum.scale,
          logo.spectrum.origin[1] + y * logo.spectrum.scale,
        ]),
      }),
    );
  }
  return {
    id: logo.id,
    width: logo.width,
    height: logo.height,
    title: logo.title,
    spectrum: Boolean(logo.spectrum),
    shapes,
  };
}

export function renderSVG(scene, scale = 1) {
  validateScale(scene, scale);
  const label = escapeXML(scene.id ?? "wordmark");
  const shapes = scene.shapes.map((shape) =>
    shape.type === "rect"
      ? `    <rect x="${number(shape.x)}" y="${number(shape.y)}" width="${number(shape.width)}" height="${number(shape.height)}" fill="${shape.color}"/>`
      : `    <polygon points="${shape.points.map((p) => p.map(number).join(",")).join(" ")}" fill="${shape.color}"/>`,
  );
  return Buffer.from(
    `<svg xmlns="http://www.w3.org/2000/svg" width="${scene.width * scale}" height="${scene.height * scale}" viewBox="0 0 ${scene.width} ${scene.height}" role="img" aria-labelledby="${label}-title ${label}-desc">\n  <title id="${label}-title">${escapeXML(scene.title)}</title>\n  <desc id="${label}-desc">${escapeXML(scene.title)} pixel wordmark${scene.spectrum ? " with Spectrum bars" : ""}. Transparent background.</desc>\n  <g shape-rendering="crispEdges">\n${shapes.join("\n")}\n  </g>\n</svg>\n`,
  );
}

function insidePolygon(x, y, points) {
  let inside = false;
  for (let i = 0, j = points.length - 1; i < points.length; j = i++) {
    const [xi, yi] = points[i],
      [xj, yj] = points[j];
    if (yi > y !== yj > y && x < ((xj - xi) * (y - yi)) / (yj - yi) + xi)
      inside = !inside;
  }
  return inside;
}

function validateScale(scene, scale) {
  requireValue(
    Number.isInteger(scale) &&
      scale >= 1 &&
      scale <= 8 &&
      scene.width * scene.height * scale * scale <= 64_000_000,
    "Scale must be 1–8 and output at most 64 million pixels",
  );
}

export function rasterize(scene, scale = 1) {
  validateScale(scene, scale);
  const width = scene.width * scale,
    height = scene.height * scale;
  const pixels = Buffer.alloc(width * height * 4);
  for (const shape of scene.shapes) {
    const color = [1, 3, 5].map((i) =>
      parseInt(shape.color.slice(i, i + 2), 16),
    );
    const points =
      shape.type === "rect"
        ? [
            [shape.x, shape.y],
            [shape.x + shape.width, shape.y + shape.height],
          ]
        : shape.points;
    const left = Math.max(
      0,
      Math.ceil(Math.min(...points.map((p) => p[0])) * scale - 0.5),
    );
    const top = Math.max(
      0,
      Math.ceil(Math.min(...points.map((p) => p[1])) * scale - 0.5),
    );
    const right = Math.min(
      width,
      Math.ceil(Math.max(...points.map((p) => p[0])) * scale - 0.5),
    );
    const bottom = Math.min(
      height,
      Math.ceil(Math.max(...points.map((p) => p[1])) * scale - 0.5),
    );
    for (let y = top; y < bottom; y++)
      for (let x = left; x < right; x++) {
        if (
          shape.type === "polygon" &&
          !insidePolygon((x + 0.5) / scale, (y + 0.5) / scale, shape.points)
        )
          continue;
        const offset = (y * width + x) * 4;
        pixels[offset] = color[0];
        pixels[offset + 1] = color[1];
        pixels[offset + 2] = color[2];
        pixels[offset + 3] = 255;
      }
  }
  return { width, height, pixels };
}

function crc32(bytes) {
  let crc = 0xffffffff;
  for (const byte of bytes) {
    crc ^= byte;
    for (let bit = 0; bit < 8; bit++)
      crc = (crc >>> 1) ^ (crc & 1 ? 0xedb88320 : 0);
  }
  return (crc ^ 0xffffffff) >>> 0;
}

function chunk(name, data) {
  const type = Buffer.from(name),
    length = Buffer.alloc(4),
    crc = Buffer.alloc(4);
  length.writeUInt32BE(data.length);
  crc.writeUInt32BE(crc32(Buffer.concat([type, data])));
  return Buffer.concat([length, type, data, crc]);
}

export function renderPNG(scene, scale = 1) {
  const { width, height, pixels } = rasterize(scene, scale);
  const header = Buffer.alloc(13);
  header.writeUInt32BE(width, 0);
  header.writeUInt32BE(height, 4);
  header[8] = 8;
  header[9] = 6;
  const stride = width * 4,
    rows = Buffer.alloc((stride + 1) * height);
  for (let y = 0; y < height; y++)
    pixels.copy(rows, y * (stride + 1) + 1, y * stride, (y + 1) * stride);
  return Buffer.concat([
    Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]),
    chunk("IHDR", header),
    chunk("IDAT", deflateSync(rows, { level: 9 })),
    chunk("IEND", Buffer.alloc(0)),
  ]);
}

// Compare pixel content instead of zlib bytes: compression versions may differ.
export function pngPixelContent(bytes) {
  requireValue(
    bytes.subarray(0, 8).equals(Buffer.from([137, 80, 78, 71, 13, 10, 26, 10])),
    "Invalid PNG signature",
  );
  const idat = [];
  let width,
    height,
    ended = false;
  for (let offset = 8; offset < bytes.length; ) {
    requireValue(offset + 12 <= bytes.length, "Truncated PNG chunk");
    const length = bytes.readUInt32BE(offset),
      type = bytes.toString("ascii", offset + 4, offset + 8);
    requireValue(offset + 12 + length <= bytes.length, "Truncated PNG data");
    const data = bytes.subarray(offset + 8, offset + 8 + length);
    requireValue(
      bytes.readUInt32BE(offset + 8 + length) ===
        crc32(bytes.subarray(offset + 4, offset + 8 + length)),
      "PNG checksum mismatch",
    );
    if (type === "IHDR") {
      requireValue(offset === 8 && !width, "Expected one leading PNG header");
      requireValue(
        length === 13 &&
          data[8] === 8 &&
          data[9] === 6 &&
          data[10] === 0 &&
          data[11] === 0 &&
          data[12] === 0,
        "Expected noninterlaced RGBA PNG",
      );
      width = data.readUInt32BE(0);
      height = data.readUInt32BE(4);
      requireValue(
        width > 0 && height > 0 && width * height <= 64_000_000,
        "Invalid PNG dimensions",
      );
    }
    if (type === "IDAT") idat.push(data);
    if (type === "IEND") {
      requireValue(
        length === 0 && offset + 12 === bytes.length,
        "Invalid PNG end marker",
      );
      ended = true;
    }
    offset += length + 12;
  }
  requireValue(width && height && idat.length && ended, "Incomplete PNG");
  const rows = inflateSync(Buffer.concat(idat), {
    maxOutputLength: (width * 4 + 1) * height,
  });
  requireValue(
    rows.length === (width * 4 + 1) * height,
    "PNG scanline size mismatch",
  );
  for (let y = 0; y < height; y++)
    requireValue(
      rows[y * (width * 4 + 1)] === 0,
      "Expected renderer PNG filter 0",
    );
  return Buffer.concat([Buffer.from(`${width}x${height}:`), rows]);
}
