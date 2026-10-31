// A Playwright reporter for `playwright test --list`: prints the tests as JSON, with the serial group of each one, and the number of workers.
//
//   playwright test --list --reporter=<this file>
//
// The JSON reporter does not say which tests are in a `test.describe.configure({ mode: 'serial' })` group (of a
// describe or of a whole file), and a group must run whole in one shard (docs/ci.md, "End to end"). The mode is read
// from Playwright's suite tree (`_parallelMode`, not public API); a Playwright without it fails the run instead of
// splitting groups.
import process from 'node:process';

// The tests under `root` as {project, file, titles, group}; group is the title path of the outermost serial suite, or null.
export function groupedTests(root) {
  const tests = [];
  const walk = (suite, group) => {
    if (suite !== root) {
      if (!('_parallelMode' in suite)) throw new Error('Playwright no longer has Suite._parallelMode: update .github/scripts/e2e-list-reporter.mjs');
      // The outermost serial suite is the group; the ones inside it belong to it.
      if (group === null && (suite.type === 'describe' || suite.type === 'file') && suite._parallelMode === 'serial') group = suite.titlePath().join(' › ');
    }
    for (const child of suite.suites) walk(child, group);
    for (const test of suite.tests) {
      const [, project, file, ...titles] = test.titlePath();
      tests.push({ project, file, titles, group });
    }
  };
  walk(root, null);
  return tests;
}

export default class ListReporter {
  printsToStdio() {
    return true;
  }

  onBegin(config, root) {
    // `workers` is what the config resolved to (Playwright's default is half the CPUs), so it is the run's own.
    process.stdout.write(JSON.stringify({ workers: config.workers, tests: groupedTests(root) }) + '\n');
  }
}
