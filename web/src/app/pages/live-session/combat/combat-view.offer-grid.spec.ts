import { By } from '@angular/platform-browser';
import { TestBed } from '@angular/core/testing';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatDialog } from '@angular/material/dialog';
import { provideRouter } from '@angular/router';
import { Code, ConnectError } from '@connectrpc/connect';

import { DiceMode, DicePreference } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  CombatantKind,
  CombatantSide,
  EncounterMode,
} from '../../../../gen/meurpg/play/v1/combat_pb';
import { TableRulesClient } from '../../../core/campaigns/table-rules';
import { CombatClient } from '../../../core/combat/combat-client';
import { anyCallCombatClient } from '../../../core/combat/combat-client-testing';
import { CombatState } from '../../../core/combat/combat-state';
import { combatant, encounter } from '../../../core/combat/combat-testing';
import { CreaturesClient } from '../../../core/creatures/creatures-client';
import { mapMessage, mapResponse } from '../../../core/maps/map-builders-testing';
import { MapState } from '../../../core/maps/map-state';
import { MapsClient } from '../../../core/maps/maps-client';
import { RosterClient } from '../../../core/maps/roster-client';
import { SpellCatalog } from '../../../core/combat/spell-catalog';
import { settle } from '../../../../test-settle';
import { CombatMapCard } from './combat-map-card/combat-map-card';
import { CombatView } from './combat-view';

// "Oferecer ataque de oportunidade" on a map: the panel opens in the NPC card, the candidates are ringed on the map.

const plain = (t: string | null | undefined) => (t ?? '').replace(/\s+/g, ' ').trim();

const toren = combatant({
  id: 't',
  label: 'Toren',
  kind: CombatantKind.PLAYER,
  side: CombatantSide.PARTY,
  placed: true,
  col: 4,
  row: 3,
});
const brisa = combatant({
  id: 'b',
  label: 'Brisa',
  kind: CombatantKind.PLAYER,
  side: CombatantSide.PARTY,
  placed: true,
  col: 8,
  row: 8,
});
const spent = combatant({
  id: 'p',
  label: 'Pensantus',
  kind: CombatantKind.PLAYER,
  side: CombatantSide.PARTY,
  placed: true,
  col: 6,
  row: 6,
  reactionUsed: true,
});
const goblin = combatant({ id: 'g', label: 'Goblin', placed: true, col: 3, row: 3 });

function setup(
  offer?: (args: unknown[]) => unknown,
  screen: 'laptop' | 'tablet' | 'phone' = 'laptop',
) {
  TestBed.resetTestingModule();
  vi.stubGlobal('matchMedia', (query: string) => ({
    matches:
      (screen === 'phone' && /max-width: 767/.test(query)) ||
      (screen === 'laptop' && /min-width: 1024px/.test(query)),
    media: query,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
    addListener: () => undefined,
    removeListener: () => undefined,
  }));
  const sent: unknown[][] = [];
  const state = new CombatState();
  state.apply(
    encounter({
      mode: EncounterMode.GRID,
      mapId: 'm1',
      gridColumns: 12,
      gridRows: 12,
      currentCombatantId: 'g',
      combatants: [toren, brisa, spent, goblin],
    }),
  );
  const api = anyCallCombatClient((name) => {
    switch (name) {
      case 'offerOpportunity':
        return async (...a: unknown[]) => {
          sent.push(a);
          return offer ? offer(a) : state.encounter();
        };
      case 'log':
        return async () => ({ rounds: [], undoableEventId: '' });
      case 'turnOptions':
        // Two attacks measure Toren twice (the nearer one counts); Brisa has no distance.
        return async () => ({
          options: undefined,
          attackTargets: [
            {
              attackKey: 'a',
              targets: [
                { combatantId: 't', label: 'Toren', distanceFt: 15 },
                { combatantId: 'b', label: 'Brisa' },
              ],
            },
            { attackKey: 'b', targets: [{ combatantId: 't', label: 'Toren', distanceFt: 10 }] },
          ],
          spellTargets: [],
          pendingDamages: [],
        });
      default:
        return async () => ({});
    }
  });
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
  const covil = mapMessage('m1', 'Covil');
  const maps = new MapState(async () => mapResponse(covil));
  maps.map.set(covil);
  const fixture = TestBed.createComponent(CombatView);
  fixture.componentRef.setInput('campaignId', 'c');
  fixture.componentRef.setInput('isMaster', true);
  fixture.componentRef.setInput('state', state);
  fixture.componentRef.setInput('mapState', maps);
  fixture.componentRef.setInput('diceMode', DiceMode.PLAYERS_CHOOSE);
  fixture.componentRef.setInput('dicePreference', DicePreference.APP);
  fixture.detectChanges();
  return { fixture, el: fixture.nativeElement as HTMLElement, sent };
}

async function openPanel(fixture: Parameters<typeof settle>[0], el: HTMLElement) {
  await settle(fixture);
  el.querySelector<HTMLButtonElement>('button[aria-label="Mais ações de Goblin"]')!.click();
  await settle(fixture);
  [...document.querySelectorAll<HTMLElement>('[role="menuitem"]')]
    .find((i) => plain(i.textContent).includes('Oferecer ataque de oportunidade'))!
    .click();
  await settle(fixture);
}

const rings = (el: HTMLElement) =>
  [...el.querySelectorAll<HTMLElement>('.cm__pick')].map((b) => b.getAttribute('data-pick'));

describe('CombatView on a map: "Oferecer ataque de oportunidade"', () => {
  afterEach(() => {
    document.querySelectorAll('.cdk-overlay-container > *').forEach((n) => n.remove());
    vi.unstubAllGlobals();
  });

  it('opens the panel in the card with the distances, the footer and the first candidate picked', async () => {
    const { fixture, el } = setup();
    await openPanel(fixture, el);
    const panel = el.querySelector('app-npc-card app-offer-panel')!;
    expect(panel).not.toBeNull();
    const text = plain(panel.textContent);
    expect(text).toContain('Oferecer ataque de oportunidade');
    expect(text).toContain('Os candidatos têm um anel no mapa.');
    // The nearer of the two measures (10 ft), none for the one the server could not measure.
    const subs = [...panel.querySelectorAll('.row__sub')].map((e) => plain(e.textContent));
    expect(subs).toEqual(['agora a 3 m', '', 'Reação usada']);
    expect(text).toContain('A distância é do mapa, só para ajudar: quem decide é você.');
    expect(plain(panel.querySelector('.actions')?.textContent)).toBe(
      'Não oferecer Oferecer a Toren',
    );
    // Choosing from the menu leaves the focus on the panel's checked radio, not back on "Mais ações".
    expect(document.activeElement).toBe(panel.querySelector('input:checked'));
  });

  it('choosing the offer again while the panel is open keeps it open and puts the focus on its checked radio', async () => {
    const { fixture, el } = setup();
    await openPanel(fixture, el);
    const panel = el.querySelector('app-offer-panel')!;
    (el.querySelector('input[type="radio"]:not(:checked):not(:disabled)') as HTMLElement).focus();
    await openPanel(fixture, el);
    await settle(fixture);
    expect(el.querySelector('app-offer-panel')).toBe(panel);
    expect(document.activeElement).toBe(panel.querySelector('input:checked'));
  });

  it('on a tablet (768 to 1023 px) the map is another tab too: no ring talk', async () => {
    const { fixture, el } = setup(undefined, 'tablet');
    await openPanel(fixture, el);
    const text = plain(el.querySelector('app-offer-panel')?.textContent);
    expect(text).toContain('Escolha quem saiu do alcance.');
    expect(text).not.toContain('anel');
  });

  it('on a phone the panel talks of no rings (the map is the Mapa tab)', async () => {
    const { fixture, el } = setup(undefined, 'phone');
    await openPanel(fixture, el);
    const text = plain(el.querySelector('app-offer-panel')?.textContent);
    expect(text).toContain('Escolha quem saiu do alcance.');
    expect(text).not.toContain('anel');
  });

  it('rings the candidates on the map, not the one with the reaction spent, and names the ring in the legend', async () => {
    const { fixture, el } = setup();
    expect(rings(el)).toEqual([]);
    await openPanel(fixture, el);
    expect(rings(el)).toEqual(['t', 'b']);
    expect(plain(el.querySelector('app-combat-map-card')?.textContent)).toContain(
      'Pode receber a oferta',
    );
    const on = [...el.querySelectorAll('.cm__pick--on')].map((b) => b.getAttribute('data-pick'));
    expect(on).toEqual(['t']);
    expect(el.querySelectorAll('.cm__pick--offer').length).toBe(2);
  });

  it('a tap on a ring picks that candidate in the panel, and the offer goes to it', async () => {
    const { fixture, el, sent } = setup();
    await openPanel(fixture, el);
    fixture.debugElement.query(By.directive(CombatMapCard)).triggerEventHandler('pickTap', 'b');
    await settle(fixture);
    expect(plain(el.querySelector('app-offer-panel .actions')?.textContent)).toContain(
      'Oferecer a Brisa',
    );
    expect(
      [...el.querySelectorAll('.cm__pick--on')].map((b) => b.getAttribute('data-pick')),
    ).toEqual(['b']);
    [...el.querySelectorAll<HTMLButtonElement>('app-offer-panel button')]
      .find((b) => plain(b.textContent) === 'Oferecer a Brisa')!
      .click();
    await settle(fixture);
    expect(sent.length).toBe(1);
    expect(sent[0].slice(2, 4)).toEqual(['g', 'b']);
    // Offered: the panel and the rings go.
    expect(el.querySelector('app-offer-panel')).toBeNull();
    expect(rings(el)).toEqual([]);
  });

  it('nothing is offered by itself: opening the panel sends nothing', async () => {
    const { fixture, el, sent } = setup();
    await openPanel(fixture, el);
    expect(sent).toEqual([]);
  });

  it('shows the refusal in the panel and keeps the rings', async () => {
    const { fixture, el } = setup(() => {
      throw new ConnectError('no', Code.PermissionDenied);
    });
    await openPanel(fixture, el);
    [...el.querySelectorAll<HTMLButtonElement>('app-offer-panel button')]
      .find((b) => plain(b.textContent) === 'Oferecer a Toren')!
      .click();
    await settle(fixture);
    expect(plain(el.querySelector('app-offer-panel [role="alert"]')?.textContent)).toContain(
      'Você não pode fazer isso agora.',
    );
    expect(rings(el)).toEqual(['t', 'b']);
  });

  it('"Não oferecer" closes the panel and takes the rings off', async () => {
    const { fixture, el } = setup();
    await openPanel(fixture, el);
    [...el.querySelectorAll<HTMLButtonElement>('app-offer-panel button')]
      .find((b) => plain(b.textContent) === 'Não oferecer')!
      .click();
    await settle(fixture);
    expect(el.querySelector('app-offer-panel')).toBeNull();
    expect(rings(el)).toEqual([]);
  });
});
