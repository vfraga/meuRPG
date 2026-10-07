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

// Finding U12-05: confirmJump overwrites the "parou antes" note that runMove set with "Você saltou 0,0 m."
// when the server stopped the long jump early (a hidden creature holds the landing: cost 0, jumper stays).

describe('Review12 U12-05: confirmJump hides the stopped-early note', () => {
  it('a long jump stopped early by an unseen blocker keeps the "parou antes" note, not "saltou 0,0 m"', async () => {
    TestBed.resetTestingModule();
    const toren = combatant({
      id: 't',
      label: 'Toren',
      kind: CombatantKind.PLAYER,
      side: CombatantSide.PARTY,
      characterId: 'toren-c',
      mine: true,
    });
    const state = new CombatState();
    const enc = encounter({
      mode: EncounterMode.THEATRE,
      mapId: '',
      gridColumns: 0,
      gridRows: 0,
      currentCombatantId: 't',
      combatants: [toren],
    });
    state.apply(enc);
    const api = {
      // The server's answer: nothing moved, nothing spent, stoppedEarly.
      move: async () => ({
        encounter: enc,
        stoppedEarly: true,
        lockedDoor: false,
        provoked: false,
      }),
      log: async () => ({ rounds: [], undoableEventId: '' }),
    };
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        { provide: CombatClient, useValue: api },
        { provide: RosterClient, useValue: { list: async () => [] } },
        { provide: MapsClient, useValue: { layers: async () => ({}) } },
        { provide: SpellCatalog, useValue: { details: async () => null } },
        {
          provide: CreaturesClient,
          useValue: { list: async () => [], statBlock: async () => null, summonOptions: async () => [] },
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
    fixture.componentRef.setInput('isMaster', false);
    fixture.componentRef.setInput('state', state);
    fixture.componentRef.setInput('mapState', new MapState(async () => ({}) as never));
    fixture.componentRef.setInput('diceMode', DiceMode.PLAYERS_CHOOSE);
    fixture.componentRef.setInput('dicePreference', DicePreference.APP);
    fixture.detectChanges();
    await fixture.whenStable();

    const view = fixture.componentInstance as unknown as {
      confirmJump(r: unknown): Promise<void>;
      moveNote(): string;
    };
    await view.confirmJump({ kind: 'long', square: { col: 3, row: 3 } });

    expect(view.moveNote()).toContain('parou antes');
    expect(view.moveNote()).not.toContain('saltou');
  });
});
