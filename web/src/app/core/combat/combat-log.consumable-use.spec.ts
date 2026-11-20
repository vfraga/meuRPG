import { type MessageInitShape, create } from '@bufbuild/protobuf';

import { ConsumableKind } from '../../../gen/meurpg/characters/v1/characters_pb';
import {
  type CombatLogEntry,
  CombatLogEntrySchema,
  CombatLogKind,
  ConsumableUseLogLineSchema,
  ConsumableUseOutcome,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { logGroups, logLine } from './combat-log';

const entry = (
  actorLabel: string,
  use: MessageInitShape<typeof ConsumableUseLogLineSchema> | null,
  over: MessageInitShape<typeof CombatLogEntrySchema> = {},
): CombatLogEntry =>
  create(CombatLogEntrySchema, {
    id: 'x',
    kind: CombatLogKind.CONSUMABLE_USED,
    actorLabel,
    consumableUse: use ? create(ConsumableUseLogLineSchema, use) : undefined,
    ...over,
  });

/** The line as the log draws it: the actor in bold, then the text. */
const said = (e: CombatLogEntry) => {
  const line = logLine(e);
  return line ? `${line.actor}${line.text}`.replace(/\u00a0/g, ' ') : null;
};

const healed = {
  itemNamePt: 'Poção de cura',
  kind: ConsumableKind.POTION,
  outcome: ConsumableUseOutcome.HEALED,
  diceText: '2d4+2',
  faces: [3, 2],
  rollTotal: 7,
  healed: 7,
};

/** What the server sends to every reader but the master and the owners of the user and of the receiver: no number. */
const withoutNumbers = {
  itemNamePt: 'Poção de cura',
  kind: ConsumableKind.POTION,
  outcome: ConsumableUseOutcome.HEALED,
};

describe('CONSUMABLE_USED in the log', () => {
  describe('with the numbers (the master and the owners of the user and of the receiver)', () => {
    it('says who gave the potion to whom, the hit points and the dice, as the board has it', () => {
      expect(said(entry('Tor', { ...healed, targetLabel: 'Brisa' }))).toBe(
        'Tor deu uma Poção de cura a Brisa: 7 PV (2d4 + 2: 3 + 2 + 2)',
      );
    });

    it('says who drank it', () => {
      expect(said(entry('Tor', healed))).toBe(
        'Tor bebeu uma Poção de cura: 7 PV (2d4 + 2: 3 + 2 + 2)',
      );
    });

    it('writes a number taken off the roll with its sign, and a heal that gave nothing as 0 PV', () => {
      expect(
        said(
          entry('Tor', {
            ...healed,
            diceText: '1d4-1',
            faces: [2],
            rollTotal: 1,
            healed: 0,
          }),
        ),
      ).toBe('Tor bebeu uma Poção de cura: 0 PV (1d4 − 1: 2 − 1)');
    });

    it('says a greater potion with its own article and name', () => {
      expect(
        said(
          entry('Tor', {
            ...healed,
            itemNamePt: 'Poção de cura maior',
            diceText: '4d4+4',
            faces: [1, 2, 3, 4],
            rollTotal: 14,
            healed: 14,
          }),
        ),
      ).toBe('Tor bebeu uma Poção de cura maior: 14 PV (4d4 + 4: 1 + 2 + 3 + 4 + 4)');
    });
  });

  describe('without the numbers (every other player)', () => {
    it('says who drank which item', () => {
      expect(said(entry('Tor', withoutNumbers))).toBe('Tor bebeu uma Poção de cura');
    });

    it('says who gave which item and to whom', () => {
      expect(said(entry('Tor', { ...withoutNumbers, targetLabel: 'Brisa' }))).toBe(
        'Tor deu uma Poção de cura a Brisa',
      );
    });
  });

  describe('a resistance', () => {
    const resistance = {
      itemNamePt: 'Poção de resistência',
      kind: ConsumableKind.POTION,
      outcome: ConsumableUseOutcome.RESISTANCE,
      damageTypeKey: 'damage-type:fire',
      damageTypeNamePt: 'Fogo',
      effectDurationTextPt: 'por 1 hora',
    };

    it('says the damage type, which every reader may know', () => {
      expect(said(entry('Tor', resistance))).toBe(
        'Tor bebeu uma Poção de resistência: resistência a fogo por 1 hora',
      );
      expect(said(entry('Tor', { ...resistance, targetLabel: 'Brisa' }))).toBe(
        'Tor deu uma Poção de resistência a Brisa: resistência a fogo por 1 hora',
      );
    });

    it('writes the length the server sent and no other: none sent, none said', () => {
      expect(said(entry('Tor', { ...resistance, effectDurationTextPt: 'por 10 minutos' }))).toBe(
        'Tor bebeu uma Poção de resistência: resistência a fogo por 10 minutos',
      );
      expect(said(entry('Tor', { ...resistance, effectDurationTextPt: '' }))).toBe(
        'Tor bebeu uma Poção de resistência: resistência a fogo',
      );
    });

    it('leaves the damage type out when the server did not name it', () => {
      expect(said(entry('Tor', { ...resistance, damageTypeNamePt: '' }))).toBe(
        'Tor bebeu uma Poção de resistência',
      );
    });
  });

  describe('an item the app applies nothing of', () => {
    const text = {
      itemNamePt: 'Poção de escalada',
      kind: ConsumableKind.POTION,
      outcome: ConsumableUseOutcome.TEXT_ONLY,
    };

    it('says only who drank or gave it', () => {
      expect(said(entry('Tor', text))).toBe('Tor bebeu uma Poção de escalada');
      expect(said(entry('Tor', { ...text, targetLabel: 'Brisa' }))).toBe(
        'Tor deu uma Poção de escalada a Brisa',
      );
    });

    it('says "usou" for what is no potion, with the article of the name', () => {
      expect(
        said(
          entry('Tor', {
            itemNamePt: 'Pergaminho de magia',
            kind: ConsumableKind.SCROLL,
            outcome: ConsumableUseOutcome.TEXT_ONLY,
          }),
        ),
      ).toBe('Tor usou um Pergaminho de magia');
      expect(
        said(
          entry('Tor', {
            itemNamePt: 'Óleo de afiar',
            kind: ConsumableKind.OIL,
            outcome: ConsumableUseOutcome.TEXT_ONLY,
          }),
        ),
      ).toBe('Tor usou um Óleo de afiar');
      expect(
        said(
          entry('Tor', {
            itemNamePt: 'Conta de força',
            kind: ConsumableKind.OTHER,
            outcome: ConsumableUseOutcome.TEXT_ONLY,
            targetLabel: 'Brisa',
          }),
        ),
      ).toBe('Tor deu uma Conta de força a Brisa');
    });

    it('takes the verb from the kind the server sent, never from the name: a potion with a strange name is drunk, a line with no kind is "usou"', () => {
      expect(
        said(
          entry('Tor', {
            itemNamePt: 'Elixir azul',
            kind: ConsumableKind.POTION,
            outcome: ConsumableUseOutcome.TEXT_ONLY,
          }),
        ),
      ).toBe('Tor bebeu um Elixir azul');
      expect(
        said(
          entry('Tor', {
            itemNamePt: 'Poção de escalada',
            outcome: ConsumableUseOutcome.TEXT_ONLY,
          }),
        ),
      ).toBe('Tor usou uma Poção de escalada');
    });
  });

  it('has the flask icon for a potion and the page for a scroll, and writes nothing without consumable_use', () => {
    expect(logLine(entry('Tor', healed))?.icon).toBe('science');
    expect(
      logLine(
        entry('Tor', {
          itemNamePt: 'Pergaminho de magia',
          kind: ConsumableKind.SCROLL,
          outcome: ConsumableUseOutcome.TEXT_ONLY,
        }),
      )?.icon,
    ).toBe('description');
    expect(logLine(entry('Tor', null))).toBeNull();
  });

  it('is grouped by round like every other line and ends with "(pelo mestre)" for a master action', () => {
    const lines = logGroups(
      [{ round: 5, entries: [entry('Tor', healed, { byMaster: true })] } as never],
      5,
    )[0].lines;
    expect(lines.map((l) => l.text)).toContain(
      ' bebeu uma Poção de cura: 7 PV (2d4 + 2: 3 + 2 + 2) (pelo mestre)',
    );
  });
});
