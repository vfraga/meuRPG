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
import { EffectPhase } from '../../../../gen/meurpg/play/v1/lasting_effects_pb';
import { TurnOptionsSchema } from '../../../../gen/meurpg/rules/v1/rules_pb';
import { TableRulesClient } from '../../../core/campaigns/table-rules';
import { CombatClient } from '../../../core/combat/combat-client';
import { CombatState } from '../../../core/combat/combat-state';
import { combatant, encounter, reactionWindow } from '../../../core/combat/combat-testing';
import { CreaturesClient } from '../../../core/creatures/creatures-client';
import { MapState } from '../../../core/maps/map-state';
import { MapsClient } from '../../../core/maps/maps-client';
import { RosterClient } from '../../../core/maps/roster-client';
import { SpellCatalog } from '../../../core/combat/spell-catalog';
import { EffectSaveSheet } from '../effects/effect-save-sheet/effect-save-sheet';
import { CombatView } from './combat-view';
import { ReactionSheet } from './reaction-sheet/reaction-sheet';
import { nextTurn } from '../../../../test-settle';

// The effects that last on the combat screen (RN-22): the saving throw of the end of the turn opens by itself and can be
// opened again, the waits are the server's words, and what the effects take from the turn is said and spent.

const plain = (t: string | null | undefined) => (t ?? '').replace(/\s+/g, ' ').trim();

const brisa = combatant({
  id: 'b',
  label: 'Brisa',
  kind: CombatantKind.PLAYER,
  side: CombatantSide.PARTY,
  characterId: 'brisa-c',
  mine: true,
  placed: false,
});
const goblin = combatant({ id: 'g', label: 'Goblin', placed: false });

const effectSave = (over: object = {}) =>
  reactionWindow({
    id: 'w1',
    kind: ReactionKind.EFFECT_SAVE,
    reactorId: 'b',
    reactorLabel: 'Brisa',
    reactorIsPlayer: true,
    prompt: {
      case: 'effectSave',
      value: {
        effectId: 'e1',
        sourceNamePt: 'Imobilizar Pessoa',
        ability: 'wis',
        abilityNamePt: 'Sabedoria',
        phase: EffectPhase.END,
        modifier: 1,
        bonusKnown: true,
        mode: 'normal',
        modeSourcesPt: [],
        extraDice: [],
        textPt: 'Teste de resistência de Sabedoria.',
      },
    } as never,
    ...over,
  } as never);

interface Opened {
  readonly component: unknown;
  readonly config: { role?: string; disableClose?: boolean; data: Record<string, unknown> };
}

function setup(opts: {
  windows?: ReturnType<typeof reactionWindow>[];
  wait?: { titlePt: string; detailPt: string };
  current?: string;
  options?: object;
}) {
  TestBed.resetTestingModule();
  const opened: Opened[] = [];
  const closes: Subject<unknown>[] = [];
  const takeAction = vi.fn(async () => ({ encounter: state.encounter() }));
  const api = new Proxy(
    {},
    {
      get: (_t, name: string) => {
        switch (name) {
          case 'log':
            return async () => ({ rounds: [], undoableEventId: '' });
          case 'turnOptions':
            return async () => ({
              options: create(TurnOptionsSchema, {}),
              attackTargets: [],
              spellTargets: [],
              pendingDamages: [],
              effectNotes: [],
              ...opts.options,
            });
          case 'takeAction':
            return takeAction;
          default:
            return async () => ({});
        }
      },
    },
  );
  const state = new CombatState();
  state.apply(
    encounter({
      mode: EncounterMode.THEATRE,
      mapId: '',
      gridColumns: 0,
      gridRows: 0,
      currentCombatantId: opts.current ?? 'b',
      combatants: [brisa, goblin],
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
  fixture.componentRef.setInput('isMaster', false);
  fixture.componentRef.setInput('state', state);
  fixture.componentRef.setInput('mapState', new MapState(async () => ({}) as never));
  fixture.componentRef.setInput('diceMode', DiceMode.PLAYERS_CHOOSE);
  fixture.componentRef.setInput('dicePreference', DicePreference.APP);
  fixture.detectChanges();
  return { fixture, el: fixture.nativeElement as HTMLElement, state, opened, closes, takeAction };
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

describe('CombatView, the effects that last (RN-22)', () => {
  it('opens the saving throw of the end of the turn by itself, in its own sheet (not the reaction alert), once', async () => {
    const { fixture, state, opened } = setup({ windows: [effectSave()] });
    await settle(fixture);
    expect(opened).toHaveLength(1);
    expect(opened[0].component).toBe(EffectSaveSheet);
    expect(opened[0].component).not.toBe(ReactionSheet);
    expect((opened[0].config.data['window'] as { id: string }).id).toBe('w1');
    state.apply(
      encounter({
        revision: 9,
        mode: EncounterMode.THEATRE,
        mapId: '',
        gridColumns: 0,
        gridRows: 0,
        currentCombatantId: 'b',
        combatants: [brisa, goblin],
        reactionWindows: [effectSave()],
      } as never),
    );
    await settle(fixture);
    expect(opened).toHaveLength(1);
  });

  it('keeps "Rolar o teste" for a sheet that was closed, and opens it again from there', async () => {
    const { fixture, el, opened, closes } = setup({ windows: [effectSave()] });
    await settle(fixture);
    expect(el.querySelector('[data-testid="effect-save-pending"]')).toBeNull();
    closes[0].complete();
    await settle(fixture);
    const notice = el.querySelector<HTMLElement>('[data-testid="effect-save-pending"]')!;
    expect(plain(notice.textContent)).toContain(
      'O teste de resistência do fim do turno espera a sua rolagem.',
    );
    Array.from(notice.querySelectorAll('button'))
      .find((b) => plain(b.textContent) === 'Rolar o teste')!
      .click();
    await settle(fixture);
    expect(opened).toHaveLength(2);
    expect(opened[1].component).toBe(EffectSaveSheet);
  });

  it('writes the wait the server wrote while the test of someone else is open', async () => {
    const { fixture, el } = setup({
      current: 'g',
      wait: { titlePt: 'Esperando o mestre', detailPt: 'O turno continua quando ele responder.' },
    });
    await settle(fixture);
    expect(plain(el.querySelector('[data-testid="reaction-wait"]')?.textContent)).toContain(
      'Esperando o mestre. O turno continua quando ele responder.',
    );
  });

  it('says what the effects take from the turn, in the server sentence', async () => {
    const { fixture, el } = setup({
      options: {
        effectNotes: [{ textPt: 'Você está Paralisada. Não age nem se move neste turno.' }],
      },
    });
    await settle(fixture);
    const note = el.querySelector('[data-testid="effect-note"]')!;
    expect(plain(note.textContent)).toBe(
      'blockVocê está Paralisada. Não age nem se move neste turno.',
    );
  });

  it('spends the extra action of Velocidade on a standard action, with the flag', async () => {
    const { fixture, el, takeAction } = setup({
      options: {
        extraAction: {
          available: true,
          labelPt: 'Ação extra (Velocidade)',
          allowedActions: ['dash'],
          allowedTextPt: 'Só: Disparada',
        },
      },
    });
    await settle(fixture);
    const group = el.querySelector('section[aria-labelledby="g-extra"]')!;
    expect(plain(group.textContent)).toContain('Só: Disparada');
    Array.from(group.querySelectorAll('button'))
      .find((b) => plain(b.textContent) === 'Disparada')!
      .click();
    await settle(fixture);
    expect(takeAction).toHaveBeenCalledWith(
      'c',
      'enc',
      'b',
      'standard:dash',
      undefined,
      undefined,
      true,
    );
  });
});
