import { create } from '@bufbuild/protobuf';

import {
  type CombatLogEntry,
  CombatLogEntrySchema,
  CombatLogFavorSchema,
  CombatLogKind,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { logLine } from './combat-log';

const entry = (kind: CombatLogKind, text: string | null): CombatLogEntry =>
  create(CombatLogEntrySchema, {
    id: 'x',
    kind,
    favor: text === null ? undefined : create(CombatLogFavorSchema, { textPt: text }),
  });

const said = (e: CombatLogEntry) => {
  const line = logLine(e);
  return line ? `${line.actor}${line.text}` : null;
};

describe('the lines of the Inspiração in the log (RN-43, state 11)', () => {
  it('says the refund as the server wrote it for the master and the roller', () => {
    expect(
      said(entry(CombatLogKind.FAVOR_REFUNDED, 'O mestre devolveu a Inspiração de Jabuti.')),
    ).toBe('O mestre devolveu a Inspiração de Jabuti.');
  });

  it('says the pass as each reader may read it', () => {
    expect(said(entry(CombatLogKind.FAVOR_PASSED, 'Brisa passou a Inspiração para Tor.'))).toBe(
      'Brisa passou a Inspiração para Tor.',
    );
    expect(said(entry(CombatLogKind.FAVOR_PASSED, 'Brisa passou a Inspiração para você.'))).toBe(
      'Brisa passou a Inspiração para você.',
    );
  });

  it('says the refusal or the cancel of a pass', () => {
    expect(
      said(entry(CombatLogKind.FAVOR_PASS_ANSWERED, 'O mestre recusou o pedido de Brisa.')),
    ).toBe('O mestre recusou o pedido de Brisa.');
  });

  it('draws the sentence whole, with no actor in bold, and an icon', () => {
    const line = logLine(
      entry(CombatLogKind.FAVOR_REFUNDED, 'O mestre devolveu a Inspiração de Jabuti.'),
    )!;
    expect(line.actor).toBe('');
    expect(line.icon).toBe('star_border');
  });

  it('draws nothing for an entry with no sentence', () => {
    expect(logLine(entry(CombatLogKind.FAVOR_PASSED, null))).toBeNull();
    expect(logLine(entry(CombatLogKind.FAVOR_PASSED, ''))).toBeNull();
  });
});
