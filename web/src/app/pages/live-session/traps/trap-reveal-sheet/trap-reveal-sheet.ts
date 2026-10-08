import { Component, computed, effect, inject, signal, viewChild } from '@angular/core';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';
import type { Observable } from 'rxjs';

import type { MapPoint } from '../../../../../gen/meurpg/maps/v1/maps_pb';
import { MapsClient } from '../../../../core/maps/maps-client';
import type { CluePlayer } from '../../../../core/maps/scene-clues';
import { trapMapErrorMessage } from '../../../../core/traps/trap-errors';
import { firstLine, knownReason } from '../../../../core/traps/trap-text';
import { joinDots, tight } from '../../../../core/format/text';
import { listNames } from '../../../../core/maps/scene-clues';
import { PairFoot } from '../../../../shared/pair-foot/pair-foot';
import { type PickRow, PersonPick } from '../../../../shared/person-pick/person-pick';
import { SheetFrame } from '../../combat/sheet-frame/sheet-frame';
import { injectSheet, openSheet } from '../../combat/sheet-host';

/** What the trap card hands "Revelar armadilha". */
export interface TrapRevealData {
  readonly campaignId: string;
  readonly mapId: string;
  readonly point: MapPoint;
  /** The campaign's player characters, with the player's name. */
  readonly players: readonly CluePlayer[];
}

/** "Revelar armadilha": a dialog from a tablet up and a bottom sheet on a phone; it answers the point as the
 * server has it once revealed, or `undefined`. */
export function openTrapReveal(
  dialog: MatDialog,
  bottomSheet: MatBottomSheet,
  data: TrapRevealData,
): Observable<MapPoint | undefined> {
  return openSheet<TrapRevealSheet, TrapRevealData, MapPoint>(
    dialog,
    bottomSheet,
    TrapRevealSheet,
    {
      data,
      ariaLabel: 'Revelar armadilha',
      labelledBy: 'trap-reveal-t',
      width: '520px',
      restoreFocus: false, // the opener takes it back with the focus ring
    },
  );
}

/** The button's words, by who is checked ("Revelar para Toren", "Revelar para 2 jogadores", "Revelar para
 * todos"; no article: the data has no gender). `open` is how many can still receive it, `players` how many
 * the campaign has: "todos" only when nobody had it before. */
export function trapRevealLabel(picked: readonly string[], open: number, players: number): string {
  if (picked.length === 0) {
    return 'Revelar a armadilha';
  }
  if (picked.length === 1) {
    return `Revelar para ${picked[0]}`;
  }
  if (picked.length === open) {
    return open === players ? 'Revelar para todos' : 'Revelar aos outros';
  }
  return `Revelar para ${picked.length} jogadores`;
}

/**
 * "Revelar armadilha" (E9-08 2, MR-035, question 59): who gets the trap is a question with one answer per
 * player, so **nobody starts checked**; the one filled button says who receives it, and with nobody checked
 * it is the app's dashed, disabled button and the sentence under the list says why. Whoever already knows
 * the trap shows checked and disabled with the reason ("já achou esta armadilha"). What the others see once
 * it is revealed (the area on the map, no DCs) is said in a note. "Marcar todos" is a text action.
 *
 * Focus: the title first, so a stray Enter reveals nothing; Esc or "Cancelar" hands it back to the card.
 */
@Component({
  selector: 'app-trap-reveal-sheet',
  imports: [MatIconModule, PairFoot, PersonPick, SheetFrame],
  template: `
    <app-sheet-frame title="Revelar armadilha" titleId="trap-reveal-t" [subtitle]="subtitle" [phone]="inSheet" (closed)="cancel()">
      @if (error()) {
        <div class="mr-notice mr-notice--danger" role="alert">
          <mat-icon aria-hidden="true">error</mat-icon>
          <p>{{ error() }}</p>
        </div>
      }
      <app-person-pick headingId="trap-reveal-who" heading="Quem recebe a armadilha" [rows]="rows()" [(picked)]="picked" />
      <p class="note" role="status" aria-live="polite">{{ summary() }}</p>
      <p class="note note--info">
        <mat-icon aria-hidden="true">info</mat-icon>
        <span>Quem receber vê a área da armadilha no mapa, sem as CDs. Quem não receber continua sem saber.</span>
      </p>
      <app-pair-foot
        foot
        [confirmLabel]="label()"
        [ready]="chosen().length > 0"
        [busy]="busy()"
        (cancel)="cancel()"
        (confirm)="confirm()"
      />
    </app-sheet-frame>
  `,
  styles: `
    .note {
      margin: var(--mr-space-3) 0 0;
      font-size: 15px;
      line-height: 20px;
      font-weight: 700;
    }

    .note--info {
      display: flex;
      align-items: flex-start;
      gap: 6px;
      font-size: 14px;
      line-height: 19px;
      font-weight: 400;
      color: var(--mr-ink-muted);
    }

    .note--info .mat-icon {
      flex: none;
      width: 18px;
      height: 18px;
      font-size: 18px;
    }
  `,
})
export class TrapRevealSheet {
  private readonly api = inject(MapsClient);
  private readonly sheet = injectSheet<TrapRevealData, MapPoint>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;
  private readonly frame = viewChild(SheetFrame);

  protected readonly subtitle = firstLine(this.data.point.description);
  private readonly knows = new Map(
    this.data.point.trapRevealedTo.map((r) => [r.characterId, r.how]),
  );
  protected readonly rows = computed<readonly PickRow[]>(() =>
    this.data.players.map((p) => {
      const how = this.knows.get(p.id);
      const sub =
        how !== undefined
          ? joinDots([p.playerName, knownReason(how)].filter(Boolean))
          : p.playerName;
      return { id: p.id, name: p.name, sub, locked: how !== undefined };
    }),
  );
  protected readonly picked = signal<ReadonlySet<string>>(new Set());
  private readonly open = computed(() => this.data.players.filter((p) => !this.knows.has(p.id)));
  protected readonly chosen = computed(() => this.open().filter((p) => this.picked().has(p.id)));
  protected readonly label = computed(() =>
    trapRevealLabel(
      this.chosen().map((p) => p.name),
      this.open().length,
      this.data.players.length,
    ),
  );
  protected readonly summary = computed(() => {
    const picked = this.chosen();
    if (picked.length === 0) {
      return 'Ninguém marcado. Escolha quem recebe a armadilha.';
    }
    if (picked.length === this.open().length && picked.length > 1) {
      return this.open().length === this.data.players.length
        ? `A armadilha vai para todos os ${this.data.players.length} jogadores.`
        : `A armadilha vai para os outros ${this.open().length} jogadores.`;
    }
    return tight(
      `A armadilha vai para ${picked.length} de ${this.open().length} ${this.open().length === 1 ? 'jogador' : 'jogadores'}: ${listNames(picked.map((p) => p.name))}.`,
    );
  });
  protected readonly busy = signal(false);
  /** A request in the air: Esc and the backdrop do not close the sheet under it. */
  protected readonly lockWhileBusy = effect(() => this.sheet.lock(this.busy()));
  protected readonly error = signal('');

  protected cancel(): void {
    this.sheet.close(undefined);
  }

  protected async confirm(): Promise<void> {
    if (this.chosen().length === 0 || this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const everyone = this.chosen().length === this.data.players.length;
      const point = await this.api.revealTrap(
        this.data.campaignId,
        this.data.mapId,
        this.data.point.id,
        everyone ? { all: true } : { characterIds: this.chosen().map((p) => p.id) },
      );
      this.sheet.close(point);
    } catch (err) {
      this.error.set(trapMapErrorMessage(err, 'revelar a armadilha'));
      this.frame()?.scrollToTop();
    } finally {
      this.busy.set(false);
    }
  }
}
