# AXLR identity

![AXLR pixel-art wordmark with Spectrum bars](assets/brand/axlr-spectrum.svg)

This is the AXLR wordmark supplied for the project, recolored with its copper, gold, brown and sand palette: `#B85C38`, `#D97B2D`, `#6B4A2E` and `#C9B08C`. Its block letters, stepped outlines and four slanted Spectrum bars form the visual language used in the documentation. The bars stack vertically into a square at the lower right. KMP and MADE share the bar geometry and have their own palettes.

The repository keeps the supplied [PNG wordmark](assets/axlr-wordmark.png) at its original 1600 × 479 resolution. The [transparent AXLR SVG](assets/brand/axlr-spectrum.svg) reconstructs those letters and adds the four Spectrum bars. The [brand asset pack](assets/brand/README.md) contains transparent KMP, MADE and AXLR logos in both SVG and PNG. Preserve each asset's aspect ratio. For text-only contexts, write **AXLR**. The wordmark itself contains the complete lettering, so do not crop a letter as a substitute icon.

The [pixel wordmark renderer](../tools/pixelart/README.md) generates both formats from editable terminal-art rows and shared geometry. It needs Node.js 22 and no npm packages or fonts. Use it to export logos directly into another site's public asset directory; change the source definitions instead of editing generated files. CI checks that the exports match their source.
