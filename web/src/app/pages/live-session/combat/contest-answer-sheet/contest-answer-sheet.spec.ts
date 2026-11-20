import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { Code, ConnectError } from '@connectrpc/connect';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { CombatantKind, CombatantSide } from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  ContestPurpose,
  ContestSkill,
  ContestStatus,
  ContestWinner,
  RollModeKind,
  RollNoteSchema,
  ShoveOutcome,
} from '../../../../../gen/meurpg/play/v1/contest_types_pb';
import { create } from '@bufbuild/protobuf';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { CombatState } from '../../../../core/combat/combat-state';
import { ContestClient } from '../../../../core/combat/contest-client';
import { ContestState } from '../../../../core/combat/contest-state';
import {
  FakeContestClient,
  checkRoll,
  contestView,
  skillOption,
  textOf,
} from '../../../../core/combat/contest-testing';
import { ContestAnswerSheet, type ContestAnswerData } from './contest-answer-sheet';

const acrobatics = skillOption({ skill: ContestSkill.ACROBATICS, modifier: 7 });
const athletics = skillOption({ skill: ContestSkill.ATHLETICS, modifier: 0 });

function table() {
  return encounter({
    combatants: [
      combatant({ id: 'h', label: 'Hobgoblin' }),
      combatant({ id: 'b', label: 'Brisa', kind: CombatantKind.PLAYER, side: CombatantSide.PARTY }),
      combatant({ id: 't', label: 'Toren', kind: CombatantKind.PLAYER, side: CombatantSide.PARTY }),
    ],
  });
}

describe('ContestAnswerSheet (board W7-Xb 4)', () => {
  let api: FakeContestClient;
  let close: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    Element.prototype.scrollTo = vi.fn() as never;
  });

  function setup(
    contest = contestView({
      initiatorId: 'h',
      defenderId: 'b',
      youAnswer: true,
      answerOptions: [athletics, acrobatics],
    }),
    over: Partial<ContestAnswerData> = {},
  ) {
    api = new FakeContestClient();
    api.encounterAnswer = table();
    close = vi.fn();
    const state = new CombatState();
    state.encounter.set(table());
    const contests = new ContestState();
    contests.applyContest(contest);
    const data: ContestAnswerData = {
      campaignId: 'c',
      encounterId: 'enc',
      contestId: contest.id,
      diceMode: DiceMode.PLAYERS_CHOOSE,
      preference: DicePreference.APP,
      state,
      contests,
      ...over,
    };
    TestBed.configureTestingModule({
      providers: [
        { provide: ContestClient, useValue: api.as() },
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close, disableClose: false } },
      ],
    });
    const fixture = TestBed.createComponent(ContestAnswerSheet);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const settle = async () => {
      for (let i = 0; i < 3; i++) {
        await fixture.whenStable();
        fixture.detectChanges();
      }
    };
    const button = (name: string) =>
      Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
        b.textContent?.includes(name),
      );
    return { fixture, el, button, settle, text: () => textOf(el), contests, state };
  }

  it('asks the target to choose Força (Atletismo) or Destreza (Acrobacia) against a creature’s test', () => {
    const { el, text } = setup();
    expect(textOf(el.querySelector('h2')!)).toBe('Hobgoblin tenta agarrar você');
    expect(text()).toContain('Disputa');
    expect(text()).toContain(
      'Você escolhe Força (Atletismo) ou Destreza (Acrobacia) contra o teste dele (SRD, Grappling).',
    );
    expect(Array.from(el.querySelectorAll('.steps__name')).map((n) => n.textContent)).toEqual([
      'Escolha',
      'Resultado',
    ]);
  });

  it('lists the skills with the modifier of each, the best first and chosen, and the three ways to roll', () => {
    const { el, text, button } = setup();
    const radios = Array.from(el.querySelectorAll<HTMLInputElement>('input[type="radio"]'));
    expect(radios.map((r) => textOf(r.closest('label')!))).toEqual([
      'Destreza (Acrobacia) Seu modificador: +7',
      'Força (Atletismo) Seu modificador: +0',
    ]);
    expect(radios[0].checked).toBe(true);
    expect(button('Rolar no app')).toBeTruthy();
    expect(button('Digitar o resultado')).toBeTruthy();
    expect(button('Deixar o mestre rolar por mim')).toBeTruthy();
    // Nothing of the other side: no total, no skill of the NPC.
    expect(text()).not.toMatch(/CD |total dele/);
  });

  it('says it against a player’s test without the rule’s citation', () => {
    const { text, el } = setup(
      contestView({
        initiatorId: 't',
        defenderId: 'b',
        youAnswer: true,
        answerOptions: [athletics, acrobatics],
      }),
    );
    expect(textOf(el.querySelector('h2')!)).toBe('Toren tenta agarrar você');
    expect(text()).toContain('contra o teste dele.');
    expect(text()).not.toContain('SRD');
  });

  it('says a shove with its own words', () => {
    const { el, text } = setup(
      contestView({
        purpose: ContestPurpose.SHOVE,
        initiatorId: 'h',
        defenderId: 'b',
        youAnswer: true,
        answerOptions: [athletics, acrobatics],
      }),
    );
    expect(textOf(el.querySelector('h2')!)).toBe('Hobgoblin tenta empurrar você');
    expect(text()).toContain('(SRD, Shoving a Creature)');
  });

  it('answers with the chosen skill and the roll in the app, under one key', async () => {
    const { button, settle } = setup();
    button('Rolar no app')!.click();
    await settle();
    expect(api.responded).toHaveLength(1);
    expect(api.responded[0].input).toEqual({
      campaignId: 'c',
      encounterId: 'enc',
      contestId: 'ct1',
      skill: ContestSkill.ACROBATICS,
      die: { inApp: true },
    });
    expect(api.responded[0].key).toMatch(/^[0-9a-f-]{36}$/);
  });

  it('answers with the other skill when it is picked, and the typed d20', async () => {
    const { el, button, settle } = setup();
    el.querySelectorAll<HTMLInputElement>('input[type="radio"]')[1].click();
    await settle();
    button('Digitar o resultado')!.click();
    await settle();
    const field = el.querySelector<HTMLInputElement>('input[type="text"]')!;
    field.value = '14';
    field.dispatchEvent(new Event('input'));
    await settle();
    button('Confirmar')!.click();
    await settle();
    expect(api.responded[0].input).toMatchObject({
      skill: ContestSkill.ATHLETICS,
      die: { faces: [14] },
    });
  });

  it('follows the campaign’s dice mode: forced app offers only "Rolar no app" and sends no typed value', async () => {
    const { el, button, settle } = setup(undefined, { diceMode: DiceMode.APP });
    expect(button('Digitar o resultado')).toBeUndefined();
    expect(el.querySelector('input[type="text"]')).toBeNull();
    button('Rolar no app')!.click();
    await settle();
    expect(api.responded[0].input).toMatchObject({ die: { inApp: true } });
  });

  it('follows the campaign’s dice mode: forced physical offers only the typed entry and sends the face, never the total', async () => {
    const { el, button, settle } = setup(undefined, { diceMode: DiceMode.PHYSICAL });
    expect(button('Rolar no app')).toBeUndefined();
    const field = el.querySelector<HTMLInputElement>('input[type="text"]')!;
    field.value = '14';
    field.dispatchEvent(new Event('input'));
    await settle();
    button('Confirmar')!.click();
    await settle();
    // Acrobacia is +7, chosen first: 21 is never sent.
    expect(api.responded[0].input).toMatchObject({
      skill: ContestSkill.ACROBATICS,
      die: { faces: [14] },
    });
  });

  it('asks for the two d20 when a skill rolls with advantage or disadvantage', async () => {
    const poisoned = skillOption({
      skill: ContestSkill.ACROBATICS,
      modifier: 7,
      mode: RollModeKind.DISADVANTAGE,
      notes: [
        create(RollNoteSchema, { kind: 'poisoned', labelPt: 'Envenenado', advantage: false }),
      ],
    });
    const { el, button, settle, text } = setup(
      contestView({
        initiatorId: 'h',
        defenderId: 'b',
        youAnswer: true,
        answerOptions: [athletics, poisoned],
      }),
    );
    expect(text()).toContain('Desvantagem: Envenenado');
    button('Digitar o resultado')!.click();
    await settle();
    expect(el.querySelectorAll('input[type="text"]')).toHaveLength(2);
  });

  it('leaves the roll to the master with "Deixar o mestre rolar por mim", saying the skill chosen', async () => {
    const { button, settle } = setup();
    button('Deixar o mestre rolar por mim')!.click();
    await settle();
    expect(api.responded[0].input.die).toBeNull();
    expect(api.responded[0].input.skill).toBe(ContestSkill.ACROBATICS);
  });

  it('sends the same key again after a lost answer', async () => {
    const { button, settle } = setup();
    api.error = new ConnectError('lost', Code.Unavailable);
    button('Rolar no app')!.click();
    await settle();
    button('Rolar no app')!.click();
    await settle();
    expect(api.responded[1].key).toBe(api.responded[0].key);
    expect(textOf(document.body)).not.toBeNull();
  });

  it('waits for the master once the roll is left to them, without the other side’s total', async () => {
    const { contests, fixture, settle, text, button } = setup();
    contests.applyContest(
      contestView({
        initiatorId: 'h',
        defenderId: 'b',
        deferred: true,
        youAnswer: false,
        answerOptions: [athletics, acrobatics],
      }),
    );
    fixture.detectChanges();
    await settle();
    expect(text()).toContain('Esperando o mestre. O mestre rola por você.');
    expect(button('Fechar a folha')).toBeTruthy();
    button('Fechar a folha')!.click();
    expect(close).toHaveBeenCalledWith({ contestId: 'ct1' });
  });

  it('a lost defence shows the own roll and "Você perdeu a disputa", with the condition (right of the board)', async () => {
    const { el, contests, fixture, settle, text } = setup();
    contests.applyContest(
      contestView({
        initiatorId: 'h',
        defenderId: 'b',
        status: ContestStatus.RESOLVED,
        winner: ContestWinner.INITIATOR,
        defenderRoll: checkRoll({
          skill: ContestSkill.ACROBATICS,
          faces: [8],
          modifier: 7,
          total: 15,
        }),
      }),
    );
    fixture.detectChanges();
    await settle();
    expect(el.querySelector('.roll__box')?.textContent).toBe('8');
    expect(el.querySelector('.roll__total')?.textContent).toBe('15');
    expect(textOf(el.querySelector('.roll__formula')!)).toBe('1d20 (8) + 7 · Acrobacia');
    expect(text()).toContain(
      'Você perdeu a disputa. O Hobgoblin agarrou você: você está Agarrado, com deslocamento 0.',
    );
    expect(document.activeElement?.textContent).toContain('Fechar');
  });

  it('a won defence, a tie and a lost shove are said without a total', async () => {
    const { contests, fixture, settle, text } = setup();
    const resolved = (over: object) =>
      contests.applyContest(
        contestView({
          initiatorId: 'h',
          defenderId: 'b',
          status: ContestStatus.RESOLVED,
          defenderRoll: checkRoll(),
          ...over,
        }),
      );
    resolved({ winner: ContestWinner.DEFENDER });
    fixture.detectChanges();
    await settle();
    expect(text()).toContain('Você venceu a disputa. O Hobgoblin não conseguiu agarrar você.');
    resolved({ winner: ContestWinner.TIE });
    fixture.detectChanges();
    await settle();
    expect(text()).toContain('Empate: nada muda.');
    resolved({
      purpose: ContestPurpose.SHOVE,
      winner: ContestWinner.INITIATOR,
      shoveOutcome: ShoveOutcome.PRONE,
    });
    fixture.detectChanges();
    await settle();
    expect(text()).toContain('Você perdeu a disputa. O Hobgoblin derrubou você.');
  });
});
