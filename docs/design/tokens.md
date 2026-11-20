# Design tokens

Tokens are CSS custom properties on `html` in `web/src/styles.scss`, prefixed `--mr-`.
Each colour is one `light-dark()` declaration, so a token works in both themes (`color-scheme: light dark` follows the operating system).
Components use tokens and never a raw colour.

## Colour

| Token | Light | Dark | Use |
| --- | --- | --- | --- |
| `--mr-ground` | `#eef0f3` | `#11151c` | Page background |
| `--mr-surface` | `#ffffff` | `#1a1f29` | Panels, app bar, fields |
| `--mr-line` | `#d5dae1` | `#2e3542` | Panel border (decorative) |
| `--mr-rule` | `#e3e7ec` | `#262c38` | Divider between a panel's rows |
| `--mr-ink` | `#1b2230` | `#e6e9ef` | Text, medallion border, filled proficiency dot |
| `--mr-ink-muted` | `#525b6b` | `#a3abb9` | Labels and supporting text |
| `--mr-control-line` | `#78818f` | `#7d879a` | Field border and the outline of every unchecked box, ring, switch and spell slot; at least 3:1 (WCAG 1.4.11) against card, page and `accent-soft` |
| `--mr-accent` | `#9e2b3b` | `#e88593` | Burgundy: main button, AC and HP outline, active tab, focus |
| `--mr-accent-strong` | `#7a1f2d` | `#f2a9b3` | Hover and pressed of the main button, link hover |
| `--mr-on-accent` | `#ffffff` | `#2a0a10` | Text on burgundy |
| `--mr-accent-text` | `#9e2b3b` | `#f0a0ab` | Links and text buttons |
| `--mr-accent-soft` | `#f6e3e6` | `#3a1e24` | Selected item |
| `--mr-focus` | `--mr-accent` | `--mr-accent` | The 2 px focus ring |
| `--mr-warning-surface` / `-line` / `-ink` | `#fbf3e2` / `#e6cf9f` / `#6e4700` | `#2e2616` / `#6b5320` / `#f0d08a` | Rule pendings, the Pendente state, warnings |
| `--mr-danger-surface` / `-ink` | `#fce8e6` / `#8c1d18` | `#3b1a1a` / `#f4b4ae` | Errors, the Morto state, destructive confirmations |
| `--mr-success-surface` / `-ink` | `#e3f2ea` / `#1f6b45` | `#16301f` / `#9ed8b4` | Confirmations, the Aprovado state |

### Map tokens

A map's picture is not themed, so what is drawn on it does not follow the theme either.
These are single values, the same in both themes.

| Token | Value | Use |
| --- | --- | --- |
| `--mr-map-token-ink` | `#1b2230` | Token letters and borders, grid, marks |
| `--mr-map-token-surface` | `#ffffff` | Token fill, label chips |
| `--mr-map-token-down`, `-down-line`, `-down-ink` | `#d5dae1`, `#8a93a3`, `#525b6b` | A defeated token |
| `--mr-map-accent` | `#9e2b3b` | Reach, chosen square, area templates |
| `--mr-map-fog` | `#000000` | Fog of war: what was never seen, and the darkening of what was |
| `--mr-map-water` | `#2f6f95` | Superfície, Água: the blue of a water square, under white waves; Escalável has no colour of its own, a pale veil under dark chevrons (`--mr-map-water-waves` and `--mr-map-climb-chevrons` hold the two drawings). The marks carry the meaning, never the blue alone |
| `--mr-map-sample` | `#a8977a` | A sample of a map's floor under the legend swatches |

### Dice tray tokens

The dice tray of the roll overlay ([session.md](session.md#rolls)) is drawn on a canvas, which reads these tokens from the page.
The paper die is the sheet's paper and ink; the garnet is the armour class shield's (`--mr-accent`), a darker one in the dark theme so a gold critical number keeps its contrast.
Numbers are at least 4.5:1 against their face and a die's edge is at least 3:1 against the felt, in both themes; `tray-tokens.spec.ts` measures it from the values in `styles.scss`.

| Token | Light | Dark | Use |
| --- | --- | --- | --- |
| `--mr-tray-felt` | `#a9bda3` | `#1d3327` | The tray's floor |
| `--mr-tray-rim` | `#7a5232` | `#5b3d25` | The wooden rim (its light and dark edges are mixed from it) |
| `--mr-tray-shadow` | `#26352a` | `#050d09` | The dice's shadows and the walls' shade on the felt |
| `--mr-die-paper` | `#ffffff` | `#2a3140` | A paper die's face |
| `--mr-die-paper-shade` | `#8d95a3` | `#0c1016` | The darker faces of a paper die |
| `--mr-die-ink` | `#1b2230` | `#e6e9ef` | Numbers on a paper die |
| `--mr-die-garnet` | `#9e2b3b` | `#8a2232` | A garnet die's face (the d20, the d100's tens die) |
| `--mr-die-garnet-shade` | `#3a0e16` | `#2b0810` | The darker faces of a garnet die |
| `--mr-die-on-garnet` | `#ffffff` | `#fdecee` | Numbers on a garnet die |
| `--mr-die-edge` | `#1b2230` | `#e6e9ef` | The outline of a die |
| `--mr-die-gold` | `#f8d36e` | `#f0c35a` | A critical: the glow, the number, the edge and the "Crítico!" label |
| `--mr-die-on-gold` | `#2a1d02` | `#2a1d02` | Text on the gold label |

The labels beside the dice ("descartado", "vale", "Falha crítica") use `--mr-surface`, `--mr-ink`, `--mr-accent-text`, `--mr-line` and `--mr-ink-muted`.

## Fonts

Two OFL families, served by the app (`@fontsource/alegreya` and `@fontsource/alegreya-sans`, Latin subset, which covers Portuguese), woff2 only.
Never Google Fonts: the CSP (`font-src 'self'`) and [privacy](../privacy.md) forbid asking a third party for a font.

| Token | Family | Faces | Use |
| --- | --- | --- | --- |
| `--mr-font-display` | Alegreya, Georgia, serif | 500, 700, 800 | Headings (`h1` to `h3`), the game numbers, names |
| `--mr-font-sans` | Alegreya Sans, Segoe UI, system-ui, sans-serif | 400, 500, 700 | Body, labels, buttons |

Each `@font-face` carries `ascent-override` and `descent-override` (Alegreya Sans 92% and 28%, Alegreya 100.5% and 35.6%).
They put the middle of the capitals at the middle of the line without changing its height, so an icon centred next to words lines up with its capitals; `e2e/tests/layout.ts` measures it.

The base is `body`: Alegreya Sans, 16 px, line height 1.4, `font-variant-numeric: lining-nums tabular-nums` (bonus columns line up).
Headings use `text-wrap: balance`.

### Type scale

Every font size the screens use is a token on `html`, named by its pixels: `--mr-font-14` is 14 px.
Components write `font-size: var(--mr-font-14)` and never a literal pixel size; a size that is not in the table is a one-off of its component and stays a literal.

| Token | Size | Where it is used |
| --- | --- | --- |
| `--mr-font-8`, `-11` | 8, 11 px | The smallest marks and captions |
| `--mr-font-12`, `-13` | 12, 13 px | Captions and labels (`.mr-label` is 13), hints under a field |
| `--mr-font-14`, `-15` | 14, 15 px | Supporting text (`.mr-tag` and `.mr-list__sub` are 14) |
| `--mr-font-16` | 16 px | Body, buttons, `.mr-notice` |
| `--mr-font-17`, `-18`, `-19` | 17, 18, 19 px | Lead text (`.mr-page-lead` is 17) and names (`.mr-list__name` is 18) |
| `--mr-font-20`, `-21`, `-22` | 20, 21, 22 px | Panel and card titles (`.mr-panel__title` is 21) |
| `--mr-font-24`, `-26`, `-28` | 24, 26, 28 px | Section headings and figures |
| `--mr-font-30`, `-32`, `-34`, `-36`, `-38` | 30 to 38 px | Page titles (`.mr-page-title` is 34, 38 from 768 px) and large figures |
| `--mr-font-40`, `-48` | 40, 48 px | The largest figures |

Not tokens, because no scale explains them: sizes used once or twice (9, 10, 23, 25, 44, 46, 52, 56 and 92 px, and 12.5 px), the `rem` and `pt` sizes (they follow the reader's own size or the printer), and the sizes computed from a component's own measure (`calc()`, `clamp()`, `min()` and container units).

Sizes the shared classes fix (the rest belong to their component):

| Where | Size / line height / weight |
| --- | --- |
| `.mr-page-title` | 34 / 38 / 800 (38 / 40 from 768 px), Alegreya |
| `.mr-page-lead` | 17, `ink-muted` |
| `.mr-panel__title` | 21 / 26 / 700, Alegreya |
| `.mr-label` | 13 / 16 / 500, `ink-muted`, never in capitals |
| `.mr-list__name` / `.mr-list__sub` | 18 / 700 and 14 `ink-muted` |
| `.mr-tag` | 14, 26 px high; `--compact`: 12, 22 px high |
| `.mr-notice` | 16 / 22 |
| Buttons | 16 / 700 (Material override) |

## Space and shape

| Token | Value |
| --- | --- |
| `--mr-space-1` to `-6` | 4, 8, 12, 16, 20, 24 px (`-1`, `-2`, `-3`, `-4`, `-5`, `-6`) |
| `--mr-space-8` | 48 px |
| `--mr-gutter` | the page's side margin: `--mr-space-4` (16 px), `--mr-space-8` (48 px) from 768 px |
| `--mr-radius-sm` | 8 px: buttons, fields, Material extra-small and small corners |
| `--mr-radius-md` | 10 px: notices |
| `--mr-radius-lg` | 12 px: panels, Material medium corner |
| `--mr-radius-xl` | 14 px: medallions, header, dialogs, bottom sheets, Material large corner |
| `--mr-radius-pill` | 9999 px: tags |

## Material mapping

`styles.scss` calls `mat.theme` with the red palette and points Material's system tokens at ours, so every Material component speaks the same palette:

- `primary` is `--mr-accent`; `on-primary` is `--mr-on-accent`; `primary-container` and `secondary-container` are `--mr-accent-soft`; `secondary` is `--mr-ink-muted`.
- `background`, `surface` and the `surface-container-*` family are `--mr-surface`, except `surface-variant` and `surface-container-highest`, which are `--mr-ground`.
- `outline` is `--mr-control-line`, `outline-variant` is `--mr-line`.
- `error` is `--mr-danger-ink` with `--mr-danger-surface` as container.
- Typography: `plain-family` is `--mr-font-sans`, `brand-family` is `--mr-font-display`; density 0.

Component overrides in the same file:

- **Buttons:** 8 px corners on every kind, weight 700, size 16, 44 px high (the filled one 48 px under 768 px).
- **Checkbox and radio:** the unchecked outline is `--mr-control-line` (`mat.checkbox-overrides`, `mat.radio-overrides`).
  A native checkbox or radio that stays visible takes the class `mr-check-input` ([components.md](components.md#global-classes)), because the browser's own unchecked box is nearly invisible in the dark theme.
- **Dialogs and bottom sheets:** 14 px corners; a bottom sheet keeps its top corners round at every width.
- **Sheet panel classes** (`panelClass` of `openSheet`): `.mr-sheet` lets the container use `100dvh − 24px`; `.mr-sheet-tall` leaves 84 px at the top (the combat log); `.mr-sheet-image` leaves 40 px (24 px under 360 px wide; the image-generation sheet); `.mr-sheet-over` is a dialog drawn like a sheet, to open over another sheet without closing it.
- **`.mr-nd-panel`:** the challenge-rating select of the NPC editor, six 44 px rows and then it scrolls.
