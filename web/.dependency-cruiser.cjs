// dependency-cruiser config: the layer rules of the web app (see docs/architecture.md#web-app-web and
// CONTRIBUTING "Web and e2e linters"). Run it with `npm run depcruise`.
//
// A ratchet, like ESLint's: violations recorded in .dependency-cruiser-known-violations.json are ignored
// (`--ignore-known`), so only a NEW violation fails. The 100 that existed when the rules came in were all fixed, so the
// file is empty. Never add to it: fix the import (or move the shared piece down to `core/` or `shared/`).
// The layers, from the bottom up:
//   core   (clients, state, display helpers; imports neither pages nor shell. It should not import shared either,
//          but that is not enforced yet: the 19 imports left are not part of the rules)
//   shared (components used by more than one page; imports core, never a page)
//   pages  (one folder per route; imports core and shared)
//   shell  (the app frame: navigation bar, notices)
// The generated Connect code (src/gen) is outside the layers.

/** @type {import('dependency-cruiser').IConfiguration} */
module.exports = {
  forbidden: [
    {
      name: 'no-circular',
      severity: 'error',
      comment:
        'A cycle makes the files impossible to load, test or move one at a time. Extract what both sides need into a third file.',
      from: {},
      to: { circular: true },
    },
    {
      name: 'core-not-to-pages',
      severity: 'error',
      comment:
        'core/ is the layer pages stand on. A model or helper a page owns but core needs belongs in core/ (or shared/).',
      from: { path: '^src/app/core/' },
      to: { path: '^src/app/pages/' },
    },
    {
      name: 'core-not-to-shell',
      severity: 'error',
      comment: 'The shell is the app frame, above core: core never reaches up to it.',
      from: { path: '^src/app/core/' },
      to: { path: '^src/app/shell/' },
    },
    {
      name: 'shared-not-to-pages',
      severity: 'error',
      comment:
        'shared/ is used by several pages, so it cannot depend on one of them. Move the piece it needs into shared/ or core/.',
      from: { path: '^src/app/shared/' },
      to: { path: '^src/app/pages/' },
    },
    {
      name: 'page-not-to-other-page',
      severity: 'error',
      comment:
        'A page is a direct child folder of src/app/pages/ and may import its own sub-folders, core/ and shared/, but not the files of another page: move what two pages share into shared/ or core/. There is no exception today: no page embeds another page as a whole (a route loads its own page only), and every cross-page import found when the rule came in was a shared piece (a sheet frame, a live-session source, a types file) or a panel that should be shared. If a page ever has to embed another, name it here with a `pathNot` entry and say why.',
      from: { path: '^src/app/pages/([^/]+)/' },
      to: {
        path: '^src/app/pages/([^/]+)/',
        // `$1` is the page of the importing file: the same page is fine.
        pathNot: ['^src/app/pages/$1/'],
      },
    },
  ],
  options: {
    tsConfig: { fileName: 'tsconfig.app.json' },
    // A `import type` is still a coupling (the 9-file knot ran through a types file).
    tsPreCompilationDeps: true,
    doNotFollow: { path: 'node_modules' },
    includeOnly: '^src/',
    exclude: { path: ['\\.spec\\.ts$', '^src/gen/'] },
  },
};
