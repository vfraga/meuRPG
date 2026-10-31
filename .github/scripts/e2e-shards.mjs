#!/usr/bin/env node
// Splits the Playwright tests across the e2e shards by measured time (docs/ci.md, "End to end").
//
//   e2e-shards.mjs list <index>/<count> [--fresh-shard=<n>] [playwright args]   the --test-list file of one shard, on stdout
//   e2e-shards.mjs check <count> [--fresh-shard=<n>] [playwright args]          fails unless every test is in exactly one of the shards
//   e2e-shards.mjs setup <true|false> <started> [playwright args]                the setup file of a shard (seconds since <started>, workers), on stdout
//   e2e-shards.mjs timings <file.json>...                                       the timing file, from the files a shard uploads, on stdout
//
// The playwright args (a --grep, a --grep-invert) select the tests to split, as they select the tests to run.
// The timing file is e2e/timings.json: one duration in seconds per test, keyed by the line of the test list, and the
// load the fresh shard's slower setup stands for (FRESH_KEY), in the unit of the durations.
// A `test.describe.configure({ mode: 'serial' })` group is placed whole in one shard, as one unit of the summed time.
import { execFile } from 'node:child_process';
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { promisify } from 'node:util';

import { eachTest } from './flake.mjs';

const run = promisify(execFile);

const SEPARATOR = ' › ';

// The entry of e2e/timings.json for the shard that migrates from an empty database and checks the devidp image.
export const FRESH_KEY = '(extra setup of the fresh shard)';

class Usage extends Error {}

// The key of a test: the line Playwright's --test-list reads, which names the project, the file and the titles.
export function testKey(project, file, titles) {
  return [`[${project}]`, file, ...titles].join(SEPARATOR);
}

// The tests of a run's JSON report, in file order.
export function testsOf(report) {
  const found = [];
  eachTest(report, (test, spec, titles) => found.push({ key: testKey(test.projectName, spec.file, [...titles, spec.title]), results: test.results ?? [] }));
  return found;
}

function median(values) {
  const sorted = [...values].sort((a, b) => a - b);
  return sorted.length ? sorted[Math.floor(sorted.length / 2)] : 1;
}

// Puts every test in exactly one of `count` shards. `tests` are {key, group}: the tests of one group (a serial
// describe) go together, as one unit that weighs their sum. The longest known units go first, each to the shard with
// the least time so far; a unit with a test the timing file does not know comes after them, that test weighing the
// median, so a new test runs without anyone updating the file. The setup project runs as a dependency of every shard,
// so it weighs nothing here. `startup[i]` is the time shard i spends before its first test. Returns the keys of each
// shard, in the order given.
export function split(tests, timings, count, startup = []) {
  const isSetup = (test) => test.key.startsWith('[setup]');
  const guess = median(tests.filter((test) => !isSetup(test) && test.key in timings).map((test) => timings[test.key]));
  const units = new Map();
  for (const test of tests.filter((test) => !isSetup(test))) {
    const id = test.group ?? test.key;
    const unit = units.get(id) ?? { keys: [], seconds: 0, known: true };
    unit.keys.push(test.key);
    unit.seconds += timings[test.key] ?? guess;
    unit.known &&= test.key in timings;
    units.set(id, unit);
  }
  const load = Array.from({ length: count }, (_, i) => startup[i] ?? 0);
  const shardOf = new Map();
  const place = (unit) => {
    let least = 0;
    for (let i = 1; i < count; i++) if (load[i] < load[least]) least = i;
    load[least] += unit.seconds;
    for (const key of unit.keys) shardOf.set(key, least);
  };
  // The sort is stable and the ties keep the order of `tests`, so every shard computes the same split.
  const all = [...units.values()];
  for (const unit of all.filter((unit) => unit.known).sort((a, b) => b.seconds - a.seconds)) place(unit);
  for (const unit of all.filter((unit) => !unit.known)) place(unit);
  for (const test of tests.filter(isSetup)) shardOf.set(test.key, 0);
  const shards = Array.from({ length: count }, () => []);
  for (const test of tests) shards[shardOf.get(test.key)].push(test.key);
  return shards;
}

// What the fresh shard's setup costs beyond the others', in the unit of the test durations: those are summed over a
// shard's workers, and the setup is wall-clock time in which every worker would have run tests, so the extra seconds
// are multiplied by the workers. The files are {fresh, seconds, workers} of every shard of a run.
export function freshOverhead(setups) {
  const fresh = setups.filter((setup) => setup.fresh);
  const others = setups.filter((setup) => !setup.fresh).map((setup) => setup.seconds);
  if (fresh.length === 0 || others.length === 0) return undefined;
  const slowest = fresh.reduce((a, b) => (b.seconds > a.seconds ? b : a));
  return Math.max(0, Math.round((slowest.seconds - median(others)) * (slowest.workers ?? 1)));
}

// Durations in seconds of the passed tests of Playwright JSON reports; a test that several reports hold keeps its longest.
// The entry of the fresh shard comes from the {fresh, seconds} files, when there are any.
export function timingsOf(files) {
  const reports = files.filter((file) => 'suites' in file);
  const seconds = {};
  for (const report of reports) {
    for (const { key, results } of testsOf(report)) {
      const passed = results.filter((result) => result.status === 'passed');
      if (passed.length === 0) continue;
      const longest = Math.max(...passed.map((result) => result.duration)) / 1000;
      seconds[key] = Math.max(seconds[key] ?? 0, Math.round(longest * 10) / 10);
    }
  }
  const overhead = freshOverhead(files.filter((file) => 'seconds' in file));
  if (overhead !== undefined) seconds[FRESH_KEY] = overhead;
  return Object.fromEntries(Object.entries(seconds).sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0)));
}

const e2eDir = join(dirname(fileURLToPath(import.meta.url)), '..', '..', 'e2e');
const reporter = join(dirname(fileURLToPath(import.meta.url)), 'e2e-list-reporter.mjs');

// The tests `playwright test --list` selects, as {key, group}, and the workers a run would use.
async function listTests(args, extra = []) {
  try {
    const { stdout } = await run('npx', ['playwright', 'test', '--list', `--reporter=${reporter}`, ...args, ...extra], { cwd: e2eDir, env: process.env, encoding: 'utf8', maxBuffer: 1 << 28 });
    const listed = JSON.parse(stdout);
    return { workers: listed.workers, tests: listed.tests.map((test) => ({ key: testKey(test.project, test.file, test.titles), group: test.group })) };
  } catch (error) {
    throw new Error(`playwright --list failed (${error.code ?? error.name}):\n${error.stderr ?? error.message}`);
  }
}

function readTimings() {
  return JSON.parse(readFileSync(join(e2eDir, 'timings.json'), 'utf8'));
}

function parseCount(text) {
  if (!/^[1-9]\d*$/.test(text ?? '')) throw new Usage(`the shard count must be a positive number, got ${text}`);
  return Number(text);
}

function parseShard(text) {
  const match = /^(\d+)\/(\d+)$/.exec(text ?? '');
  if (!match || Number(match[1]) < 1 || Number(match[1]) > Number(match[2])) throw new Usage(`the shard must look like 1/3, got ${text}`);
  return { index: Number(match[1]), count: Number(match[2]) };
}

// `--fresh-shard=<n>` among the playwright args names the shard that migrates from zero.
function takeFresh(args, count) {
  const flags = args.filter((arg) => arg.startsWith('--fresh-shard'));
  if (flags.length === 0) return { fresh: undefined, args };
  const match = flags.length === 1 ? /^--fresh-shard=(\d+)$/.exec(flags[0]) : null;
  if (!match || Number(match[1]) < 1 || Number(match[1]) > count) throw new Usage(`--fresh-shard must be given once, between 1 and ${count}: ${flags.join(' ')}`);
  return { fresh: Number(match[1]), args: args.filter((arg) => !arg.startsWith('--fresh-shard')) };
}

async function shardsOf(args, count) {
  const { fresh, args: playwrightArgs } = takeFresh(args, count);
  const { tests } = await listTests(playwrightArgs);
  const timings = readTimings();
  const missing = tests.filter((test) => !test.key.startsWith('[setup]') && !(test.key in timings)).length;
  if (fresh && !(FRESH_KEY in timings)) console.error(`::notice::e2e/timings.json has no "${FRESH_KEY}": the fresh shard gets no less test time. Refresh the file (docs/ci.md).`);
  const startup = fresh ? Array.from({ length: count }, (_, i) => (i === fresh - 1 ? (timings[FRESH_KEY] ?? 0) : 0)) : [];
  return { tests, keys: tests.map((test) => test.key), args: playwrightArgs, missing, shards: split(tests, timings, count, startup) };
}

async function main(argv) {
  const [command, ...rest] = argv;
  if (command === 'list') {
    const { index, count } = parseShard(rest[0]);
    const { keys, missing, shards } = await shardsOf(rest.slice(1), count);
    if (missing > 0) console.error(`::notice::${missing} of ${keys.length} tests are not in e2e/timings.json; they ran in the shard with the least time. Refresh the file (docs/ci.md).`);
    process.stdout.write(shards[index - 1].join('\n') + '\n');
  } else if (command === 'check') {
    const count = parseCount(rest[0]);
    const { keys, tests, args, shards } = await shardsOf(rest.slice(1), count);
    // Playwright, not this script, decides what a list selects, so each list is read back through `--test-list`.
    // Without --no-deps every shard would list the setup project, which runs ahead of all of them.
    const dir = mkdtempSync(join(tmpdir(), 'e2e-shards-'));
    let readBack;
    try {
      readBack = await Promise.all(
        shards.map((shard, i) => {
          const file = join(dir, `${i + 1}.txt`);
          writeFileSync(file, shard.join('\n') + '\n');
          return listTests(args, ['--no-deps', '--test-list', file]).then((listed) => listed.tests);
        }),
      );
    } finally {
      rmSync(dir, { recursive: true });
    }
    const seen = new Map();
    readBack.forEach((tests, i) => {
      for (const { key } of tests) if (!key.startsWith('[setup]')) seen.set(key, [...(seen.get(key) ?? []), i + 1]);
      // The setup project is never listed back: it runs ahead of every shard whatever the list says.
      for (const key of shards[i]) if (key.startsWith('[setup]')) seen.set(key, [...(seen.get(key) ?? []), i + 1]);
    });
    const problems = keys.filter((key) => (seen.get(key) ?? []).length !== 1).map((key) => `${key}: in shards [${(seen.get(key) ?? []).join(', ')}]`);
    // A serial group runs whole in one shard.
    const groups = new Map();
    for (const { key, group } of tests) if (group) groups.set(group, new Set([...(groups.get(group) ?? []), ...(seen.get(key) ?? [])]));
    for (const [group, inShards] of groups) if (inShards.size > 1) problems.push(`${group}: a serial group in shards [${[...inShards].join(', ')}]`);
    const unknown = [...seen.keys()].filter((key) => !keys.includes(key));
    if (problems.length > 0 || unknown.length > 0) {
      console.error(`::error::The ${count} lists do not hold every test exactly once.`);
      for (const line of [...problems, ...unknown.map((key) => `${key}: not a listed test`)]) console.error(line);
      process.exit(1);
    }
    console.log(`${keys.length} tests, each in exactly one of ${count} lists.`);
  } else if (command === 'setup') {
    // The file a shard uploads next to its timing: how long its setup took (started is the epoch second it began).
    const seconds = Math.round(Date.now() / 1000) - Number(rest[1]);
    const { workers } = await listTests(rest.slice(2));
    process.stdout.write(JSON.stringify({ fresh: rest[0] === 'true', seconds, workers }) + '\n');
  } else if (command === 'timings') {
    process.stdout.write(JSON.stringify(timingsOf(rest.map((file) => JSON.parse(readFileSync(file, 'utf8')))), null, 1) + '\n');
  } else {
    throw new Usage('usage: e2e-shards.mjs list <index>/<count> [args] | check <count> [args] | timings <file.json>...');
  }
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  main(process.argv.slice(2)).catch((error) => {
    console.error(error instanceof Usage ? `::error::${error.message}\nusage: e2e-shards.mjs list <index>/<count> [--fresh-shard=<n>] [args] | check <count> [--fresh-shard=<n>] [args] | timings <file.json>...` : error.stack);
    process.exit(error instanceof Usage ? 2 : 1);
  });
}
