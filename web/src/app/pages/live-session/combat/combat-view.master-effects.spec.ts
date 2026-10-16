// The panel "Efeitos em jogo" is the master's: a running combat draws it for him and never for a player (RN-10).
import { TestBed } from '@angular/core/testing';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatDialog } from '@angular/material/dialog';
import { provideRouter } from '@angular/router';
import { create } from '@bufbuild/protobuf';

import { DiceMode, DicePreference } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { TurnOptionsSchema } from '../../../../gen/meurpg/rules/v1/rules_pb';
import { EncounterStatus } from '../../../../gen/meurpg/play/v1/combat_pb';
import { CombatClient } from '../../../core/combat/combat-client';
import { CombatState } from '../../../core/combat/combat-state';
import { combatant, encounter } from '../../../core/combat/combat-testing';
import { CreaturesClient } from '../../../core/creatures/creatures-client';
import { EffectsClient } from '../../../core/effects/effects-client';
import { boardEffects } from '../../../core/effects/effects-testing';
import { MapState } from '../../../core/maps/map-state';
import { MapsClient } from '../../../core/maps/maps-client';
import { RosterClient } from '../../../core/maps/roster-client';
import { SpellCatalog } from '../../../core/combat/spell-catalog';
import { TableRulesClient } from '../../../core/campaigns/table-rules';
import { CombatView } from './combat-view';
import { nextTurn } from '../../../../test-settle';

async function settle(fixture: { detectChanges: () => void; whenStable: () => Promise<unknown> }) {
  await fixture.whenStable();
  await nextTurn();
  await fixture.whenStable();
  fixture.detectChanges();
}

describe('the panel of effects in the combat page', () => {
  const list = vi.fn();

  function render(isMaster: boolean, status: EncounterStatus) {
    list.mockReset().mockResolvedValue(boardEffects());
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
    state.apply(
      encounter({
        status,
        currentCombatantId: 'g',
        combatants: [combatant({ id: 'g', label: 'Goblin', placed: false })],
      }),
    );
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        { provide: CombatClient, useValue: api },
        { provide: EffectsClient, useValue: { list } },
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
    fixture.componentRef.setInput('isMaster', isMaster);
    fixture.componentRef.setInput('state', state);
    fixture.componentRef.setInput('mapState', new MapState(async () => ({}) as never));
    fixture.componentRef.setInput('diceMode', DiceMode.PLAYERS_CHOOSE);
    fixture.componentRef.setInput('dicePreference', DicePreference.APP);
    fixture.detectChanges();
    return fixture;
  }

  it('is drawn for the master of a running combat, which reads the effects of that combat', async () => {
    const fixture = render(true, EncounterStatus.ACTIVE);
    await settle(fixture);
    const el = fixture.nativeElement as HTMLElement;
    expect(el.querySelector('app-effects-panel [data-testid="effects-panel"]')).toBeTruthy();
    expect(list).toHaveBeenCalledWith('c', 'enc');
  });

  it('is not drawn while the combat is still being set up', async () => {
    const fixture = render(true, EncounterStatus.SETUP);
    await settle(fixture);
    expect((fixture.nativeElement as HTMLElement).querySelector('app-effects-panel')).toBeNull();
    expect(list).not.toHaveBeenCalled();
  });

  it("is never drawn for a player, and the server is never asked for the master's list", async () => {
    const fixture = render(false, EncounterStatus.ACTIVE);
    await settle(fixture);
    const el = fixture.nativeElement as HTMLElement;
    expect(el.querySelector('app-effects-panel')).toBeNull();
    expect(el.textContent).not.toContain('Efeitos em jogo');
    expect(list).not.toHaveBeenCalled();
  });
});
