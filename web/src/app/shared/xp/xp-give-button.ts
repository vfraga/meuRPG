import { Component, ElementRef, inject, input, output, viewChild } from '@angular/core';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatButtonModule } from '@angular/material/button';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';

import { XpMode } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import type { TreasureToConvert, XPAward } from '../../../gen/meurpg/progression/v1/progression_pb';
import type { ExperienceRow, LoadState } from '../../core/progression/experience-store';
import { openSheet } from '../sheet/sheet-host';
import {
  type AwardXpData,
  type AwardXpResult,
  AwardXpSheet,
  type TownRequest,
} from './award-xp-sheet';
import { type MilestoneData, MilestoneSheet } from './milestone-sheet';
import { type TownData, TownSheet } from './town-sheet';

/** What a master gave from the button: an XP award, or a milestone. */
export type GiveResult =
  | { readonly kind: 'xp'; readonly result: AwardXpResult }
  | { readonly kind: 'milestone'; readonly award: XPAward };

/** Opens "Dar XP" (a dialog on a desktop, a bottom sheet on a phone). It
 * answers the result, or `undefined` when the master cancelled. */
export function openAwardXp(
  dialog: MatDialog,
  bottomSheet: MatBottomSheet,
  data: AwardXpData,
  restoreFocus = true,
) {
  return openSheet<AwardXpSheet, AwardXpData, AwardXpResult | TownRequest | undefined>(
    dialog,
    bottomSheet,
    AwardXpSheet,
    {
      data,
      ariaLabel: 'Dar XP',
      labelledBy: 'sheet-t',
      width: '560px',
      focus: '[data-initial-focus]',
      restoreFocus,
    },
  );
}

/** Opens "Voltar à cidade" (E9-09): the treasures to convert, who receives, the
 * calculation. It answers the award, or `undefined` when the master cancelled. */
export function openTown(dialog: MatDialog, bottomSheet: MatBottomSheet, data: TownData) {
  return openSheet<TownSheet, TownData, AwardXpResult | undefined>(dialog, bottomSheet, TownSheet, {
    data,
    ariaLabel: 'Voltar à cidade',
    labelledBy: 'sheet-t',
    width: '600px',
    restoreFocus: false,
  });
}

/** Opens "Registrar marco". */
export function openMilestone(
  dialog: MatDialog,
  bottomSheet: MatBottomSheet,
  data: MilestoneData,
  restoreFocus = true,
) {
  return openSheet<MilestoneSheet, MilestoneData, XPAward | undefined>(
    dialog,
    bottomSheet,
    MilestoneSheet,
    {
      data,
      ariaLabel: 'Registrar marco',
      labelledBy: 'sheet-t',
      width: '560px',
      focus: '[data-initial-focus]',
      restoreFocus,
    },
  );
}

/**
 * The master's "Dar XP" button (outlined: the page's one filled button is
 * somewhere else), or "Registrar marco" in a campaign that levels by
 * milestones. In a campaign by gold, `showTown` adds "Voltar à cidade" beside
 * it (the campaign's panel); "Dar XP" has the same entry in its strip. It
 * opens the matching sheet and says what was given; the host (the campaign's
 * "Experiência" panel, the session's "Grupo") reads the XP again and shows the
 * confirmation. Nothing here for a campaign whose mode is not known yet.
 *
 * When a sheet closes the focus goes back to the button that opened it, with
 * the app's focus ring (code puts it there, so the browser would not draw one).
 * `block` makes every button the full width, 48 px high, on a phone.
 */
@Component({
  selector: 'app-xp-give-button',
  imports: [MatButtonModule, MatIconModule],
  host: { '[class.block]': 'blockOn()' },
  template: `
    @if (milestones()) {
      <button #mark mat-stroked-button type="button" class="give" [class.give--block]="blockOn()" (click)="openMark()">
        <mat-icon aria-hidden="true">flag</mat-icon>Registrar marco
      </button>
    } @else if (xpMode() !== Mode.UNSPECIFIED) {
      <button #give mat-stroked-button type="button" class="give" [class.give--block]="blockOn()" (click)="openXp()">Dar XP</button>
      @if (pair()) {
        <button #town mat-stroked-button type="button" class="give give--block" (click)="openTownSheet()">
          <mat-icon aria-hidden="true">currency_exchange</mat-icon>Voltar à cidade
        </button>
      }
    }
  `,
  styles: `
    // Two outlined buttons of one width side by side from a tablet up; stacked,
    // full width, on a phone (neither label fits half of it).
    :host {
      display: inline-flex;
      flex-direction: column;
      gap: var(--mr-space-2);

      @media (min-width: 768px) {
        flex-direction: row;
        gap: var(--mr-space-3);
      }
    }

    // On a phone: one under the other, the whole width, 48px high.
    :host(.block) {
      width: 100%;

      @media (min-width: 768px) {
        width: auto;
      }
    }

    .give--block {
      --mat-button-outlined-container-height: 48px;

      @media (max-width: 767.98px) {
        width: 100%;
      }

      @media (min-width: 768px) {
        --mat-button-outlined-container-height: 44px;
      }
    }

    .give {
      --mat-button-outlined-label-text-color: var(--mr-accent-text);
      white-space: nowrap;

      @media (min-width: 768px) {
        min-width: 176px;
      }
    }
  `,
})
export class XpGiveButton {
  private readonly dialog = inject(MatDialog);
  private readonly bottomSheet = inject(MatBottomSheet);

  readonly campaignId = input.required<string>();
  readonly campaignName = input('');
  readonly xpMode = input.required<XpMode>();
  readonly rows = input.required<readonly ExperienceRow[]>();
  /** The found treasures waiting to be converted (master, campaign by gold), and whether they are known. */
  readonly treasures = input<readonly TreasureToConvert[]>([]);
  readonly treasuresTotal = input(0);
  readonly treasuresState = input<LoadState>('ready');
  /** Draws "Voltar à cidade" beside "Dar XP". */
  readonly showTown = input(false);
  /** The buttons are the full width, 48 px high, on a phone (the campaign's panel). */
  readonly block = input(false);

  /** The master gave XP or marked a milestone. */
  readonly given = output<GiveResult>();

  private readonly giveButton = viewChild('give', { read: ElementRef<HTMLButtonElement> });
  private readonly townButton = viewChild('town', { read: ElementRef<HTMLButtonElement> });
  private readonly markButton = viewChild('mark', { read: ElementRef<HTMLButtonElement> });

  protected readonly Mode = XpMode;
  protected pair(): boolean {
    return this.showTown() && this.xpMode() === XpMode.GOLD;
  }
  protected blockOn(): boolean {
    return this.block() || this.pair();
  }
  protected milestones(): boolean {
    return this.xpMode() === XpMode.MILESTONES;
  }

  /** What the host knows of the treasures: the list once it was read, nothing before (the sheet says "lendo"). */
  private knownTreasures(): readonly TreasureToConvert[] | undefined {
    return this.treasuresState() === 'ready' ? this.treasures() : undefined;
  }

  /** Puts the focus back on the opener, with the focus ring, once the sheet's own restore is done. */
  private focusBack(button: ElementRef<HTMLButtonElement> | undefined): void {
    setTimeout(() => button?.nativeElement.focus({ focusVisible: true } as FocusOptions));
  }

  protected openXp(): void {
    const opener = this.giveButton();
    openAwardXp(
      this.dialog,
      this.bottomSheet,
      {
        campaignId: this.campaignId(),
        xpMode: this.xpMode(),
        rows: this.rows(),
        treasures: this.xpMode() === XpMode.GOLD ? this.knownTreasures() : undefined,
        treasuresTotal: this.treasuresTotal(),
      },
      false,
    ).subscribe((result) => {
      if (!result) {
        this.focusBack(opener);
        return;
      }
      if ('town' in result) {
        this.openTownSheet(opener);
      } else {
        this.given.emit({ kind: 'xp', result });
        this.focusBack(opener);
      }
    });
  }

  protected openTownSheet(
    back: ElementRef<HTMLButtonElement> | undefined = this.townButton(),
  ): void {
    openTown(this.dialog, this.bottomSheet, {
      campaignId: this.campaignId(),
      xpMode: this.xpMode(),
      rows: this.rows(),
      treasures: this.knownTreasures(),
      total: this.treasuresTotal(),
    }).subscribe((result) => {
      if (result) {
        this.given.emit({ kind: 'xp', result });
      }
      this.focusBack(back);
    });
  }

  protected openMark(): void {
    const opener = this.markButton();
    openMilestone(
      this.dialog,
      this.bottomSheet,
      {
        campaignId: this.campaignId(),
        campaignName: this.campaignName(),
        rows: this.rows(),
      },
      false,
    ).subscribe((award) => {
      if (award) {
        this.given.emit({ kind: 'milestone', award });
      }
      this.focusBack(opener);
    });
  }
}
