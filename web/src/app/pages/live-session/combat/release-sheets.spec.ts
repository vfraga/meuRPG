import { create } from '@bufbuild/protobuf';
import { of } from 'rxjs';

import {
  AreaPlacement,
  CombatantKind,
  GetTurnOptionsResponseSchema,
  ReactionKind,
  ReadiedActionSchema,
  ReadiedKind,
  ReadiedStatus,
  SpellTargetsSchema,
} from '../../../../gen/meurpg/play/v1/combat_pb';
import { combatant, encounter, reactionWindow } from '../../../core/combat/combat-testing';
import { nextTurn } from '../../../../test-settle';
import { CastSheet } from './cast-sheet/cast-sheet';
import { MasterAreaCast } from './area-cast/master-area-cast';
import type { CombatSheetsContext } from './combat-sheets-context';
import { ReleaseSheets } from './release-sheets';

const spell = create(ReadiedActionSchema, {
  id: 'r1',
  kind: ReadiedKind.SPELL,
  actionKey: 'spell:fireball',
  actionNamePt: 'Bola de Fogo',
  triggerText: 'Quando o ogro passar pela porta',
  slotLevel: 3,
  status: ReadiedStatus.WAITING,
});
const move = create(ReadiedActionSchema, { ...spell, kind: ReadiedKind.MOVE, actionNamePt: '' });

function setup(
  over: { row?: typeof spell; master?: boolean; map?: boolean; targets?: boolean } = {},
) {
  const dialogOpen = vi.fn().mockReturnValue({ afterClosed: () => of(true) });
  const bottomOpen = vi.fn().mockReturnValue({ afterDismissed: () => of(true) });
  const e = encounter({
    combatants: [
      combatant({
        id: 'pen',
        label: 'Pensantus',
        kind: CombatantKind.PLAYER,
        readied: over.row ?? spell,
      }),
    ],
    reactionWindows: [
      reactionWindow({
        id: 'w1',
        kind: ReactionKind.READIED,
        reactorId: 'pen',
        reactorLabel: 'Pensantus',
      } as never),
    ],
  } as never);
  const releaseMove = vi.fn();
  const note = vi.fn();
  const turnOptions = vi.fn().mockResolvedValue(
    create(GetTurnOptionsResponseSchema, {
      spellTargets:
        over.targets === false
          ? []
          : [
              create(SpellTargetsSchema, {
                spellKey: 'spell:fireball',
                placement: AreaPlacement.POINT,
              }),
            ],
    }),
  );
  const ctx = {
    dialog: { open: dialogOpen },
    bottomSheet: { open: bottomOpen },
    api: { moves: { turnOptions } },
    campaignId: () => 'camp',
    encounter: () => e,
    isMaster: () => over.master ?? false,
    state: () => ({}),
    diceMode: () => 0,
    preference: () => 0,
    releaseMove,
    note,
  } as unknown as CombatSheetsContext;
  const reopen = vi.fn();
  const map = over.map === false ? null : ({ mapName: 'Cripta' } as never);
  const sheets = new ReleaseSheets(ctx, () => map, reopen);
  return { sheets, dialogOpen, releaseMove, note, turnOptions, reopen };
}

describe('ReleaseSheets, the map of a readied action (#857, #856)', () => {
  it('opens the Mover page for a held move', () => {
    const { sheets, releaseMove, turnOptions } = setup({ row: move });
    sheets.open('w1');
    expect(releaseMove).toHaveBeenCalledWith('w1');
    expect(turnOptions).not.toHaveBeenCalled();
  });

  it('opens the cast sheet in release mode for the player, reading the readier’s own options', async () => {
    const { sheets, dialogOpen, turnOptions } = setup();
    sheets.open('w1');
    await nextTurn();
    expect(turnOptions).toHaveBeenCalledWith('camp', 'enc', 'pen');
    expect(dialogOpen).toHaveBeenCalledTimes(1);
    const [component, config] = dialogOpen.mock.calls[0];
    expect(component).toBe(CastSheet);
    expect(config.data.release).toMatchObject({
      windowId: 'w1',
      reactorLabel: 'Pensantus',
      slotLevel: 3,
    });
    expect(config.data.master).toBe(false);
  });

  it('shows the window again once the player’s cast sheet closed', async () => {
    const { sheets, reopen } = setup();
    sheets.open('w1');
    await nextTurn();
    expect(reopen).toHaveBeenCalledWith('w1');
  });

  it('opens the master’s area dialog with the held slot for "Soltar por Pensantus", and shows no window again', async () => {
    const { sheets, dialogOpen, reopen } = setup({ master: true });
    sheets.open('w1');
    await nextTurn();
    const [component, config] = dialogOpen.mock.calls[0];
    expect(component).toBe(MasterAreaCast);
    expect(config.data.slot).toBeNull();
    expect(config.data.release.windowId).toBe('w1');
    expect(reopen).not.toHaveBeenCalled();
  });

  it('opens one dialog when the button is pressed twice while the readier’s options are read', async () => {
    const { sheets, dialogOpen, turnOptions } = setup({ master: true });
    sheets.open('w1');
    sheets.open('w1');
    await nextTurn();
    expect(turnOptions).toHaveBeenCalledTimes(1);
    expect(dialogOpen).toHaveBeenCalledTimes(1);
    sheets.open('w1');
    await nextTurn();
    expect(dialogOpen).toHaveBeenCalledTimes(2);
  });

  it('says so, and opens nothing, when there is no map or the options do not list the spell', async () => {
    const noMap = setup({ map: false });
    noMap.sheets.open('w1');
    await nextTurn();
    expect(noMap.note).toHaveBeenCalledWith('Não deu para abrir o mapa. Tente de novo.');
    const noSpell = setup({ targets: false });
    noSpell.sheets.open('w1');
    await nextTurn();
    expect(noSpell.note).toHaveBeenCalledWith('Não deu para abrir o mapa. Tente de novo.');
    expect(noSpell.dialogOpen).not.toHaveBeenCalled();
  });

  it('does nothing for a window that is not in the combat', () => {
    const { sheets, releaseMove, turnOptions } = setup();
    sheets.open('nope');
    expect(releaseMove).not.toHaveBeenCalled();
    expect(turnOptions).not.toHaveBeenCalled();
  });
});
