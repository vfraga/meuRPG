import { TestBed } from '@angular/core/testing';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatDialog } from '@angular/material/dialog';
import { provideRouter } from '@angular/router';
import { create } from '@bufbuild/protobuf';

import {
  DeathSaveVisibility,
  DiceMode,
  DicePreference,
} from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  CombatantKind,
  CombatantSide,
  EncounterMode,
  HiddenRevealQuestionSchema,
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
import { nextTurn } from '../../../../test-settle';

// With --coverage test-setup.ts only hooks the first file of a worker, so the stubbed globals this file leaves on would
// reach the next file (a real setTimeout never fires on a fake clock).
afterEach(() => {
  vi.unstubAllGlobals();
});

// A player's area spell hit hidden creatures and the table asks (PM-02c): the master's question and the held turn, as each
// audience reads them.

const plain = (t: string | null | undefined) =>
  (t ?? '').replace(/ /g, ' ').replace(/\s+/g, ' ').trim();

const pensantus = (mine: boolean) =>
  combatant({
    id: 'p',
    label: 'Pensantus',
    kind: CombatantKind.PLAYER,
    side: CombatantSide.PARTY,
    characterId: 'pen-c',
    mine,
  });
const salvia = (mine: boolean) =>
  combatant({
    id: 's',
    label: 'Sálvia',
    kind: CombatantKind.PLAYER,
    side: CombatantSide.PARTY,
    characterId: 'sal-c',
    mine,
  });
const g3 = combatant({ id: 'g3', label: 'Goblin 3', hidden: true });

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

function setup(opts: { master: boolean; me?: 'p' | 's'; held: boolean; questions?: number }) {
  TestBed.resetTestingModule();
  laptop();
  const resolved: unknown[][] = [];
  const state = new CombatState();
  const api = new Proxy(
    {},
    {
      get: (_t, name: string) => {
        switch (name) {
          case 'resolveHiddenReveal':
            return async (...a: unknown[]) => {
              resolved.push(a);
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
  const question = (i: number) =>
    create(HiddenRevealQuestionSchema, {
      id: `q${i + 1}`,
      casterId: 'p',
      spellKey: 'spell:fireball',
      combatantIds: ['g3'],
    });
  const combatants = opts.master
    ? [pensantus(false), salvia(false), g3]
    : [pensantus(opts.me === 'p'), salvia(opts.me === 's')];
  state.apply(
    encounter({
      mode: EncounterMode.GRID,
      currentCombatantId: 'p',
      combatants,
      turnHeld: opts.held,
      pendingHiddenReveals: opts.master
        ? Array.from({ length: opts.questions ?? 1 }, (_, i) => question(i))
        : [],
    }),
  );
  TestBed.configureTestingModule({
    providers: [
      provideRouter([]),
      { provide: CombatClient, useValue: api },
      { provide: RosterClient, useValue: { list: async () => [] } },
      { provide: MapsClient, useValue: { layers: async () => ({}) } },
      {
        provide: SpellCatalog,
        useValue: { details: async () => ({ spell: { namePt: 'Bola de Fogo' } }) },
      },
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
  fixture.componentRef.setInput('isMaster', opts.master);
  fixture.componentRef.setInput('state', state);
  fixture.componentRef.setInput('mapState', new MapState(async () => ({}) as never));
  fixture.componentRef.setInput('diceMode', DiceMode.PLAYERS_CHOOSE);
  fixture.componentRef.setInput('dicePreference', DicePreference.APP);
  fixture.detectChanges();
  return { fixture, el: fixture.nativeElement as HTMLElement, resolved, state };
}

async function settle(fixture: { detectChanges: () => void; whenStable: () => Promise<unknown> }) {
  await fixture.whenStable();
  await nextTurn();
  await fixture.whenStable();
  fixture.detectChanges();
}

describe('CombatView: a turn held by a question about hidden creatures', () => {
  it('tells the caster only "Esperando o mestre", and offers no end of turn', async () => {
    const { fixture, el } = setup({ master: false, me: 'p', held: true });
    await settle(fixture);
    const notice = plain(el.querySelector('app-turn-panel [role="status"].mr-notice')?.textContent);
    expect(notice).toContain('Esperando o mestre. A sua vez continua quando ele responder.');
    expect(plain(el.textContent)).not.toMatch(/escondid|Goblin 3|pergunta/i);
    const end = [...el.querySelectorAll<HTMLButtonElement>('app-end-turn button')].find((b) =>
      plain(b.textContent).includes('Encerrar turno'),
    );
    expect(end?.getAttribute('aria-disabled') === 'true' || end?.disabled).toBe(true);
  });

  it('tells another player the turn waits for the master, and nothing of why', async () => {
    const { fixture, el } = setup({ master: false, me: 's', held: true });
    await settle(fixture);
    expect(plain(el.querySelector('app-turn-panel')?.textContent)).toContain(
      'Esperando o mestre. O turno continua quando ele responder.',
    );
    expect(plain(el.textContent)).not.toMatch(/escondid|pergunta/i);
  });

  it('says nothing when the turn is not held', async () => {
    const { fixture, el } = setup({ master: false, me: 's', held: false });
    await settle(fixture);
    expect(plain(el.textContent)).not.toContain('Esperando o mestre');
  });

  it('gives the master the question, the bar\'s wait and "Próximo turno" off with its reason; the answer goes to the server', async () => {
    const { fixture, el, resolved } = setup({ master: true, held: true });
    await settle(fixture);
    expect(plain(el.querySelector('app-combat-bar')?.textContent)).toContain(
      'Esperando a sua resposta: escondidas atingidas',
    );
    const next = el.querySelector<HTMLButtonElement>('app-combat-bar button.next')!;
    expect(next.getAttribute('aria-disabled')).toBe('true');
    expect(plain(el.querySelector('app-combat-bar .why')?.textContent)).toContain(
      'Responda ao pedido abaixo para seguir.',
    );
    const card = el.querySelector('app-hidden-reveal-card section')!;
    expect(plain(card.querySelector('h2')?.textContent)).toBe(
      'Bola de Fogo atingiu 1 criatura escondida',
    );
    [...card.querySelectorAll<HTMLButtonElement>('button')][0].click();
    await settle(fixture);
    expect(resolved).toEqual([['c', 'enc', 'q1', true]]);
  });

  it("counts two questions on the master's bar", async () => {
    const { fixture, el } = setup({ master: true, held: true, questions: 2 });
    await settle(fixture);
    expect(plain(el.querySelector('app-combat-bar')?.textContent)).toContain(
      'Esperando a sua resposta: 2 perguntas de escondidas',
    );
    expect(plain(el.querySelector('app-combat-bar .why')?.textContent)).toContain(
      'Responda aos pedidos abaixo para seguir.',
    );
  });
});

describe('CombatView: "Combate atualizado agora." after a reload (PM-02c 9c)', () => {
  const line = (el: HTMLElement) => el.querySelector('p.refreshed[role="status"]');

  it('says the screen was rebuilt when the page comes up with the turn held, to the master and to the player', async () => {
    for (const opts of [
      { master: true, held: true },
      { master: false, me: 'p' as const, held: true },
    ]) {
      const { fixture, el } = setup(opts);
      await settle(fixture);
      expect(plain(line(el)?.textContent)).toBe('Combate atualizado agora.');
    }
  });

  it('says nothing when the turn is not held, and the line goes when the wait is over', async () => {
    const quiet = setup({ master: false, me: 'p', held: false });
    await settle(quiet.fixture);
    expect(line(quiet.el)).toBeNull();

    const { fixture, el, state } = setup({ master: false, me: 'p', held: true });
    await settle(fixture);
    expect(line(el)).not.toBeNull();
    state.apply(
      encounter({
        mode: EncounterMode.GRID,
        currentCombatantId: 'p',
        combatants: [pensantus(true), salvia(false)],
        turnHeld: false,
      }),
    );
    await settle(fixture);
    expect(line(el)).toBeNull();
  });
});
