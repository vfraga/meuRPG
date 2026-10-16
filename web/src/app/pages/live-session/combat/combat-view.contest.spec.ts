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
import {
  CheckOptionSchema,
  ContestActionSchema,
  ContestAttackOptionKind,
  ContestAttackOptionSchema,
  ContestPurpose,
  ContestSkill,
  ContestStatus,
  ContestTargetSchema,
  ContestTurnStateSchema,
  RollModeKind,
} from '../../../../gen/meurpg/play/v1/contest_types_pb';
import {
  ActionEconomy,
  ActionOptionSchema,
  ActionSchema,
  AttackKind,
  TurnOptionsSchema,
  CreatureSize,
} from '../../../../gen/meurpg/rules/v1/rules_pb';
import { TableRulesClient } from '../../../core/campaigns/table-rules';
import { CombatClient } from '../../../core/combat/combat-client';
import { CombatState } from '../../../core/combat/combat-state';
import { combatant, encounter, reactionWindow } from '../../../core/combat/combat-testing';
import { ContestClient } from '../../../core/combat/contest-client';
import {
  FakeContestClient,
  contestState,
  contestView,
  skillOption,
  textOf,
} from '../../../core/combat/contest-testing';
import { SpellCatalog } from '../../../core/combat/spell-catalog';
import { CreaturesClient } from '../../../core/creatures/creatures-client';
import { MapState } from '../../../core/maps/map-state';
import { MapsClient } from '../../../core/maps/maps-client';
import { RosterClient } from '../../../core/maps/roster-client';
import { CombatView } from './combat-view';
import { ContestAnswerSheet } from './contest-answer-sheet/contest-answer-sheet';
import { ContestSheet } from './contest-sheet/contest-sheet';
import { HelpSheet } from './help-sheet/help-sheet';
import { HideSheet } from './hide-sheet/hide-sheet';
import { ReactionSheet } from './reaction-sheet/reaction-sheet';
import { nextTurn } from '../../../../test-settle';
import { anyCallCombatClient } from '../../../core/combat/combat-client-testing';

// The player's side of the contests on the combat screen (W7-X): the lines of the attack list, the sheets that open by
// themselves when a contest asks something of the player, Hide and Help from the standard actions, and the reaction cards that
// never see a window of kind CONTEST.

const toren = combatant({
  id: 't',
  label: 'Toren',
  kind: CombatantKind.PLAYER,
  side: CombatantSide.PARTY,
  characterId: 'toren-c',
  mine: true,
  placed: false,
  size: CreatureSize.MEDIUM,
} as never);
const hobgoblin = combatant({ id: 'h', label: 'Hobgoblin', placed: false });
const orla = combatant({
  id: 'o',
  label: 'Orla',
  kind: CombatantKind.PLAYER,
  side: CombatantSide.PARTY,
  characterId: 'orla-c',
  placed: false,
});

const std = (key: string, namePt: string) =>
  create(ActionOptionSchema, { enabled: true, action: create(ActionSchema, { key, namePt }) });

function turnOptions(contest: ReturnType<typeof create<typeof ContestTurnStateSchema>>) {
  return {
    options: create(TurnOptionsSchema, {
      attacks: [
        {
          enabled: true,
          attack: {
            key: 'attack:longsword',
            name: 'Longsword',
            namePt: 'Espada longa',
            kind: AttackKind.WEAPON,
            melee: true,
            saveDc: 0,
            rangeFt: 5,
          },
        },
      ],
      standardActions: [
        std('standard:hide', 'Esconder'),
        std('standard:help', 'Ajudar'),
        std('standard:dash', 'Disparada'),
      ],
    }),
    yourTurn: true,
    attackTargets: [
      { attackKey: 'attack:longsword', targets: [{ combatantId: 'h', label: 'Hobgoblin' }] },
    ],
    spellTargets: [],
    pendingDamages: [],
    contestAttackOptions: [
      create(ContestAttackOptionSchema, {
        kind: ContestAttackOptionKind.GRAPPLE,
        replacesAttack: true,
        enabled: true,
        targets: [
          create(ContestTargetSchema, {
            combatantId: 'h',
            size: CreatureSize.MEDIUM,
            distanceFt: 5,
            eligible: true,
          }),
        ],
        rollOption: create(CheckOptionSchema, { modifier: 5, known: true }),
      }),
    ],
    contestState: contest,
  };
}

interface Opened {
  readonly component: unknown;
  readonly config: { role?: string; data: Record<string, unknown> };
}

function setup(opts: {
  master?: boolean;
  windows?: ReturnType<typeof reactionWindow>[];
  contest?: ReturnType<typeof create<typeof ContestTurnStateSchema>>;
  contests?: ReturnType<typeof contestState>;
  current?: string;
}) {
  TestBed.resetTestingModule();
  const opened: Opened[] = [];
  const closes: Subject<unknown>[] = [];
  const taken: string[] = [];
  const contestApi = new FakeContestClient();
  contestApi.stateResponse = opts.contests ?? contestState();
  const options = turnOptions(
    opts.contest ??
      create(ContestTurnStateSchema, {
        hideActions: [
          create(ContestActionSchema, {
            key: 'standard:hide',
            economy: ActionEconomy.ACTION,
            enabled: true,
          }),
        ],
        hideOption: create(CheckOptionSchema, {
          modifier: 7,
          known: true,
          mode: RollModeKind.NORMAL,
        }),
      }),
  );
  const api = anyCallCombatClient((name) => {
    switch (name) {
      case 'log':
        return async () => ({ rounds: [], undoableEventId: '' });
      case 'turnOptions':
        return async () => options;
      case 'takeAction':
        return async (_c: string, _e: string, _id: string, key: string) => {
          taken.push(key);
          return { encounter: encounter() };
        };
      default:
        return async () => ({});
    }
  });
  const state = new CombatState();
  state.apply(
    encounter({
      mode: EncounterMode.THEATRE,
      mapId: '',
      gridColumns: 0,
      gridRows: 0,
      currentCombatantId: opts.current ?? 't',
      combatants: [toren, hobgoblin, orla],
      reactionWindows: opts.windows ?? [],
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
      { provide: ContestClient, useValue: contestApi.as() },
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
  return {
    fixture,
    el: fixture.nativeElement as HTMLElement,
    state,
    opened,
    closes,
    taken,
    contestApi,
  };
}

async function settle(fixture: {
  detectChanges: () => void;
  whenStable: () => Promise<unknown>;
}): Promise<void> {
  await fixture.whenStable();
  await nextTurn();
  await fixture.whenStable();
  fixture.detectChanges();
  await nextTurn();
  fixture.detectChanges();
}

const contestWindow = (over: object = {}) =>
  reactionWindow({
    id: 'wc',
    kind: ReactionKind.CONTEST,
    reactorId: 't',
    reactorLabel: 'Toren',
    reactorIsPlayer: true,
    prompt: { case: 'contest', value: { contestId: 'ct1' } } as never,
    ...over,
  } as never);

const defending = (over: object = {}) =>
  contestView({
    initiatorId: 'h',
    defenderId: 't',
    youAnswer: true,
    answerOptions: [skillOption({ skill: ContestSkill.ATHLETICS, modifier: 2 })],
    ...over,
  });

describe('CombatView, the contests of the player (W7-X)', () => {
  it('lists Agarrar under the attacks and opens its sheet with the options the server sent', async () => {
    const { fixture, el, opened } = setup({});
    await settle(fixture);
    const rows = Array.from(el.querySelectorAll('app-action-row'));
    const grapple = rows.find(
      (r) => r.querySelector('.row__name')?.textContent?.trim() === 'Agarrar',
    )!;
    expect(grapple).toBeTruthy();
    grapple.querySelector<HTMLButtonElement>('button')!.click();
    await settle(fixture);
    expect(opened).toHaveLength(1);
    expect(opened[0].component).toBe(ContestSheet);
    expect(opened[0].config.data['purpose']).toBe(ContestPurpose.GRAPPLE);
    expect(opened[0].config.data['initiatorId']).toBe('t');
    expect((opened[0].config.data['attack'] as { targets: unknown[] }).targets).toHaveLength(1);
  });

  it('opens the defender’s sheet, not a reaction sheet, for a CONTEST window the player answers, once', async () => {
    const { fixture, opened, state } = setup({
      current: 'h',
      windows: [contestWindow()],
      contests: contestState({ contests: [defending()] }),
    });
    await settle(fixture);
    expect(opened.map((o) => o.component)).toEqual([ContestAnswerSheet]);
    expect(opened[0].config.data['contestId']).toBe('ct1');
    // Read again: still one.
    state.apply(
      encounter({
        revision: 9,
        mode: EncounterMode.THEATRE,
        mapId: '',
        gridColumns: 0,
        gridRows: 0,
        currentCombatantId: 'h',
        combatants: [toren, hobgoblin, orla],
        reactionWindows: [contestWindow()],
      } as never),
    );
    await settle(fixture);
    expect(opened.filter((o) => o.component === ReactionSheet)).toHaveLength(0);
    expect(opened.filter((o) => o.component === ContestAnswerSheet)).toHaveLength(1);
  });

  it('keeps a way back to the question when the sheet is closed without answering', async () => {
    const { fixture, el, opened, closes } = setup({
      current: 'h',
      windows: [contestWindow()],
      contests: contestState({ contests: [defending()] }),
    });
    await settle(fixture);
    expect(el.querySelector('[data-testid="contest-pending"]')).toBeNull();
    closes[0].next({ contestId: 'ct1' });
    closes[0].complete();
    await settle(fixture);
    expect(textOf(el.querySelector('[data-testid="contest-pending"]')!)).toBe(
      'Uma disputa espera a sua resposta. Responder',
    );
    el.querySelector<HTMLButtonElement>('[data-testid="contest-pending"] button')!.click();
    await settle(fixture);
    expect(opened.filter((o) => o.component === ContestAnswerSheet)).toHaveLength(2);
  });

  it('opens the shove’s choice for the one that won it', async () => {
    const { fixture, opened } = setup({
      contests: contestState({
        contests: [
          contestView({
            purpose: ContestPurpose.SHOVE,
            status: ContestStatus.AWAITING_OUTCOME,
            initiatorId: 't',
            defenderId: 'h',
            youChoose: true,
          }),
        ],
      }),
    });
    await settle(fixture);
    expect(opened.map((o) => o.component)).toEqual([ContestSheet]);
    expect(opened[0].config.data['contestId']).toBe('ct1');
    expect(opened[0].config.data['purpose']).toBe(ContestPurpose.SHOVE);
  });

  it('opens Hide, not a plain action, from "Esconder", and Help from "Ajudar"', async () => {
    const { fixture, el, opened, taken } = setup({});
    await settle(fixture);
    const press = (name: string) =>
      Array.from(el.querySelectorAll<HTMLButtonElement>('.std__btn'))
        .find((b) => b.textContent?.trim() === name)!
        .click();
    press('Esconder');
    await settle(fixture);
    expect(opened.map((o) => o.component)).toEqual([HideSheet]);
    expect(opened[0].config.data['actionKey']).toBe('standard:hide');
    expect(opened[0].config.data['economy']).toBe('Ação');
    expect(taken).toEqual([]);
  });

  it('opens Help with the allies and no plain action', async () => {
    const { fixture, el, opened, taken } = setup({});
    await settle(fixture);
    Array.from(el.querySelectorAll<HTMLButtonElement>('.std__btn'))
      .find((b) => b.textContent?.trim() === 'Ajudar')!
      .click();
    await settle(fixture);
    expect(opened.map((o) => o.component)).toEqual([HelpSheet]);
    expect((opened[0].config.data['allies'] as { label: string }[]).map((a) => a.label)).toEqual([
      'Orla',
    ]);
    expect(taken).toEqual([]);
  });

  it('takes the other standard actions as before', async () => {
    const { fixture, el, opened, taken } = setup({});
    await settle(fixture);
    Array.from(el.querySelectorAll<HTMLButtonElement>('.std__btn'))
      .find((b) => b.textContent?.trim() === 'Disparada')!
      .click();
    await settle(fixture);
    expect(taken).toEqual(['standard:dash']);
    expect(opened).toHaveLength(0);
  });

  it('says "Escondida" on the turn and the notes of a surprised character', async () => {
    const hidden = setup({ contest: create(ContestTurnStateSchema, { hidden: true }) });
    await settle(hidden.fixture);
    expect(textOf(hidden.el)).toContain('Escondido Você está escondido.');
    const surprised = setup({ contest: create(ContestTurnStateSchema, { surprised: true }) });
    await settle(surprised.fixture);
    expect(textOf(surprised.el)).toContain('Surpresa Você está surpreso neste turno.');
    expect(textOf(surprised.el)).toContain('Passar o turno');
    expect(textOf(surprised.el)).toContain('Reação Indisponível Surpresa até o fim do turno.');
  });

  it('the master’s screen answers a contest in a card of his own and opens no player sheet', async () => {
    const { fixture, el, opened, contestApi } = setup({
      master: true,
      current: 'h',
      windows: [contestWindow({ reactorId: '', forYou: true })],
      contests: contestState({ contests: [defending()] }),
    });
    await settle(fixture);
    // The master reads the contests of the combat for his own cards (W7-X), never for the player's sheets.
    expect(contestApi.calls).toContain('state');
    expect(el.querySelector('app-contest-master app-contest-card')).not.toBeNull();
    expect(
      opened.filter((o) => o.component === ContestAnswerSheet || o.component === ContestSheet),
    ).toHaveLength(0);
  });
});
