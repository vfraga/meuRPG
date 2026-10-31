// @ts-check
// A plain-string locator on a page matches any text that contains the string, and the page can hold another test's text.
//
// The app bar carries `Ao vivo: sessão 1 de <campaign name>` and the live notice `A sessão 1 de <campaign name> começou.`, for
// whichever session of the signed-in account is open. A spec that names its campaign with `${Date.now()}` runs at the same time
// as the others, with the same account, so `page.getByLabel('Nome')` can also match the link of a session whose campaign is
// called "Nomes e legenda". The match is strict mode's error (more than one element), and only when the two tests share a shard.
//
// The rule reports `getByLabel('x')` and `getByRole('link', { name: 'x' })` on a Playwright `Page` or `Frame` (the two ways to find
// an element by its `aria-label`) when 'x' is part of a campaign name some spec uses and the call has no `exact`. A locator scoped
// to an element (`dialog.getByLabel(..)`) cannot see the bar, so it is not reported.
//
// A campaign name is a template literal that ends in `${Date.now()}`, or a string given to a helper that builds one
// (`atCave(browser, 'Nomes e legenda', ..)`, a function that writes `${name} ${Date.now()}`).
//
// Limits, on purpose: `getByText` is not reported (the live notice carries the name as text, but `exact` there would make an
// absence check such as `toHaveCount(0)` weaker); a fixed name is the one test's own; a name built at run time is not read.
const fs = require('node:fs');
const path = require('node:path');
const ts = require('typescript');

const PLAYWRIGHT_DECLARATIONS = /[\\/]playwright-core[\\/]/;
const PAGE_TYPES = new Set(['Page', 'Frame']);
/** The method whose first argument is the label to find; `getByRole` gives the text as its `name` option. */
const TEXT_METHODS = new Set(['getByLabel']);
/** The roles of the elements that carry a campaign name: the live link and the notice's link. */
const NAMED_ROLES = new Set(['link']);
const NAME_IN_SPEC = /`([^`$]*)\$\{Date\.now\(\)\}/g;
/** A function that writes `${name} ${Date.now()}`: the nearest declaration before it. */
const NAMING_HELPER = /\$\{\w+\} \$\{Date\.now\(\)\}/g;
const DECLARATION = /(?:async\s+)?function\s+(\w+)\s*(?:<[^>]*>)?\(|(?:const|let)\s+(\w+)\s*=\s*(?:async\s*)?\(/g;
const STRING_ARGUMENT = /(['"])((?:(?!\1)[^\n])*)\1/g;

/** The names, lower-cased, of the campaigns the specs in `dir` create. @type {Map<string, string[]>} */
const cache = new Map();
/** @param {string} dir */
function campaignNames(dir) {
  let names = cache.get(dir);
  if (!names) {
    const sources = fs
      .readdirSync(dir)
      .filter((file) => file.endsWith('.ts'))
      .map((file) => fs.readFileSync(path.join(dir, file), 'utf8'));
    const found = new Set();
    const helpers = new Set();
    for (const source of sources) {
      for (const m of source.matchAll(NAME_IN_SPEC)) found.add(m[1].trim().toLowerCase());
      for (const m of source.matchAll(NAMING_HELPER)) {
        const declared = [...source.slice(0, m.index).matchAll(DECLARATION)].at(-1);
        if (declared) helpers.add(declared[1] ?? declared[2]);
      }
    }
    for (const source of sources) {
      for (const helper of helpers) {
        for (const call of source.matchAll(new RegExp(`\\b${helper}\\(([^;]*?)\\)\\s*[;,)]`, 'gs'))) {
          for (const arg of call[1].matchAll(STRING_ARGUMENT)) found.add(arg[2].trim().toLowerCase());
        }
      }
    }
    found.delete('');
    names = [...found];
    cache.set(dir, names);
  }
  return names;
}

/** The string a node holds, when it is a plain one. @param {any} node */
const plainText = (node) => {
  if (node?.type === 'Literal' && typeof node.value === 'string') return node.value;
  if (node?.type === 'TemplateLiteral' && node.expressions.length === 0) return node.quasis[0].value.cooked;
  return undefined;
};

/** Whether an object expression has the key. @param {any} node @param {string} key */
const hasKey = (node, key) =>
  node?.type === 'ObjectExpression' &&
  node.properties.some((/** @type {any} */ p) => p.type === 'Property' && !p.computed && (p.key.name === key || p.key.value === key));

/** @type {import('eslint').Rule.RuleModule} */
module.exports = {
  meta: {
    type: 'problem',
    schema: [{ type: 'object', properties: { testsDir: { type: 'string' } }, additionalProperties: false }],
    messages: {
      shared:
        "'{{text}}' is part of the campaign name '{{name}}': the app bar and the live notice of an open session carry that name, so this {{method}} can match them too. Add `exact: true`.",
    },
  },
  create(context) {
    const services = context.sourceCode.parserServices;
    if (!services?.program || !services.esTreeNodeToTSNodeMap) {
      throw new Error('exact-shared-text needs type information (parserOptions.project).');
    }
    const checker = services.program.getTypeChecker();
    const options = /** @type {{ testsDir?: string }} */ (context.options[0] ?? {});
    const dir = options.testsDir ? path.resolve(context.cwd, options.testsDir) : path.resolve(__dirname, '..', '..', 'tests');

    /** Whether a node is a Playwright Page or Frame, by the file that declares its type. @param {any} node */
    const isPage = (node) => {
      /** @type {any[]} */
      const parts = [];
      /** @param {any} t */
      const visit = (t) => {
        const base = t.flags & ts.TypeFlags.TypeParameter ? checker.getBaseConstraintOfType(t) : t;
        if (!base) return;
        if (base.isUnion() || base.isIntersection()) base.types.forEach(visit);
        else parts.push(base);
      };
      visit(checker.getNonNullableType(checker.getTypeAtLocation(services.esTreeNodeToTSNodeMap.get(node))));
      return parts.some(
        (t) =>
          t.symbol &&
          PAGE_TYPES.has(t.symbol.name) &&
          (t.symbol.declarations ?? []).some((/** @type {any} */ d) => PLAYWRIGHT_DECLARATIONS.test(d.getSourceFile().fileName)),
      );
    };

    return {
      /** @param {any} node */
      CallExpression(node) {
        const callee = node.callee;
        if (callee.type !== 'MemberExpression' || callee.computed) return;
        const method = callee.property.name;
        /** @type {string | undefined} */
        let text;
        if (TEXT_METHODS.has(method)) {
          text = plainText(node.arguments[0]);
          if (hasKey(node.arguments[1], 'exact')) return;
        } else if (method === 'getByRole' && NAMED_ROLES.has(plainText(node.arguments[0]) ?? '')) {
          const given = node.arguments[1];
          if (hasKey(given, 'exact')) return;
          const name = given?.type === 'ObjectExpression' ? given.properties.find((/** @type {any} */ p) => p.type === 'Property' && p.key.name === 'name') : undefined;
          text = plainText(name?.value);
        }
        if (!text || !isPage(callee.object)) return;
        const wanted = text.toLowerCase();
        const name = campaignNames(dir).find((n) => n.includes(wanted));
        if (name) context.report({ node, messageId: 'shared', data: { text, name, method } });
      },
    };
  },
};
