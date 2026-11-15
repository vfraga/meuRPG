import { type MessageInitShape, create } from '@bufbuild/protobuf';

import {
  type CombatLogEntry,
  CombatLogEntrySchema,
  CombatLogKind,
  CombatLogRoundSchema,
  ReadiedKind,
  ReadiedStatus,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { logGroups, logLine } from './combat-log';

type Over = MessageInitShape<typeof CombatLogEntrySchema>;
const entry = (over: Over): CombatLogEntry =>
  create(CombatLogEntrySchema, { id: 'l1', actorLabel: 'Tor', ...over });
const line = (over: Over) => logLine(entry(over), '', { master: false, players: new Set(['Tor']) });
const said = (over: Over): string => {
  const l = line(over);
  return `${l?.actor ?? ''}${l?.text ?? ''}`.replace(/ /g, ' ');
};

const held = (over: MessageInitShape<typeof CombatLogEntrySchema>['readied'] = {}) => ({
  readiedId: 'r1',
  triggerText: 'Quando o zumbi passar pela porta',
  kind: ReadiedKind.ATTACK,
  actionKey: 'equipment:longsword',
  actionNamePt: 'Espada longa',
  ...over,
});

describe('the lines of a readied action (#582, board 582-08)', () => {
  it('the readier and the master read the action and the trigger when it is readied', () => {
    expect(
      said({
        kind: CombatLogKind.ACTION_READIED,
        targetLabel: 'Zumbi 1',
        readied: held(),
      }),
    ).toBe('Tor preparou uma ação: Espada longa no Zumbi 1, “Quando o zumbi passar pela porta”');
  });

  it('the others read only that Tor readied an action', () => {
    expect(said({ kind: CombatLogKind.ACTION_READIED })).toBe('Tor preparou uma ação');
  });

  it('says a use with the action and the trigger, or the plain words to the others', () => {
    expect(
      said({
        kind: CombatLogKind.READIED_FIRED,
        targetLabel: 'Zumbi 1',
        readiedFired: held({ status: ReadiedStatus.FIRED }),
      }),
    ).toBe(
      'Tor usou a ação preparada: Espada longa no Zumbi 1, “Quando o zumbi passar pela porta”',
    );
    // The others' entry has neither the trigger nor the status: it cannot say a use from a pass.
    expect(said({ kind: CombatLogKind.READIED_FIRED })).toBe('Tor respondeu à ação preparada');
  });

  it('says a pass, with the action for who may read it', () => {
    expect(
      said({
        kind: CombatLogKind.READIED_FIRED,
        readiedFired: held({ status: ReadiedStatus.IGNORED }),
      }),
    ).toBe('Tor deixou passar a ação preparada (Espada longa)');
    expect(
      said({
        kind: CombatLogKind.READIED_FIRED,
        readiedFired: { readiedId: 'r1', status: ReadiedStatus.IGNORED },
      }),
    ).toBe('Tor deixou passar a ação preparada');
  });

  it('every loss reads "perdeu a ação preparada" to the others', () => {
    expect(said({ kind: CombatLogKind.READIED_LOST })).toBe('Tor perdeu a ação preparada');
  });

  it('the owner and the master read which action was lost, and that it was given up', () => {
    expect(said({ kind: CombatLogKind.READIED_LOST, readiedLost: held() })).toBe(
      'A Espada longa preparada de Tor se perdeu',
    );
    expect(said({ kind: CombatLogKind.READIED_LOST, readiedLost: held({ canceled: true }) })).toBe(
      'Tor desistiu da ação preparada (Espada longa)',
    );
  });

  it('a held spell is named to the owner and the master, never to the others', () => {
    const spell = held({
      kind: ReadiedKind.SPELL,
      actionKey: 'spell:magic-missile',
      actionNamePt: 'Mísseis Mágicos',
    });
    expect(said({ kind: CombatLogKind.ACTION_READIED, readied: spell })).toContain(
      'Mísseis Mágicos',
    );
    const others = said({ kind: CombatLogKind.ACTION_READIED });
    expect(others).not.toMatch(/Mísseis|magia|círculo|nível/i);
  });

  it("does not call the readied attack's own line an opportunity attack: it comes right after the fired line of the same actor", () => {
    const attack = (id: string, asReaction: boolean) =>
      entry({
        id,
        kind: CombatLogKind.ATTACK,
        targetLabel: 'Zumbi 1',
        key: 'equipment:dagger',
        keyNamePt: 'Adaga',
        asReaction,
      });
    const fired = entry({
      id: 'f1',
      kind: CombatLogKind.READIED_FIRED,
      readiedFired: held({ status: ReadiedStatus.FIRED }),
    });
    const passed = entry({
      id: 'f2',
      kind: CombatLogKind.READIED_FIRED,
      readiedFired: held({ status: ReadiedStatus.IGNORED }),
    });
    const texts = (entries: CombatLogEntry[]) =>
      logGroups([create(CombatLogRoundSchema, { round: 1, entries })], 1, '', {
        master: true,
        players: new Set(['Tor']),
      })[0].lines.map((l) => l.text);
    // The entries of a round are the latest first: the attack, then the line that fired it.
    expect(texts([attack('a1', true), fired])[0]).toBe(' ataca o Zumbi 1 com a Adaga: acertou');
    // An opportunity attack, or one of another actor right after the fired line, keeps its words.
    expect(texts([attack('a2', true)])[0]).toContain('(ataque de oportunidade)');
    // A pass is not a use: an opportunity attack right after it keeps its words.
    expect(texts([attack('a4', true), passed])[0]).toContain('(ataque de oportunidade)');
    expect(
      texts([{ ...attack('a3', true), actorLabel: 'Brisa' } as CombatLogEntry, fired])[0],
    ).toContain('(ataque de oportunidade)');
  });
});
