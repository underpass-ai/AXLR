# Draft ceremony definitions

These definitions are designed but not shipped. They are validated and published into a disposable MADE store by CI (`tools/ceremonies/check_pins.py` validates them, `tools/ceremonies/spike_drafts.py` walks their happy and blocked paths), but no console mode starts them, no pin exists for them in `tui/adapters/ceremonyhost/definitions.go`, and `/mcp → P` does not publish them.

| Definition | Design | Purpose |
|:--|:--|:--|
| `axlr_plan` 1.0 | [Plan, task and sync](../../../docs/plans/2026-10-06-local-27b-ceremonies.md) | Decompose a brief into atomic, console-verified tasks the person approves |
| `axlr_task` 1.0 | same | Finish one atomic task in a fresh, precise context, test-first when the plan asks for it |
| `axlr_sync` 1.0 | same | Integrate a wave with the end-to-end check and relay the workers' notes |

A draft moves to `tui/adapters/ceremonyhost/definitions/` only together with the driver code that runs it and its digest pin. A draft must not share a name and version with a pinned definition; the pin check fails if it does.

Validate the drafts locally with a compatible `made-mcp` binary (CI uses the checksummed 0.10.0 release):

```bash
python3 tools/ceremonies/check_pins.py --made-bin /absolute/path/to/made-mcp
python3 tools/ceremonies/spike_drafts.py --made-bin /absolute/path/to/made-mcp
```
