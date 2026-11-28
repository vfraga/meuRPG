// The workflows that choose what runs state the train rule in one expression (DIFF), copied into each of them because a
// local composite action needs a checkout first: node --test .github/scripts/
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

// The rule (docs/ci.md, "What runs when"): only a pull request that is not a train, from a branch of this repository named
// refactor/train-*, needs the diff.
const DIFF = "${{ github.event_name == 'pull_request' && !(startsWith(github.head_ref, 'refactor/train-') && github.event.pull_request.head.repo.full_name == github.repository) }}";
const SELECTING = ['backend', 'codeql', 'e2e', 'osv-scan', 'web', 'workflows-lint'];

const workflows = path.join(path.dirname(fileURLToPath(import.meta.url)), '..', 'workflows');

test('each selecting workflow has the DIFF expression, exactly, and no other workflow has one', () => {
  for (const name of fs.readdirSync(workflows).filter((f) => f.endsWith('.yml'))) {
    const found = [...fs.readFileSync(path.join(workflows, name), 'utf8').matchAll(/^ {6}DIFF: (.+)$/gm)].map((m) => m[1]);
    const workflow = name.replace(/\.yml$/, '');
    assert.deepEqual(found, SELECTING.includes(workflow) ? [DIFF] : [], name);
  }
});

test('e2e names the folders of its own composite actions, so the shared selector does not run its gate', () => {
  const text = fs.readFileSync(path.join(workflows, 'e2e.yml'), 'utf8');
  const pattern = new RegExp(/pattern: '([^']+)'\n {10}ignore-md/.exec(text)[1].replace(/\\\./g, '.'));
  for (const file of ['.github/actions/build-e2e-images/action.yml', '.github/actions/start-e2e-stack/action.yml', '.github/actions/go-modules/action.yml']) assert.match(file, pattern);
  assert.doesNotMatch('.github/actions/select/action.yml', pattern);
  const paths = [...text.matchAll(/^ {6}- "(\.github\/actions\/[^"]+)"$/gm)].map((m) => m[1]);
  assert.deepEqual(paths, ['.github/actions/build-e2e-images/**', '.github/actions/start-e2e-stack/**', '.github/actions/go-modules/**']);
});

// Evaluates a workflow expression of the one shape DIFF has: ==, &&, !, parentheses, startsWith, string literals and github.* paths.
const evaluate = (expression, github) => {
  const js = expression.replace(/^\$\{\{\s*|\s*\}\}$/g, '').replace(/==/g, '===');
  return new Function('github', 'startsWith', `return ${js};`)(github, (s, prefix) => String(s).startsWith(prefix));
};

test('DIFF is true only for a pull request that is not a train of this repository', () => {
  const repo = 'o/meuRPG';
  const pull = (headRef, headRepo) => ({ event_name: 'pull_request', head_ref: headRef, repository: repo, event: { pull_request: { head: { repo: { full_name: headRepo } } } } });
  const cases = [
    ['a push to main', { event_name: 'push', head_ref: '', repository: repo, event: {} }, false],
    ['the nightly', { event_name: 'schedule', head_ref: '', repository: repo, event: {} }, false],
    ['a dispatch', { event_name: 'workflow_dispatch', head_ref: '', repository: repo, event: {} }, false],
    ['a train from this repository', pull('refactor/train-70', repo), false],
    ['a branch named like a train, from a fork', pull('refactor/train-70', 'someone/meuRPG'), true],
    ['an ordinary pull request', pull('feat/x', repo), true],
  ];
  for (const [name, github, want] of cases) assert.equal(evaluate(DIFF, github), want, name);
});
