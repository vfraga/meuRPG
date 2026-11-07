import { signal } from '@angular/core';

import type { HeldRollOffer } from '../../../gen/meurpg/play/v1/combat_pb';
import { HeldFollow } from './held-follow';
import { forgetAnsweredHere, markAnsweredHere } from './held-roll-controller';
import { MASTER_ANSWERED_TEXT } from './held-vanish';
import { heldDie, heldLucky } from './held-roll-testing';

function setup(shown: HeldRollOffer | null) {
  const offer = signal<HeldRollOffer | null>(shown);
  const held = { offer, ask: (o: HeldRollOffer | null | undefined) => offer.set(o ?? null) };
  return { offer, follow: new HeldFollow(held) };
}

describe('HeldFollow', () => {
  it('says the master answered when the question is no longer open', () => {
    const lucky = heldLucky([1], 5, { holdId: 'follow-1' });
    const { offer, follow } = setup(lucky);
    follow.see(lucky);
    expect(follow.vanish.notice()).toBe('');
    follow.see(null);
    expect(follow.vanish.notice()).toBe(MASTER_ANSWERED_TEXT);
    expect(offer()).toBeNull();
  });

  it('says nothing when this browser sent the answer', () => {
    const lucky = heldLucky([1], 5, { holdId: 'follow-2' });
    const { offer, follow } = setup(lucky);
    follow.see(lucky);
    markAnsweredHere(lucky);
    offer.set(null);
    follow.see(null);
    expect(follow.vanish.notice()).toBe('');
  });

  it('treats another hold on the roller as this question gone', () => {
    const mine = heldLucky([1], 5, { holdId: 'follow-3' });
    const other = heldLucky([1], 5, { holdId: 'follow-other' });
    const { offer, follow } = setup(mine);
    follow.see(mine);
    follow.see(other);
    expect(follow.vanish.notice()).toBe(MASTER_ANSWERED_TEXT);
    expect(offer()).toBeNull();
  });

  it('does nothing while the sheet shows no question', () => {
    const { follow } = setup(null);
    follow.see(heldLucky([1], 5, { holdId: 'follow-4' }));
    expect(follow.vanish.notice()).toBe('');
  });

  it('asks the next question once, however many times the place reads it again', () => {
    const lucky = heldLucky([1], 5, { holdId: 'follow-5' });
    const { offer, follow } = setup(lucky);
    const asked: (HeldRollOffer | null | undefined)[] = [];
    const held = {
      offer,
      ask: (o: HeldRollOffer | null | undefined) => {
        asked.push(o);
        offer.set(o ?? null);
      },
    };
    const f = new HeldFollow(held);
    f.see(lucky);
    // The master answered Lucky's question: the die's comes next, and every read gives a new object with the same key.
    for (let i = 0; i < 4; i++) {
      f.see(heldDie(5, 5, 8, { holdId: 'follow-5' }));
    }
    expect(asked).toHaveLength(1);
    expect(follow).toBeDefined();
  });

  it('drops the note when the next question goes on screen, and does not bring it back after the player answers it', () => {
    const lucky = heldLucky([1], 5, { holdId: 'follow-6' });
    const die = heldDie(5, 5, 8, { holdId: 'follow-6' });
    const { offer, follow } = setup(lucky);
    follow.see(lucky);
    follow.see(die);
    expect(offer()?.question.case).toBe(die.question.case);
    expect(follow.vanish.notice()).toBe('');
    markAnsweredHere(die);
    offer.set(null);
    follow.see(null);
    expect(follow.vanish.notice()).toBe('');
  });

  it('says the master answered once a refused click no longer hides it', () => {
    const lucky = heldLucky([1], 5, { holdId: 'follow-7' });
    const { offer, follow } = setup(lucky);
    follow.see(lucky);
    markAnsweredHere(lucky); // the click was on its way when the master's answer came
    follow.see(null);
    expect(follow.vanish.notice()).toBe('');
    forgetAnsweredHere(lucky); // the server refused the click: the master answered first
    offer.set(lucky);
    follow.see(null);
    expect(follow.vanish.notice()).toBe(MASTER_ANSWERED_TEXT);
  });
});
