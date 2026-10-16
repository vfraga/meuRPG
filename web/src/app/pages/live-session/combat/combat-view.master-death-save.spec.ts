// The master rolls the death save of a downed player who is not there (RN-03), and is asked before passing it unrolled.
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
  type Combatant,
  CombatantKind,
  CombatantSide,
  CombatantState,
  DeathSaveOutcome,
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
const brisa = (over: Partial<Combatant> = {}) =>
  combatant({
    id: 'b',
    label: 'Brisa',
    kind: CombatantKind.PLAYER,
    side: CombatantSide.PARTY,
    characterId: 'brisa-c',
    placed: false,
    state: CombatantState.DOWN,
    deathSaveDue: true,
    ...over,
  });
const goblin = combatant({ id: 'g', label: 'Goblin', placed: false });
const enc = (turn: string, ...members: ReturnType<typeof combatant>[]) =>
  encounter({
    mode: EncounterMode.THEATRE,
    mapId: '',
    gridColumns: 0,
    gridRows: 0,
    currentCombatantId: turn,
    combatants: [...members, goblin],
  });

async function settle(fixture: { detectChanges: () => void; whenStable: () => Promise<unknown> }) {
  await fixture.whenStable();
  await nextTurn();
  await fixture.whenStable();
  fixture.detectChanges();
}

const SAVE = { roll: undefined, outcome: DeathSaveOutcome.SUCCESS, failures: 0, stable: false };

async function mount(first: ReturnType<typeof enc>, diceMode = DiceMode.PLAYERS_CHOOSE) {
  vi.stubGlobal('matchMedia', (query: string) => ({
    matches: /min-width: 1024px/.test(query),
    media: query,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
    addListener: () => undefined,
    removeListener: () => undefined,
  }));
  const rolls: { combatantId: string; die: unknown }[] = [];
  const turnsEnded: string[] = [];
  const api = anyCallCombatClient((name) => {
    switch (name) {
      case 'turnOptions':
        return async () => ({
          options: create(TurnOptionsSchema, {}),
          attackTargets: [],
          spellTargets: [],
          pendingDamages: [],
        });
      case 'log':
        return async () => ({ rounds: [], undoableEventId: '' });
      case 'rollDeathSave':
        return async (_c: string, _e: string, combatantId: string, die: unknown) => {
          rolls.push({ combatantId, die });
          return {
            encounter: enc('b', brisa({ deathSaveDue: false, deathSuccesses: 1 })),
            save: SAVE,
          };
        };
      case 'endTurn':
        return async (_c: string, _e: string, who: string) => {
          turnsEnded.push(who);
          return enc('g', brisa());
        };
      default:
        return async () => ({});
    }
  });
  const state = new CombatState();
  state.apply(first);
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
  fixture.componentRef.setInput('diceMode', diceMode);
  fixture.componentRef.setInput('dicePreference', DicePreference.APP);
  fixture.detectChanges();
  await settle(fixture);
  return { fixture, el: fixture.nativeElement as HTMLElement, rolls, turnsEnded };
}

const button = (el: HTMLElement, text: string) =>
  [...el.querySelectorAll('button')].find((b) => plain(b.textContent).includes(text)) as
    HTMLButtonElement | undefined;

describe('the master rolls the death save of an absent player', () => {
  afterEach(() => TestBed.resetTestingModule());

  it('offers it for a downed player character on its turn with the save due', async () => {
    const { el } = await mount(enc('b', brisa()));
    expect(el.querySelector('app-death-save-due [data-testid="death-save-due"]')).not.toBeNull();
    expect(button(el, 'Rolar o teste contra a morte de Brisa')).toBeDefined();
    expect(button(el, 'Digitar o resultado')).toBeDefined();
  });

  it('does not offer it for an NPC, a stable or dead player, or a save not due', async () => {
    for (const first of [
      enc('g'),
      enc('b', brisa({ state: CombatantState.STABLE, deathSaveDue: false })),
      enc('b', brisa({ state: CombatantState.DEAD, deathSaveDue: false })),
      enc('b', brisa({ deathSaveDue: false })),
    ]) {
      const { el, fixture } = await mount(first);
      expect(el.querySelector('app-death-save-due [data-testid="death-save-due"]')).toBeNull();
      fixture.destroy();
      TestBed.resetTestingModule();
    }
  });

  it('is not on the player screen', async () => {
    const { fixture, el } = await mount(enc('b', brisa()));
    fixture.componentRef.setInput('isMaster', false);
    fixture.detectChanges();
    await settle(fixture);
    expect(el.querySelector('app-death-save-due [data-testid="death-save-due"]')).toBeNull();
  });

  it('rolls in the app with the combatant id and keeps the result line', async () => {
    const { fixture, el, rolls } = await mount(enc('b', brisa()));
    button(el, 'Rolar o teste contra a morte de Brisa')!.click();
    await settle(fixture);
    expect(rolls).toEqual([{ combatantId: 'b', die: { inApp: true } }]);
    expect(plain(el.querySelector('[data-testid="death-save-result"]')?.textContent)).toContain(
      'Teste contra a morte. Sucesso.',
    );
  });

  it('rolls a typed d20 with the combatant id', async () => {
    const { fixture, el, rolls } = await mount(enc('b', brisa()));
    button(el, 'Digitar o resultado')!.click();
    await settle(fixture);
    const field = el.querySelector('app-death-save-due input') as HTMLInputElement;
    field.value = '14';
    field.dispatchEvent(new Event('input'));
    field.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter' }));
    await settle(fixture);
    expect(rolls).toEqual([{ combatantId: 'b', die: { face: 14 } }]);
  });

  it('follows the dice mode: physical dice only type', async () => {
    const { el } = await mount(enc('b', brisa()), DiceMode.PHYSICAL);
    expect(button(el, 'Rolar o teste contra a morte de Brisa')).toBeUndefined();
    expect(el.querySelector('app-death-save-due input')).not.toBeNull();
  });

  it('asks in place on "Próximo turno" while the save is due, and "Passar sem rolar" ends the turn', async () => {
    const { fixture, el, turnsEnded } = await mount(enc('b', brisa()));
    button(el, 'Próximo turno')!.click();
    await settle(fixture);
    expect(turnsEnded).toEqual([]);
    expect(
      plain(el.querySelector('app-death-save-due [role="alertdialog"]')?.textContent),
    ).toContain('O teste contra a morte de Brisa ainda não foi rolado.');
    button(el, 'Passar sem rolar')!.click();
    await settle(fixture);
    expect(turnsEnded).toEqual(['b']);
  });

  it('"Rolar agora" puts the question away without passing the turn', async () => {
    const { fixture, el, turnsEnded } = await mount(enc('b', brisa()));
    button(el, 'Próximo turno')!.click();
    await settle(fixture);
    button(el, 'Rolar agora')!.click();
    await settle(fixture);
    expect(el.querySelector('app-death-save-due [role="alertdialog"]')).toBeNull();
    expect(turnsEnded).toEqual([]);
  });

  it('passes the turn at once when no save is due', async () => {
    const { fixture, el, turnsEnded } = await mount(enc('g', brisa({ deathSaveDue: false })));
    button(el, 'Próximo turno')!.click();
    await settle(fixture);
    expect(turnsEnded).toEqual(['g']);
  });
});
