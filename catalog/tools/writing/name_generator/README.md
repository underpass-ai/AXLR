# name_generator

Invents names from syllable tables written for this tool (MIT): a start syllable, up to one or
two middle syllables and an ending, chosen with `random.Random(seed)`. After a syllable ending
in a vowel, the next one is drawn from those starting with a consonant when there are any (so
no *Eloo*).

| `kind` | Styles | Shape |
|:--|:--|:--|
| `character` | spanish, english, nordic, japanese, greek, fantasy | given name (female or male endings; `gender: "any"` draws one) + a surname from the style's surname table unless `surname: false` |
| `place` | spanish, english, nordic, fantasy | optional prefix (*San*, *Upper*, *Port*…) + root + ending (*-ford*, *-vik*, *-mar*…) |
| `product` | tech, pharma, brand | start + optional middle + suffix (*-ify*, *-vax*, *-ko*…) |

`min_length` and `max_length` bound the letters of each generated word (given name and
surname separately; for places, the main word). Names that miss the bounds, or repeat one
already given when `unique` is true, are discarded and drawn again, up to 200 attempts per
name wanted; if that is not enough the answer has fewer names and a `warning`. `checks`
recounts the final list for the bounds and uniqueness. Each name lists its `parts`, so it can
be traced back to the tables.

Determinism: the seed is an integer (strings are refused, because Python salts string hashing
per process) and every choice goes through one `random.Random(seed)` with `choice` on lists and
`randint`, so the same input gives the same names on any CPython 3.x. Changing the tables is a
new version of the tool, since it changes the names a seed gives.

The tests run the same input in two separate processes and expect the same list, whose names
were checked by hand against the syllable tables and the length bounds; they also check the
bounds over 50 names, an impossible bound and the style list.

Limits: the styles are flavours, not real naming conventions — the generator does not know
which names exist, whether a product name is trademarked, or what a name means in another
language. Check before using one.
