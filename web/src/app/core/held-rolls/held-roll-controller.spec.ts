import { signal } from '@angular/core';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  EncounterBlockedReason,
  EncounterBlockedSchema,
} from '../../../gen/meurpg/play/v1/combat_pb';
import {
  ResourceBlockedReason,
  ResourceBlockedSchema,
} from '../../../gen/meurpg/play/v1/resources_pb';
import {
  MASTER_WAIT_SECONDS,
  type HeldRollHost,
  HeldRollController,
  forgetAnsweredHere,
  wasAnsweredHere,
} from './held-roll-controller';
import { HELD_ROLL_ANSWERED_TEXT, MASTER_TOO_EARLY_REASON } from './held-roll-errors';
import type { HeldAnswer } from './held-roll-offer';
import {
  askedAt,
  heldAnswer,
  heldDie,
  heldIndomitable,
  heldLucky,
  heldStroke,
} from './held-roll-testing';

const failed = (code: Code, detail?: ConstructorParameters<typeof ConnectError>[3]) =>
  new ConnectError('x', code, undefined, detail);

const blockedBy = (reason: ResourceBlockedReason) =>
  failed(Code.FailedPrecondition, [
    { desc: ResourceBlockedSchema, value: create(ResourceBlockedSchema, { reason }) },
  ]);

/** A reason number no generated value uses. */
const TOO_EARLY = 9999;

describe('HeldRollController', () => {
  const rerollInApp: HeldAnswer = { kind: 'lucky', reroll: true, roll: { inApp: true } };
  const useDie: HeldAnswer = { kind: 'die', use: true, roll: { inApp: true } };
  let sent: { hold: string; answer: HeldAnswer; key: string }[];
  let settled: unknown[];
  let counter: number;
  let reply: () => Promise<ReturnType<typeof heldAnswer>>;

  function build(over: Partial<HeldRollHost> = {}) {
    return new HeldRollController({
      send: (hold, answer, key) => {
        sent.push({ hold, answer, key });
        return reply();
      },
      settle: (res, done) => {
        settled.push([res, done.answer, done.asked.holdId]);
      },
      newKey: () => `k${counter++}`,
      ...over,
    });
  }

  beforeEach(() => {
    sent = [];
    settled = [];
    counter = 0;
    reply = () => Promise.resolve(heldAnswer());
  });

  it('asks nothing until a question is shown, and answers nothing without one', async () => {
    const held = build();
    expect(held.offer()).toBeNull();
    await held.answer(rerollInApp);
    expect(sent).toEqual([]);
    held.ask(heldLucky());
    expect(held.offer()?.holdId).toBe('h1');
    held.ask(undefined);
    expect(held.offer()).toBeNull();
  });

  it('sends the answer for the hold and hands the response to the host to finish', async () => {
    const held = build();
    held.ask(heldLucky());
    const res = heldAnswer({ result: { case: 'attack', value: {} } });
    reply = () => Promise.resolve(res);
    await held.answer(rerollInApp);
    expect(sent).toEqual([
      { hold: 'h1', answer: rerollInApp, key: expect.stringMatching(/^k\d+$/) },
    ]);
    expect(settled).toEqual([[res, rerollInApp, 'h1']]);
    // The roll was written: nothing more to ask.
    expect(held.offer()).toBeNull();
    expect(held.busy()).toBe(false);
    expect(held.error()).toBe('');
  });

  it('takes `next` as the following question: Lucky first, then the die on the new roll', async () => {
    const held = build();
    held.ask(heldLucky());
    const next = heldDie(14, 5, 8, {}, 1);
    reply = () => Promise.resolve(heldAnswer({ next }));
    await held.answer(rerollInApp);
    expect(held.offer()).toBe(next);
    reply = () => Promise.resolve(heldAnswer());
    await held.answer(useDie);
    expect(sent.map((s) => s.answer)).toEqual([rerollInApp, useDie]);
    expect(held.offer()).toBeNull();
  });

  it('sends the Indomitable answer, then the new roll’s result closes the question', async () => {
    const held = build();
    held.ask(heldIndomitable());
    const again: HeldAnswer = { kind: 'indomitable', reroll: true, roll: { inApp: true } };
    await held.answer(again);
    expect(sent.map((s) => s.answer)).toEqual([again]);
    expect(settled).toHaveLength(1);
    expect(held.offer()).toBeNull();
  });

  it('sends the two typed faces of a save with advantage as they are', async () => {
    const held = build();
    held.ask(heldIndomitable());
    const pair: HeldAnswer = { kind: 'indomitable', reroll: true, roll: { faces: [4, 15] } };
    await held.answer(pair);
    expect(sent.map((s) => s.answer)).toEqual([pair]);
  });

  it('sends the Stroke of Luck answer and the result closes the question', async () => {
    const held = build();
    held.ask(heldStroke());
    const use: HeldAnswer = { kind: 'stroke', use: true };
    await held.answer(use);
    expect(sent.map((s) => s.answer)).toEqual([use]);
    expect(settled).toHaveLength(1);
    expect(held.offer()).toBeNull();
  });

  it('words the Stroke of Luck question that blocks the next roll', async () => {
    const held = build({
      send: () => Promise.reject(blockedBy(ResourceBlockedReason.STROKE_OF_LUCK_PENDING)),
    });
    held.ask(heldStroke());
    await held.answer({ kind: 'stroke', use: false });
    expect(held.error()).toContain('Golpe de Sorte');
    expect(held.offer()).not.toBeNull();
  });

  it('words the Indomitable question that blocks the next roll', async () => {
    const held = build({
      send: () => Promise.reject(blockedBy(ResourceBlockedReason.INDOMITABLE_PENDING)),
    });
    held.ask(heldIndomitable());
    await held.answer({ kind: 'indomitable', reroll: false, roll: null });
    expect(held.error()).toContain('Indomitável');
    expect(held.offer()).not.toBeNull();
  });

  it('keeps one key for an answer repeated after a failure, and renews it once the answer got through', async () => {
    const held = build();
    held.ask(heldLucky());
    reply = () => Promise.reject(failed(Code.Unavailable));
    await held.answer(rerollInApp);
    await held.answer(rerollInApp);
    expect(sent[1].key).toBe(sent[0].key);
    expect(held.offer()).not.toBeNull();
    // Another answer is another request: another key.
    await held.answer({ kind: 'lucky', reroll: false, roll: null });
    expect(sent[2].key).not.toBe(sent[0].key);
    reply = () => Promise.resolve(heldAnswer({ next: heldDie() }));
    await held.answer(rerollInApp);
    await held.answer(useDie);
    // After an answer that got through the key is new, even for the same answer.
    expect(sent.at(-1)?.key).not.toBe(sent.at(-2)?.key);
  });

  it('ignores a second tap while an answer is on the way', async () => {
    const held = build();
    held.ask(heldLucky());
    let release!: (v: ReturnType<typeof heldAnswer>) => void;
    reply = () => new Promise((r) => (release = r));
    const first = held.answer(rerollInApp);
    expect(held.busy()).toBe(true);
    await held.answer(rerollInApp);
    expect(sent).toHaveLength(1);
    release(heldAnswer());
    await first;
    expect(held.busy()).toBe(false);
  });

  it('shares the sheet’s flags when it is given them', async () => {
    const busy = signal(false);
    const error = signal('antes');
    const held = build({ busy, error });
    held.ask(heldLucky());
    reply = () => Promise.reject(failed(Code.Unavailable));
    await held.answer(rerollInApp);
    expect(busy()).toBe(false);
    expect(error()).toContain('o servidor não respondeu');
    expect(held.error).toBe(error);
  });

  describe('a refusal', () => {
    const answering = async (err: Error, over: Partial<HeldRollHost> = {}) => {
      const held = build(over);
      held.ask(heldLucky([1], 5, askedAt(Date.UTC(2026, 0, 1, 12, 0, 0))));
      reply = () => Promise.reject(err);
      await held.answer(rerollInApp);
      return held;
    };

    it('says the roll was answered already when another answer won the race, and keeps the question for the stream to settle', async () => {
      const held = await answering(failed(Code.FailedPrecondition));
      expect(held.error()).toBe(HELD_ROLL_ANSWERED_TEXT);
      expect(held.refusal()).toBe('answered');
      expect(held.offer()).not.toBeNull();
    });

    it('words the way of rolling the table does not allow, and the pending question, by the rule that refused', async () => {
      const mode = failed(Code.FailedPrecondition, [
        {
          desc: EncounterBlockedSchema,
          value: create(EncounterBlockedSchema, { reason: EncounterBlockedReason.WRONG_DICE_MODE }),
        },
      ]);
      const wrong = await answering(mode);
      expect(wrong.refusal()).toBeNull();
      expect(wrong.error()).not.toBe(HELD_ROLL_ANSWERED_TEXT);
      expect(wrong.error()).not.toBe('');
      const pending = await answering(blockedBy(ResourceBlockedReason.LUCKY_PENDING));
      expect(pending.refusal()).toBeNull();
      expect(pending.error()).toContain('Responda primeiro');
    });

    it('says an unknown or gone roll in words and stays at the question', async () => {
      const held = await answering(failed(Code.NotFound));
      expect(held.error()).toContain('não existe mais');
      expect(held.refusal()).toBeNull();
      expect(held.offer()).not.toBeNull();
    });

    it('words a settle that failed as an error and keeps the question', async () => {
      const held = build({
        settle: () => {
          throw new Error('AnswerHeldRoll answered without its attack');
        },
      });
      held.ask(heldLucky());
      await held.answer(rerollInApp);
      expect(held.error()).not.toBe('');
      expect(held.offer()).not.toBeNull();
      expect(held.busy()).toBe(false);
    });

    it('reads the master’s early answer as a wait, not a failure: the roller’s name and the seconds left, the countdown resumes', async () => {
      // The server names this reason with the master's panel; the generated enum has no such value yet, so the spec adds
      // one to its reverse map the way the new enum would carry it.
      const names = ResourceBlockedReason as unknown as Record<number, string>;
      names[TOO_EARLY] = MASTER_TOO_EARLY_REASON;
      try {
        const created = Date.UTC(2026, 0, 1, 12, 0, 0);
        const held = build({ master: true, now: () => created + 7_000 });
        held.ask(heldLucky([1], 5, askedAt(created)));
        reply = () => Promise.reject(blockedBy(TOO_EARLY as ResourceBlockedReason));
        await held.answer(rerollInApp);
        expect(held.refusal()).toBe('too-early');
        expect(held.error()).toBe('Ainda é a vez de Brisa responder. Você pode responder em 3 s.');
        expect(held.offer()).not.toBeNull();
        expect(held.busy()).toBe(false);
        // The clock moves on: the same question says the new wait, and the answer can go again.
        expect(held.waitSeconds(created + 9_000)).toBe(1);
        reply = () => Promise.resolve(heldAnswer());
        await held.answer(rerollInApp);
        expect(held.refusal()).toBeNull();
        expect(held.error()).toBe('');
        expect(sent[1].key).toBe(sent[0].key);
      } finally {
        delete names[TOO_EARLY];
      }
    });
  });

  describe('the master’s wait', () => {
    const created = Date.UTC(2026, 0, 1, 12, 0, 0);

    it('counts the seconds from when the roll was held, rounded up, and is zero once they passed', () => {
      const held = build({ master: true });
      held.ask(heldLucky([1], 5, askedAt(created)));
      expect(held.waitSeconds(created)).toBe(MASTER_WAIT_SECONDS);
      expect(held.waitSeconds(created + 2_500)).toBe(8);
      expect(held.waitSeconds(created + MASTER_WAIT_SECONDS * 1000)).toBe(0);
      expect(held.waitSeconds(created + 60_000)).toBe(0);
    });

    it('reads the injected clock when no time is given, so a page can correct it by the server’s', () => {
      let now = created + 3_000;
      const held = build({ master: true, now: () => now });
      held.ask(heldLucky([1], 5, askedAt(created)));
      expect(held.waitSeconds()).toBe(7);
      now += 7_000;
      expect(held.waitSeconds()).toBe(0);
    });

    it('is zero for the roller, and for a question with no time', () => {
      const roller = build();
      roller.ask(heldLucky([1], 5, askedAt(created)));
      expect(roller.waitSeconds(created)).toBe(0);
      const master = build({ master: true });
      master.ask(heldLucky());
      expect(master.waitSeconds(created)).toBe(0);
      master.ask(null);
      expect(master.waitSeconds(created)).toBe(0);
    });
  });

  describe('the questions answered in this browser', () => {
    beforeEach(() => forgetAnsweredHere());

    it('remembers an answer from the moment it is sent, so a hint that comes first is not read as the master’s answer', async () => {
      const held = build();
      const question = heldLucky();
      held.ask(question);
      let seen = false;
      reply = () => {
        seen = wasAnsweredHere(question);
        return Promise.resolve(heldAnswer());
      };
      await held.answer(rerollInApp);
      expect(seen).toBe(true);
      expect(wasAnsweredHere(question)).toBe(true);
      // Another question of the same roll is another answer.
      expect(wasAnsweredHere(heldDie())).toBe(false);
    });

    it('forgets an answer that did not get through', async () => {
      const held = build();
      const question = heldLucky();
      held.ask(question);
      reply = () => Promise.reject(failed(Code.Unavailable));
      await held.answer(rerollInApp);
      expect(wasAnsweredHere(question)).toBe(false);
    });
  });
});
