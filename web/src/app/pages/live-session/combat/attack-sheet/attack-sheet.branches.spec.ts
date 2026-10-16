// The branches of `attack-sheet.ts` the other specs of the attack sheet never took (found with the coverage report): the
// target list of a reaction or a throw, what the attack spent, the damage before it is rolled, the requests to the master
// that are taken back, a hit that waits for a reaction, and the questions a roll can answer with. They read the sheet
// through its protected members, as the specs next to this one do, and were written green on the code as it was before
// it was split.
import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  AttackOutcome,
  AttackRollSchema,
  CombatantKind,
  CombatantSide,
  DiceRollSchema,
  EncounterMode,
  InspirationOfferSchema,
  LuckyOfferSchema,
  PendingDamageStatus,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { RollMode, RollModeRequestStatus } from '../../../../../gen/meurpg/play/v1/combat_rolls_pb';
import {
  AttackKind,
  BonusAttackRule,
  type Attack,
} from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { CombatState } from '../../../../core/combat/combat-state';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { LUCKY_TRAIT } from '../../../../core/combat/lucky';
import { ResourceClient } from '../../../../core/resources/resources-client';
import { AttackSheet, type AttackSheetData } from './attack-sheet';
import { nextTurn } from '../../../../../test-settle';
import { fakeCombatClient } from '../../../../core/combat/combat-client-testing';

/** The sheet seen from a spec: protected members and all. */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
type Sheet = any;

/** Where the members the sheet handed to its parts live now: a name a spec reads finds its part, bound to it. */
const MOVED: Record<string, string> = {
  damageIntro: 'dmg.intro',
  damageLabel: 'dmg.label',
  damageHint: 'dmg.hint',
  damageModifier: 'dmg.modifier',
  damageAppLabel: 'dmg.appLabel',
  modeReady: 'mode.ready',
  askMaster: 'modes.ask',
  cancelAsk: 'modes.cancel',
  setMode: 'mode.set',
  modeChoice: 'mode.choice',
  followReaction: 'rolls.followReaction',
};
const PARTS = ['s', 'mode', 'dmg', 'questions', 'modes', 'rolls'];

// eslint-disable-next-line @typescript-eslint/no-explicit-any
function seen(cmp: any): Sheet {
  return new Proxy(cmp, {
    get(target, key: string) {
      const path =
        MOVED[key] ??
        (key in target && key !== 'busy'
          ? ''
          : (PARTS.find((p) => key in target[p]) ?? '') + '.' + key);
      if (path === '') {
        return target[key];
      }
      const steps = path.split('.').filter(Boolean);
      const owner = steps.slice(0, -1).reduce((o, k) => o[k], target);
      const value = owner[steps[steps.length - 1]];
      // A signal is a field of its owner and stays as it is; a method is bound to it.
      const isMethod =
        typeof value === 'function' && !Object.hasOwn(owner, steps[steps.length - 1]);
      return isMethod ? value.bind(owner) : value;
    },
  });
}

const plain = (t: string | null | undefined) =>
  (t ?? '').replace(/ /g, ' ').replace(/\s+/g, ' ').trim();

const sword = {
  key: 'attack:longsword',
  name: 'Longsword',
  namePt: 'Espada longa',
  attackBonus: 6,
  saveDc: 0,
  rangeFt: 5,
  longRangeFt: 0,
  damage: '1d8 + 3',
  damageTypePt: 'cortante',
  kind: 0,
} as unknown as Attack;

const dagger = {
  ...sword,
  key: 'attack:dagger',
  namePt: 'Adaga',
  rangeFt: 20,
  longRangeFt: 60,
} as Attack;
/** A cantrip is an attack with a spell for its key. */
const bolt = {
  ...sword,
  key: 'spell:eldritch-blast',
  namePt: 'Rajada Mística',
  rangeFt: 120,
  kind: AttackKind.SPELL,
} as Attack;

const toren = combatant({
  id: 't',
  label: 'Toren',
  kind: CombatantKind.PLAYER,
  side: CombatantSide.PARTY,
  mine: true,
});
const brisa = combatant({
  id: 'b',
  label: 'Brisa',
  kind: CombatantKind.PLAYER,
  side: CombatantSide.PARTY,
});
const goblin = combatant({ id: 'g', label: 'Goblin' });

const reach = (id: string, over: Record<string, unknown> = {}) => ({
  combatantId: id,
  label: id.toUpperCase(),
  tooFar: false,
  ...over,
});

const d20 = (face: number, over: Record<string, unknown> = {}) =>
  create(DiceRollSchema, {
    diceCount: 1,
    diceSides: 20,
    faces: [face],
    modifier: 6,
    total: face + 6,
    ...over,
  });

const pendingDamage = (over: Record<string, unknown> = {}) =>
  ({
    id: 'p1',
    attackerId: 't',
    targetId: 'g',
    attackKey: 'attack:longsword',
    status: PendingDamageStatus.AWAITING_ROLL,
    diceCount: 1,
    diceSides: 8,
    bonus: 3,
    parts: [],
    ...over,
  }) as never;

interface Over {
  data?: Record<string, unknown>;
  enc?: Record<string, unknown>;
  noEncounter?: boolean;
  api?: Record<string, unknown>;
  resources?: Record<string, unknown>;
}

async function open(over: Over = {}) {
  TestBed.resetTestingModule();
  const state = new CombatState();
  if (!over.noEncounter) {
    state.apply(
      encounter({
        mode: EncounterMode.THEATRE,
        currentCombatantId: 't',
        combatants: [toren, brisa, goblin],
        ...over.enc,
      }),
    );
  }
  const data = {
    campaignId: 'c',
    encounterId: 'enc',
    attackerId: 't',
    round: 1,
    attack: sword,
    targets: [reach('g'), reach('b')],
    diceMode: DiceMode.PLAYERS_CHOOSE,
    preference: DicePreference.APP,
    state,
    ...over.data,
  } as unknown as AttackSheetData;
  const close = vi.fn();
  const api = { rollAttack: vi.fn(), rollDamage: vi.fn(), ...over.api };
  const resources = {
    answerLucky: vi.fn(),
    answerBardicInspiration: vi.fn(),
    ...over.resources,
  };
  TestBed.configureTestingModule({
    providers: [
      { provide: MAT_DIALOG_DATA, useValue: data },
      { provide: MatDialogRef, useValue: { close } },
      { provide: CombatClient, useValue: fakeCombatClient(api) },
      { provide: ResourceClient, useValue: resources },
    ],
  });
  const fixture = TestBed.createComponent(AttackSheet);
  fixture.detectChanges();
  return {
    fixture,
    sheet: seen(fixture.componentInstance),
    el: fixture.nativeElement as HTMLElement,
    state,
    api,
    resources,
    close,
    settle: async () => {
      fixture.detectChanges();
      await nextTurn();
      await fixture.whenStable();
      fixture.detectChanges();
    },
  };
}

beforeEach(() => {
  // The sheet scrolls its error into view; the test DOM has no scroll.
  Element.prototype.scrollTo = vi.fn();
});

describe('AttackSheet: the list of targets', () => {
  it('tags an ally, and not a creature the combat does not know', async () => {
    const { sheet } = await open({ data: { targets: [reach('b'), reach('ghost')] } });
    const rows = sheet.rows();
    expect(plain(rows.find((r: { id: string }) => r.id === 'b').sub)).toContain('Aliada');
    expect(rows.find((r: { id: string }) => r.id === 'ghost').sub).not.toContain('Aliad');
    expect(sheet.target()).toBeNull();
    expect(sheet.reach()).toBeNull();
  });

  it('reaches 5 ft for a reaction, whoever has no distance being out of reach', async () => {
    const { sheet } = await open({
      data: {
        asReaction: true,
        targets: [reach('g', { distanceFt: 5 }), reach('b', { distanceFt: 10 }), reach('t')],
      },
    });
    const blocked = (id: string) => sheet.rows().find((r: { id: string }) => r.id === id).blocked;
    expect(blocked('g')).toBe('');
    expect(blocked('b')).not.toBe('');
    expect(blocked('t')).not.toBe('');
    expect(sheet.detail).toContain('Reação');
  });

  it("reaches the long range for the throw back, keeping the server's word where it has no distance", async () => {
    const { sheet } = await open({
      data: {
        attack: dagger,
        catchWindowId: 'w1',
        targets: [
          reach('g', { distanceFt: 50 }),
          reach('b', { distanceFt: 80 }),
          reach('t', { tooFar: true }),
        ],
      },
    });
    const blocked = (id: string) => sheet.rows().find((r: { id: string }) => r.id === id).blocked;
    expect(blocked('g')).toBe('');
    expect(blocked('b')).not.toBe('');
    expect(blocked('t')).not.toBe('');
  });

  it('opens an opportunity attack at its single target, with no mode to pick', async () => {
    const { sheet } = await open({
      data: {
        opportunity: { offerId: 'o1', targetId: 'g', targetLabel: 'Goblin', byMaster: false },
      },
    });
    expect(sheet.rows().map((r: { id: string }) => r.id)).toEqual(['g']);
    expect(sheet.hasMode()).toBe(false);
    expect(sheet.modeReady()).toBe(true);
    expect(sheet.faceCount()).toBe(1);
    expect(sheet.doneLabel).toBe('Fechar');
  });

  it('takes a request the combat closed as no request, and one it has not heard of as the one sent', async () => {
    const sent = { id: 'r1', status: RollModeRequestStatus.PENDING } as never;
    const { sheet, state } = await open({ noEncounter: true });
    sheet.modes.asked.set(sent);
    expect(sheet.request()).toBe(sent);
    state.apply(
      encounter({
        combatants: [toren],
        rollModeRequests: [{ id: 'r1', status: RollModeRequestStatus.CLOSED }] as never,
      }),
    );
    expect(sheet.request()).toBeNull();
  });

  it('knows no combat before it is read', async () => {
    const { sheet } = await open({ noEncounter: true });
    expect(sheet.rows().length).toBe(2);
    expect(sheet.held()).toBeNull();
    sheet.roll.set(create(AttackRollSchema, { heldForReaction: true }));
    expect(sheet.held()).toEqual({
      title: 'Esperando o mestre',
      detail: 'O resultado do seu ataque sai quando ele responder.',
    });
    expect(sheet.request()).toBeNull();
    expect(sheet.npcTarget('g')).toBe(false);
    expect(sheet.creatureTarget('g')).toBe(false);
  });
});

describe('AttackSheet: the title of the target', () => {
  it('falls back to the label of the damage it resumes, or of the held roll it reopens', async () => {
    const resumed = await open({
      data: { resume: { pending: pendingDamage(), targetLabel: 'Capitão' } },
    });
    expect(resumed.sheet.targetLabel()).toBe('Capitão');
    const reopened = await open({
      data: {
        lucky: {
          offer: create(LuckyOfferSchema, { holdId: 'h', targetId: 'ghost', d20: d20(1) }),
          targetLabel: 'Goblin guardado',
        },
      },
    });
    expect(reopened.sheet.targetLabel()).toBe('Goblin guardado');
    const none = await open();
    expect(none.sheet.targetLabel()).toBe('');
  });
});

describe('AttackSheet: what the attack spent', () => {
  it("says the master's reaction was used, or the player's", async () => {
    const master = await open({
      data: { opportunity: { offerId: 'o', targetId: 'g', targetLabel: 'Goblin', byMaster: true } },
    });
    expect(master.sheet.spent()).toBe('A reação dele foi usada.');
    const own = await open({ data: { asReaction: true } });
    expect(own.sheet.spent()).toBe('Sua reação foi usada.');
  });

  it('says the bonus action, and the strikes of the flurry that are left', async () => {
    const off = await open({ data: { bonusRule: BonusAttackRule.OFF_HAND } });
    expect(off.sheet.spent()).toBe('Sua ação bônus foi usada.');
    const flurry = await open({
      data: { bonusRule: BonusAttackRule.FLURRY_OF_BLOWS, bonusAttacksLeft: 2 },
    });
    expect(flurry.sheet.spent()).toContain('Rajada de Golpes');
  });

  it('counts the beams a cantrip has left, and the attacks the action has left', async () => {
    const beams = await open({ data: { attack: bolt, beamsLeft: 3 } });
    expect(beams.sheet.cantrip).toBe(true);
    expect(beams.sheet.spent()).toBe('Rajada Mística: 2 raios restantes.');
    const last = await open({ data: { attack: bolt, beamsLeft: 1 } });
    expect(last.sheet.spent()).toBe('Sua ação foi usada.');
    const unknown = await open({ data: { attack: bolt } });
    expect(unknown.sheet.spent()).toBe('Sua ação foi usada.');
    const two = await open({ data: { attacksLeft: 3, attacksPerAction: 3 } });
    expect(two.sheet.spent()).toBe('Você ainda tem 2 ataques desta ação.');
    const one = await open({ data: { attacksLeft: 2, attacksPerAction: 2 } });
    expect(one.sheet.spent()).toBe('Você ainda tem 1 ataque desta ação.');
    const done = await open({ data: { attacksLeft: 1, attacksPerAction: 2 } });
    expect(done.sheet.spent()).toBe('Sua ação foi usada.');
  });
});

describe('AttackSheet: the damage before it is rolled', () => {
  it('has no dice, no hint and no intro without a pending damage', async () => {
    const { sheet } = await open();
    expect(sheet.dice()).toBeNull();
    expect(sheet.range()).toEqual({ min: 1, max: 1 });
    expect(sheet.damageIntro()).toBe('');
    expect(sheet.damageLabel()).toBe('Role  para o dano');
    expect(sheet.damageAppLabel()).toBe('Rolar  no app');
    expect(sheet.after()).toBe('');
    expect(sheet.damageHint()).toBeDefined();
    expect(sheet.damageModifier()).toBe(0);
    expect(sheet.fixedText()).toBeDefined();
  });

  it('has nothing to follow when the sheet holds no damage', async () => {
    const turnOptions = vi.fn();
    const { sheet } = await open({ api: { turnOptions } });
    await sheet.followReaction();
    expect(turnOptions).not.toHaveBeenCalled();
  });

  it('says "Acertou" for a plain hit and a doubled-dice line for a critical one', async () => {
    const hit = await open({
      data: { resume: { pending: pendingDamage({ critical: false }), targetLabel: 'Goblin' } },
    });
    expect(hit.sheet.damageIntro()).toBe('Acertou: role o dano.');
    const crit = await open({
      data: { resume: { pending: pendingDamage({ critical: true }), targetLabel: 'Goblin' } },
    });
    expect(crit.sheet.damageIntro()).toContain('Acerto crítico');
  });

  it('rolls "some os dois" for two dice', async () => {
    const { sheet } = await open({
      data: { resume: { pending: pendingDamage({ diceCount: 2 }), targetLabel: 'Goblin' } },
    });
    expect(sheet.damageLabel()).toContain('some os dois');
  });

  it('writes the modifier of a critical with extra dice, positive, negative or none', async () => {
    const brutal = (bonus: number) => ({
      resume: {
        pending: pendingDamage({
          critical: true,
          bonus,
          criticalMax: 0,
          extraDiceCount: 1,
          extraDiceNamePt: 'Crítico Brutal',
        }),
        targetLabel: 'Goblin',
      },
    });
    const plus = await open({ data: brutal(3) });
    const minus = await open({ data: brutal(-2) });
    const none = await open({ data: brutal(0) });
    expect(plus.sheet.brutal()).not.toBeNull();
    expect(plus.sheet.fixedText()).toBe('+ 3 de bônus');
    expect(minus.sheet.fixedText()).toBe('− 2 de bônus');
    expect(none.sheet.fixedText()).toBe('');
  });

  it('signs a negative attack bonus with a minus', async () => {
    const { sheet } = await open({ data: { attack: { ...sword, attackBonus: -1 } } });
    expect(sheet.rollLabel()).toContain('−1');
  });
});

describe('AttackSheet: the mode of the d20', () => {
  it('moves on to the roll only with a target', async () => {
    const { sheet } = await open();
    sheet.advance();
    expect(sheet.stage()).toBe('target');
    sheet.pick('g');
    sheet.advance();
    expect(sheet.stage()).toBe('roll');
    sheet.change();
    expect(sheet.stage()).toBe('target');
  });

  it('takes a request not in the combat as the one sent, and an answered one with the same mode as no reason', async () => {
    const sent = {
      id: 'r1',
      status: RollModeRequestStatus.PENDING,
      suggestedMode: RollMode.NORMAL,
      requestedMode: RollMode.ADVANTAGE,
      decidedMode: RollMode.ADVANTAGE,
      reason: 'escondido',
    };
    const requestRollMode = vi
      .fn()
      .mockResolvedValue({ encounter: encounter({ combatants: [toren] }), request: sent });
    const { sheet, state } = await open({ api: { requestRollMode } });
    sheet.pick('g');
    sheet.setMode(RollMode.ADVANTAGE);
    sheet.reason.set('escondido');
    await sheet.askMaster();
    expect(sheet.request()).toEqual(sent);
    state.apply(
      encounter({
        mode: EncounterMode.THEATRE,
        combatants: [toren, brisa, goblin],
        rollModeRequests: [{ ...sent, status: RollModeRequestStatus.ANSWERED }] as never,
        revision: 5,
      }),
    );
    expect(sheet.rollMode()).toBe(RollMode.ADVANTAGE);
    expect(sheet.modeChoice()).toEqual({
      mode: RollMode.ADVANTAGE,
      reason: 'escondido',
      requestId: 'r1',
    });
    state.apply(
      encounter({
        mode: EncounterMode.THEATRE,
        combatants: [toren, brisa, goblin],
        rollModeRequests: [
          { ...sent, status: RollModeRequestStatus.ANSWERED, suggestedMode: RollMode.ADVANTAGE },
        ] as never,
        revision: 6,
      }),
    );
    expect(sheet.modeChoice().reason).toBe('');
    state.apply(
      encounter({
        mode: EncounterMode.THEATRE,
        combatants: [toren],
        rollModeRequests: [],
        revision: 7,
      }),
    );
    expect(sheet.request()).toEqual(sent);
  });

  it('takes back a request still waiting when the target changes, and ignores the answer of that call', async () => {
    const pending = {
      id: 'r1',
      status: RollModeRequestStatus.PENDING,
      suggestedMode: RollMode.NORMAL,
      requestedMode: RollMode.ADVANTAGE,
      decidedMode: RollMode.NORMAL,
      reason: 'x',
    };
    const requestRollMode = vi
      .fn()
      .mockResolvedValue({ encounter: encounter({ combatants: [toren] }), request: pending });
    const cancelRollModeRequest = vi
      .fn()
      .mockResolvedValueOnce(encounter({ combatants: [toren] }))
      .mockRejectedValueOnce(new Error('late'));
    const { sheet, settle } = await open({ api: { requestRollMode, cancelRollModeRequest } });
    sheet.pick('g');
    sheet.setMode(RollMode.ADVANTAGE);
    sheet.reason.set('x');
    await sheet.askMaster();
    sheet.pick('b');
    await settle();
    expect(cancelRollModeRequest).toHaveBeenCalledTimes(1);
    await sheet.askMaster();
    sheet.change();
    await settle();
    expect(cancelRollModeRequest).toHaveBeenCalledTimes(2);
  });

  it('asks the master only with a target and when not busy, and says when it fails', async () => {
    const requestRollMode = vi.fn().mockRejectedValue(new ConnectError('x', Code.Unavailable));
    const { sheet } = await open({ api: { requestRollMode } });
    await sheet.askMaster();
    expect(requestRollMode).not.toHaveBeenCalled();
    sheet.pick('g');
    sheet.busy.set(true);
    await sheet.askMaster();
    expect(requestRollMode).not.toHaveBeenCalled();
    sheet.busy.set(false);
    await sheet.askMaster();
    expect(sheet.error()).not.toBe('');
  });

  it('cancels a request only when one was sent, and says when it fails', async () => {
    const sent = {
      id: 'r1',
      status: RollModeRequestStatus.PENDING,
      suggestedMode: RollMode.NORMAL,
      requestedMode: RollMode.ADVANTAGE,
      decidedMode: RollMode.NORMAL,
      reason: 'x',
    };
    const requestRollMode = vi
      .fn()
      .mockResolvedValue({ encounter: encounter({ combatants: [toren] }), request: sent });
    const cancelRollModeRequest = vi
      .fn()
      .mockRejectedValue(new ConnectError('x', Code.Unavailable));
    const { sheet } = await open({ api: { requestRollMode, cancelRollModeRequest } });
    await sheet.cancelAsk();
    expect(cancelRollModeRequest).not.toHaveBeenCalled();
    sheet.pick('g');
    sheet.setMode(RollMode.ADVANTAGE);
    sheet.reason.set('x');
    await sheet.askMaster();
    sheet.busy.set(true);
    await sheet.cancelAsk();
    expect(cancelRollModeRequest).not.toHaveBeenCalled();
    sheet.busy.set(false);
    await sheet.cancelAsk();
    expect(sheet.error()).not.toBe('');
  });
});

describe('AttackSheet: rolling', () => {
  it('rolls nothing without a target, or while busy', async () => {
    const rollAttack = vi.fn();
    const { sheet } = await open({ api: { rollAttack } });
    await sheet.rollAttackTyped(12);
    sheet.pick('g');
    sheet.busy.set(true);
    await sheet.rollAttackTyped(12);
    expect(rollAttack).not.toHaveBeenCalled();
  });

  it('asks for the d4 of an effect before a physical roll, in the singular and the plural', async () => {
    const one = await open({ data: { diceMode: DiceMode.PHYSICAL } });
    one.sheet.pick('g');
    one.sheet.extraFields.set([{ key: 'e1', label: 'Bênção', faces: 4 }]);
    await one.sheet.rollAttackTyped(12);
    expect(one.sheet.error()).toBe('Digite o resultado do d4 antes de confirmar.');
    one.sheet.extraFields.set([
      { key: 'e1', label: 'Bênção', faces: 4 },
      { key: 'e2', label: 'Perdição', faces: 4 },
    ]);
    await one.sheet.rollAttackTyped(12);
    expect(one.sheet.error()).toBe('Digite o resultado de cada dado extra antes de confirmar.');
  });

  it('asks for more d4 when the server says the roll takes them, one or several', async () => {
    const rollAttack = vi
      .fn()
      .mockRejectedValueOnce(new ConnectError('the roll takes 1 more die(s)', Code.InvalidArgument))
      .mockRejectedValueOnce(new ConnectError('the roll takes 2 more die(s)', Code.InvalidArgument))
      .mockRejectedValue(new ConnectError('x', Code.Unavailable));
    const { sheet } = await open({ api: { rollAttack } });
    sheet.pick('g');
    await sheet.rollAttackTyped(12);
    expect(sheet.error()).toBe('Esta rolagem leva mais um d4: role-o e digite o resultado.');
    expect(sheet.extraFields().length).toBe(1);
    await sheet.rollAttack({ inApp: true });
    expect(sheet.error()).toBe('Esta rolagem leva mais 2 d4: role-os e digite os resultados.');
    expect(sheet.extraFields().length).toBe(2);
    await sheet.rollAttack({ inApp: true });
    expect(sheet.error()).not.toBe('');
  });

  it('answers a roll held for a question with the d20 and no word', async () => {
    const luckyOffer = create(LuckyOfferSchema, { holdId: 'h', targetId: 'g', d20: d20(1) });
    const rollAttack = vi.fn().mockResolvedValue({
      encounter: encounter({ combatants: [toren] }),
      roll: create(AttackRollSchema, { outcome: AttackOutcome.UNSPECIFIED, d20: d20(1) }),
      pending: undefined,
      lucky: luckyOffer,
    });
    const onHeld = vi.fn();
    const { sheet } = await open({ api: { rollAttack }, data: { onHeld } });
    sheet.pick('g');
    await sheet.rollAttackTyped(1);
    expect(sheet.luckyOffer()).toBe(luckyOffer);
    expect(sheet.asking()).toBe(true);
    expect(sheet.outcome()).toBeNull();
    expect(onHeld).toHaveBeenCalledWith('h');
  });
});

describe('AttackSheet: the damage', () => {
  const resume = (over: Record<string, unknown> = {}) => ({
    resume: { pending: pendingDamage(over), targetLabel: 'Goblin' },
  });

  it('rolls nothing without a pending damage, or while busy', async () => {
    const rollDamage = vi.fn();
    const none = await open({ api: { rollDamage } });
    await none.sheet.rollDamageTyped(5);
    const busy = await open({ api: { rollDamage }, data: resume() });
    busy.sheet.busy.set(true);
    await busy.sheet.rollDamageTyped(5);
    expect(rollDamage).not.toHaveBeenCalled();
  });

  it('does not animate a damage that waits for a reaction', async () => {
    const rollDamage = vi.fn().mockResolvedValue({
      encounter: encounter({ combatants: [toren] }),
      pending: pendingDamage({ status: PendingDamageStatus.AWAITING_REACTION }),
    });
    const { sheet } = await open({ api: { rollDamage }, data: resume() });
    const play = vi.spyOn(sheet.animator, 'play');
    await sheet.rollDamageTyped(5);
    expect(play).not.toHaveBeenCalled();
    expect(sheet.stage()).toBe('done');
    sheet.close();
  });

  it('closes with whether it is done, and not while busy', async () => {
    const { sheet, close } = await open();
    sheet.busy.set(true);
    sheet.close();
    expect(close).not.toHaveBeenCalled();
    sheet.busy.set(false);
    sheet.close();
    expect(close).toHaveBeenCalledWith(false);
  });
});

describe('AttackSheet: a hit that waits for a reaction', () => {
  const awaiting = {
    resume: {
      pending: pendingDamage({ status: PendingDamageStatus.AWAITING_REACTION }),
      targetLabel: 'Goblin',
    },
  };

  it('keeps what it shows when the options cannot be read', async () => {
    const turnOptions = vi.fn().mockRejectedValue(new Error('offline'));
    const { sheet, settle } = await open({ api: { turnOptions }, data: awaiting });
    await settle();
    expect(turnOptions).toHaveBeenCalled();
    expect(sheet.pending()).not.toBeNull();
  });

  it('turns into the damage to roll when the reaction let it through, and into a miss when it stopped it', async () => {
    const through = vi.fn().mockResolvedValue({
      pendingDamages: [pendingDamage({ status: PendingDamageStatus.AWAITING_ROLL })],
    });
    const a = await open({ api: { turnOptions: through }, data: awaiting });
    await a.settle();
    expect(a.sheet.stage()).toBe('damage');

    let answer: (v: unknown) => void = () => undefined;
    const gone = vi.fn().mockReturnValue(new Promise((ok) => (answer = ok)));
    const b = await open({ api: { turnOptions: gone }, data: awaiting });
    b.sheet.roll.set(create(AttackRollSchema, { outcome: AttackOutcome.HIT }));
    answer({ pendingDamages: [] });
    await b.settle();
    expect(b.sheet.stage()).toBe('done');
    expect(b.sheet.roll().outcome).toBe(AttackOutcome.MISS);
    expect(b.sheet.pending()).toBeNull();

    const gone2 = vi.fn().mockResolvedValue({ pendingDamages: [] });
    const bare = await open({ api: { turnOptions: gone2 }, data: awaiting });
    await bare.settle();
    expect(bare.sheet.stage()).toBe('done');
  });

  it('ignores the answer for a damage the sheet has left', async () => {
    let answer: (v: unknown) => void = () => undefined;
    const turnOptions = vi.fn().mockReturnValue(new Promise((ok) => (answer = ok)));
    const { sheet, settle } = await open({ api: { turnOptions }, data: awaiting });
    sheet.pending.set(pendingDamage({ id: 'other' }));
    answer({ pendingDamages: [] });
    await settle();
    expect(sheet.pending().id).toBe('other');
  });
});

describe('AttackSheet: the roll a reaction window held', () => {
  const held = create(AttackRollSchema, {
    attackerId: 't',
    targetId: 'g',
    heldForReaction: true,
    d20: d20(1),
  });

  it("shows the question the combat now carries for the attacker, Lucky first and then the die's", async () => {
    const lucky = create(LuckyOfferSchema, { holdId: 'h1', targetId: 'g', d20: d20(1) });
    const onHeld = vi.fn();
    const { sheet, state, settle } = await open({ data: { onHeld } });
    sheet.roll.set(held);
    sheet.stage.set('done');
    state.apply(
      encounter({
        mode: EncounterMode.THEATRE,
        combatants: [{ ...toren, luckyOffer: lucky } as typeof toren, brisa, goblin],
        revision: 3,
      }),
    );
    await settle();
    expect(sheet.stage()).toBe('lucky');
    expect(onHeld).toHaveBeenCalledWith('h1');
  });

  it("shows the die's question when there is no Lucky one, and nothing when the combat carries none", async () => {
    const offer = create(InspirationOfferSchema, { holdId: 'h2', targetId: 'g', d20: d20(5) });
    const { sheet, state, settle } = await open();
    sheet.roll.set(held);
    sheet.stage.set('done');
    state.apply(
      encounter({ mode: EncounterMode.THEATRE, combatants: [toren, brisa, goblin], revision: 3 }),
    );
    await settle();
    expect(sheet.stage()).toBe('done');
    state.apply(
      encounter({
        mode: EncounterMode.THEATRE,
        combatants: [{ ...toren, inspirationOffer: offer } as typeof toren, brisa, goblin],
        revision: 4,
      }),
    );
    await settle();
    expect(sheet.stage()).toBe('inspire');
    expect(sheet.luckyOffer()).toBeNull();
  });

  it('says what the combat waits for while a reaction holds the result', async () => {
    const { sheet, state, settle } = await open();
    sheet.roll.set(held);
    state.apply(
      encounter({
        mode: EncounterMode.THEATRE,
        combatants: [toren, brisa, goblin],
        reactionWait: { titlePt: 'Esperando Brisa', detailPt: 'Ela decide.' } as never,
        revision: 3,
      }),
    );
    await settle();
    expect(sheet.held()).toEqual({ title: 'Esperando Brisa', detail: 'Ela decide.' });
    expect(sheet.outcome()).toBeNull();
  });
});

describe('AttackSheet: answering the questions', () => {
  const luckyOffer = create(LuckyOfferSchema, { holdId: 'h', targetId: 'g', d20: d20(1) });
  const dieOffer = create(InspirationOfferSchema, { holdId: 'h2', targetId: 'g', d20: d20(5) });
  const answered = (over: Record<string, unknown> = {}) => ({
    encounter: encounter({ combatants: [toren] }),
    roll: create(AttackRollSchema, {
      outcome: AttackOutcome.HIT,
      d20: d20(14, {
        rerolled: [{ index: 0, from: 1, to: 14, sourceKey: LUCKY_TRAIT }],
      }),
      bonusDice: [{ sides: 8, face: 5, used: true }],
    }),
    pending: undefined,
    ...over,
  });

  it('ignores a Lucky answer without a question, or while busy', async () => {
    const answerLucky = vi.fn();
    const { sheet } = await open({ resources: { answerLucky } });
    await sheet.answerLucky(true, null);
    sheet.luckyOffer.set(luckyOffer);
    sheet.busy.set(true);
    await sheet.answerLucky(true, null);
    expect(answerLucky).not.toHaveBeenCalled();
  });

  it('tumbles the new face when the app rolled again, and nothing for a typed one or none', async () => {
    const answerLucky = vi.fn().mockResolvedValue(answered());
    const { sheet } = await open({ resources: { answerLucky } });
    const play = vi.spyOn(sheet.animator, 'play');
    sheet.luckyOffer.set(luckyOffer);
    await sheet.answerLucky(true, { inApp: true });
    expect(play).toHaveBeenCalledTimes(1);
    sheet.luckyOffer.set(luckyOffer);
    await sheet.answerLucky(true, { face: 14 });
    expect(play).toHaveBeenCalledTimes(1);
  });

  it('tumbles nothing when the roll shows no Lucky reroll, or has no d20', async () => {
    const answerLucky = vi
      .fn()
      .mockResolvedValue(
        answered({ roll: create(AttackRollSchema, { outcome: AttackOutcome.MISS }) }),
      );
    const { sheet } = await open({ resources: { answerLucky } });
    const play = vi.spyOn(sheet.animator, 'play');
    sheet.luckyOffer.set(luckyOffer);
    await sheet.answerLucky(true, { inApp: true });
    expect(play).not.toHaveBeenCalled();
  });

  it("says when Lucky's answer fails", async () => {
    const answerLucky = vi.fn().mockRejectedValue(new ConnectError('x', Code.Unavailable));
    const { sheet } = await open({ resources: { answerLucky } });
    sheet.luckyOffer.set(luckyOffer);
    await sheet.answerLucky(false, null);
    expect(sheet.error()).not.toBe('');
  });

  it('ignores a die answer without a question, or while busy', async () => {
    const answerBardicInspiration = vi.fn();
    const { sheet } = await open({ resources: { answerBardicInspiration } });
    await sheet.answerInspiration(true, null);
    sheet.offer.set(dieOffer);
    sheet.busy.set(true);
    await sheet.answerInspiration(true, null);
    expect(answerBardicInspiration).not.toHaveBeenCalled();
  });

  it('tumbles the die the player added in the app, and not one typed or kept', async () => {
    const answerBardicInspiration = vi.fn().mockResolvedValue(answered());
    const { sheet } = await open({ resources: { answerBardicInspiration } });
    const play = vi.spyOn(sheet.animator, 'play');
    sheet.offer.set(dieOffer);
    await sheet.answerInspiration(true, { inApp: true });
    expect(play).toHaveBeenCalledTimes(1);
    sheet.offer.set(dieOffer);
    await sheet.answerInspiration(false, null);
    sheet.offer.set(dieOffer);
    await sheet.answerInspiration(true, { face: 3 });
    expect(play).toHaveBeenCalledTimes(1);
  });

  it("says when the die's answer fails", async () => {
    const answerBardicInspiration = vi
      .fn()
      .mockRejectedValue(new ConnectError('x', Code.Unavailable));
    const { sheet } = await open({ resources: { answerBardicInspiration } });
    sheet.offer.set(dieOffer);
    await sheet.answerInspiration(true, null);
    expect(sheet.error()).not.toBe('');
  });
});
