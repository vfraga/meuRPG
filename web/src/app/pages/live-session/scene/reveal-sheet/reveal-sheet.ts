import { NgTemplateOutlet } from '@angular/common';
import {
  Component,
  DestroyRef,
  ElementRef,
  Injector,
  afterNextRender,
  computed,
  effect,
  inject,
  signal,
  viewChild,
} from '@angular/core';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatButtonModule } from '@angular/material/button';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';
import type { Observable } from 'rxjs';

import type { SceneClue } from '../../../../../gen/meurpg/maps/v1/maps_pb';
import { revealErrorMessage } from '../../../../core/maps/map-errors';
import { MapsClient } from '../../../../core/maps/maps-client';
import {
  type CluePlayer,
  playersWithout,
  revealLabel,
  revealSummary,
} from '../../../../core/maps/scene-clues';
import { SheetFrame } from '../../combat/sheet-frame/sheet-frame';
import { injectSheet, openSheet } from '../../combat/sheet-host';

/** What the open scene hands "Revelar pista". */
export interface RevealSheetData {
  readonly campaignId: string;
  readonly clue: SceneClue;
  /** The clue's place in the scene's list (1-based) and the list's size. */
  readonly number: number;
  readonly total: number;
  readonly sceneName: string;
  /** The campaign's player characters, with the player's name. */
  readonly players: readonly CluePlayer[];
}

/** "Revelar pista": a dialog from a tablet up and a bottom sheet on a phone;
 * it answers the clue as the server has it once revealed, or `undefined`. */
export function openRevealSheet(
  dialog: MatDialog,
  bottomSheet: MatBottomSheet,
  data: RevealSheetData,
): Observable<SceneClue | undefined> {
  return openSheet<RevealSheet, RevealSheetData, SceneClue>(dialog, bottomSheet, RevealSheet, {
    data,
    ariaLabel: 'Revelar pista',
    labelledBy: 'reveal-t',
    width: '520px',
  });
}

/**
 * "Revelar pista" (E8-05, MR-029, question 59): who gets the clue is a
 * question with one answer per player, so **nobody starts checked**; the
 * master picks, and the one filled button says who receives it ("Revelar para
 * Brisa", "Revelar para 2 jogadores", "Revelar para todos"; no article, the
 * data has no gender). With nobody checked it is the app's dashed disabled
 * button ("Revelar a pista", ⊘, `aria-disabled`) and the sentence above says
 * why, so the reason is never only a colour. "Marcar todos" is a text action
 * for the day the clue is for the whole table. Someone who already has the
 * clue shows checked and disabled ("Já tem a pista"). A revealing is not
 * undone, and the app has no button to share it: the table does that.
 *
 * The pair at the bottom is two equal buttons, and stacks (both full width)
 * whenever the label does not fit half the width.
 *
 * Focus: the title first, so a stray Enter reveals nothing; Esc or "Cancelar"
 * hands it back to the clue's "Revelar" button.
 */
@Component({
  selector: 'app-reveal-sheet',
  imports: [MatButtonModule, MatIconModule, NgTemplateOutlet, SheetFrame],
  templateUrl: './reveal-sheet.html',
  styleUrl: './reveal-sheet.scss',
})
export class RevealSheet {
  private readonly api = inject(MapsClient);
  private readonly sheet = injectSheet<RevealSheetData, SceneClue>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;

  protected readonly subtitle = `Pista ${this.data.number} de ${this.data.total} · ${this.data.sceneName}`;
  /** Those who can still receive it, and the ones who already have it. */
  protected readonly open = computed(() => playersWithout(this.data.clue, this.data.players));
  protected readonly has = computed(() => {
    const open = new Set(this.open().map((p) => p.id));
    return this.data.players.filter((p) => !open.has(p.id));
  });
  protected readonly picked = signal<ReadonlySet<string>>(new Set());
  protected readonly chosen = computed(() => this.open().filter((p) => this.picked().has(p.id)));
  protected readonly allPicked = computed(
    () => this.open().length > 0 && this.chosen().length === this.open().length,
  );
  protected readonly label = computed(() =>
    revealLabel(this.chosen(), this.open().length, this.data.players.length),
  );
  protected readonly summary = computed(() =>
    revealSummary(this.chosen(), this.open().length, this.data.players.length),
  );
  /** The pair stacks, both full width, when a label does not fit half the width. */
  protected readonly stacked = signal(false);
  protected readonly busy = signal(false);
  /** A request in the air: Esc and the backdrop do not close the sheet under it. */
  protected readonly lockWhileBusy = effect(() => this.sheet.lock(this.busy()));
  protected readonly error = signal('');

  private readonly frame = viewChild(SheetFrame);
  private readonly foot = viewChild<ElementRef<HTMLElement>>('foot');
  private readonly injector = inject(Injector);

  constructor() {
    const destroyRef = inject(DestroyRef);
    afterNextRender(() => {
      const foot = this.foot()?.nativeElement;
      if (!foot) {
        return;
      }
      const check = () => this.stacked.set(needsStack(foot));
      check();
      const observer = typeof ResizeObserver === 'function' ? new ResizeObserver(check) : null;
      observer?.observe(foot);
      destroyRef.onDestroy(() => observer?.disconnect());
      // A new label (someone checked or unchecked) may fit or not.
      effect(
        () => {
          this.label();
          afterNextRender(check, { injector: this.injector });
        },
        { injector: this.injector },
      );
    });
  }

  protected toggle(id: string): void {
    this.picked.update((set) => {
      const next = new Set(set);
      if (!next.delete(id)) {
        next.add(id);
      }
      return next;
    });
  }

  protected toggleAll(): void {
    this.picked.set(this.allPicked() ? new Set() : new Set(this.open().map((p) => p.id)));
  }

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
      const clue = await this.api.revealSceneClue(
        this.data.campaignId,
        this.data.clue.id,
        this.chosen().map((p) => p.id),
      );
      this.sheet.close(clue);
    } catch (err) {
      this.error.set(revealErrorMessage(err));
      this.frame()?.scrollToTop();
    } finally {
      this.busy.set(false);
    }
  }
}

/** Whether either button's words need more than half of the footer: then the
 * two stack. Measured on the buttons' own content (icon and label), which has
 * the same width whether they sit side by side or one over the other. */
function needsStack(foot: HTMLElement): boolean {
  // Only a phone stacks; from a tablet up the pair is right-aligned and equal.
  if (foot.ownerDocument.defaultView?.matchMedia?.('(min-width: 768px)').matches) {
    return false;
  }
  const buttons = Array.from(foot.querySelectorAll<HTMLElement>('button'));
  if (buttons.length < 2) {
    return false;
  }
  const gap = parseFloat(getComputedStyle(foot).columnGap) || 12;
  const half = (foot.clientWidth - gap) / 2;
  return buttons.some((button) => {
    const style = getComputedStyle(button);
    const padding = parseFloat(style.paddingLeft) + parseFloat(style.paddingRight);
    const content = Array.from(button.children).reduce(
      (sum, child) => sum + child.getBoundingClientRect().width,
      0,
    );
    return content + padding > half;
  });
}
