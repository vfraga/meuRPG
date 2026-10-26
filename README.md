# Dados em 2D: a dice-rolling prototype

`index.html` is one self-contained page: open it in a browser. It has no libraries, no CDN scripts and no network calls. The only exception is the Google Fonts link for Alegreya and Alegreya Sans, and the page still works without it. Everything is in one `DiceTray` object, in the first `<script>`. The second `<script>` is the demo board.

## How it works

1. **The whole roll is simulated before anything is drawn.** It runs at a fixed step of 1/120 s and every step is recorded. That makes it deterministic: the same dice, tray size and seed give the same roll on any screen.
   - Each die is a circle on the table (x, y), with a height (z) for the throw and hops, a flat spin, and a *tumble*.
   - The tumble is a progress `p` from 0 to 1 through tipping over one edge onto the next face. On the felt it follows rolling without slipping. In the air it keeps turning. On an edge, the die falls back before half-way and over after it.
   - Collisions, with the walls and between dice, are circles with restitution and Coulomb friction. A glancing hit turns into spin.
   - On the felt, the die slows with rolling resistance, a speed loss each time it lands on a face (a d4 stops dead, a d20 runs on) and a brake after 1.6 s.
2. **Hindsight: the result is set after the fact, by walking the recording backwards.** The dice are real polyhedra; the d10 is a pentagonal trapezohedron. Walking back from the server's result at rest, each stretch of the tumble becomes a real tip over a real edge of the face the die rests on: where a tip started is where it ended, turned back over that edge by the edge's angle.
   - So the die lands flat on every face on the way, ends on the result with its number upright (±14°), and moves exactly as simulated.
   - The physics is never steered toward the result. Since the collisions are circles, the orientation cannot change the motion, so nothing snaps at the end.
   - A headless sweep, 4,200 rolls over 7 scenarios and 2 tray sizes, read the top face back from the final pose: it matched the server's result in every roll. Every intermediate pose was flat to 1e-15.
3. **Playback:**
   - Interpolates the recorded steps with `requestAnimationFrame`, at the chosen speed.
   - Draws each die from straight above with an orthographic projection, flat shading in bands, ink edges and a flat offset shadow. It reads as a drawing, but it really rolls.
   - Prints the numbers on the faces through each face's own projected axes, so a face turning away squeezes its number exactly as it would look.
   - Once the dice settle, the side numbers step back and only the result stays strong.

## API

```js
const tray = DiceTray.create(canvas, { width: 480, height: 260, reducedMotion: false, sound: false });
const out = await tray.roll(
  [{ sides: 20, result: 17, tag: 'vale' }, { sides: 20, result: 4, discarded: true }],
  { seed: 42, speed: 1 },
);
// out: { cancelled, seed, durationMs, dice: [{ sides, result, discarded }] }
```

- **Die fields:**
  - `sides`: 4, 6, 8, 10, 12, 20 or 100. A d100 is two d10, "dezenas" ("00" to "90") and "unidades" ("0" to "9"); 100 is "00" and "0".
  - `result`: required. It always comes from the caller.
  - `discarded`: drawn at half strength, with a "descartado" label.
  - `tag`: a label such as "vale".
  - `critical`: a gold glow, gold number, sparks and "Crítico!".
  - `fumble`: darkened, a short shake, "Falha crítica".
  - `style`: `'paper'` or `'garnet'`. The default is garnet for a d20 and for the d100's tens die.
- **Roll options:** `seed` is a uint32 (random when omitted). `speed` is the playback rate, where 1 is normal; it does not change the physics.
- **Calls:**
  - A new `roll` cancels the one in flight, which resolves with `cancelled: true`.
  - `finish()` jumps to the end.
  - `resize(w, h)` lays the dice out again, at rest.
  - `setReducedMotion(on)`: the dice simply appear with their results.
  - `setSound(on)`: call it from a tap (browsers start audio only after one). See [Sound](#sound).
  - `setSoundLevels({ volume, taps, rumble, room })`: multipliers from 0 to 3 (1 = as designed), for tuning by ear.
  - `refreshTheme()` re-reads the colour tokens.
  - `redraw()` repaints, for example after the fonts load.
  - `clear()` and `destroy()`.
- **For tests:** `DiceTray.simulate(dice, { width, height, seed })` runs the same simulation headless. It returns the duration, the faces each die went through, the result read back from the final pose, and `sound` (the sound events by kind and `quietMs`, the longest stretch with a die rolling and no event). `DiceTray.renderSound(dice, { seed, levels })` renders a roll's audio offline (browsers only) and resolves with the `AudioBuffer`.
- **Colours:** custom properties read from the canvas: `--tray-*`, `--die-*`, `--surface`, `--ink`, `--line`, `--accent`, `--font-display`, `--font-text`.

## Sound

**Off by default, and quiet:** the tray starts with `sound: false`, and the master level is half of what was first measured (−6 dB). Everything is synthesized when the sound is first switched on; nothing is loaded. The simulation already knows every impact, so the sound is scheduled from it on the audio clock, about 120 ms ahead of the frame.

- **What makes a sound:**
  - two dice hitting (a bright resin clack);
  - a die against the wooden rim (a knock with the die's click on top);
  - every landing on the felt, including the soft one that does not bounce (a muffled thud);
  - every face a die tips onto while it rolls on the felt (a soft tap). These taps are the rolling sound: they come closer together the faster the die rolls;
  - a die rocking back onto its face as it settles (the same tap, lower).
- **Under the taps:** a soft rumble in the 600 Hz band (a phone's speaker plays next to nothing below about 300 Hz). Its level follows how fast the dice roll, so it grows with more dice.
- **Variety:** four takes of each sound. The pitch follows the die (a d4 higher, a d20 lower) and the strength of the hit, and a soft hit is duller. The pan follows where the die is across the tray.
- **Many dice:** one die sounds at most once per 18 ms. Past 6 voices in 30 ms, the soft taps drop out. Everything goes through one compressor, with a touch of a small room, so a handful of dice blend instead of piling up.
- **A stalled tab:** impacts the page is late for are skipped, not played all at once.

Measured offline (`renderSound`, 4 seeds per preset):
- Peaks ran from about −13 dBFS for one d20 to −3.6 dBFS for ten dice, with no clipping, before the master was halved. They are about 6 dB lower now.
- Eight d6 sound without a gap from the first landing until about 1.2 s.
- A single d20 still goes quiet while it topples slowly over its last edge, then taps as it lands, as a real die would.

The page has an "Ajustar o som" panel with volume, taps while rolling, rumble and room. The numbers it shows are what to report back. The constants are in `SOUND`, and the event thresholds (`landTick`, `rollTap`, `rockTick`, `rumbleSpeed`) are in `PHYS`.

## What to tune

All parameters are in `PHYS`, `KIND` and `LOOK` at the top of the module, each with a comment.

- **Feel:** `throwSpeed`, `rollDecel`, `KIND[n].keep` (the speed kept per face), `tipGravity` (how fast an edge balance resolves), `floorBounce` and `wallBounce`.
- **Length:** `softEnd`, `brake` and `hardEnd`. Measured with the sweep, a normal roll ends in 1.0–1.8 s (median 1.2–1.6 s); 8 to 10 dice end in 1.3–1.8 s. No roll hit the 3 s cap.
- **Size and legibility:** `dieRadius` (the die size against the tray and the count), `LOOK[n].rest` (the silhouette) and `LOOK[n].digits` (the number size).
- **Rest angle:** `REST_TILT`, ±14°. 0 means every number perfectly upright.

Measured cost with 10 dice: about 0.4 ms of script per frame, and 1.2 ms (at most 2.6 ms) under 4× CPU throttling. The simulation takes under 0.5 ms per roll.

## Why no three.js

It is allowed, but it isn't needed. The look is an orthographic, top-down view of convex polyhedra with flat shading, and a 2D canvas draws exactly that:

- For a convex solid, back-face culling replaces the depth buffer.
- The numbers stay crisp vector text, with no textures.
- three.js has no physics, so we would still write ours, or add cannon-es as a second dependency.
- It would cost about 150–170 kB gzipped for the whole build. That is an estimate, not measured: tree shaking saves little, because `WebGLRenderer` pulls in most of the core.
- This module is about 22 kB gzipped before minification, comments included.

If we ever want lit, glossy 3D dice, three.js would be the way: it is MIT, so compatible with Apache 2.0, with a `NOTICE` entry and a "Créditos" line. It would be pinned exactly in npm and lazy-loaded only with the roll overlay.

## Port plan (Angular)

- **Component:** a standalone, OnPush component, `RollTray`, that replaces today's CSS tumble in `web/src/app/shared/roll-overlay`. It keeps the overlay's contract (`RollAnimator.play(RollShow)`, the polite live region, Esc or a tap to close, the 10 s auto-close) and maps `ShownDie` to `{ sides, result: face, discarded: counts === false, critical, fumble }`.
- **Module:**
  - `DiceTray` becomes a plain TypeScript file (`dice-tray.ts`) with no Angular inside.
  - The component owns the `<canvas>` and calls `create`, `roll` and `destroy`.
  - It never touches signals inside the rAF loop: run it outside the zone, or zoneless, as the app is.
- **Loading:**
  - Lazy-loaded with the overlay (`@defer` or a dynamic `import()`), so none of it is in the initial bundle (budget 500 kB).
  - No new npm dependency.
  - Under the CSP (`self` only), the canvas uses the app's self-hosted Alegreya fonts, not Google Fonts.
- **Theme:**
  - Map the colour tokens to the app's `--mr-*` tokens. `--tray-*` and `--die-*` become new tokens in `styles.scss`, with light and dark values.
  - The app's real tokens are the cooler grey "ficha"; this page uses the warmer paper from the brief.
- **Reduced motion:**
  - Keep the app's rule: with `prefers-reduced-motion`, or without `matchMedia`, nothing animates. The e2e and axe runs set `reducedMotion: 'reduce'`, so they never see the canvas.
  - `setReducedMotion(true)` gives the static result if the overlay should still show the dice.
- **Tests:**
  - `DiceTray.simulate` is pure. A Vitest spec can assert that every seed lands on the requested results, ends under the cap, and is the same for the same seed.
  - Playwright screenshots can use `reducedMotion` (the static layout) or a fixed seed with `finish()`.
- **Server seed:** for the same roll on every screen, the server could send a seed with the roll; the result stays the server's either way.
- **Docs, in the port PR:** `docs/design.md` ("Roll animation"), `docs/architecture.md` if the module gets its own section, and the overlay's spec.

## Known limitations

- **No real rigid-body physics:**
  - The collisions are circles, so polygon corners can overlap a little when two dice touch, and can tuck a few pixels under the rim.
  - A die never stands on an edge for long, never stacks, and never ends tilted against a wall.
- **Hops:** the hops (z) are drawn only as size and shadow.
- **d4:** seen from above, its faces are steep, so its corner numbers keep their place and turn but are not squashed: they still read. The other dice print the numbers exactly as projected.
- **Number layout:** opposite faces add up to the same total, but the arrangement around the die is our own, not a commercial die's.
- **Determinism:** exact on one browser and device; across engines, `Math.sin` and `Math.cos` could differ in the last bit. The result never depends on that.
- **Sound:** synthesized, not recorded. It was tuned by measurement only (levels, gaps, clipping), not by ear: it needs a listen, on a laptop and on real phones.
- **Display changes:** a zoom (a DPR change) with no resize is picked up at the next roll.
