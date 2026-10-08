# Sweep brief (shared by every haiku sweep)

Repo: /home/user/meuRPG (Go backend in backend/, Angular app in web/src/app, protos in proto/meurpg, docs in docs/).
Rules engine: backend/internal/rules/** (incl. combat/, grid/, vision/); live play + combat: backend/internal/play/**;
characters: backend/internal/characters/**; maps/fog/doors/traps: backend/internal/maps/**; screens: web/src/app/{pages,shared,core}/**.
SRD text (read-only, outside repo): /tmp/5e-srd-api/packages/5e-database/src/2014/en/5e-SRD-Rules.json
(array of sections: index, name, desc[list of paragraphs], children) and 5e-SRD-Conditions.json.
Read the section's SRD text first (python3 -I -c ...), list its distinct MECHANICS (each rule a table could apply), then for each
mechanic search the code (Grep with several Portuguese AND English terms: the UI is Portuguese; identifiers are mixed) and the
docs (docs/product/rules.md, docs/product/stories.md, docs/architecture.md, docs/roadmap.md).
DO NOT edit any repo file. Write your output ONLY to the raw file named below.
Output format, one block per mechanic, no opinions, no prose:
  MECHANIC: <section index> / <short name>
  SERVER: file:line (what the code does, 1 line) | or "no match for <terms searched>"
  SCREEN: file:line (what the UI shows/asks, 1 line) | or "no match for <terms>"
  DOCS: file:line (what the doc claims, quote <=15 words; say if it claims deliberately-not-built) | or "no match"
  TESTS: file:line of a test that exercises it, if any
Every file:line must be one you actually opened. If unsure, say "unsure". Be exhaustive on mechanics; be terse per mechanic.
