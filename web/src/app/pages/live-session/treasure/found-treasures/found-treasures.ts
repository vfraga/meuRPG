import { Component, computed, inject, input } from '@angular/core';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatButtonModule } from '@angular/material/button';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';
import type { Observable } from 'rxjs';

import { type MapPoint, MapPointKind } from '../../../../../gen/meurpg/maps/v1/maps_pb';
import { joinDots } from '../../../../core/format/text';
import { finders } from '../../../../core/traps/treasure-text';
import { listNames } from '../../../../core/maps/scene-clues';
import { ChestIcon } from '../../../../shared/chest-icon/chest-icon';
import { TreasureFacts } from '../../../../shared/treasure-facts/treasure-facts';
import { SheetFrame } from '../../../../shared/sheet/sheet-frame/sheet-frame';
import { injectSheet, openSheet } from '../../../../shared/sheet/sheet-host';

/** The sheet of a treasure that was found: a bottom sheet on a phone, a dialog from a tablet up. */
@Component({
  selector: 'app-treasure-sheet',
  imports: [MatButtonModule, SheetFrame, TreasureFacts],
  template: `
    <app-sheet-frame [title]="point.name" titleId="treasure-t" [subtitle]="subtitle" [phone]="inSheet" (closed)="close()">
      <app-treasure-facts [point]="point" />
      <div foot class="foot">
        <button matButton="outlined" type="button" class="foot__btn" (click)="close()">Fechar</button>
      </div>
    </app-sheet-frame>
  `,
  styles: `
    .foot {
      display: flex;
    }

    .foot__btn {
      flex: 1 1 auto;
      min-height: 48px;
    }

    @media (min-width: 768px) {
      .foot {
        justify-content: flex-end;
      }

      .foot__btn {
        flex: none;
        min-width: 176px;
      }
    }
  `,
})
export class TreasureSheet {
  private readonly sheet = injectSheet<{ point: MapPoint; mapName: string }, void>();
  protected readonly point = this.sheet.data.point;
  protected readonly inSheet = this.sheet.inSheet;
  protected readonly subtitle = joinDots(
    ['Tesouro', this.sheet.data.mapName ? `em ${this.sheet.data.mapName}` : ''].filter(Boolean),
  );

  protected close(): void {
    this.sheet.close();
  }
}

export function openTreasureSheet(
  dialog: MatDialog,
  bottomSheet: MatBottomSheet,
  point: MapPoint,
  mapName: string,
): Observable<void | undefined> {
  return openSheet<TreasureSheet, { point: MapPoint; mapName: string }, void>(
    dialog,
    bottomSheet,
    TreasureSheet,
    {
      data: { point, mapName },
      ariaLabel: point.name,
      labelledBy: 'treasure-t',
    },
  );
}

/**
 * The treasures that were found, on the player's session page (E9-09 4, MR-041): one row each, "Baú de moedas ·
 * Tesouro, encontrado por Brisa", that opens the treasure's sheet with its value and what is inside. The
 * server sends a treasure to a player only once it is found, with its value and description; the marker is
 * on the map for everyone and this list is where the player reads it.
 */
@Component({
  selector: 'app-found-treasures',
  imports: [ChestIcon, MatIconModule],
  template: `
    @if (found().length > 0) {
      <section class="ft" aria-label="Tesouros encontrados">
        <ul class="mr-list">
          @for (p of found(); track p.id) {
            <li>
              <button type="button" class="mr-list__row ft__row" [attr.aria-label]="p.name + ', ' + sub(p) + '. Abrir'" (click)="open(p)">
                <app-chest-icon />
                <span class="mr-list__text">
                  <b class="mr-list__name">{{ p.name }}</b>
                  <span class="mr-list__sub">{{ sub(p) }}</span>
                </span>
                <mat-icon aria-hidden="true">chevron_right</mat-icon>
              </button>
            </li>
          }
        </ul>
      </section>
    }
  `,
  styles: `
    :host {
      display: contents;
    }

    .ft {
      margin-top: var(--mr-space-4);
      padding: 0 var(--mr-space-3);
      border: 1px solid var(--mr-line);
      border-radius: var(--mr-radius-md);
      background: var(--mr-surface);
    }

    .ft__row {
      width: 100%;
      padding: 0;
      border: 0;
      background: none;
      font: inherit;
      text-align: left;
      cursor: pointer;
    }

    .ft__row:focus-visible {
      outline: 2px solid var(--mr-focus);
      outline-offset: 2px;
    }
  `,
})
export class FoundTreasures {
  private readonly dialog = inject(MatDialog);
  private readonly bottomSheet = inject(MatBottomSheet);

  readonly points = input.required<readonly MapPoint[]>();
  readonly mapName = input('');

  protected readonly found = computed(() =>
    this.points().filter(
      (p) => p.kind === MapPointKind.TREASURE && p.treasureFoundAt !== undefined,
    ),
  );

  protected sub(p: MapPoint): string {
    const names = finders(p);
    return joinDots([
      'Tesouro',
      names.length > 0 ? `encontrado por ${listNames(names)}` : 'encontrado',
    ]);
  }

  protected open(p: MapPoint): void {
    openTreasureSheet(this.dialog, this.bottomSheet, p, this.mapName()).subscribe();
  }
}
