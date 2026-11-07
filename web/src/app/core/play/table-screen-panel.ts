import { computed, signal } from '@angular/core';
import { timestampDate } from '@bufbuild/protobuf/wkt';

import {
  type GetTableScreenResponse,
  TableScreenKind,
} from '../../../gen/meurpg/play/v1/table_screen_pb';
import type { TableScreenChange } from './live-session.types';
import type { TableScreenClient, TableScreenPick } from './table-screen-client';
import { tableScreenErrorKind } from './table-screen-errors';
import {
  type TableScreenLabelState,
  barButtonLabel,
  menuLine,
  noticeText,
} from './table-screen-labels';
import { stageKindOf } from './table-screen-view';

/** How a choice ended: stored, refused because the choice moved meanwhile (the panel read again), or not stored. */
export type ChooseResult = 'ok' | 'stale' | 'failed';

/** Who is at the TV tab: whether one holds a stream, and since when none does. */
export interface TvPresence {
  readonly open: boolean;
  readonly lostAt: Date | null;
}

/**
 * The master's side of the table screen: the choice, the options, what the TV shows and whether a TV tab is
 * connected. It reads `GetTableScreen` (on load, after each choice, when the session stream says the choice
 * moved) and keeps the presence current from `table_screen_changed`. The session bar's button and the panel
 * both draw from it. A plain class: the session page builds it with its own client.
 */
export class TableScreenPanel {
  /** The last read; `null` until the first one (or after it failed with nothing to show). */
  readonly data = signal<GetTableScreenResponse | null>(null);
  readonly presence = signal<TvPresence>({ open: false, lostAt: null });
  /** A choice is on its way. */
  readonly saving = signal(false);
  /** The last choice was not stored ("Não deu para mudar a Tela da mesa. A TV continua como estava."). */
  readonly failed = signal(false);
  /** The last choice was refused because the choice had moved; the panel shows what is there now. */
  readonly stale = signal(false);

  /** A TV tab was connected and none is now. */
  readonly lost = computed(() => !this.presence().open && this.presence().lostAt !== null);
  readonly options = computed(() => this.data()?.options ?? []);
  readonly mirror = computed(() => this.data()?.choice?.mirror ?? true);
  readonly pin = computed(() => this.data()?.choice?.pin ?? TableScreenKind.UNSPECIFIED);
  /** The text of the stage's notice, empty without one. */
  readonly notice = computed(() => {
    const data = this.data();
    return data ? noticeText(data.notice, this.pin()) : '';
  });
  private readonly labelState = computed<TableScreenLabelState>(() => {
    const data = this.data();
    return {
      ...this.presence(),
      mirror: this.mirror(),
      pin: this.pin(),
      shown: data ? stageKindOf(data) : TableScreenKind.IDLE,
    };
  });
  /** The session bar's button. */
  readonly barLabel = computed(() => barButtonLabel(this.labelState()));
  /** The second line of the phone's menu. */
  readonly menuLine = computed(() => menuLine(this.labelState()));

  private generation = 0;

  constructor(
    private readonly client: Pick<TableScreenClient, 'get' | 'set'>,
    private readonly campaignId: () => string,
  ) {}

  /** Reads the screen again. A read that fails leaves what the panel held. */
  async load(): Promise<void> {
    const generation = ++this.generation;
    const campaignId = this.campaignId();
    if (campaignId === '') {
      return;
    }
    try {
      const res = await this.client.get(campaignId);
      if (generation === this.generation) {
        this.data.set(res);
        if (res.presence) {
          this.presence.set({
            open: res.presence.open,
            lostAt: res.presence.lostAt ? timestampDate(res.presence.lostAt) : null,
          });
        }
      }
    } catch {
      // The panel keeps what it showed; the next stream hint or the next open reads again.
    }
  }

  /** The master's choice: follow the table (`mirror`) or pin one kind. */
  async choose(pick: TableScreenPick): Promise<ChooseResult> {
    const current = this.data();
    const campaignId = this.campaignId();
    if (!current || campaignId === '') {
      return 'failed';
    }
    this.saving.set(true);
    this.failed.set(false);
    this.stale.set(false);
    try {
      await this.client.set(campaignId, current.choiceRevision, pick);
    } catch (err) {
      this.saving.set(false);
      // A choice that moved since it was read: show what is there now, and let the master choose again.
      if (tableScreenErrorKind(err) === 'stale') {
        this.stale.set(true);
        await this.load();
        return 'stale';
      }
      this.failed.set(true);
      return 'failed';
    }
    this.saving.set(false);
    await this.load();
    return 'ok';
  }

  /** `table_screen_changed`: a TV tab came or went, or the choice moved. */
  heard(change: TableScreenChange): void {
    if (change.presenceKnown) {
      this.presence.set({ open: change.open, lostAt: change.open ? null : change.lostAt });
    }
    const held = this.data()?.choiceRevision ?? 0;
    // The stage and the options move with the choice (and with the table): read them again.
    if (change.choiceRevision > held) {
      void this.load();
    }
  }
}
