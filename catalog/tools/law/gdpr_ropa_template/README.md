# gdpr_ropa_template

Builds the record of processing activities that GDPR Article 30 requires: for a controller,
Art. 30(1)(a)–(g); for a processor, Art. 30(2)(a)–(d). The output gives the article's items in
English or Spanish (`items`, the Official Journal wording), the record per processing activity
(`record`), what is missing, and a Markdown table with one row per activity and one column per
item.

| Item (controller) | Input field | Counted as |
|:--|:--|:--|
| 30(1)(a) controller, contact; joint controller, representative, DPO | `organisation.name`, `organisation.contact`; `joint_controller`, `organisation.representative`, `organisation.dpo` | mandatory; where applicable |
| 30(1)(b) purposes | `purposes` | mandatory |
| 30(1)(c) categories of data subjects and of personal data | `data_subjects`, `data_categories` | mandatory |
| 30(1)(d) categories of recipients | `recipients` | mandatory |
| 30(1)(e) transfers and safeguards | `transfers` | where applicable |
| 30(1)(f) time limits for erasure | `erasure_time_limits` | where possible |
| 30(1)(g) security measures (Art. 32(1)) | `security_measures` | where possible |

For a processor: 30(2)(a) `organisation.name` and `contact` and, per activity, `controller`
(mandatory), representative and DPO (where applicable); 30(2)(b) `processing_categories`
(mandatory); 30(2)(c) `transfers` (where applicable); 30(2)(d) `security_measures` (where
possible).

The article's own words decide the split: items that say "where applicable" or "where
possible" go to `missing_conditional` (fill them in or record that they do not apply, e.g.
`"transfers": "none"`); the rest to `missing_mandatory`, and `complete` is true when that list
is empty. `legal_basis` is accepted as an extra column because supervisory authorities
recommend it and Spanish public bodies must publish it (LOPDGDD art. 31.2), but Art. 30 does
not require it. Markdown cells escape `|` and turn line breaks into `<br>`.

## Tests

The known value is Article 30 itself: the controller items are exactly 30(1)(a)–(g), with the
Official Journal wording of 30(1)(d); the processor items are 30(2)(a)–(d), with the Spanish
wording of 30(2)(b). Other cases check a filled activity, an empty one, Markdown escaping and
usage errors.

## Limits

The tool records what it is told; it cannot judge whether purposes or measures are accurate,
nor whether the Art. 30(5) exemption applies. Not legal advice; the output carries that
disclaimer.
