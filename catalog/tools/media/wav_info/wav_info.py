#!/usr/bin/env python3
"""Read a WAV file's format and duration from its RIFF chunks: format tag (PCM, IEEE float,
A-law, mu-law, ADPCM, extensible with its sub-format GUID), channels, sample rate, bit depth,
block align, frames and duration; for PCM and float also the peak sample. Plain PCM is cross-
checked with Python's wave module.

Input (JSON on stdin):
  hex    the file's bytes as hexadecimal (whitespace allowed), or
  path   a relative path to the file inside the working directory
"""
import array
import io
import json
import math
import os
import re
import struct
import sys
import wave
from fractions import Fraction

MAX_BYTES = 256 * 1024 * 1024
FORMATS = {0x0001: "PCM", 0x0002: "Microsoft ADPCM", 0x0003: "IEEE float", 0x0006: "A-law", 0x0007: "mu-law",
           0x0011: "IMA ADPCM", 0x0031: "GSM 6.10", 0x0050: "MPEG", 0x0055: "MPEG Layer III (MP3)",
           0x0161: "Windows Media Audio", 0xFFFE: "WAVE_FORMAT_EXTENSIBLE"}
GUID_TAIL = bytes.fromhex("000000001000800000aa00389b71")  # KSDATAFORMAT_SUBTYPE_* = tag + this
SPEAKERS = ["FL", "FR", "FC", "LFE", "BL", "BR", "FLC", "FRC", "BC", "SL", "SR", "TC", "TFL", "TFC", "TFR", "TBL", "TBC", "TBR"]


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
        if len(text) > 2 * MAX_BYTES:
            fail("the file may be at most 256 MiB")
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
        fail("the file may be at most 256 MiB")
    return data


def clock(seconds):
    total_ms = round(seconds * 1000)
    h, rest = divmod(total_ms, 3_600_000)
    m, rest = divmod(rest, 60_000)
    s, ms = divmod(rest, 1000)
    return f"{h}:{m:02d}:{s:02d}.{ms:03d}"


def peak(samples, bits, tag):
    """Peak absolute sample and its level in dBFS, for 8/16/32-bit PCM and 32/64-bit float."""
    if tag == 1 and bits == 8:
        values = [b - 128 for b in samples]
        full = 128
    elif tag == 1 and bits in (16, 32):
        values = array.array("h" if bits == 16 else "i")
        values.frombytes(samples[:len(samples) - len(samples) % (bits // 8)])
        if sys.byteorder == "big":
            values.byteswap()
        full = 2 ** (bits - 1)
    elif tag == 3 and bits in (32, 64):
        values = array.array("f" if bits == 32 else "d")
        values.frombytes(samples[:len(samples) - len(samples) % (bits // 8)])
        if sys.byteorder == "big":
            values.byteswap()
        full = 1.0
    else:
        return None
    if not len(values):
        return None
    top = max(abs(min(values)), abs(max(values)))
    level = None if top == 0 else round(20 * math.log10(top / full), 2)
    return {"peak_sample": top, "peak_dbfs": level}


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
    if len(data) < 12:
        fail("too short for a RIFF header (12 bytes)")
    if data[:4] in (b"RF64", b"BW64"):
        fail("RF64/BW64 (64-bit WAV) is not supported")
    if data[:4] == b"RIFX":
        fail("RIFX (big-endian WAV) is not supported")
    if data[:4] != b"RIFF" or data[8:12] != b"WAVE":
        fail("not a WAV file: expected RIFF....WAVE")
    riff_size = struct.unpack("<I", data[4:8])[0]
    warnings, chunks = [], []
    fmt = fact = None
    data_offset = data_size = None
    pos = 12
    while pos + 8 <= len(data):
        cid = data[pos:pos + 4].decode("latin-1")
        size = struct.unpack("<I", data[pos + 4:pos + 8])[0]
        chunks.append({"id": cid, "offset": pos, "size": size})
        body = data[pos + 8:pos + 8 + size]
        if cid == "fmt " and fmt is None:
            fmt = body
        elif cid == "fact" and len(body) >= 4:
            fact = struct.unpack("<I", body[:4])[0]
        elif cid == "data" and data_offset is None:
            data_offset, data_size = pos + 8, size
        pos += 8 + size + (size & 1)  # chunks are padded to an even size
    if fmt is None or len(fmt) < 16:
        fail("no fmt chunk of at least 16 bytes")
    tag, channels, rate, byte_rate, align, bits = struct.unpack("<HHIIHH", fmt[:16])
    out = {"format": "WAV", "format_tag": tag, "format_name": FORMATS.get(tag, f"unknown (0x{tag:04x})"),
           "channels": channels, "sample_rate": rate, "byte_rate": byte_rate, "block_align": align,
           "bits_per_sample": bits}
    real_tag = tag
    if tag == 0xFFFE:
        if len(fmt) < 40:
            fail("WAVE_FORMAT_EXTENSIBLE needs a 40-byte fmt chunk")
        cb, valid, mask = struct.unpack("<HHI", fmt[16:24])
        guid = fmt[24:40]
        real_tag = struct.unpack("<H", guid[:2])[0]
        out.update(valid_bits_per_sample=valid, channel_mask=mask,
                   speakers=[SPEAKERS[i] for i in range(len(SPEAKERS)) if mask >> i & 1],
                   subformat_tag=real_tag if guid[2:] == GUID_TAIL else None,
                   subformat_name=FORMATS.get(real_tag, f"unknown (0x{real_tag:04x})") if guid[2:] == GUID_TAIL else "non-standard GUID",
                   subformat_guid=guid.hex())
        if guid[2:] != GUID_TAIL:
            real_tag = None
    if channels == 0 or rate == 0:
        fail("channels and sample rate must be non-zero")
    linear = real_tag in (1, 3, 6, 7)
    if linear and align and byte_rate != rate * align:
        warnings.append(f"byte rate {byte_rate} is not sample rate x block align ({rate * align})")
    if linear and bits and align != channels * ((bits + 7) // 8):
        warnings.append(f"block align {align} is not channels x bytes per sample ({channels * ((bits + 7) // 8)})")
    if data_offset is None:
        fail("no data chunk")
    present = max(0, min(data_size, len(data) - data_offset))
    out["data_bytes"] = data_size
    if present < data_size:
        warnings.append(f"data chunk truncated: {present} of {data_size} bytes present; frames and duration use the declared size")
    if linear and align:
        frames = data_size // align
        if data_size % align:
            warnings.append(f"data size is not a whole number of {align}-byte frames")
        method = "data bytes / block align"
    elif fact is not None:
        frames, method = fact, "fact chunk (sample frames)"
    elif byte_rate:
        frames, method = None, "data bytes / byte rate (compressed, no fact chunk)"
    else:
        frames, method = None, None
    if frames is not None:
        duration = Fraction(frames, rate)
    elif byte_rate:
        duration = Fraction(data_size, byte_rate)
    else:
        duration = None
    out.update(frames=frames, duration_seconds=None if duration is None else float(duration),
               duration_exact=None if duration is None else str(duration),
               duration=None if duration is None else clock(float(duration)), duration_method=method,
               riff_size=riff_size, file_bytes=len(data))
    if fact is not None:
        out["fact_sample_length"] = fact
    if riff_size + 8 != len(data):
        warnings.append(f"RIFF size says {riff_size + 8} bytes, the file has {len(data)}")
    if linear and present:
        level = peak(data[data_offset:data_offset + present], bits, real_tag)
        if level:
            out.update(level)
    if tag == 1 and present == data_size:
        try:
            with wave.open(io.BytesIO(data)) as w:
                cross = {"channels": w.getnchannels(), "sample_rate": w.getframerate(),
                         "bits_per_sample": 8 * w.getsampwidth(), "frames": w.getnframes()}
            out["wave_module_check"] = {"agrees": cross == {k: out[k] for k in cross}, **cross}
        except (wave.Error, EOFError, struct.error) as error:
            out["wave_module_check"] = {"agrees": False, "error": str(error)}
    out["chunks"] = chunks
    out["warnings"] = warnings
    print(json.dumps(out))


if __name__ == "__main__":
    try:
        main()
    except SystemExit:
        raise
    except Exception as error:  # never a traceback
        fail(f"{type(error).__name__}: {error}")
