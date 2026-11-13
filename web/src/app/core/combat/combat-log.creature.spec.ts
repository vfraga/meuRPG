// The log lines of an NPC's action that is no attack roll (`creature_action`) and of a creature's rider (`hit_rider` with an
// action name): what the master reads (DC, SRD text) and what a player reads (the save they made and the damage, no key, no DC).
import { type MessageInitShape, create } from '@bufbuild/protobuf';

import {
  type CombatLogEntry,
  CombatLogEntrySchema,
  CombatLogKind,
  HitRiderChoice,
  HitRiderLogResult,
  PendingDamageStatus,
  SaveOutcome,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { logLine } from './combat-log';
import { d20TotalText } from './combat-dice';
import { dcLabel } from '../format/save-dc';

type Over = MessageInitShape<typeof CombatLogEntrySchema>;
const entry = (over: Over): CombatLogEntry =>
  create(CombatLogEntrySchema, { id: 'e1', ...over } as never);

const MASTER = { master: true, players: new Set(['Toren', 'Brisa']) };
const PLAYER = { master: false, players: new Set(['Toren', 'Brisa']) };

const save = (outcome: SaveOutcome, dc: number) => ({
  outcome,
  dc,
  roll: { diceCount: 1, diceSides: 20, faces: [9], modifier: 2, total: 11 },
});
const damage = (amount: number) => ({ amount, damageTypePt: 'fogo' });

const breath = (over: Record<string, unknown> = {}) =>
  entry({
    kind: CombatLogKind.SPELL_CAST,
    actorLabel: 'Cão Infernal 1',
    key: 'monster:hell-hound#fire-breath',
    keyNamePt: 'Sopro de fogo',
    creatureAction: { actionKey: 'monster:hell-hound#fire-breath', actionNamePt: 'Sopro de fogo' },
    spell: {
      targets: [
        {
          targetId: 't1',
          targetLabel: 'Toren',
          save: save(SaveOutcome.FAILED, 13),
          damage: damage(21),
        },
        {
          targetId: 't2',
          targetLabel: 'Brisa',
          save: save(SaveOutcome.SAVED, 13),
          damage: damage(10),
        },
      ],
    },
    ...over,
  } as never);

describe('the line of a creature action', () => {
  it("writes each target's save and damage, with the DC for the master", () => {
    const line = logLine(breath(), '', MASTER);
    expect(line?.icon).toBe('bolt');
    expect(line?.text).toBe(
      ' usou Sopro de fogo: o Toren falhou (CD 13; ' +
        'd20 11, bônus desconhecido), 21 de dano; a Brisa resistiu (CD 13; d20 11, bônus desconhecido), 10 de dano',
    );
  });

  it('writes the condition, who got it, its duration and the repeated save', () => {
    const line = logLine(
      breath({
        creatureAction: {
          actionNamePt: 'Semblante horripilante',
          conditionKey: 'condition:frightened',
          conditionNamePt: 'Amedrontado',
          conditionTargetIds: ['t1'],
          durationText: '1 minute',
          repeatSave: true,
        },
      }),
      '',
      PLAYER,
    );
    expect(line?.text).toContain('usou Semblante horripilante: o Toren falhou');
    expect(line?.text).toContain(
      '. Amedrontado: o Toren, por 1 minuto; repete o teste no fim de cada turno dele',
    );
  });

  it('shows the SRD text under the line for the master only, because the server sends it to nobody else', () => {
    const withText = breath({
      creatureAction: { actionNamePt: 'Sopro de fogo', text: 'The hound exhales fire.' },
    });
    expect(logLine(withText, '', MASTER)?.notes).toEqual(['Texto do SRD: The hound exhales fire.']);
    const masked = breath({ creatureAction: { actionNamePt: 'Sopro de fogo' } });
    expect(logLine(masked, '', PLAYER)?.notes).toBeUndefined();
  });

  it('writes an action with no roll as the reminder that spent the action', () => {
    const line = logLine(
      entry({
        kind: CombatLogKind.SPELL_CAST,
        actorLabel: 'Fantasma',
        creatureAction: { actionNamePt: 'Forma etérea', reminder: true },
      }),
      '',
      MASTER,
    );
    expect(line?.text).toBe(' usou Forma etérea: lembrete, a ação foi gasta');
  });

  it("renders only what arrives: a player's line carries no DC", () => {
    const masked = breath({
      spell: {
        targets: [
          {
            targetId: 't1',
            targetLabel: 'Toren',
            save: { outcome: SaveOutcome.FAILED },
            damage: damage(21),
          },
        ],
      },
    });
    expect(logLine(masked, '', PLAYER)?.text).toBe(
      ' usou Sopro de fogo: o Toren falhou, 21 de dano',
    );
  });
});

describe("the line of a creature's rider", () => {
  const rider = (over: Record<string, unknown>, ctx = MASTER) =>
    logLine(
      entry({
        kind: CombatLogKind.HIT_RIDER,
        actorLabel: 'Carniçal 1',
        targetLabel: 'Brisa',
        hitRider: { actionNamePt: 'Garras', ...over } as never,
      }),
      '',
      ctx,
    )?.text;

  it('writes a failed save with the roll against the DC and the condition it gave', () => {
    expect(
      rider({
        choice: HitRiderChoice.APPLY,
        result: HitRiderLogResult.FAILED,
        numbers: true,
        d20: 7,
        saveTotal: 9,
        dc: 10,
        conditionNamePt: 'Paralisado',
        durationText: '1 minute',
        repeatSave: true,
      }),
    ).toBe(
      ' usou Garras em Brisa: Brisa falhou no teste (1d20 (7) + 2 = 9 contra CD 10); ficou Paralisado por 1 minuto; repete o teste no fim de cada turno dele',
    );
  });

  it('writes the bonus of the roll with its sign, a negative one too', () => {
    const line = (d20: number, saveTotal: number) =>
      rider({
        choice: HitRiderChoice.APPLY,
        result: HitRiderLogResult.FAILED,
        numbers: true,
        d20,
        saveTotal,
      });
    expect(line(2, 4)).toBe(' usou Garras em Brisa: Brisa falhou no teste (1d20 (2) + 2 = 4)');
    expect(line(4, 3)).toBe(' usou Garras em Brisa: Brisa falhou no teste (1d20 (4) − 1 = 3)');
  });

  it('says what a table feature raised the d20 to, because the total counts the raised number', () => {
    expect(
      rider({
        choice: HitRiderChoice.APPLY,
        result: HitRiderLogResult.FAILED,
        numbers: true,
        d20: 2,
        saveTotal: 10,
        treatedAs: 8,
        treatedAsSource: 'content:feat-keen',
        treatedAsSourceName: 'Olho Aguçado',
      }),
    ).toBe(' usou Garras em Brisa: Brisa falhou no teste (d20: 2 → 8 (Olho Aguçado) + 2 = 10)');
  });

  it('tells the damage the save governs after the save, and what the player reads of it', () => {
    const base = {
      choice: HitRiderChoice.APPLY,
      result: HitRiderLogResult.FAILED,
      actionNamePt: 'Mordida',
    };
    expect(
      rider({
        ...base,
        damage: { amount: 9, damageTypePt: 'veneno', status: PendingDamageStatus.ROLLED },
      }),
    ).toBe(' usou Mordida em Brisa: Brisa falhou no teste, 9 de dano, esperando o mestre aplicar');
    expect(rider({ ...base, damage: { status: PendingDamageStatus.AWAITING_ROLL } }, PLAYER)).toBe(
      ' usou Mordida em Brisa: Brisa falhou no teste, falta rolar o dano',
    );
  });

  it('writes a save that held, a grapple with its escape DC and a declined rider', () => {
    expect(rider({ choice: HitRiderChoice.APPLY, result: HitRiderLogResult.SAVED })).toBe(
      ' usou Garras em Brisa: Brisa passou no teste',
    );
    expect(
      rider({
        choice: HitRiderChoice.APPLY,
        result: HitRiderLogResult.APPLIED,
        actionNamePt: 'Teia',
        conditionNamePt: 'Agarrado',
        escapeDc: 12,
      }),
    ).toBe(' usou Teia em Brisa: Brisa ficou Agarrado (CD de fuga 12)');
    expect(rider({ choice: HitRiderChoice.DECLINE })).toBe(
      ' dispensou o efeito de Garras em Brisa',
    );
  });

  it("leaves the DC and the key out of a player's line", () => {
    expect(
      rider(
        {
          choice: HitRiderChoice.APPLY,
          result: HitRiderLogResult.FAILED,
          conditionNamePt: 'Paralisado',
        },
        PLAYER,
      ),
    ).toBe(' usou Garras em Brisa: Brisa falhou no teste; ficou Paralisado');
  });

  it('tells the master the save he made a success, a monk rider and a creature rider alike, and a player a save that passed', () => {
    const resisted = {
      result: HitRiderLogResult.SAVED,
      numbers: true,
      d20: 1,
      saveTotal: 14,
      dc: 16,
      resisted: true,
      resistedLeft: 2,
    };
    const monk = (over: Record<string, unknown>, ctx = MASTER) =>
      logLine(
        entry({
          kind: CombatLogKind.HIT_RIDER,
          actorLabel: 'Toren',
          targetLabel: 'Draco',
          hitRider: { choice: HitRiderChoice.STUN, ...over } as never,
        }),
        '',
        ctx,
      )?.text;
    expect(monk(resisted)).toBe(
      ` usou o Golpe Atordoante em Draco: Draco falhou no teste (${d20TotalText(1, 14)} contra ${dcLabel(16)}) e usou Resistência Lendária: sucesso (restam 2)`,
    );
    expect(rider({ ...resisted, choice: HitRiderChoice.APPLY })).toBe(
      ` usou Garras em Brisa: Brisa falhou no teste (${d20TotalText(1, 14)} contra ${dcLabel(16)}) e usou Resistência Lendária: sucesso (restam 2)`,
    );
    // The server sends a player none of it: the line is an ordinary save that passed.
    const plain = { result: HitRiderLogResult.SAVED };
    expect(monk(plain, PLAYER)).toBe(' usou o Golpe Atordoante em Draco: Draco passou no teste');
  });
});
