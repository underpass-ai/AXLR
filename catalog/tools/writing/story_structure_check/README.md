# story_structure_check

Compares an outline with a story structure. Each scene names its beat (an id such as
`break_into_two`, the beat's name, or an alias such as `inciting incident`, `pp1`) and where it
falls: `position` as a percentage of the story, or `page` with `total_pages` (pages of a
script, minutes of a film, chapters…). For each beat of the framework the tool reports the
expected range, the position given, and `ok`, `early`, `late` or `missing`; `tolerance`
(default 5 percentage points) widens every range on both sides. A beat placed after another
whose whole range comes later is listed in `out_of_order`; labels it does not know are in
`unknown_labels`. Percentages are computed exactly (fractions) and shown with two decimals.

| `framework` | Beats and reference |
|:--|:--|
| `save_the_cat` | Blake Snyder's Beat Sheet (*Save the Cat!*, 2005, ch. 4) on its published pages of a 110-page script: Opening Image 1, Theme Stated 5, Set-Up 1–10, Catalyst 12, Debate 12–25, Break into Two 25, B Story 30, Fun and Games 30–55, Midpoint 55, Bad Guys Close In 55–75, All Is Lost 75, Dark Night of the Soul 75–85, Break into Three 85, Finale 85–110, Final Image 110. Percent = page ÷ 110 |
| `three_act` | Syd Field's paradigm (*Screenplay*, 1979) for 120 pages: Act I Setup 1–30, Plot Point I 25–27, Act II Confrontation 30–90, Midpoint 60 (*The Screenwriter's Workbook*, 1984), Plot Point II 85–90, Act III Resolution 90–120 |
| `heros_journey` | Christopher Vogler's 12 stages (*The Writer's Journey*). Vogler places stages 1–5 in Act One, 6–9 in Act Two with the Ordeal as its central crisis, 10–12 in Act Three, but gives no page numbers; the ranges are this tool's reading on acts of 25/50/25 %: Ordinary World, Call, Refusal, Mentor 0–25; Threshold 20–30; Tests and Approach 25–50; Ordeal 40–60; Reward 50–75; Road Back 70–80; Resurrection 75–100; Return 90–100 |

With `total_pages`, the expected range is also given in pages scaled to that length
(`expected_pages`). `list_beats: true` lists a framework's beats, pages and percentages.

The tests reproduce Snyder's page numbers and their percentages worked by hand (Catalyst
12/110 = 10.91 %, Midpoint 55/110 = 50 %, All Is Lost 68.18 %, Break into Three 77.27 %),
an outline placed on those pages, Field's plot points on a 120-page script, and an
out-of-order hero's journey.

Limits: one position per beat (the first scene labelled with it counts); range beats are
checked at the scene's start; the frameworks are craft conventions, and a story that departs
from them is not wrong.
