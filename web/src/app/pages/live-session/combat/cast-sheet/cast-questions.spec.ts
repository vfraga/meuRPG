import { signal } from '@angular/core';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  DiceRollSchema,
  LuckyOfferSchema,
  OutsideInspirationOfferSchema,
  SpellCastSchema,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { LuckyQuestion } from '../../../../core/combat/lucky-question';
import { encounter } from '../../../../core/combat/combat-testing';
import { InspirationQuestion } from '../../../../core/resources/bardic-inspiration';
import { CastQuestions } from './cast-questions';

const d20 = (face: number, over: Record<string, unknown> = {}) =>
  create(DiceRollSchema, {
    diceCount: 1,
    diceSides: 20,
    faces: [face],
    modifier: 5,
    total: face + 5,
    ...over,
  });
const lucky = create(LuckyOfferSchema, { holdId: 'h1', d20: d20(1) });
const die = create(OutsideInspirationOfferSchema, { holdId: 'h2', d20: d20(9) });
const enc = encounter({});

describe('CastQuestions', () => {
  let resources: {
    answerOutsideLucky: ReturnType<typeof vi.fn>;
    answerOutsideInspiration: ReturnType<typeof vi.fn>;
  };
  let shown: unknown[];
  let played: unknown[];
  let applied: unknown[];
  const busy = signal(false);
  const error = signal('');
  const typing = signal(true);

  function questions() {
    return new CastQuestions({
      resources: () => resources as never,
      campaignId: 'c',
      name: 'Raio',
      state: { apply: (e) => applied.push(e) },
      animator: { play: (s) => played.push(s) },
      busy,
      error,
      typing,
      showCast: (e, c) => shown.push([e, c]),
    });
  }

  beforeEach(() => {
    resources = { answerOutsideLucky: vi.fn(), answerOutsideInspiration: vi.fn() };
    shown = [];
    played = [];
    applied = [];
    busy.set(false);
    error.set('');
    typing.set(true);
  });

  describe('a cast that answered with a question', () => {
    it("shows Lucky's, applies the combat, stops typing and draws the held d20", () => {
      const q = questions();
      expect(q.asks(new LuckyQuestion(lucky, enc))).toBe(true);
      expect(q.luckyOffer()).toBe(lucky);
      expect(q.luckyHeld()).not.toBe('');
      expect(applied).toEqual([enc]);
      expect(typing()).toBe(false);
      expect(played.length).toBe(1);
    });

    it("shows the die's, and draws nothing when the held d20 cannot be drawn", () => {
      const q = questions();
      expect(
        q.asks(
          new InspirationQuestion(create(OutsideInspirationOfferSchema, { holdId: 'h' }), enc),
        ),
      ).toBe(true);
      expect(q.offer()?.holdId).toBe('h');
      expect(played).toEqual([]);
    });

    it('is not one for any other error', () => {
      expect(questions().asks(new Error('x'))).toBe(false);
      expect(applied).toEqual([]);
    });
  });

  describe("Lucky's answer", () => {
    const cast = create(SpellCastSchema, {});

    it('does nothing without the question, or while busy', async () => {
      const q = questions();
      await q.answerLucky(true, null);
      q.luckyOffer.set(lucky);
      busy.set(true);
      await q.answerLucky(true, null);
      expect(resources.answerOutsideLucky).not.toHaveBeenCalled();
    });

    it('shows the cast it brings, and renews the key', async () => {
      resources.answerOutsideLucky.mockResolvedValue({
        result: { case: 'spellCast', value: { encounter: enc, cast } },
      });
      const q = questions();
      q.luckyOffer.set(lucky);
      await q.answerLucky(true, { face: 14 });
      await q.answerLucky(true, { face: 14 });
      expect(shown).toEqual([[enc, cast]]);
      expect(q.luckyOffer()).toBeNull();
      expect(typing()).toBe(false);
      q.luckyOffer.set(lucky);
      await q.answerLucky(true, { face: 14 });
      const keys = resources.answerOutsideLucky.mock.calls.map((c) => c[4]);
      expect(keys[1]).not.toBe(keys[0]);
    });

    it("goes on to the die's question on the new roll, tumbling the d20 the app rolled", async () => {
      const again = create(OutsideInspirationOfferSchema, {
        holdId: 'h3',
        d20: d20(14, { rerolled: [{ index: 0, from: 1, to: 14, sourceKey: 'trait:lucky' }] }),
      });
      resources.answerOutsideLucky.mockResolvedValue({
        result: { case: 'spellCast', value: { encounter: enc, inspirationOffer: again } },
      });
      const q = questions();
      q.luckyOffer.set(lucky);
      await q.answerLucky(true, { inApp: true });
      expect(q.offer()).toBe(again);
      expect(applied).toEqual([enc]);
      expect(played.length).toBe(1);
      expect(shown).toEqual([]);
    });

    it('says when the answer brings no cast, or fails, and frees the sheet', async () => {
      resources.answerOutsideLucky
        .mockResolvedValueOnce({ result: { case: undefined } })
        .mockResolvedValueOnce({ result: { case: 'spellCast', value: { encounter: enc } } })
        .mockRejectedValueOnce(new ConnectError('x', Code.Unavailable));
      const q = questions();
      for (let i = 0; i < 3; i++) {
        q.luckyOffer.set(lucky);
        error.set('');
        await q.answerLucky(true, null);
        expect(error()).not.toBe('');
        expect(busy()).toBe(false);
      }
    });
  });

  describe("the die's answer", () => {
    it('does nothing without the question, or while busy', async () => {
      const q = questions();
      await q.answerDie(true, null);
      q.offer.set(die);
      busy.set(true);
      await q.answerDie(true, null);
      expect(resources.answerOutsideInspiration).not.toHaveBeenCalled();
    });

    it('shows the cast it brings, and says when it brings none or fails', async () => {
      const cast = create(SpellCastSchema, {});
      resources.answerOutsideInspiration
        .mockResolvedValueOnce({ result: { case: 'spellCast', value: { encounter: enc, cast } } })
        .mockResolvedValueOnce({ result: { case: 'spellCast', value: { encounter: enc } } })
        .mockRejectedValueOnce(new Error('x'));
      const q = questions();
      q.offer.set(die);
      await q.answerDie(true, null);
      expect(shown).toEqual([[enc, cast]]);
      expect(q.offer()).toBeNull();
      for (let i = 0; i < 2; i++) {
        q.offer.set(die);
        error.set('');
        await q.answerDie(false, null);
        expect(error()).not.toBe('');
      }
    });
  });
});
