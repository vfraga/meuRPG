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
  untracked,
} from '@angular/core';
import { create } from '@bufbuild/protobuf';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import { EncounterSchema, type ReactionWindow } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { refreshIfStale } from '../../../../core/combat/combat-calls';
import { CombatClient } from '../../../../core/combat/combat-client';
import { combatErrorMessage } from '../../../../core/combat/combat-errors';
import type { CombatState } from '../../../../core/combat/combat-state';
import { ownCombatant } from '../../../../core/combat/combat-view';
import { dismountGoneText, dismountView } from '../../../../core/combat/reaction-dismount';
import { ActionKey } from '../../../../core/connect/idempotency';
import { EconomyMark } from '../../../../shared/economy-shapes/economy-shapes';
import { SheetFrame } from '../../../../shared/sheet/sheet-frame/sheet-frame';
import { injectSheet } from '../../../../shared/sheet/sheet-host';

/** What the page hands the prompt of a rider's dismount window. */
export interface DismountSheetData {
  readonly campaignId: string;
  readonly encounterId: string;
  readonly window: ReactionWindow;
  readonly state: CombatState;
}

/**
 * "O Cavalo de guerra caiu. Usar a reação para desmontar de pé?": the `alertdialog` of a rider whose mount
 * was knocked prone or dropped to 0 hit points, opened by itself for the rider's player and answered with `AnswerReaction`.
 * "Cair (Derrubado)" (PASS) comes first and "Desmontar de pé" (USE) is the filled one; the focus starts on the title, on neither. Unlike the optional
 * reactions it has no countdown: nothing passes it, because passing is a fall. The master may answer too; if the window leaves
 * the combat meanwhile the sheet says how it ended and only closes. The server decides every refusal.
 */
@Component({
  host: { class: 'mr-outline-ink' },
  selector: 'app-dismount-sheet',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [EconomyMark, MatButtonModule, MatIconModule, SheetFrame],
  template: `
    <app-sheet-frame
      icon="download"
      [title]="view.title"
      subtitleLead="Reação"
      [subtitle]="view.subtitleRest"
      [phone]="inSheet"
      [closable]="false"
      [focusableBody]="true"
      [tile]="true"
    >
      <app-economy-mark sub-mark kind="reaction" />
      <div class="body">
        <div class="mr-live" role="alert">
          @if (error()) {
            <div class="mr-notice mr-notice--danger">
              <mat-icon aria-hidden="true">error</mat-icon>
              <p>{{ error() }}</p>
            </div>
          }
        </div>
        <div class="mr-live" role="status">
          @if (gone()) {
            <p class="what">{{ goneText() }}</p>
          }
        </div>
        @if (!gone()) {
          <p class="what">
            <b>{{ view.lead }} {{ view.question }}</b>
          </p>
          <p class="small">{{ view.note }}</p>
          <div class="tags">
            @for (chip of view.chips; track chip; let first = $first) {
              <span class="tag" [class.tag--cost]="first">{{ chip }}</span>
            }
          </div>
          <p class="small wait"><mat-icon aria-hidden="true">hourglass_top</mat-icon>{{ view.wait }}</p>
        }
      </div>
      <div foot>
        @if (gone()) {
          <button matButton="outlined" type="button" class="foot__one" data-initial-focus (click)="sheet.close()">
            Fechar
          </button>
        } @else {
          <div class="pair">
            <button
              matButton="outlined"
              type="button"
              class="pair__btn"
              [disabled]="busy()"
              (click)="answer(false)"
            >
              <mat-icon aria-hidden="true">airline_seat_flat</mat-icon>{{ view.declineLabel }}
            </button>
            <button matButton="filled" type="button" class="pair__btn" [disabled]="busy()" (click)="answer(true)">
              <mat-icon aria-hidden="true">download</mat-icon>{{ view.useLabel }}
            </button>
          </div>
        }
      </div>
    </app-sheet-frame>
  `,
  styleUrls: [
    '../resource-pick/resource-sheet.scss',
    '../feature-window-sheet/feature-window-sheet.scss',
  ],
  // The two answers are side by side where each label keeps one line; under 400 px they stack, the filled one on top.
  styles: `
    .pair {
      grid-template-columns: 1fr 1fr;
    }
    .pair__btn {
      white-space: nowrap;
    }
    @media (max-width: 399.98px) {
      .pair {
        grid-template-columns: 1fr;
      }
      .pair__btn:last-child {
        order: -1;
      }
    }
    .tag--cost {
      border-color: var(--mr-accent);
      color: var(--mr-accent-text);
    }
    .wait {
      display: flex;
      align-items: flex-start;
      gap: var(--mr-space-1);
    }
    .wait .mat-icon {
      flex: none;
      width: 18px;
      height: 18px;
      font-size: var(--mr-font-18);
    }
  `,
})
export class DismountSheet {
  private readonly api = inject(CombatClient);
  private readonly injector = inject(Injector);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  protected readonly sheet = injectSheet<DismountSheetData, void>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;

  /** The words as the combat was when the prompt opened: the mount is named from the rider's row, which the answer clears. */
  protected readonly view = dismountView(
    this.data.state.encounter() ?? create(EncounterSchema),
    this.data.window,
  );

  protected readonly busy = signal(false);
  /** A request in the air: Esc and the backdrop do not close the sheet under it. */
  protected readonly lockWhileBusy = effect(() => this.sheet.lock(this.busy()));
  protected readonly error = signal('');
  private readonly key = new ActionKey();

  /** The window is no longer in the combat: the master answered for the player, or it closed by itself. */
  protected readonly gone = computed(
    () =>
      !this.busy() &&
      !(this.data.state.encounter()?.reactionWindows ?? []).some(
        (w) => w.id === this.data.window.id,
      ),
  );
  protected readonly goneText = computed(() => {
    const e = this.data.state.encounter();
    return dismountGoneText(e ? ownCombatant(e) : null);
  });
  /** The window going away takes the focus to "Fechar", so it does not fall to the page behind. */
  protected readonly focusClose = effect(() => {
    if (this.gone()) {
      untracked(() => afterNextRender(() => this.focusInitial(), { injector: this.injector }));
    }
  });

  constructor() {
    // The sheet opens by itself and neither answer is safe under a stray Enter (one spends the reaction, the other is a fall):
    // the focus starts on the title, and the two answers are the next stops.
    afterNextRender(() => this.host.nativeElement.querySelector<HTMLElement>('h2')?.focus(), {
      injector: this.injector,
    });
  }

  private focusInitial(): void {
    (
      this.host.nativeElement.querySelector('[data-initial-focus]') as HTMLButtonElement | null
    )?.focus({ focusVisible: true } as FocusOptions);
  }

  /** "Desmontar de pé" (USE) or "Cair (Derrubado)" (PASS): the answer to the window. */
  protected async answer(use: boolean): Promise<void> {
    if (this.busy()) {
      return;
    }
    const answer = { use };
    this.busy.set(true);
    this.error.set('');
    try {
      const res = await this.api.attacks.answerReaction(
        this.data.campaignId,
        this.data.encounterId,
        this.data.window.id,
        answer,
        this.key.keyFor({ window: this.data.window.id, answer }),
      );
      this.key.renew();
      this.data.state.apply(res.encounter);
      this.busy.set(false);
      this.sheet.close();
    } catch (err) {
      this.error.set(combatErrorMessage(err, 'responder à reação'));
      // The window may have been answered elsewhere: the combat read again says so.
      await refreshIfStale(err, this.data.state, this.api, this.data.campaignId);
      this.busy.set(false);
    }
  }
}
