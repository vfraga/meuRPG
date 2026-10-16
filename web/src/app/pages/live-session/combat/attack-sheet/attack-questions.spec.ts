import { signal } from '@angular/core';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  AttackOutcome,
  AttackRollSchema,
  DiceRollSchema,
  InspirationOfferSchema,
  LuckyOfferSchema,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import type { AttackStage } from '../../../../core/combat/attack-flow';
import { encounter } from '../../../../core/combat/combat-testing';
import { AttackQuestions } from './attack-questions';

const d20 = (face: number, over: Record<string, unknown> = {}) =>
  create(DiceRollSchema, {
    diceCount: 1,
    diceSides: 20,
    faces: [face],
    modifier: 6,
    total: face + 6,
    ...over,
  });
const lucky = create(LuckyOfferSchema, { holdId: 'h1', d20: d20(1) });
const die = create(InspirationOfferSchema, { holdId: 'h2', d20: d20(9) });

describe('AttackQuestions', () => {
  let resources: {
    answerLucky: ReturnType<typeof vi.fn>;
    answerBardicInspiration: ReturnType<typeof vi.fn>;
  };
  let settled: unknown[];
  let applied: unknown[];
  let played: unknown[];
  let held: string[];
  const busy = signal(false);
  const error = signal('');
  const stage = signal<AttackStage>('done');

  function questions(
    start: { offer: typeof die | null; lucky: typeof lucky | null } = { offer: null, lucky: null },
  ) {
    return new AttackQuestions(
      {
        resources: resources as never,
        campaignId: 'c',
        encounterId: 'enc',
        state: { apply: (e) => applied.push(e) },
        animator: { play: (s) => played.push(s) },
        busy,
        error,
        stage,
        onHeld: (id) => held.push(id),
        settle: (res) => settled.push(res),
      },
      start,
    );
  }

  const answered = (over: Record<string, unknown> = {}) => ({
    encounter: encounter({}),
    roll: create(AttackRollSchema, {
      outcome: AttackOutcome.HIT,
      d20: d20(14, { rerolled: [{ index: 0, from: 1, to: 14, sourceKey: 'trait:lucky' }] }),
      bonusDice: [{ sides: 8, face: 5, used: true }],
    }),
    pending: undefined,
    ...over,
  });

  beforeEach(() => {
    resources = { answerLucky: vi.fn(), answerBardicInspiration: vi.fn() };
    settled = [];
    applied = [];
    played = [];
    held = [];
    busy.set(false);
    error.set('');
    stage.set('done');
  });

  it('starts with the questions a reopened sheet carries', () => {
    const q = questions({ offer: die, lucky });
    expect(q.offer()).toBe(die);
    expect(q.luckyOffer()).toBe(lucky);
  });

  describe('the question a reaction window held', () => {
    it("shows Lucky's first, then the die's, and tells the page about the hold", () => {
      const q = questions();
      q.askAfterWindow(lucky, die);
      expect(stage()).toBe('lucky');
      expect(q.luckyOffer()).toBe(lucky);
      expect(q.offer()).toBe(die);
      expect(held).toEqual(['h1']);
      q.askAfterWindow(undefined, die);
      expect(stage()).toBe('inspire');
      expect(q.luckyOffer()).toBeNull();
      expect(held).toEqual(['h1', 'h2']);
    });

    it('does nothing when the combat carries no question', () => {
      const q = questions();
      q.askAfterWindow(undefined, undefined);
      expect(stage()).toBe('done');
      expect(held).toEqual([]);
    });
  });

  describe("Lucky's answer", () => {
    it('does nothing without the question, or while busy', async () => {
      const q = questions();
      await q.answerLucky(true, null);
      const withOffer = questions({ offer: null, lucky });
      busy.set(true);
      await withOffer.answerLucky(true, null);
      expect(resources.answerLucky).not.toHaveBeenCalled();
    });

    it('settles the roll it brings, tumbling the new face when the app rolled again, and renews the key', async () => {
      resources.answerLucky.mockResolvedValue(answered());
      const q = questions({ offer: null, lucky });
      await q.answerLucky(true, { inApp: true });
      expect(resources.answerLucky).toHaveBeenCalledWith(
        'c',
        'enc',
        'h1',
        true,
        { inApp: true },
        expect.any(String),
      );
      expect(settled.length).toBe(1);
      expect(applied.length).toBe(1);
      expect(played.length).toBe(1);
      await q.answerLucky(true, { inApp: true });
      const keys = resources.answerLucky.mock.calls.map((c) => c[5]);
      expect(keys[1]).not.toBe(keys[0]);
    });

    it('tumbles nothing for a typed face', async () => {
      resources.answerLucky.mockResolvedValue(answered());
      await questions({ offer: null, lucky }).answerLucky(true, { face: 14 });
      expect(played).toEqual([]);
    });

    it('says when it fails, and frees the sheet', async () => {
      resources.answerLucky.mockRejectedValue(new ConnectError('x', Code.Unavailable));
      await questions({ offer: null, lucky }).answerLucky(false, null);
      expect(error()).not.toBe('');
      expect(busy()).toBe(false);
    });
  });

  describe("the die's answer", () => {
    it('does nothing without the question, or while busy', async () => {
      await questions().answerInspiration(true, null);
      busy.set(true);
      await questions({ offer: die, lucky: null }).answerInspiration(true, null);
      expect(resources.answerBardicInspiration).not.toHaveBeenCalled();
    });

    it('settles the roll, tumbling the die the player added in the app', async () => {
      resources.answerBardicInspiration.mockResolvedValue(answered());
      const q = questions({ offer: die, lucky: null });
      await q.answerInspiration(true, { inApp: true });
      expect(resources.answerBardicInspiration).toHaveBeenCalledWith(
        'c',
        'enc',
        'h2',
        true,
        { inApp: true },
        expect.any(String),
      );
      expect(settled.length).toBe(1);
      expect(played.length).toBe(1);
      await q.answerInspiration(false, null);
      expect(played.length).toBe(1);
    });

    it('says when it fails', async () => {
      resources.answerBardicInspiration.mockRejectedValue(new Error('x'));
      await questions({ offer: die, lucky: null }).answerInspiration(true, null);
      expect(error()).not.toBe('');
    });
  });
});
