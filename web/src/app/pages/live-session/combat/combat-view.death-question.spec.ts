// "Ainda não" holds only while the character stays dying.
import { TestBed } from '@angular/core/testing';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatDialog } from '@angular/material/dialog';
import { provideRouter } from '@angular/router';
import { create } from '@bufbuild/protobuf';
import { TurnOptionsSchema } from '../../../../gen/meurpg/rules/v1/rules_pb';
import {
  DeathSaveVisibility,
  DiceMode,
  DicePreference,
} from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  CombatantKind,
  CombatantSide,
  CombatantState,
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

const plain = (t: string | null | undefined) => (t ?? '').replace(/\s+/g, ' ').trim();
const brisa = (state: CombatantState) =>
  combatant({
    id: 'b',
    label: 'Brisa',
    kind: CombatantKind.PLAYER,
    side: CombatantSide.PARTY,
    characterId: 'brisa-c',
    placed: false,
    state,
  });
const goblin = combatant({ id: 'g', label: 'Goblin', placed: false });
const enc = (state: CombatantState) =>
  encounter({
    mode: EncounterMode.THEATRE,
    mapId: '',
    gridColumns: 0,
    gridRows: 0,
    currentCombatantId: 'g',
    combatants: [brisa(state), goblin],
  });

async function settle(fixture: { detectChanges: () => void; whenStable: () => Promise<unknown> }) {
  await fixture.whenStable();
  await new Promise((r) => setTimeout(r, 0));
  await fixture.whenStable();
  fixture.detectChanges();
}

describe('"Ainda não" survives a heal and a second fall', () => {
  it('asks again when the same character is dying again after being healed', async () => {
    vi.stubGlobal('matchMedia', (query: string) => ({
      matches: /min-width: 1024px/.test(query),
      media: query,
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
      addListener: () => undefined,
      removeListener: () => undefined,
    }));
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
    state.apply(enc(CombatantState.DYING));
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
    await settle(fixture);
    const el = fixture.nativeElement as HTMLElement;

    // First fall: the question shows, the master says "Ainda não".
    expect(el.querySelector('app-death-question [role="alertdialog"]')).not.toBeNull();
    const later = [...el.querySelectorAll('app-death-question button')].find((b) =>
      plain(b.textContent).includes('Ainda não'),
    ) as HTMLButtonElement;
    later.click();
    await settle(fixture);
    expect(el.querySelector('app-death-question [role="alertdialog"]')).toBeNull();

    // Healed: back to a normal state.
    state.apply(enc(CombatantState.UNHURT));
    await settle(fixture);
    expect(el.querySelector('app-death-question [role="alertdialog"]')).toBeNull();

    // Falls again and fails three times: the same id is DYING again, the question must show.
    state.apply(enc(CombatantState.DYING));
    await settle(fixture);
    expect(el.querySelector('app-death-question [role="alertdialog"]')).not.toBeNull();
  });
});
