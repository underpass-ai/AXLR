# syllables_rhyme

Syllables, stress and rhyme for Spanish verse and prose, with a rough English mode.

## Spanish (exact orthographic rules)

Syllabification follows the conventions the RAE fixes "a efectos ortográficos" (DPD, *tilde*
2.1.1, 2.2.1, 2.3.1; *diptongo*; *hiato*):

- **Diphthong**: open vowel (a, e, o) + unstressed closed vowel (i, u) or the reverse (*vien.to*,
  *cau.sa*, *sua.ve*), and two different closed vowels, accented or not (*ciu.dad*, *je.suí.ti.co*).
- **Triphthong**: open vowel between two unstressed closed vowels (*a.ve.ri.guáis*, *buey*).
- **Hiatus**: two open vowels (*ca.er*, *te.a.tro*), a stressed closed vowel next to an open one
  (*grú.a*, *pa.ís*, *o.í.a*), and two equal vowels (*chi.i.ta*, *po.se.er*).
- An **h between vowels** changes nothing: *prohi.bir*, *de.sahu.cio* keep the diphthong;
  *a.hín.co*, *bú.ho* keep the hiatus.
- Letters: *ch*, *ll*, *rr* are one consonant; the *u* of *que, qui* and of *gue, gui* is silent,
  *ü* is a vowel (*pin.güi.no*); *y* is a vowel when no vowel follows it (*rey*, *muy*, *y*).
- **Consonants between vowels**: one goes with the next vowel; of two, the second goes on
  unless they are an inseparable onset (*pl pr bl br fl fr gl gr cl cr kl kr dr tr*); of three
  or more, the last two go on if they form such an onset (*obs.truc.ción*), else the last one
  (*ins.tan.te*). *tl* is split as in Spain (*at.las*; American usage keeps *a.tlas*).

The **stressed syllable** is the one with the written accent; without one, the second to last
when the word ends in a vowel or in *-n* or *-s* not preceded by another consonant, else the
last (DPD *tilde* 1.2: *dicen*, *robots*, *virrey*). Words in *-mente* take the stress on *men*.
`type` is aguda, llana, esdrújula or sobresdrújula.

**Rhyme** starts at the stressed vowel. It is *consonant* when every sound from there matches
(Castilian pronunciation: silent h, b = v, c before e/i = z, c/qu = k, g before e/i = j,
x = ks, final y = i; no seseo or yeísmo), *assonant* when only the vowels match: the stressed
vowel and the last vowel, skipping the middle vowels of esdrújulas (*rápido* a-o) and the
unstressed weak vowel of a diphthong (*gloria* o-a), with a final unstressed i counted as e and
u as o. A **scheme** first groups lines whose last words rhyme in consonance; with `kind: "any"`
a line that rhymes with nothing then joins a group it assonates with. Letters follow the order
of appearance, `-` marks an unrhymed line, and with `case: "auto"` capitals mark arte mayor
(more than 8 metric syllables, counted with every synalepha and the final-word adjustment;
`meter_check` weighs hiatus and other licences).

## English (heuristic, approximate)

Syllables are vowel groups (a e i o u y) less a silent final *e* (not *-le* after a consonant),
a silent *-ed* or *-es*, and the silent *e* of *-ely, -ement, -eful*; *i* + vowel adds one
except after t, s, c, x or g (*-tion*). Rhyme compares the spelling from the last vowel group
(the one before a silent final *e* or a final *-y*), with a plural *s* set aside. English
spelling does not mark stress, so these answers are often wrong for irregular words.

## Tests and limits

The tests take their words from the RAE's own examples: *vien.to, a.cei.te, cau.sa, sua.ve*
(*diptongo* 1), *grú.a, pa.ís, ca.er, dis.cu.tí.ais* (*hiato* 1), *prohibir, ahínco, búho,
chiita* (*hiato* 3 and 2.3.1), *averiguáis, buey, biaural* (*tilde* 2.2), and the stress classes
of *tilde* 1.2 (*balón, robots, virrey, dicen, clímax, cántaro, llévesemelo*). Rhymes: canción /
corazón (consonant), casa / mala (assonant); schemes: Garcilaso's Soneto V quatrain (ABBA) and
Lorca's *Romance sonámbulo* (-a-a, octosyllables), and Shakespeare's Sonnet 18 for English.

Limits: orthographic, not phonetic, syllables (speakers may say *gra.tu.i.to* or *cru.el*);
foreign words and acronyms are syllabified as Spanish; unstressed words (articles,
prepositions) still get a nominal stressed syllable here.
