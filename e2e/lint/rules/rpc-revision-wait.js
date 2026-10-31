// @ts-check
// A spec that changes the server over RPC waits until the page shows the change before its next page action.
//
// Several calls in a row need one wait, after the last: only the last call before an action is reported.
//
// The rule looks at `await <helper>(...)` whose answer is an `Encounter` (a numeric `revision` and `combatants`): the helper
// says which revision the server reached. After it, in the same block, in the blocks around it and after an awaited
// callback that it ends (`test.step`), the first thing that is either a page action on a Playwright `Locator`, `Page`,
// `Frame`, `ElementHandle`, keyboard, mouse or touchscreen (click, fill, press, ...), a revision wait
// (`waitForEncounterRevision`) or a page load (`goto`, `openSessionPage`, `openLive`) decides: an action first is an error.
// `goto` and `reload` are not actions: a fresh load reads the state itself.
//
// A callback runs where it is written when a directly awaited call takes it (`await test.step(..)`): the rule reads into it, before and
// after the call. A callback the page calls later (`route`, `exposeFunction`, a `waitForEvent` predicate), one that runs in the browser
// (`evaluate`) or one nobody awaits (`page.on(..)`) is a different moment: the rule reads neither into it nor past it. Callbacks inside `Promise.all`, `expect.poll` or `toPass` are not followed.
//
// A receiver is a Playwright object by where its type is declared (playwright-core), not by its name: an intersection, a union,
// a generic that extends `Locator` count by their members; an app type called `Page` does not.
//
// Limits, on purpose: only Playwright's own action methods count as actions (a helper that clicks inside is not seen);
// a loop's next turn is not followed; a page the rule cannot name is judged by any wait.

const ts = require('typescript');

const ACTIONS = new Set([
  'click',
  'dblclick',
  'fill',
  'press',
  'pressSequentially',
  'check',
  'uncheck',
  'setChecked',
  'selectOption',
  'type',
  'hover',
  'focus',
  'tap',
  'dragTo',
  'setInputFiles',
  'clear',
  'dragAndDrop',
  'dispatchEvent',
  'selectText',
  'down',
  'up',
  'insertText',
  'move',
  'wheel',
]);
// A wait, or a load: a page that loads after the call reads the state itself.
const WAIT_NAMES = new Set(['waitForEncounterRevision', 'openSessionPage', 'openPlayerMap', 'gotoCampaignTab', 'openLive']);
// The Playwright types whose methods are page actions, and the one that names a page.
const ACTION_TYPES = new Set(['Locator', 'Page', 'Frame', 'ElementHandle', 'Keyboard', 'Mouse', 'Touchscreen']);
// Where Playwright's types are declared: `node_modules/playwright-core/types/types.d.ts`.
const PLAYWRIGHT_DECLARATIONS = /[\\/]playwright-core[\\/]/;
const PAGE_TYPES = new Set(['Page']);
const LOCATOR_TYPES = new Set(['Locator']);
// Methods that keep a callback for later (the page calls it) or run it away from the test: its body is not this moment.
const LATER_CALLBACKS = new Set([
  'route',
  'unroute',
  'routeFromHAR',
  'routeWebSocket',
  'exposeFunction',
  'exposeBinding',
  'addInitScript',
  'addLocatorHandler',
  'waitForEvent',
  'waitForRequest',
  'waitForResponse',
  'waitForURL',
  'waitForFunction',
  'evaluate',
  'evaluateAll',
  'evaluateHandle',
  '$eval',
  '$$eval',
  'on',
  'once',
  'addListener',
  'prependListener',
]);
// A read changes nothing on the server: there is no change for the page to show.
const READS = /^(get|list|read)[A-Z]\w*$/;
const LOAD_METHODS = new Set(['goto', 'reload']);

/** @param {any} node */
const isFunction = (node) =>
  node.type === 'ArrowFunctionExpression' || node.type === 'FunctionDeclaration' || node.type === 'FunctionExpression';

/** @param {any} parent */
const listOf = (parent) => {
  if (parent.type === 'BlockStatement' || parent.type === 'Program' || parent.type === 'StaticBlock') return parent.body;
  if (parent.type === 'SwitchCase') return parent.consequent;
  return null;
};

/**
 * Whether a function runs where it is written: an awaited call takes it (`await test.step(..)`), and the method is not one that
 * keeps the callback for later. @param {any} fn
 */
const runsNow = (fn) => {
  const call = fn.parent;
  if (call?.type !== 'CallExpression' || !call.arguments.includes(fn) || call.parent?.type !== 'AwaitExpression') return false;
  const callee = call.callee;
  return !(callee.type === 'MemberExpression' && !callee.computed && LATER_CALLBACKS.has(callee.property.name));
};

// A variable holds a locator (so it belongs to the page it was found on) when it is built by one of these methods;
// anything else, such as `await context.newPage()`, is a page itself.
const LOCATOR_METHODS = /^(getBy\w+|locator|filter|first|last|nth|and|or|frameLocator|contentFrame)$/;

/** @param {any} node */
const isLocatorExpression = (node) => {
  let cur = node;
  while (cur && (cur.type === 'AwaitExpression' || cur.type === 'TSNonNullExpression')) cur = cur.argument ?? cur.expression;
  return (
    cur?.type === 'CallExpression' &&
    cur.callee.type === 'MemberExpression' &&
    !cur.callee.computed &&
    LOCATOR_METHODS.test(cur.callee.property.name)
  );
};

/** @type {import('eslint').Rule.RuleModule} */
module.exports = {
  meta: {
    type: 'problem',
    schema: [],
    docs: { description: 'Wait for the page to show the revision an RPC returned before acting on the page.' },
    messages: {
      wait: '`{{name}}` returns a revision the page may not show yet: await `waitForEncounterRevision(page, ...)` before the next page action, on the page that action uses (`{{page}}`; docs/testing.md).',
    },
  },
  create(context) {
    const sourceCode = context.sourceCode;
    const services = sourceCode.parserServices;
    if (!services?.program || !services.esTreeNodeToTSNodeMap) {
      throw new Error('rpc-revision-wait needs type information (parserOptions.project).');
    }
    const checker = services.program.getTypeChecker();

    /** Whether the awaited value has a numeric `revision`. @param {any} node */
    const hasRevision = (node) => {
      const type = checker.getNonNullableType(checker.getTypeAtLocation(services.esTreeNodeToTSNodeMap.get(node)));
      const prop = type.getProperty('revision');
      return Boolean(prop) && Boolean(type.getProperty('combatants')) && (checker.getTypeOfSymbol(prop).flags & ts.TypeFlags.Number) !== 0;
    };

    /**
     * The types a type is made of: the members of a union or an intersection, the constraint of a generic. Nothing for `undefined`.
     * @param {any} type @returns {any[]}
     */
    const partsOf = (type) => {
      /** @type {any[]} */
      const out = [];
      /** @param {any} t */
      const visit = (t) => {
        const base = t.flags & ts.TypeFlags.TypeParameter ? checker.getBaseConstraintOfType(t) : t;
        if (!base) return;
        if (base.isUnion() || base.isIntersection()) base.types.forEach(visit);
        else out.push(base);
      };
      visit(checker.getNonNullableType(type));
      return out;
    };

    /** Whether a type is one of Playwright's own `names`, by the file that declares it. @param {any} type @param {Set<string>} names */
    const isPlaywrightType = (type, names) =>
      partsOf(type).some((t) => {
        const symbol = t.symbol;
        return Boolean(symbol) && names.has(symbol.name) && (symbol.declarations ?? []).some((/** @type {any} */ d) => PLAYWRIGHT_DECLARATIONS.test(d.getSourceFile().fileName));
      });

    /** @param {any} node @param {Set<string>} names */
    const isPlaywrightObject = (node, names = ACTION_TYPES) => isPlaywrightType(checker.getTypeAtLocation(services.esTreeNodeToTSNodeMap.get(node)), names);

    /** A move is patched into the page with no new revision (CombatState.applyMove): there is nothing to wait for. @param {any} call */
    const isMove = (call) =>
      call.callee.name === 'movePensantus' || call.arguments.some((/** @type {any} */ a) => a.type === 'Literal' && a.value === 'MoveCombatant');

    /** An awaited call of a helper whose answer is an `Encounter`, other than a read, a wait or a move. @param {any} node */
    const isTracked = (node) => {
      const call = node.argument;
      return (
        call.type === 'CallExpression' &&
        call.callee.type === 'Identifier' &&
        !READS.test(call.callee.name) &&
        !WAIT_NAMES.has(call.callee.name) &&
        !isMove(call) &&
        hasRevision(node)
      );
    };

    /** The variable an identifier names, looked up from the node that uses it. @param {any} node @param {string} name */
    const variableOf = (node, name) => {
      for (let scope = sourceCode.getScope(node); scope; scope = scope.upper) {
        const found = scope.set.get(name);
        if (found) return found;
      }
      return null;
    };

    /**
     * The page an expression acts on: the leftmost identifier of `p.getByRole(..)`, `lia.page.locator(..)`, followed
     * through `const card = m.getByTestId(..)`. null when it cannot be told (a parameter, a destructured name).
     * @param {any} node @param {number} [depth]
     * @returns {string | null}
     */
    const pageOf = (node, depth = 0) => {
      let cur = node;
      while (cur) {
        // `lia.page`, `ctx.page`: the page is that member, whatever object holds it.
        if (cur.type === 'MemberExpression' && !cur.computed && cur.property.name === 'page') return sourceCode.getText(cur);
        if (cur.type === 'MemberExpression') cur = cur.object;
        else if (cur.type === 'CallExpression') cur = cur.callee;
        else if (cur.type === 'AwaitExpression') cur = cur.argument;
        else if (cur.type === 'ChainExpression' || cur.type === 'TSNonNullExpression') cur = cur.expression;
        else break;
      }
      if (!cur || cur.type !== 'Identifier') return null;
      if (isPlaywrightObject(cur, PAGE_TYPES)) return cur.name;
      // A locator belongs to the page it was found on, when we can see where it came from.
      const def = variableOf(cur, cur.name)?.defs[0];
      if (isPlaywrightObject(cur, LOCATOR_TYPES) && def?.type === 'Variable' && def.node.id.type === 'Identifier' && def.node.init && depth < 6) {
        return isLocatorExpression(def.node.init) ? pageOf(def.node.init, depth + 1) : null;
      }
      return null;
    };

    /** The pages a wait call waits for: its first argument, a page or an array of pages. @param {any} call */
    const waitPages = (call) => {
      const arg = call.arguments[0];
      if (!arg) return [null];
      return arg.type === 'ArrayExpression' ? arg.elements.map((/** @type {any} */ e) => pageOf(e)) : [pageOf(arg)];
    };

    /** What a statement does is read once, however many calls come before it. @type {WeakMap<object, any[]>} */
    const scans = new WeakMap();

    /**
     * What a statement does, in source order (a callback is entered only where `runsNow` says
     * it runs where it is written): `action` on a page, `wait` for a page
     * (a revision wait or a page load), or a later tracked `rpc`. @param {any} root
     */
    const scan = (root) => {
      const known = scans.get(root);
      if (known) return known;
      /** @type {{ kind: 'action' | 'wait' | 'rpc', page: string | null, pages?: (string | null)[], at: number }[]} */
      const events = [];
      /** @param {any} node */
      const walk = (node) => {
        if (!node || typeof node.type !== 'string') return;
        if (isFunction(node) && !runsNow(node)) return;
        if (node.type === 'AwaitExpression' && isTracked(node)) {
          events.push({ kind: 'rpc', page: null, at: node.range[0] });
        }
        if (node.type === 'CallExpression') {
          const callee = node.callee;
          if (callee.type === 'MemberExpression' && !callee.computed && ACTIONS.has(callee.property.name) && isPlaywrightObject(callee.object)) {
            events.push({ kind: 'action', page: pageOf(callee.object), at: node.range[0] });
          }
          if (callee.type === 'Identifier' && WAIT_NAMES.has(callee.name)) {
            events.push({ kind: 'wait', page: null, pages: waitPages(node), at: node.range[0] });
          } else if (callee.type === 'MemberExpression' && !callee.computed && LOAD_METHODS.has(callee.property.name)) {
            events.push({ kind: 'wait', page: null, pages: [pageOf(callee.object)], at: node.range[0] });
          }
        }
        const keys = sourceCode.visitorKeys[node.type] ?? [];
        for (const key of keys) {
          const child = node[key];
          if (Array.isArray(child)) child.forEach(walk);
          else walk(child);
        }
      };
      walk(root);
      const sorted = events.sort((a, b) => a.at - b.at);
      scans.set(root, sorted);
      return sorted;
    };

    return {
      /** @param {any} node */
      AwaitExpression(node) {
        if (!isTracked(node)) return;
        const call = node.argument;
        // The pages the page waited for so far: a wait on one page does not clear an action on another.
        /** @type {Set<string | null>} */
        const waited = new Set();
        // `await waitForEncounterRevision(page, await someRPC(...))`: the call is waited on in the same breath.
        for (let up = node.parent; up; up = up.parent) {
          if (up.type === 'CallExpression' && up.callee.type === 'Identifier' && WAIT_NAMES.has(up.callee.name)) {
            waitPages(up).forEach((page) => waited.add(page));
            break;
          }
          if (isFunction(up)) break;
        }
        /** @type {any} */
        let cur = node;
        while (cur.parent) {
          const parent = cur.parent;
          // The call ends a callback's body (`test.step(..)`): what follows the callback is judged too.
          if (isFunction(parent)) {
            // Only a callback that runs now, so that an event handler (`page.on(..)`) or a route handler is not read as running before what follows.
            if (!runsNow(parent)) break;
            cur = parent.parent;
            continue;
          }
          const list = listOf(parent);
          if (list) {
            const at = list.indexOf(cur);
            for (const next of list.slice(at + 1)) {
              for (const event of scan(next)) {
                // A later call that changes the server takes over: the wait it needs covers this one too.
                if (event.kind === 'rpc') return;
                if (event.kind === 'wait') (event.pages ?? []).forEach((page) => waited.add(page));
                // An action on a page the rule cannot name is judged by any wait.
                else if (!waited.has(event.page) && !(event.page === null && waited.size > 0)) {
                  context.report({ node: call, messageId: 'wait', data: { name: call.callee.name, page: event.page ?? 'the page' } });
                  return;
                }
              }
            }
          }
          cur = parent;
        }
      },
    };
  },
};
