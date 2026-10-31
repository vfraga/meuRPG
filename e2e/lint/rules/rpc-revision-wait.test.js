// @ts-check
// The rule fails on the bad fixture and passes on the fixed one. Run with `npm run lint:test` from e2e/.
const test = require('node:test');
const assert = require('node:assert/strict');
const path = require('node:path');
const { ESLint } = require('eslint');
const tseslint = require('typescript-eslint');
const rule = require('./rpc-revision-wait');

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
        plugins: { meurpg: { rules: { 'rpc-revision-wait': rule } } },
        rules: { 'meurpg/rpc-revision-wait': 'error' },
      },
    ],
  });
  const [result] = await eslint.lintFiles([path.join(fixtures, file)]);
  return result.messages;
}

test('reports the call a page action follows without a revision wait', async () => {
  const messages = await lint('bad.ts');
  assert.deepEqual(
    messages.map((m) => [m.ruleId, m.line]),
    [
      ['meurpg/rpc-revision-wait', 6],
      ['meurpg/rpc-revision-wait', 12],
      ['meurpg/rpc-revision-wait', 18],
      ['meurpg/rpc-revision-wait', 24],
      ['meurpg/rpc-revision-wait', 46],
      ['meurpg/rpc-revision-wait', 53],
    ],
  );
});

test('passes once the call is followed by a revision wait', async () => {
  assert.deepEqual(await lint('fixed.ts'), []);
});
