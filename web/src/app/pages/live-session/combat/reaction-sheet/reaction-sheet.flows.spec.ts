// The branches the other specs of the reaction sheet leave: the maneuver, the guards of every answer, the answers about
// the die that the server settles badly, the second step of the monk and the keys.
import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  EncounterBlockedReason,
  EncounterBlockedSchema,
  LuckyOfferSchema,
  OutsideInspirationOfferSchema,
  ReactionKind,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { CombatState } from '../../../../core/combat/combat-state';
import { encounter, reactionWindow } from '../../../../core/combat/combat-testing';
import { LUCKY_TRAIT } from '../../../../core/combat/lucky';
import { inspirationOffer } from '../../../../core/play/scene-testing';
import { ResourceClient } from '../../../../core/resources/resources-client';
import { RollAnimator } from '../../../../shared/roll-overlay/roll-animator';
import { settle } from '../../../../../test-settle';
import { ReactionSheet, type ReactionSheetData } from './reaction-sheet';
import { fakeCombatClient } from '../../../../core/combat/combat-client-testing';

const plain = (text: string | null | undefined) =>
  (text ?? '').replace(/ /g, ' ').replace(/\s+/g, ' ').trim();

const blocked = (reason: EncounterBlockedReason) =>
  new ConnectError('x', Code.FailedPrecondition, undefined, [
    { desc: EncounterBlockedSchema, value: create(EncounterBlockedSchema, { reason }) },
  ]);

function win(case_: string, value: object, over: object = {}) {
  return reactionWindow({
    id: 'w1',
    reactorId: 'me',
    reactorLabel: 'Brisa',
    reactorIsPlayer: true,
    prompt: { case: case_, value } as never,
    ...over,
  } as never);
}

const shield = (over: object = {}, slots = [{ level: 1, pact: false, free: 2 }]) =>
  win(
    'shield',
    { attackerLabel: 'Capitão Goblin', attackNamePt: 'Cimitarra', spellNamePt: 'Escudo', slots },
    { kind: ReactionKind.SHIELD, ...over },
  );

const maneuver = () =>
  win(
    'maneuverReduce',
    {
      attackerLabel: 'Capitão Goblin',
      attackNamePt: 'Cimitarra',
      damage: 9,
      options: [
        { key: 'parry', namePt: 'Aparar', dieSides: 8, flatBonus: 2, usesLeft: 3 },
        { key: 'riposte', namePt: 'Contra-ataque', dieSides: 10, flatBonus: -1, usesLeft: 1 },
      ],
    },
    { kind: ReactionKind.MANEUVER_REDUCE },
  );

const concentration = () =>
  win(
    'concentrationSave',
    {
      spellNamePt: 'Bênção',
      damageTaken: 8,
      dc: 10,
      sourceLabel: 'Cimitarra do Goblin',
      saveBonus: 2,
      bonusKnown: true,
    },
    { kind: ReactionKind.CONCENTRATION_SAVE },
  );

const deflect = (secondStep = false) =>
  win(
    secondStep ? 'deflectThrow' : 'deflectMissiles',
    secondStep
      ? { kiLeft: 2, normalRangeFt: 20, longRangeFt: 60 }
      : {
          attackerLabel: 'Goblin',
          attackNamePt: 'Arco curto',
          damage: 9,
          dieSides: 10,
          flatBonus: 8,
          dexMod: 3,
          monkLevel: 5,
        },
    { kind: ReactionKind.DEFLECT_MISSILES, secondStep, ...(secondStep ? { id: 'w2' } : {}) },
  );

/** The d20 of a save, with the faces and the total the overlay can tumble. */
const d20 = (face: number, rerolled: object[] = []) => ({
  diceCount: 1,
  diceSides: 20,
  faces: [face],
  modifier: 2,
  total: face + 2,
  rerolled,
});

interface Members {
  answerLucky(reroll: boolean, die: unknown): Promise<void>;
  answerDie(use: boolean, die: unknown): Promise<void>;
  use(die?: unknown): Promise<void>;
  throwBack(): void;
  keepMissile(): Promise<void>;
}

describe('ReactionSheet, the answers and the keys', () => {
  const resources = { answerOutsideLucky: vi.fn(), answerOutsideInspiration: vi.fn() };
  const api = {
    answerReaction: vi.fn(),
    resolveConcentrationSave: vi.fn(),
    markReactionRolling: vi.fn(),
    get: vi.fn(),
  };
  const animator = { play: vi.fn() };
  const closed: unknown[] = [];
  let opened = shield();

  function setup(window = shield(), others: ReturnType<typeof win>[] = []) {
    opened = window;
    const state = new CombatState();
    state.apply(encounter({ reactionWindows: [window, ...others] } as never));
    state.noteReactionOpened(window.id);
    const data: ReactionSheetData = {
      campaignId: 'camp',
      encounterId: 'enc',
      window,
      round: 2,
      usage: [
        { level: 1, total: 2, used: 0 },
        { level: 3, total: 2, used: 0 },
      ],
      pact: null,
      state,
      armorClass: 15,
      diceMode: DiceMode.PLAYERS_CHOOSE,
      preference: DicePreference.APP,
    };
    closed.length = 0;
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [
        { provide: MAT_DIALOG_DATA, useValue: data },
        {
          provide: MatDialogRef,
          useValue: { close: (r?: unknown) => closed.push(r ?? 'closed'), disableClose: true },
        },
        { provide: CombatClient, useValue: fakeCombatClient(api) },
        { provide: ResourceClient, useValue: resources },
        { provide: RollAnimator, useValue: animator },
      ],
    });
    const fixture = TestBed.createComponent(ReactionSheet);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const sheet = fixture.componentInstance as unknown as Members;
    return { fixture, el, state, sheet };
  }
  const button = (el: HTMLElement, label: string) =>
    Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
      plain(b.textContent).includes(label),
    )!;
  const key = (el: HTMLElement, name: string, target: HTMLElement = el) =>
    target.dispatchEvent(new KeyboardEvent('keydown', { key: name, bubbles: true }));
  const alert = (el: HTMLElement) => plain(el.querySelector('[role="alert"]')?.textContent);

  beforeEach(() => {
    for (const fn of [...Object.values(api), ...Object.values(resources), animator.play]) {
      fn.mockReset();
    }
    // The server answers with the combat, the window still open (and now marked).
    api.markReactionRolling.mockImplementation(async () =>
      encounter({ reactionWindows: [{ ...opened, rolling: true }] } as never),
    );
  });

  describe('the maneuver that cuts a hit', () => {
    it('lists the maneuvers with the first picked, and sends the one picked', async () => {
      api.answerReaction.mockResolvedValue({
        encounter: encounter(),
        result: {
          used: true,
          nextWindowId: '',
          result: {
            case: 'maneuverReduce',
            value: { namePt: 'Contra-ataque', damageBefore: 9, damageAfter: 0 },
          },
        },
      });
      const { fixture, el } = setup(maneuver());
      const radios = Array.from(el.querySelectorAll<HTMLInputElement>('input[type="radio"]'));
      expect(radios.map((r) => r.checked)).toEqual([true, false]);
      expect(plain(el.textContent)).toContain('d8 + 2 · 3 usos');
      expect(plain(el.textContent)).toContain('d10 + -1 · 1 usos');
      radios[1].click();
      fixture.detectChanges();
      button(el, 'Usar a manobra').click();
      await settle(fixture);
      expect(api.answerReaction.mock.calls[0][3]).toEqual({
        use: true,
        maneuverKey: 'riposte',
        die: { inApp: true },
      });
      expect(el.querySelector('h2')?.textContent).toContain('Contra-ataque usada');
    });

    it('rolls its die in the app when the player rolls in the app', async () => {
      api.answerReaction.mockResolvedValue({ encounter: encounter(), result: undefined });
      const { fixture, el } = setup(maneuver());
      button(el, 'Usar a manobra').click();
      await settle(fixture);
      expect(api.answerReaction.mock.calls[0][3]).toEqual({
        use: true,
        maneuverKey: 'parry',
        die: { inApp: true },
      });
      // An answer with no result still ends the question: the window closes with "Fechar".
      expect(button(el, 'Fechar')).toBeDefined();
    });
  });

  describe('the question', () => {
    it('says the master may decide when the reaction is not an optional one', () => {
      const { el } = setup(shield({ kind: ReactionKind.MASTER_CHECK }));
      expect(plain(el.textContent)).toContain(
        'Se você não responder, o mestre pode decidir por você.',
      );
    });

    it('opens with no question for a window the player has no sheet for', () => {
      const { el } = setup(win('masterCheck', {}, { kind: ReactionKind.MASTER_CHECK }));
      expect(el.querySelector('h2')?.textContent).toBe('Reação');
      expect(el.querySelector('.what')).toBeNull();
    });

    it('does not use a slot that is not free, and the arrows have nowhere to go with one', async () => {
      const { fixture, el, sheet } = setup(shield({}, [{ level: 1, pact: false, free: 0 }]));
      expect(button(el, 'Conjurar Escudo').getAttribute('aria-disabled')).toBe('true');
      await sheet.use();
      key(el, 'ArrowDown', button(el, 'Deixar passar'));
      await settle(fixture);
      expect(api.answerReaction).not.toHaveBeenCalled();
    });

    it('ignores the arrows when the question has no list, and the other keys always', () => {
      const { el } = setup(concentration());
      key(el, 'ArrowDown', button(el, 'Rolar no app'));
      key(el, 'a');
      expect(closed).toEqual([]);
    });

    it('answers once while an answer is in the air', async () => {
      api.answerReaction.mockResolvedValue({ encounter: encounter(), result: undefined });
      const { fixture, el } = setup();
      button(el, 'Conjurar Escudo').click();
      button(el, 'Deixar passar').click();
      await settle(fixture);
      expect(api.answerReaction).toHaveBeenCalledTimes(1);
    });
  });

  describe('Queda Suave', () => {
    it('puts a creature back in the list after taking it out', () => {
      const { fixture, el } = setup(
        win(
          'featherFall',
          {
            falling: [
              { combatantId: 't', label: 'Toren', ally: true, distanceFt: 15 },
              { combatantId: 'g', label: 'Goblin 2', ally: false },
            ],
            maxTargets: 5,
            fallFt: 20,
            slotOptions: [{ level: 1, pact: false, free: 2 }],
          },
          { kind: ReactionKind.FEATHER_FALL },
        ),
      );
      expect(plain(el.textContent)).toContain('até cinco');
      const first = el.querySelector<HTMLInputElement>('input[type="checkbox"]')!;
      first.click();
      fixture.detectChanges();
      expect(first.checked).toBe(false);
      first.click();
      fixture.detectChanges();
      expect(first.checked).toBe(true);
    });
  });

  describe('the concentration save', () => {
    const typed = (el: HTMLElement, fixture: { detectChanges(): void }, face: number, at = 0) => {
      const field = el.querySelectorAll<HTMLInputElement>('input[type="text"]')[at];
      field.value = String(face);
      field.dispatchEvent(new Event('input'));
      fixture.detectChanges();
    };

    it('holds the answer until the d4 an effect adds are typed', async () => {
      api.resolveConcentrationSave.mockRejectedValueOnce(
        new ConnectError('the roll takes 1 more die(s)', Code.InvalidArgument),
      );
      const { fixture, el } = setup(concentration());
      button(el, 'Digitar o resultado').click();
      fixture.detectChanges();
      typed(el, fixture, 12);
      button(el, 'Confirmar').click();
      await settle(fixture);
      // Only the d20 is typed: the d4 is missing, so nothing is sent a second time.
      typed(el, fixture, 12, 1);
      button(el, 'Confirmar').click();
      await settle(fixture);
      expect(api.resolveConcentrationSave).toHaveBeenCalledTimes(1);
      expect(alert(el)).toContain('Digite o resultado do d4 antes de confirmar.');
    });

    it('shows a refusal of any other kind', async () => {
      api.resolveConcentrationSave.mockRejectedValue(new Error('boom'));
      const { fixture, el } = setup(concentration());
      button(el, 'Rolar no app').click();
      await settle(fixture);
      expect(el.querySelector('[role="alert"]')).not.toBeNull();
      expect(el.querySelector('app-extra-dice')).toBeNull();
    });

    it('answers once while the answer is in the air', async () => {
      api.resolveConcentrationSave.mockResolvedValue({ encounter: encounter(), result: undefined });
      const { fixture, el } = setup(concentration());
      button(el, 'Rolar no app').click();
      button(el, 'Deixar o mestre rolar por mim').click();
      await settle(fixture);
      expect(api.resolveConcentrationSave).toHaveBeenCalledTimes(1);
    });
  });

  describe('the answers about the die', () => {
    const asked = async (extra: object) => {
      api.resolveConcentrationSave.mockResolvedValue({
        encounter: encounter({ reactionWindows: [concentration()] } as never),
        result: undefined,
        ...extra,
      });
      const s = setup(concentration());
      button(s.el, 'Rolar no app').click();
      await settle(s.fixture);
      return s;
    };
    const luckyOffer = create(LuckyOfferSchema, { holdId: 'h1', luckyIndex: 0, d20: d20(1) });

    it('ignores an answer when nothing is asked, or one is already on the way', async () => {
      const { sheet } = setup(concentration());
      await sheet.answerLucky(true, null);
      await sheet.answerDie(true, null);
      expect(resources.answerOutsideLucky).not.toHaveBeenCalled();
      expect(resources.answerOutsideInspiration).not.toHaveBeenCalled();
    });

    it('tumbles the d20 that Lucky rolled again, and asks about the Bardic Inspiration die on it', async () => {
      const s = await asked({ lucky: luckyOffer });
      resources.answerOutsideLucky.mockResolvedValue({
        result: {
          case: 'concentrationSave',
          value: {
            encounter: encounter({ reactionWindows: [concentration()] } as never),
            inspirationOffer: create(OutsideInspirationOfferSchema, {
              holdId: 'h1',
              sides: 8,
              fromName: 'Orla',
              d20: d20(14, [{ index: 0, from: 1, to: 14, sourceKey: LUCKY_TRAIT }]),
            }),
          },
        },
      });
      button(s.el, 'Rolar de novo').click();
      await settle(s.fixture);
      expect(animator.play).toHaveBeenCalledTimes(1);
      expect(animator.play.mock.calls[0][0]).toMatchObject({
        label: 'Sortudo',
        line: '1 → 14',
      });
      expect(plain(s.el.textContent)).toContain('Usar a Inspiração de Bardo (d8)?');
    });

    it('does not tumble a d20 that was typed', async () => {
      const s = await asked({ lucky: luckyOffer });
      resources.answerOutsideLucky.mockResolvedValue({
        result: {
          case: 'concentrationSave',
          value: {
            encounter: encounter({ reactionWindows: [concentration()] } as never),
            inspirationOffer: create(OutsideInspirationOfferSchema, {
              holdId: 'h1',
              sides: 8,
              d20: d20(14),
            }),
          },
        },
      });
      button(s.el, 'Ficar com o 1').click();
      await settle(s.fixture);
      expect(animator.play).not.toHaveBeenCalled();
    });

    it('says so when the server settles Lucky without the save', async () => {
      const s = await asked({ lucky: luckyOffer });
      resources.answerOutsideLucky.mockResolvedValue({ result: { case: undefined } });
      button(s.el, 'Ficar com o 1').click();
      await settle(s.fixture);
      expect(s.el.querySelector('[role="alert"]')).not.toBeNull();
      expect(plain(s.el.textContent)).toContain('Tirou 1: rolar de novo?');
    });

    it('says so when the server settles the die without the save', async () => {
      const s = await asked({ offer: inspirationOffer(7, 2) });
      resources.answerOutsideInspiration.mockResolvedValue({
        result: { case: 'concentrationSave', value: { encounter: encounter() } },
      });
      button(s.el, 'Guardar o dado').click();
      await settle(s.fixture);
      expect(s.el.querySelector('[role="alert"]')).not.toBeNull();
    });

    it('keeps the question when the server fails the answer about the die', async () => {
      const s = await asked({ offer: inspirationOffer(7, 2) });
      resources.answerOutsideInspiration.mockRejectedValue(
        new ConnectError('down', Code.Unavailable),
      );
      button(s.el, 'Guardar o dado').click();
      await settle(s.fixture);
      expect(s.el.querySelector('[role="alert"]')).not.toBeNull();
      expect(plain(s.el.textContent)).toContain('Usar a Inspiração de Bardo (d8)?');
    });
  });

  describe('a refusal out of order', () => {
    it('goes on when the combat cannot be read again, or reads nothing', async () => {
      api.answerReaction.mockRejectedValue(blocked(EncounterBlockedReason.NOT_YOUR_TURN_TO_ANSWER));
      api.get.mockResolvedValueOnce(undefined).mockRejectedValueOnce(new Error('offline'));
      const { fixture, el } = setup();
      button(el, 'Conjurar Escudo').click();
      await settle(fixture);
      button(el, 'Conjurar Escudo').click();
      await settle(fixture);
      expect(api.get).toHaveBeenCalledTimes(2);
      expect(alert(el)).toContain('não espera a sua resposta agora');
    });
  });

  describe('the keys', () => {
    it('Escape in the roll goes back to the question', async () => {
      api.answerReaction.mockRejectedValue(blocked(EncounterBlockedReason.REACTION_NEEDS_ROLL));
      const { fixture, el } = setup(
        win(
          'counterspell',
          {
            casterLabel: 'Mago 1',
            distanceFt: 30,
            slotOptions: [{ level: 3, pact: false, free: 2 }],
          },
          { kind: ReactionKind.COUNTERSPELL },
        ),
      );
      button(el, 'Usar Contramágica').click();
      await settle(fixture);
      expect(button(el, 'Voltar')).toBeDefined();
      key(el, 'Escape');
      await settle(fixture);
      expect(button(el, 'Deixar passar')).toBeDefined();
      expect(closed).toEqual([]);
    });

    it('Escape on the result closes the sheet', async () => {
      api.answerReaction.mockResolvedValue({
        encounter: encounter(),
        result: {
          used: true,
          nextWindowId: '',
          result: { case: 'shield', value: { stopped: true } },
        },
      });
      const { fixture, el } = setup();
      button(el, 'Conjurar Escudo').click();
      await settle(fixture);
      key(el, 'Escape');
      expect(closed).toEqual(['closed']);
    });

    it('Escape when the window is gone closes the sheet', async () => {
      const { fixture, el, state } = setup();
      state.apply(encounter({ reactionWindows: [], revision: 5 } as never));
      await settle(fixture);
      key(el, 'Escape');
      expect(closed).toEqual(['closed']);
    });
  });

  describe("the monk's second step", () => {
    const caught = (nextWindowId: string) => ({
      encounter: encounter(),
      result: {
        used: true,
        nextWindowId,
        result: {
          case: 'deflectMissiles',
          value: {
            reduction: undefined,
            damageBefore: 9,
            damageAfter: 0,
            caught: true,
            throwBackAvailable: true,
          },
        },
      },
    });

    it('Escape keeps the missile', async () => {
      api.answerReaction.mockResolvedValueOnce(caught('w2'));
      api.answerReaction.mockResolvedValue({ encounter: encounter(), result: undefined });
      const { fixture, el } = setup(deflect(), [deflect(true)]);
      button(el, 'Usar Defletir Projéteis').click();
      await settle(fixture);
      key(el, 'Escape');
      await settle(fixture);
      expect(closed).toEqual(['closed']);
    });

    it('tells a refusal to keep the missile and stays', async () => {
      api.answerReaction.mockResolvedValueOnce(caught('w2'));
      api.answerReaction.mockRejectedValue(new Error('boom'));
      const { fixture, el } = setup(deflect(), [deflect(true)]);
      button(el, 'Usar Defletir Projéteis').click();
      await settle(fixture);
      button(el, 'Guardar a flecha').click();
      await settle(fixture);
      expect(el.querySelector('[role="alert"]')).not.toBeNull();
      expect(closed).toEqual([]);
    });

    it('closes without a question when there is no second window', async () => {
      api.answerReaction.mockResolvedValueOnce(caught(''));
      const { fixture, el, sheet } = setup(deflect());
      button(el, 'Usar Defletir Projéteis').click();
      await settle(fixture);
      sheet.throwBack();
      expect(closed).toEqual([]);
      await sheet.keepMissile();
      expect(closed).toEqual(['closed']);
    });
  });
});
