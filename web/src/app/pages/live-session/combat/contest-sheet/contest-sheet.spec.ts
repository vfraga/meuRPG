import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  CombatantKind,
  CombatantSide,
  HeldRollKind,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  CheckOptionSchema,
  ContestAttackOptionKind,
  ContestAttackOptionSchema,
  ContestBlockedReason,
  ContestBlockedSchema,
  ContestKind,
  ContestPurpose,
  ContestSkill,
  ContestStatus,
  ContestTargetReason,
  ContestTargetSchema,
  ContestWaitFor,
  ContestWinner,
  RollModeKind,
  RollNoteSchema,
  ShoveBlockedReason,
  ShoveChoiceSchema,
  ShoveOutcome,
} from '../../../../../gen/meurpg/play/v1/contest_types_pb';
import { CreatureSize } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { heldDie } from '../../../../core/held-rolls/held-roll-testing';
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
import { ContestSheet, type ContestSheetData } from './contest-sheet';

const plain = (s: string | null | undefined) =>
  (s ?? '').replace(/ /g, ' ').replace(/\s+/g, ' ').trim();

const target = (id: string, size: CreatureSize, eligible = true) =>
  create(ContestTargetSchema, {
    combatantId: id,
    size,
    distanceFt: 5,
    eligible,
    reason: eligible ? ContestTargetReason.UNSPECIFIED : ContestTargetReason.TOO_BIG,
  });

function grappleOption(kind = ContestAttackOptionKind.GRAPPLE) {
  return create(ContestAttackOptionSchema, {
    kind,
    replacesAttack: true,
    enabled: true,
    targets: [
      target('h', CreatureSize.MEDIUM),
      target('g', CreatureSize.SMALL),
      target('x', CreatureSize.HUGE, false),
    ],
    rollOption: create(CheckOptionSchema, { modifier: 5, known: true, mode: RollModeKind.NORMAL }),
  });
}

function table() {
  return encounter({
    combatants: [
      combatant({
        id: 't',
        label: 'Toren',
        kind: CombatantKind.PLAYER,
        side: CombatantSide.PARTY,
        size: CreatureSize.MEDIUM,
      } as never),
      combatant({ id: 'h', label: 'Hobgoblin' }),
      combatant({ id: 'g', label: 'Goblin 1' }),
      combatant({ id: 'x', label: 'Gigante da colina' }),
      combatant({ id: 'b', label: 'Brisa', kind: CombatantKind.PLAYER, side: CombatantSide.PARTY }),
    ],
  });
}

describe('ContestSheet', () => {
  let api: FakeContestClient;
  let close: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    Element.prototype.scrollTo = vi.fn() as never;
  });

  function setup(over: Partial<ContestSheetData> = {}, enc = table()) {
    api = new FakeContestClient();
    api.encounterAnswer = enc;
    close = vi.fn();
    const state = new CombatState();
    state.encounter.set(enc);
    const contests = new ContestState();
    const data: ContestSheetData = {
      campaignId: 'c',
      encounterId: 'enc',
      initiatorId: 't',
      purpose: ContestPurpose.GRAPPLE,
      attack: grappleOption(),
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
    const fixture = TestBed.createComponent(ContestSheet);
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
    const text = () => textOf(el);
    const mark = async () => {
      el.querySelector<HTMLInputElement>('input[type="checkbox"]')!.click();
      await settle();
    };
    return { fixture, el, button, settle, text, mark, state, contests, data };
  }

  describe('the choice of the target (board W7-Xa 2)', () => {
    it('says what the grapple needs and lists who is at reach, with the size and the distance', () => {
      const { el, text } = setup();
      expect(plain(el.querySelector('h2')?.textContent)).toBe('Agarrar');
      expect(text()).toContain('Ação · substitui um ataque');
      expect(text()).toContain(
        'Você precisa de uma mão livre. Alvo: no máximo um tamanho acima do seu (Médio), ao alcance.',
      );
      expect(text()).toContain('Tenho uma mão livre');
      expect(text()).toContain('Confirme antes de rolar.');
      expect(text()).toContain('Hobgoblin');
      expect(text()).toContain('Médio · a 1,5 m');
      expect(text()).toContain('Goblin 1');
      expect(text()).toContain('Pequeno · a 1,5 m');
      expect(el.querySelector('ol')?.getAttribute('aria-label')).toBe('Passos da disputa');
      expect(Array.from(el.querySelectorAll('.steps__name')).map((n) => n.textContent)).toEqual([
        'Alvo',
        'Disputa',
        'Resultado',
      ]);
    });

    it('lists a creature that is too big dashed, with the reason written, and not choosable', () => {
      const { el } = setup();
      const radios = Array.from(el.querySelectorAll<HTMLInputElement>('input[type="radio"]'));
      expect(radios).toHaveLength(3);
      expect(radios[2].disabled).toBe(true);
      expect(radios[2].closest('label')?.classList).toContain('card--off');
      expect(textOf(radios[2].closest('label')!)).toContain(
        'Enorme · a 1,5 m Grande demais: no máximo um tamanho acima do seu.',
      );
      // The first creature that can be a target starts chosen.
      expect(radios[0].checked).toBe(true);
    });

    it('never lists a creature out of reach', () => {
      const out = create(ContestAttackOptionSchema, {
        ...grappleOption(),
        targets: [
          target('h', CreatureSize.MEDIUM),
          create(ContestTargetSchema, {
            combatantId: 'g',
            reason: ContestTargetReason.OUT_OF_REACH,
          }),
        ],
      });
      const { el } = setup({ attack: out });
      expect(el.querySelectorAll('input[type="radio"]')).toHaveLength(1);
    });

    it('keeps "Rolar a disputa" off until the hand is marked, and says why', async () => {
      const { el, button, mark, text } = setup();
      const go = button('Rolar a disputa')!;
      expect(go.getAttribute('aria-disabled')).toBe('true');
      expect(go.classList).toContain('foot__btn--off');
      expect(el.querySelector('#contest-why')?.textContent).toContain(
        'Confirme que tem uma mão livre',
      );
      go.click();
      expect(text()).not.toContain('Seu teste de');
      await mark();
      expect(button('Rolar a disputa')!.getAttribute('aria-disabled')).toBeNull();
      expect(el.querySelector('#contest-why')).toBeNull();
    });

    it('asks for no free hand on a shove (SRD 5.1: only a grapple needs one)', () => {
      const { el, text, button } = setup({ purpose: ContestPurpose.SHOVE });
      expect(text()).not.toContain('Tenho uma mão livre');
      expect(text()).not.toContain('mão livre');
      expect(el.querySelector('#contest-why')?.textContent ?? '').not.toContain('mão livre');
      expect(button('Rolar a disputa')).not.toBeNull();
    });

    it('closes with "Voltar" and starts nothing', () => {
      const { button } = setup();
      button('Voltar')!.click();
      expect(close).toHaveBeenCalledWith(undefined);
      expect(api.calls).toEqual([]);
    });
  });

  describe('the roll and the wait (boards W7-Xa 3 and 3b)', () => {
    async function toRoll() {
      const s = setup();
      await s.mark();
      s.button('Rolar a disputa')!.click();
      await s.settle();
      return s;
    }

    it('asks for the roll of the chosen target: Força (Atletismo) and the modifier', async () => {
      const { el, text, button } = await toRoll();
      expect(plain(el.querySelector('h2')?.textContent)).toBe('Agarrar o Hobgoblin');
      expect(text()).toContain('Disputa');
      expect(text()).toContain('Seu teste de Força (Atletismo): +5.');
      expect(button('Rolar no app')).toBeTruthy();
      expect(button('Digitar o resultado')).toBeTruthy();
    });

    it('says the mode of the roll and why before it, for what the server says changes it', async () => {
      const poisoned = create(ContestAttackOptionSchema, {
        ...grappleOption(),
        rollOption: create(CheckOptionSchema, {
          modifier: 5,
          mode: RollModeKind.DISADVANTAGE,
          notes: [
            create(RollNoteSchema, { kind: 'poisoned', labelPt: 'Envenenado', advantage: false }),
          ],
        }),
      });
      const s = setup({ attack: poisoned });
      await s.mark();
      s.button('Rolar a disputa')!.click();
      await s.settle();
      expect(s.text()).toContain('Desvantagem Desvantagem: Envenenado');
      expect(s.button('Rolar 2d20 no app')).toBeTruthy();
    });

    it('rolls in the app once, with the request the contract names and one key', async () => {
      const { button, settle } = await toRoll();
      api.contest = contestView({ initiatorRoll: checkRoll() });
      button('Rolar no app')!.click();
      await settle();
      expect(api.started).toHaveLength(1);
      expect(api.started[0].input).toMatchObject({
        campaignId: 'c',
        encounterId: 'enc',
        initiatorId: 't',
        targetId: 'h',
        purpose: ContestPurpose.GRAPPLE,
        kind: ContestKind.CONTEST,
        skill: ContestSkill.ATHLETICS,
        die: { inApp: true },
      });
      expect(api.started[0].key).toMatch(/^[0-9a-f-]{36}$/);
    });

    it('shows the own d20 and total, and waits for the master with the combat’s own wait', async () => {
      const { el, button, settle, text, state } = await toRoll();
      api.contest = contestView({ initiatorRoll: checkRoll() });
      api.encounterAnswer = encounter({
        combatants: table().combatants,
        reactionWait: {
          titlePt: 'Esperando o mestre',
          detailPt: 'O turno continua quando ele responder.',
        },
      } as never);
      button('Rolar no app')!.click();
      await settle();
      expect(el.querySelector('.roll__box')?.textContent).toBe('15');
      expect(el.querySelector('.roll__total')?.textContent).toBe('20');
      expect(plain(el.querySelector('.roll__formula')?.textContent)).toBe(
        '1d20 (15) + 5 · Atletismo',
      );
      expect(textOf(el.querySelector('[data-testid="contest-wait"]')!)).toBe(
        'Esperando o mestre. O Hobgoblin escolhe Atletismo ou Acrobacia e rola.',
      );
      expect(text()).toContain(
        '“Fechar a folha” só a esconde: a disputa continua esperando o mestre, e a folha volta com o resultado.',
      );
      expect(button('Fechar a folha')).toBeTruthy();
      // The answer's combat went to the page's copy.
      expect(state.encounter()?.reactionWait?.titlePt).toBe('Esperando o mestre');
      // Nothing of the other side: no DC, no total but the own.
      expect(text()).not.toMatch(/CD |Atletismo \(\+\d|escape/);
    });

    it('says "Esperando Brisa" for a player, with no footnote about the master', async () => {
      const { el, button, settle, text, data } = setup({
        attack: create(ContestAttackOptionSchema, {
          ...grappleOption(),
          targets: [target('b', CreatureSize.MEDIUM)],
        }),
      });
      await (async () => {
        el.querySelector<HTMLInputElement>('input[type="checkbox"]')!.click();
        await settle();
        button('Rolar a disputa')!.click();
        await settle();
      })();
      api.contest = contestView({
        defenderId: 'b',
        initiatorRoll: checkRoll(),
        waitingFor: ContestWaitFor.PLAYER,
        waitingCombatantId: 'b',
      });
      button('Rolar no app')!.click();
      await settle();
      expect(plain(el.querySelector('h2')?.textContent)).toBe('Agarrar Brisa');
      expect(textOf(el.querySelector('[data-testid="contest-wait"]')!)).toBe(
        'Esperando Brisa. Brisa escolhe Atletismo ou Acrobacia e rola.',
      );
      expect(text()).not.toContain('Fechar a folha” só a esconde');
      expect(data.contests.contests()).toHaveLength(1);
    });

    it('sends the typed d20 as the faces, and the pair when the roll has advantage', async () => {
      const adv = create(ContestAttackOptionSchema, {
        ...grappleOption(),
        rollOption: create(CheckOptionSchema, { modifier: 5, mode: RollModeKind.ADVANTAGE }),
      });
      const { el, button, mark, settle } = setup({ attack: adv });
      await mark();
      button('Rolar a disputa')!.click();
      await settle();
      button('Digitar o resultado')!.click();
      await settle();
      const fields = Array.from(el.querySelectorAll<HTMLInputElement>('input[type="text"]'));
      expect(fields).toHaveLength(2);
      fields[0].value = '15';
      fields[0].dispatchEvent(new Event('input'));
      fields[1].value = '8';
      fields[1].dispatchEvent(new Event('input'));
      await settle();
      api.contest = contestView({
        initiatorRoll: checkRoll({ faces: [15, 8], mode: RollModeKind.ADVANTAGE }),
      });
      button('Confirmar')!.click();
      await settle();
      expect(api.started[0].input.die).toEqual({ faces: [15, 8] });
      expect(el.textContent).toContain('Vantagem');
      expect(el.textContent).toContain('vale');
      expect(el.textContent).toContain('descartado');
    });

    it('asks the question when the d20 is kept for one and opens no contest until it is answered', async () => {
      const { el, button, settle } = await toRoll();
      api.held = heldDie(15, 5, 8, { kind: HeldRollKind.CONTEST });
      button('Rolar no app')!.click();
      await settle();
      expect(api.started).toHaveLength(1);
      expect(el.querySelector('app-held-roll-prompt')).not.toBeNull();
      expect(button('Rolar no app')).toBeUndefined();
    });

    it('keeps the key when the same roll is sent again after a lost answer, and a new one for other values', async () => {
      const { button, settle } = await toRoll();
      api.error = new ConnectError('lost', Code.Unavailable);
      button('Rolar no app')!.click();
      await settle();
      button('Rolar no app')!.click();
      await settle();
      expect(api.started).toHaveLength(2);
      expect(api.started[1].key).toBe(api.started[0].key);
    });

    it('says what was refused, in words, and stays where it is', async () => {
      const { el, button, settle, text } = await toRoll();
      api.error = new ConnectError('x', Code.FailedPrecondition, undefined, [
        {
          desc: ContestBlockedSchema,
          value: create(ContestBlockedSchema, { reason: ContestBlockedReason.TARGET_TOO_BIG }),
        },
      ]);
      button('Rolar no app')!.click();
      await settle();
      expect(el.querySelector('[role="alert"]')?.textContent).toContain(
        'Grande demais: no máximo um tamanho acima do seu.',
      );
      expect(text()).toContain('Seu teste de Força (Atletismo)');
    });

    it('hides on "Fechar a folha" and hands the page the contest to bring back', async () => {
      const { button, settle } = await toRoll();
      api.contest = contestView({ id: 'ct9', initiatorRoll: checkRoll() });
      button('Rolar no app')!.click();
      await settle();
      button('Fechar a folha')!.click();
      expect(close).toHaveBeenCalledWith({ contestId: 'ct9' });
    });
  });

  describe('the result', () => {
    async function waiting() {
      const s = setup();
      await s.mark();
      s.button('Rolar a disputa')!.click();
      await s.settle();
      api.contest = contestView({ initiatorRoll: checkRoll() });
      s.button('Rolar no app')!.click();
      await s.settle();
      return s;
    }

    it('a won grapple: the target is Agarrado and how it is held (board 3, right)', async () => {
      const { el, contests, fixture, settle, text, button } = await waiting();
      contests.applyContest(
        contestView({
          initiatorRoll: checkRoll(),
          status: ContestStatus.RESOLVED,
          winner: ContestWinner.INITIATOR,
        }),
      );
      fixture.detectChanges();
      await settle();
      expect(text()).toContain('Você venceu a disputa. O Hobgoblin está Agarrado.');
      expect(text()).toContain(
        'Você mantém o agarrão enquanto quiser (solte sem gastar ação). Se você se mover, leva a criatura junto, com o deslocamento pela metade.',
      );
      expect(
        Array.from(el.querySelectorAll('.steps__item')).at(2)?.getAttribute('aria-current'),
      ).toBe('step');
      expect(button('Fechar')).toBeTruthy();
      expect(document.activeElement?.textContent).toContain('Fechar');
    });

    it('a lost grapple says only that it did not work, never the other total', async () => {
      const { contests, fixture, settle, text } = await waiting();
      contests.applyContest(
        contestView({
          initiatorRoll: checkRoll(),
          status: ContestStatus.RESOLVED,
          winner: ContestWinner.DEFENDER,
        }),
      );
      fixture.detectChanges();
      await settle();
      expect(text()).toContain('Você não conseguiu agarrar o Hobgoblin.');
      expect(text()).not.toMatch(/venceu/);
    });
  });

  describe('a shove (board W7-Xb 7)', () => {
    function shove(choice: { push: boolean; blocked?: ShoveBlockedReason }) {
      const s = setup({
        purpose: ContestPurpose.SHOVE,
        contestId: 'ct1',
        attack: undefined,
      });
      s.contests.applyContest(
        contestView({
          purpose: ContestPurpose.SHOVE,
          status: ContestStatus.AWAITING_OUTCOME,
          winner: ContestWinner.INITIATOR,
          youChoose: true,
          initiatorRoll: checkRoll(),
          shoveChoice: create(ShoveChoiceSchema, {
            proneAvailable: true,
            pushAvailable: choice.push,
            pushBlocked: choice.blocked ?? ShoveBlockedReason.UNSPECIFIED,
          }),
        }),
      );
      s.fixture.detectChanges();
      return s;
    }

    it('offers Derrubar and Empurrar 1,5 m to the one that won, and confirms the choice', async () => {
      const { el, text, button, settle } = shove({ push: true });
      expect(plain(el.querySelector('h2')?.textContent)).toBe('Empurrar o Hobgoblin');
      expect(text()).toContain('Resultado');
      expect(text()).toContain('Você venceu a disputa. Escolha o que fazer com o Hobgoblin.');
      expect(text()).toContain(
        'Derrubar O Hobgoblin fica Derrubado: só rasteja. Ataque corpo a corpo contra a criatura a até 1,5 m tem vantagem; de mais longe, desvantagem.',
      );
      expect(text()).toContain('Empurrar 1,5 m Para longe de você, em linha reta.');
      const push = el.querySelectorAll<HTMLInputElement>('input[name="shove"]')[1];
      push.click();
      await settle();
      button('Confirmar')!.click();
      await settle();
      expect(api.shoves).toHaveLength(1);
      expect(api.shoves[0]).toMatchObject({ contestId: 'ct1', outcome: ShoveOutcome.PUSH });
      expect(api.shoves[0].key).toMatch(/^[0-9a-f-]{36}$/);
    });

    it('starts on Derrubar, and sends it', async () => {
      const { button, settle } = shove({ push: true });
      button('Confirmar')!.click();
      await settle();
      expect(api.shoves[0].outcome).toBe(ShoveOutcome.PRONE);
    });

    it('disables the push with the reason when the square behind is a wall (right)', () => {
      const { el, text } = shove({ push: false, blocked: ShoveBlockedReason.WALL });
      const push = el.querySelectorAll<HTMLInputElement>('input[name="shove"]')[1];
      expect(push.disabled).toBe(true);
      expect(push.closest('label')?.classList).toContain('card--off');
      expect(text()).toContain(
        'Empurrar 1,5 m Para longe de você. Há uma parede na casa de trás: o alvo não sai do lugar.',
      );
    });

    it('says what the shove did once it is chosen', async () => {
      const { el, contests, fixture, settle, text } = shove({ push: true });
      contests.applyContest(
        contestView({
          purpose: ContestPurpose.SHOVE,
          status: ContestStatus.RESOLVED,
          winner: ContestWinner.INITIATOR,
          shoveOutcome: ShoveOutcome.PRONE,
          initiatorRoll: checkRoll(),
        }),
      );
      fixture.detectChanges();
      await settle();
      expect(text()).toContain('Você venceu a disputa. O Hobgoblin está Derrubado.');
      expect(el.querySelector('input[name="shove"]')).toBeNull();
    });

    it('says a lost shove with its own verb', async () => {
      const s = setup({ purpose: ContestPurpose.SHOVE, contestId: 'ct1', attack: undefined });
      s.contests.applyContest(
        contestView({
          purpose: ContestPurpose.SHOVE,
          status: ContestStatus.RESOLVED,
          winner: ContestWinner.DEFENDER,
        }),
      );
      s.fixture.detectChanges();
      expect(s.text()).toContain('Você não conseguiu empurrar o Hobgoblin.');
    });
  });

  describe('an escape (board W7-Xb 5)', () => {
    const escapeOptions = [
      skillOption({ skill: ContestSkill.ATHLETICS, modifier: 0 }),
      skillOption({ skill: ContestSkill.ACROBATICS, modifier: 7 }),
    ];

    function escape() {
      return setup({
        initiatorId: 'b',
        purpose: ContestPurpose.ESCAPE,
        attack: undefined,
        escape: escapeOptions,
        holderId: 'h',
      });
    }

    it('says the action is used, against whom the test is, and offers the two skills with their modifiers', () => {
      const { el, text } = escape();
      expect(plain(el.querySelector('h2')?.textContent)).toBe('Escapar');
      expect(text()).toContain('Ação');
      expect(text()).toContain(
        'Você usa a ação. Escolha Atletismo ou Acrobacia: o teste é contra o Atletismo do Hobgoblin.',
      );
      expect(text()).toContain('Destreza (Acrobacia) Seu modificador: +7');
      expect(text()).toContain('Força (Atletismo) Seu modificador: +0');
      expect(Array.from(el.querySelectorAll('.steps__name')).map((n) => n.textContent)).toEqual([
        'Teste',
        'Resultado',
      ]);
      // The skill that rolls best starts chosen.
      const radios = el.querySelectorAll<HTMLInputElement>('input[name="contest-skill"]');
      expect(radios[0].checked).toBe(true);
    });

    it('rolls with the chosen skill, spends nothing here, and says "Continua Agarrado. A ação foi gasta." when lost', async () => {
      const { el, button, settle, text, contests } = escape();
      api.contest = contestView({
        purpose: ContestPurpose.ESCAPE,
        initiatorId: 'b',
        status: ContestStatus.RESOLVED,
        winner: ContestWinner.DEFENDER,
        initiatorRoll: checkRoll({
          skill: ContestSkill.ACROBATICS,
          skillKey: 'skill:acrobatics',
          faces: [9],
          modifier: 7,
          total: 16,
        }),
      });
      button('Rolar no app')!.click();
      await settle();
      expect(api.started[0].input).toMatchObject({
        purpose: ContestPurpose.ESCAPE,
        targetId: '',
        skill: ContestSkill.ACROBATICS,
        initiatorId: 'b',
      });
      expect(el.querySelector('.roll__box')?.textContent).toBe('9');
      expect(el.querySelector('.roll__total')?.textContent).toBe('16');
      expect(plain(el.querySelector('.roll__formula')?.textContent)).toBe(
        '1d20 (9) + 7 · Acrobacia',
      );
      expect(text()).toContain('Continua Agarrado. A ação foi gasta.');
      expect(contests.contests()).toHaveLength(1);
    });

    it('a win says "Você se soltou", and an escape from a fixed DC says "Você escapou" without the DC', async () => {
      const { button, settle, text } = escape();
      api.contest = contestView({
        purpose: ContestPurpose.ESCAPE,
        kind: ContestKind.ESCAPE_DC,
        status: ContestStatus.RESOLVED,
        winner: ContestWinner.INITIATOR,
        defenderId: 'h',
        initiatorRoll: checkRoll({ faces: [16], modifier: 7, total: 23 }),
      });
      button('Rolar no app')!.click();
      await settle();
      expect(text()).toContain('Você escapou.');
      expect(text()).toContain(
        'Você usa a ação: um teste de Atletismo ou Acrobacia contra a força do Hobgoblin.',
      );
      expect(text()).not.toMatch(/CD /);
    });

    it('waits for the one that holds, with the combat’s own wait', async () => {
      const { button, settle, text, state } = escape();
      api.contest = contestView({ purpose: ContestPurpose.ESCAPE, initiatorId: 'b' });
      api.encounterAnswer = encounter({
        combatants: table().combatants,
        reactionWait: {
          titlePt: 'Esperando o mestre',
          detailPt: 'O turno continua quando ele responder.',
        },
      } as never);
      button('Rolar no app')!.click();
      await settle();
      expect(text()).toContain('Esperando o mestre. O turno continua quando ele responder.');
      expect(state.encounter()?.reactionWait?.titlePt).toBe('Esperando o mestre');
    });
  });
});
