// The familiar's name is read once for the character, not once per read of the combat.
import { TestBed } from '@angular/core/testing';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatDialog } from '@angular/material/dialog';
import { provideRouter } from '@angular/router';
import { create } from '@bufbuild/protobuf';

import { DiceMode, DicePreference } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  CombatantKind,
  CombatantSide,
  EncounterMode,
} from '../../../../gen/meurpg/play/v1/combat_pb';
import { TurnOptionsSchema } from '../../../../gen/meurpg/rules/v1/rules_pb';
import { TableRulesClient } from '../../../core/campaigns/table-rules';
import { CombatClient } from '../../../core/combat/combat-client';
import { CombatState } from '../../../core/combat/combat-state';
import { combatant, encounter } from '../../../core/combat/combat-testing';
import { SpellCatalog } from '../../../core/combat/spell-catalog';
import { CreaturesClient } from '../../../core/creatures/creatures-client';
import { MapState } from '../../../core/maps/map-state';
import { MapsClient } from '../../../core/maps/maps-client';
import { RosterClient } from '../../../core/maps/roster-client';
import { CombatView } from './combat-view';

describe('familiar name is read once per character', () => {
  const fresh = (revision: number) =>
    encounter({
      mode: EncounterMode.THEATRE,
      mapId: '',
      gridColumns: 0,
      gridRows: 0,
      revision,
      currentCombatantId: 't',
      combatants: [
        combatant({
          id: 't',
          label: 'Toren',
          kind: CombatantKind.PLAYER,
          side: CombatantSide.PARTY,
          characterId: 'toren-c',
          mine: true,
          placed: false,
        }),
      ],
    });

  it('asks ListCharacterCreatures once for the character, not once per refresh', async () => {
    let calls = 0;
    const api = new Proxy(
      {},
      {
        get: (_t, name: string) =>
          name === 'turnOptions'
            ? async () => ({
                options: create(TurnOptionsSchema, {}),
                attackTargets: [],
                spellTargets: [],
                pendingDamages: [],
              })
            : name === 'log'
              ? async () => ({ rounds: [], undoableEventId: '' })
              : async () => ({}),
      },
    );
    const state = new CombatState();
    state.apply(fresh(1));
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
            list: async () => {
              calls++;
              return [];
            },
            statBlock: async () => null,
            summonOptions: async () => [],
          },
        },
        {
          provide: TableRulesClient,
          useValue: { get: async () => ({ saved: { deathSaves: 0 } }) },
        },
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
    fixture.componentRef.setInput('isMaster', false);
    fixture.componentRef.setInput('state', state);
    fixture.componentRef.setInput('mapState', new MapState(async () => ({}) as never));
    fixture.componentRef.setInput('diceMode', DiceMode.PLAYERS_CHOOSE);
    fixture.componentRef.setInput('dicePreference', DicePreference.APP);
    fixture.detectChanges();
    await fixture.whenStable();
    for (let n = 2; n <= 4; n++) {
      state.apply(fresh(n));
      fixture.detectChanges();
      await fixture.whenStable();
    }
    expect(calls).toBe(1);
  });
});
