# Conversa da cena (web): harness captures

**These are HARNESS CAPTURES, not the live stack.** The real components (`app-conversation`, `app-session-tabs`, the audience chooser opened through `openSheet`) were rendered in Chromium with fake data (`ConversationState` filled by hand, fake `RosterClient` / `CampaignsService`, a fake `Audio` for the "Ouvir" states). Nothing ran against the server, the stream or the database. Rendered from commit `7622b9d90` (branch `feat/conversa-web`, after the six fixes below) by a throwaway harness kept outside that branch. Fonts are the app's self-hosted ones; the `board-*.png` files are the approved board rendered offline (Google Fonts unreachable: Georgia/system fallback, compare layout, not glyphs).

Matrix: widths 390 (device scale 2, full-screen phone view), 768 (panel under the board), 1280 (420 px column beside the stage), each in light and dark (`prefers-color-scheme`). File name: `<state>-<role>-<width>-<theme>.png`.

States: `empty`, `lines` (narration with image, NPC, own, out-of-character), `whisper` (player: "Sussurro do mestre · só você lê"; master: "Narração · só para Pensantus e Brisa", "só para Tor", "só o mestre lê"), `chooser` (master, "Só algumas pessoas", two chosen; dialog on 768/1280, bottom sheet on 390; viewport shots), `reconnecting`, `failed`, plus `tabs` (390), `typing`, `composer-ooc`, `falar-como` (select open), `listen-loading|playing|error`.

## Measured against the board (computed styles)

Matches: panel 420 px at 1280 (380-440); narration Alegreya 18/26 on the page ground, 12 px padding, radius 10; NPC portrait 40 px round with 2 px ink border, bold 13/16 name, bordered bubble radius 10, padding 8/12; own line right-aligned, accent-soft; out-of-character 14/19 muted with a 13 px label; restricted: dashed accent border, lock, label; "Falar como" and "Para" 44 px high, radius 8, 1 px control line; "Enviar" 48 px high (48x48 icon on 390); "Ouvir" 44 px; exactly one filled button in each composer (and "Pronto" alone in the chooser); chooser structure (title, note, two radios, checkboxes with class and race muted, live line, Cancelar/Pronto); failed and reconnecting notices with the board's words; typing hint; tab badge.

## Defects of the previous review: all six fixed

Each was re-measured in this harness (computed styles and boxes in Chromium) and looked at against the board.

1. **Fixed.** The master's own NPC line keeps the NPC look: 40 px round portrait, bold 13/16 "Bartolo · você", bordered bubble (1 px), left-aligned (`lines-master-*`, `whisper-master-*`). A player's own character line is still the right-aligned accent-soft "Brisa · você" (`lines-player-*`). The "do not announce back to its author" test no longer depends on the look (`LineView.mine`).
2. **Fixed.** "Falar como" chevron: the glyph now ends 13 px from the select's outer edge (12 px + the 1 px border) at 390 and 1280, the same as the "Para" icon (13 px); before: 46 px. No other select or arrow-bearing control exists in the panel or the chooser (the chooser has native radios and checkboxes only).
3. **Fixed.** Player composer: ONE `role=radiogroup` "Como <personagem> | Fora do personagem" in a rounded ground-tinted track (10 px radius, 4 px padding): options 40 px high on 390 (track 48 px), 36 px on 1280 (track 44 px), the chosen one a raised bold chip. It was the shared segmented control drawn as separate 48/44 px bordered buttons, twice. The shared control got an opt-in `appearance="track"`; every other screen keeps its look. "Para: só o mestre" is a 48 px lock toggle (`aria-pressed`, tooltip "Só o mestre lê") in the send row, so there is no second row.
4. **Fixed.** On 390 the narration is a white card: `--mr-surface`, 1 px `--mr-line` border, 10 px radius, 12 px padding (computed: `rgb(255,255,255)`, `1px solid rgb(213,218,225)`, `10px`, `12px`). On 768/1280 it stays the page-ground block.
5. **Fixed.** Audience bottom sheet (390): title Alegreya 21/26 bold (computed 21px / 26px / 700), no grab handle, a 44x44 close button with `aria-label="Fechar"` and `title="Fechar"`. The dialog (768/1280) and every other sheet are unchanged (the frame got opt-in `handle` and `small`).
6. **Fixed.** The narration picture reserves its box from `width`/`height` (`aspect-ratio: 640 / 360`, so 332x186.8 at 390 before it loads) and the feed scrolls to the end again on the image's `load` only when the reader was at the end. Covered by specs (a reader at the bottom stays at the end; one who scrolled up is not moved); the captures wait for the images and still end on the newest line.

Still different from the board: the player's "Para" choice is an icon toggle (the board's composer has none; the spec allows it); the switch options are 36/40 px as the board, below the 44 px target of the other controls by the owner's decision; fonts in `board-*.png` are fallbacks.

Not captured: the real live stack (stream, gallery picker, real voice audio), the full session page around the panel (only the panel and the tab bar).
