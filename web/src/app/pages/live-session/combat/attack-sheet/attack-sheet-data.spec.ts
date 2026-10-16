import { create } from '@bufbuild/protobuf';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import type {
  GetTurnOptionsResponse,
  InspirationOffer,
  LuckyOffer,
  PendingDamage,
  TargetInReach,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  AttackKind,
  AttackSchema,
  BonusAttackRule,
  TurnOptionsSchema,
} from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { CombatState } from '../../../../core/combat/combat-state';
import { combatant, encounter, reactionWindow } from '../../../../core/combat/combat-testing';
import {
  THROW_BACK_KEY,
  type AttackSheetRequest,
  attackAriaLabel,
  attackSheetData,
  throwBackAttack,
} from './attack-sheet-data';

const sword = create(AttackSchema, {
  key: 'attack:sword',
  name: 'Sword',
  namePt: 'Espada',
  kind: AttackKind.WEAPON,
});
const toren = combatant({ id: 't', label: 'Toren' });
const goblin = combatant({ id: 'g', label: 'Goblin' });
const enc = encounter({ round: 3, combatants: [toren, goblin] });
const inReach = (id: string) => ({ combatantId: id, label: id }) as unknown as TargetInReach;

function turn(over: Partial<GetTurnOptionsResponse> = {}, attacks = [{ attack: sword }]) {
  return {
    options: create(TurnOptionsSchema, { attacks }),
    attackTargets: [{ attackKey: 'attack:sword', targets: [inReach('g')] }],
    spellTargets: [],
    pendingDamages: [],
    ...over,
  } as unknown as GetTurnOptionsResponse;
}

function request(over: Partial<AttackSheetRequest> = {}): AttackSheetRequest {
  return {
    key: 'attack:sword',
    campaignId: 'c',
    encounter: enc,
    attacker: toren,
    options: turn(),
    diceMode: DiceMode.PLAYERS_CHOOSE,
    preference: DicePreference.APP,
    state: new CombatState(),
    master: false,
    asReaction: false,
    useExtraAction: false,
    catchWindowId: '',
    onHeld: () => undefined,
    ...over,
  };
}

describe('attackSheetData', () => {
  it('hands the sheet the attack, its targets and the plain defaults of a combatant with no economy', () => {
    const data = attackSheetData(request());
    expect(data).toMatchObject({
      campaignId: 'c',
      encounterId: 'enc',
      attackerId: 't',
      round: 3,
      attack: sword,
      asReaction: false,
      master: false,
      attacksLeft: 0,
      attacksPerAction: 1,
    });
    expect(data?.targets.map((t) => t.combatantId)).toEqual(['g']);
    expect(data).not.toHaveProperty('catchWindowId');
    expect(data).not.toHaveProperty('useExtraAction');
    expect(data?.resume).toBeUndefined();
    expect(data?.inspiration).toBeUndefined();
    expect(data?.lucky).toBeUndefined();
  });

  it('is null when the attack is not among the options', () => {
    expect(attackSheetData(request({ key: 'attack:axe' }))).toBeNull();
    expect(attackSheetData(request({ options: null }))).toBeNull();
    expect(attackSheetData(request({ options: undefined }))).toBeNull();
  });

  it('has no targets when the options list none for the attack', () => {
    expect(attackSheetData(request({ options: turn({ attackTargets: [] }) }))?.targets).toEqual([]);
  });

  it('takes the Extra Attack numbers from the economy and the bonus rule and beams from the option', () => {
    const options = turn({}, [
      {
        attack: sword,
        bonusRule: BonusAttackRule.OFF_HAND,
        bonusAttacksLeft: 2,
        beamsLeft: 3,
      } as never,
    ]);
    options.options!.economy = { attacksLeft: 1, attacksPerAction: 2 } as never;
    expect(attackSheetData(request({ options }))).toMatchObject({
      bonusRule: BonusAttackRule.OFF_HAND,
      bonusAttacksLeft: 2,
      beamsLeft: 3,
      attacksLeft: 1,
      attacksPerAction: 2,
    });
  });

  it("falls back to the attack's own beams when the option has none left", () => {
    const eldritch = create(AttackSchema, { key: 'attack:eldritch', beams: 4 });
    const options = turn({}, [{ attack: eldritch }]);
    expect(attackSheetData(request({ key: 'attack:eldritch', options }))?.beamsLeft).toBe(4);
  });

  it('carries the flags only when they are on', () => {
    const data = attackSheetData(
      request({ asReaction: true, master: true, useExtraAction: true, catchWindowId: 'w1' }),
    );
    expect(data).toMatchObject({
      asReaction: true,
      master: true,
      useExtraAction: true,
      catchWindowId: 'w1',
    });
  });

  it("names the target of a hit whose damage waits, even if it left the combat (then the name is '')", () => {
    const pending = { id: 'p1', targetId: 'g' } as unknown as PendingDamage;
    expect(attackSheetData(request({ resume: pending }))?.resume).toEqual({
      pending,
      targetLabel: 'Goblin',
    });
    const gone = { id: 'p2', targetId: 'ghost' } as unknown as PendingDamage;
    expect(attackSheetData(request({ resume: gone }))?.resume?.targetLabel).toBe('');
  });

  it('reopens at the held roll, Lucky and the die each with the name of the target', () => {
    const offer = { holdId: 'h1', targetId: 'g' } as unknown as InspirationOffer;
    const lucky = { holdId: 'h2', targetId: 'ghost' } as unknown as LuckyOffer;
    const data = attackSheetData(request({ held: { offer, lucky } }));
    expect(data?.inspiration).toEqual({ offer, targetLabel: 'Goblin' });
    expect(data?.lucky).toEqual({ offer: lucky, targetLabel: '' });
  });

  it('tells the page about a hold through onHeld', () => {
    const held: string[] = [];
    attackSheetData(request({ onHeld: (id) => held.push(id) }))?.onHeld?.('h9');
    expect(held).toEqual(['h9']);
  });

  it('uses the thrown attack and its own targets, which are not in the options', () => {
    const thrown = {
      attack: create(AttackSchema, { key: THROW_BACK_KEY }),
      targets: [inReach('g')],
    };
    const data = attackSheetData(
      request({ key: THROW_BACK_KEY, options: turn({}, []), thrown, catchWindowId: 'w2' }),
    );
    expect(data?.attack).toBe(thrown.attack);
    expect(data?.targets).toBe(thrown.targets);
    expect(data?.bonusRule).toBeUndefined();
  });
});

describe('attackAriaLabel', () => {
  it('names the weapon in Portuguese, falling back to the English name', () => {
    expect(attackAriaLabel(sword, false)).toBe('Atacar com Espada');
    expect(attackAriaLabel(create(AttackSchema, { name: 'Club' }), false)).toBe('Atacar com Club');
  });

  it("says whose creature attacks, and that an opportunity attack is one (the creature's name is not added)", () => {
    expect(attackAriaLabel(sword, false, goblin)).toBe('Atacar com Espada: Goblin');
    expect(attackAriaLabel(sword, true, goblin)).toBe('Ataque de oportunidade com Espada');
  });
});

describe('throwBackAttack', () => {
  const window = (prompt: unknown) => reactionWindow({ id: 'w', prompt: prompt as never });

  it('builds the thrown missile from the prompt and lists each target once', () => {
    const w = window({
      case: 'deflectThrow',
      value: { missileNamePt: 'Arco curto', attackBonus: 7, normalRangeFt: 20, longRangeFt: 60 },
    });
    const options = turn({
      attackTargets: [
        { attackKey: 'a', targets: [inReach('g'), inReach('t')] },
        { attackKey: 'b', targets: [inReach('g')] },
      ] as never,
    });
    const thrown = throwBackAttack(w, options);
    expect(thrown?.attack).toMatchObject({
      key: THROW_BACK_KEY,
      name: 'Arco curto',
      namePt: 'Arco curto',
      attackBonus: 7,
      kind: AttackKind.WEAPON,
      rangeFt: 20,
      longRangeFt: 60,
    });
    expect(thrown?.targets.map((t) => t.combatantId)).toEqual(['g', 't']);
  });

  it('calls a missile with no name a "Projétil" and has no targets without options', () => {
    const w = window({ case: 'deflectThrow', value: { missileNamePt: '' } });
    const thrown = throwBackAttack(w, null);
    expect(thrown?.attack.namePt).toBe('Projétil');
    expect(thrown?.targets).toEqual([]);
  });

  it('is null for another prompt, and for no window', () => {
    expect(throwBackAttack(window({ case: 'shield', value: {} }), turn())).toBeNull();
    expect(throwBackAttack(undefined, turn())).toBeNull();
  });
});
