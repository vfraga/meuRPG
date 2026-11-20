import { create } from '@bufbuild/protobuf';
import { of } from 'rxjs';

import {
  CombatantKind,
  ReactionKind,
  ReadiedActionSchema,
  ReadiedKind,
  ReadiedStatus,
} from '../../../../gen/meurpg/play/v1/combat_pb';
import { CombatState } from '../../../core/combat/combat-state';
import { combatant, encounter, reactionWindow } from '../../../core/combat/combat-testing';
import type { CombatSheetsContext } from './combat-sheets-context';
import { ReactionSheets } from './reaction-sheets';
import type { ReadiedWindowSheetData } from './readied-window-sheet/readied-window-sheet';

const spell = create(ReadiedActionSchema, {
  id: 'r1',
  kind: ReadiedKind.SPELL,
  actionKey: 'spell:fireball',
  actionNamePt: 'Bola de Fogo',
  status: ReadiedStatus.WAITING,
});

function setup(windows: 'open' | 'none' = 'open') {
  const opened: { data: ReadiedWindowSheetData }[] = [];
  const dialog = {
    open: (_c: unknown, config: { data: ReadiedWindowSheetData }) => {
      opened.push(config);
      return { afterClosed: () => of(undefined) };
    },
  };
  const state = new CombatState();
  const enc = encounter({
    combatants: [
      combatant({ id: 'pen', label: 'Pensantus', kind: CombatantKind.PLAYER, readied: spell }),
    ],
    reactionWindows:
      windows === 'open'
        ? [
            reactionWindow({
              id: 'w1',
              kind: ReactionKind.READIED,
              reactorId: 'pen',
              reactorLabel: 'Pensantus',
            } as never),
          ]
        : [],
  } as never);
  const ctx = {
    dialog,
    bottomSheet: {},
    campaignId: () => 'c',
    encounter: () => enc,
    state: () => state,
    own: () => null,
    vitals: () => [],
  } as unknown as CombatSheetsContext;
  const release = vi.fn();
  return { sheets: new ReactionSheets(ctx, () => undefined, release), opened, release };
}

describe('ReactionSheets, the window of a readied action (#857, #856)', () => {
  it('hands the window the way to the map: "Soltar no mapa" opens the release for that window', () => {
    const { sheets, opened, release } = setup();
    sheets.reaction(encounterWindow());
    opened[0].data.toMap?.(null);
    expect(release).toHaveBeenCalledWith('w1');
  });

  it('shows the window again for a player who went back from the map', () => {
    const { sheets, opened } = setup();
    sheets.reopenReadied('w1');
    expect(opened).toHaveLength(1);
    expect(opened[0].data.window.id).toBe('w1');
  });

  it('shows nothing again when the window is gone', () => {
    const { sheets, opened } = setup('none');
    sheets.reopenReadied('w1');
    expect(opened).toHaveLength(0);
  });
});

function encounterWindow() {
  return reactionWindow({
    id: 'w1',
    kind: ReactionKind.READIED,
    reactorId: 'pen',
    reactorLabel: 'Pensantus',
  } as never);
}
