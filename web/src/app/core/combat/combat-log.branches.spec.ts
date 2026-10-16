// The lines of `combat-log.ts` the other specs never read (found with the coverage report): what an undo names for each
// kind of entry, the hit-point spells for the master and for a player, the pool card, the monk's riders, the states and
// the small kinds. Each expectation is the text the code wrote before `undoLabel` and the spell lines were split into one
// function per piece, recorded as an inline snapshot.
import { type MessageInitShape, create } from '@bufbuild/protobuf';

import { CombatantStateKind } from '../../../gen/meurpg/play/v1/combat_rolls_pb';
import {
  AttackOutcome,
  type CombatLogEntry,
  CombatLogEntrySchema,
  CombatLogKind,
  DeathSaveOutcome,
  HitRiderChoice,
  HitRiderLogResult,
  PendingDamageStatus,
  SpellEffectGain,
  SpellEffectKind,
  SpellEffectOutcome,
  SpellEffectReason,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { logLine, undoLabel } from './combat-log';

type Over = MessageInitShape<typeof CombatLogEntrySchema>;
const entry = (over: Over): CombatLogEntry =>
  create(CombatLogEntrySchema, { id: 'e1', ...over } as never);

const MASTER = { master: true, players: new Set(['Toren']) };
const PLAYER = { master: false, players: new Set(['Toren']) };
const text = (over: Over, ctx = MASTER) => logLine(entry(over), '', ctx)?.text;

describe('undoLabel', () => {
  const label = (over: Over) => undoLabel(entry(over));

  it('names an attack with its target and what it did', () => {
    expect(
      label({ kind: CombatLogKind.ATTACK, actorLabel: 'Capitão Goblin', targetLabel: 'Toren' }),
    ).toMatchInlineSnapshot(`"o ataque do Capitão Goblin ao Toren"`);
    expect(
      label({ kind: CombatLogKind.ATTACK, actorLabel: 'Brisa', targetLabel: 'Goblin' }),
    ).toMatchInlineSnapshot(`"o ataque da Brisa ao Goblin"`);
    expect(
      label({ kind: CombatLogKind.ATTACK, actorLabel: 'Goblin', targetLabel: 'Brisa' }),
    ).toMatchInlineSnapshot(`"o ataque do Goblin à Brisa"`);
    expect(label({ kind: CombatLogKind.ATTACK, actorLabel: 'Goblin' })).toMatchInlineSnapshot(
      `"o ataque do Goblin ao alguém"`,
    );
  });

  it('adds the damage once rolled, "errou" for a miss, and nothing while the damage waits', () => {
    const hit = { kind: CombatLogKind.ATTACK, actorLabel: 'Goblin', targetLabel: 'Toren' };
    expect(
      label({
        ...hit,
        outcome: AttackOutcome.HIT,
        damage: { status: PendingDamageStatus.APPLIED, amount: 5 } as never,
      }),
    ).toMatchInlineSnapshot(`"o ataque do Goblin ao Toren (5 de dano)"`);
    expect(
      label({
        ...hit,
        outcome: AttackOutcome.HIT,
        damage: { status: PendingDamageStatus.AWAITING_ROLL, amount: 0 } as never,
      }),
    ).toMatchInlineSnapshot(`"o ataque do Goblin ao Toren"`);
    expect(label({ ...hit, outcome: AttackOutcome.MISS })).toMatchInlineSnapshot(
      `"o ataque do Goblin ao Toren (errou)"`,
    );
    expect(
      label({
        ...hit,
        outcome: AttackOutcome.MISS,
        damage: { status: PendingDamageStatus.APPLIED, amount: 5 } as never,
      }),
    ).toMatchInlineSnapshot(`"o ataque do Goblin ao Toren (errou)"`);
    expect(label({ ...hit, outcome: AttackOutcome.HIT })).toMatchInlineSnapshot(
      `"o ataque do Goblin ao Toren"`,
    );
  });

  it('names the other kinds', () => {
    expect(
      label({ kind: CombatLogKind.ACTION, actorLabel: 'Toren', keyNamePt: 'Correr' }),
    ).toMatchInlineSnapshot(`"a ação de Toren (Correr)"`);
    expect(label({ kind: CombatLogKind.ACTION, actorLabel: 'Toren' })).toMatchInlineSnapshot(
      `"a ação de Toren"`,
    );
    expect(
      label({
        kind: CombatLogKind.HIT_POINTS_ADJUSTED,
        actorLabel: 'Toren',
        targetLabel: 'Goblin',
      }),
    ).toMatchInlineSnapshot(`"o ajuste de PV de Toren"`);
    expect(
      label({ kind: CombatLogKind.HIT_POINTS_ADJUSTED, targetLabel: 'Goblin' }),
    ).toMatchInlineSnapshot(`"o ajuste de PV de Goblin"`);
    expect(
      label({ kind: CombatLogKind.SPELL_CAST, actorLabel: 'Brisa', keyNamePt: 'Sono' }),
    ).toMatchInlineSnapshot(`"a magia Sono da Brisa"`);
    expect(label({ kind: CombatLogKind.SPELL_CAST, actorLabel: 'Brisa' })).toMatchInlineSnapshot(
      `"a magia da Brisa"`,
    );
    expect(label({ kind: CombatLogKind.REACTION, actorLabel: 'Brisa' })).toMatchInlineSnapshot(
      `"a reação da Brisa"`,
    );
    expect(label({ kind: CombatLogKind.REACTION, actorLabel: 'Toren' })).toMatchInlineSnapshot(
      `"a reação do Toren"`,
    );
    expect(
      label({ kind: CombatLogKind.OPPORTUNITY_OFFERED, targetLabel: 'Toren' }),
    ).toMatchInlineSnapshot(`"a oferta de ataque de oportunidade a Toren"`);
    expect(label({ kind: CombatLogKind.OPPORTUNITY_OFFERED })).toMatchInlineSnapshot(
      `"a oferta de ataque de oportunidade a alguém"`,
    );
    expect(label({ kind: CombatLogKind.DEATH_SAVE, actorLabel: 'Toren' })).toMatchInlineSnapshot(
      `"o teste contra a morte do Toren"`,
    );
    expect(
      label({ kind: CombatLogKind.CONDITIONS_CHANGED, targetLabel: 'Brisa' }),
    ).toMatchInlineSnapshot(`"a mudança de condições da Brisa"`);
    expect(
      label({ kind: CombatLogKind.CONDITIONS_CHANGED, targetLabel: 'Toren' }),
    ).toMatchInlineSnapshot(`"a mudança de condições do Toren"`);
    expect(label({ kind: CombatLogKind.COMBAT_ENDED })).toMatchInlineSnapshot(`"a última ação"`);
  });
});

describe('the small kinds', () => {
  it('writes a death save with no data, and the standard actions by key', () => {
    expect(text({ kind: CombatLogKind.DEATH_SAVE, actorLabel: 'Toren' })).toMatchInlineSnapshot(
      `" faz um teste contra a morte"`,
    );
    const keys = [
      'standard:hide',
      'standard:search',
      'standard:ready',
      'standard:use-an-object',
      'standard:other',
    ];
    expect(
      keys.map((key) =>
        text({ kind: CombatLogKind.ACTION, actorLabel: 'Toren', key, keyNamePt: 'Outra' }),
      ),
    ).toMatchInlineSnapshot(`
      [
        " se esconde",
        " procura ao redor",
        " se prepara",
        " usa um objeto",
        " usa Outra",
      ]
    `);
    expect(
      text({ kind: CombatLogKind.ACTION, actorLabel: 'Toren', key: 'x' }),
    ).toMatchInlineSnapshot(`" usa uma ação"`);
  });

  it('writes the hit points the master changed, down, up or by zero, with or without the total', () => {
    const base = { kind: CombatLogKind.HIT_POINTS_ADJUSTED, targetLabel: 'Goblin' };
    expect(text({ ...base, hitPointsDelta: -3, hitPointsAfter: 4 })).toMatchInlineSnapshot(
      `" perdeu 3 PV, por ajuste do mestre, agora com 4 PV"`,
    );
    expect(text({ ...base, hitPointsDelta: 3, hitPointsAfter: 10 })).toMatchInlineSnapshot(
      `" recuperou 3 PV, por ajuste do mestre, agora com 10 PV"`,
    );
    expect(text({ ...base, hitPointsDelta: 3 })).toMatchInlineSnapshot(
      `" recuperou 3 PV, por ajuste do mestre"`,
    );
    expect(text({ ...base, hitPointsDelta: 0, hitPointsAfter: 7 })).toMatchInlineSnapshot(
      `" teve os PV ajustados pelo mestre, agora com 7 PV"`,
    );
  });

  it('writes the end of a part of a joint turn, in the gender of the name', () => {
    expect(
      text({ kind: CombatLogKind.TURN_PART_ENDED, actorLabel: 'Brisa' }),
    ).toMatchInlineSnapshot(`" encerrou a parte dela"`);
    expect(
      text({ kind: CombatLogKind.TURN_PART_ENDED, actorLabel: 'Toren' }),
    ).toMatchInlineSnapshot(`" encerrou a parte dele"`);
  });

  it('writes the states that began and ended, with the reason of each', () => {
    const state = (kind: CombatantStateKind, started: boolean, reason = '') =>
      text({
        kind: CombatLogKind.STATE_CHANGED,
        actorLabel: 'Toren',
        state: { kind, started, reason } as never,
      });
    expect(state(CombatantStateKind.RAGE, true)).toMatchInlineSnapshot(`"Fúria de Toren começou"`);
    expect(state(CombatantStateKind.HUNTERS_MARK_TARGET, true)).toMatchInlineSnapshot(
      `"Marca de Toren começou"`,
    );
    expect(state(CombatantStateKind.DODGING, true)).toMatchInlineSnapshot(
      `"Esquiva de Toren começou"`,
    );
    expect(state(CombatantStateKind.RECKLESS, true)).toMatchInlineSnapshot(
      `"Ataque descuidado de Toren começou"`,
    );
    expect(state(CombatantStateKind.UNSPECIFIED, true)).toMatchInlineSnapshot(
      `"Estado de Toren começou"`,
    );
    const reasons = ['no_attack', 'unconscious', 'by_choice', 'duration', 'left', 'other', ''];
    expect(reasons.map((reason) => state(CombatantStateKind.RAGE, false, reason)))
      .toMatchInlineSnapshot(`
      [
        "Fúria de Toren acabou (não atacou nem sofreu dano)",
        "Fúria de Toren acabou (ficou inconsciente)",
        "Fúria de Toren acabou (por escolha)",
        "Fúria de Toren acabou (a duração acabou)",
        "Fúria de Toren acabou (a concentração ou o combate acabou)",
        "Fúria de Toren acabou (other)",
        "Fúria de Toren acabou",
      ]
    `);
    expect(
      text({
        kind: CombatLogKind.STATE_CHANGED,
        state: { kind: CombatantStateKind.RAGE, started: true } as never,
      }),
    ).toMatchInlineSnapshot(`"Fúria começou"`);
    expect(logLine(entry({ kind: CombatLogKind.STATE_CHANGED }))).toMatchInlineSnapshot(`null`);
  });

  it("writes the monk's riders: declined, each technique, each result", () => {
    const rider = (over: Record<string, unknown>) =>
      text({
        kind: CombatLogKind.HIT_RIDER,
        actorLabel: 'Toren',
        targetLabel: 'Goblin',
        hitRider: over as never,
      });
    expect(text({ kind: CombatLogKind.HIT_RIDER, actorLabel: 'Toren' })).toMatchInlineSnapshot(
      `" dispensou a técnica em alguém"`,
    );
    expect(rider({ choice: HitRiderChoice.DECLINE })).toMatchInlineSnapshot(
      `" dispensou a técnica em Goblin"`,
    );
    const choices = [
      HitRiderChoice.PRONE,
      HitRiderChoice.PUSH,
      HitRiderChoice.NO_REACTIONS,
      HitRiderChoice.STUN,
    ];
    expect(
      choices.flatMap((choice) => [
        rider({
          choice,
          result: HitRiderLogResult.SAVED,
          numbers: true,
          d20: 14,
          saveTotal: 17,
          dc: 15,
        }),
        rider({
          choice,
          result: HitRiderLogResult.FAILED,
          numbers: true,
          d20: 4,
          saveTotal: 7,
          dc: 15,
        }),
        rider({ choice, result: HitRiderLogResult.FAILED }),
        rider({ choice }),
      ]),
    ).toMatchInlineSnapshot(`
      [
        " usou a Técnica da Mão Aberta (Derrubar) em Goblin: Goblin passou no teste (1d20 14 = 17 contra CD 15)",
        " usou a Técnica da Mão Aberta (Derrubar) em Goblin: Goblin falhou no teste (1d20 4 = 7 contra CD 15)",
        " usou a Técnica da Mão Aberta (Derrubar) em Goblin: Goblin falhou no teste",
        " usou a Técnica da Mão Aberta (Derrubar) em Goblin",
        " usou a Técnica da Mão Aberta (Empurrar até 4,5 m) em Goblin: Goblin passou no teste (1d20 14 = 17 contra CD 15)",
        " usou a Técnica da Mão Aberta (Empurrar até 4,5 m) em Goblin: Goblin falhou no teste (1d20 4 = 7 contra CD 15); o mestre posiciona a criatura até 4,5 m mais longe",
        " usou a Técnica da Mão Aberta (Empurrar até 4,5 m) em Goblin: Goblin falhou no teste; o mestre posiciona a criatura até 4,5 m mais longe",
        " usou a Técnica da Mão Aberta (Empurrar até 4,5 m) em Goblin",
        " usou a Técnica da Mão Aberta (sem reações até o fim do próximo turno) em Goblin: Goblin passou no teste (1d20 14 = 17 contra CD 15)",
        " usou a Técnica da Mão Aberta (sem reações até o fim do próximo turno) em Goblin: Goblin falhou no teste (1d20 4 = 7 contra CD 15)",
        " usou a Técnica da Mão Aberta (sem reações até o fim do próximo turno) em Goblin: Goblin falhou no teste",
        " usou a Técnica da Mão Aberta (sem reações até o fim do próximo turno) em Goblin",
        " usou o Golpe Atordoante em Goblin: Goblin passou no teste (1d20 14 = 17 contra CD 15)",
        " usou o Golpe Atordoante em Goblin: Goblin falhou no teste (1d20 4 = 7 contra CD 15)",
        " usou o Golpe Atordoante em Goblin: Goblin falhou no teste",
        " usou o Golpe Atordoante em Goblin",
      ]
    `);
    expect(rider({ choice: 99 })).toMatchInlineSnapshot(`" usou a técnica em Goblin"`);
    expect(
      text({
        kind: CombatLogKind.HIT_RIDER,
        hitRider: { choice: HitRiderChoice.DECLINE } as never,
      }),
    ).toMatchInlineSnapshot(`" dispensou a técnica em alguém"`);
  });

  it('writes the dead and the death saves that went on', () => {
    const save = (over: Record<string, unknown>) =>
      text({ kind: CombatLogKind.DEATH_SAVE, actorLabel: 'Toren', deathSave: over as never });
    expect(save({ stable: true })).toMatchInlineSnapshot(`" estabilizou"`);
    expect(
      save({ outcome: DeathSaveOutcome.FAILURE, failures: 1, stable: false, dying: true }),
    ).toMatchInlineSnapshot(
      `" faz um teste contra a morte: falha (0 sucessos, 1 falha). Morrendo: o mestre confirma a morte"`,
    );
    expect(save({ outcome: DeathSaveOutcome.REVIVED })).toMatchInlineSnapshot(
      `" faz um teste contra a morte: volta com 1 PV"`,
    );
    expect(save({ outcome: DeathSaveOutcome.CRITICAL_FAILURE, failures: 2 })).toMatchInlineSnapshot(
      `" faz um teste contra a morte: falha crítica, conta duas falhas (0 sucessos, 2 falhas)"`,
    );
    expect(
      save({ outcome: DeathSaveOutcome.SUCCESS, successes: 3, stable: true }),
    ).toMatchInlineSnapshot(
      `" faz um teste contra a morte: sucesso (3 sucessos, 0 falhas). Estável: não rola mais"`,
    );
  });
});

describe('the spells that read hit points', () => {
  const target = (label: string, effect: Record<string, unknown> | undefined) => ({
    targetId: label,
    targetLabel: label,
    effect: effect as never,
  });
  const cast = (spell: Record<string, unknown>, ctx = MASTER) =>
    logLine(
      entry({
        kind: CombatLogKind.SPELL_CAST,
        actorLabel: 'Pensantus',
        keyNamePt: 'Sono',
        spell: spell as never,
      }),
      '',
      ctx,
    );
  const AFFECTED = SpellEffectOutcome.AFFECTED;
  const NOT = SpellEffectOutcome.NOT_AFFECTED;

  it('lists, for the master, each creature with its hit points, the limit and what happened', () => {
    const spell = {
      effectKind: SpellEffectKind.THRESHOLD,
      effectConditionKey: 'condition:stunned',
      effectThreshold: 150,
      targets: [
        target('Capitão Goblin', { outcome: AFFECTED, hitPointsBefore: 27 }),
        target('Toren', {
          outcome: NOT,
          reason: SpellEffectReason.ABOVE_LIMIT,
          hitPointsBefore: 200,
        }),
        target('', { outcome: NOT }),
        target('Orc', undefined),
      ],
    };
    expect(cast(spell)).toMatchInlineSnapshot(`
      {
        "actor": "Pensantus",
        "card": undefined,
        "hidden": false,
        "icon": "auto_awesome",
        "id": "e1",
        "text": " conjura Sono no Capitão Goblin (27 PV, limite de 150): fica atordoado; em Toren (200 PV, limite de 150): não é afetado: 200 PV, acima do limite de 150; no alguém (limite de 150): não é afetado; no Orc (limite de 150): não é afetado",
        "undoable": false,
      }
    `);
  });

  it('says the heal, the zero-hit-point kill and the Aid, with the amounts only where sent', () => {
    expect(
      cast({
        effectKind: SpellEffectKind.FLAT_HEAL,
        targets: [
          target('Brisa', { outcome: AFFECTED, healed: 7, hitPointsBefore: 3 }),
          target('Orc', { outcome: AFFECTED }),
        ],
      }),
    ).toMatchInlineSnapshot(`
      {
        "actor": "Pensantus",
        "card": undefined,
        "hidden": false,
        "icon": "auto_awesome",
        "id": "e1",
        "text": " conjura Sono na Brisa (3 PV): recupera 7 PV; no Orc: é curado",
        "undoable": false,
      }
    `);
    expect(
      cast({
        effectKind: SpellEffectKind.ZERO_HP,
        targets: [
          target('Orc', {
            outcome: NOT,
            reason: SpellEffectReason.NOT_AT_ZERO,
            hitPointsBefore: 9,
          }),
        ],
      }),
    ).toMatchInlineSnapshot(`
      {
        "actor": "Pensantus",
        "card": undefined,
        "hidden": false,
        "icon": "auto_awesome",
        "id": "e1",
        "text": " conjura Sono no Orc (9 PV): não é afetado: não estava a 0 PV",
        "undoable": false,
      }
    `);
    expect(
      cast({
        effectKind: SpellEffectKind.MAX_HP,
        targets: [target('Toren', { outcome: AFFECTED, gain: SpellEffectGain.CURRENT, healed: 5 })],
      }),
    ).toMatchInlineSnapshot(`
      {
        "actor": "Pensantus",
        "card": undefined,
        "hidden": false,
        "icon": "auto_awesome",
        "id": "e1",
        "text": " conjura Sono em Toren: acorda com 5 PV",
        "undoable": false,
      }
    `);
  });

  it('tells a player only who was affected, in "em" for the players and "no/na" for the rest', () => {
    const spell = {
      effectKind: SpellEffectKind.POOL,
      effectConditionKey: 'condition:unconscious',
      targets: [
        target('Toren', { outcome: AFFECTED }),
        target('Goblin', { outcome: NOT }),
        target('Brisa', { outcome: AFFECTED }),
      ],
    };
    expect(cast(spell, PLAYER)).toMatchInlineSnapshot(`
      {
        "actor": "Pensantus",
        "card": undefined,
        "hidden": false,
        "icon": "auto_awesome",
        "id": "e1",
        "text": " conjura Sono: Toren adormece. O Goblin não foi afetado. A Brisa adormece.",
        "undoable": false,
      }
    `);
  });

  it('draws the pool for the master: the dice, each creature from the lowest hit points, the slot, and who changes', () => {
    const roll = { diceCount: 5, diceSides: 8, faces: [4, 6, 2, 7, 3], total: 22, modifier: 0 };
    const spell = {
      effectKind: SpellEffectKind.POOL,
      effectConditionKey: 'condition:unconscious',
      slot: { level: 1, pact: false },
      poolRoll: roll,
      targets: [
        target('Goblin 2', {
          outcome: NOT,
          reason: SpellEffectReason.ABOVE_POOL,
          hitPointsBefore: 30,
          poolOrder: 3,
        }),
        target('Goblin 1', { outcome: AFFECTED, hitPointsBefore: 7, poolLeft: 15, poolOrder: 1 }),
        target('Orc', { outcome: AFFECTED, hitPointsBefore: 9, poolLeft: 6, poolOrder: 2 }),
        target('Lobo', { outcome: NOT, reason: SpellEffectReason.SKIPPED }),
        target('Ghoul', undefined),
      ],
    };
    const line = cast(spell);
    expect(line?.text).toMatchInlineSnapshot(`" conjura Sono (1º nível)"`);
    expect(line?.card).toMatchInlineSnapshot(`
      {
        "changeFor": [
          {
            "id": "Goblin 1",
            "label": "Goblin 1",
          },
          {
            "id": "Orc",
            "label": "Orc",
          },
        ],
        "note": "A magia começa por quem tem menos PV e vai descontando do total; só afeta quem tem PV igual ou menor que o que sobra. Os PV de quem ficou assim não mudam: a condição não é ferimento.",
        "physical": false,
        "roll": "5d8 (4, 6, 2, 7, 3) = 22",
        "rows": [
          {
            "affected": true,
            "hitPoints": "7 PV",
            "icon": "bedtime",
            "id": "Goblin 1",
            "label": "Goblin 1",
            "math": "22 − 7 = 15 restam",
            "word": "Adormeceu · Inconsciente",
          },
          {
            "affected": true,
            "hitPoints": "9 PV",
            "icon": "bedtime",
            "id": "Orc",
            "label": "Orc",
            "math": "15 − 9 = 6 restam",
            "word": "Adormeceu · Inconsciente",
          },
          {
            "affected": false,
            "hitPoints": "30 PV",
            "icon": "block",
            "id": "Goblin 2",
            "label": "Goblin 2",
            "math": "mais PV do que sobrou do total",
            "word": "Não afetado",
          },
          {
            "affected": false,
            "hitPoints": "",
            "icon": "block",
            "id": "Lobo",
            "label": "Lobo",
            "math": "já estava inconsciente ou a 0 PV",
            "word": "Não afetado",
          },
          {
            "affected": false,
            "hitPoints": "",
            "icon": "block",
            "id": "Ghoul",
            "label": "Ghoul",
            "math": "",
            "word": "Não afetado",
          },
        ],
        "slot": "Pensantus gastou um espaço de 1º nível.",
        "summary": "5d8 (4, 6, 2, 7, 3) = 22 PV. Goblin 1 (7 PV) adormeceu · inconsciente, 22 − 7 = 15 restam. Orc (9 PV) adormeceu · inconsciente, 15 − 9 = 6 restam. Goblin 2 (30 PV) não é afetado. Lobo não é afetado. Ghoul não é afetado.",
      }
    `);
  });

  it('draws the pool without a condition, with a physical die, and without a slot', () => {
    const line = cast({
      effectKind: SpellEffectKind.POOL,
      poolRoll: { diceCount: 2, diceSides: 8, faces: [], physical: true, total: 9, modifier: 0 },
      targets: [
        target('Goblin', { outcome: AFFECTED, hitPointsBefore: 5, poolLeft: 4, poolOrder: 1 }),
      ],
    });
    expect(line?.card).toMatchInlineSnapshot(`
      {
        "changeFor": [],
        "note": "A magia começa por quem tem menos PV e vai descontando do total; só afeta quem tem PV igual ou menor que o que sobra. Os PV de quem ficou assim não mudam: a condição não é ferimento.",
        "physical": true,
        "roll": "2d8 = 9",
        "rows": [
          {
            "affected": true,
            "hitPoints": "5 PV",
            "icon": "heart_broken",
            "id": "Goblin",
            "label": "Goblin",
            "math": "9 − 5 = 4 restam",
            "word": "Morreu",
          },
        ],
        "slot": "",
        "summary": "2d8 = 9 PV, dado físico. Goblin (5 PV) morreu, 9 − 5 = 4 restam.",
      }
    `);
  });

  it("writes a pool cast with no roll for a player, and a master's without the roll as the plain sentence", () => {
    const spell = {
      effectKind: SpellEffectKind.POOL,
      slot: { level: 2, pact: false },
      targets: [target('Goblin', { outcome: AFFECTED })],
    };
    expect(cast(spell, PLAYER)?.text).toMatchInlineSnapshot(`" conjura Sono: o Goblin morre."`);
    const noRoll = cast(spell);
    expect(noRoll?.text).toMatchInlineSnapshot(`" conjura Sono (2º nível)"`);
    expect(noRoll?.card).toMatchInlineSnapshot(`undefined`);
  });
});
