# wav_info

Reads a WAV file's format and length from its RIFF chunks. The file comes as `hex` or as a
relative `path` inside the working directory; a header alone is enough, because the duration
comes from the sizes the header declares.

## Method

The RIFF container (Microsoft/IBM Multimedia Programming Interface and Data Specifications 1.0,
1991) is `RIFF <size> WAVE` followed by chunks of `id`, little-endian 32-bit size and data,
each padded to an even length. Every chunk is listed with its offset and size; three are read:

- `fmt `: format tag, channels, sample rate, byte rate, block align, bits per sample. For
  `WAVE_FORMAT_EXTENSIBLE` (0xFFFE) the 40-byte form also gives the valid bits, the speaker mask
  (FL, FR, FC, LFE…) and the sub-format GUID, whose first two bytes are the real format tag
  when the rest is the standard `-0000-0010-8000-00AA00389B71` tail.
- `data`: its declared size.
- `fact`: the number of sample frames, needed for compressed formats.

Frames are `data bytes / block align` for PCM, IEEE float, A-law and mu-law; the `fact` count
for compressed formats (ADPCM, GSM, MP3…); without `fact`, only the duration is estimated as
`data bytes / byte rate`. The duration is exact (`duration_exact`, a fraction of seconds), as a
float and as `H:MM:SS.mmm`. For 8-bit (unsigned), 16- and 32-bit PCM and 32/64-bit float the
peak sample and its level `20 log10(peak / full scale)` in dBFS are computed from the samples
present. Plain PCM is cross-checked with Python's `wave` module (`wave_module_check`).

Warnings, not errors: a data chunk shorter than declared, a RIFF size that disagrees with the
file, a byte rate other than sample rate × block align, a block align other than channels ×
bytes per sample, and a data size that is not a whole number of frames.

## Tests

A canonical 44-byte PCM header with four 16-bit samples (8000 Hz mono: 4 frames, 1/2000 s,
peak 16384 = −6.02 dBFS) worked out by hand byte by byte, a header-only CD-quality file
declaring one second, IEEE float with a `fact` chunk, an extensible 24-bit file, IMA ADPCM
timed by `fact`, an odd-sized chunk with its pad byte, and the errors.

## Limits

No RF64/BW64 (files over 4 GiB) and no big-endian RIFX; no decoding of compressed audio; `LIST`,
`bext`, `cue ` and other metadata chunks are listed but not parsed. Files up to 256 MiB.
