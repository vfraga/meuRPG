# Maps

One map component, `shared/map-view`, serves the editor, the player's viewer and the session page; battle maps use `shared/combat-map`, and fog maps `shared/fog-map`.

## Rules

- **The player never receives what is hidden.** Hidden points, tokens, traps and secret doors do not exist on the player's screen, not even drawn grey: the server does not send them and the map component filters again.
- **State is line, icon and word, never colour alone:** dashed + seal + "Escondido".
- **The map's marks use the `--mr-map-*` tokens,** which do not follow the theme, like the image.
- **A pointer-free path always exists:** the lists "Pontos do mapa" and "Tokens no mapa" (`shared/map-lists`), arrows on a chosen item, a hidden list of positions.
- **A button the server would refuse is dashed but clickable,** with the reason under it linked by `aria-describedby`.

## `app-map-view`

- The box has the image's aspect ratio (nothing moves while it loads) and never exceeds about 78% of the screen height.
- Points and tokens sit at base points (0 to 10000) as a percentage.
- Zoom 100% to 400% without a library: wheel around the pointer, two-finger pinch (one finger still scrolls the page), drag to pan, and the 44 px buttons "Reduzir", "Ampliar", "Ajustar à tela" and "Tela cheia" (`aria-pressed`; the browser's full screen, left out where the API does not exist; `FullscreenOverlay` keeps dialogs working). On a phone the buttons sit below the map.
- Modes: `preview` (still), `view` (pans and zooms, points open), `tokens` (the master drags tokens) and `edit` (drags points).
- A point marker's shape tells its kind: diamond with crossed swords (Batalha), rounded square with stairs (Submapa), circle with a speech bubble (Cena de RP); 34 px with a 44 px touch area. Hidden is dashed with `ink-muted` icon and the seal. Chosen is a 3 px `accent` border.
- A label is a pill below the marker; the map measures it and slides it inside the image (`placeLabel`, `shared/map-view/map-geometry.ts`). Points on the same square share one label.
- A token is a 26 px `ink` disc with the initial; the player's own has an `accent` halo and "(você)".

## Legend

The master's map has one row of token marks ("Personagem · NPC · Escondido (só você vê) · Vez") and no line about dragging; with fog on, the fog map's own legend leaves those marks to that row (no word twice) and keeps the layers and pins. The player's scene tab on a phone captions its map "Mapa · <nome>".

Below every master's map, `app-map-layers-legend` lists only what the map draws, in the same order everywhere: wall, difficult terrain, the covers, doors (closed, open, locked, portcullis, secret), then the marks and tokens. Painted light is called "Claro", "Penumbra", "Escuro" everywhere.
A swatch is `.mr-swatch` (22 px, `aria-hidden`); the words carry the meaning.

## Layers (`shared/map-layers`)

`MapLayersOverlay` draws over the image the squares decoded by `core/maps/layers.ts` from `GetMapLayers` (drawing, not rule):

- **Parede:** a dark diagonal hatch over a veil.
- **Terreno difícil:** a lighter crossed hatch.
- **Não dá para passar:** a checkerboard of the map's ink (a pit, a chasm, a column of fire). It blocks movement, not sight, and is the only mark that is neither a hatch nor a pictogram; the legend names it "Não dá para passar" only when the map has one.
- **Cobertura:** a dotted square filled from the bottom to halfway ("Meia cobertura") or three quarters ("Três quartos"). Never an object's name.
- **Doors** (`app-door-mark`): one mark per kind, the same on the map, legend and editor buttons; the player's layer has no padlock or secret door.

## Fog of war (`shared/fog-map`)

The browser draws only what the server sends: each square comes from `GetMapVision` (`core/maps/vision.ts` only unpacks it), the image comes in 16 × 16-square tiles and tokens arrive filtered.
One component (`app-fog-map`) serves the player, the master's own map ("Todos") and "Ver como".

- **Inputs of the table screen:** `tools` (default on) draws the zoom, fit and full-screen buttons under the map, and the TV turns it off; the picture is at most `--fm-max-h` tall (78 vh by default) and the TV's card sets it to the height it has, so the legend never leaves the card.
- **Square states:** Visto (the image as is), Penumbra (25% black), No escuro em cinza (no colour), Já visto (60% black with fine dots, no NPCs, layer marks faded) and Não visto (black). Contrast comes from brightness and texture.
- **Loading** shows thin still stripes and "Carregando o mapa" only on the first load of a map and player; the party's tokens always show.
- **Tokens:** the player's own has an `accent` ring and is above the others; creature tokens have the dashed ring.
- **"Você vê"** under the legend tells the player what they can use (darkvision, carried light, enemies in sight).
- **Carried light:** "Luz que você carrega" opens a sheet with radio cards; the choice applies at once ("Pronto" only closes).
- **"Ver como"** (master): a radio group with one tab stop beside the map; choosing a character shows an amber strip and the same map component with that player's tiles. The combat order has no "Ver como".

## The map editor (`pages/maps`)

Computer only (768 px and up); on a phone the master pans and zooms and uses the lists.

- **Bar:** "Pontos | Pintar" selector; in "Pintar" the tools (Terreno difícil, Parede, Cobertura, Luz, "Não dá para passar", Porta, "Apagar"), the brush (1×1, 3×3) and the degree row. The chosen item always looks the same: `accent-soft` and a check in a slot that already exists, so choosing never shifts neighbours. Without a grid no tool looks chosen and the reason is written in the bar.
- **Painting:** the surface is above the markers and moves with the map; one finger paints on touch and two fingers pan; arrows move the brush cursor and Space paints.
- **One tap, one action for doors:** the door is written before the wall disappears, and the reverse, so no player sees a gap.
- **Questions in place** (`app-map-ask`): "Mudar a grade?", "Trocar a imagem?", "Esquecer o que foi visto?" and the leave-with-unsaved-strokes question share one shape; nothing is erased before the second click.
- **Layers panel:** shows "Tudo salvo", "Salvando" or "Não salvou" with the reason and "Tentar de novo" only when trying again can help.
- **Point panels** (Luz, Armadilha, Tesouro) follow the usual panel contract: filled "Salvar ponto", same-size "Apagar ponto" that asks in place, a field error shown after the first save attempt.
- **Doors in session** (`door-sheet`): each door has a 44 px transparent button; the sheet applies the choice at once; "Revelar a porta secreta" asks first.

## Printing (`pages/maps/map-print`)

Master only. The paper rule: what comes out of the printer is print CSS only. The sheets (`print-sheets`) exist on the page only while the browser prints (`beforeprint` / `afterprint`); in `@media print` the screen disappears and one sheet per page remains. The chosen paper's `@page` (size and 1 cm margin) is written to a `<style>` in `head` while the screen is open.
Sheet colours are fixed (white paper, black ink): paper has no dark theme. The grid is SVG in centimetres over the image, never an edit of the image. Numbers and units never split (`tightCm`, `joinDots`).
"Imprimir com a grade" without a grid is dashed with the reason beside it.
