# meter_check

Scans a line (or `lines`) of verse and says whether it fits a meter, with the scansion.

## Spanish

1. **Syllables** of each word by the RAE's orthographic rules (diphthongs, triphthongs, hiatus,
   intercalated h; the same code as `syllables_rhyme`).
2. **Synalepha**: a word ending in a vowel (or final *y*) joins the next word when that one starts
   with a vowel or *h* + vowel; *hie-, hue-* (hielo, hierba, huevo) start with a consonant sound
   and block it. The scansion shows the joins with `_`: `to_es`, `tá_en`.
3. **Final-word rule**: the metric count is the phonological count +1 when the last word is
   aguda (or a stressed monosyllable), −1 when esdrújula (−2 sobresdrújula), unchanged when llana.
4. **Fitting a meter**: the count with every synalepha is the `natural` scansion. When the meter
   needs more syllables, the tool breaks as many synalephas as needed (hiatus), preferring a
   contact before a stressed vowel, then one after a stressed word-final vowel, then the leftmost.
   That preference is a documented heuristic for the poet's licence; it changes which boundary is
   named, not the count. Syneresis and dieresis inside words are not tried, so a line that is
   too long even with every synalepha is reported as not fitting.
5. **Stresses** are the stressed syllables of tonic words. Articles, prepositions (except
   *según*), conjunctions, unstressed pronouns and possessives (*me, te, se, mi, tu, su*…) and
   relatives (*que, cuando, donde, cuanto*…) are unstressed; *un*, *nuestro*, *vuestro*,
   subject pronouns and interrogatives are tonic.

Meters: `octosilabo` (8), `endecasilabo` (11), `alejandrino` (14 as two heptasyllables: the
tool tries every word boundary as the caesura, with no synalepha across it and the final-word
rule in each half), any count with `syllables`, or `auto` (the natural count and its name, and
which of the three meters a hiatus could reach).

**Endecasílabo types** follow the definitions in Spanish Wikipedia's *Endecasílabo* article
(Ritmos y acentos): enfático 1 and 6 with nothing stressed between; heroico 2 and 6; melódico 3
and 6; sáfico 4 and 8; dactílico 4 and 7; mixto 4 and 6; yámbico every even syllable; vacío 6
with nothing before. A line often meets several; all are listed because the source sets no
precedence. `a_maiore` means a stress on the 6th. A stress on the 9th, or none on the 4th or
6th, is warned about.

## English (heuristic)

Iambic pentameter: syllables by vowel groups (the `syllables_rhyme` heuristic); a stress is
guessed only for polysyllables (two-syllable words on the first syllable unless they begin with
an unstressed prefix such as *a-, be-, com-, de-, re-*; *-tion, -sion, -ic* on the penult) and
must land on an even position; monosyllables may take either stress. Ten syllables fit, or
eleven with a feminine ending; a trochee in the first foot is reported but allowed.

## Tests and limits

Hand-scanned canonical lines: Garcilaso's "Escrito está en mi alma vuestro gesto" (11, one
hiatus, stresses 2-4-6-8-10), "El dulce lamentar de dos pastores" (heroico) and "Cuando me paro
a contemplar mi estado" (sáfico); Darío's "La princesa está triste… ¿qué tendrá la princesa?"
(alejandrino 7 + 7); octosyllables from Lorca and the *Romance del prisionero*; Shakespeare's
"Shall I compare thee to a summer's day?" and Hamlet's feminine "…that is the question".

Limits: orthographic syllables stand in for phonetic ones; no syneresis, dieresis or
synalepha blocked by punctuation; a homograph's stress follows its spelling (*como* is always
unstressed, *cómo* stressed); English stress is a guess, not a dictionary lookup.
