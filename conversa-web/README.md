# Conversa da cena (web): harness captures

**These are HARNESS CAPTURES, not the live stack.** The real components (`app-conversation`, `app-session-tabs`, the audience chooser opened through `openSheet`) were rendered in Chromium with fake data (`ConversationState` filled by hand, fake `RosterClient` / `CampaignsService`, a fake `Audio` for the "Ouvir" states). Nothing ran against the server, the stream or the database. Rendered from commit `95f273065` (branch `feat/conversa-web`) by a throwaway harness kept outside that branch. Fonts are the app's self-hosted ones; the `board-*.png` files are the approved board rendered offline (Google Fonts unreachable: Georgia/system fallback, compare layout, not glyphs).

Matrix: widths 390 (device scale 2, full-screen phone view), 768 (panel under the board), 1280 (420 px column beside the stage), each in light and dark (`prefers-color-scheme`). File name: `<state>-<role>-<width>-<theme>.png`.

States: `empty`, `lines` (narration with image, NPC, own, out-of-character), `whisper` (player: "Sussurro do mestre · só você lê"; master: "Narração · só para Pensantus e Brisa", "só para Tor", "só o mestre lê"), `chooser` (master, "Só algumas pessoas", two chosen; dialog on 768/1280, bottom sheet on 390; viewport shots), `reconnecting`, `failed`, plus `tabs` (390), `typing`, `composer-ooc`, `falar-como` (select open), `listen-loading|playing|error`.

## Measured against the board (computed styles)

Matches: panel 420 px at 1280 (380-440); narration Alegreya 18/26 on the page ground, 12 px padding, radius 10; NPC portrait 40 px round with 2 px ink border, bold 13/16 name, bordered bubble radius 10, padding 8/12; own line right-aligned, accent-soft; out-of-character 14/19 muted with a 13 px label; restricted: dashed accent border, lock, label; "Falar como" and "Para" 44 px high, radius 8, 1 px control line; "Enviar" 48 px high (48x48 icon on 390); "Ouvir" 44 px; exactly one filled button in each composer (and "Pronto" alone in the chooser); chooser structure (title, note, two radios, checkboxes with class and race muted, live line, Cancelar/Pronto); failed and reconnecting notices with the board's words; typing hint; tab badge.

## Defects vs the board

1. **Medium.** The master's own NPC line is drawn as an "own" line (right-aligned, accent-soft, "Bartolo · você") instead of the board's NPC look (portrait, bold "Bartolo, o taverneiro · você", bordered bubble, left). Cause: `core/conversation/message-view.ts:88` (`baseLook`) returns `own` for any NPC/CHARACTER line whose author is the viewer. Fix: for a master viewer return `npc` for `Kind.NPC` (keep the "· você" label in `baseLabel`), keeping `own` only for a player's character line.
2. **Low-medium.** "Falar como" chevron is not 12 px from the right edge: measured 46 px (arrow wrapper 53 px) at 1280 and 390. `master-composer.scss:52-55` positions `.mat-mdc-select-arrow-wrapper` absolutely, but its containing block is `.mat-mdc-select-trigger` (position: relative, already inside the 40 px padding). Fix: `.composer__select { position: relative; ::ng-deep .mat-mdc-select-trigger { position: static; } }` (or `right: calc(var(--mr-space-3) - 40px)`).
3. **Low.** Player composer is heavier than the board: two rows of four large toggles (Como/Fora + Todos/Só o mestre) instead of the single compact segmented switch; costs about 100 px of the phone feed. The extra "Para" row is allowed by the spec; consider a compact segmented look or showing "Para" only on demand.
4. **Low.** On 390 the board puts narration on a white card over a grey page; the app uses the desktop look (grey block on the surface). The two boards disagree; the app follows Main.
5. **Low.** Bottom sheet: title is larger than the board's and there is a close X and grab handle the board does not show. Dialog (768/1280) is fine.
6. **Low (observed).** A `loading="lazy"` narration image can grow after the feed scrolled to the end, leaving the newest line below the fold until the next scroll (seen when the first line has an image; captures scroll again after the image loads).

Not captured: the real live stack (stream, gallery picker, real voice audio), the full session page around the panel (only the panel and the tab bar).
