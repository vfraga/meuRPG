import { pensantusVitals } from '../testing';
import { changeBetween, draftFrom } from './adjust-vitals.types';

// Finding U17-6 (review/unit-17-contract.md)
describe('Review17 U17-6: the master cannot restore spent resource uses from the live session', () => {
  it('carries a changed resource count as resourcesUsed in the outgoing change', () => {
    const v = {
      ...pensantusVitals(),
      resources: [{ key: 'wild_shape', total: 2, used: 2 }],
    } as unknown as ReturnType<typeof pensantusVitals>;
    // The master gives one use back (spent 2 -> 1), as AdjustCharacterVitals allows.
    const draft = { ...draftFrom(v), resourcesUsed: { wild_shape: 1 } };
    const change = changeBetween(v, draft as never) as Record<string, unknown> | null;
    expect(change).not.toBeNull();
    expect(change?.['resourcesUsed']).toEqual([{ key: 'wild_shape', used: 1 }]);
  });
});
