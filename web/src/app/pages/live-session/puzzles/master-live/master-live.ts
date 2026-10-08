import {
  ChangeDetectionStrategy,
  Component,
  DestroyRef,
  Injector,
  afterNextRender,
  computed,
  inject,
  input,
  signal,
} from '@angular/core';

import {
  type MasterPuzzleRun,
  PuzzleRunStatus,
} from '../../../../../gen/meurpg/play/v1/puzzles_pb';
import { focusWithRing } from '../../../../core/creatures/focus-ring';
import { PuzzleSessionState } from '../../../../core/puzzles/puzzle-session';
import { MasterRun } from '../master-run/master-run';

/**
 * The master's live view of the puzzle he chose with "Ver ao vivo" (MR-038, E10-06 states 3 to 5), in the session's main column, above the
 * map: the board needs room, and the rail is for the list. One card is open at a time. When he closes the puzzle, the focus goes to the
 * row that can show it again.
 */
@Component({
  selector: 'app-master-live',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [MasterRun],
  template: `
    @for (r of shown(); track r.puzzle?.id) {
      <app-master-run [campaignId]="campaignId()" [run]="r" [now]="now()" (updated)="updated($event)" />
    }
  `,
  styles: ':host { display: block; } :host:empty { display: none; }',
})
export class MasterLive {
  private readonly injector = inject(Injector);

  readonly campaignId = input.required<string>();
  readonly state = input.required<PuzzleSessionState>();

  protected readonly now = signal(new Date());
  /** The chosen puzzle, when it is shown or solved (a closed one is a row again). */
  protected readonly run = computed(() => {
    const id = this.state().selectedId();
    return this.state()
      .runs()
      .find(
        (r) =>
          r.puzzle?.id === id &&
          (r.status === PuzzleRunStatus.SHOWN || r.status === PuzzleRunStatus.SOLVED),
      );
  });

  /** The chosen run as a list of one, tracked by its puzzle: choosing another puzzle builds a new card, so the
   * solution shown, the open question and the notices of the previous one never carry over. */
  protected readonly shown = computed(() => {
    const r = this.run();
    return r ? [r] : [];
  });

  constructor() {
    const timer = setInterval(() => this.now.set(new Date()), 1000);
    inject(DestroyRef).onDestroy(() => clearInterval(timer));
  }

  protected updated(run: MasterPuzzleRun): void {
    this.state().replace(run);
    if (run.status === PuzzleRunStatus.CLOSED) {
      const name = run.puzzle?.name ?? '';
      afterNextRender(
        () =>
          focusWithRing(
            document.querySelector<HTMLElement>(`[aria-label="Mostrar de novo ${name}"]`),
          ),
        { injector: this.injector },
      );
    }
  }
}
