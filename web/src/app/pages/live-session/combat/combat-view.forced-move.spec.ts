import { TestBed } from '@angular/core/testing';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatDialog } from '@angular/material/dialog';
import { provideRouter } from '@angular/router';

import { DiceMode, DicePreference } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  CombatantKind,
  CombatantSide,
  EncounterMode,
} from '../../../../gen/meurpg/play/v1/combat_pb';
import { TableRulesClient } from '../../../core/campaigns/table-rules';
import { CombatClient } from '../../../core/combat/combat-client';
import { CombatState } from '../../../core/combat/combat-state';
import { combatant, encounter } from '../../../core/combat/combat-testing';
import { CreaturesClient } from '../../../core/creatures/creatures-client';
import { MapState } from '../../../core/maps/map-state';
import { MapsClient } from '../../../core/maps/maps-client';
import { RosterClient } from '../../../core/maps/roster-client';
import { SpellCatalog } from '../../../core/combat/spell-catalog';
import { CombatView } from './combat-view';
import { anyCallCombatClient } from '../../../core/combat/combat-client-testing';

// The master's drag on the map of a running combat: "Movimento forçado" is for one drag.

interface Drag {
  forced: boolean;
}
interface Internals {
  forcedMove: { (): boolean; set(v: boolean): void };
  forcedNote: () => string;
  error: () => string;
  drop(d: { id: string; col: number; row: number }): Promise<void>;
}

function setup(move: (args: unknown[]) => Promise<void>) {
  TestBed.resetTestingModule();
  const sent: Drag[] = [];
  const state = new CombatState();
  const e = encounter({
    mode: EncounterMode.GRID,
    currentCombatantId: 'p',
    combatants: [
      combatant({
        id: 'p',
        label: 'Pensantus',
        kind: CombatantKind.PLAYER,
        side: CombatantSide.PARTY,
        placed: true,
        col: 1,
        row: 1,
      }),
      combatant({
        id: 'b',
        label: 'Brisa',
        kind: CombatantKind.PLAYER,
        side: CombatantSide.PARTY,
        placed: true,
        col: 2,
        row: 2,
      }),
    ],
  });
  state.apply(e);
  const api = anyCallCombatClient((name) => {
    if (name === 'move') {
      return async (...a: unknown[]) => {
        sent.push({ forced: a[7] === true });
        await move(a);
        return { encounter: e, stoppedEarly: false, lockedDoor: false, provoked: false };
      };
    }
    if (name === 'log') {
      return async () => ({ rounds: [], undoableEventId: '' });
    }
    return async () => ({});
  });
  TestBed.configureTestingModule({
    providers: [
      provideRouter([]),
      { provide: CombatClient, useValue: api },
      { provide: RosterClient, useValue: { list: async () => [] } },
      { provide: MapsClient, useValue: { layers: async () => ({}) } },
      { provide: SpellCatalog, useValue: { details: async () => null } },
      {
        provide: CreaturesClient,
        useValue: {
          list: async () => [],
          statBlock: async () => null,
          summonOptions: async () => [],
        },
      },
      { provide: TableRulesClient, useValue: { get: async () => ({ saved: {} }) } },
      {
        provide: MatDialog,
        useValue: { open: () => ({ afterClosed: () => ({ subscribe: () => undefined }) }) },
      },
      {
        provide: MatBottomSheet,
        useValue: { open: () => ({ afterDismissed: () => ({ subscribe: () => undefined }) }) },
      },
    ],
  });
  const fixture = TestBed.createComponent(CombatView);
  fixture.componentRef.setInput('campaignId', 'c');
  fixture.componentRef.setInput('isMaster', true);
  fixture.componentRef.setInput('state', state);
  fixture.componentRef.setInput('mapState', new MapState(async () => ({}) as never));
  fixture.componentRef.setInput('diceMode', DiceMode.PLAYERS_CHOOSE);
  fixture.componentRef.setInput('dicePreference', DicePreference.APP);
  fixture.detectChanges();
  return { view: fixture.componentInstance as unknown as Internals, sent };
}

describe('CombatView: the master\'s "Movimento forçado"', () => {
  it('a plain drag sends forced as false', async () => {
    const { view, sent } = setup(async () => undefined);
    await view.drop({ id: 'p', col: 5, row: 5 });
    expect(sent).toEqual([{ forced: false }]);
    expect(view.forcedNote()).toBe('');
  });

  it('a drag with the box on sends forced, says it in words and switches the box off', async () => {
    const { view, sent } = setup(async () => undefined);
    view.forcedMove.set(true);
    await view.drop({ id: 'p', col: 5, row: 5 });
    expect(sent).toEqual([{ forced: true }]);
    expect(view.forcedMove()).toBe(false);
    expect(view.forcedNote()).toBe(
      'Pensantus foi movido à força. Ninguém recebeu oferta de ataque de oportunidade.',
    );
    await view.drop({ id: 'p', col: 6, row: 6 });
    expect(sent[1]).toEqual({ forced: false });
    expect(view.forcedNote()).toBe('');
  });

  it("agrees with the name's gender", async () => {
    const { view } = setup(async () => undefined);
    view.forcedMove.set(true);
    await view.drop({ id: 'b', col: 5, row: 5 });
    expect(view.forcedNote()).toContain('Brisa foi movida à força.');
  });

  it('a refusal keeps the box on for the next try and says nothing of success', async () => {
    const { view } = setup(async () => {
      throw new Error('refused');
    });
    view.forcedMove.set(true);
    await view.drop({ id: 'p', col: 5, row: 5 });
    expect(view.forcedMove()).toBe(true);
    expect(view.forcedNote()).toBe('');
    expect(view.error()).not.toBe('');
  });
});
