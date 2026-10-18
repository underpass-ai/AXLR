#!/usr/bin/env python3
"""Read the dimensions, format, bit depth and colour type of an image from its header, without
decoding pixels: PNG (IHDR and the chunk list), JPEG (SOFn markers, JFIF, Exif APP1 presence,
Adobe APP14), GIF (logical screen, frames, transparency, loop), BMP (all DIB header versions)
and WebP (VP8, VP8L, VP8X).

Input (JSON on stdin):
  hex    the file's bytes as hexadecimal (spaces and newlines allowed), or
  path   a relative path to the file inside the working directory
"""
import json
import os
import re
import struct
import sys
import zlib

MAX_BYTES = 64 * 1024 * 1024
PNG_SIG = bytes.fromhex("89504e470d0a1a0a")
PNG_TYPES = {0: ("greyscale", 1, (1, 2, 4, 8, 16)), 2: ("truecolour (RGB)", 3, (8, 16)),
             3: ("indexed-colour (palette)", 1, (1, 2, 4, 8)), 4: ("greyscale with alpha", 2, (8, 16)),
             6: ("truecolour with alpha (RGBA)", 4, (8, 16))}
SOF = {0xC0: "baseline DCT", 0xC1: "extended sequential DCT", 0xC2: "progressive DCT", 0xC3: "lossless",
       0xC5: "differential sequential DCT", 0xC6: "differential progressive DCT", 0xC7: "differential lossless",
       0xC9: "extended sequential DCT, arithmetic", 0xCA: "progressive DCT, arithmetic", 0xCB: "lossless, arithmetic",
       0xCD: "differential sequential DCT, arithmetic", 0xCE: "differential progressive DCT, arithmetic",
       0xCF: "differential lossless, arithmetic"}
BMP_HEADERS = {12: "BITMAPCOREHEADER", 16: "OS22XBITMAPHEADER (short)", 40: "BITMAPINFOHEADER", 52: "BITMAPV2INFOHEADER",
               56: "BITMAPV3INFOHEADER", 64: "OS22XBITMAPHEADER", 108: "BITMAPV4HEADER", 124: "BITMAPV5HEADER"}
BMP_COMPRESSION = {0: "BI_RGB (none)", 1: "BI_RLE8", 2: "BI_RLE4", 3: "BI_BITFIELDS", 4: "BI_JPEG", 5: "BI_PNG",
                   6: "BI_ALPHABITFIELDS", 11: "BI_CMYK", 12: "BI_CMYKRLE8", 13: "BI_CMYKRLE4"}


class Bad(Exception):
    pass


def fail(message):
    print(json.dumps({"error": message}))
    sys.exit(2)


def load(request):
    if ("hex" in request) == ("path" in request):
        fail("give hex (the bytes) or path (a relative file), not both")
    if "hex" in request:
        text = request["hex"]
        if not isinstance(text, str):
            fail("hex must be a string")
        text = re.sub(r"\s+", "", text)
        if text[:2].lower() == "0x":
            text = text[2:]
        if len(text) > 2 * MAX_BYTES:
            fail("the image may be at most 64 MiB")
        try:
            return bytes.fromhex(text)
        except ValueError:
            fail("hex is not valid hexadecimal")
    path = request["path"]
    if not isinstance(path, str) or not path or os.path.isabs(path) or ".." in re.split(r"[\\/]", path):
        fail("path must be a relative path inside the working directory, without ..")
    full = os.path.realpath(path)
    if not full.startswith(os.path.realpath(os.getcwd()) + os.sep):
        fail("path must stay inside the working directory")
    try:
        with open(full, "rb") as handle:
            data = handle.read(MAX_BYTES + 1)
    except OSError as error:
        fail(f"cannot read {path}: {error.strerror}")
    if len(data) > MAX_BYTES:
        fail("the image may be at most 64 MiB")
    return data


def need(data, end, what):
    if len(data) < end:
        raise Bad(f"truncated {what}: needs {end} bytes, has {len(data)}")


def png(data):
    need(data, 33, "PNG IHDR")
    length, kind = struct.unpack(">I4s", data[8:16])
    if kind != b"IHDR" or length != 13:
        raise Bad("the first PNG chunk must be IHDR of length 13")
    width, height, depth, ctype, comp, filt, interlace = struct.unpack(">IIBBBBB", data[16:29])
    if ctype not in PNG_TYPES:
        raise Bad(f"invalid PNG colour type {ctype}")
    name, channels, depths = PNG_TYPES[ctype]
    crc_ok = zlib.crc32(data[12:29]) & 0xFFFFFFFF == struct.unpack(">I", data[29:33])[0]
    out = {"format": "PNG", "mime": "image/png", "width": width, "height": height, "bit_depth": depth,
           "colour_type": ctype, "colour_type_name": name, "channels": channels, "bits_per_pixel": depth * channels,
           "interlace": "Adam7" if interlace == 1 else "none" if interlace == 0 else f"invalid ({interlace})",
           "compression_method": comp, "filter_method": filt, "ihdr_crc_ok": crc_ok,
           "has_alpha": ctype in (4, 6)}
    problems = []
    if depth not in depths:
        problems.append(f"bit depth {depth} is not allowed for colour type {ctype}")
    if width == 0 or height == 0 or width > 2 ** 31 - 1 or height > 2 ** 31 - 1:
        problems.append("width and height must be 1 to 2^31-1")
    chunks, pos = [], 8
    while pos + 8 <= len(data) and len(chunks) < 10000:
        length, kind = struct.unpack(">I4s", data[pos:pos + 8])
        name_ = kind.decode("latin-1")
        body = data[pos + 8:pos + 8 + length]
        chunks.append(name_)
        if name_ == "tRNS":
            out["has_alpha"] = True
            out["transparency"] = "tRNS chunk"
        elif name_ == "PLTE":
            out["palette_entries"] = length // 3
        elif name_ == "pHYs" and len(body) == 9:
            ppx, ppy, unit = struct.unpack(">IIB", body)
            out["pixels_per_unit"] = {"x": ppx, "y": ppy, "unit": "metre" if unit == 1 else "unknown (aspect ratio only)"}
            if unit == 1:
                out["dpi"] = {"x": round(ppx * 0.0254, 2), "y": round(ppy * 0.0254, 2)}
        elif name_ == "acTL" and len(body) == 8:
            frames, plays = struct.unpack(">II", body)
            out["animated"] = True
            out["frames"] = frames
            out["loop_count"] = plays
        elif name_ in ("eXIf",):
            out["exif"] = True
        pos += 12 + length
        if name_ == "IEND":
            break
    out["chunks"] = chunks
    if "IEND" not in chunks:
        problems.append("no IEND chunk (truncated or only a header given)")
    if problems:
        out["warnings"] = problems
    return out


def subsampling(components):
    if len(components) < 3:
        return None
    (h0, v0), (h1, v1), (h2, v2) = [(c["h"], c["v"]) for c in components[:3]]
    if (h1, v1) != (h2, v2) or h1 == 0 or v1 == 0:
        return f"{h0}x{v0},{h1}x{v1},{h2}x{v2}"
    ratio = (h0 // h1, v0 // v1)
    return {(1, 1): "4:4:4", (2, 1): "4:2:2", (2, 2): "4:2:0", (4, 1): "4:1:1", (1, 2): "4:4:0"}.get(ratio, f"{h0}x{v0},{h1}x{v1},{h2}x{v2}")


def jpeg(data):
    pos, out, markers = 2, {"format": "JPEG", "mime": "image/jpeg"}, []
    adobe = None
    sof = None
    while pos < len(data):
        if data[pos] != 0xFF:
            raise Bad(f"expected a marker at byte {pos}")
        while pos < len(data) and data[pos] == 0xFF:
            pos += 1
        if pos >= len(data):
            break
        marker = data[pos]
        pos += 1
        if marker in (0x01, 0xD8) or 0xD0 <= marker <= 0xD7:
            continue
        if marker == 0xD9:
            markers.append("EOI")
            break
        need(data, pos + 2, f"segment FF{marker:02X}")
        length = struct.unpack(">H", data[pos:pos + 2])[0]
        if length < 2:
            raise Bad(f"bad segment length at byte {pos}")
        body = data[pos + 2:pos + length]
        if 0xE0 <= marker <= 0xEF:
            name = f"APP{marker - 0xE0}"
        elif marker in SOF:
            name = f"SOF{marker - 0xC0}"
        else:
            name = {0xDB: "DQT", 0xC4: "DHT", 0xDA: "SOS", 0xDD: "DRI", 0xFE: "COM", 0xCC: "DAC", 0xDC: "DNL"}.get(marker, f"FF{marker:02X}")
        markers.append(name)
        if marker == 0xE0 and body.startswith(b"JFIF\x00") and len(body) >= 12:
            units = {0: "aspect ratio only", 1: "dots per inch", 2: "dots per cm"}.get(body[7], "unknown")
            out["jfif"] = {"version": f"{body[5]}.{body[6]:02d}", "density_units": units,
                           "x_density": struct.unpack(">H", body[8:10])[0], "y_density": struct.unpack(">H", body[10:12])[0]}
        elif marker == 0xE1 and body.startswith(b"Exif\x00\x00"):
            out["exif"] = True
        elif marker == 0xE1 and body.startswith(b"http://ns.adobe.com/xap/1.0/"):
            out["xmp"] = True
        elif marker == 0xE2 and body.startswith(b"ICC_PROFILE\x00"):
            out["icc_profile"] = True
        elif marker == 0xEE and body.startswith(b"Adobe") and len(body) >= 12:
            adobe = body[11]
        elif marker in SOF and sof is None:
            if len(body) < 6:
                raise Bad("truncated SOF segment")
            precision, height, width, ncomp = struct.unpack(">BHHB", body[:6])
            comps = []
            for i in range(ncomp):
                c = body[6 + 3 * i:9 + 3 * i]
                if len(c) < 3:
                    raise Bad("truncated SOF component list")
                comps.append({"id": c[0], "h": c[1] >> 4, "v": c[1] & 15, "quant_table": c[2]})
            sof = {"marker": f"SOF{marker - 0xC0}", "process": SOF[marker], "precision": precision, "height": height,
                   "width": width, "components": comps}
        if marker == 0xDA:
            break  # entropy-coded data follows; every header precedes it
        pos += length
    if sof is None:
        raise Bad("no SOF marker before the scan data (not a complete JPEG header)")
    n = len(sof["components"])
    ids = bytes(c["id"] for c in sof["components"])
    if n == 1:
        colour = "greyscale"
    elif n == 3:
        colour = "RGB" if adobe == 0 or ids == b"RGB" else "YCbCr"
    elif n == 4:
        colour = "YCCK" if adobe == 2 else "CMYK"
    else:
        colour = f"{n} components"
    out.update(width=sof["width"], height=sof["height"], bit_depth=sof["precision"], channels=n,
               bits_per_pixel=sof["precision"] * n, colour_type_name=colour, has_alpha=False,
               sof=sof["marker"], process=sof["process"], progressive=sof["marker"] in ("SOF2", "SOF6", "SOF10", "SOF14"),
               components=sof["components"], exif=out.get("exif", False), markers=markers)
    if n >= 3 and colour == "YCbCr":
        out["chroma_subsampling"] = subsampling(sof["components"])
    if adobe is not None:
        out["adobe_transform"] = adobe
    if sof["height"] == 0:
        out["warnings"] = ["height 0 in SOF: the height is given later by a DNL marker"]
    return out


def gif_sub_blocks(data, pos):
    """Skip a chain of data sub-blocks; return (end, joined data)."""
    parts = []
    while True:
        need(data, pos + 1, "GIF sub-block")
        size = data[pos]
        pos += 1
        if size == 0:
            return pos, b"".join(parts)
        need(data, pos + size, "GIF sub-block")
        parts.append(data[pos:pos + size])
        pos += size


def gif(data):
    need(data, 13, "GIF header")
    width, height, packed, bg, aspect = struct.unpack("<HHBBB", data[6:13])
    gct = bool(packed & 0x80)
    gct_bits = (packed & 7) + 1
    out = {"format": "GIF", "mime": "image/gif", "version": data[3:6].decode("latin-1"), "width": width, "height": height,
           "global_colour_table": gct, "global_colour_table_size": 2 ** gct_bits if gct else 0,
           "colour_resolution_bits": ((packed >> 4) & 7) + 1, "background_colour_index": bg,
           "colour_type_name": "indexed-colour (palette)", "channels": 1, "has_alpha": False}
    if aspect:
        out["pixel_aspect_ratio"] = round((aspect + 15) / 64, 4)
    pos = 13 + (3 * 2 ** gct_bits if gct else 0)
    frames, depth_bits, problems = 0, gct_bits if gct else 0, []
    try:
        while True:
            need(data, pos + 1, "GIF block")
            block = data[pos]
            if block == 0x3B:
                break
            if block == 0x21:
                need(data, pos + 2, "GIF extension")
                label = data[pos + 1]
                end, body = gif_sub_blocks(data, pos + 2)
                if label == 0xF9 and len(body) >= 1 and body[0] & 1:
                    out["has_alpha"] = True
                    out["transparency"] = "graphic control extension"
                elif label == 0xFF and body[:11] in (b"NETSCAPE2.0", b"ANIMEXTS1.0") and len(body) >= 14:
                    out["loop_count"] = struct.unpack("<H", body[12:14])[0]
                pos = end
            elif block == 0x2C:
                need(data, pos + 10, "GIF image descriptor")
                fpacked = data[pos + 9]
                pos += 10
                if fpacked & 0x80:
                    lbits = (fpacked & 7) + 1
                    depth_bits = max(depth_bits, lbits)
                    pos += 3 * 2 ** lbits
                pos += 1  # LZW minimum code size
                pos, _ = gif_sub_blocks(data, pos)
                frames += 1
            else:
                raise Bad(f"unknown GIF block 0x{block:02x} at byte {pos}")
    except Bad as error:
        problems.append(str(error))
    out.update(frames=frames, animated=frames > 1, bit_depth=depth_bits or None, bits_per_pixel=depth_bits or None)
    if problems:
        out["warnings"] = problems
    return out


def bmp(data):
    need(data, 18, "BMP header")
    file_size, offset, dib = struct.unpack("<I4xII", data[2:18])
    if dib not in BMP_HEADERS:
        raise Bad(f"unknown BMP DIB header size {dib}")
    need(data, 14 + dib, "BMP DIB header")
    h = data[14:14 + dib]
    out = {"format": "BMP", "mime": "image/bmp", "dib_header": BMP_HEADERS[dib], "declared_file_size": file_size,
           "pixel_data_offset": offset}
    if dib == 12:
        width, height, planes, bpp = struct.unpack("<HHHH", h[4:12])
        compression = 0
    else:
        width, height, planes, bpp = struct.unpack("<iiHH", h[4:16])
        compression = struct.unpack("<I", h[16:20])[0] if dib >= 20 else 0
        if dib >= 40:
            xppm, yppm, used = struct.unpack("<iiI", h[24:36])
            if xppm > 0 and yppm > 0:
                out["dpi"] = {"x": round(xppm * 0.0254, 2), "y": round(yppm * 0.0254, 2)}
            if bpp <= 8:
                out["palette_entries"] = used or 2 ** bpp
    out["top_down"] = height < 0
    alpha = False
    if bpp in (16, 32) and compression in (3, 6):
        if dib >= 56:
            alpha = struct.unpack("<I", h[52:56])[0] != 0
        elif dib == 40 and compression == 6 and len(data) >= 70:
            alpha = struct.unpack("<I", data[66:70])[0] != 0
    colour = "indexed-colour (palette)" if bpp <= 8 else "RGB" if not alpha else "RGBA"
    channels = 1 if bpp <= 8 else 4 if alpha else 3
    out.update(width=abs(width), height=abs(height), planes=planes, bits_per_pixel=bpp,
               bit_depth=bpp if bpp <= 8 else (8 if bpp in (24, 32) else 5 if bpp == 16 else bpp),
               channels=channels, colour_type_name=colour, has_alpha=alpha,
               compression=BMP_COMPRESSION.get(compression, f"unknown ({compression})"))
    if bpp == 16:
        out["note"] = "16-bit BMP: 5 bits per channel (or as the bit fields say)"
    return out


def webp(data):
    need(data, 16, "WebP header")
    riff_size = struct.unpack("<I", data[4:8])[0]
    out = {"format": "WebP", "mime": "image/webp", "riff_size": riff_size, "bit_depth": 8}
    pos, chunks, frames = 12, [], 0
    first = None
    while pos + 8 <= len(data) and len(chunks) < 100000:
        kind = data[pos:pos + 4].decode("latin-1")
        size = struct.unpack("<I", data[pos + 4:pos + 8])[0]
        body = data[pos + 8:pos + 8 + size]
        chunks.append(kind)
        if first is None:
            first = (kind, body)
        if kind == "ANMF":
            frames += 1
        pos += 8 + size + (size & 1)
    if first is None:
        raise Bad("no WebP chunk after the header")
    kind, body = first
    if kind == "VP8 ":
        if len(body) < 10 or body[3:6] != bytes.fromhex("9d012a"):
            raise Bad("VP8 chunk without the 9d 01 2a start code")
        w, hh = struct.unpack("<HH", body[6:10])
        out.update(encoding="lossy (VP8)", width=w & 0x3FFF, height=hh & 0x3FFF, has_alpha=False, channels=3,
                   colour_type_name="YUV 4:2:0 (lossy)", key_frame=not (body[0] & 1))
        if w >> 14 or hh >> 14:
            out["upscale"] = {"x": w >> 14, "y": hh >> 14}
    elif kind == "VP8L":
        if len(body) < 5 or body[0] != 0x2F:
            raise Bad("VP8L chunk without the 0x2f signature")
        bits = struct.unpack("<I", body[1:5])[0]
        alpha = bool((bits >> 28) & 1)
        out.update(encoding="lossless (VP8L)", width=(bits & 0x3FFF) + 1, height=((bits >> 14) & 0x3FFF) + 1,
                   has_alpha=alpha, channels=4 if alpha else 3, colour_type_name="RGBA" if alpha else "RGB",
                   vp8l_version=bits >> 29)
    elif kind == "VP8X":
        if len(body) < 10:
            raise Bad("truncated VP8X chunk")
        flags = body[0]
        width = int.from_bytes(body[4:7], "little") + 1
        height = int.from_bytes(body[7:10], "little") + 1
        alpha = bool(flags & 0x10)
        out.update(encoding="extended (VP8X)", width=width, height=height, has_alpha=alpha, channels=4 if alpha else 3,
                   colour_type_name="RGBA" if alpha else "RGB", icc_profile=bool(flags & 0x20), exif=bool(flags & 0x08),
                   xmp=bool(flags & 0x04), animated=bool(flags & 0x02))
        if "VP8L" in chunks:
            out["encoding"] += ", lossless image"
        elif "VP8 " in chunks:
            out["encoding"] += ", lossy image"
        if out["animated"]:
            out["frames"] = frames
    else:
        raise Bad(f"the first WebP chunk is {kind!r}, not VP8, VP8L or VP8X")
    out["bits_per_pixel"] = 8 * out["channels"]
    out["chunks"] = chunks
    return out


def main():
    try:
        request = json.load(sys.stdin)
    except ValueError as error:
        fail(f"stdin is not JSON: {error}")
    if not isinstance(request, dict):
        fail("input must be a JSON object")
    extra = set(request) - {"hex", "path"}
    if extra:
        fail(f"unknown field(s): {', '.join(sorted(extra))}")
    data = load(request)
    try:
        if data.startswith(PNG_SIG):
            out = png(data)
        elif data[:3] == b"\xff\xd8\xff":
            out = jpeg(data)
        elif data[:6] in (b"GIF87a", b"GIF89a"):
            out = gif(data)
        elif data[:2] == b"BM":
            out = bmp(data)
        elif data[:4] == b"RIFF" and data[8:12] == b"WEBP":
            out = webp(data)
        else:
            fail("not a PNG, JPEG, GIF, BMP or WebP file (unknown signature " + data[:12].hex() + ")")
    except Bad as error:
        fail(str(error))
    except struct.error as error:
        fail(f"truncated or malformed header: {error}")
    out["bytes"] = len(data)
    print(json.dumps(out))


if __name__ == "__main__":
    try:
        main()
    except SystemExit:
        raise
    except Exception as error:  # never a traceback
        fail(f"{type(error).__name__}: {error}")
