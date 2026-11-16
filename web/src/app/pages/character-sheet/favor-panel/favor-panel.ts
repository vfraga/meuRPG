import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  Injector,
  afterNextRender,
  computed,
  effect,
  inject,
  input,
  signal,
  untracked,
} from '@angular/core';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatButtonModule } from '@angular/material/button';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';

import { FavorRecipients, type FavorRecipient } from '../../../core/play/favor-recipients';
import { FAVOR_WORK_LABEL, favorPassRow } from '../../../core/play/favor-pass';
import { FavorState } from '../../../core/play/favor-state';
import type { VitalsVm } from '../../../core/play/live-session.types';
import { openSheet } from '../../../shared/sheet/sheet-host';
import { type FavorPassData, FavorPassSheet } from './favor-pass-sheet';

/**
 * "Inspiração" on the character sheet (RN-43, board 581-04): drawn only while the character has the master's Inspiração, for the
 * master and for the owner (nobody else reads the sheet of a character, RN-10). The owner reads what it is for: "Do mestre.
 * Vantagem num d20 seu: marque “Usar a Inspiração” antes de rolar."; the master reads the title alone, since those words are
 * the player's.
 *
 * The owner who holds it gets "Passar a…", which opens "Passar a Inspiração" (the other player characters, then the master answers).
 * The page draws the panel only while the campaign has an open session, which is the only time the pass is asked, so the button never
 * offers what the server would refuse for lack of one; it is not drawn either while the character has no one to pass it to. While the
 * ask waits the button gives way to "Para Tor · esperando o mestre" and "Cancelar o pedido" (581-07, item 4): until the master answers
 * the Inspiração stays with the character and cannot be spent.
 *
 * The page gives it the live numbers (`vitals`): each `vitals_changed` hint is a new copy, and it is the only hint a grant, an ask, an
 * answer and a cancel tell, so the panel reads again when the copy changes. It keeps its own `FavorState`; a read that fails draws nothing.
 */
@Component({
  selector: 'app-favor-panel',
  imports: [MatButtonModule, MatIconModule],
  changeDetection: ChangeDetectionStrategy.OnPush,
  providers: [FavorState],
  template: `
    @if (favor.has(characterId())) {
      <section class="mr-panel favor" aria-labelledby="favor-heading">
        <div class="favor__head">
          <h2 class="favor__title" id="favor-heading">
            <mat-icon class="mr-icon-filled" aria-hidden="true">star</mat-icon>Inspiração
          </h2>
          @if (canPass()) {
            <button matButton="outlined" type="button" class="favor__pass" (click)="open()">
              <mat-icon aria-hidden="true">keyboard_tab</mat-icon>Passar a…
            </button>
          }
        </div>
        @if (viewer() === 'owner') {
          <p class="favor__text">
            Do mestre. Vantagem num d20 seu: marque “Usar a Inspiração” antes de rolar.
          </p>
          @if (ask(); as a) {
            <div class="favor__ask" data-testid="favor-ask">
              <mat-icon aria-hidden="true">keyboard_tab</mat-icon>
              <span class="favor__who"
                ><b>{{ row(a.toName).lead }}</b
                >{{ row(a.toName).rest }}</span
              >
              <button
                matButton="outlined"
                type="button"
                class="favor__cancel"
                [class.mr-button--off]="canceling()"
                [attr.aria-disabled]="canceling() ? 'true' : null"
                (click)="cancel()"
              >
                {{ canceling() ? working.cancel : 'Cancelar o pedido' }}
              </button>
            </div>
          }
          <div class="favor__live" role="alert">
            @if (failure()) {
              <div class="mr-notice mr-notice--danger">
                <mat-icon aria-hidden="true">error</mat-icon>
                <p>{{ failure() }}</p>
              </div>
            }
          </div>
        }
      </section>
    }
  `,
  styles: `
    :host {
      display: block;

      &:empty {
        display: none;
      }
    }

    .favor__head {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: var(--mr-space-3);
      flex-wrap: wrap;
    }

    .favor__title {
      display: flex;
      align-items: center;
      gap: var(--mr-space-2);
      margin: 0;
      font-family: var(--mr-font-display);
      font-size: var(--mr-font-21);
      line-height: 26px;
      font-weight: 700;
      color: var(--mr-ink);
    }

    .favor__pass,
    .favor__cancel {
      --mat-button-outlined-container-height: 44px;
      --mat-button-outlined-label-text-color: var(--mr-ink);
      --mat-button-outlined-outline-color: var(--mr-control-line);
      min-height: 44px;
    }

    .favor__text {
      margin: var(--mr-space-2) 0 0;
      font-size: var(--mr-font-14);
      line-height: 19px;
      color: var(--mr-ink-muted);
    }

    // The ask that waits: a bordered row, the receiver and the state in words, "Cancelar o pedido" beside them.
    .favor__ask {
      display: flex;
      align-items: center;
      flex-wrap: wrap;
      gap: var(--mr-space-3);
      margin-top: var(--mr-space-3);
      padding: var(--mr-space-3);
      border: 1px solid var(--mr-line);
      border-radius: var(--mr-radius-md);
      background: var(--mr-surface);

      .mat-icon {
        flex: none;
      }
    }

    .favor__who {
      flex: 1 1 8rem;
      min-width: 0;
      font-size: var(--mr-font-15);
      line-height: 20px;
    }

    .favor__live:empty {
      display: none;
    }

    .favor__live .mr-notice {
      margin-top: var(--mr-space-3);
    }
  `,
})
export class FavorPanel {
  readonly campaignId = input.required<string>();
  readonly characterId = input.required<string>();
  /** Who reads the sheet: the owner reads what to do with it. */
  readonly viewer = input<'owner' | 'master'>('owner');
  /** The character's live numbers: a new copy is a `vitals_changed` hint, and the cue to read again. */
  readonly vitals = input<VitalsVm | null>(null);

  protected readonly favor = inject(FavorState);
  private readonly recipients = inject(FavorRecipients);
  private readonly dialog = inject(MatDialog);
  private readonly bottomSheet = inject(MatBottomSheet);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly injector = inject(Injector);

  protected readonly working = FAVOR_WORK_LABEL;
  protected readonly row = favorPassRow;
  /** The other player characters the Inspiração may go to; none while the list has not come or the server gives none. */
  private readonly others = signal<readonly FavorRecipient[]>([]);
  /** What a refused cancel says. */
  protected readonly failure = signal('');
  /** The character's own ask that waits for the master. */
  protected readonly ask = computed(() => this.favor.passOf(this.characterId()));
  protected readonly canceling = computed(() => {
    const a = this.ask();
    return !!a && this.favor.workOf(a.id) === 'cancel';
  });
  /** The owner holds it, no ask waits, and there is someone to name: "Passar a…" is drawn. */
  protected readonly canPass = computed(
    () =>
      this.viewer() === 'owner' &&
      this.favor.campaignId() !== '' &&
      this.ask() === undefined &&
      this.others().length > 0,
  );
  private opened = false;

  constructor() {
    effect(() => {
      const id = this.campaignId();
      const master = this.viewer() === 'master';
      untracked(() => this.favor.follow(id, master));
    });
    effect(() => {
      this.vitals();
      untracked(() => void this.favor.refresh());
    });
    // The list of whom to pass it to is read when the owner has the Inspiração, never for the master, who answers asks and does not make them.
    effect(() => {
      const id = this.campaignId();
      const mine = this.characterId();
      if (this.viewer() === 'owner' && this.favor.has(mine)) {
        untracked(() => void this.loadOthers(id, mine));
      }
    });
  }

  protected open(): void {
    if (this.opened) {
      return;
    }
    this.opened = true;
    this.failure.set('');
    const data: FavorPassData = {
      favor: this.favor,
      fromId: this.characterId(),
      fromName: this.favor.holders().get(this.characterId()) ?? '',
      recipients: this.others(),
    };
    openSheet<FavorPassSheet, FavorPassData, boolean>(
      this.dialog,
      this.bottomSheet,
      FavorPassSheet,
      {
        data,
        ariaLabel: 'Passar a Inspiração',
        labelledBy: 'favor-pass-t',
        width: '460px',
      },
    ).subscribe({
      next: (asked) => {
        // The button gave way to the waiting row: the focus goes to its "Cancelar o pedido", not to nowhere.
        if (asked) {
          this.focusLater('.favor__cancel');
        }
      },
      complete: () => (this.opened = false),
    });
  }

  protected async cancel(): Promise<void> {
    const ask = this.ask();
    if (!ask || this.canceling()) {
      return;
    }
    this.failure.set('');
    const failure = await this.favor.cancelAsk(ask);
    if (failure === null) {
      this.focusLater('.favor__pass');
    } else {
      this.failure.set(failure);
    }
  }

  private focusLater(selector: string): void {
    afterNextRender(() => this.host.nativeElement.querySelector<HTMLElement>(selector)?.focus(), {
      injector: this.injector,
    });
  }

  private async loadOthers(campaignId: string, characterId: string): Promise<void> {
    try {
      this.others.set(await this.recipients.list(campaignId, characterId));
    } catch {
      // No list, no "Passar a…": there is no one to name.
      this.others.set([]);
    }
  }
}
