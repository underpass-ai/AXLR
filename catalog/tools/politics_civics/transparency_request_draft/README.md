# transparency_request_draft

Drafts a request for public information that a public body has to take as a formal request.

| `jurisdiction` | Law | What the draft includes | Response time |
|:--|:--|:--|:--|
| `es` | Ley 19/2013 (and art. 105.b of the Constitution) | identity, information requested, address for notifications, preferred format (art. 17.2), no reasons needed (17.3), partial access (16) | one month, extendable by one (20.1); silence means refusal (20.4) |
| `eu` | Regulation (EC) 1049/2001 | documents requested, no reasons needed (6(1)), partial access (4(6)) | 15 working days, extendable by 15 (7(1), 7(3)) |
| `us` | FOIA, 5 U.S.C. § 552 | records reasonably described, fee category, optional fee waiver, segregable portions (§ 552(b)) | 20 working days, extendable by 10 (§ 552(a)(6)) |
| `uk` | Freedom of Information Act 2000 | name, address, the information, preferred means (s. 11), the rest if part is exempt (s. 17) | 20 working days (s. 10) |

The Spanish draft is in Spanish, the others in English. With `date` the output adds the due
date (calendar months for Spain; weekdays for the working-day laws, without public holidays).
`next_steps` says what follows a refusal or silence: the Consejo de Transparencia y Buen
Gobierno, a confirmatory application and the European Ombudsman, an agency appeal and OGIS, an
internal review and the ICO.

Limits: Spanish regional and local bodies have their own transparency laws and portals, and
Scottish authorities answer under FOISA 2002; check the right law before sending. Not legal
advice; the output carries that disclaimer.
