import {
  afterNextRender,
  ChangeDetectionStrategy,
  Component,
  computed,
  effect,
  ElementRef,
  inject,
  Injector,
  signal,
} from '@angular/core';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatDialog } from '@angular/material/dialog';
import type { Observable } from 'rxjs';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import { MapLayer } from '../../../../gen/meurpg/maps/v1/maps_pb';
import type { Square } from '../../../core/combat/combat-grid';
import type { DoorKind, DoorSquare } from '../../../core/maps/layers';
import { mapErrorMessage } from '../../../core/maps/map-errors';
import { MapsClient } from '../../../core/maps/maps-client';
import { DoorMark } from '../../../shared/map-layers/door-mark';
import { SheetFrame } from '../combat/sheet-frame/sheet-frame';
import { injectSheet, openSheet } from '../combat/sheet-host';

/** What the page hands the door sheet: the door and whether a wall is painted under it (a revealed secret door clears it, over the whole block of the drawing's square). */
export interface DoorSheetData {
  readonly campaignId: string;
  readonly mapId: string;
  readonly door: DoorSquare;
  /** The squares of the door's block that have a wall painted under them. */
  readonly wallSquares: readonly Square[];
}

const CHOICES: readonly {
  readonly state: DoorKind;
  readonly name: string;
  readonly what: string;
}[] = [
  { state: 1, name: 'Aberta', what: 'Dá para passar; não bloqueia a vista' },
  { state: 2, name: 'Fechada', what: 'Quem andar até ela a abre' },
  { state: 3, name: 'Trancada', what: 'Só você a destranca' },
  { state: 4, name: 'Grade', what: 'Dá para ver e passar a luz; ninguém passa' },
];

/** A grade offers itself and "Aberta"; any other door, the three it can be. Each sheet starts with the current state checked. */
function choicesFor(state: DoorKind): typeof CHOICES {
  return state === 4 ? [CHOICES[0], CHOICES[3]] : CHOICES.slice(0, 3);
}

/** The popover's width on a computer (the sheet never changes size between its stages: it keeps its top and left). */
const POPOVER_WIDTH = 340;

/**
 * Opens the door's sheet: a bottom sheet on a phone, and on a computer a popover beside the door with no dimmed backdrop, so the master
 * sees the door change on the map while he chooses. It is placed once, to the right of the door (to the left near the edge), and keeps
 * its top-left corner through the stages.
 */
export function openDoorSheet(
  dialog: MatDialog,
  bottomSheet: MatBottomSheet,
  data: DoorSheetData,
): Observable<boolean | undefined> {
  const phone = typeof matchMedia === 'function' && matchMedia('(max-width: 767.98px)').matches;
  const anchor = document.querySelector<HTMLElement>(
    `app-door-picks [data-door="${data.door.col},${data.door.row}"]`,
  );
  if (phone || !anchor) {
    return openSheet<DoorSheet, DoorSheetData, boolean>(dialog, bottomSheet, DoorSheet, {
      data,
      ariaLabel: 'Porta',
      labelledBy: 'door-t',
      width: '440px',
    });
  }
  const rect = anchor.getBoundingClientRect();
  const room = window.innerWidth - rect.right - 14;
  const left = Math.max(
    12,
    room >= POPOVER_WIDTH + 12 ? rect.right + 14 : rect.left - 14 - POPOVER_WIDTH,
  );
  const top = Math.max(72, Math.min(rect.top + rect.height / 2 - 130, window.innerHeight - 420));
  return dialog
    .open<DoorSheet, DoorSheetData, boolean>(DoorSheet, {
      data,
      width: `${POPOVER_WIDTH}px`,
      maxWidth: 'calc(100vw - 24px)',
      hasBackdrop: false,
      position: { top: `${top}px`, left: `${left}px` },
      ariaLabelledBy: 'door-t',
      autoFocus: 'first-heading',
      panelClass: 'mr-door-popover',
    })
    .afterClosed();
}

/**
 * "Porta" (E10-05 10 and 11, RN-26): the master taps a door of the session map and opens, closes or locks it. A choice is painted at
 * once (`PaintMapCells`, the doors layer), the same for everyone, and "Pronto" closes the sheet. A grade (a portcullis) offers only
 * "Aberta" and "Fechada". A secret door has one filled button, "Revelar a porta secreta", which asks in place ("Os jogadores vão ver a
 * porta. Revelar?") and paints it closed. A bottom sheet on a phone and a dialog from a tablet up (`openSheet`); the options are 48 px
 * high and the title is the first focus.
 */
@Component({
  selector: 'app-door-sheet',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [DoorMark, MatButtonModule, MatIconModule, SheetFrame],
  template: `
    <app-sheet-frame
      [title]="stage() === 'ask' ? 'Revelar a porta secreta?' : door.state === 5 ? 'Porta secreta' : 'Porta'"
      [subtitle]="place"
      titleId="door-t"
      [phone]="inSheet"
      (closed)="finish()"
    >
      @if (error()) {
        <div class="mr-notice mr-notice--danger" role="alert">
          <mat-icon aria-hidden="true">error</mat-icon>
          <p>{{ error() }}</p>
        </div>
      }
      @if (door.state === 5) {
        @if (stage() === 'ask') {
          <p class="what">Os jogadores vão ver a porta. Revelar?</p>
        } @else {
          <p class="what">Só você vê esta porta: os jogadores veem uma parede. Revelada, ela vira uma porta fechada para todos.</p>
        }
      } @else {
        <div class="opts" role="radiogroup" aria-label="Como a porta fica" (keydown)="onKey($event)">
          @for (c of choices(); track c.state) {
            <button
              type="button"
              role="radio"
              class="opt"
              [class.opt--on]="now() === c.state"
              [attr.aria-checked]="now() === c.state"
              [attr.aria-disabled]="busy() ? 'true' : null"
              [tabindex]="now() === c.state ? 0 : -1"
              (click)="choose(c.state)"
            >
              <span class="opt__mark" aria-hidden="true"><app-door-mark [state]="c.state" [axis]="door.axis" /></span>
              <span class="opt__text"><b>{{ c.name }}</b><span>{{ c.what }}</span></span>
              @if (now() === c.state) {
                <mat-icon class="opt__check" aria-hidden="true">check</mat-icon>
              }
            </button>
          }
        </div>
        @if (now() === 4) {
          <p class="small">Abrir a grade a troca por uma porta aberta.</p>
        }
        <p class="small" role="status">{{ saved() ? 'A mudança valeu na hora, para todos.' : 'A mudança vale na hora, para todos.' }}</p>
      }
      <div foot>
        @if (door.state === 5) {
          @if (stage() === 'ask') {
            <div class="pair">
              <button #back mat-stroked-button type="button" class="pair__btn" data-ask-back [disabled]="busy()" disabledInteractive (click)="show('sheet', '[data-reveal]')">Voltar</button>
              <button mat-flat-button type="button" class="pair__btn" [disabled]="busy()" disabledInteractive (click)="reveal()">Revelar</button>
            </div>
          } @else {
            <button mat-flat-button type="button" class="pair__btn pair__btn--one" data-reveal (click)="show('ask', '[data-ask-back]')">Revelar a porta secreta</button>
          }
        } @else {
          <button mat-stroked-button type="button" class="pair__btn pair__btn--one" (click)="finish()">Pronto</button>
        }
      </div>
    </app-sheet-frame>
  `,
  styleUrl: './door-sheet.scss',
})
/** The sheet answers `true` when it changed the door (the page may read the map again). */
export class DoorSheet {
  private readonly api = inject(MapsClient);
  private readonly sheet = injectSheet<DoorSheetData, boolean>();
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  protected readonly door = this.sheet.data.door;
  protected readonly inSheet = this.sheet.inSheet;
  protected readonly choices = computed(() => choicesFor(this.door.state));
  protected readonly place = `Coluna ${this.door.col + 1}, linha ${this.door.row + 1}`;

  private readonly injector = inject(Injector);
  protected readonly stage = signal<'sheet' | 'ask'>('sheet');
  /** The kind the door has now (the sheet stays open after a choice, showing it checked). */
  protected readonly now = signal<DoorKind>(this.door.state);
  protected readonly busy = signal(false);
  /** A request in the air: Esc and the backdrop do not close the sheet under it. */
  protected readonly lockWhileBusy = effect(() => this.sheet.lock(this.busy()));
  protected readonly error = signal('');
  protected readonly saved = signal(false);
  private changed = false;

  /** The arrow keys move through the choices (the radio pattern): the focus goes to the next radio and chooses it. */
  protected onKey(event: KeyboardEvent): void {
    const keys = ['ArrowLeft', 'ArrowUp', 'ArrowRight', 'ArrowDown'];
    if (!keys.includes(event.key)) {
      return;
    }
    event.preventDefault();
    const step = event.key === 'ArrowLeft' || event.key === 'ArrowUp' ? -1 : 1;
    const radios = Array.from(
      (event.currentTarget as HTMLElement).querySelectorAll<HTMLElement>('[role="radio"]'),
    );
    const here = Math.max(
      0,
      radios.findIndex((r) => r === document.activeElement),
    );
    const next = radios[(here + step + radios.length) % radios.length];
    next?.focus();
    next?.click();
  }

  /** Changes the stage of the sheet and puts the focus where the person goes on: "Voltar" of the question, or the button that asked. */
  protected show(stage: 'sheet' | 'ask', focus: string): void {
    this.stage.set(stage);
    afterNextRender(() => this.host.nativeElement.querySelector<HTMLElement>(focus)?.focus(), {
      injector: this.injector,
    });
  }

  /** The option the master tapped: the door is painted that way at once. A tap on the one checked is sent too: the
   * door may have changed since the sheet opened (a player opened it), and painting what is already there changes
   * nothing on the server. */
  protected async choose(state: DoorKind): Promise<void> {
    if (this.busy()) {
      return;
    }
    if (await this.paint([{ layer: MapLayer.DOORS, value: state }])) {
      this.now.set(state);
      this.saved.set(true);
    }
  }

  /** "Revelar": the secret door becomes a closed one (and a wall painted under it goes, or it would still be a wall). */
  protected async reveal(): Promise<void> {
    const { wallSquares } = this.sheet.data;
    const writes = [
      ...(wallSquares.length > 0 ? [{ layer: MapLayer.WALL, value: 0, squares: wallSquares }] : []),
      { layer: MapLayer.DOORS, value: 2 },
    ];
    if (await this.paint(writes)) {
      this.sheet.close(true);
    }
  }

  protected finish(): void {
    this.sheet.close(this.changed);
  }

  private async paint(
    writes: readonly { layer: MapLayer; value: number; squares?: readonly Square[] }[],
  ): Promise<boolean> {
    const { campaignId, mapId, door } = this.sheet.data;
    this.busy.set(true);
    this.error.set('');
    try {
      for (const w of writes) {
        await this.api.paint(
          campaignId,
          mapId,
          w.layer,
          w.value,
          w.squares ?? [{ col: door.col, row: door.row }],
        );
      }
      this.changed = true;
      return true;
    } catch (err) {
      this.error.set(mapErrorMessage(err, 'mudar a porta'));
      return false;
    } finally {
      this.busy.set(false);
    }
  }
}
