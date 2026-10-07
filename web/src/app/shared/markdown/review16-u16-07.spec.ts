// Finding U16-07 in review/unit-16-web-content-campaigns.md
import { describe, expect, it } from 'vitest';
import { parseMarkdown } from './markdown';

function timeParse(lines: number): number {
  const body = '- a\n' + ' b\n'.repeat(lines);
  const start = performance.now();
  parseMarkdown(body);
  return performance.now() - start;
}

describe('Review16 U16-07: indented list continuation lines parse in linear time', () => {
  it('parses 30k continuation lines (about 90KB, under the 200KB limit) quickly', () => {
    timeParse(1000); // warm up
    const ms = timeParse(30_000);
    console.log(`U16-07 30k continuation lines: ${Math.round(ms)} ms`);
    // Linear parsing takes tens of ms; the quadratic copy takes seconds.
    expect(ms).toBeLessThan(1500);
  }, 120_000);

  it('doubling the continuation lines does not quadruple the time', () => {
    timeParse(1000);
    const t1 = timeParse(15_000);
    const t2 = timeParse(30_000);
    console.log(`U16-07 15k: ${Math.round(t1)} ms, 30k: ${Math.round(t2)} ms`);
    expect(t2).toBeLessThan(Math.max(t1, 50) * 3);
  }, 120_000);
});
