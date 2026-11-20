import { create } from '@bufbuild/protobuf';

import { CombatLogEntrySchema, CombatLogKind } from '../../../gen/meurpg/play/v1/combat_pb';
import {
  CombatLogContestSchema,
  ContestLogLine,
} from '../../../gen/meurpg/play/v1/contest_types_pb';
import { logLine } from './combat-log';

const plain = (s: string) => s.replace(/ /g, ' ');

function line(
  contest: ContestLogLine,
  actorLabel: string,
  targetLabel = '',
  players: string[] = ['Toren', 'Brisa', 'Orla', 'Nael'],
) {
  const entry = create(CombatLogEntrySchema, {
    id: 'l',
    kind: CombatLogKind.CONTEST,
    actorLabel,
    targetLabel,
    contest: create(CombatLogContestSchema, { line: contest }),
  });
  const out = logLine(entry, '', { master: false, players: new Set(players) });
  return out ? { ...out, text: plain(out.text) } : null;
}

describe('the contest lines of the combat log (W7-X)', () => {
  it.each([
    [ContestLogLine.GRAPPLED, 'Toren', 'Hobgoblin', ' agarrou o Hobgoblin'],
    [ContestLogLine.GRAPPLE_FAILED, 'Toren', 'Hobgoblin', ' não conseguiu agarrar o Hobgoblin'],
    [ContestLogLine.SHOVE_PRONE, 'Toren', 'Hobgoblin', ' derrubou o Hobgoblin'],
    [ContestLogLine.SHOVE_PUSHED, 'Toren', 'Hobgoblin', ' empurrou o Hobgoblin 1,5 m'],
    [
      ContestLogLine.SHOVE_STAYS,
      'Toren',
      'Hobgoblin',
      ' empurrou o Hobgoblin, que não saiu do lugar',
    ],
    [ContestLogLine.SHOVE_FAILED, 'Toren', 'Hobgoblin', ' não conseguiu empurrar o Hobgoblin'],
    [ContestLogLine.ESCAPED, 'Brisa', 'Hobgoblin', ' se soltou do Hobgoblin'],
    [ContestLogLine.ESCAPE_FAILED, 'Brisa', 'Hobgoblin', ' não conseguiu se soltar do Hobgoblin'],
    [ContestLogLine.RELEASED, 'Toren', 'Hobgoblin', ' soltou o Hobgoblin'],
    [ContestLogLine.HELPED, 'Orla', 'Toren', ' ajudou Toren'],
    [ContestLogLine.HIDE_APPLIED, 'Brisa', '', ' se escondeu'],
    [ContestLogLine.HIDE_REFUSED, 'Brisa', '', ' não conseguiu se esconder'],
    [ContestLogLine.HIDE_TRIED, 'Brisa', '', ' tentou se esconder'],
    [ContestLogLine.SURPRISED, 'Nael', '', ' sofre surpresa'],
    [ContestLogLine.SURPRISED, 'Brisa', '', ' sofre surpresa'],
    [ContestLogLine.SURPRISE_CLEARED, 'Brisa', '', ' não sofre mais surpresa'],
    [ContestLogLine.STOOD_UP, 'Brisa', '', ' se levantou'],
    [ContestLogLine.HIDE_REVEALED, 'Brisa', '', ' não se esconde mais'],
    [ContestLogLine.HIDE_REVEALED, 'Nael', '', ' não se esconde mais'],
  ])('%s', (kind, actor, target, text) => {
    expect(line(kind, actor, target)?.text).toBe(text);
  });

  it('names a player bare and the one that grapples a player bare too', () => {
    expect(line(ContestLogLine.GRAPPLED, 'Hobgoblin', 'Brisa')?.text).toBe(' agarrou Brisa');
    expect(line(ContestLogLine.ESCAPED, 'Toren', 'Brisa')?.text).toBe(' se soltou de Brisa');
    expect(line(ContestLogLine.ESCAPED, 'Brisa', 'Cobra constritora gigante')?.text).toBe(
      ' se soltou da Cobra constritora gigante',
    );
  });

  it('says that the master closed a contest', () => {
    expect(line(ContestLogLine.CLOSED, 'Toren', 'Brisa')?.text).toBe(
      ' teve a disputa com Brisa encerrada pelo mestre',
    );
  });

  it('draws an icon for each line and carries the actor in bold', () => {
    const grapple = line(ContestLogLine.GRAPPLED, 'Toren', 'Hobgoblin');
    expect(grapple?.icon).toBe('pan_tool');
    expect(grapple?.actor).toBe('Toren');
    expect(line(ContestLogLine.HELPED, 'Orla', 'Toren')?.icon).toBe('handshake');
    expect(line(ContestLogLine.HIDE_APPLIED, 'Brisa')?.icon).toBe('visibility_off');
  });

  it('skips a line this app does not know, and never shows a total or a DC', () => {
    expect(line(ContestLogLine.UNSPECIFIED, 'Toren', 'Hobgoblin')).toBeNull();
    const entry = create(CombatLogEntrySchema, { id: 'x', kind: CombatLogKind.CONTEST });
    expect(logLine(entry)).toBeNull();
    for (const kind of Object.values(ContestLogLine)) {
      if (typeof kind === 'number') {
        expect(line(kind, 'Toren', 'Hobgoblin')?.text ?? '').not.toMatch(/\d{2}|CD/);
      }
    }
  });
});
