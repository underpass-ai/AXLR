# json_diff

Compares two JSON values structurally and says what changed, where, and how to get from one
to the other.

## Diff

Objects are compared member by member (order does not matter), scalars by JSON equality
(RFC 6902 section 4.6: `1` equals `1.0`, `true` is not `1`, `"10"` is not `10`). Each difference
is `added`, `removed` or `changed` with its path as a JSON Pointer (RFC 6901: `~` is written
`~0` and `/` is written `~1`), and the old or new value. Arrays, by `arrays`:

- `index` (default): position by position, then the extra tail removed or added;
- `lcs`: a longest common subsequence, giving the minimal inserts and deletes (`["bar","baz"]`
  to `["bar","qux","baz"]` is one insertion); a delete next to an insert at the same place is
  treated as a change and compared in depth;
- `key`: elements are objects matched by the value of `key` (an `id`), so an inserted record
  does not make every later record "changed"; each difference carries its `key`. When the
  matched elements appear in another order, `reordered` is reported and the patch replaces the
  array.

`patch` is an RFC 6902 JSON Patch (`add`, `remove`, `replace`) that turns `a` into `b`, with
array indexes adjusted for the operations before them; `patch_verified` says that applying it
to `a` gave `b` (it always should).

## Apply

`operation: "apply"` applies a patch to `doc`: `add` (with `-` for the end of an array),
`remove`, `replace`, `move`, `copy` and `test`, following RFC 6902 section 4. The patch is
atomic: if any operation fails (a missing path, an out-of-range index, a failed `test`), the
output has `applied: false`, the reason, and no result.

## Tests

The RFC 6902 Appendix A examples: A.1 adding an object member, A.2 adding an array element and
A.4 removing one (with `lcs`, and A.4 also by index), A.3 removing a member, A.5 replacing, A.10
adding a nested object, A.14 the `~01` escape; and applied: A.6 and A.7 moves, A.8 and A.9
tests, A.12 adding to a nonexistent target, A.15 comparing strings and numbers, A.16 `/-`. Plus a
keyed array diff worked out by hand, number and boolean equality, and usage errors.

## Limits

The `lcs` mode is quadratic (arrays up to n × m = 4 million); no `move` or `copy` operations are
generated in diffs; values nested more than 500 levels deep are refused.
