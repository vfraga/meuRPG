import { TestBed } from '@angular/core/testing';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatDialog } from '@angular/material/dialog';
import { provideRouter } from '@angular/router';
import { create } from '@bufbuild/protobuf';
import { Subject, of } from 'rxjs';

import { DiceMode, DicePreference } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  CombatantKind,
  CombatantSide,
  EncounterMode,
  ReactionKind,
} from '../../../../gen/meurpg/play/v1/combat_pb';
import { AttackKind, TurnOptionsSchema } from '../../../../gen/meurpg/rules/v1/rules_pb';
import { TableRulesClient } from '../../../core/campaigns/table-rules';
import { CombatClient } from '../../../core/combat/combat-client';
import { CombatState } from '../../../core/combat/combat-state';
import { combatant, encounter, reactionWindow } from '../../../core/combat/combat-testing';
import { CreaturesClient } from '../../../core/creatures/creatures-client';
import { MapState } from '../../../core/maps/map-state';
import { MapsClient } from '../../../core/maps/maps-client';
import { RosterClient } from '../../../core/maps/roster-client';
import { SpellCatalog } from '../../../core/combat/spell-catalog';
import { AttackSheet } from './attack-sheet/attack-sheet';
import { CombatView } from './combat-view';
import { ReactionSheet } from './reaction-sheet/reaction-sheet';
import { nextTurn } from '../../../../test-settle';
import { anyCallCombatClient } from '../../../core/combat/combat-client-testing';

// The reaction window on the combat screen (PM-04): the sheet that opens by itself for the player, the master's cards,
// the line that waits, and the sentence of a window that closed by itself.

const plain = (t: string | null | undefined) =>
  (t ?? '').replace(/ /g, ' ').replace(/\s+/g, ' ').trim();

const toren = combatant({
  id: 't',
  label: 'Toren',
  kind: CombatantKind.PLAYER,
  side: CombatantSide.PARTY,
  characterId: 'toren-c',
  mine: true,
  placed: false,
});
const goblin = combatant({ id: 'g', label: 'Goblin', placed: false });
const mago = combatant({ id: 'm1', label: 'Mago 1', placed: false });

const shield = (over: object = {}) =>
  reactionWindow({
    id: 'w1',
    kind: ReactionKind.SHIELD,
    reactorId: 't',
    reactorLabel: 'Toren',
    reactorIsPlayer: true,
    prompt: {
      case: 'shield',
      value: {
        attackerLabel: 'Goblin',
        spellNamePt: 'Escudo Arcano',
        slots: [{ level: 1, pact: false, free: 2 }],
      },
    } as never,
    ...over,
  } as never);

interface Opened {
  readonly component: unknown;
  readonly config: { role?: string; disableClose?: boolean; data: Record<string, unknown> };
}

function setup(opts: {
  master?: boolean;
  windows?: ReturnType<typeof reactionWindow>[];
  wait?: { titlePt: string; detailPt: string };
  current?: string;
}) {
  TestBed.resetTestingModule();
  const opened: Opened[] = [];
  const closes: Subject<unknown>[] = [];
  const calls: unknown[][] = [];
  const api = anyCallCombatClient((name) => {
    switch (name) {
      case 'log':
        return async () => ({ rounds: [], undoableEventId: '' });
      case 'turnOptions':
        return async () => ({
          options: create(TurnOptionsSchema, {
            attacks: [
              {
                attack: {
                  key: 'attack:shortbow',
                  name: 'Shortbow',
                  namePt: 'Arco curto',
                  kind: AttackKind.WEAPON,
                  melee: false,
                  saveDc: 0,
                  rangeFt: 80,
                },
              },
            ],
          }),
          attackTargets: [
            { attackKey: 'attack:shortbow', targets: [{ combatantId: 'g', label: 'Goblin' }] },
          ],
          spellTargets: [],
          pendingDamages: [],
        });
      default:
        return async (...args: unknown[]) => {
          calls.push([name, ...args]);
          return {};
        };
    }
  });
  const state = new CombatState();
  state.apply(
    encounter({
      mode: EncounterMode.THEATRE,
      mapId: '',
      gridColumns: 0,
      gridRows: 0,
      currentCombatantId: opts.current ?? 'g',
      combatants: [toren, goblin, mago],
      reactionWindows: opts.windows ?? [],
      reactionWait: opts.wait,
    } as never),
  );
  const dialog = {
    open: (component: unknown, config: Opened['config']) => {
      opened.push({ component, config });
      const closed = new Subject<unknown>();
      closes.push(closed);
      return { afterClosed: () => closed.asObservable() };
    },
  };
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
      { provide: MatDialog, useValue: dialog },
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
  return { fixture, el: fixture.nativeElement as HTMLElement, state, opened, closes, calls };
}

async function settle(fixture: {
  detectChanges: () => void;
  whenStable: () => Promise<unknown>;
}): Promise<void> {
  await fixture.whenStable();
  await nextTurn();
  await fixture.whenStable();
  fixture.detectChanges();
}

describe('CombatView, the reaction windows', () => {
  it('opens the reaction sheet by itself for the window the player answers, as an alert dialog that must be answered', async () => {
    const { fixture, opened } = setup({ windows: [shield()] });
    await settle(fixture);
    expect(opened).toHaveLength(1);
    expect(opened[0].component).toBe(ReactionSheet);
    expect(opened[0].config.role).toBe('alertdialog');
    expect(opened[0].config.disableClose).toBe(true);
    expect((opened[0].config.data['window'] as { id: string }).id).toBe('w1');
  });

  it('opens it once for a window, however many times the combat is read again', async () => {
    const { fixture, state, opened } = setup({ windows: [shield()] });
    await settle(fixture);
    state.apply(
      encounter({
        revision: 5,
        mode: EncounterMode.THEATRE,
        mapId: '',
        gridColumns: 0,
        gridRows: 0,
        currentCombatantId: 'g',
        combatants: [toren, goblin, mago],
        reactionWindows: [shield()],
      } as never),
    );
    await settle(fixture);
    expect(opened).toHaveLength(1);
  });

  it("opens nothing for a window that is not the player's to answer, or the master's check", async () => {
    const { fixture, opened } = setup({
      windows: [
        shield({ id: 'w2', forYou: false }),
        reactionWindow({ id: 'w3', kind: ReactionKind.MASTER_CHECK }),
      ],
    });
    await settle(fixture);
    expect(opened).toHaveLength(0);
  });

  it('the master gets no sheet: the cards of the windows stand below the turn instead', async () => {
    const { fixture, el, opened } = setup({
      master: true,
      current: 'g',
      windows: [
        reactionWindow({
          id: 'w9',
          kind: ReactionKind.SHIELD,
          reactorId: 'm1',
          reactorLabel: 'Mago 1',
          trigger: {
            actorId: 't',
            actorLabel: 'Toren',
            attackTotal: 15,
            armorClassWithShield: 17,
            slotEffects: [],
          } as never,
        }),
      ],
    });
    await settle(fixture);
    expect(opened.filter((o) => o.component === ReactionSheet)).toHaveLength(0);
    expect(plain(el.querySelector('app-reaction-queue')?.textContent)).toContain(
      'Esperando a sua reação: o Mago 1 pode conjurar Escudo Arcano',
    );
  });

  it('writes the wait the server wrote, off turn, in a status line', async () => {
    const { fixture, el } = setup({
      wait: { titlePt: 'Esperando o mestre', detailPt: 'O turno continua quando ele responder.' },
    });
    await settle(fixture);
    expect(plain(el.querySelector('[data-testid="reaction-wait"]')?.textContent)).toContain(
      'Esperando o mestre. O turno continua quando ele responder.',
    );
  });

  it('says why a window closed by itself in a status line, and "Entendi" takes it away', async () => {
    const { fixture, el, state } = setup({});
    await settle(fixture);
    state.noteReactionClosed('w1', 'Queda Suave fechou. Você já usou a sua reação.');
    await settle(fixture);
    const line = el.querySelector('[data-testid="reaction-closed"]');
    expect(plain(line?.textContent)).toContain('Queda Suave fechou. Você já usou a sua reação.');
    expect(line?.closest('[role="status"]')).not.toBeNull();
    [...el.querySelectorAll('button')].find((b) => plain(b.textContent) === 'Entendi')!.click();
    await settle(fixture);
    expect(el.querySelector('[data-testid="reaction-closed"]')).toBeNull();
  });

  // The second step of Defletir Projéteis: the monk caught the arrow and may throw it back for 1 ki.
  const throwWindow = () =>
    reactionWindow({
      id: 'w2',
      kind: ReactionKind.DEFLECT_MISSILES,
      reactorId: 't',
      reactorLabel: 'Toren',
      reactorIsPlayer: true,
      secondStep: true,
      prompt: {
        case: 'deflectThrow',
        value: {
          kiLeft: 3,
          normalRangeFt: 20,
          longRangeFt: 60,
          missileNamePt: 'Arco curto',
          attackBonus: 7,
        },
      } as never,
    } as never);

  it('"Devolver (1 de chi)" opens the ranged throw of the same reaction, naming the window, with the missile and not the monk\'s own attacks', async () => {
    const { fixture, opened, closes } = setup({ windows: [throwWindow()], current: 'g' });
    await settle(fixture);
    // The sheet closes asking to throw the missile back.
    closes[0].next({ throwWindowId: 'w2' });
    closes[0].complete();
    await settle(fixture);
    const attack = opened.find((o) => o.component === AttackSheet);
    expect(attack).toBeDefined();
    expect(attack?.config.data['catchWindowId']).toBe('w2');
    expect(attack?.config.data['asReaction']).toBe(true);
    const thrown = attack?.config.data['attack'] as {
      namePt: string;
      attackBonus: number;
      rangeFt: number;
      longRangeFt: number;
    };
    expect(thrown).toMatchObject({
      namePt: 'Arco curto',
      attackBonus: 7,
      rangeFt: 20,
      longRangeFt: 60,
    });
    expect(
      (attack?.config.data['targets'] as { combatantId: string }[]).map((t) => t.combatantId),
    ).toEqual(['g']);
  });

  it('closing the throw-back sheet without a roll keeps the missile: the window is passed, so the combat does not wait', async () => {
    const { fixture, opened, closes, calls } = setup({ windows: [throwWindow()], current: 'g' });
    await settle(fixture);
    closes[0].next({ throwWindowId: 'w2' });
    closes[0].complete();
    await settle(fixture);
    const at = opened.findIndex((o) => o.component === AttackSheet);
    closes[at].complete(); // "Fechar" with no roll
    await settle(fixture);
    const pass = calls.find((c) => c[0] === 'answerReaction');
    expect(pass?.[3]).toBe('w2');
    expect(pass?.[4]).toEqual({ use: false });
  });

  it("opens the next window's sheet once the first closed", async () => {
    const { fixture, state, opened, closes } = setup({ windows: [shield()] });
    await settle(fixture);
    closes[0].complete();
    state.apply(
      encounter({
        revision: 6,
        mode: EncounterMode.THEATRE,
        mapId: '',
        gridColumns: 0,
        gridRows: 0,
        currentCombatantId: 'g',
        combatants: [toren, goblin, mago],
        reactionWindows: [shield({ id: 'w5' })],
      } as never),
    );
    await settle(fixture);
    expect(opened.map((o) => (o.config.data['window'] as { id: string }).id)).toEqual(['w1', 'w5']);
  });
});
