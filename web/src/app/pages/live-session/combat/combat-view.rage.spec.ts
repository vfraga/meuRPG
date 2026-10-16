// The rage question: the owner and the master are asked, and each answer reaches the server.
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
  EncounterMode,
} from '../../../../gen/meurpg/play/v1/combat_pb';
import {
  CombatantEffectSchema,
  CombatantStateKind,
} from '../../../../gen/meurpg/play/v1/combat_rolls_pb';
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

// With --coverage test-setup.ts only hooks the first file of a worker, so the stubbed globals this file leaves on would
// reach the next file (a real setTimeout never fires on a fake clock).
afterEach(() => {
  vi.unstubAllGlobals();
});

const plain = (t: string | null | undefined) => (t ?? '').replace(/\s+/g, ' ').trim();
const toren = combatant({
  id: 't',
  label: 'Toren',
  kind: CombatantKind.PLAYER,
  side: CombatantSide.PARTY,
  characterId: 'toren-c',
  mine: true,
  placed: false,
  states: [
    create(CombatantEffectSchema, { id: 's', kind: CombatantStateKind.RAGE, labelPt: 'Em fúria' }),
  ],
});
const goblin = combatant({ id: 'g', label: 'Goblin', placed: false });
const enc = (pending: string) =>
  encounter({
    mode: EncounterMode.THEATRE,
    mapId: '',
    gridColumns: 0,
    gridRows: 0,
    currentCombatantId: 't',
    combatants: [toren, goblin],
    ragePendingCombatantId: pending,
    rollModeRequests: [],
  });

async function settle(fixture: { detectChanges: () => void; whenStable: () => Promise<unknown> }) {
  await fixture.whenStable();
  await nextTurn();
  await fixture.whenStable();
  fixture.detectChanges();
}

async function open(isMaster: boolean, pending: string) {
  vi.stubGlobal('matchMedia', (query: string) => ({
    matches: /min-width: 1024px/.test(query),
    media: query,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
    addListener: () => undefined,
    removeListener: () => undefined,
  }));
  const answered: unknown[][] = [];
  const ended: unknown[][] = [];
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
            : name === 'endTurn'
              ? async (...args: unknown[]) => {
                  ended.push(args);
                  return enc('');
                }
              : name === 'answerRageEnd'
                ? async (...args: unknown[]) => {
                    answered.push(args);
                    return enc('');
                  }
                : async () => ({}),
    },
  );
  const state = new CombatState();
  state.apply(enc(pending));
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
  fixture.componentRef.setInput('isMaster', isMaster);
  fixture.componentRef.setInput('state', state);
  fixture.componentRef.setInput('mapState', new MapState(async () => ({}) as never));
  fixture.componentRef.setInput('diceMode', DiceMode.PLAYERS_CHOOSE);
  fixture.componentRef.setInput('dicePreference', DicePreference.APP);
  fixture.detectChanges();
  await settle(fixture);
  return { fixture, el: fixture.nativeElement as HTMLElement, answered, ended, state };
}

const button = (el: HTMLElement, text: string) =>
  [...el.querySelectorAll('app-rage-question button')].find((b) =>
    plain(b.textContent).includes(text),
  ) as HTMLButtonElement;

describe('CombatView: the rage question', () => {
  it('asks the owner and sends "Voltar e atacar" as end_rage false', async () => {
    const { fixture, el, answered, ended } = await open(false, 't');
    expect(
      plain(el.querySelector('app-rage-question [role="alertdialog"]')?.textContent),
    ).toContain('A sua fúria vai acabar?');
    button(el, 'Voltar e atacar').click();
    await settle(fixture);
    expect(answered).toEqual([['c', 'enc', 't', false]]);
    expect(ended).toEqual([]);
    expect(el.querySelector('app-rage-question [role="alertdialog"]')).toBeNull();
  });

  it('asks the master about the character and sends "Deixar a fúria acabar" as end_rage true', async () => {
    const { fixture, el, answered, ended } = await open(true, 't');
    expect(
      plain(el.querySelector('app-rage-question [role="alertdialog"]')?.textContent),
    ).toContain('A fúria de Toren vai acabar?');
    button(el, 'Deixar a fúria acabar').click();
    await settle(fixture);
    expect(answered).toEqual([['c', 'enc', 't', true]]);
    // One tap: the rage ends and the turn passes (a single end-turn call, for the raging barbarian).
    expect(ended).toHaveLength(1);
    expect(ended[0].slice(0, 4)).toEqual(['c', 'enc', 't', false]);
  });

  it('asks nobody when no rage waits', async () => {
    const { el } = await open(false, '');
    expect(el.querySelector('app-rage-question [role="alertdialog"]')).toBeNull();
  });
});
