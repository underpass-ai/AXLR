# image_info

Reads what an image is from its header bytes, without decoding any pixels: format and MIME
type, width, height, bit depth per channel, channels, bits per pixel, colour type and alpha,
plus what each format records about itself. The file comes as `hex` or as a relative `path`
inside the working directory; the format is recognised by its signature, not its name.

## Formats

- **PNG** (W3C PNG 2nd ed., ISO/IEC 15948): the signature, then IHDR (width, height, bit depth,
  colour type 0/2/3/4/6, compression, filter and interlace methods), with the IHDR CRC checked
  and the bit depth checked against the colour type. The chunk list is walked to IEND: PLTE gives
  the palette size, tRNS transparency, pHYs the pixel density (dpi when the unit is the metre),
  acTL the APNG frame and loop counts, eXIf an Exif block.
- **JPEG** (ITU-T T.81): markers are walked from SOI to the first scan, skipping 0xFF fill bytes
  and the length-less RSTn markers. The first SOFn (C0–C3, C5–C7, C9–CB, CD–CF; C4, C8 and CC
  are DHT, JPG and DAC) gives precision, height, width and the components with their sampling
  factors, from which the chroma subsampling (4:4:4, 4:2:2, 4:2:0, 4:1:1, 4:4:0) follows. JFIF
  APP0 gives version and density; APP1 says whether Exif or XMP is present (not decoded); APP2
  an ICC profile; Adobe APP14's transform decides RGB vs YCbCr and CMYK vs YCCK.
- **GIF** (GIF89a specification): logical screen size, global colour table, colour resolution
  and background index; blocks are walked to the trailer to count frames, find a transparent
  colour (graphic control extension) and the NETSCAPE2.0 loop count. Bit depth is the size
  exponent of the largest colour table.
- **BMP**: BITMAPFILEHEADER and every DIB header (core 12, OS/2, INFO 40, V2, V3, V4 108, V5
  124): width, height (negative means top-down), planes, bits per pixel, compression,
  density and palette size; alpha only when bit fields carry an alpha mask.
- **WebP** (RFC 9649): the RIFF container and its first chunk: `VP8 ` (lossy: key-frame tag,
  start code `9d 01 2a`, 14-bit width and height), `VP8L` (lossless: signature `0x2f`, 14-bit
  width−1 and height−1, alpha bit, version) or `VP8X` (extended: ICC, alpha, Exif, XMP and
  animation flags, 24-bit canvas width−1 and height−1, ANMF frame count).

## Tests

Tiny files built by hand byte by byte from each specification's layout (all under 100 bytes, as
hex in `tests.json`): PNG RGBA with pHYs and valid CRCs, an indexed interlaced PNG with tRNS, a
bare IHDR; progressive colour and baseline greyscale JPEG headers; the widely published 43-byte
1×1 transparent GIF89a and a two-frame looping GIF; a top-down 24-bit BMP; WebP VP8L, VP8 and
VP8X; and the error cases.

## Limits

Header information only: no pixel decoding, no EXIF/XMP/ICC parsing beyond presence, no TIFF,
HEIC, AVIF or SVG. A JPEG must have its SOF before the first scan, as conforming files do.
Files up to 64 MiB.
