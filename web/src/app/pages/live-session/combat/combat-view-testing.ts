import { type Type } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatDialog } from '@angular/material/dialog';
import { provideRouter } from '@angular/router';
import { create } from '@bufbuild/protobuf';
import { Subject } from 'rxjs';
import { vi } from 'vitest';

import { DiceMode, DicePreference } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import type { Encounter } from '../../../../gen/meurpg/play/v1/combat_pb';
import { type GetTurnOptionsResponse } from '../../../../gen/meurpg/play/v1/combat_pb';
import { TurnOptionsSchema } from '../../../../gen/meurpg/rules/v1/rules_pb';
import { TableRulesClient } from '../../../core/campaigns/table-rules';
import { CombatClient } from '../../../core/combat/combat-client';
import { CombatState } from '../../../core/combat/combat-state';
import { SpellCatalog } from '../../../core/combat/spell-catalog';
import { CreaturesClient } from '../../../core/creatures/creatures-client';
import { MapState } from '../../../core/maps/map-state';
import { MapsClient } from '../../../core/maps/maps-client';
import { RosterClient } from '../../../core/maps/roster-client';
import { CombatView } from './combat-view';

/** A sheet or dialog the view opened: the component, the config `openSheet` gave (data, aria label, width...) and its result stream. */
export interface Opened {
  readonly component: Type<unknown>;
  readonly config: {
    readonly data: Record<string, unknown>;
    readonly ariaLabel?: string;
    readonly ariaLabelledBy?: string;
    readonly role?: string;
    readonly width?: string;
    readonly disableClose?: boolean;
    readonly autoFocus?: string;
  };
  /** Closes it with a result (`next` then `complete`), as the sheet's own "Fechar" or confirm would. */
  readonly close: (result?: unknown) => void;
}

/** One call to the combat client: its name and the arguments it got. */
export interface Call {
  readonly name: string;
  readonly args: unknown[];
}

export interface MountOptions {
  readonly enc: Encounter;
  readonly master?: boolean;
  /** `GetTurnOptions` answer for a combatant id (`undefined`: empty options). */
  readonly turnOptions?: (combatantId: string) => Partial<GetTurnOptionsResponse> | undefined;
  /** Replaces a client method: what it returns is what the call answers (a thrown error is a refusal). */
  readonly api?: Readonly<Record<string, (...args: unknown[]) => unknown>>;
  /** Inputs of the component besides `campaignId`, `isMaster`, `state` and `mapState`. */
  readonly inputs?: Readonly<Record<string, unknown>>;
  /** Replaces `CreaturesClient` methods. */
  readonly creatures?: Readonly<Record<string, (...args: unknown[]) => unknown>>;
  readonly tableRules?: unknown;
  readonly mapState?: MapState;
  /** The media queries that match (e.g. `PHONE_QUERY`, `'(min-width: 1024px)'`); a phone's sheets open as bottom sheets, with an `ariaLabel`. */
  readonly matches?: readonly string[];
}

/** The members of `CombatView` the specs reach into (protected or private in the class). */
// eslint-disable-next-line @typescript-eslint/no-explicit-any -- the specs call members the class keeps protected or private
export type ViewApi = any;

/**
 * Mounts the combat view over a fake `CombatClient` that records every call and answers `{}` (or what `api` says), with the
 * dialogs and bottom sheets replaced by recorders. For the specs that drive the component's own logic (the sheets it opens,
 * the calls it makes, the notes it keeps), not the screens inside it.
 */
export function mountCombatView(opts: MountOptions) {
  TestBed.resetTestingModule();
  const matching = opts.matches ?? [];
  // `vi.stubGlobal` is undone by the setup file after each test.
  vi.stubGlobal('matchMedia', (query: string) => ({
    matches: matching.includes(query),
    media: query,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
  }));
  const opened: Opened[] = [];
  const calls: Call[] = [];
  const state = new CombatState();
  state.apply(opts.enc);
  const emptyOptions = () => ({
    options: create(TurnOptionsSchema, {}),
    attackTargets: [],
    spellTargets: [],
    pendingDamages: [],
    resourceTargets: [],
  });
  const api = new Proxy(
    {},
    {
      get: (_t, name: string) => {
        const override = opts.api?.[name];
        if (override) {
          return async (...args: unknown[]) => {
            calls.push({ name, args });
            return override(...args);
          };
        }
        switch (name) {
          case 'log':
            return async () => ({ rounds: [], undoableEventId: '' });
          case 'turnOptions':
            return async (...args: unknown[]) => {
              calls.push({ name, args });
              return { ...emptyOptions(), ...opts.turnOptions?.(args[2] as string) };
            };
          case 'get':
            return async () => opts.enc;
          default:
            return async (...args: unknown[]) => {
              calls.push({ name, args });
              return { encounter: opts.enc };
            };
        }
      },
    },
  );
  const record = (component: Type<unknown>, config: Opened['config'], result: Subject<unknown>) =>
    opened.push({
      component,
      config,
      close: (value) => {
        if (value !== undefined) {
          result.next(value);
        }
        result.complete();
      },
    });
  const dialog = {
    open: (component: Type<unknown>, config: Opened['config']) => {
      const closed = new Subject<unknown>();
      record(component, config, closed);
      return { afterClosed: () => closed.asObservable() };
    },
  };
  const bottomSheet = {
    open: (component: Type<unknown>, config: Opened['config']) => {
      const closed = new Subject<unknown>();
      record(component, config, closed);
      return { afterDismissed: () => closed.asObservable() };
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
          summonOptions: async () => ({ spells: [] }),
          ...opts.creatures,
        },
      },
      {
        provide: TableRulesClient,
        useValue: opts.tableRules ?? { get: async () => ({ saved: {} }) },
      },
      { provide: MatDialog, useValue: dialog },
      { provide: MatBottomSheet, useValue: bottomSheet },
    ],
  });
  const fixture = TestBed.createComponent(CombatView);
  const mapState = opts.mapState ?? new MapState(async () => ({}) as never);
  fixture.componentRef.setInput('campaignId', 'c');
  fixture.componentRef.setInput('isMaster', opts.master ?? false);
  fixture.componentRef.setInput('state', state);
  fixture.componentRef.setInput('mapState', mapState);
  fixture.componentRef.setInput('diceMode', DiceMode.PLAYERS_CHOOSE);
  fixture.componentRef.setInput('dicePreference', DicePreference.APP);
  for (const [name, value] of Object.entries(opts.inputs ?? {})) {
    fixture.componentRef.setInput(name, value);
  }
  fixture.detectChanges();
  return {
    fixture,
    el: fixture.nativeElement as HTMLElement,
    view: fixture.componentInstance as unknown as ViewApi,
    state,
    mapState,
    opened,
    calls,
    callsOf: (name: string) => calls.filter((c) => c.name === name),
  };
}
