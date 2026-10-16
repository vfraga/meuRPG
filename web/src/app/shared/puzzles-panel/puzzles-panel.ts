import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  Injector,
  OnInit,
  afterNextRender,
  computed,
  inject,
  input,
  signal,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { RouterLink } from '@angular/router';

import type { Puzzle } from '../../../gen/meurpg/play/v1/puzzles_pb';
import { focusWithRing } from '../../core/creatures/focus-ring';
import { kindIcon, puzzleSummary } from '../../core/puzzles/puzzle-format';
import { puzzleErrorMessage } from '../../core/puzzles/puzzle-errors';
import { PuzzlesClient } from '../../core/puzzles/puzzles-client';
import { MapAsk } from '../map-ask/map-ask';

type State =
  | { readonly status: 'loading' }
  | { readonly status: 'error'; readonly message: string }
  | { readonly status: 'ready' };

/**
 * The master's "Quebra-cabeças" panel on `/campaigns/:id` (MR-038, E10-06 state 1): one row for each puzzle with its kind's
 * icon, its name, a line ("Apagar as luzes · 5 × 5") and its state, and "Novo quebra-cabeça". Only the master sees it: the
 * answers live here. "Editar" is there until the puzzle is first shown (the server refuses an edit after that), and "Arquivar"
 * asks in place: an archived puzzle leaves this list and the session's menu, what happened in past sessions stays, and
 * "Mostrar arquivados" brings it back with "Desarquivar". The artboard says "Apagar"; the server only archives (and what was
 * shown is part of the sessions' history), so the words say what happens.
 */
@Component({
  selector: 'app-puzzles-panel',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [MapAsk, MatButtonModule, MatIconModule, RouterLink],
  templateUrl: './puzzles-panel.html',
  styleUrl: './puzzles-panel.scss',
})
export class PuzzlesPanel implements OnInit {
  private readonly api = inject(PuzzlesClient);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly injector = inject(Injector);

  readonly campaignId = input.required<string>();

  protected readonly state = signal<State>({ status: 'loading' });
  protected readonly puzzles = signal<readonly Puzzle[]>([]);
  protected readonly withArchived = signal(false);
  /** The puzzle whose archive question is open. */
  protected readonly asking = signal<string | null>(null);
  protected readonly busy = signal(false);
  protected readonly notice = signal('');
  protected readonly icon = kindIcon;
  protected readonly summary = puzzleSummary;
  /** What the list shows: the archived ones only when asked (they are read with the rest, so "Mostrar os arquivados" costs no call). */
  protected readonly visible = computed(() =>
    this.withArchived() ? this.puzzles() : this.puzzles().filter((p) => !p.archived),
  );
  protected readonly archivedCount = computed(
    () => this.puzzles().filter((p) => p.archived).length,
  );

  ngOnInit(): void {
    void this.reload();
  }

  protected async reload(): Promise<void> {
    try {
      this.puzzles.set(await this.api.list(this.campaignId(), true));
      this.state.set({ status: 'ready' });
    } catch (err) {
      this.state.set({
        status: 'error',
        message: puzzleErrorMessage(err, 'abrir os quebra-cabeças'),
      });
    }
  }

  protected toggleArchived(): void {
    this.withArchived.update((v) => !v);
  }

  protected ask(puzzle: Puzzle): void {
    this.notice.set('');
    this.asking.set(puzzle.id);
  }

  protected cancel(puzzle: Puzzle): void {
    this.asking.set(null);
    // The question took the row's place: the button that asked gets the focus back.
    afterNextRender(
      () =>
        focusWithRing(
          this.host.nativeElement.querySelector<HTMLElement>(`[data-archive="${puzzle.id}"]`),
        ),
      { injector: this.injector },
    );
  }

  protected async archive(puzzle: Puzzle): Promise<void> {
    this.busy.set(true);
    try {
      await this.api.archive(this.campaignId(), puzzle.id);
      const at = this.visible().findIndex((p) => p.id === puzzle.id);
      this.asking.set(null);
      this.notice.set(`“${puzzle.name}” foi arquivado.`);
      await this.reload();
      // The row is gone: the focus goes to the row that took its place, or to the panel's title when it was the last.
      afterNextRender(
        () => {
          const buttons = Array.from(
            this.host.nativeElement.querySelectorAll<HTMLElement>('[data-archive]'),
          );
          focusWithRing(
            buttons[Math.min(at, buttons.length - 1)] ??
              this.host.nativeElement.querySelector<HTMLElement>('#puzzles-heading'),
          );
        },
        { injector: this.injector },
      );
    } catch (err) {
      this.asking.set(null);
      this.notice.set(puzzleErrorMessage(err, 'arquivar o quebra-cabeça'));
    } finally {
      this.busy.set(false);
    }
  }

  protected async unarchive(puzzle: Puzzle): Promise<void> {
    try {
      await this.api.unarchive(this.campaignId(), puzzle.id);
      this.notice.set(`“${puzzle.name}” voltou para a lista.`);
      await this.reload();
    } catch (err) {
      this.notice.set(puzzleErrorMessage(err, 'desarquivar o quebra-cabeça'));
    }
  }

  protected canEdit(puzzle: Puzzle): boolean {
    return !puzzle.archived && !puzzle.shown;
  }
}
