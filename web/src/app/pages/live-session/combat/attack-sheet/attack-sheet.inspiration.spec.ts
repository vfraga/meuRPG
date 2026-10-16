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
  InspirationDieSchema,
  InspirationOfferSchema,
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
import { ResourceClient } from '../../../../core/resources/resources-client';
import { AttackSheet, type AttackSheetData } from './attack-sheet';
import { fakeCombatClient } from '../../../../core/combat/combat-client-testing';

const flat = (n: Element | null | undefined) =>
  n?.textContent?.replace(/\s+/g, ' ').trim().replace(/ /g, ' ') ?? '';

const longsword = {
  key: 'attack:longsword',
  name: 'Longsword',
  namePt: 'Espada longa',
  attackBonus: 5,
  saveDc: 0,
  rangeFt: 5,
  longRangeFt: 0,
  damage: '1d8 + 3',
  damageTypePt: 'cortante',
  kind: 0,
} as unknown as Attack;

const toren = combatant({
  id: 't',
  label: 'Toren',
  kind: CombatantKind.PLAYER,
  side: CombatantSide.PARTY,
  mine: true,
});
const cap = combatant({ id: 'cap', label: 'Capitão Goblin' });

const die = create(InspirationDieSchema, {
  sides: 8,
  fromLabel: 'Orla',
  fromCombatantId: 'o',
  expiresAtRound: 90,
});
const d20 = (face: number, total = face + 5) =>
  create(DiceRollSchema, {
    diceCount: 1,
    diceSides: 20,
    faces: [face],
    modifier: 5,
    total,
  });
const offer = create(InspirationOfferSchema, {
  holdId: 'hold-1',
  die,
  d20: d20(9),
  attackKey: longsword.key,
  targetId: 'cap',
});

/** What `RollAttack` answers for a character that holds a die: the d20 and the question, no outcome. */
const held = (state: CombatState): AttackResult => ({
  encounter: state.encounter()!,
  roll: create(AttackRollSchema, { attackKey: longsword.key, d20: offer.d20 }),
  pending: undefined,
  offer,
});

/** What the answer resolves to: a hit, with the die in the total when it was used. */
const resolved = (state: CombatState, used: boolean): AttackResult => ({
  encounter: state.encounter()!,
  roll: create(AttackRollSchema, {
    attackKey: longsword.key,
    outcome: AttackOutcome.HIT,
    d20: d20(9, used ? 20 : 14),
    bonusDice: used
      ? [{ sourceKey: 'feature:bardic-inspiration-d6', sides: 8, face: 6, used: true }]
      : [],
  }),
  pending: undefined,
});

describe('AttackSheet: the Bardic Inspiration question of a held roll (PM-07c 12)', () => {
  const rollAttack = vi.fn();
  const answerBardicInspiration = vi.fn();
  const onHeld = vi.fn();

  function open(over: { diceMode?: DiceMode; inspiration?: AttackSheetData['inspiration'] } = {}) {
    const state = new CombatState();
    state.apply(encounter({ currentCombatantId: 't', combatants: [toren, cap] }));
    const data: AttackSheetData = {
      campaignId: 'camp',
      encounterId: 'enc',
      attackerId: 't',
      round: 3,
      attack: longsword,
      targets: [],
      diceMode: over.diceMode ?? DiceMode.APP,
      preference: DicePreference.APP,
      state,
      // The target is the mover of an opportunity, so the sheet opens at "Rolar".
      opportunity: over.inspiration
        ? undefined
        : { offerId: 'o', targetId: 'cap', targetLabel: 'Capitão Goblin', byMaster: false },
      inspiration: over.inspiration,
      onHeld,
    };
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [
        { provide: CombatClient, useValue: fakeCombatClient({ rollAttack }) },
        { provide: ResourceClient, useValue: { answerBardicInspiration } },
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
    return { fixture, el, state, settle, button };
  }

  beforeEach(() => {
    // The sheet scrolls its error into view; the test DOM has no scroll.
    Element.prototype.scrollTo = vi.fn();
    rollAttack.mockReset();
    answerBardicInspiration.mockReset();
    onHeld.mockReset();
  });

  it('shows the question after the d20 and before the result: the d20 is there, "Acertou", "Errou" and "Crítico" are not', async () => {
    const { el, state, settle, button } = open();
    rollAttack.mockImplementation(() => Promise.resolve(held(state)));
    button('Rolar no app')!.click();
    await settle();
    const text = flat(el);
    expect(text).toContain('Usar a Inspiração de Bardo (d8)?');
    expect(text).toContain('14');
    expect(text).toContain('1d20 (9) + 5 = 14');
    expect(text).toContain('O mestre ainda não disse se acertou.');
    expect(text).not.toMatch(/Acertou|Errou|Crítico/);
    expect(text).not.toContain('Voltar à sua vez');
    expect(button('Somar o d8 (Inspiração de Orla)')).toBeTruthy();
    expect(button('Guardar o dado')).toBeTruthy();
    // The page learns the hold, so reading the screen again does not open a second question.
    expect(onHeld).toHaveBeenCalledWith('hold-1');
  });

  it('never prints an armor class while the roll is held', async () => {
    const { el, state, settle, button } = open();
    rollAttack.mockImplementation(() => Promise.resolve(held(state)));
    button('Rolar no app')!.click();
    await settle();
    expect(flat(el)).not.toMatch(/\bCA\b/);
  });

  it('"Somar o d8" answers with use and the die rolled in the app, then shows the result with the die in it', async () => {
    const { el, state, settle, button } = open();
    rollAttack.mockImplementation(() => Promise.resolve(held(state)));
    answerBardicInspiration.mockImplementation(() => Promise.resolve(resolved(state, true)));
    button('Rolar no app')!.click();
    await settle();
    button('Somar o d8 (Inspiração de Orla)')!.click();
    await settle();
    expect(answerBardicInspiration).toHaveBeenCalledWith(
      'camp',
      'enc',
      'hold-1',
      true,
      { inApp: true },
      expect.any(String),
    );
    const text = flat(el);
    expect(text).not.toContain('Usar a Inspiração de Bardo');
    expect(text).toContain('Acertou');
    expect(text).toContain('Com o d8 da Inspiração de Bardo (+6).');
    expect(text).not.toContain('O mestre ainda não disse');
  });

  it('"Guardar o dado" answers without using it and shows the result', async () => {
    const { el, state, settle, button } = open();
    rollAttack.mockImplementation(() => Promise.resolve(held(state)));
    answerBardicInspiration.mockImplementation(() => Promise.resolve(resolved(state, false)));
    button('Rolar no app')!.click();
    await settle();
    button('Guardar o dado')!.click();
    await settle();
    expect(answerBardicInspiration).toHaveBeenCalledWith(
      'camp',
      'enc',
      'hold-1',
      false,
      null,
      expect.any(String),
    );
    expect(flat(el)).toContain('Acertou');
    expect(flat(el)).not.toContain('Com o d8');
  });

  it('with real dice the player types the d8, 1 to 8', async () => {
    const { fixture, el, state, settle, button } = open({ diceMode: DiceMode.PHYSICAL });
    // A physical d20 is typed too.
    rollAttack.mockImplementation(() => Promise.resolve(held(state)));
    answerBardicInspiration.mockImplementation(() => Promise.resolve(resolved(state, true)));
    const d20Field = el.querySelector('input.type__field') as HTMLInputElement;
    d20Field.value = '9';
    d20Field.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    button('Confirmar 9')!.click();
    await settle();
    expect(flat(el)).toContain('Usar a Inspiração de Bardo (d8)?');
    expect(flat(el)).toContain('1 a 8');
    const field = el.querySelector('input.type__field') as HTMLInputElement;
    field.value = '6';
    field.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    button('Confirmar 6')!.click();
    await settle();
    expect(answerBardicInspiration).toHaveBeenCalledWith(
      'camp',
      'enc',
      'hold-1',
      true,
      { face: 6 },
      expect.any(String),
    );
  });

  it('opens at the question when the screen is read again (the offer is on the combatant), with the d20 that was rolled', () => {
    const { el, button } = open({ inspiration: { offer, targetLabel: 'Capitão Goblin' } });
    const text = flat(el);
    expect(text).toContain('Usar a Inspiração de Bardo (d8)?');
    expect(text).toContain('Capitão Goblin');
    expect(text).toContain('1d20 (9) + 5 = 14');
    expect(text).not.toMatch(/Acertou|Errou/);
    expect(button('Somar o d8')).toBeTruthy();
  });

  it('shows the wait for the bard first, and the question when the window is answered and the combat says the d20 is kept', async () => {
    const { el, state, settle, button } = open();
    rollAttack.mockImplementation(() =>
      Promise.resolve({
        encounter: state.encounter()!,
        roll: create(AttackRollSchema, { attackKey: longsword.key, heldForReaction: true }),
        pending: undefined,
      }),
    );
    button('Rolar no app')!.click();
    await settle();
    expect(flat(el)).not.toContain('Usar a Inspiração de Bardo');
    // The window is answered: the combat now carries the question on the attacker.
    state.apply(
      encounter({
        currentCombatantId: 't',
        combatants: [{ ...toren, inspirationOffer: offer } as typeof toren, cap],
      }),
    );
    await settle();
    expect(flat(el)).toContain('Usar a Inspiração de Bardo (d8)?');
    expect(onHeld).toHaveBeenCalledWith('hold-1');
  });

  it('keeps the key of an answer that failed, so a retry never rolls the die twice', async () => {
    const { el, settle, button } = open({ inspiration: { offer, targetLabel: 'Capitão Goblin' } });
    answerBardicInspiration.mockRejectedValue(new ConnectError('down', Code.Unavailable));
    button('Somar o d8')!.click();
    await settle();
    button('Somar o d8')!.click();
    await settle();
    expect(flat(el.querySelector('[role=alert] p'))).toContain('o servidor não respondeu');
    const keys = answerBardicInspiration.mock.calls.map((c) => c[5] as string);
    expect(keys[0]).toBe(keys[1]);
  });

  it('says the held roll is gone in words, and stays at the question', async () => {
    const { el, settle, button } = open({ inspiration: { offer, targetLabel: 'Capitão Goblin' } });
    answerBardicInspiration.mockRejectedValue(new ConnectError('gone', Code.NotFound));
    button('Guardar o dado')!.click();
    await settle();
    expect(flat(el.querySelector('[role=alert] p'))).toContain('Esse combate não existe mais');
    expect(flat(el)).toContain('Usar a Inspiração de Bardo (d8)?');
  });

  it('says another action waits for the answer when the server refuses with INSPIRATION_PENDING', async () => {
    const { el, state, settle, button } = open();
    rollAttack.mockRejectedValue(
      new ConnectError('pending', Code.FailedPrecondition, undefined, [
        {
          desc: ResourceBlockedSchema,
          value: create(ResourceBlockedSchema, {
            reason: ResourceBlockedReason.INSPIRATION_PENDING,
          }),
        },
      ]),
    );
    expect(state.encounter()).not.toBeNull();
    button('Rolar no app')!.click();
    await settle();
    expect(flat(el.querySelector('[role=alert] p'))).toContain('Responda primeiro');
  });
});
