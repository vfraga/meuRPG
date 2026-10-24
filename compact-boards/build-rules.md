# Building the compact screens: rules for every unit

Every unit of the compact screens follows these rules. Your own brief says what your unit builds and which files are yours.

## The boards are the spec

- The approved boards are in this folder (`compact-boards/` on the fork's `pr-screenshots` branch): a PNG per board and its `.dc.html` source, which has the exact Portuguese of every label. `proposal.md` explains each change, the keyboard order, the accessibility and the e2e impact. Read the boards your brief names, and the proposal sections it names, before you write code.
- Build only what an approved board shows. A board that is not in this folder yet is not approved: build the parts that have one, and say which parts wait. If the boards and the proposal disagree, the board wins; if a board leaves a state out, follow the proposal and say so.
- Samuel may comment on the boards later. His suggestions come as a later round, not in this one.

## How a screen is made here

- `docs/design/` (the "Ficha de papel" system: `README.md`, `tokens.md`, `components.md`, `session.md`, `combat.md`), the `--mr-*` tokens in `web/src/styles.scss` and the shared classes in `web/src/styles/_ui.scss`. No hand-written colour. Material is themed through `mat.theme` and the override mixins, never `::ng-deep`.
- One filled button per screen. Targets of at least 44 px (48 px for a phone's filled button). Icons are Material Symbols from the set the app already ships, always beside a word or with an `aria-label`.
- Nothing is stored on the device: a fold's or a tab's state lives in memory, or in the URL when the proposal says so (`?vista=mapa`, the campaign tab routes). No Web Storage.
- Inactive tab panels stay mounted with `hidden`, so a draft in the composer and the place in a feed survive a tab change. Rail and in-page tabs follow the ARIA tabs pattern, a single tab stop with arrow keys; route tabs are links in a `nav` with `aria-current="page"`.
- Sticky bars use `scroll-padding-top` (or the bottom equivalent) so focus is never hidden under them (WCAG 2.2, 2.4.11).
- Keep the initial bundle under the 500 kB gate: nothing new in `app.ts`, `app.config.ts`, `app.routes.ts` or `shell/` unless it is small and needed by the first screen.
- The web lint limits hold with no exception: `complexity` 15, `max-depth` 4, no app file over 400 lines of code, no magic numbers, the dependency-cruiser layers. A new shared component goes in `shared/`, never imported across pages.

## Tests

- Run the specs of every file you touch, `npm run lint`, `npm run format:check`, `npm run depcruise` and `npm run build` in `web/`, and `npm run typecheck` and `npm run lint` in `e2e/`. CI runs the rest and is the proof.
- Moving a panel behind a tab breaks the e2e specs that reach it: update each one to open the tab first (a helper per tab set, in `e2e/tests/support.ts` or next to it). Keep one button per accessible name per screen: the specs use `getByRole` in strict mode.
- `a11y.spec.ts` visits each new tab, fold and state, in light and dark, at desktop and phone.
- No `setTimeout` waits in specs (`nextTurn()` / `settle(fixture)`).

## Screenshots

- At 390, 768 and 1280 px, light and dark, in every state the board shows (and the empty, loading and error ones).
- If your VM can run the stack (`make up`), screenshot the real app. If it cannot, render the real components with fake data in a throwaway harness (never committed) and label the captures as harness captures.
- Push them to the fork's `pr-screenshots` branch, in the folder your brief names, and list their raw URLs.
- Look at each one up close (2x) against its board: alignment, same-size controls, icon beside its words, nothing clipped, nothing floating over the map. List every difference from the board you know of, honestly, in your "ready:".

## Docs and git

- In the same branch: the `docs/design/` pages your unit changes (`session.md`, `combat.md`, `components.md`), written fresh (state the current design only, no "now" or "no longer"); a story's acceptance criteria in English and Portuguese (`docs/product/stories.md`, `docs/pt-BR/produto/historias.md`) when behaviour a person sees changes.
- Branch from the fork's `main`, named in your brief. Conventional Commits in English. Never force-push.
- Touch only the files your brief gives you. If you must change a file another unit owns, say so before you do.
- When done, say "ready: <branch>" with the screenshot URLs and the known differences. Then stop and wait: the next item comes from the orchestrator.
