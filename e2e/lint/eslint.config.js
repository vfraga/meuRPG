// @ts-check
// ESLint for the Playwright tests in e2e/. Run it with `npm run lint` from e2e/ (after `npm run lint:install`).
//
// The aim is the classic Playwright flake: a missing `await` on a locator action or an `expect`, which lets the
// test move on before the page did. Like in web/, violations that already existed are listed in
// eslint-suppressions.json, so the gate fails only on NEW ones (see CONTRIBUTING: never grow that file).
const path = require('node:path');
const eslint = require('@eslint/js');
const tseslint = require('typescript-eslint');
const playwright = require('eslint-plugin-playwright');
const rpcRevisionWait = require('./rules/rpc-revision-wait');
const exactSharedText = require('./rules/exact-shared-text');

// ESLint's bulk suppressions only cover errors, and the Playwright preset ships many rules as warnings.
// Every rule of the preset is made an error so the ratchet sees it (a warning nobody reads is not a gate).
const playwrightRecommended = playwright.configs['flat/recommended'];
const playwrightRules = Object.fromEntries(
  Object.entries(playwrightRecommended.rules ?? {}).map(([name, setting]) => {
    const [level, ...options] = Array.isArray(setting) ? setting : [setting];
    return [name, level === 'off' || level === 0 ? setting : ['error', ...options]];
  }),
);

module.exports = tseslint.config(
  { ignores: ['node_modules/**', 'lint/**', 'playwright-report/**', 'test-results/**', 'blob-report/**'] },
  {
    files: ['**/*.ts'],
    extends: [eslint.configs.recommended, ...tseslint.configs.recommended, { ...playwrightRecommended, rules: playwrightRules }],
    languageOptions: {
      parserOptions: {
        // Type-aware rules need the project's own tsconfig (this config lives one folder below it).
        project: ['./tsconfig.json'],
        tsconfigRootDir: path.resolve(__dirname, '..'),
      },
    },
    plugins: { meurpg: { rules: { 'rpc-revision-wait': rpcRevisionWait, 'exact-shared-text': exactSharedText } } },
    rules: {
      // A call that changes the server is followed by a wait for the page to show it before the next page action
      // (docs/testing.md, "Waiting for the page to show a change"). No baseline: do not add suppressions for it.
      'meurpg/rpc-revision-wait': 'error',
      // A plain string that is part of a campaign name is `exact: true` on a page (docs/testing.md, "Texts the app bar can repeat").
      // No baseline: do not add suppressions for it.
      'meurpg/exact-shared-text': 'error',
      // Most tests assert through helpers (support.ts, *-support.ts), so "no expect in the test body" is noise;
      // the spacing rule is style, which Prettier owns.
      'playwright/expect-expect': 'off',
      'playwright/consistent-spacing-between-blocks': 'off',
      // No exits: a skipped test hides a flake instead of fixing it, and `test.fixme` is a skip with a nicer name.
      'playwright/no-skipped-test': ['error', { disallowFixme: true }],
      // `test` comes from tests/diagnostics.ts, whose automatic fixture attaches what the pages reported when a test
      // fails (docs/testing.md, "A page that hangs"). No baseline: do not add suppressions for it.
      'no-restricted-imports': [
        'error',
        {
          paths: [
            ...['@playwright/test', 'playwright/test'].map((name) => ({
              name,
              importNames: ['test'],
              message: "Import `test` from './diagnostics': it attaches the pages' diagnostics to a failed test.",
            })),
          ],
        },
      ],
      '@typescript-eslint/no-floating-promises': 'error',
      '@typescript-eslint/await-thenable': 'error',
      '@typescript-eslint/no-misused-promises': 'error',
      '@typescript-eslint/no-explicit-any': 'error',
      '@typescript-eslint/no-unused-vars': [
        'error',
        { argsIgnorePattern: '^_', varsIgnorePattern: '^_', caughtErrorsIgnorePattern: '^_' },
      ],
    },
  },
  {
    // The one place that builds `test` on Playwright's.
    files: ['tests/diagnostics.ts'],
    rules: { 'no-restricted-imports': 'off' },
  },
);
