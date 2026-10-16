import {
  Component,
  DestroyRef,
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
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';

import type { StageNpc } from '../../../../../gen/meurpg/play/v1/scene_pb';
import type { SceneState } from '../../../../core/play/scene-state';
import { joinNames } from '../../../../core/play/stage-view';
import { openSheet } from '../../../../shared/sheet/sheet-host';
import { StageFigure } from './stage-figure';
import { StageViewer, type StageViewerData } from './stage-viewer';

/**
 * The stage, as everyone but the master sees it (MR-031, E8-10): the first
 * block of the open scene, under its title. The NPCs the master put "em cena"
 * stand side by side as cut-outs (no box behind them), the name under each,
 * and the one who speaks is in front: bigger, with a 4px accent base line, a
 * bold name and the words "Fala agora". With three or four NPCs on a phone the
 * figures are one size on a grid of two columns, so nothing overlaps and every
 * name stays whole. The stage disappears when it is empty: there is no "Ninguém
 * em cena".
 *
 * Tapping or clicking a figure (or Enter on it) opens it larger, in a bottom
 * sheet on a phone and a 480px dialog from a tablet up: `StageViewer`, with
 * nothing but the name and the portrait. Focus goes back to the figure that
 * was tapped. If that NPC leaves the stage with the view open, the view closes
 * by itself (the live region says who left).
 *
 * Motion (entering, leaving, a new speaker growing) exists only with
 * `prefers-reduced-motion: no-preference` and only in answer to the master:
 * the first stage this page shows, and a reload, never animate.
 */
/** How long a leaving figure stays drawn, as long as its fade (E8-10 state 4). */
const LEAVE_MS = 200;

/** A figure as drawn: the NPC, and whether it is fading in or out. */
interface Fig {
  readonly npc: StageNpc;
  readonly entering: boolean;
  readonly leaving: boolean;
}

@Component({
  selector: 'app-stage-player',
  imports: [MatIconModule, StageFigure],
  templateUrl: './stage-player.html',
  styleUrl: './stage-player.scss',
})
export class StagePlayer {
  private readonly dialog = inject(MatDialog);
  private readonly bottomSheet = inject(MatBottomSheet);
  private readonly injector = inject(Injector);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private alive = true;

  readonly state = input.required<SceneState>();

  protected readonly stage = computed(() => this.state().stage());
  /** What is drawn: the stage, plus the NPCs that just left, kept for the length of their fade. A plain
   * CSS animation does the work (no Angular animation runtime, which would weigh on every page). */
  protected readonly figs = signal<readonly Fig[]>([]);
  private readonly timers = new Set<ReturnType<typeof setTimeout>>();
  protected readonly count = computed(() => this.stage().length);
  protected readonly label = computed(
    () => `Em cena: ${joinNames(this.stage().map((n) => n.name))}`,
  );
  /** Whether a change is an answer to the master (false until the first stage was drawn). */
  protected readonly motion = signal(false);

  constructor() {
    inject(DestroyRef).onDestroy(() => {
      this.alive = false;
      this.timers.forEach(clearTimeout);
    });
    afterNextRender(() => this.motion.set(true));
    effect(() => {
      const next = this.stage();
      untracked(() => this.sync(next));
    });
  }

  /** Whether motion is wanted: after the first draw, and with the person's preference allowing it. */
  private animates(): boolean {
    const view = this.host.nativeElement.ownerDocument.defaultView;
    return (
      this.motion() &&
      (view?.matchMedia?.('(prefers-reduced-motion: no-preference)').matches ?? false)
    );
  }

  /** Keeps the drawn figures in step with the stage: new ones fade in, gone ones fade out and are dropped. */
  private sync(next: readonly StageNpc[]): void {
    const animate = this.animates();
    const prev = this.figs();
    const live = new Map(next.map((n) => [n.id, n]));
    const out: Fig[] = [];
    for (const f of prev) {
      const npc = live.get(f.npc.id);
      if (npc) {
        out.push({ npc, entering: false, leaving: false });
      } else if (animate) {
        out.push({ ...f, entering: false, leaving: true });
        this.dropLater(f.npc.id);
      }
    }
    const known = new Set(prev.map((f) => f.npc.id));
    for (const npc of next) {
      if (!known.has(npc.id)) {
        out.push({ npc, entering: animate, leaving: false });
      }
    }
    this.figs.set(out);
  }

  private dropLater(id: string): void {
    const timer = setTimeout(() => {
      this.timers.delete(timer);
      this.figs.update((list) => list.filter((f) => !(f.leaving && f.npc.id === id)));
    }, LEAVE_MS);
    this.timers.add(timer);
  }

  protected figureLabel(npc: StageNpc): string {
    return npc.speaking ? `Ver ${npc.name} maior, fala agora` : `Ver ${npc.name} maior`;
  }

  protected thumb(npc: StageNpc): string {
    return npc.portraitUrl ? `${npc.portraitUrl}/thumb` : '';
  }

  protected side(index: number): 'left' | 'right' {
    return index < this.count() / 2 ? 'left' : 'right';
  }

  protected enlarge(npc: StageNpc): void {
    const data: StageViewerData = { state: this.state(), entryId: npc.id };
    openSheet<StageViewer, StageViewerData, void>(this.dialog, this.bottomSheet, StageViewer, {
      data,
      ariaLabel: npc.name,
      labelledBy: 'stage-viewer-title',
    }).subscribe(() => this.focusFigure(npc.id));
  }

  /** The dialog gives focus back, but a figure that was redrawn meanwhile is a new element. */
  private focusFigure(id: string): void {
    if (!this.alive) {
      return; // the page was left with the view open
    }
    afterNextRender(
      () => {
        const active = this.host.nativeElement.ownerDocument.activeElement;
        if (active && active !== this.host.nativeElement.ownerDocument.body) {
          return;
        }
        this.host.nativeElement.querySelector<HTMLElement>(`[data-stage-id="${id}"]`)?.focus();
      },
      { injector: this.injector },
    );
  }
}
