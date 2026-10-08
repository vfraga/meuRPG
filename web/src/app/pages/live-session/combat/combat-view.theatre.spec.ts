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
  OpportunityOfferSchema,
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

// The combat screen of a combat without a map (RN-25, E10-04), as the page mounts it: what each audience gets, and what is not drawn.

const plain = (t: string | null | undefined) => (t ?? '').replace(/\s+/g, ' ').trim();
const toren = combatant({
  id: 't',
  label: 'Toren',
  kind: CombatantKind.PLAYER,
  side: CombatantSide.PARTY,
  characterId: 'toren-c',
  hitPointsCurrent: 49,
  hitPointsMax: 49,
  armorClass: 16,
  mine: true,
  placed: false,
});
const brisa = combatant({
  id: 'b',
  label: 'Brisa',
  kind: CombatantKind.PLAYER,
  side: CombatantSide.PARTY,
  characterId: 'brisa-c',
  placed: false,
});
const goblin = combatant({
  id: 'g',
  label: 'Goblin',
  placed: false,
  hitPointsCurrent: 7,
  hitPointsMax: 7,
  armorClass: 15,
});
const cap = combatant({
  id: 'cap',
  label: 'Capitão Goblin',
  placed: false,
  hitPointsCurrent: 27,
  hitPointsMax: 27,
  armorClass: 13,
});

/** A laptop-sized screen (1024 px): the master has the combat bar and the card with its title, the player the desktop layout. */
function laptop(): void {
  vi.stubGlobal('matchMedia', (query: string) => ({
    matches: /min-width: 1024px/.test(query),
    media: query,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
    addListener: () => undefined,
    removeListener: () => undefined,
  }));
}

function setup(opts: {
  master: boolean;
  current: string;
  combatants?: ReturnType<typeof combatant>[];
  offers?: unknown[];
  deathSaves?: DeathSaveVisibility;
  mode?: EncounterMode;
}) {
  // A test may mount the screen twice (two tables to compare): start from a clean module each time.
  TestBed.resetTestingModule();
  laptop();
  const calls: { offer: unknown[][]; withdraw: string[] } = { offer: [], withdraw: [] };
  const api = new Proxy(
    {},
    {
      get: (_t, name: string) => {
        switch (name) {
          case 'offerOpportunity':
            return async (...a: unknown[]) => {
              calls.offer.push(a);
              return state.encounter()!;
            };
          case 'withdrawOpportunity':
            return async (_c: string, _e: string, id: string) => {
              calls.withdraw.push(id);
              return state.encounter()!;
            };
          case 'log':
            return async () => ({ rounds: [], undoableEventId: '' });
          case 'turnOptions':
            return async () => ({
              options: create(TurnOptionsSchema, {}),
              attackTargets: [],
              spellTargets: [],
              pendingDamages: [],
            });
          default:
            return async () => ({});
        }
      },
    },
  );
  const state = new CombatState();
  state.apply(
    encounter({
      mode: opts.mode ?? EncounterMode.THEATRE,
      mapId: '',
      gridColumns: 0,
      gridRows: 0,
      currentCombatantId: opts.current,
      combatants: opts.combatants ?? [toren, brisa, goblin, cap],
      opportunityOffers: (opts.offers ?? []) as never,
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
          get: async () => ({
            saved: { deathSaves: opts.deathSaves ?? DeathSaveVisibility.VISIBLE_TO_ALL },
          }),
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
  fixture.componentRef.setInput('isMaster', opts.master);
  fixture.componentRef.setInput('state', state);
  fixture.componentRef.setInput('mapState', new MapState(async () => ({}) as never));
  fixture.componentRef.setInput('diceMode', DiceMode.PLAYERS_CHOOSE);
  fixture.componentRef.setInput('dicePreference', DicePreference.APP);
  fixture.detectChanges();
  return { fixture, el: fixture.nativeElement as HTMLElement, state, calls };
}

async function settle(fixture: {
  detectChanges: () => void;
  whenStable: () => Promise<unknown>;
}): Promise<void> {
  await fixture.whenStable();
  await new Promise((r) => setTimeout(r, 0));
  await fixture.whenStable();
  fixture.detectChanges();
}

const press = (el: HTMLElement, name: string) =>
  [...el.querySelectorAll('button')].find((b) => plain(b.textContent).includes(name))!.click();

describe("CombatView, the master's screen without a map", () => {
  it('draws a card, the order, the cover and the log: no map, no "Sem quadrado no mapa", and the mode\'s pill', async () => {
    const { fixture, el } = setup({ master: true, current: 'g' });
    await settle(fixture);
    expect(el.querySelector('app-combat-map-card')).toBeNull();
    expect(plain(el.textContent)).not.toContain('Sem quadrado no mapa');
    expect(plain(el.textContent)).toContain('Cobertura dos alvos');
    expect(plain(el.textContent)).toContain('Ações do Goblin');
    expect(plain(el.querySelector('app-combat-bar')?.textContent)).toContain('Teatro da mente');
    expect(el.querySelector('app-order-list')).not.toBeNull();
  });

  it("opens the offer's form right under its button, inside the card, with one key per reactor, and closes it with the turn", async () => {
    const { fixture, el, calls, state } = setup({ master: true, current: 'g' });
    await settle(fixture);
    expect(el.querySelector('app-offer-panel')).toBeNull();
    press(el, 'Oferecer ataque de oportunidade');
    fixture.detectChanges();
    const card = el.querySelector('app-npc-card')!;
    expect(card.querySelector('app-offer-panel')).not.toBeNull();
    // The button comes before the form in the card.
    const button = [...card.querySelectorAll('button')].find((b) =>
      plain(b.textContent).includes('Oferecer ataque de oportunidade'),
    )!;
    expect(
      button.compareDocumentPosition(card.querySelector('app-offer-panel')!) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    await settle(fixture);
    press(el, 'Oferecer a ');
    await settle(fixture);
    expect(calls.offer.length).toBe(1);
    expect(calls.offer[0].slice(2, 4)).toEqual(['g', 't']);
    expect(typeof calls.offer[0][4]).toBe('string');
    // The turn passes: the form closes.
    state.applyTurn({ encounterId: 'enc', round: 1, currentCombatantId: 't', masterTurn: false });
    fixture.detectChanges();
    expect(el.querySelector('app-offer-panel')).toBeNull();
  });

  it("does not disable the offer while another reactor's offer waits, and dashes it with the reason only when nobody can react", async () => {
    const waiting = create(OpportunityOfferSchema, {
      id: 'o1',
      moverId: 'g',
      reactorId: 't',
      moverLabel: 'Goblin',
      reactorLabel: 'Toren',
      forYou: true,
    });
    const { fixture, el } = setup({ master: true, current: 'g', offers: [waiting] });
    await settle(fixture);
    const offer = [...el.querySelectorAll('button')].find((b) =>
      plain(b.textContent).includes('Oferecer ataque de oportunidade'),
    )!;
    expect(offer.getAttribute('aria-disabled')).not.toBe('true');
    const both = [
      create(OpportunityOfferSchema, { id: 'o1', moverId: 'g', reactorId: 't' }),
      create(OpportunityOfferSchema, { id: 'o2', moverId: 'g', reactorId: 'b' }),
    ];
    const alone = setup({ master: true, current: 'g', offers: both });
    await settle(alone.fixture);
    const off = [...alone.el.querySelectorAll('button')].find((b) =>
      plain(b.textContent).includes('Oferecer ataque de oportunidade'),
    )!;
    expect(off.getAttribute('aria-disabled')).toBe('true');
    expect(plain(alone.el.textContent)).toContain('Ninguém pode reagir agora.');
  });

  it('gives a player on turn the card with the movement he may spend, but no offer; and none for a fallen one', async () => {
    const { fixture, el } = setup({ master: true, current: 't' });
    await settle(fixture);
    expect(plain(el.textContent)).toContain('Ações do Toren');
    expect(plain(el.textContent)).toContain('Gastar movimento');
    expect(plain(el.textContent)).not.toContain('Oferecer ataque de oportunidade');
    const down = setup({
      master: true,
      current: 'b',
      combatants: [toren, { ...brisa, state: CombatantState.DOWN }, goblin, cap],
    });
    await settle(down.fixture);
    expect(plain(down.el.querySelector('app-npc-card')?.textContent)).not.toContain(
      'Gastar movimento',
    );
  });
});

describe("CombatView, the player's screen without a map", () => {
  it('draws the panel in the map\'s place, "Gastar movimento" and the reaction card; no "Mover", no map, no familiar eyes', async () => {
    const { fixture, el } = setup({ master: false, current: 't' });
    await settle(fixture);
    expect(el.querySelector('app-no-map-panel')).not.toBeNull();
    expect(el.querySelector('app-combat-map-card')).toBeNull();
    const text = plain(el.textContent);
    expect(text).toContain('Combate sem mapa');
    expect(text).toContain('Gastar movimento');
    expect(text).not.toContain('Ver mapa');
    expect(text).not.toContain('Ver pelos olhos');
    expect(el.querySelector('app-theatre-reaction')).not.toBeNull();
    expect([...el.querySelectorAll('button')].some((b) => plain(b.textContent) === 'Mover')).toBe(
      false,
    );
  });

  it('shows no live control to a fallen character: no reaction card, no movement', async () => {
    const { fixture, el } = setup({
      master: false,
      current: 'g',
      combatants: [{ ...toren, state: CombatantState.DOWN }, brisa, goblin, cap],
    });
    await settle(fixture);
    expect(el.querySelector('app-theatre-reaction')).toBeNull();
    expect(plain(el.textContent)).not.toContain('Gastar movimento');
  });

  it('says why a table hides the death saves only to the other players, and only when someone is down', async () => {
    const hidden = DeathSaveVisibility.OWNER_AND_MASTER;
    const none = setup({ master: false, current: 't', deathSaves: hidden });
    await settle(none.fixture);
    expect(none.el.querySelector('app-combat-log-panel')).not.toBeNull();
    expect(plain(none.el.textContent)).not.toContain('Esta mesa só deixa o dono e o mestre');
    const someone = setup({
      master: false,
      current: 'b',
      deathSaves: hidden,
      combatants: [toren, { ...brisa, state: CombatantState.DOWN }, goblin, cap],
    });
    await settle(someone.fixture);
    expect(someone.el.querySelector('[data-testid="death-hidden-note"]')).not.toBeNull();
    // The owner of the fallen character sees their own marks: the note is for the others.
    const mine = setup({
      master: false,
      current: 't',
      deathSaves: hidden,
      combatants: [{ ...toren, state: CombatantState.DOWN }, brisa, goblin, cap],
    });
    await settle(mine.fixture);
    expect(mine.el.querySelector('[data-testid="death-hidden-note"]')).toBeNull();
  });
});
