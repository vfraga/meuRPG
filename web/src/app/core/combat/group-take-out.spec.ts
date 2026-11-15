import {
  CombatLogKind,
  HitRiderChoice,
  HitRiderLogResult,
  CombatantKind,
  type CombatLogRound,
  PendingDamageStatus,
} from '../../../gen/meurpg/play/v1/combat_pb';
import {
  awaitedSumOf,
  keptRowsOf,
  removalFor,
  removalsOf,
  spellSavesOf,
  sumFormula,
  takeOutPlan,
} from './group-take-out';
import { combatant } from './combat-testing';

const part = (over: Record<string, unknown>) =>
  ({
    key: 'weapon',
    labelPt: 'Espada curta',
    diceCount: 1,
    diceSides: 6,
    flat: 3,
    damageTypeKey: 'damage-type:piercing',
    damageTypePt: 'perfurante',
    choosable: false,
    selected: false,
    available: false,
    auto: false,
    ...over,
  }) as never;

const roll = (over: Record<string, unknown>) =>
  ({
    partKey: 'weapon',
    labelPt: 'Espada curta',
    diceCount: 1,
    diceSides: 6,
    faces: [],
    flat: 3,
    sum: 10,
    counted: true,
    damageTypeKey: 'damage-type:piercing',
    rerolled: [],
    hitEffect: false,
    ...over,
  }) as never;

/** Espada curta 1d6 + 3 and Ataque Furtivo 3d6, one typed sum of 10 for both, and Geada no Punho 2d8 (6) frio. */
function damage(over: Record<string, unknown> = {}) {
  return {
    id: 'p1',
    attackerId: 'jab',
    targetId: 'zum',
    status: PendingDamageStatus.APPLIED,
    amount: 19,
    parts: [
      part({}),
      part({
        key: 'sneak-attack',
        labelPt: 'Ataque Furtivo',
        diceCount: 3,
        flat: 0,
        choosable: true,
        selected: true,
        available: true,
      }),
      part({
        key: 'frost',
        labelPt: 'Geada no Punho',
        diceCount: 2,
        diceSides: 8,
        flat: 0,
        auto: true,
        damageTypeKey: 'damage-type:cold',
        damageTypePt: 'frio',
      }),
    ],
    partRolls: [
      roll({}),
      roll({ partKey: 'sneak-attack', labelPt: 'Ataque Furtivo', diceCount: 3, flat: 0, sum: 0 }),
      roll({
        partKey: 'frost',
        labelPt: 'Geada no Punho',
        diceCount: 2,
        diceSides: 8,
        faces: [3, 3],
        flat: 0,
        sum: 6,
        damageTypeKey: 'damage-type:cold',
        hitEffect: true,
      }),
    ],
    ...over,
  } as never;
}

const entry = (over: Record<string, unknown>) =>
  ({
    kind: CombatLogKind.DAMAGE_PART_REMOVED,
    actorId: 'jab',
    targetId: 'zum',
    key: 'sneak-attack',
    keyNamePt: 'Ataque Furtivo',
    reason: 'Sem aliado',
    ...over,
  }) as never;

describe('removalsOf and removalFor', () => {
  it('reads the removal lines that carry the part key, newest first, never those of the other players', () => {
    const rounds = [
      { round: 2, entries: [entry({ reason: 'segunda' }), entry({ key: '', reason: '' })] },
      { round: 1, entries: [entry({ reason: 'primeira' })] },
    ] as unknown as CombatLogRound[];
    const all = removalsOf(rounds);
    expect(all.map((r) => r.reason)).toEqual(['segunda', 'primeira']);
    expect(removalFor(damage(), all)?.reason).toBe('segunda');
    expect(removalFor(damage({ attackerId: 'other' }), all)).toBeUndefined();
    expect(removalFor(damage({ targetId: 'other' }), all)).toBeUndefined();
  });

  it('drops a removal that an attack by the same one on the same target came after: it is of an earlier attack', () => {
    const attack = (over: Record<string, unknown> = {}) =>
      ({ kind: CombatLogKind.ATTACK, actorId: 'jab', targetId: 'zum', ...over }) as never;
    const all = removalsOf([
      {
        round: 2,
        entries: [attack(), entry({ reason: 'nova' }), attack({ targetId: 'other' })],
      },
      { round: 1, entries: [entry({ reason: 'velha' })] },
    ] as unknown as CombatLogRound[]);
    // The newest attack (round 2) stands before 'nova' in the list, so 'nova' and 'velha' are both of an earlier attack.
    expect(all).toEqual([]);
    const kept = removalsOf([
      { round: 2, entries: [entry({ reason: 'nova' }), attack()] },
      { round: 1, entries: [entry({ reason: 'velha' }), attack()] },
    ] as unknown as CombatLogRound[]);
    expect(kept.map((r) => r.reason)).toEqual(['nova']);
  });

  it('does not match a removal whose part the damage has not', () => {
    const all = removalsOf([{ round: 1, entries: [entry({ key: 'rage' })] }] as never);
    expect(removalFor(damage(), all)).toBeUndefined();
  });
});

const saveEntry = (over: Record<string, unknown> = {}, rider: Record<string, unknown> = {}) =>
  ({
    kind: CombatLogKind.HIT_RIDER,
    actorId: 'jab',
    targetId: 'zum',
    key: 'frost',
    keyNamePt: 'Geada no Punho',
    hitRider: {
      choice: HitRiderChoice.APPLY,
      result: HitRiderLogResult.FAILED,
      conditionNamePt: 'Impedido',
      ...rider,
    },
    ...over,
  }) as never;

describe('spellSavesOf', () => {
  it('reads the saves of spell hit effects with the condition they gave, newest first', () => {
    const saves = spellSavesOf([
      { round: 2, entries: [saveEntry(), entry({})] },
      { round: 1, entries: [saveEntry({ targetId: 'old' }, { conditionNamePt: '' })] },
    ] as never);
    expect(saves).toEqual([
      { attackerId: 'jab', targetId: 'zum', key: 'frost', condition: 'Impedido' },
      { attackerId: 'jab', targetId: 'old', key: 'frost', condition: '' },
    ]);
  });
});

describe('awaitedSumOf', () => {
  const waiting = damage({
    status: PendingDamageStatus.AWAITING_GROUP_SUM,
    awaitedDamageTypeKey: 'damage-type:piercing',
    parts: [
      part({}),
      part({
        key: 'sneak-attack',
        labelPt: 'Ataque Furtivo',
        diceCount: 3,
        flat: 0,
        choosable: true,
        selected: false,
        available: false,
      }),
      part({
        key: 'frost',
        labelPt: 'Geada no Punho',
        diceCount: 2,
        diceSides: 8,
        flat: 0,
        auto: true,
        damageTypeKey: 'damage-type:cold',
        damageTypePt: 'frio',
      }),
    ],
    partRolls: [
      roll({
        partKey: 'frost',
        labelPt: 'Geada no Punho',
        diceCount: 2,
        diceSides: 8,
        faces: [3, 3],
        flat: 0,
        sum: 6,
        damageTypeKey: 'damage-type:cold',
        hitEffect: true,
      }),
    ],
  });

  it('asks one sum for the parts of the awaited type that are left, without the part taken out', () => {
    const a = awaitedSumOf(waiting)!;
    expect(a.label).toBe('Espada curta, perfurante: 1d6');
    expect(a.hint).toBe('De 1 a 6.');
    expect([a.min, a.max, a.flat, a.dice]).toEqual([1, 6, 3, '1d6']);
  });

  it('names the parts when the group continues with two, and merges their dice', () => {
    const two = damage({
      ...(waiting as object),
      parts: [
        part({}),
        part({
          key: 'mark',
          labelPt: 'Sinal da Presa',
          diceCount: 1,
          flat: 0,
          choosable: true,
          selected: true,
          available: true,
        }),
        part({
          key: 'frost',
          diceCount: 2,
          diceSides: 8,
          flat: 0,
          auto: true,
          damageTypeKey: 'damage-type:cold',
          damageTypePt: 'frio',
        }),
      ],
    });
    const a = awaitedSumOf(two)!;
    expect(a.label).toBe('Espada curta e Sinal da Presa, perfurante: 2d6');
    expect(a.hint).toBe('De 2 a 12.');
  });

  it('is nothing for a damage that is not awaiting a group sum, even when it names a damage type', () => {
    expect(awaitedSumOf(damage())).toBeNull();
    expect(
      awaitedSumOf({
        ...(waiting as object),
        status: PendingDamageStatus.ROLLED,
      } as never),
    ).toBeNull();
  });

  it('says "Deve ser N." when the dice can only make one sum', () => {
    const fixed = {
      ...(waiting as object),
      parts: [part({ diceCount: 2, diceSides: 1, flat: 0 })],
    } as never;
    expect(awaitedSumOf(fixed)!.hint).toBe('Deve ser 2.');
  });

  it('lists the sums kept and writes the live total with them', () => {
    expect(keptRowsOf(waiting)).toEqual([
      { label: 'Geada no Punho', formula: '2d8', typePt: 'frio', total: 6, auto: true },
    ]);
    expect(sumFormula(awaitedSumOf(waiting)!, keptRowsOf(waiting), 4)).toEqual({
      text: 'Dano: 4 + 3 + 6 de frio = 13',
      total: 13,
    });
    // A negative fixed number is said with the true minus sign, never "+ −1".
    expect(sumFormula({ ...awaitedSumOf(waiting)!, flat: -1 }, keptRowsOf(waiting), 4).text).toBe(
      'Dano: 4 − 1 + 6 de frio = 9',
    );
  });
});

describe('takeOutPlan', () => {
  const jab = combatant({ id: 'jab', label: 'Jabuti', kind: CombatantKind.PLAYER });
  const zum = (over = {}) =>
    combatant({
      id: 'zum',
      label: 'Zumbi 1',
      hitPointsCurrent: 3,
      hitPointsMax: 22,
      hitPointsTemporary: 0,
      ...over,
    } as never);

  it('lists what taking an extra out of an applied group undoes, then the new sum it asks', () => {
    const p = damage();
    const plan = takeOutPlan((p as never as { partRolls: never[] }).partRolls[1], p, jab, zum());
    expect(plan.group).toBe(true);
    expect(plan.title).toBe('Tirar o Ataque Furtivo do dano?');
    expect(plan.confirm).toBe('Tirar e pedir a soma');
    expect(plan.lines).toEqual([
      'Desfaz os 19 de dano: o Zumbi 1 volta a 22 PV.',
      'Desfaz o efeito de Geada no Punho no alvo; ele vale de novo com a nova soma.',
      'Devolve o Ataque Furtivo deste turno.',
      'Pede a Jabuti só a soma de perfurante (1d6).',
    ]);
    expect(plan.notice).toBe('');
  });

  it('says the save of the hit effect and its condition when the log has them, the effect in general when not', () => {
    const p = damage();
    const rolls = (p as never as { partRolls: never[] }).partRolls;
    const save = { attackerId: 'jab', targetId: 'zum', key: 'frost', condition: 'Impedido' };
    expect(takeOutPlan(rolls[1], p, jab, zum(), [save]).lines[1]).toBe(
      'Desfaz o teste da Geada no Punho (Impedido).',
    );
    expect(takeOutPlan(rolls[1], p, jab, zum(), [{ ...save, condition: '' }]).lines[1]).toBe(
      'Desfaz o teste da Geada no Punho.',
    );
    // A save of another attacker, another target or another spell is not this damage's.
    expect(takeOutPlan(rolls[1], p, jab, zum(), [{ ...save, attackerId: 'other' }]).lines[1]).toBe(
      'Desfaz o efeito de Geada no Punho no alvo; ele vale de novo com a nova soma.',
    );
    expect(takeOutPlan(rolls[1], p, jab, zum(), [{ ...save, targetId: 'other' }]).lines[1]).toBe(
      'Desfaz o efeito de Geada no Punho no alvo; ele vale de novo com a nova soma.',
    );
  });

  it('never says the target comes back above its maximum, and leaves out an extra that no longer counts from the sum asked', () => {
    const p = damage();
    const parts = (p as never as { parts: unknown[] }).parts;
    const withDead = {
      ...(p as object),
      parts: [
        ...parts,
        part({
          key: 'other',
          labelPt: 'Outro',
          diceCount: 4,
          choosable: true,
          selected: true,
          available: false,
        }),
      ],
    } as never;
    const rolls = (p as never as { partRolls: never[] }).partRolls;
    const plan = takeOutPlan(rolls[1], withDead, jab, zum({ hitPointsCurrent: 10 }));
    expect(plan.lines[0]).toBe('Desfaz os 19 de dano: o Zumbi 1 volta a 22 PV.');
    expect(plan.lines.at(-1)).toBe('Pede a Jabuti só a soma de perfurante (1d6).');
  });

  it('does not name the hit points it cannot know when the target is down or has temporary points', () => {
    const p = damage();
    const temp = takeOutPlan(
      (p as never as { partRolls: never[] }).partRolls[1],
      p,
      jab,
      zum({ hitPointsTemporary: 5 }),
    );
    expect(temp.lines[0]).toBe('Desfaz os 19 de dano: o Zumbi 1 recupera os PV que perdeu.');
    const down = takeOutPlan(
      (p as never as { partRolls: never[] }).partRolls[1],
      p,
      jab,
      zum({ hitPointsCurrent: 0 }),
    );
    expect(down.lines[0]).toBe('Desfaz os 19 de dano: o Zumbi 1 recupera os PV que perdeu.');
  });

  it('says nothing is undone for a damage that was only rolled', () => {
    const p = damage({ status: PendingDamageStatus.ROLLED });
    const plan = takeOutPlan((p as never as { partRolls: never[] }).partRolls[1], p, jab, zum());
    expect(plan.lines).toEqual([]);
    expect(plan.notice).toBe(
      'O dano ainda não foi aplicado: nada é desfeito. Jabuti digita de novo só a soma de perfurante, e o mestre aplica como sempre.',
    );
  });

  it('keeps the plain question for a part alone in its type', () => {
    const lone = damage({
      amount: 16,
      parts: [
        part({}),
        part({
          key: 'smite',
          labelPt: 'Golpe Divino',
          diceCount: 2,
          diceSides: 8,
          flat: 0,
          choosable: true,
          selected: true,
          available: true,
          damageTypeKey: 'damage-type:radiant',
          damageTypePt: 'radiante',
        }),
      ],
      partRolls: [
        roll({ sum: 4 }),
        roll({
          partKey: 'smite',
          labelPt: 'Golpe Divino',
          diceCount: 2,
          diceSides: 8,
          faces: [4, 5],
          flat: 0,
          sum: 9,
          damageTypeKey: 'damage-type:radiant',
        }),
      ],
    });
    const plan = takeOutPlan(
      (lone as never as { partRolls: never[] }).partRolls[1],
      lone,
      jab,
      zum(),
    );
    expect(plan.group).toBe(false);
    expect(plan.title).toBe('Tirar o Golpe Divino? O dano cai de 16 para 7.');
    expect(plan.confirm).toBe('Tirar');
  });
});
