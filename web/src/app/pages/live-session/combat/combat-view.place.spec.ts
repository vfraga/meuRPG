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

// "Colocar no mapa": the server picks the square, so a wall or the players' side is never chosen here.

interface Internals {
  place(id: string): Promise<void>;
}

function setup() {
  TestBed.resetTestingModule();
  const calls: { name: string; args: unknown[] }[] = [];
  const state = new CombatState();
  const e = encounter({
    mode: EncounterMode.GRID,
    gridColumns: 8,
    gridRows: 8,
    combatants: [
      combatant({
        id: 'p',
        label: 'Pensantus',
        kind: CombatantKind.PLAYER,
        side: CombatantSide.PARTY,
        placed: true,
        col: 4,
        row: 4,
      }),
      combatant({ id: 'g', label: 'Goblin 1', kind: CombatantKind.NPC, placed: false }),
    ],
  });
  state.apply(e);
  const api = anyCallCombatClient((name) => {
    if (name === 'place' || name === 'move') {
      return async (...args: unknown[]) => {
        calls.push({ name, args });
        return name === 'place'
          ? e
          : { encounter: e, stoppedEarly: false, lockedDoor: false, provoked: false };
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
  return { view: fixture.componentInstance as unknown as Internals, calls, encounterId: e.id };
}

describe('CombatView: "Colocar no mapa"', () => {
  it('asks the server to place the combatant instead of choosing a square', async () => {
    const { view, calls, encounterId } = setup();
    await view.place('g');
    expect(calls).toEqual([{ name: 'place', args: ['c', encounterId, 'g'] }]);
  });
});
