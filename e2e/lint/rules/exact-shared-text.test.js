// @ts-check
// The rule fails on the bad fixture and passes on the fixed one. Run with `npm run lint:test` from e2e/.
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { ESLint } = require('eslint');
const tseslint = require('typescript-eslint');
const rule = require('./exact-shared-text');

const fixtures = path.resolve(__dirname, '..', 'fixtures');

/** @param {string} file */
async function lint(file) {
  const eslint = new ESLint({
    cwd: fixtures,
    overrideConfigFile: true,
    overrideConfig: [
      {
        files: ['**/*.ts'],
        languageOptions: {
          parser: tseslint.parser,
          parserOptions: { project: ['./tsconfig.json'], tsconfigRootDir: fixtures },
        },
        plugins: { meurpg: { rules: { 'exact-shared-text': rule } } },
        rules: { 'meurpg/exact-shared-text': ['error', { testsDir: 'campaign-names' }] },
      },
    ],
  });
  const [result] = await eslint.lintFiles([path.join(fixtures, file)]);
  return result.messages;
}

test('reports a plain string that is part of a campaign name', async () => {
  const marked = fs
    .readFileSync(path.join(fixtures, 'exact-bad.ts'), 'utf8')
    .split('\n')
    .flatMap((line, i) => (line.endsWith('// error') ? [i + 1] : []));
  assert.equal(marked.length, 5);
  const messages = await lint('exact-bad.ts');
  assert.deepEqual(
    messages.map((m) => [m.ruleId, m.line]),
    marked.map((line) => ['meurpg/exact-shared-text', line]),
  );
});

test('passes with `exact`, on a scoped locator, on other text and on other roles', async () => {
  assert.deepEqual(await lint('exact-fixed.ts'), []);
});
