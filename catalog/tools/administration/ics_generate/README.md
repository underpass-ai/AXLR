# ics_generate

Writes an RFC 5545 iCalendar object (`VCALENDAR` with one `VEVENT`) from structured input and
returns it as text in `ics`, ready to save as a `.ics` file.

## What it writes

- `VERSION:2.0`, `PRODID`, `CALSCALE:GREGORIAN`, optional `METHOD`.
- `UID`: the given one, or the first 32 hex digits of a SHA-256 of the input plus
  `@ics-generate.invalid`, so the same input always gives the same UID. `DTSTAMP`: the given UTC
  time, or the fixed placeholder `19700101T000000Z`; the clock is never read.
- `DTSTART` and `DTEND` or `DURATION`. A start `YYYY-MM-DD` is an all-day event
  (`VALUE=DATE`, ending the next day unless `end` says otherwise). A date-time is *floating* with
  no `tzid` and no offset, local to `tzid` (`DTSTART;TZID=Europe/Madrid:...`), or UTC (`...Z`)
  when it carries an offset or `utc` is true.
- `RRULE` built from `freq`, `interval`, `count` or `until` (never both), `wkst`, `byday` (`MO`,
  or `1FR`, `-1SU` with monthly or yearly), `bymonthday`, `bymonth`, in that order. `UNTIL` is
  converted to UTC when the start has a TZID or is UTC, stays local when floating, and is a date
  for all-day events (RFC 5545 3.3.10); a date given for a timed event means 23:59:59 that day.
  `EXDATE` uses the start's form.
- `ORGANIZER` and `ATTENDEE` with `CN`, `ROLE`, `PARTSTAT`, `RSVP`, `CUTYPE`; a `CN` with `;`,
  `:` or `,` is quoted. `VALARM` with `TRIGGER:-PT15M` (minutes before the start) and, for
  `DISPLAY`, a `DESCRIPTION` (default: the summary).
- With a TZID, a `VTIMEZONE` built from the system's IANA data: the offset transitions of the
  year before the event, each with a yearly `RRULE` (`BYMONTH`, `BYDAY` as nth or last weekday)
  when the rule is the same in the event's year; a zone without daylight time gets one
  `STANDARD` block.

Text values are escaped as RFC 5545 3.3.11 says (backslash, `;`, `,`, and newline as `\n`; the
colon is not escaped), lines are folded at 75 octets of UTF-8 with CRLF and one leading space
(never splitting a multi-byte character), and every line ends in CRLF.

## Tests

Against RFC 5545's own examples: the TEXT escaping example of 3.3.11, the recurrence examples of
3.8.5.3 ("every other week on Monday, Wednesday and Friday until December 24, 1997" and "monthly
on the first Friday for 10 occurrences"), the ORGANIZER example of 3.8.4.3, the folding rule of
3.1 (lengths counted by hand, including two-octet characters), and the US 1997 and EU daylight
saving rules in the generated VTIMEZONE.

## Limits

One event per calendar; no VTODO or VJOURNAL, no RDATE, no BYSETPOS, BYHOUR or BYWEEKNO. The
VTIMEZONE describes the rule in force around the event's year; for a recurrence that runs across
a later change of a country's rules, the client's own zone data is better. Attendee e-mail
addresses are checked only for shape.
