import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  AttackOutcome,
  CombatantKind,
  CriticalDamageRule,
  PendingDamageStatus,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  ActionEconomy,
  SpellHitPointEffectKind,
} from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { SpellCatalog } from '../../../../core/combat/spell-catalog';
import { RollAnimator } from '../../../../shared/roll-overlay/roll-animator';
import { CastSheet, type CastSheetData } from './cast-sheet';
import { fakeCombatClient } from '../../../../core/combat/combat-client-testing';

const plain = (t: string | null | undefined) =>
  (t ?? '').replace(/ /g, ' ').replace(/\s+/g, ' ').trim();

/** What the sheet says once the spell is cast, or opens on a damage never rolled: title, circle, notes, the damage still owed. */
describe('CastSheet: the result and the damage still to roll', () => {
  const d20 = (face: number) => ({
    diceCount: 1,
    diceSides: 20,
    faces: [face],
    modifier: 5,
    total: face + 5,
    countedIndex: 0,
  });
  const plainSpell = { spell: { level: 2 }, damage: [], healBySlotLevel: {}, higherLevel: [] };
  const people = [
    combatant({ id: 's', label: 'Sálvia', kind: CombatantKind.PLAYER, mine: true }),
    combatant({ id: 'g1', label: 'Goblin 1' }),
    combatant({ id: 'g2', label: 'Goblin 2' }),
  ];
  const castAnswer = (over: Record<string, unknown> = {}) => ({
    encounter: encounter({ combatants: people }),
    cast: {
      targets: [],
      pendingDamages: [],
      metamagicKeys: [],
      sorceryPointsSpent: 0,
      slot: { level: 2, pact: false },
      concentrating: false,
      concentrationEndedSpellKey: '',
      ...over,
    },
    summoned: [],
  });

  function open(
    opts: {
      data?: Record<string, unknown>;
      answer?: unknown;
      details?: unknown;
      rollDamage?: ReturnType<typeof vi.fn>;
    } = {},
  ) {
    TestBed.resetTestingModule();
    const play = vi.fn();
    const castSpell = vi.fn().mockResolvedValue(opts.answer ?? castAnswer());
    const rollDamage = opts.rollDamage ?? vi.fn().mockRejectedValue(new Error('lost'));
    const data = {
      campaignId: 'c',
      encounterId: 'enc',
      casterId: 's',
      round: 4,
      spellKey: 'spell:bless',
      name: 'Bênção',
      level: 2,
      concentration: true,
      cantripDice: '',
      economy: ActionEconomy.ACTION,
      slots: [{ level: 2, free: 2, pact: false }],
      usage: [{ level: 2, total: 3, used: 1 }],
      pact: null,
      targets: undefined,
      shieldFree: null,
      shieldName: 'Escudo Arcano',
      attackBonus: 5,
      diceMode: DiceMode.PLAYERS_CHOOSE,
      preference: DicePreference.APP,
      state: { encounter: signal(encounter({ combatants: people })), apply: vi.fn() },
      ...opts.data,
    } as unknown as CastSheetData;
    TestBed.configureTestingModule({
      providers: [
        { provide: CombatClient, useValue: fakeCombatClient({ castSpell, rollDamage }) },
        {
          provide: SpellCatalog,
          useValue: { details: () => Promise.resolve(opts.details ?? plainSpell) },
        },
        { provide: RollAnimator, useValue: { play } },
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close: vi.fn() } },
      ],
    });
    const fixture = TestBed.createComponent(CastSheet);
    const settle = async () => {
      for (let i = 0; i < 3; i++) {
        fixture.detectChanges();
        await fixture.whenStable();
      }
    };
    const el = fixture.nativeElement as HTMLElement;
    const cast = async () => {
      await settle();
      el.querySelector<HTMLButtonElement>('button.cast')!.click();
      await settle();
    };
    return {
      fixture,
      el,
      play,
      castSpell,
      rollDamage,
      settle,
      cast,
      text: () => plain(el.textContent),
    };
  }
  const title = (el: HTMLElement) => plain(el.querySelector('h2')?.textContent);

  it('titles a told cast "Você conjurou X", with the circle and the notes of what it spent', async () => {
    const s = open({
      answer: castAnswer({ concentrating: true, concentrationEndedSpellKey: 'spell:web' }),
      data: { shieldFree: 1 },
    });
    await s.cast();
    expect(title(s.el)).toBe('Você conjurou Bênção');
    expect(s.text()).toContain('2º nível');
    expect(s.text()).toContain('Você está concentrado em Bênção.');
    expect(s.text()).toContain('A sua concentração anterior acabou.');
    expect(s.text()).toContain('Sua ação foi usada.');
    expect(s.text()).toContain('Espaços de 2º nível: 1 livre de 3');
    expect(s.text()).toContain('Escudo Arcano indisponível: sem espaço de 2º nível.');
    expect(s.el.querySelector('button.cast')?.textContent).toContain('Voltar à sua vez');
  });

  it('says the bonus action was used, and names the metamagic and the points it cost', async () => {
    const s = open({
      data: {
        economy: ActionEconomy.BONUS_ACTION,
        sorceryPoints: { current: 5, max: 5 },
        metamagic: [{ key: 'metamagic:quickened', namePt: 'Magia Acelerada', cost: 2 }],
      },
      answer: castAnswer({
        metamagicKeys: ['metamagic:quickened', 'metamagic:unknown'],
        sorceryPointsSpent: 2,
      }),
    });
    await s.cast();
    expect(s.text()).toContain('Sua ação bônus foi usada.');
    expect(s.text()).toContain('Magia Acelerada');
    expect(s.text()).toContain('Metamagia');
  });

  it('shows no slot line when the slot of the cast is not listed', async () => {
    const s = open({ answer: castAnswer({ slot: { level: 5, pact: false } }) });
    await s.cast();
    expect(s.text()).not.toContain('Espaços de');
    expect(s.text()).not.toContain('indisponível');
  });

  it('opens on the damage never rolled: its title, who is hit and the dice, with the critical line when typing', async () => {
    const s = open({
      data: {
        resume: [
          {
            id: 'p1',
            targetId: 'g1',
            attackKey: 'spell:fire-bolt',
            status: PendingDamageStatus.AWAITING_ROLL,
            diceCount: 2,
            diceSides: 10,
            bonus: 3,
            criticalMax: 0,
            critical: true,
            criticalRule: CriticalDamageRule.DOUBLED_DICE,
          },
          {
            id: 'p2',
            targetId: 'g2',
            status: PendingDamageStatus.AWAITING_ROLL,
            diceCount: 2,
            diceSides: 10,
            bonus: 3,
            criticalMax: 0,
          },
        ],
      },
    });
    await s.settle();
    expect(title(s.el)).toBe('Role o dano de Bênção');
    expect(s.text()).toContain('Dano em Goblin 1.');
    expect(s.text()).toContain('Role um de cada vez ou todos no app.');
    expect(s.text()).toContain('Rolar o dano no app');
  });

  it('says "Cura" for a heal still to roll, and one die in the singular', async () => {
    const s = open({
      data: {
        resume: [
          {
            id: 'p1',
            targetId: 'g1',
            status: PendingDamageStatus.AWAITING_ROLL,
            diceCount: 1,
            diceSides: 8,
            bonus: 4,
            criticalMax: 0,
            healing: true,
          },
        ],
      },
    });
    await s.settle();
    expect(s.text()).toContain('Cura em Goblin 1.');
    expect(s.text()).toContain('Rolar 1d8 no app');
    expect(s.text()).not.toContain('Role um de cada vez');
  });

  it('names a target it cannot find "alvo"', async () => {
    const s = open({
      data: {
        resume: [
          {
            id: 'p1',
            targetId: 'ghost',
            status: PendingDamageStatus.AWAITING_ROLL,
            diceCount: 1,
            diceSides: 8,
            bonus: 0,
            criticalMax: 0,
          },
        ],
      },
    });
    await s.settle();
    expect(s.text()).toContain('Dano em alvo.');
  });

  it('animates the d20 of a single spell attack with the total and the word', async () => {
    const s = open({
      answer: castAnswer({
        targets: [{ combatantId: 'g1', attackRoll: d20(14), outcome: AttackOutcome.HIT, darts: 0 }],
      }),
    });
    await s.cast();
    expect(s.play).toHaveBeenCalledTimes(1);
    expect(s.play.mock.calls[0][0]).toMatchObject({
      label: 'Ataque com Bênção',
      outcome: { word: 'Acertou', good: true },
    });
  });

  it('animates a natural 20 as a critical and a natural 1 as a fumble, and a roll nobody told with no word', async () => {
    const crit = open({
      answer: castAnswer({
        targets: [
          { combatantId: 'g1', attackRoll: d20(20), outcome: AttackOutcome.CRITICAL_HIT, darts: 0 },
        ],
      }),
    });
    await crit.cast();
    expect(crit.play.mock.calls[0][0]).toMatchObject({ critical: true });
    const miss = open({
      answer: castAnswer({
        targets: [{ combatantId: 'g1', attackRoll: d20(1), outcome: AttackOutcome.MISS, darts: 0 }],
      }),
    });
    await miss.cast();
    expect(miss.play.mock.calls[0][0]).toMatchObject({
      fumble: true,
      outcome: { word: 'Errou', good: false },
    });
    const untold = open({
      answer: castAnswer({
        targets: [
          { combatantId: 'g1', attackRoll: d20(9), outcome: AttackOutcome.UNSPECIFIED, darts: 0 },
        ],
      }),
    });
    await untold.cast();
    expect(untold.play.mock.calls[0][0].outcome).toBeUndefined();
  });

  it('puts the d20 of several targets in one overlay, with no word', async () => {
    const s = open({
      answer: castAnswer({
        targets: [
          { combatantId: 'g1', attackRoll: d20(14), outcome: AttackOutcome.HIT, darts: 0 },
          { combatantId: 'g2', attackRoll: d20(3), outcome: AttackOutcome.MISS, darts: 0 },
        ],
      }),
    });
    await s.cast();
    const show = s.play.mock.calls[0][0];
    expect(show.label).toBe('Ataque com Bênção');
    expect(show.dice).toHaveLength(2);
    expect(show.outcome).toBeUndefined();
  });

  it('animates the pool of dice a spell rolled, with its total', async () => {
    const s = open({
      answer: castAnswer({
        targets: [],
        poolRoll: { diceCount: 2, diceSides: 8, faces: [3, 5], modifier: 0, total: 8 },
      }),
    });
    await s.cast();
    expect(s.play).toHaveBeenCalledTimes(1);
    expect(s.play.mock.calls[0][0]).toMatchObject({ label: 'Bênção' });
    expect(s.play.mock.calls[0][0].line).toBeDefined();
  });

  it('plays nothing when the cast rolled no dice', async () => {
    const s = open();
    await s.cast();
    expect(s.play).not.toHaveBeenCalled();
  });

  it('opens a cantrip result under "Truque" with no slot', async () => {
    const s = open({
      data: {
        spellKey: 'spell:light',
        name: 'Luz',
        level: 0,
        concentration: false,
        slots: [],
        usage: [],
      },
      answer: castAnswer({ slot: undefined }),
    });
    await s.cast();
    expect(s.text()).toContain('Truque');
    expect(s.text()).toContain('Sua ação foi usada.');
    expect(s.text()).not.toContain('concentrado');
  });

  it('titles a spell that reads hit points "Sono conjurado", with the creatures in the area', async () => {
    const sleep = {
      spell: { level: 1 },
      hitPointEffect: {
        kind: SpellHitPointEffectKind.POOL,
        poolDiceCount: 5,
        poolDicePerLevel: 2,
        poolDiceSides: 8,
      },
      healBySlotLevel: {},
      damage: [],
      higherLevel: [],
    };
    const s = open({
      data: {
        spellKey: 'spell:sleep',
        name: 'Sono',
        level: 2,
        diceMode: DiceMode.APP,
        concentration: false,
      },
      details: sleep,
      answer: castAnswer({
        targets: [
          { combatantId: 'g1', darts: 0 },
          { combatantId: 'g2', darts: 0 },
        ],
      }),
    });
    await s.cast();
    expect(title(s.el)).toBe('Sono conjurado');
    expect(plain(s.el.querySelector('app-sheet-frame')?.textContent)).toContain(
      '2º nível · 2 criaturas na área',
    );
  });

  it('rolls every damage owed in the app, one call each, and plays the dice of each', async () => {
    const rollDamage = vi.fn(async (_c: string, _e: string, id: string) => ({
      encounter: encounter({ combatants: people }),
      pending: {
        id,
        status: PendingDamageStatus.ROLLED,
        roll: { diceCount: 1, diceSides: 6, faces: [4], modifier: 0, total: 4 },
        damageTypePt: 'fogo',
      },
      cast: [],
    }));
    const group = (id: string, target: string, sides: number) => ({
      id,
      targetId: target,
      status: PendingDamageStatus.AWAITING_ROLL,
      diceCount: 1,
      diceSides: sides,
      bonus: 0,
      criticalMax: 0,
    });
    const s = open({
      rollDamage,
      data: { resume: [group('p1', 'g1', 6), group('p2', 'g2', 8)] },
    });
    await s.settle();
    [...s.el.querySelectorAll('button')]
      .find((b) => b.textContent?.includes('Rolar o dano no app'))!
      .click();
    await s.settle();
    expect(rollDamage).toHaveBeenCalledTimes(2);
    expect(s.play).toHaveBeenCalledTimes(2);
    expect(s.play.mock.calls[0][0]).toMatchObject({ label: 'Dano', note: 'fogo' });
  });

  it('says the damage could not be rolled when the call fails, and stops there', async () => {
    const s = open({
      data: {
        resume: [
          {
            id: 'p1',
            targetId: 'g1',
            status: PendingDamageStatus.AWAITING_ROLL,
            diceCount: 1,
            diceSides: 6,
            bonus: 0,
            criticalMax: 0,
          },
        ],
      },
    });
    await s.settle();
    [...s.el.querySelectorAll('button')]
      .find((b) => b.textContent?.includes('Rolar 1d6 no app'))!
      .click();
    await s.settle();
    expect(s.rollDamage).toHaveBeenCalledTimes(1);
    expect(s.el.querySelector('[role="alert"]')).not.toBeNull();
    expect(s.play).not.toHaveBeenCalled();
  });
});
