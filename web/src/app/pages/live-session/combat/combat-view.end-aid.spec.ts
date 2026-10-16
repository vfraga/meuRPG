// The master's "Encerrar Ajuda" reaches the server as the new effect action.
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
  CombatEffect,
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
import { nextTurn } from '../../../../test-settle';
import { anyCallCombatClient } from '../../../core/combat/combat-client-testing';

// With --coverage test-setup.ts only hooks the first file of a worker, so the stubbed globals this file leaves on would
// reach the next file (a real setTimeout never fires on a fake clock).
afterEach(() => {
  vi.unstubAllGlobals();
});

const plain = (t: string | null | undefined) => (t ?? '').replace(/\s+/g, ' ').trim();
const salvia = combatant({
  id: 's',
  label: 'Sálvia',
  kind: CombatantKind.PLAYER,
  side: CombatantSide.PARTY,
  characterId: 'salvia-c',
  placed: false,
  state: CombatantState.UNHURT,
  hitPointsCurrent: 43,
  hitPointsMax: 43,
  hitPointsMaxBonus: 5,
});
const goblin = combatant({ id: 'g', label: 'Goblin', placed: false });
const enc = () =>
  encounter({
    mode: EncounterMode.THEATRE,
    mapId: '',
    gridColumns: 0,
    gridRows: 0,
    currentCombatantId: 'g',
    combatants: [salvia, goblin],
  });

async function settle(fixture: { detectChanges: () => void; whenStable: () => Promise<unknown> }) {
  await fixture.whenStable();
  await nextTurn();
  await fixture.whenStable();
  fixture.detectChanges();
}

describe('"Encerrar Ajuda" in the master\'s order of initiative', () => {
  it('sends the Ajuda of that combatant to the server once the master confirms', async () => {
    const ended: unknown[][] = [];
    vi.stubGlobal('matchMedia', (query: string) => ({
      matches: /min-width: 1024px/.test(query),
      media: query,
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
      addListener: () => undefined,
      removeListener: () => undefined,
    }));
    const api = anyCallCombatClient((name) =>
      name === 'endCombatEffect'
        ? async (...args: unknown[]) => {
            ended.push(args);
            return enc();
          }
        : name === 'turnOptions'
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
    const state = new CombatState();
    state.apply(enc());
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

    const more = el.querySelector<HTMLButtonElement>(
      'button[aria-label="Mais ações para Sálvia"]',
    )!;
    more.click();
    await settle(fixture);
    const item = [...document.querySelectorAll<HTMLButtonElement>('.mat-mdc-menu-item')].find((b) =>
      plain(b.textContent).includes('Encerrar Ajuda em Sálvia'),
    )!;
    item.click();
    await settle(fixture);
    const sure = [...el.querySelectorAll<HTMLButtonElement>('app-end-aid-question button')].find(
      (b) => plain(b.textContent) === 'Encerrar Ajuda',
    )!;
    expect(ended).toEqual([]);
    sure.click();
    await settle(fixture);
    expect(ended).toEqual([['c', 'enc', 's', CombatEffect.AID]]);
    expect(el.querySelector('app-end-aid-question')).toBeNull();
  });
});
