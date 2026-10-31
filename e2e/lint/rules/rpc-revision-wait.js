// @ts-check
// A spec that changes the server over RPC waits until the page shows the change before its next page action.
//
// Several calls in a row need one wait, after the last: only the last call before an action is reported.
//
// The rule looks at `await <helper>(...)` whose answer is an `Encounter` (a numeric `revision` and `combatants`): the helper
// says which revision the server reached. After it, in the same block, in the blocks around it and after an awaited
// callback that it ends (`test.step`), the first thing that is either a page action on a Playwright `Locator`, `Page`,
// keyboard or mouse (click, fill, press, ...), a revision wait (`waitForEncounterRevision`) or a page load (`goto`,
// `openSessionPage`, `openLive`) decides: an action first is an error. `goto` and `reload` are not actions: a fresh load reads the state itself.
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
]);
/** An RPC helper that waits on the pages in its last argument. */
const WAITING_RPC = 'passTurnsToAndWait';
// A wait, or a load: a page that loads after the call reads the state itself.
const WAIT_NAMES = new Set(['waitForEncounterRevision', 'openSessionPage', 'openPlayerMap', 'gotoCampaignTab', 'openLive', WAITING_RPC]);
// The receivers whose methods are page actions.
const PLAYWRIGHT_TYPES = new Set(['Locator', 'Page', 'Keyboard', 'Mouse', 'Touchscreen']);
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

    /** Whether a receiver is a Playwright `Locator`, `Page` or the page's keyboard, mouse or touchscreen: `Set.clear()` is no action. @param {any} node */
    const isPlaywrightObject = (node) => {
      const type = checker.getTypeAtLocation(services.esTreeNodeToTSNodeMap.get(node));
      const types = checker.getNonNullableType(type);
      return (types.isUnion() ? types.types : [types]).some((t) => PLAYWRIGHT_TYPES.has(t.symbol?.name ?? ''));
    };

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
      const typeName = checker.getTypeAtLocation(services.esTreeNodeToTSNodeMap.get(cur)).symbol?.name;
      if (typeName === 'Page') return cur.name;
      // A locator belongs to the page it was found on, when we can see where it came from.
      const def = variableOf(cur, cur.name)?.defs[0];
      if (typeName === 'Locator' && def?.type === 'Variable' && def.node.id.type === 'Identifier' && def.node.init && depth < 6) {
        return isLocatorExpression(def.node.init) ? pageOf(def.node.init, depth + 1) : null;
      }
      return null;
    };

    /**
     * The pages a wait call waits for: its first argument (the last for `passTurnsToAndWait`), a page or an array of pages.
     * @param {any} call
     */
    const waitPages = (call) => {
      const arg = call.callee.name === WAITING_RPC ? call.arguments[3] : call.arguments[0];
      if (!arg) return [null];
      return arg.type === 'ArrayExpression' ? arg.elements.map((/** @type {any} */ e) => pageOf(e)) : [pageOf(arg)];
    };

    /**
     * What a statement does, in source order (function bodies are not entered): `action` on a page, `wait` for a page
     * (a revision wait or a page load), or a later tracked `rpc`. @param {any} root
     */
    const scan = (root) => {
      /** @type {{ kind: 'action' | 'wait' | 'rpc', page: string | null, pages?: (string | null)[], at: number }[]} */
      const events = [];
      /** @param {any} node */
      const walk = (node) => {
        if (!node || typeof node.type !== 'string') return;
        if (isFunction(node)) return;
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
      return events.sort((a, b) => a.at - b.at);
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
            // Only an awaited call, so that an event handler (`page.on(..)`) is not read as running before what follows.
            if (parent.parent?.type !== 'CallExpression' || !parent.parent.arguments.includes(parent) || parent.parent.parent?.type !== 'AwaitExpression') break;
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
