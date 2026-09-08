const fs = require('node:fs');
const assert = require('node:assert/strict');
const report = JSON.parse(fs.readFileSync('test-results/site-results.json', 'utf8'));
assert.equal(report.stats.unexpected, 0, 'unexpected browser failures');
assert.equal(report.stats.skipped, 0, 'browser tests must execute');
assert.equal(report.stats.flaky, 0, 'retries must not hide failures');
assert.equal(report.stats.expected, 28, 'seven flows must pass at four viewports');
const counts = new Map();
function visit(suite) {
  for (const spec of suite.specs || []) for (const test of spec.tests) {
    assert.equal(test.results.length, 1, 'each case must run once');
    assert.equal(test.results[0].status, 'passed', 'case did not pass');
    const names = counts.get(test.projectName) || new Set();
    names.add(spec.title);
    counts.set(test.projectName, names);
  }
  for (const child of suite.suites || []) visit(child);
}
for (const suite of report.suites) visit(suite);
assert.deepEqual([...counts.keys()].sort(), ['laptop', 'mobile', 'ultrawide', 'wide']);
for (const [project, names] of counts) assert.equal(names.size, 7, project + ' missing a flow');
console.log('Adoption browser suite: 28 passed, 7 distinct flows × 4 viewports, 0 skipped');
