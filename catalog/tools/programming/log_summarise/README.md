# log_summarise

Turns a log given inline into a short list of message templates, with how often each occurs
and when it first and last occurred.

For every non-empty line:

1. **Timestamp.** The first timestamp found is taken out of the line: an RFC 3164 syslog
   prefix (`Oct 18 09:05:00`, optionally after a `<PRI>`), an Apache Common/Combined Log
   Format date (`[18/Oct/2026:09:07:00 +0000]`), or an ISO 8601 date-time in the first 80
   characters (`2026-10-18T09:00:01Z`, `2026-10-18 09:00:00,123`, with or without an offset,
   any number of fractional digits). Times with an offset are compared in UTC, times without
   one as they are. Syslog has no year: give `year`, or the time is shown as
   `--MM-DDTHH:MM:SS`, ordered within one year inside its template and left out of the
   overall range.
2. **Level.** `level=`/`lvl=`/`severity=` (logfmt) or a standalone level word: FATAL (also
   EMERG, ALERT, CRIT, SEVERE), ERROR (ERR), WARN (WARNING), NOTICE, INFO, DEBUG, TRACE;
   otherwise UNKNOWN.
3. **Masking**, in this order: UUIDs `<UUID>`, quoted strings `<STR>`, other timestamps and
   clock times `<TS>`, IPv4 (with port) and IPv6 addresses `<IP>`, hex ids (`0x…`, or 8+ hex
   digits mixing letters and digits) `<HEX>`, numbers not glued to a word `<NUM>` (so
   `30s` becomes `<NUM>s` but `sda1` and `web01` stay). `mask` limits which are applied.
4. **Clustering**, as in Drain (He et al., ICWS 2017): lines are grouped by token count and
   first token (a first token with digits counts as a wildcard), and within a group a line
   joins the most similar template when the share of positions with equal tokens (wildcard
   positions excluded, as Drain's `simSeq`) reaches `similarity` (default 0.5); the positions
   that differ become `<*>`. Otherwise it starts a new template.

The answer lists the templates by count (ties by first appearance), each with its share of
lines, first and last timestamp (the earliest and latest, not the first and last seen),
levels, the line number of its first occurrence and that line as an example; and overall:
lines, templates, level counts, time range and lines without a timestamp. `top` limits the
list (default 20).

The tests use a hand-checked sample mixing ISO 8601, syslog, Apache and untimed lines, whose
six templates, counts and times were worked out by hand, plus out-of-order lines, a
yearless syslog file, the similarity threshold on both sides of 3/6, logfmt and the masks.

Limits: one line is one record (multi-line stack traces become several templates); Drain's
fixed-depth prefix tree is reduced to grouping by (length, first token), so lines whose
first tokens differ never share a template; epoch-second timestamps and other formats are
not recognised (such lines count as untimed); the example line is printed as it is, so any
secret in the log appears in the output.
