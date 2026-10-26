# font-size tokens: before and after

`before/` is `origin/main`; `after/` is `chore/font-size-tokens`. Six screens, at 390 and 1280, light and dark (24 pairs), taken with the same throwaway harness, fixed data, animations off.

| Screen | What is drawn |
| --- | --- |
| 1-sessao | The master's live session (the real page, "Mapa" view) |
| 2-combate | The master's combat: the map card and an NPC's turn card |
| 3-ficha | The sheet's "Características e traços" panel (languages and proficiencies) |
| 4-editor | The editor's "Escolhas" step |
| 5-campanha | The campaign page (the real page, "Mesa" tab) |
| 6-dialogo | "Ajustar Pensantus": a dialog at 1280, a bottom sheet at 390 |

Comparison: every pair decoded and compared pixel by pixel (Pillow `ImageChops.difference`): **24 of 24 identical** (same size, no differing pixel).

Besides the screenshots, the whole app was compared: the built global stylesheet with the tokens put back as literals is byte for byte the one from `main`, and the 2,787 `font-size` declarations of the component styles are the same multiset before and after.
