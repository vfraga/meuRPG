import assert from 'node:assert/strict';
import { test } from 'node:test';

import { groupedTests } from './e2e-list-reporter.mjs';
import { FRESH_KEY, freshOverhead, split as splitTests, testKey, testsOf, timingsOf } from './e2e-shards.mjs';

// Invented tests: a file, a title and how long it takes.
const key = (file, title) => testKey('chrome', file, [title]);
const setup = testKey('setup', 'auth.setup.ts', ['sign in']);

// Keys, or {key, group} for a test of a serial group; the shards come back as keys.
const split = (tests, timings, count, startup) => splitTests(tests.map((test) => (typeof test === 'string' ? { key: test } : test)), timings, count, startup);

function suite(...specs) {
  return { suites: specs.map(([file, title, status, duration]) => ({ title: file, specs: [{ file, title, tests: [{ projectName: 'chrome', results: [{ status, duration }] }] }] })) };
}

test('every test lands in exactly one shard, the setup project included', () => {
  const keys = [setup, ...Array.from({ length: 20 }, (_, i) => key(`f${i}.spec.ts`, `test ${i}`))];
  const timings = Object.fromEntries(keys.slice(1).map((k, i) => [k, i + 1]));
  for (const count of [1, 3, 6]) {
    const shards = split(keys, timings, count);
    assert.equal(shards.length, count);
    assert.deepEqual(shards.flat().sort(), [...keys].sort());
  }
});

test('the shards are balanced by time, not by count', () => {
  // One test as long as ten short ones: a split by count would put five tests in each shard.
  const long = key('slow.spec.ts', 'slow');
  const short = Array.from({ length: 10 }, (_, i) => key('fast.spec.ts', `fast ${i}`));
  const timings = { [long]: 10, ...Object.fromEntries(short.map((k) => [k, 1])) };
  const [first, second] = split([long, ...short], timings, 2);
  const total = (shard) => shard.reduce((sum, k) => sum + timings[k], 0);
  assert.equal(total(first), 10);
  assert.equal(total(second), 10);
  assert.deepEqual(first, [long]);
});

test('a test the timing file does not know still runs, in the shard with the least time', () => {
  const heavy = key('a.spec.ts', 'heavy');
  const light = key('a.spec.ts', 'light');
  const fresh = key('b.spec.ts', 'new');
  const [first, second] = split([heavy, light, fresh], { [heavy]: 30, [light]: 10 }, 2);
  assert.deepEqual(first, [heavy]);
  assert.deepEqual(second, [light, fresh]);
});

test('unknown tests spread over the shards instead of piling up in one', () => {
  const keys = Array.from({ length: 6 }, (_, i) => key('new.spec.ts', `new ${i}`));
  const shards = split(keys, {}, 3);
  assert.deepEqual(shards.map((shard) => shard.length), [2, 2, 2]);
});

test('a subset is split on its own: only its tests are listed and they are balanced', () => {
  const all = Array.from({ length: 12 }, (_, i) => key(`f${i}.spec.ts`, `test ${i}`));
  const timings = Object.fromEntries(all.map((k, i) => [k, i % 2 === 0 ? 8 : 1]));
  const slow = all.filter((_, i) => i % 2 === 0);
  const shards = split(slow, timings, 3);
  assert.deepEqual(shards.flat().sort(), [...slow].sort());
  assert.deepEqual(shards.map((shard) => shard.length), [2, 2, 2]);
});

test('the split does not depend on who computes it', () => {
  // A fixed split: a change to the placement rule changes this list, on purpose.
  const names = ['a', 'b', 'c', 'd', 'e', 'f', 'g'];
  const keys = names.map((name) => key('f.spec.ts', name));
  const seconds = { a: 5, b: 4, c: 3, d: 3, e: 2, f: 1 }; // g is not in the timing file
  const timings = Object.fromEntries(Object.entries(seconds).map(([name, s]) => [key('f.spec.ts', name), s]));
  const named = (shards) => shards.map((shard) => shard.map((k) => k.at(-1)));
  // Longest first: a, b, c go to shards 1, 2, 3; d joins c (3), e joins b (4), f joins a (5); g weighs the median (3) and comes last, to the first shard of the tie.
  assert.deepEqual(named(split(keys, timings, 3)), [['a', 'f', 'g'], ['b', 'e'], ['c', 'd']]);
  assert.deepEqual(named(split([...keys].reverse(), timings, 3)).map((shard) => [...shard].sort()), [['a', 'f', 'g'], ['b', 'e'], ['c', 'd']]);
});

test('a serial group lands whole in one shard, as one unit of the summed time', () => {
  const group = ['one', 'two', 'three'].map((title) => ({ key: key('serial.spec.ts', title), group: 'serial.spec.ts › importar' }));
  const others = ['x', 'y', 'z'].map((title) => key('other.spec.ts', title));
  const timings = { ...Object.fromEntries(group.map((test) => [test.key, 4])), ...Object.fromEntries(others.map((k) => [k, 6])) };
  const shards = split([...group, ...others], timings, 2);
  const holders = shards.filter((shard) => group.some((test) => shard.includes(test.key)));
  assert.equal(holders.length, 1);
  assert.deepEqual(holders[0].filter((k) => k.includes('serial.spec.ts')), group.map((test) => test.key));
  // The group (12) is one unit and takes a shard; the three tests of 6 s share the other.
  const total = (shard) => shard.reduce((sum, k) => sum + timings[k], 0);
  assert.deepEqual(shards.map(total).sort(), [12, 18]);
});

test('a group with a test the timing file does not know is placed whole after the known units', () => {
  const group = ['one', 'two'].map((title) => ({ key: key('serial.spec.ts', title), group: 'g' }));
  const solo = key('solo.spec.ts', 'solo');
  const shards = split([...group, solo], { [group[0].key]: 5, [solo]: 5 }, 2);
  assert.equal(shards.filter((shard) => shard.includes(group[0].key) && shard.includes(group[1].key)).length, 1);
});

test('the startup of a shard counts as time already spent', () => {
  const keys = ['a', 'b', 'c', 'd'].map((title) => key('f.spec.ts', title));
  const timings = Object.fromEntries(keys.map((k) => [k, 10]));
  // Shard 1 starts 20 s late: it gets two tests fewer than shard 2 of equal tests, not an equal share.
  const shards = split(keys, timings, 2, [20, 0]);
  assert.deepEqual(shards.map((shard) => shard.length), [1, 3]);
});

test('the fresh shard costs its extra setup seconds times the workers, because durations are summed over the workers', () => {
  const others = [28, 30, 29].map((seconds) => ({ fresh: false, seconds, workers: 2 }));
  assert.equal(freshOverhead([{ fresh: true, seconds: 94, workers: 2 }, ...others]), 130);
  assert.equal(freshOverhead([{ fresh: true, seconds: 94, workers: 4 }, ...others]), 260);
  assert.equal(freshOverhead([{ fresh: true, seconds: 20, workers: 2 }, { fresh: false, seconds: 30, workers: 2 }]), 0);
  assert.equal(freshOverhead([{ fresh: false, seconds: 30, workers: 2 }]), undefined);
});

test('a shard that loses the overhead in wall time ends with the others once the workers are counted', () => {
  // 2 workers: a shard of load L takes L / 2 s. The fresh shard needs 65 s more setup, which is 130 s of load.
  const keys = Array.from({ length: 29 }, (_, i) => key('f.spec.ts', `t${i}`));
  const timings = Object.fromEntries(keys.map((k) => [k, 10]));
  const wall = (shards, startup) => shards.map((shard, i) => (shard.length * 10) / 2 + startup[i]);
  const shards = split(keys, timings, 3, [130, 0, 0]);
  assert.deepEqual(wall(shards, [65, 0, 0]), [70, 70, 70]);
  const unweighted = split(keys, timings, 3, [65, 0, 0]);
  assert.notDeepEqual(wall(unweighted, [65, 0, 0]), [70, 70, 70]);
});

test('timingsOf writes the fresh shard entry from the setup files', () => {
  const files = [suite(['a.spec.ts', 'one', 'passed', 1000]), { fresh: true, seconds: 90, workers: 2 }, { fresh: false, seconds: 30, workers: 2 }];
  assert.deepEqual(timingsOf(files), { '[chrome] › a.spec.ts › one': 1, [FRESH_KEY]: 120 });
});

test('a timing file with stale entries and no tests at all still splits', () => {
  assert.deepEqual(split([], { [key('gone.spec.ts', 'gone')]: 5 }, 2), [[], []]);
});

test('testsOf names a test by project, file and the titles of its describes', () => {
  const report = {
    suites: [
      {
        title: 'a.spec.ts',
        specs: [{ file: 'a.spec.ts', title: 'alone', tests: [{ projectName: 'chrome' }] }],
        suites: [{ title: 'group', specs: [{ file: 'a.spec.ts', title: 'inside', tests: [{ projectName: 'chrome' }] }] }],
      },
    ],
  };
  assert.deepEqual(testsOf(report).map((t) => t.key), ['[chrome] › a.spec.ts › alone', '[chrome] › a.spec.ts › group › inside']);
});

test('timingsOf keeps the longest passed run in seconds and drops the tests that did not pass', () => {
  const timings = timingsOf([suite(['a.spec.ts', 'one', 'passed', 1234], ['b.spec.ts', 'two', 'failed', 9000]), suite(['a.spec.ts', 'one', 'passed', 2051])]);
  assert.deepEqual(timings, { '[chrome] › a.spec.ts › one': 2.1 });
});

test('unknown tests weigh the median of the known ones, and 1 second when nothing is known', () => {
  const [a, b, c] = ['a', 'b', 'c'].map((t) => key('a.spec.ts', t));
  const [d, e] = ['d', 'e'].map((t) => key('b.spec.ts', t));
  // Known: 1, 2, 9 (median 2). Two unknown tests of weight 2 each go to the two shards that hold 1 and 2.
  const shards = split([a, b, c, d, e], { [a]: 1, [b]: 2, [c]: 9 }, 3);
  assert.deepEqual(shards.map((shard) => shard.length), [1, 2, 2]);
  assert.deepEqual(split([d, e], {}, 2), [[d], [e]]);
});

test('the list reporter finds a serial group of a describe or of a whole file, and fails on a Playwright without the mode', () => {
  const node = (type, title, mode, children = [], tests = []) => ({
    type,
    title,
    _parallelMode: mode,
    suites: children,
    tests,
    titlePath() {
      return [...(this.parent?.titlePath() ?? []), this.title];
    },
  });
  const leaf = (suite, title) => {
    const test = { titlePath: () => [...suite.titlePath(), title] };
    suite.tests.push(test);
  };
  const build = (fileMode, describeMode) => {
    const root = node('root', '', undefined);
    const project = node('project', 'chrome', 'parallel');
    const file = node('file', 'a.spec.ts', fileMode);
    const group = node('describe', 'group', describeMode);
    for (const [parent, child] of [[root, project], [project, file], [file, group]]) {
      child.parent = parent;
      parent.suites.push(child);
    }
    leaf(file, 'alone');
    leaf(group, 'inside');
    return root;
  };
  assert.deepEqual(groupedTests(build('parallel', 'serial')).map((t) => t.group), [' › chrome › a.spec.ts › group', null]);
  assert.deepEqual(groupedTests(build('serial', 'parallel')).map((t) => t.group), [' › chrome › a.spec.ts', ' › chrome › a.spec.ts']);
  const old = build('parallel', 'parallel');
  delete old.suites[0]._parallelMode;
  assert.throws(() => groupedTests(old), /_parallelMode/);
});
