import { TestBed } from '@angular/core/testing';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatDialog } from '@angular/material/dialog';
import { provideRouter } from '@angular/router';
import { create } from '@bufbuild/protobuf';
import { of } from 'rxjs';
import { vi } from 'vitest';

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
import { TURN_ALERT_ENV, type TurnAlertEnv } from '../../../core/combat/turn-alert';
import { CreaturesClient } from '../../../core/creatures/creatures-client';
import { MapState } from '../../../core/maps/map-state';
import { MapsClient } from '../../../core/maps/maps-client';
import { RosterClient } from '../../../core/maps/roster-client';
import { SpellCatalog } from '../../../core/combat/spell-catalog';
import { CombatView } from './combat-view';
import { anyCallCombatClient } from '../../../core/combat/combat-client-testing';

// The turn alert on the combat screen: it fires when the turn passes to what the viewer plays, never on load or on an NPC.

const pen = combatant({
  id: 'p',
  label: 'Pensantus',
  kind: CombatantKind.PLAYER,
  side: CombatantSide.PARTY,
  characterId: 'pen-c',
  mine: true,
  placed: false,
});
const wolf = combatant({
  id: 'w',
  label: 'Lobo',
  kind: CombatantKind.CREATURE,
  monsterKey: 'monster:wolf',
  controlledByMe: true,
  summonGroupId: 'g1',
  placed: false,
});
const goblin = combatant({ id: 'g', label: 'Goblin', placed: false });
const order = [pen, wolf, goblin];

function setup(opts: { current: string; master?: boolean }) {
  TestBed.resetTestingModule();
  const vibrate = vi.fn();
  const env: TurnAlertEnv = {
    vibrate,
    audio: () => null,
    hidden: () => false,
    reducedMotion: () => false,
    onVisible: () => () => undefined,
    onGesture: () => () => undefined,
  };
  const api = anyCallCombatClient((name) =>
    name === 'turnOptions'
      ? async () => ({
          options: create(TurnOptionsSchema, {}),
          attackTargets: [],
          spellTargets: [],
          pendingDamages: [],
        })
      : async () => ({ rounds: [], undoableEventId: '' }),
  );
  const state = new CombatState();
  const apply = (current: string) =>
    state.apply(
      encounter({
        mode: EncounterMode.THEATRE,
        mapId: '',
        gridColumns: 0,
        gridRows: 0,
        currentCombatantId: current,
        combatants: order,
      } as never),
    );
  apply(opts.current);
  TestBed.configureTestingModule({
    providers: [
      provideRouter([]),
      { provide: TURN_ALERT_ENV, useValue: env },
      { provide: CombatClient, useValue: api },
      { provide: RosterClient, useValue: { list: async () => [] } },
      { provide: MapsClient, useValue: { layers: async () => ({}) } },
      { provide: SpellCatalog, useValue: { details: async () => null } },
      {
        provide: CreaturesClient,
        useValue: {
          list: async () => [],
          statBlock: async () => ({ armorClass: 13 }),
          summonOptions: async () => [],
        },
      },
      { provide: TableRulesClient, useValue: { get: async () => ({ saved: {} }) } },
      { provide: MatDialog, useValue: { open: () => ({ afterClosed: () => of(undefined) }) } },
      {
        provide: MatBottomSheet,
        useValue: { open: () => ({ afterDismissed: () => of(undefined) }) },
      },
    ],
  });
  const fixture = TestBed.createComponent(CombatView);
  fixture.componentRef.setInput('campaignId', 'c');
  fixture.componentRef.setInput('isMaster', opts.master ?? false);
  fixture.componentRef.setInput('state', state);
  fixture.componentRef.setInput('mapState', new MapState(async () => ({}) as never));
  fixture.componentRef.setInput('diceMode', DiceMode.PLAYERS_CHOOSE);
  fixture.componentRef.setInput('dicePreference', DicePreference.APP);
  fixture.detectChanges();
  const turn = async (current: string) => {
    apply(current);
    fixture.detectChanges();
    await fixture.whenStable();
  };
  return { fixture, el: fixture.nativeElement as HTMLElement, vibrate, turn };
}

describe('CombatView, the turn alert', () => {
  it("stays silent when the page loads on the player's own turn", async () => {
    const { vibrate, fixture } = setup({ current: 'p' });
    await fixture.whenStable();
    expect(vibrate).not.toHaveBeenCalled();
  });

  it('fires when the turn passes to the character, once, however many times it is read again', async () => {
    const { vibrate, turn, fixture } = setup({ current: 'g' });
    await fixture.whenStable();
    await turn('p');
    await turn('p');
    expect(vibrate).toHaveBeenCalledTimes(1);
    expect(fixture.nativeElement.querySelector('.player__turn')?.classList).toContain('mr-nudge');
  });

  it("fires when the turn passes to one of the player's creatures, and not for the NPC's turn", async () => {
    const { vibrate, turn, fixture } = setup({ current: 'p' });
    await fixture.whenStable();
    await turn('g');
    expect(vibrate).not.toHaveBeenCalled();
    await turn('w');
    expect(vibrate).toHaveBeenCalledTimes(1);
  });

  it('never fires for the master, who plays no character', async () => {
    const { vibrate, turn, fixture } = setup({ current: 'p', master: true });
    await fixture.whenStable();
    await turn('g');
    await turn('p');
    expect(vibrate).not.toHaveBeenCalled();
  });

  it('has the switch, labelled, on by default', async () => {
    const { el, fixture } = setup({ current: 'g' });
    await fixture.whenStable();
    fixture.detectChanges();
    const input = el.querySelector<HTMLInputElement>('app-check-row input[type=checkbox]');
    expect(el.querySelector('app-check-row')?.textContent).toContain(
      'Avisar na minha vez (som e vibração)',
    );
    expect(input?.checked).toBe(true);
  });
});
