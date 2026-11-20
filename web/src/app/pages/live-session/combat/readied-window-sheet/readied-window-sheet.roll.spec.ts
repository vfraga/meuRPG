import { create } from '@bufbuild/protobuf';
import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { Code, ConnectError } from '@connectrpc/connect';
import { timestampFromMs } from '@bufbuild/protobuf/wkt';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  AttackOutcome,
  AttackRollSchema,
  AttackTargetsSchema,
  CombatantState,
  GetTurnOptionsResponseSchema,
  PendingDamageSchema,
  PendingDamageStatus,
  ReactionBusy,
  ReactionKind,
  ReadiedActionSchema,
  ReadiedKind,
  ReadiedStatus,
  SpellTargetsSchema,
  TargetInReachSchema,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { AdvantageSourceSchema, RollMode } from '../../../../../gen/meurpg/play/v1/combat_rolls_pb';
import {
  AttackKind,
  AttackOptionSchema,
  SpellDetailsSchema,
  SpellHitPointEffectKind,
  TurnOptionsSchema,
} from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { fakeCombatClient } from '../../../../core/combat/combat-client-testing';
import { CombatState as State } from '../../../../core/combat/combat-state';
import { combatant, encounter, reactionWindow } from '../../../../core/combat/combat-testing';
import { SpellCatalog } from '../../../../core/combat/spell-catalog';
import { ReadiedWindowSheet, type ReadiedWindowSheetData } from './readied-window-sheet';

const flat = (n: Element | null | undefined) =>
  n?.textContent?.replace(/\s+/g, ' ').replace(/ /g, ' ').trim() ?? '';

const window = reactionWindow({
  id: 'w1',
  kind: ReactionKind.READIED,
  reactorId: 'tor',
  reactorLabel: 'Tor',
  reactorIsPlayer: true,
  forYou: true,
} as never);

const sleep = create(SpellDetailsSchema, {
  spell: { key: 'spell:sleep', level: 1 },
  hitPointEffect: {
    kind: SpellHitPointEffectKind.POOL,
    poolDiceCount: 5,
    poolDiceSides: 8,
    poolDicePerLevel: 2,
  },
} as never);

describe('ReadiedWindowSheet: the roll step of the release (#938, boards 938-01 to 938-07)', () => {
  const answerReaction = vi.fn();
  const markReactionRolling = vi.fn();
  const rollDamage = vi.fn();
  const close = vi.fn();

  interface Opts {
    mode?: DiceMode;
    preference?: DicePreference;
    rollMode?: RollMode;
    sources?: object[];
    row?: Record<string, unknown>;
    window?: Record<string, unknown>;
    spellTargets?: Record<string, unknown>;
  }

  function setup(opts: Opts = {}) {
    const row = create(ReadiedActionSchema, {
      id: 'r1',
      triggerText: 'Quando o zumbi passar pela porta',
      kind: ReadiedKind.ATTACK,
      actionKey: 'equipment:longsword',
      actionNamePt: 'Espada longa',
      targetCombatantId: 'z1',
      status: ReadiedStatus.WAITING,
      ...opts.row,
    } as never);
    const win = { ...window, ...opts.window } as typeof window;
    const state = new State();
    state.apply(
      encounter({
        combatants: [
          combatant({ id: 'tor', label: 'Tor', mine: true, readied: row }),
          combatant({ id: 'z1', label: 'Zumbi 1' }),
        ],
        reactionWindows: [win],
      } as never),
    );
    const data: ReadiedWindowSheetData = {
      campaignId: 'c',
      encounterId: 'enc',
      window: win,
      round: 2,
      readied: row,
      reactorLabel: 'Tor',
      state,
      diceMode: opts.mode ?? DiceMode.PHYSICAL,
      preference: opts.preference ?? DicePreference.PHYSICAL,
    };
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close, disableClose: false } },
        { provide: SpellCatalog, useValue: { details: async () => sleep } },
        {
          provide: CombatClient,
          useValue: fakeCombatClient({
            answerReaction,
            markReactionRolling,
            rollDamage,
            turnOptions: async () =>
              create(GetTurnOptionsResponseSchema, {
                options: create(TurnOptionsSchema, {
                  attacks: [
                    create(AttackOptionSchema, {
                      enabled: true,
                      attack: {
                        key: 'equipment:longsword',
                        namePt: 'Espada longa',
                        kind: AttackKind.WEAPON,
                        melee: true,
                        rangeFt: 5,
                        attackBonus: 6,
                      },
                    } as never),
                  ],
                }),
                spellTargets: opts.spellTargets
                  ? [create(SpellTargetsSchema, opts.spellTargets as never)]
                  : [],
                attackTargets: [
                  create(AttackTargetsSchema, {
                    attackKey: 'equipment:longsword',
                    targets: [
                      create(TargetInReachSchema, {
                        combatantId: 'z1',
                        label: 'Zumbi 1',
                        state: CombatantState.UNHURT,
                        distanceFt: 5,
                        rollMode: opts.rollMode ?? RollMode.NORMAL,
                        sources: (opts.sources ?? []).map((s) =>
                          create(AdvantageSourceSchema, s as never),
                        ),
                      }),
                    ],
                  }),
                ],
              }),
            moveOptions: async () => ({ movementLeftDft: 300 }),
          }),
        },
      ],
    });
    const fixture = TestBed.createComponent(ReadiedWindowSheet);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const settle = async () => {
      await fixture.whenStable();
      fixture.detectChanges();
    };
    const button = (text: string) =>
      Array.from(el.querySelectorAll('button')).find((b) => flat(b).includes(text))!;
    const field = () =>
      el.querySelector('input[type=number], input[inputmode=numeric]') as HTMLInputElement;
    const type = async (n: string, i = 0) => {
      const f = el.querySelectorAll('input')[i] as HTMLInputElement;
      f.value = n;
      f.dispatchEvent(new Event('input'));
      await settle();
    };
    return { el, settle, button, state, field, type };
  }

  const marked = (busy: ReactionBusy | undefined) =>
    encounter({
      revision: 3,
      combatants: [combatant({ id: 'tor', label: 'Tor', mine: true })],
      reactionWindows: [
        {
          ...window,
          rolling: busy !== undefined,
          busy,
          waitBeganAt: timestampFromMs(Date.now()),
        },
      ],
    } as never);

  beforeEach(() => {
    answerReaction.mockReset();
    rollDamage.mockReset();
    markReactionRolling.mockReset();
    markReactionRolling.mockResolvedValue(marked(ReactionBusy.DICE));
    close.mockReset();
  });

  async function toStepTwo(opts: Opts = {}) {
    const s = setup(opts);
    await s.settle();
    s.button('Atacar o Zumbi 1').click();
    await s.settle();
    return s;
  }

  describe('step 2 of a held weapon attack at a table that types its dice', () => {
    it('does not answer on "Atacar": it opens the step, marks the window as the dice and keeps the header', async () => {
      const { el, settle, button, field } = await toStepTwo();
      await settle();
      expect(answerReaction).not.toHaveBeenCalled();
      expect(markReactionRolling).toHaveBeenCalledWith('c', 'enc', 'w1', false, ReactionBusy.DICE);
      expect(flat(el.querySelector('h2'))).toBe('Ação preparada');
      expect(flat(el.querySelector('[data-roll]'))).toContain('Role 1d20 para Espada longa (+6)');
      expect(document.activeElement).toBe(field());
      expect(button('Voltar')).toBeTruthy();
      expect(flat(el.querySelector('[data-testid=readied-clock]'))).toMatch(
        /^Você está rolando: a mesa espera \d+ s\. Depois disso, passa sozinho\.$/,
      );
    });

    it('answers with the number typed, once, on "Confirmar"', async () => {
      answerReaction.mockResolvedValue({
        encounter: encounter({ revision: 5 } as never),
        attackRoll: create(AttackRollSchema, {
          outcome: AttackOutcome.MISS,
          d20: { total: 9 },
        } as never),
      });
      const { settle, button, type } = await toStepTwo();
      await type('14');
      button('Confirmar 14').click();
      await settle();
      expect(answerReaction).toHaveBeenCalledTimes(1);
      expect(answerReaction.mock.calls[0]).toContainEqual({ use: true, die: { typed: 14 } });
    });

    it('says the number is out of range in danger colours and keeps "Confirmar" off', async () => {
      const { el, settle, button, type } = await toStepTwo();
      await type('21');
      await settle();
      expect(flat(el.querySelector('[role=alert]'))).toContain('Digite um número de 1 a 20');
      button('Confirmar').click();
      await settle();
      expect(answerReaction).not.toHaveBeenCalled();
    });

    it('asks the pair of d20 with the mode tag and the sentence of the server, with no mode chooser', async () => {
      const { el } = await toStepTwo({
        rollMode: RollMode.DISADVANTAGE,
        sources: [{ effect: RollMode.DISADVANTAGE, textPt: 'Zumbi a mais de 1,5 m' }],
      });
      expect(flat(el.querySelector('.sources'))).toContain('Desvantagem');
      expect(flat(el.querySelector('.sources'))).toContain('Zumbi a mais de 1,5 m');
      expect(el.querySelector('input[type=radio][name*=mode]')).toBeNull();
      expect(el.querySelectorAll('[data-roll] input[type=number], [data-roll] input').length).toBe(
        2,
      );
    });
  });

  describe('"Voltar"', () => {
    it('returns to step 1 with the pick kept, sends nothing, takes the mark off and puts the focus on the use button', async () => {
      const { el, settle, button } = await toStepTwo();
      markReactionRolling.mockResolvedValue(marked(undefined));
      button('Voltar').click();
      await settle();
      expect(answerReaction).not.toHaveBeenCalled();
      expect(markReactionRolling).toHaveBeenLastCalledWith('c', 'enc', 'w1', true);
      expect(el.querySelector('[data-roll]')).toBeNull();
      expect(flat(el.querySelector('input[type=radio]:checked')?.closest('label'))).toContain(
        'Zumbi 1',
      );
      expect(document.activeElement).toBe(button('Atacar o Zumbi 1'));
    });

    it('does not restart the one wait: the second visit to step 2 marks again but the clock goes on from the first mark', async () => {
      const { el, settle, button } = await toStepTwo({
        window: { waitBeganAt: timestampFromMs(Date.now() - 48_000), waitedMs: 48_000n },
      });
      expect(flat(el.querySelector('[data-testid=readied-clock]'))).toMatch(
        /1 min 1\d s|1 min 2\d s/,
      );
      button('Voltar').click();
      await settle();
      button('Atacar o Zumbi 1').click();
      await settle();
      expect(flat(el.querySelector('[data-testid=readied-clock]'))).toMatch(/1 min \d+ s/);
    });
  });

  describe('the sheet of the second device', () => {
    it('stays on step 1 and says the player is rolling on another device', async () => {
      const { el, settle } = setup({
        window: {
          rolling: true,
          busy: ReactionBusy.DICE,
          waitBeganAt: timestampFromMs(Date.now() - 29_000),
          waitedMs: 29_000n,
        },
      });
      await settle();
      expect(flat(el.querySelector('[data-testid=readied-clock]'))).toMatch(
        /^Você está rolando em outro aparelho\. Passa sozinho em 1 min \d+ s\.$/,
      );
      expect(el.querySelector('[data-roll]')).toBeNull();
      expect(markReactionRolling).not.toHaveBeenCalled();
    });
  });

  describe('the states', () => {
    it('says the window is gone when the 120 s end while the player types, sends nothing and offers "Fechar"', async () => {
      const { el, settle, button, state } = await toStepTwo();
      state.apply(
        encounter({
          revision: 9,
          combatants: [combatant({ id: 'tor', label: 'Tor', mine: true })],
          reactionWindows: [],
        } as never),
      );
      await settle();
      expect(el.querySelector('[data-testid=readied-gone]')).not.toBeNull();
      expect(answerReaction).not.toHaveBeenCalled();
      expect(button('Fechar')).toBeTruthy();
    });

    it('writes the refusal at the top, keeps the number and leaves both buttons live', async () => {
      answerReaction.mockRejectedValue(
        new ConnectError('A reação já foi usada.', Code.FailedPrecondition),
      );
      const { el, settle, button, type } = await toStepTwo();
      await type('14');
      button('Confirmar 14').click();
      await settle();
      expect(flat(el.querySelector('.body [role=alert]'))).toContain('A reação já foi usada.');
      expect((el.querySelector('input') as HTMLInputElement).value).toBe('14');
      expect(button('Voltar').disabled).toBe(false);
      expect(button('Confirmar 14').disabled).toBe(false);
    });
  });

  describe('after the answer', () => {
    const hit = () => ({
      encounter: encounter({ revision: 5 } as never),
      attackRoll: create(AttackRollSchema, {
        outcome: AttackOutcome.HIT,
        d20: { total: 17 },
      } as never),
      pendingDamage: create(PendingDamageSchema, {
        id: 'p1',
        attackerId: 'tor',
        targetId: 'z1',
        attackKey: 'equipment:longsword',
        status: PendingDamageStatus.AWAITING_ROLL,
        diceCount: 1,
        diceSides: 8,
        bonus: 3,
      } as never),
    });

    it('shows the result in the same sheet, asks the damage on a hit, and drops the clock and "Voltar"', async () => {
      answerReaction.mockResolvedValue(hit());
      const { el, settle, button, type } = await toStepTwo();
      await type('11');
      button('Confirmar 11').click();
      await settle();
      expect(close).not.toHaveBeenCalled();
      expect(el.querySelector('[data-testid=readied-clock]')).toBeNull();
      expect(Array.from(el.querySelectorAll('button')).some((b) => flat(b) === 'Voltar')).toBe(
        false,
      );
      expect(flat(el.querySelector('.body'))).toContain('Acertou');
      expect(document.activeElement).toBe(el.querySelector('h2'));
      expect(markReactionRolling).toHaveBeenLastCalledWith('c', 'enc', 'w1', true);
    });

    it('rolls the damage with RollDamage on the pending damage, and ends on "Voltar à sessão"', async () => {
      answerReaction.mockResolvedValue(hit());
      rollDamage.mockResolvedValue({
        encounter: encounter({ revision: 6 } as never),
        pending: create(PendingDamageSchema, {
          id: 'p1',
          status: PendingDamageStatus.APPLIED,
        } as never),
      });
      const { settle, button, type } = await toStepTwo({ mode: DiceMode.PHYSICAL });
      await type('11');
      button('Confirmar 11').click();
      await settle();
      expect(rollDamage).not.toHaveBeenCalled();
    });

    it('ends a miss on "Voltar à sessão", which closes the sheet', async () => {
      answerReaction.mockResolvedValue({
        encounter: encounter({ revision: 5 } as never),
        attackRoll: create(AttackRollSchema, {
          outcome: AttackOutcome.MISS,
          d20: { total: 4 },
        } as never),
      });
      const { settle, button, type } = await toStepTwo();
      await type('3');
      button('Confirmar 3').click();
      await settle();
      button('Voltar à sessão').click();
      expect(close).toHaveBeenCalled();
    });
  });

  describe('a held pool spell in a combat without a map', () => {
    const sleepRow = {
      kind: ReadiedKind.SPELL,
      actionKey: 'spell:sleep',
      actionNamePt: 'Sono',
      targetCombatantId: '',
      slotLevel: 1,
    };

    it('asks the sum of the dice, in the words of the pool', async () => {
      const s = setup({
        row: sleepRow,
        spellTargets: { spellKey: 'spell:sleep', maxTargets: 0, targets: [] },
      });
      await s.settle();
      s.button('Soltar').click();
      await s.settle();
      expect(flat(s.el.querySelector('[data-roll]'))).toContain(
        'Role 5d8 para Sono: some os dados',
      );
      expect(answerReaction).not.toHaveBeenCalled();
    });
  });

  it('answers at once at a table where the app rolls', async () => {
    answerReaction.mockResolvedValue({ encounter: encounter({ revision: 5 } as never) });
    const { settle, button, el } = setup({ mode: DiceMode.APP, preference: DicePreference.APP });
    await settle();
    button('Atacar o Zumbi 1').click();
    await settle();
    expect(answerReaction).toHaveBeenCalledTimes(1);
    expect(el.querySelector('[data-roll]')).toBeNull();
  });

  it('draws the picker with both buttons where the players choose, the preference first', async () => {
    const { el } = await toStepTwo({
      mode: DiceMode.PLAYERS_CHOOSE,
      preference: DicePreference.APP,
    });
    const names = Array.from(el.querySelectorAll('[data-roll] button')).map((b) => flat(b));
    expect(names[0]).toContain('Rolar');
    expect(names.at(-1)).toContain('Voltar');
  });
});
