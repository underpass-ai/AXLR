# Connections between abouts

`kmp_relate` reads comparisons and proposals; it does not create relations.
If the selected about does not exist yet, defer this comparison until the
first source-backed durable fact is recorded there. Do not create a dummy
fact or guess another about to make a comparison pass.
Read its current guide card and live schema first. Use the current exact about
and `dimensions.scope="abouts"` with a nonempty `dimensions.abouts` list when
the task or retrieved evidence identifies other relevant scopes. Include the
current about. If none are known, the startup comparison may explicitly use
`dimensions.scope="all_abouts"` to discover candidates in the selected store.
Keep the result budget bounded and finish relevant pages. This broader scope
has a real cost: use it once, disclose partial results, and do not invent a time
window merely to reduce the selection. Use a task-established interval when
one exists. User scope restrictions take precedence.

Inspect both endpoints of a relevant proposal before relying on it. Shared
labels, similar summaries and chronological order locate candidates; they
do not establish identity or causality. No proposals is a valid outcome.
Never manufacture a link to make the bootstrap appear successful.

To declare a supported cross-about identity, use `kmp_write_memory`'s
`relations` form for stored endpoints, not a replacement `memories` record.
The source must belong to the declared about. The target may cross abouts only
for `same_event_as` or `same_entity_as`, with the exact returned Relate proposal
in `read_context` as required by the live schema. Copy canonical endpoint refs
and proposal tokens exactly. Supply your own `why` and concrete `evidence`
supporting that particular identity, and one idempotency key for the logical
write. Review the returned neighborhood before resuming its unchanged
continuation; `accepted=true` is the persistence receipt.

Other dependencies or similarities can remain findings with source refs.
Do not label a dependency `same_entity_as`, or declare unsupported arbitrary
cross-about links. Use `kmp_trace` when the conclusion depends on a stored
connection. Record actual outcomes, constraints and decisions rather than
transcripts, with faithful English search renderings when required.
