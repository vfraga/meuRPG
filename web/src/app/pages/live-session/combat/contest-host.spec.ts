import { Injector, runInInjectionContext, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatDialog } from '@angular/material/dialog';
import { create } from '@bufbuild/protobuf';
import { Subject } from 'rxjs';

import { DiceMode, DicePreference } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  type Combatant,
  CombatantKind,
  CombatantSide,
  type Encounter,
  type GetTurnOptionsResponse,
  GetTurnOptionsResponseSchema,
} from '../../../../gen/meurpg/play/v1/combat_pb';
import {
  CheckOptionSchema,
  ContestActionSchema,
  ContestAttackOptionKind,
  ContestAttackOptionSchema,
  ContestPurpose,
  ContestStatus,
  ContestTurnStateSchema,
  HideAttemptStatus,
} from '../../../../gen/meurpg/play/v1/contest_types_pb';
import { ActionEconomy } from '../../../../gen/meurpg/rules/v1/rules_pb';
import { CombatState } from '../../../core/combat/combat-state';
import { combatant, encounter } from '../../../core/combat/combat-testing';
import type { ContestClient } from '../../../core/combat/contest-client';
import {
  FakeContestClient,
  contestState,
  contestView,
  hideAttempt,
  skillOption,
} from '../../../core/combat/contest-testing';
import { ContestAnswerSheet } from './contest-answer-sheet/contest-answer-sheet';
import { ContestHost } from './contest-host';
import { ContestSheet } from './contest-sheet/contest-sheet';
import { HelpSheet } from './help-sheet/help-sheet';
import { HideSheet } from './hide-sheet/hide-sheet';
import { nextTurn } from '../../../../test-settle';

const toren = combatant({
  id: 't',
  label: 'Toren',
  kind: CombatantKind.PLAYER,
  side: CombatantSide.PARTY,
  mine: true,
});
const orla = combatant({
  id: 'o',
  label: 'Orla',
  kind: CombatantKind.PLAYER,
  side: CombatantSide.PARTY,
});
const hob = combatant({ id: 'h', label: 'Hobgoblin' });

interface Opened {
  readonly component: unknown;
  readonly data: Record<string, unknown>;
}

function setup(
  over: { master?: boolean; options?: GetTurnOptionsResponse | null; revision?: number } = {},
) {
  const api = new FakeContestClient();
  const opened: Opened[] = [];
  const closes: Subject<unknown>[] = [];
  const open = (component: unknown, config: { data: Record<string, unknown> }) => {
    opened.push({ component, data: config.data });
    const closed = new Subject<unknown>();
    closes.push(closed);
    return { afterClosed: () => closed.asObservable() };
  };
  TestBed.resetTestingModule();
  const enc = signal<Encounter | null>(
    encounter({ combatants: [toren, orla, hob], revision: over.revision ?? 1 }),
  );
  const options = signal<GetTurnOptionsResponse | null>(
    over.options ?? create(GetTurnOptionsResponseSchema),
  );
  const own = signal<Combatant | null>(toren);
  const master = signal(over.master ?? false);
  const state = new CombatState();
  const host = runInInjectionContext(TestBed.inject(Injector), () => {
    return new ContestHost({
      dialog: { open } as unknown as MatDialog,
      bottomSheet: {} as MatBottomSheet,
      api: api.as() as ContestClient,
      campaignId: () => 'c',
      isMaster: () => master(),
      encounter: enc,
      own,
      options,
      state: () => state,
      diceMode: () => DiceMode.PLAYERS_CHOOSE,
      preference: () => DicePreference.APP,
    });
  });
  const settle = async () => {
    TestBed.tick();
    await Promise.resolve();
    await nextTurn();
    TestBed.tick();
  };
  const read = async (response: ReturnType<typeof contestState>, revision: number) => {
    api.stateResponse = response;
    enc.set(encounter({ combatants: [toren, orla, hob], revision }));
    await settle();
  };
  return { api, host, opened, closes, options, settle, read, master, enc };
}

describe('ContestHost', () => {
  it('reads the contests of the combat each time it changes, and none for the master', async () => {
    const s = setup();
    await s.settle();
    expect(s.api.calls.filter((c) => c === 'state')).toHaveLength(1);
    await s.read(contestState(), 2);
    expect(s.api.calls.filter((c) => c === 'state')).toHaveLength(2);
    const m = setup({ master: true });
    await m.settle();
    expect(m.api.calls).toEqual([]);
  });

  it('opens the defender’s sheet once for a contest that waits for the player', async () => {
    const s = setup();
    await s.read(
      contestState({
        contests: [
          contestView({
            id: 'ct1',
            defenderId: 't',
            initiatorId: 'h',
            youAnswer: true,
            answerOptions: [skillOption()],
          }),
        ],
      }),
      2,
    );
    expect(s.opened.map((o) => o.component)).toEqual([ContestAnswerSheet]);
    expect(s.host.pending().map((c) => c.id)).toEqual(['ct1']);
    // Closed, read again: it does not open by itself a second time, but the way back is there.
    s.closes[0].next({ contestId: 'ct1' });
    s.closes[0].complete();
    await s.settle();
    expect(s.host.sheetOpen()).toBe(false);
    await s.read(s.api.stateResponse, 3);
    expect(s.opened).toHaveLength(1);
    s.host.answerPending();
    expect(s.opened).toHaveLength(2);
  });

  it('brings a hidden sheet back with the result once the contest is decided, once', async () => {
    const waiting = contestView({ id: 'ct2', initiatorId: 't', defenderId: 'h' });
    const s = setup({
      options: create(GetTurnOptionsResponseSchema, {
        contestAttackOptions: [
          create(ContestAttackOptionSchema, {
            kind: ContestAttackOptionKind.GRAPPLE,
            enabled: true,
          }),
        ],
      }),
    });
    await s.read(contestState({ contests: [waiting] }), 2);
    s.host.openAttack(ContestAttackOptionKind.GRAPPLE);
    expect(s.opened.map((o) => o.component)).toEqual([ContestSheet]);
    expect(s.opened[0].data['purpose']).toBe(ContestPurpose.GRAPPLE);
    // The player hides the sheet while the master has not answered.
    s.closes[0].next({ contestId: 'ct2' });
    s.closes[0].complete();
    await s.settle();
    expect(s.opened).toHaveLength(1);
    // The contest is decided: the sheet comes back at it, once.
    await s.read(
      contestState({ contests: [{ ...waiting, status: ContestStatus.RESOLVED } as never] }),
      3,
    );
    expect(s.opened.map((o) => o.component)).toEqual([ContestSheet, ContestSheet]);
    expect(s.opened[1].data['contestId']).toBe('ct2');
    s.closes[1].next({ contestId: 'ct2' });
    s.closes[1].complete();
    await s.read(s.api.stateResponse, 4);
    expect(s.opened).toHaveLength(2);
  });

  it('does not bring the sheet back when it was closed on the result', async () => {
    const resolved = contestView({
      id: 'ct3',
      initiatorId: 't',
      defenderId: 'h',
      status: ContestStatus.RESOLVED,
    });
    const s = setup({
      options: create(GetTurnOptionsResponseSchema, {
        contestAttackOptions: [
          create(ContestAttackOptionSchema, { kind: ContestAttackOptionKind.SHOVE, enabled: true }),
        ],
      }),
    });
    await s.read(contestState({ contests: [resolved] }), 2);
    s.host.openAttack(ContestAttackOptionKind.SHOVE);
    expect(s.opened[0].data['purpose']).toBe(ContestPurpose.SHOVE);
    s.closes[0].next({ contestId: 'ct3' });
    s.closes[0].complete();
    await s.read(s.api.stateResponse, 3);
    expect(s.opened).toHaveLength(1);
  });

  it('opens an escape with the skills the server sent and who holds the character', async () => {
    const s = setup({
      options: create(GetTurnOptionsResponseSchema, {
        contestState: create(ContestTurnStateSchema, {
          grappled: true,
          grapplerId: 'h',
          canEscape: true,
          escapeOptions: [skillOption({ modifier: 7 })],
        }),
      }),
    });
    s.host.openEscape();
    expect(s.opened[0].component).toBe(ContestSheet);
    expect(s.opened[0].data['purpose']).toBe(ContestPurpose.ESCAPE);
    expect(s.opened[0].data['holderId']).toBe('h');
    expect((s.opened[0].data['escape'] as unknown[]).length).toBe(1);
  });

  it('opens Hide with the action key and its economy, and brings back the answer of the master', async () => {
    const s = setup({
      options: create(GetTurnOptionsResponseSchema, {
        contestState: create(ContestTurnStateSchema, {
          hideActions: [
            create(ContestActionSchema, {
              key: 'feature:cunning-action:hide',
              economy: ActionEconomy.BONUS_ACTION,
              enabled: true,
            }),
          ],
          hideOption: create(CheckOptionSchema, { modifier: 7 }),
        }),
      }),
    });
    expect(s.host.hideKeys()).toEqual(['feature:cunning-action:hide']);
    s.host.openHide('feature:cunning-action:hide');
    expect(s.opened[0].component).toBe(HideSheet);
    expect(s.opened[0].data['economy']).toBe('Ação bônus');
    expect(s.opened[0].data['actionKey']).toBe('feature:cunning-action:hide');
    // Hidden while the master has not decided.
    await s.read(
      contestState({
        hideAttempts: [hideAttempt({ id: 'hd1', status: HideAttemptStatus.PENDING })],
      }),
      2,
    );
    s.closes[0].next({ attemptId: 'hd1' });
    s.closes[0].complete();
    await s.settle();
    await s.read(
      contestState({
        hideAttempts: [hideAttempt({ id: 'hd1', status: HideAttemptStatus.APPLIED })],
      }),
      3,
    );
    expect(s.opened.map((o) => o.component)).toEqual([HideSheet, HideSheet]);
    expect(s.opened[1].data['attemptId']).toBe('hd1');
  });

  it('opens Help with the allies of the character', () => {
    const s = setup();
    s.host.openHelp();
    expect(s.opened[0].component).toBe(HelpSheet);
    expect((s.opened[0].data['allies'] as { label: string }[]).map((a) => a.label)).toEqual([
      'Orla',
    ]);
  });

  it('opens nothing for the master', async () => {
    const s = setup({ master: true });
    await s.read(
      contestState({
        contests: [contestView({ defenderId: 't', initiatorId: 'h', youAnswer: true })],
      }),
      2,
    );
    expect(s.opened).toEqual([]);
    expect(s.host.pending()).toEqual([]);
  });
});
