# Ceremony drafts

This directory holds definitions that are designed but not yet driven by the console. CI validates and publishes each draft into a disposable MADE store (`tools/ceremonies/check_pins.py`) and walks its happy and blocked paths (`tools/ceremonies/spike_drafts.py`). No pin exists for a draft and `/mcp → P` does not publish it.

There are no drafts at the moment. `axlr_plan`, `axlr_task` and `axlr_sync` 1.0 started here on 6 Oct 2026. They became pinned definitions under [`tui/adapters/ceremonyhost/definitions/`](../../../tui/adapters/ceremonyhost/definitions/) on 7 Oct 2026, when the console started driving them; `spike_drafts.py` still walks their scenarios there.

Validate locally with a compatible `made-mcp` binary (CI uses the checksummed 0.10.0 release):

```bash
python3 tools/ceremonies/check_pins.py --made-bin /absolute/path/to/made-mcp
python3 tools/ceremonies/spike_drafts.py --made-bin /absolute/path/to/made-mcp
```
