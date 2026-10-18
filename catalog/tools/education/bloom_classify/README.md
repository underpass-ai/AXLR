# bloom_classify

Classifies learning objectives by the level of the revised Bloom taxonomy (Anderson & Krathwohl,
2001): 1 Remember, 2 Understand, 3 Apply, 4 Analyze, 5 Evaluate, 6 Create (in Spanish Recordar,
Comprender, Aplicar, Analizar, Evaluar, Crear).

**Method.** The objective is lower-cased and stripped of accents, split into words, and scanned
left to right for the verbs of a bundled table (phrases such as *carry out* or *break down*
first). The first verb found is the main verb and decides the level; later verbs are listed in
`other_verbs`, with the `highest_level` of all. Inflected forms are recognised: English *-s,
-es, -ed, -ing*, *-ies/-ied*, doubled consonants and British *-ise*; Spanish infinitive, third
person present, future (*identificará*), gerund, participle and subjunctive, with the common
irregular stems (*resuelve*, *construye*, *demuestra*, *conozca*). `language: "auto"` decides per
objective from function words and accents.

**The verb table** is in the program. Its core is the 19 cognitive processes of the revised
taxonomy and their alternative names, Table 5.1 of Anderson & Krathwohl (summarised in
Krathwohl 2002): Remember — recognizing (identifying), recalling (retrieving); Understand —
interpreting (clarifying, paraphrasing, representing, translating), exemplifying
(illustrating, instantiating), classifying (categorizing, subsuming), summarizing
(abstracting, generalizing), inferring (concluding, extrapolating, interpolating, predicting),
comparing (contrasting, mapping, matching), explaining (constructing models); Apply — executing
(carrying out), implementing (using); Analyze — differentiating (discriminating,
distinguishing, focusing, selecting), organizing (finding coherence, integrating, outlining,
parsing, structuring), attributing (deconstructing); Evaluate — checking (coordinating,
detecting, monitoring, testing), critiquing (judging); Create — generating (hypothesizing),
planning (designing), producing (constructing). For these verbs the output names the
`process`. Around them the table adds verbs common in objective-writing guides (*list, define,
solve, calculate, examine, justify, compose*…), compiled for this tool with Spanish equivalents.

**Flags.** A verb listed at more than one level (*compare*, *identify*, *organize*, *construct*,
*test*, *describe*, *demonstrate*…) is `ambiguous`: its level is the taxonomy's own when it has
one, and `also_listed_at` names the others, because the level really depends on what the learner
does with the content. Verbs that describe no observable performance (*understand, know, learn,
appreciate, be aware of, become familiar with*; *comprender, entender, saber, conocer,
aprender*…) are `non_measurable`; an objective with nothing else is left unclassified with a
suggestion, and one where a measurable verb follows is classified by that verb and still flagged.

The tests check the six levels against the process names of Table 5.1, six alternative names
(retrieving, extrapolating, carrying out, outlining, detecting, designing) in inflected forms,
the non-measurable flags, an ambiguous verb, British spelling, and Spanish objectives.

Limits: a keyword method, not a parser: a noun that looks like a verb (*a list of*, *the test
results*) can be taken as the main verb when it comes first; only the cognitive dimension
(not the knowledge dimension, nor affective or psychomotor domains); English and Spanish only.
