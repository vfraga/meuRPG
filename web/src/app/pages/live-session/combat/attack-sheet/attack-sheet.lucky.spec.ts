import { create } from '@bufbuild/protobuf';
import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { Code, ConnectError } from '@connectrpc/connect';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  AttackOutcome,
  AttackRollSchema,
  CombatantKind,
  CombatantSide,
  DiceRollSchema,
  HeldRollKind,
  HeldRollOfferSchema,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  ResourceBlockedReason,
  ResourceBlockedSchema,
} from '../../../../../gen/meurpg/play/v1/resources_pb';
import type { Attack } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { type AttackResult } from '../../../../core/combat/combat-client-types';
import { CombatState } from '../../../../core/combat/combat-state';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { LUCKY_TRAIT } from '../../../../core/combat/lucky';
import { heldAttackAnswer } from '../../../../core/held-rolls/held-roll-testing';
import { ResourceClient } from '../../../../core/resources/resources-client';
import { AttackSheet, type AttackSheetData } from './attack-sheet';
import { fakeCombatClient } from '../../../../core/combat/combat-client-testing';

const flat = (n: Element | null | undefined) => n?.textContent?.replace(/\s+/g, ' ').trim() ?? '';

const battleaxe = {
  key: 'equipment:battleaxe',
  name: 'Battleaxe',
  namePt: 'Machado de batalha',
  attackBonus: 5,
  saveDc: 0,
  rangeFt: 5,
  longRangeFt: 0,
  damage: '1d8 + 3',
  damageTypePt: 'cortante',
  kind: 0,
} as unknown as Attack;

const pipo = combatant({
  id: 'p',
  label: 'Pipo',
  kind: CombatantKind.PLAYER,
  side: CombatantSide.PARTY,
  mine: true,
});
const goblin = combatant({ id: 'g', label: 'Goblin' });

/** A d20 of the battleaxe (+5): one face, or the pair of a roll with advantage, and what Lucky rolled again. */
const d20 = (faces: number[], counted = 0, again?: { index: number; to: number }) =>
  create(DiceRollSchema, {
    diceCount: faces.length,
    diceSides: 20,
    faces,
    modifier: 5,
    total: (faces[counted] ?? 0) + 5,
    countedIndex: counted,
    rerolled: again ? [{ index: again.index, from: 1, to: again.to, sourceKey: LUCKY_TRAIT }] : [],
  });

const lucky = create(HeldRollOfferSchema, {
  holdId: 'hold-1',
  kind: HeldRollKind.WEAPON_ATTACK,
  d20: d20([1]),
  question: { case: 'lucky', value: { luckyIndex: 0 } },
  attackKey: battleaxe.key,
  targetId: 'g',
});

/** What `RollAttack` answers for a halfling whose d20 shows a 1: the d20 and the Lucky question, no outcome. */
const held = (state: CombatState, offer = lucky): AttackResult => ({
  encounter: state.encounter()!,
  roll: create(AttackRollSchema, { attackKey: battleaxe.key, d20: offer.d20 }),
  pending: undefined,
  held: offer,
});

/** The answer: the attack resolved with the new d20 (or with the 1 kept). */
const resolved = (state: CombatState, to: number | null): AttackResult => ({
  encounter: state.encounter()!,
  roll: create(AttackRollSchema, {
    attackKey: battleaxe.key,
    outcome: to !== null && to + 5 >= 12 ? AttackOutcome.HIT : AttackOutcome.MISS,
    d20: to === null ? d20([1]) : d20([to], 0, { index: 0, to }),
  }),
  pending: undefined,
});

describe('AttackSheet: the halfling’s Lucky question, "Tirou 1: rolar de novo?"', () => {
  const rollAttack = vi.fn();
  const answerHeldRoll = vi.fn();
  const onHeld = vi.fn();

  function open(over: { diceMode?: DiceMode; held?: AttackSheetData['held'] } = {}) {
    const state = new CombatState();
    state.apply(encounter({ currentCombatantId: 'p', combatants: [pipo, goblin] }));
    const data: AttackSheetData = {
      campaignId: 'camp',
      encounterId: 'enc',
      attackerId: 'p',
      round: 2,
      attack: battleaxe,
      targets: [],
      diceMode: over.diceMode ?? DiceMode.APP,
      preference: DicePreference.APP,
      state,
      // The target is the mover of an opportunity, so the sheet opens at "Rolar".
      opportunity: over.held
        ? undefined
        : { offerId: 'o', targetId: 'g', targetLabel: 'Goblin', byMaster: false },
      held: over.held,
      onHeld,
    };
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [
        { provide: CombatClient, useValue: fakeCombatClient({ rollAttack }) },
        { provide: ResourceClient, useValue: { answerHeldRoll } },
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close: vi.fn(), disableClose: false } },
      ],
    });
    const fixture = TestBed.createComponent(AttackSheet);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const settle = async () => {
      await fixture.whenStable();
      fixture.detectChanges();
      await fixture.whenStable();
    };
    const button = (text: string) =>
      Array.from(el.querySelectorAll('button')).find((b) => flat(b).includes(text));
    const type = (value: string) => {
      const field = el.querySelector('input.type__field') as HTMLInputElement;
      field.value = value;
      field.dispatchEvent(new Event('input'));
      fixture.detectChanges();
    };
    return { fixture, el, state, settle, button, type };
  }

  beforeEach(() => {
    // The sheet scrolls its error into view; the test DOM has no scroll.
    Element.prototype.scrollTo = vi.fn();
    rollAttack.mockReset();
    answerHeldRoll.mockReset();
    onHeld.mockReset();
  });

  it('asks after a d20 of 1 and before the result: the 1 is there, "Acertou" and "Errou" are not', async () => {
    const { el, state, settle, button } = open();
    rollAttack.mockImplementation(() => Promise.resolve(held(state)));
    button('Rolar no app')!.click();
    await settle();
    const text = flat(el);
    expect(text).toContain('Tirou 1: rolar de novo?');
    expect(text).toContain(
      'Sortudo: você pode rolar esse d20 de novo e fica com o novo resultado.',
    );
    expect(text).toContain('1d20 (1) + 5 = 6');
    expect(text).toContain('O mestre ainda não disse se acertou.');
    expect(text).not.toMatch(/Acertou|Errou|Crítico|1 natural/);
    expect(button('Rolar de novo')).toBeTruthy();
    expect(button('Ficar com o 1')).toBeTruthy();
    // The page learns the hold, so reading the screen again does not open a second question.
    expect(onHeld).toHaveBeenCalledWith('hold-1');
  });

  it('"Rolar de novo" answers with the d20 rolled in the app, then shows the result and "Sortudo: 1 → 14"', async () => {
    const { el, state, settle, button } = open();
    rollAttack.mockImplementation(() => Promise.resolve(held(state)));
    answerHeldRoll.mockImplementation(() => Promise.resolve(heldAttackAnswer(resolved(state, 14))));
    button('Rolar no app')!.click();
    await settle();
    button('Rolar de novo')!.click();
    await settle();
    expect(answerHeldRoll).toHaveBeenCalledWith(
      'camp',
      'hold-1',
      { kind: 'lucky', reroll: true, roll: { inApp: true } },
      expect.any(String),
    );
    const text = flat(el);
    expect(text).not.toContain('Tirou 1: rolar de novo?');
    expect(text).toContain('Acertou');
    expect(text).toContain('Sortudo: 1 → 14');
    expect(text).not.toContain('O mestre ainda não disse');
  });

  it('"Ficar com o 1" keeps it: the attack misses on the natural 1, and nothing says Lucky rolled', async () => {
    const { el, state, settle, button } = open();
    rollAttack.mockImplementation(() => Promise.resolve(held(state)));
    answerHeldRoll.mockImplementation(() =>
      Promise.resolve(heldAttackAnswer(resolved(state, null))),
    );
    button('Rolar no app')!.click();
    await settle();
    button('Ficar com o 1')!.click();
    await settle();
    expect(answerHeldRoll).toHaveBeenCalledWith(
      'camp',
      'hold-1',
      { kind: 'lucky', reroll: false, roll: null },
      expect.any(String),
    );
    const text = flat(el);
    expect(text).toContain('Errou · 1 natural');
    expect(text).not.toContain('Sortudo:');
  });

  it('with real dice the player types the new d20, 1 to 20', async () => {
    const { el, state, settle, button, type } = open({ diceMode: DiceMode.PHYSICAL });
    rollAttack.mockImplementation(() => Promise.resolve(held(state)));
    answerHeldRoll.mockImplementation(() => Promise.resolve(heldAttackAnswer(resolved(state, 15))));
    type('1');
    button('Confirmar 1')!.click();
    await settle();
    expect(flat(el)).toContain('Tirou 1: rolar de novo?');
    expect(flat(el)).toContain('1 a 20');
    type('15');
    button('Confirmar 15')!.click();
    await settle();
    expect(answerHeldRoll).toHaveBeenCalledWith(
      'camp',
      'hold-1',
      { kind: 'lucky', reroll: true, roll: { face: 15 } },
      expect.any(String),
    );
    expect(flat(el)).toContain('Sortudo: 1 → 15');
  });

  it('with advantage names the die that showed the 1', async () => {
    const { el, state, settle, button } = open();
    const pair = create(HeldRollOfferSchema, { ...lucky, d20: d20([1, 13], 1) });
    rollAttack.mockImplementation(() => Promise.resolve(held(state, pair)));
    button('Rolar no app')!.click();
    await settle();
    expect(flat(el)).toContain(
      'Sortudo: você pode rolar de novo o dado que deu 1 e fica com o novo resultado.',
    );
  });

  it('after the new d20, a Bardic Inspiration die the attacker holds is asked about, on the new roll', async () => {
    const { el, state, settle, button } = open();
    const next = d20([6], 0, { index: 0, to: 6 });
    rollAttack.mockImplementation(() => Promise.resolve(held(state)));
    answerHeldRoll.mockImplementation(() =>
      Promise.resolve(
        heldAttackAnswer({
          encounter: state.encounter()!,
          roll: create(AttackRollSchema, { attackKey: battleaxe.key, d20: next }),
          pending: undefined,
          held: create(HeldRollOfferSchema, {
            holdId: 'hold-1',
            kind: HeldRollKind.WEAPON_ATTACK,
            question: { case: 'die', value: { sides: 8, fromName: 'Orla' } },
            d20: next,
            attackKey: battleaxe.key,
            targetId: 'g',
          }),
        }),
      ),
    );
    button('Rolar no app')!.click();
    await settle();
    button('Rolar de novo')!.click();
    await settle();
    const text = flat(el);
    expect(text).not.toContain('Tirou 1: rolar de novo?');
    expect(text).toContain('Usar a Inspiração de Bardo (d8)?');
    expect(text).toContain('1d20 (6) + 5 = 11');
    expect(text).toContain('Sortudo: 1 → 6');
    expect(text).not.toMatch(/Acertou|Errou/);
    expect(button('Somar o d8 (Inspiração de Orla)')).toBeTruthy();
  });

  it('opens at the question when the screen is read again (the offer is on the combatant)', () => {
    const { el, button } = open({ held: { offer: lucky, targetLabel: 'Goblin' } });
    const text = flat(el);
    expect(text).toContain('Tirou 1: rolar de novo?');
    expect(text).toContain('Goblin');
    expect(text).toContain('1d20 (1) + 5 = 6');
    expect(text).not.toMatch(/Acertou|Errou/);
    expect(button('Ficar com o 1')).toBeTruthy();
  });

  // A hold of its own: the page remembers which questions this browser answered, across specs.
  const masterHold = create(HeldRollOfferSchema, { ...lucky, holdId: 'hold-master' });

  it('drops the question and says the master answered when the combat no longer carries it', async () => {
    const { fixture, el, state, settle, button } = open({
      held: { offer: masterHold, targetLabel: 'Goblin' },
    });
    // The combat the sheet reads carries the question on the attacker, as the server sends it.
    state.apply(
      encounter({
        currentCombatantId: 'p',
        combatants: [{ ...pipo, heldRollOffer: masterHold }, goblin],
      }),
    );
    await settle();
    expect(button('Ficar com o 1')).toBeTruthy();
    // The master answered: the next read of the combat has no question on the attacker.
    state.apply(encounter({ currentCombatantId: 'p', combatants: [pipo, goblin] }));
    fixture.detectChanges();
    await settle();
    expect(flat(el.querySelector('[data-testid="sheet-held-vanished"]'))).toContain(
      'O mestre respondeu por você.',
    );
    expect(button('Rolar de novo')).toBeUndefined();
    expect(button('Ficar com o 1')).toBeUndefined();
  });

  it('keeps the question when the combat it reads no longer lists the attacker at all', async () => {
    const hold = create(HeldRollOfferSchema, { ...lucky, holdId: 'hold-absent' });
    const { el, state, settle, button } = open({ held: { offer: hold, targetLabel: 'Goblin' } });
    state.apply(
      encounter({
        currentCombatantId: 'p',
        combatants: [{ ...pipo, heldRollOffer: hold }, goblin],
      }),
    );
    await settle();
    state.apply(encounter({ currentCombatantId: 'g', combatants: [goblin] }));
    await settle();
    expect(el.querySelector('[data-testid="sheet-held-vanished"]')).toBeNull();
    expect(button('Ficar com o 1')).toBeTruthy();
  });

  it('does not say the master answered when the player answered here', async () => {
    const { el, state, settle, button } = open({
      held: { offer: masterHold, targetLabel: 'Goblin' },
    });
    state.apply(
      encounter({
        currentCombatantId: 'p',
        combatants: [{ ...pipo, heldRollOffer: masterHold }, goblin],
      }),
    );
    await settle();
    answerHeldRoll.mockImplementation(() => Promise.resolve(heldAttackAnswer(resolved(state, 14))));
    button('Ficar com o 1')!.click();
    state.apply(encounter({ currentCombatantId: 'p', combatants: [pipo, goblin] }));
    await settle();
    expect(el.querySelector('[data-testid="sheet-held-vanished"]')).toBeNull();
  });

  it('keeps the key of an answer that failed, so a retry never rolls the d20 twice', async () => {
    const { el, settle, button } = open({ held: { offer: lucky, targetLabel: 'Goblin' } });
    answerHeldRoll.mockRejectedValue(new ConnectError('down', Code.Unavailable));
    button('Rolar de novo')!.click();
    await settle();
    button('Rolar de novo')!.click();
    await settle();
    expect(flat(el.querySelector('[role=alert] p'))).toContain('o servidor não respondeu');
    const keys = answerHeldRoll.mock.calls.map((c) => c[3] as string);
    expect(keys[0]).toBe(keys[1]);
    expect(flat(el)).toContain('Tirou 1: rolar de novo?');
  });

  it('says another action waits for the Lucky answer when the server refuses with LUCKY_PENDING', async () => {
    const { el, settle, button } = open();
    rollAttack.mockRejectedValue(
      new ConnectError('pending', Code.FailedPrecondition, undefined, [
        {
          desc: ResourceBlockedSchema,
          value: create(ResourceBlockedSchema, { reason: ResourceBlockedReason.LUCKY_PENDING }),
        },
      ]),
    );
    button('Rolar no app')!.click();
    await settle();
    expect(flat(el.querySelector('[role=alert] p'))).toContain(
      'Responda primeiro à pergunta do Sortudo',
    );
  });
});
