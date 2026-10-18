# letter_formal

Builds the skeleton of a formal letter: sender and recipient blocks, the date line, reference
and subject lines, the salutation, the body (given, or a placeholder), the closing and a
signature placeholder. It knows the conventions that are easy to get wrong in each language;
the date always comes from the input.

| Language | Nobody in particular | Named recipient | Date line |
|:--|:--|:--|:--|
| `en` UK | Dear Sir or Madam, → Yours faithfully, | Dear Ms Smith, → Yours sincerely, (Mr, Ms, Dr without a full stop) | 18 October 2026 |
| `en` US | Dear Sir or Madam: → Sincerely, | Dear Dr. Smith: → Sincerely, (Respectfully, when very formal) | Boston, October 18, 2026 |
| `es` | Estimados señores: → Atentamente, | Estimada Sra. García: → Atentamente,; very formal Distinguida Sra. García: → Le saluda atentamente, | Madrid, 18 de octubre de 2026 |
| `fr` | Madame, Monsieur, → Veuillez agréer, Madame, Monsieur, l'expression de mes salutations distinguées. | Madame, / Monsieur, (never the surname); very formal Monsieur le Directeur, → Je vous prie de croire, Monsieur le Directeur, à l'assurance de ma haute considération. | Paris, le 1er octobre 2026 |
| `de` | Sehr geehrte Damen und Herren, → Mit freundlichen Grüßen (no comma) | Sehr geehrte Frau Dr. Müller, / Sehr geehrter Herr Müller,; gender not given: Guten Tag Alex Müller, | Berlin, 18. Oktober 2026 |
| `pt` | Exmos. Senhores, → Com os melhores cumprimentos, | Exma. Senhora Maria Silva, | Lisboa, 18 de outubro de 2026 |
| `it` | Gentili Signori, or Spettabile Acme, → Cordiali saluti | Gentile Signora Rossi, / very formal Egregio Signor Rossi, → Distinti saluti | Roma, 18 ottobre 2026 |

Gender is never guessed from a name: without `recipient.gender` the tool uses the full name
(English, German "Guten Tag", Italian) or the double form (Spanish "Estimado/a", French "Madame,
Monsieur", Portuguese "Exmo.(a) Senhor(a)"). The French closing always repeats the salutation.
German notes remind that the body starts in lower case after the comma and that DIN 5008 writes
the subject without the word "Betreff".

The administrative register follows each country's petition format where there is one: in
Spanish, the *escrito* or *instancia* with the applicant's identification, `EXPONE` (the body,
numbered), `SOLICITA` (`request`), "En Madrid, a 18 de octubre de 2026.", the signature and the
addressed body in capitals at the foot; in Portuguese, the *requerimento* closing "Pede
deferimento."; elsewhere the formal letter with the stricter closings.

## Tests

The UK faithfully/sincerely rule as the Cambridge Dictionary entries state it, the US colon and
"Sincerely" (Gregg Reference Manual), the German DIN 5008 / Duden formulas, the French formula of
the service-public.fr model letters with "le 1er", the Spanish colon and lower-case month of the
RAE, the Spanish administrative layout, Italian and Portuguese closings, and the errors.

## Limits

A skeleton, not a writer: the body is yours. Only the six languages; Portuguese follows
European usage (Brazil prefers "Prezado Senhor" and "Atenciosamente"). Honorifics beyond the
given `title`, and protocol forms for royalty, clergy or ambassadors, are not covered.
