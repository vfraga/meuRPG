import { TestBed } from '@angular/core/testing';
import { Code, ConnectError } from '@connectrpc/connect';
import { create } from '@bufbuild/protobuf';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { CombatantKind } from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  ContestKind,
  ContestPurpose,
  ContestSkill,
  ContestStatus,
  ContestWaitFor,
  ContestWinner,
  RollModeKind,
  ShoveBlockedReason,
  ShoveChoiceSchema,
  ShoveOutcome,
  type ContestView,
} from '../../../../../gen/meurpg/play/v1/contest_types_pb';
import { CombatState } from '../../../../core/combat/combat-state';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { ContestClient } from '../../../../core/combat/contest-client';
import { ContestState } from '../../../../core/combat/contest-state';
import {
  FakeContestClient,
  checkRoll,
  contestView,
  skillOption,
  textOf,
} from '../../../../core/combat/contest-testing';
import { ContestCard } from './contest-card';

const table = () =>
  encounter({
    combatants: [
      combatant({ id: 't', label: 'Toren', kind: CombatantKind.PLAYER }),
      combatant({ id: 'b', label: 'Brisa', kind: CombatantKind.PLAYER }),
      combatant({ id: 'h', label: 'Hobgoblin' }),
    ],
  });

const answerOptions = [
  skillOption({ skill: ContestSkill.ATHLETICS, modifier: 1 }),
  skillOption({ skill: ContestSkill.ACROBATICS, modifier: 3, suggested: true }),
];

const toren = checkRoll();

function setup(contest: ContestView, input: Record<string, unknown> = {}) {
  const api = new FakeContestClient();
  const e = table();
  api.encounterAnswer = encounter({ ...e, revision: 9 } as never);
  const state = new CombatState();
  state.encounter.set(e);
  const contests = new ContestState();
  contests.applyContest(contest);
  TestBed.configureTestingModule({
    providers: [{ provide: ContestClient, useValue: api.as() }],
  });
  const fixture = TestBed.createComponent(ContestCard);
  const ref = fixture.componentRef;
  ref.setInput('contest', contest);
  ref.setInput('encounter', e);
  ref.setInput('campaignId', 'camp');
  ref.setInput('state', state);
  ref.setInput('contests', contests);
  ref.setInput('diceMode', DiceMode.PLAYERS_CHOOSE);
  ref.setInput('preference', DicePreference.APP);
  for (const [k, v] of Object.entries(input)) {
    ref.setInput(k, v);
  }
  const said: string[] = [];
  fixture.componentInstance.said.subscribe((s) => said.push(s));
  fixture.detectChanges();
  const el = fixture.nativeElement as HTMLElement;
  const button = (name: string) =>
    Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
      textOf(b).includes(name),
    );
  const settle = async () => {
    for (let i = 0; i < 3; i++) {
      await fixture.whenStable();
      fixture.detectChanges();
    }
  };
  return { api, fixture, el, button, settle, said, state, contests };
}

const npcDefender = () =>
  contestView({ initiatorRoll: toren, initiatorBonusKnown: true, youAnswer: true, answerOptions });

describe('ContestCard, the master answers a contest', () => {
  describe('an NPC defends', () => {
    it("reads Toren's total, the suggested skill and what each rolls", () => {
      const { el } = setup(npcDefender());
      const text = textOf(el);
      expect(textOf(el.querySelector('h2')!)).toBe('Toren tenta agarrar o Hobgoblin');
      expect(text).toContain('Disputa');
      expect(text).toContain('Toren: Força (Atletismo) 15 + 5 = 20');
      expect(text).toContain(
        'O alvo escolhe a habilidade (SRD). Sugerido: a de maior modificador.',
      );
      const radios = Array.from(el.querySelectorAll<HTMLInputElement>('input[type="radio"]'));
      expect(radios.map((r) => textOf(r.closest('label')!))).toEqual([
        'Atletismo (+1)',
        'Acrobacia (+3) Sugerida',
      ]);
      expect(radios[1].checked).toBe(true);
    });

    it('rolls in the app for the Hobgoblin with the chosen skill and its own key', async () => {
      const { api, el, button, settle, said, state, contests } = setup(npcDefender());
      el.querySelectorAll<HTMLInputElement>('input[type="radio"]')[0].click();
      await settle();
      api.contest = contestView({
        status: ContestStatus.RESOLVED,
        winner: ContestWinner.INITIATOR,
        initiatorRoll: toren,
        defenderRoll: checkRoll({ faces: [9], modifier: 1, total: 10 }),
      });
      button('Rolar pelo Hobgoblin')!.click();
      await settle();
      expect(api.responded).toHaveLength(1);
      expect(api.responded[0].input).toMatchObject({
        campaignId: 'camp',
        encounterId: 'enc',
        contestId: 'ct1',
        skill: ContestSkill.ATHLETICS,
        die: { inApp: true },
      });
      expect(api.responded[0].key).toEqual(expect.any(String));
      expect(state.encounter()?.revision).toBe(9);
      expect(contests.contest('ct1')?.status).toBe(ContestStatus.RESOLVED);
      expect(said).toEqual(['Hobgoblin: Força (Atletismo) 9 + 1 = 10. Toren vence.']);
    });

    it('sends the typed d20', async () => {
      const { api, el, button, settle, fixture } = setup(npcDefender());
      button('Digitar o resultado')!.click();
      await settle();
      const field = el.querySelector<HTMLInputElement>('input[type="text"]')!;
      field.value = '9';
      field.dispatchEvent(new Event('input'));
      fixture.detectChanges();
      button('Confirmar')!.click();
      await settle();
      expect(api.responded[0].input).toMatchObject({
        skill: ContestSkill.ACROBATICS,
        die: { faces: [9] },
      });
    });

    it('asks for the pair when the skill has advantage', () => {
      const { button } = setup(
        contestView({
          initiatorRoll: toren,
          youAnswer: true,
          answerOptions: [skillOption({ mode: 2, modifier: 1, suggested: true })],
        }),
      );
      expect(button('Rolar 2d20 pelo Hobgoblin')).toBeTruthy();
    });

    it('shows the refusal of the server and lets the master try again', async () => {
      const { api, el, button, settle } = setup(npcDefender());
      api.error = new ConnectError('already', Code.FailedPrecondition);
      button('Rolar pelo Hobgoblin')!.click();
      await settle();
      expect(el.querySelector('[role="alert"]')).toBeTruthy();
      expect(button('Rolar pelo Hobgoblin')!.disabled).toBe(false);
    });

    it('"Encerrar disputa" ends it with no result, once', async () => {
      const { api, button, settle, said, state } = setup(npcDefender());
      button('Encerrar disputa')!.click();
      await settle();
      expect(api.closed).toEqual([{ contestId: 'ct1', key: expect.any(String) }]);
      expect(state.encounter()?.revision).toBe(9);
      expect(said).toEqual(['Disputa encerrada.']);
    });
  });

  describe('a player defends', () => {
    const waiting = () =>
      contestView({
        initiatorId: 'h',
        defenderId: 'b',
        initiatorRoll: checkRoll({ faces: [17], modifier: 1, total: 18 }),
        youAnswer: true,
        waitingFor: ContestWaitFor.PLAYER,
        waitingCombatantId: 'b',
        answerOptions,
      });

    it('waits for the player, offers to roll for them and ends the contest', () => {
      const { el, button } = setup(waiting());
      const text = textOf(el);
      expect(textOf(el.querySelector('h2')!)).toBe('Hobgoblin tenta agarrar Brisa');
      expect(text).toContain('Hobgoblin: Força (Atletismo) 17 + 1 = 18');
      expect(text).toContain('Esperando Brisa. O jogador está respondendo no celular.');
      expect(button('Rolar por Brisa')).toBeTruthy();
      expect(button('Encerrar disputa')).toBeTruthy();
      expect(el.querySelector('input[type="radio"]')).toBeNull();
    });

    it('"Rolar por Brisa" opens the form and rolls for her', async () => {
      const { api, button, settle } = setup(waiting());
      button('Rolar por Brisa')!.click();
      await settle();
      button('Rolar por Brisa')!.click();
      await settle();
      expect(api.responded[0].input).toMatchObject({
        contestId: 'ct1',
        skill: ContestSkill.ACROBATICS,
        die: { inApp: true },
      });
    });

    it('a player who left the roll to the master has the skill fixed and no choice', () => {
      const { el, button } = setup(
        contestView({
          initiatorId: 'h',
          defenderId: 'b',
          initiatorRoll: checkRoll({ total: 18 }),
          youAnswer: true,
          deferred: true,
          deferredSkill: ContestSkill.ACROBATICS,
          answerOptions: [answerOptions[1]],
        }),
      );
      expect(textOf(el)).toContain('Brisa deixou o mestre rolar o teste.');
      expect(el.querySelector('input[type="radio"]')).toBeNull();
      expect(button('Rolar por Brisa')).toBeTruthy();
    });
  });

  describe("a won shove of an NPC's", () => {
    const won = (choice = {}) =>
      contestView({
        purpose: ContestPurpose.SHOVE,
        initiatorId: 'h',
        defenderId: 'b',
        status: ContestStatus.AWAITING_OUTCOME,
        winner: ContestWinner.INITIATOR,
        youChoose: true,
        shoveChoice: create(ShoveChoiceSchema, {
          proneAvailable: true,
          pushAvailable: true,
          ...choice,
        }),
      });

    it('asks Derrubar or Empurrar 1,5 m', () => {
      const { el, button } = setup(won());
      const text = textOf(el);
      expect(textOf(el.querySelector('h2')!)).toBe('Hobgoblin tenta empurrar Brisa');
      expect(text).toContain('Hobgoblin vence a disputa. Escolha o que fazer com Brisa.');
      expect(text).toContain('Derrubar Brisa fica Derrubado: só rasteja.');
      expect(text).toContain('Empurrar 1,5 m Para longe do Hobgoblin, em linha reta.');
      expect(button('Confirmar')).toBeTruthy();
    });

    it('knocks prone with the key', async () => {
      const { api, button, settle, said } = setup(won());
      button('Confirmar')!.click();
      await settle();
      expect(api.shoves).toEqual([
        { contestId: 'ct1', outcome: ShoveOutcome.PRONE, key: expect.any(String) },
      ]);
      expect(said).toEqual(['Brisa ficou Derrubado.']);
    });

    it('pushes, and a blocked square says why and sends "não sai do lugar"', async () => {
      const free = setup(won());
      free.el.querySelectorAll<HTMLInputElement>('input[type="radio"]')[1].click();
      await free.settle();
      free.button('Confirmar')!.click();
      await free.settle();
      expect(free.api.shoves[0].outcome).toBe(ShoveOutcome.PUSH);
      TestBed.resetTestingModule();
      const blocked = setup(won({ pushAvailable: false, pushBlocked: ShoveBlockedReason.WALL }));
      expect(textOf(blocked.el)).toContain(
        'Para longe do Hobgoblin. Há uma parede na casa de trás: o alvo não sai do lugar.',
      );
      blocked.el.querySelectorAll<HTMLInputElement>('input[type="radio"]')[1].click();
      await blocked.settle();
      blocked.button('Confirmar')!.click();
      await blocked.settle();
      expect(blocked.api.shoves[0].outcome).toBe(ShoveOutcome.STAYS);
    });
  });

  it('never answers with a kind that is not a contest: an escape against a creature has one skill', () => {
    const { el } = setup(
      contestView({
        purpose: ContestPurpose.ESCAPE,
        kind: ContestKind.CONTEST,
        initiatorId: 'b',
        defenderId: 'h',
        youAnswer: true,
        initiatorRoll: checkRoll({ faces: [9], modifier: 7, total: 16 }),
        answerOptions: [skillOption({ modifier: 1 })],
      }),
    );
    expect(textOf(el.querySelector('h2')!)).toBe('Brisa tenta escapar');
    expect(textOf(el)).toContain('O teste de quem agarra');
    expect(el.querySelectorAll('input[type="radio"]')).toHaveLength(1);
  });

  describe('the natural mark (information only)', () => {
    const marks = (faces: number[], mode?: RollModeKind) => {
      const { el } = setup(
        contestView({
          initiatorRoll: checkRoll({ faces, mode, modifier: 5, total: 25 }),
          initiatorBonusKnown: true,
          youAnswer: true,
          answerOptions,
        }),
      );
      return Array.from(el.querySelectorAll('[data-testid="natural-mark"]'), (m) => textOf(m));
    };

    it.each([
      ['a natural 20', [20], undefined, ['20 natural']],
      ['a natural 1', [1], undefined, ['1 natural']],
      ['another face', [15], undefined, []],
      ['advantage keeping the 20', [20, 4], RollModeKind.ADVANTAGE, ['20 natural']],
      ['disadvantage dropping the 20', [20, 4], RollModeKind.DISADVANTAGE, []],
    ])("beside Toren's total: %s", (_name, faces, mode, expected) => {
      expect(marks(faces, mode)).toEqual(expected);
    });
  });
});
