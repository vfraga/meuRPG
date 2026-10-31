// How to walk a Playwright JSON report, for every script that reads one (flake.mjs, e2e-shards.mjs).

/**
 * Calls `visit(test, spec, titles)` for every test of a Playwright JSON report, in file order. `titles` are the
 * describes that hold the spec, the file's own suite left out.
 */
export function eachTest(report, visit) {
  const walk = (suite, titles) => {
    for (const spec of suite.specs ?? []) for (const t of spec.tests ?? []) visit(t, spec, titles);
    for (const child of suite.suites ?? []) walk(child, [...titles, child.title]);
  };
  for (const file of report.suites ?? []) walk(file, []);
}
