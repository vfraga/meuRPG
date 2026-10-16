import { describe, expect, it } from 'vitest';

import { PAPERS, planFor } from '../print-math';
import { buildSheet } from './print-sheet';

describe('buildSheet', () => {
  const size = { width: 60, height: 80 };
  const plan = planFor(PAPERS[0], 'portrait', size);

  it('draws the dashed edge and the cross of every side that has a neighbour', () => {
    const middle = buildSheet(plan, size, 2.54, 1, 1);
    expect(middle.marks).toBe(
      `M1 0V${plan.usableH}M${plan.usableW - 1} 0V${plan.usableH}M0 1H${plan.usableW}M0 ${plan.usableH - 1}H${plan.usableW}`,
    );
    expect(middle.crosses.startsWith('M0.65 1H1.35M1 0.65V1.35')).toBe(true);
  });
});
