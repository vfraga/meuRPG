import {
  ChangeDetectionStrategy,
  Component,
  Injector,
  OnDestroy,
  type Signal,
  afterNextRender,
  computed,
  effect,
  inject,
  signal,
  untracked,
  viewChild,
  ElementRef,
} from '@angular/core';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatButtonModule } from '@angular/material/button';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';
import type { Observable } from 'rxjs';

import { TableScreenKind } from '../../../../gen/meurpg/play/v1/table_screen_pb';
import type { TableScreenPick } from '../../../core/play/table-screen-client';
import { clockText } from '../../../core/play/table-screen-labels';
import type { TableScreenPanel } from '../../../core/play/table-screen-panel';
import { COPIED_FOR_MS, copyText } from '../../../shared/session-link/session-link';
import { SheetFrame } from '../../../shared/sheet/sheet-frame/sheet-frame';
import { injectSheet, openSheet } from '../../../shared/sheet/sheet-host';
import { tableScreenLink } from './table-screen-link';
import { MIRROR_ORDER, type TvNames, type TvPinRow, nowLine, pinRows } from './table-screen-rows';
import { TvChoiceRow } from './tv-choice-row';

/** What the page hands the Tela da mesa panel. */
export interface TableScreenSheetData {
  readonly panel: TableScreenPanel;
  readonly campaignId: string;
  /** The names of the map, the image and the scene the session has now, for the rows' subtitles. */
  readonly names: Signal<TvNames>;
}

/** The Tela da mesa panel: a bottom sheet on a phone, a dialog from a tablet up. */
export function openTableScreenSheet(
  dialog: MatDialog,
  bottomSheet: MatBottomSheet,
  data: TableScreenSheetData,
): Observable<void | undefined> {
  return openSheet<TableScreenSheet, TableScreenSheetData, void>(
    dialog,
    bottomSheet,
    TableScreenSheet,
    { data, ariaLabel: 'Tela da mesa', labelledBy: 'tv-sheet-t', width: '400px' },
  );
}

type Badge = { readonly text: string; readonly tone: 'success' | 'pending' | 'neutral' };

const MIRROR_PICK: TableScreenPick = { mirror: true, pin: TableScreenKind.UNSPECIFIED };

/**
 * "Tela da mesa", the master's panel for the TV tab (#648): whether a TV is connected, what it shows now, "Espelhar o que eu mostro"
 * or one pinned item ("Fixar") as radio rows, the ⓘ that says how the mirror chooses, the privacy line, "Abrir a Tela da mesa"
 * and "Copiar o link" (the tab's address only). It reads the panel when it opens; a choice that did not go through says so and
 * leaves the TV as it was. The rows show only while a TV is connected: before that there is nothing to choose for.
 */
@Component({
  selector: 'app-table-screen-sheet',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [MatButtonModule, MatIconModule, SheetFrame, TvChoiceRow],
  templateUrl: './table-screen-sheet.html',
  styleUrl: './table-screen-sheet.scss',
})
export class TableScreenSheet implements OnDestroy {
  private readonly sheet = injectSheet<TableScreenSheetData, void>();
  private readonly injector = inject(Injector);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly data = this.sheet.data;
  protected readonly panel = this.data.panel;
  protected readonly inSheet = this.sheet.inSheet;
  protected readonly group = 'tv-choice';
  protected readonly mirrorOrder = MIRROR_ORDER;
  protected readonly link = tableScreenLink(this.data.campaignId);

  /** The first read is on its way. */
  protected readonly loading = signal(true);
  protected readonly helpOpen = signal(false);
  /** "Abrir a Tela da mesa" was used and no TV has connected yet. */
  protected readonly opening = signal(false);
  protected readonly copied = signal(false);
  /** The clipboard refused: the address is shown in a field, selected, to copy by hand. */
  protected readonly linkShown = signal(false);
  /** What a screen reader hears when a choice ends. */
  protected readonly announce = signal('');

  protected readonly open = computed(() => this.panel.presence().open);
  /** "21:14", the moment the last TV tab was lost; empty while none was. */
  protected readonly lostAt = computed(() => {
    const lostAt = this.panel.presence().lostAt;
    return this.panel.lost() && lostAt ? clockText(lostAt) : '';
  });
  protected readonly badge = computed<Badge>(() => {
    if (this.panel.lost()) {
      return { text: 'Sem conexão', tone: 'pending' };
    }
    if (this.open()) {
      return { text: 'Aberta', tone: 'success' };
    }
    return { text: this.opening() ? 'Abrindo…' : 'Fechada', tone: 'neutral' };
  });
  protected readonly rows = computed<TvPinRow[]>(() =>
    pinRows(this.panel.options(), this.data.names()),
  );
  protected readonly now = computed(() => {
    const data = this.panel.data();
    return data ? nowLine(data, !this.panel.mirror()) : null;
  });
  protected readonly notSaved = computed(() => this.panel.failed() || this.panel.stale());
  /** The first read failed and there is nothing to show. */
  protected readonly unread = computed(() => !this.loading() && this.panel.data() === null);

  private readonly field = viewChild<ElementRef<HTMLInputElement>>('linkField');
  /** The last choice, for "Tentar de novo"; none until the master picks. */
  private lastPick: TableScreenPick | null = null;
  private copiedTimer: ReturnType<typeof setTimeout> | null = null;

  /** A TV that connects ends the "Abrindo…". */
  protected readonly connected = effect(() => {
    if (this.open()) {
      untracked(() => this.opening.set(false));
    }
  });

  constructor() {
    void this.read();
  }

  ngOnDestroy(): void {
    if (this.copiedTimer !== null) {
      clearTimeout(this.copiedTimer);
    }
  }

  protected close(): void {
    this.sheet.close();
  }

  protected async read(): Promise<void> {
    this.loading.set(true);
    await this.panel.load();
    this.loading.set(false);
  }

  protected isPinned(kind: TableScreenKind): boolean {
    return !this.panel.mirror() && this.panel.pin() === kind;
  }

  protected pickMirror(): void {
    void this.pick(MIRROR_PICK);
  }

  protected pickPin(kind: TableScreenKind): void {
    void this.pick({ mirror: false, pin: kind });
  }

  protected retry(): void {
    if (this.lastPick) {
      void this.pick(this.lastPick);
    }
  }

  private async pick(pick: TableScreenPick): Promise<void> {
    this.lastPick = pick;
    this.announce.set('Mudando a Tela da mesa…');
    const result = await this.panel.choose(pick);
    // A refusal is the alert's to say, once: the region stays silent for it.
    this.announce.set(result === 'ok' ? 'A Tela da mesa mudou.' : '');
    if (result !== 'ok') {
      // The rows give way to the notice: the focus goes to "Tentar de novo", not to nothing.
      afterNextRender(
        () => this.host.nativeElement.querySelector<HTMLElement>('.js-retry')?.focus(),
        {
          injector: this.injector,
        },
      );
    }
  }

  protected async copyLink(): Promise<void> {
    if (await copyText(this.link)) {
      this.copied.set(true);
      this.linkShown.set(false);
      if (this.copiedTimer !== null) {
        clearTimeout(this.copiedTimer);
      }
      this.copiedTimer = setTimeout(() => {
        this.copiedTimer = null;
        this.copied.set(false);
      }, COPIED_FOR_MS);
      return;
    }
    this.linkShown.set(true);
    afterNextRender(
      () => {
        const field = this.field()?.nativeElement;
        field?.focus();
        field?.select();
      },
      { injector: this.injector },
    );
  }
}
