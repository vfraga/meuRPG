// Choosing Revivificar in the cast list opens its own sheet, not the target picker.
import { TestBed } from '@angular/core/testing';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatDialog } from '@angular/material/dialog';
import { provideRouter } from '@angular/router';
import { create } from '@bufbuild/protobuf';
import { of } from 'rxjs';

import {
  DeathSaveVisibility,
  DiceMode,
  DicePreference,
} from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
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
import { RevivifySheet } from './revivify-sheet/revivify-sheet';
import { anyCallCombatClient } from '../../../core/combat/combat-client-testing';

describe('CombatView: Revivificar in the cast list', () => {
  function mount() {
    vi.stubGlobal('matchMedia', (query: string) => ({
      matches: /min-width: 1024px/.test(query),
      media: query,
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
      addListener: () => undefined,
      removeListener: () => undefined,
    }));
    const api = anyCallCombatClient((name) =>
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
    );
    const open = vi.fn((..._args: unknown[]) => ({ afterClosed: () => of(undefined) }));
    const state = new CombatState();
    state.apply(
      encounter({
        id: 'enc',
        round: 8,
        mode: EncounterMode.THEATRE,
        mapId: '',
        gridColumns: 0,
        gridRows: 0,
        currentCombatantId: 'i',
        combatants: [
          combatant({
            id: 'i',
            label: 'Ilaria',
            kind: CombatantKind.PLAYER,
            side: CombatantSide.PARTY,
            characterId: 'ilaria-c',
            mine: true,
            placed: false,
          }),
        ],
      }),
    );
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
        {
          provide: TableRulesClient,
          useValue: {
            get: async () => ({ saved: { deathSaves: DeathSaveVisibility.VISIBLE_TO_ALL } }),
          },
        },
        { provide: MatDialog, useValue: { open } },
        {
          provide: MatBottomSheet,
          useValue: { open: () => ({ afterDismissed: () => ({ subscribe: () => undefined }) }) },
        },
      ],
    });
    const fixture = TestBed.createComponent(CombatView);
    fixture.componentRef.setInput('campaignId', 'c');
    fixture.componentRef.setInput('state', state);
    fixture.componentRef.setInput('mapState', new MapState(async () => ({}) as never));
    fixture.componentRef.setInput('diceMode', DiceMode.PLAYERS_CHOOSE);
    fixture.componentRef.setInput('dicePreference', DicePreference.APP);
    fixture.componentRef.setInput('casterClasses', 'Clérigo 5');
    fixture.detectChanges();
    return { fixture, open };
  }

  afterEach(() => vi.unstubAllGlobals());

  it('opens the Revivificar sheet for the combatant that casts, with the combat to answer to', async () => {
    const { fixture, open } = mount();
    await (
      fixture.componentInstance as unknown as { openCast(key: string): Promise<void> }
    ).openCast('spell:revivify');

    expect(open).toHaveBeenCalledTimes(1);
    const [component, config] = open.mock.calls[0] as [unknown, { data: Record<string, unknown> }];
    expect(component).toBe(RevivifySheet);
    expect(config.data).toMatchObject({
      campaignId: 'c',
      casterName: 'Ilaria',
      classes: 'Clérigo 5',
      casterCharacterId: 'ilaria-c',
      combat: { encounterId: 'enc', casterId: 'i', round: 8 },
    });
  });

  it('keeps the other spells on the cast sheet', async () => {
    const { fixture, open } = mount();
    await (
      fixture.componentInstance as unknown as { openCast(key: string): Promise<void> }
    ).openCast('spell:cure-wounds');

    // No options are loaded here, so the cast sheet has nothing to open: what matters is that Revivificar's did not.
    expect(open.mock.calls.some(([component]) => component === RevivifySheet)).toBe(false);
  });
});
