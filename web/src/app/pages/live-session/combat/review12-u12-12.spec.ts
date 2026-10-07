import { Code, ConnectError } from '@connectrpc/connect';
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
import { CONNECT_TRANSPORT } from '../../../core/connect/transport';
import { CombatState } from '../../../core/combat/combat-state';
import { combatant, encounter } from '../../../core/combat/combat-testing';
import { CreaturesClient } from '../../../core/creatures/creatures-client';
import { MapState } from '../../../core/maps/map-state';
import { MapsClient } from '../../../core/maps/maps-client';
import { RosterClient } from '../../../core/maps/roster-client';
import { SpellCatalog } from '../../../core/combat/spell-catalog';
import { CombatView } from './combat-view';

// Finding U12-12: CombatClient.move() makes a new idempotency key per call, so a retried high jump (the first answer
// was lost) is a new request to the server, which charges the height again.

describe('Review12 U12-12: a retried high jump reuses its idempotency key', () => {
  it('after a lost answer (Unavailable) the second tap of the same high jump sends the same key', async () => {
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
    const keys: string[] = [];
    const transport = {
      unary: async (method: { name: string }, _s: unknown, _t: unknown, _h: unknown, input: unknown) => {
        let message: unknown = {};
        if (method.name === 'MoveCombatant') {
          keys.push((input as { idempotencyKey: string }).idempotencyKey);
          if (keys.length === 1) {
            throw new ConnectError('lost answer', Code.Unavailable);
          }
          message = { encounter: enc };
        }
        return {
          stream: false,
          service: {},
          method,
          header: new Headers(),
          trailer: new Headers(),
          message,
        };
      },
    };
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        { provide: CONNECT_TRANSPORT, useValue: transport },
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
    expect(TestBed.inject(CombatClient)).toBeTruthy();
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
    };
    await view.confirmJump({ kind: 'high', heightDft: 18 });
    await view.confirmJump({ kind: 'high', heightDft: 18 });

    expect(keys).toHaveLength(2);
    expect(keys[1]).toBe(keys[0]);
  });
});
