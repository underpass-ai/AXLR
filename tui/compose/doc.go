// Package compose wires the parts of the model and tool loop that the
// console (cmd/axlr-tui) and the service (cmd/axlr-serve) share: local
// models, Jev, the ceremony driver, the session that continues a capped
// one, and the tools a continuation is equipped with (usage, context cap,
// forged and catalog tools, memory). Each binary keeps what only it has:
// the console its terminal, setup and background jobs, the service its
// transports and principals. Until 21 October 2026 the service built the
// loop with almost none of these, and a session there had no context cap,
// no MADE instance and no forged tools.
package compose
