// @ts-check
// The rule fails on the bad fixture and passes on the fixed one. Run with `npm run lint:test` from e2e/.
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
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
  // The lines the fixture marks with `// error`: the call whose answer the page may not show yet.
  const marked = fs
    .readFileSync(path.join(fixtures, 'bad.ts'), 'utf8')
    .split('\n')
    .flatMap((line, i) => (line.endsWith('// error') ? [i + 1] : []));
  assert.equal(marked.length, 24);
  assert.deepEqual(
    messages.map((m) => [m.ruleId, m.line]),
    marked.map((line) => ['meurpg/rpc-revision-wait', line]),
  );
});

test('passes once the call is followed by a revision wait', async () => {
  assert.deepEqual(await lint('fixed.ts'), []);
});
