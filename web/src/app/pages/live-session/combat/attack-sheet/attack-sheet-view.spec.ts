import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  AttackOutcome,
  AttackRollSchema,
  CombatantSide,
  DiceRollSchema,
  InspirationOfferSchema,
  PendingDamageSchema,
  PendingDamageStatus,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  RollMode,
  RollModeRequestStatus,
  type RollModeRequest,
} from '../../../../../gen/meurpg/play/v1/combat_rolls_pb';
import { BonusAttackRule } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { targetRows } from '../../../../core/combat/attack-flow';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import {
  afterDamageText,
  attackShow,
  attackTargetRows,
  brutalOf,
  currentRequest,
  damageDiceOf,
  damageHintOf,
  damageIntroOf,
  damageLabelOf,
  damageRangeOf,
  damageShow,
  extraDiceProblem,
  fixedTextOf,
  heldWait,
  inspirationShow,
  luckyRerollShow,
  modeChoiceOf,
  outcomeOf,
  rollRefusal,
  signedBonus,
  spentText,
  withAllies,
} from './attack-sheet-view';

const plain = (t: string) => t.replace(/ /g, ' ');

const reach = (id: string, over: Record<string, unknown> = {}) =>
  ({ combatantId: id, label: id.toUpperCase(), tooFar: false, ...over }) as never;

const d20 = (face: number, over: Record<string, unknown> = {}) =>
  create(DiceRollSchema, {
    diceCount: 1,
    diceSides: 20,
    faces: [face],
    modifier: 6,
    total: face + 6,
    ...over,
  });

const pending = (over: Record<string, unknown> = {}) =>
  create(PendingDamageSchema, {
    id: 'p1',
    targetId: 'g',
    status: PendingDamageStatus.AWAITING_ROLL,
    diceCount: 1,
    diceSides: 8,
    bonus: 3,
    ...over,
  } as never);

const brutal = pending({
  critical: true,
  criticalMax: 0,
  bonus: -2,
  extraDiceCount: 1,
  extraDiceNamePt: 'Crítico Brutal',
});

describe('withAllies', () => {
  const rows = targetRows([reach('b'), reach('g'), reach('ghost')], 5);

  it('tags a player or a party member with the gender of its name, and no one else', () => {
    const party = combatant({ id: 'b', label: 'Brisa', side: CombatantSide.PARTY });
    const dwarf = combatant({ id: 'g', label: 'Gimli', side: CombatantSide.PARTY });
    const tagged = withAllies(rows, [party, dwarf, combatant({ id: 'x', label: 'X' })]);
    expect(plain(tagged[0].sub)).toContain('Aliada');
    expect(plain(tagged[1].sub)).toContain('Aliado');
    expect(tagged[2].sub).not.toContain('Aliad');
    expect(withAllies(rows, [combatant({ id: 'b', label: 'Brisa' })])[0].sub).not.toContain(
      'Aliad',
    );
  });
});

describe('attackTargetRows', () => {
  const attack = { rangeFt: 5, longRangeFt: 60 };
  const targets = [
    reach('a', { distanceFt: 5 }),
    reach('b', { distanceFt: 30 }),
    reach('c'),
    reach('d', { tooFar: true }),
  ];
  const blocked = (rows: { id: string; blocked: string }[]) =>
    Object.fromEntries(rows.map((r) => [r.id, r.blocked !== '']));

  it('lists only the mover for an opportunity attack', () => {
    const rows = attackTargetRows({
      opportunity: { targetId: 'g', targetLabel: 'Goblin' },
      thrown: false,
      asReaction: false,
      targets,
      attack,
    });
    expect(rows).toEqual([
      { id: 'g', label: 'Goblin', sub: '', blocked: '', cover: '', coverMark: null },
    ]);
  });

  it('reaches the weapon range, 5 ft for a reaction, and the long range for the throw back', () => {
    const base = { opportunity: undefined, targets, attack };
    expect(blocked(attackTargetRows({ ...base, thrown: false, asReaction: false }))).toEqual({
      a: false,
      b: false,
      c: false,
      d: true,
    });
    expect(blocked(attackTargetRows({ ...base, thrown: false, asReaction: true }))).toEqual({
      a: false,
      b: true,
      c: true,
      d: true,
    });
    expect(blocked(attackTargetRows({ ...base, thrown: true, asReaction: false }))).toEqual({
      a: false,
      b: false,
      c: false,
      d: true,
    });
  });
});

describe('currentRequest', () => {
  const sent = { id: 'r1', status: RollModeRequestStatus.PENDING } as RollModeRequest;
  const withRequests = (list: unknown[]) => encounter({ rollModeRequests: list as never });

  it('is none before a request is sent', () => {
    expect(currentRequest(null, withRequests([]))).toBeNull();
  });

  it("is the combat's copy while it is open, none once it closed, and the one sent while the combat has not heard of it", () => {
    const open = { id: 'r1', status: RollModeRequestStatus.ANSWERED };
    expect(currentRequest(sent, withRequests([open]))).toBe(open);
    expect(
      currentRequest(sent, withRequests([{ id: 'r1', status: RollModeRequestStatus.CLOSED }])),
    ).toBeNull();
    expect(currentRequest(sent, withRequests([]))).toBe(sent);
    expect(currentRequest(sent, null)).toBe(sent);
  });
});

describe('modeChoiceOf', () => {
  const answered = (over: Record<string, unknown> = {}) =>
    ({
      id: 'r1',
      decidedMode: RollMode.ADVANTAGE,
      suggestedMode: RollMode.NORMAL,
      reason: 'escondido',
      ...over,
    }) as RollModeRequest;
  const base = {
    hasMode: true,
    answered: null,
    chosen: RollMode.NORMAL,
    suggested: RollMode.NORMAL,
    reason: ' x ',
  };

  it('says nothing without a mode to pick, or when the suggestion stands', () => {
    expect(modeChoiceOf({ ...base, hasMode: false })).toBeUndefined();
    expect(modeChoiceOf(base)).toBeUndefined();
  });

  it('sends the mode picked and why', () => {
    expect(modeChoiceOf({ ...base, chosen: RollMode.ADVANTAGE })).toEqual({
      mode: RollMode.ADVANTAGE,
      reason: 'x',
    });
  });

  it("sends the master's decision with the request, and the reason only when it differs from the suggestion", () => {
    expect(modeChoiceOf({ ...base, answered: answered() })).toEqual({
      mode: RollMode.ADVANTAGE,
      reason: 'escondido',
      requestId: 'r1',
    });
    expect(
      modeChoiceOf({ ...base, answered: answered({ suggestedMode: RollMode.ADVANTAGE }) })?.reason,
    ).toBe('');
  });
});

describe('the result of the roll', () => {
  const roll = (outcome: AttackOutcome, over: Record<string, unknown> = {}) =>
    create(AttackRollSchema, { outcome, d20: d20(15), ...over });

  it('tells the word, the hit and the critical of a roll that is told', () => {
    expect(outcomeOf(roll(AttackOutcome.HIT), false)).toMatchObject({ hit: true, crit: false });
    expect(outcomeOf(roll(AttackOutcome.CRITICAL_HIT), false)).toMatchObject({
      hit: true,
      crit: true,
    });
    expect(outcomeOf(roll(AttackOutcome.MISS), false)).toMatchObject({ hit: false });
  });

  it('tells nothing for a roll that is held, a question that waits, or no roll', () => {
    expect(outcomeOf(roll(AttackOutcome.HIT, { heldForReaction: true }), false)).toBeNull();
    expect(outcomeOf(roll(AttackOutcome.HIT), true)).toBeNull();
    expect(outcomeOf(null, false)).toBeNull();
  });

  it('says what the combat waits for while a reaction holds the roll, else a default', () => {
    const held = roll(AttackOutcome.UNSPECIFIED, { heldForReaction: true });
    expect(heldWait(roll(AttackOutcome.HIT), encounter({}))).toBeNull();
    expect(heldWait(null, null)).toBeNull();
    expect(heldWait(held, null)?.title).toBe('Esperando o mestre');
    expect(heldWait(held, encounter({}))?.title).toBe('Esperando o mestre');
    expect(
      heldWait(
        held,
        encounter({
          reactionWait: { titlePt: 'Esperando Brisa', detailPt: 'Ela decide.' } as never,
        }),
      ),
    ).toEqual({ title: 'Esperando Brisa', detail: 'Ela decide.' });
  });
});

describe('spentText', () => {
  const base = {
    byMaster: false,
    asReaction: false,
    bonusRule: undefined,
    bonusAttacksLeft: undefined,
    cantrip: false,
    name: 'Rajada Mística',
    beamsLeft: undefined,
    attacksLeft: undefined,
    attacksPerAction: undefined,
  };

  it("says the reaction, the master's or the player's", () => {
    expect(spentText({ ...base, byMaster: true })).toBe('A reação dele foi usada.');
    expect(spentText({ ...base, asReaction: true })).toBe('Sua reação foi usada.');
  });

  it('says the bonus action, and the flurry strikes that are left', () => {
    expect(spentText({ ...base, bonusRule: BonusAttackRule.OFF_HAND })).toBe(
      'Sua ação bônus foi usada.',
    );
    expect(
      spentText({ ...base, bonusRule: BonusAttackRule.FLURRY_OF_BLOWS, bonusAttacksLeft: 3 }),
    ).toContain('Rajada de Golpes');
  });

  it('counts the beams of a cantrip, only while more than one was left', () => {
    expect(spentText({ ...base, cantrip: true, beamsLeft: 3 })).toBe(
      'Rajada Mística: 2 raios restantes.',
    );
    expect(spentText({ ...base, cantrip: true, beamsLeft: 2 })).toBe(
      'Rajada Mística: 1 raio restante.',
    );
    expect(spentText({ ...base, cantrip: true, beamsLeft: 1 })).toBe('Sua ação foi usada.');
    expect(spentText({ ...base, cantrip: true })).toBe('Sua ação foi usada.');
  });

  it('counts the attacks of the action that are left, not for a cantrip', () => {
    expect(spentText({ ...base, attacksLeft: 3, attacksPerAction: 3 })).toBe(
      'Você ainda tem 2 ataques desta ação.',
    );
    expect(spentText({ ...base, attacksLeft: 2, attacksPerAction: 2 })).toBe(
      'Você ainda tem 1 ataque desta ação.',
    );
    expect(spentText({ ...base, attacksLeft: 1, attacksPerAction: 2 })).toBe('Sua ação foi usada.');
    expect(spentText({ ...base, attacksLeft: 3, attacksPerAction: 3, cantrip: true })).toBe(
      'Sua ação foi usada.',
    );
    expect(spentText({ ...base, attacksLeft: 3 })).toBe('Sua ação foi usada.');
  });
});

describe('the damage still to roll', () => {
  it('names the dice, with none before there is a damage', () => {
    expect(damageDiceOf(null)).toBeNull();
    expect(damageDiceOf(pending({ diceCount: 2, diceSides: 6 }))).toEqual({
      count: 2,
      sides: 6,
      bonus: 3,
      name: '2d6',
    });
  });

  it('knows the damage with extra dice on a critical, else none', () => {
    expect(brutalOf(null)).toBeNull();
    expect(brutalOf(pending())).toBeNull();
    expect(brutalOf(brutal)).toBe(brutal);
  });

  it("gives the range a typed sum may be: the groups' when brutal, the dice's, or 1", () => {
    expect(damageRangeOf(null, { count: 2, sides: 6 })).toEqual({ min: 2, max: 12 });
    expect(damageRangeOf(null, null)).toEqual({ min: 1, max: 1 });
    expect(damageRangeOf(brutal, null).max).toBeGreaterThan(0);
  });

  it('says what to roll: one die, two dice, the brutal groups, or nothing yet', () => {
    expect(damageLabelOf(null, { count: 1, name: '1d8' }, 'Espada')).toBe('Role 1d8 para o dano');
    expect(damageLabelOf(null, { count: 2, name: '2d8' }, 'Espada')).toBe(
      'Role 2d8 para o dano: some os dois',
    );
    expect(damageLabelOf(null, null, 'Espada')).toBe('Role  para o dano');
    expect(damageLabelOf(brutal, damageDiceOf(brutal), 'Espada')).not.toBe('');
  });

  it("says the hint of the typed field: the rule's, the brutal one, or the plain one", () => {
    const range = { min: 1, max: 8 };
    expect(damageHintOf(null, pending(), range)).toBeDefined();
    expect(damageHintOf(null, null, range)).toBeDefined();
    expect(damageHintOf(brutal, brutal, range)).not.toBe(damageHintOf(null, pending(), range));
  });

  it('says what the typed sum is added to', () => {
    expect(fixedTextOf(null, null)).toBeDefined();
    expect(fixedTextOf(null, pending({ criticalMax: 8 }))).toContain('8 do crítico');
    expect(fixedTextOf(brutal, brutal)).toBe('− 2 de bônus');
    expect(
      fixedTextOf(
        pending({ bonus: 3, criticalMax: 0, extraDiceCount: 1, extraDiceNamePt: 'Crítico Brutal' }),
        pending(),
      ),
    ).toBe('+ 3 de bônus');
    expect(fixedTextOf({ ...brutal, bonus: 0 }, brutal)).toBe('');
  });

  it('introduces the damage: nothing, "Acertou", or the doubled dice of a critical', () => {
    expect(damageIntroOf(null, false)).toBe('');
    expect(damageIntroOf(pending({ critical: false }), false)).toBe('Acertou: role o dano.');
    expect(damageIntroOf(pending({ critical: true }), false)).toContain('Acerto crítico');
    expect(damageIntroOf(pending({ critical: false }), true)).toContain('Acerto crítico');
  });

  it('says what the target has left once the damage is rolled', () => {
    const dealt = pending({
      status: PendingDamageStatus.ROLLED,
      roll: create(DiceRollSchema, { diceCount: 1, diceSides: 8, faces: [5], total: 8 }),
      targetDefeated: true,
    });
    expect(afterDamageText(null, null, 'Goblin')).toBe('');
    expect(
      afterDamageText(
        dealt,
        encounter({ combatants: [combatant({ id: 'g', label: 'Goblin' })] }),
        'Goblin',
      ),
    ).not.toBe('');
    expect(afterDamageText(dealt, null, 'Goblin')).not.toBe('');
  });
});

describe('the dice animated', () => {
  it('shows the d20 with its word when the sheet tells it, and the face alone when held', () => {
    const told = attackShow('Espada', {
      roll: create(AttackRollSchema, { outcome: AttackOutcome.CRITICAL_HIT, d20: d20(20) }),
    });
    expect(told?.critical).toBe(true);
    expect(told?.outcome).toBeDefined();
    const missed = attackShow('Espada', {
      roll: create(AttackRollSchema, { outcome: AttackOutcome.MISS, d20: d20(1) }),
    });
    expect(missed?.fumble).toBe(true);
    const held = attackShow('Espada', {
      roll: create(AttackRollSchema, {
        outcome: AttackOutcome.UNSPECIFIED,
        heldForReaction: true,
        d20: d20(9),
      }),
    });
    expect(held?.outcome).toBeUndefined();
    expect(held?.line).toBeUndefined();
    const asked = attackShow('Espada', {
      roll: create(AttackRollSchema, { outcome: AttackOutcome.HIT, d20: d20(9) }),
      offer: create(InspirationOfferSchema, { holdId: 'h' }),
    });
    expect(asked?.outcome).toBeUndefined();
    expect(attackShow('Espada', { roll: create(AttackRollSchema, {}) })).toBeNull();
  });

  it('shows the damage dice, but not for a damage that waits for a reaction', () => {
    const rolled = pending({
      status: PendingDamageStatus.ROLLED,
      damageTypePt: 'cortante',
      roll: create(DiceRollSchema, { diceCount: 1, diceSides: 8, faces: [5], total: 8 }),
    });
    expect(damageShow(rolled)?.note).toBe('cortante');
    expect(damageShow(undefined)).toBeNull();
    expect(damageShow(pending({ status: PendingDamageStatus.AWAITING_REACTION }))).toBeNull();
    expect(damageShow(pending({ status: PendingDamageStatus.ROLLED }))).toBeNull();
  });

  it('shows the new face Lucky rolled in the app, and nothing for a typed one, a plain roll or none', () => {
    const rerolled = create(AttackRollSchema, {
      d20: d20(14, { rerolled: [{ index: 0, from: 1, to: 14, sourceKey: 'trait:lucky' }] }),
    });
    expect(luckyRerollShow(rerolled, { inApp: true })).not.toBeNull();
    expect(luckyRerollShow(rerolled, { face: 14 })).toBeNull();
    expect(luckyRerollShow(rerolled, null)).toBeNull();
    expect(luckyRerollShow(create(AttackRollSchema, { d20: d20(14) }), { inApp: true })).toBeNull();
    expect(luckyRerollShow(create(AttackRollSchema, {}), { inApp: true })).toBeNull();
  });

  it('shows the Bardic Inspiration die the player added in the app', () => {
    const roll = create(AttackRollSchema, { bonusDice: [{ sides: 8, face: 5, used: true }] });
    expect(inspirationShow(roll, true, { inApp: true })).not.toBeNull();
    expect(inspirationShow(roll, true, { face: 5 })).toBeNull();
    expect(inspirationShow(roll, false, { inApp: true })).toBeNull();
    expect(inspirationShow(roll, true, null)).toBeNull();
    expect(
      inspirationShow(
        create(AttackRollSchema, { bonusDice: [{ sides: 8, face: 5, used: false }] }),
        true,
        { inApp: true },
      ),
    ).toBeNull();
  });
});

describe('what a refused roll says', () => {
  it('says how many more d4 the roll takes, one or several', () => {
    const takes = (n: number) =>
      new ConnectError(`the roll takes ${n} more die(s)`, Code.InvalidArgument);
    expect(rollRefusal(takes(1))).toEqual({
      more: 1,
      message: 'Esta rolagem leva mais um d4: role-o e digite o resultado.',
    });
    expect(rollRefusal(takes(2))).toEqual({
      more: 2,
      message: 'Esta rolagem leva mais 2 d4: role-os e digite os resultados.',
    });
  });

  it('says the refusal in words for any other error, with none to ask', () => {
    expect(rollRefusal(new ConnectError('x', Code.Unavailable)).more).toBe(0);
    expect(rollRefusal(new ConnectError('nothing to say', Code.InvalidArgument)).more).toBe(0);
    expect(rollRefusal(new Error('x')).message).not.toBe('');
  });
});

describe('extraDiceProblem', () => {
  const field = { key: 'e', label: 'Bênção', faces: 4 } as never;

  it('is empty when every d4 is typed', () => {
    expect(extraDiceProblem([field], [3])).toBe('');
    expect(extraDiceProblem([], [])).toBe('');
  });

  it('asks for the d4, or for each extra die', () => {
    expect(extraDiceProblem([field], [])).toBe('Digite o resultado do d4 antes de confirmar.');
    expect(extraDiceProblem([field], [null])).toBe('Digite o resultado do d4 antes de confirmar.');
    expect(extraDiceProblem([field, field], [3, null])).toBe(
      'Digite o resultado de cada dado extra antes de confirmar.',
    );
  });
});

describe('signedBonus', () => {
  it('writes a plus or a true minus sign', () => {
    expect(signedBonus(6)).toBe('+6');
    expect(signedBonus(0)).toBe('+0');
    expect(signedBonus(-1)).toBe('−1');
  });
});
