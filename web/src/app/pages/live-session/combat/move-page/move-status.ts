import { ChangeDetectionStrategy, Component, input, output } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import type { ExtraPart } from '../../../../core/combat/move-plan';

/** What the page says about the square chosen (all of it from the server's options). */
export interface MoveSummary {
  readonly kind: 'idle' | 'ok' | 'refused';
  /** "Mover 2,1 m", or "Sem caminho reto". */
  readonly title: string;
  /** "Depois restam 6,9 m.", or the way out of a refusal. */
  readonly detail: string;
  /** "Sair do alcance do Goblin 2 pode provocar um ataque de oportunidade." (or ''). */
  readonly warning: string;
  /** The known trap this square is inside ("Fosso escondido"), or ''. */
  readonly trap: string;
  /** What swimming, climbing and crawling add to the cost, each with its word, as the server counted them; absent when none. */
  readonly extra?: { readonly parts: readonly ExtraPart[]; readonly note: string };
}

/**
 * The line under the map on the "Mover" page (E6-10, E9-05, E9-13): the move
 * in words (a live region, so it works without seeing the map), a refusal as an
 * alert with the way out, the warning that leaving a reach may provoke an
 * opportunity attack (a warning: the server says "pode"), the known trap the
 * square is in, and "Desengajar", the text action that stops the warning.
 */
@Component({
  selector: 'app-move-status',
  imports: [MatButtonModule, MatIconModule],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    @switch (summary().kind) {
      @case ('ok') {
        <div class="status" role="status">
          <p class="status__title">{{ summary().title }}</p>
          @if (summary().extra; as extra) {
            <p class="status__extra">
              @for (part of extra.parts; track part.kind) {
                @if (!$first) {
                  <span aria-hidden="true"> · </span>
                }
                <span class="status__part">
                  @if (icons[part.kind]; as icon) {
                    <mat-icon aria-hidden="true">{{ icon }}</mat-icon>
                  }{{ part.text }}</span>
              }{{ extra.note }}
            </p>
          }
          <p class="status__text">{{ note() || summary().detail }}</p>
        </div>
      }
      @case ('refused') {
        <div class="mr-notice mr-notice--danger" role="alert">
          <mat-icon aria-hidden="true">block</mat-icon>
          <p><strong class="err__title">{{ summary().title }}</strong>{{ summary().detail }}</p>
        </div>
      }
      @default {
        <!-- Nothing chosen: the page's lead says what to do; the live region is here for what comes. -->
        <p class="mr-visually-hidden" role="status"></p>
      }
    }
    @if (summary().kind === 'ok' && summary().warning) {
      @if (chosen()) {
        <!-- A jump with Desengajar chosen: the action is spent when the jump is made, so it can still be taken back. -->
        <div class="mr-notice mr-notice--success" role="status">
          <mat-icon aria-hidden="true">check_circle</mat-icon>
          <p>
            <strong class="err__title">Desengajar escolhido.</strong>Nenhum movimento deste turno provoca ataque de
            oportunidade. Gasta a sua ação ao saltar.
          </p>
        </div>
        <button matButton type="button" class="disengage" [disabled]="busy()" (click)="undo.emit()">
          Voltar atrás (não desengajar)
        </button>
      } @else {
        <div role="status" class="mr-notice mr-notice--warning">
          <mat-icon aria-hidden="true">warning</mat-icon>
          <p>
            {{ summary().warning }}
            @if (canDisengage()) {
              <span class="note">Com Desengajar, nenhum movimento deste turno provoca isso.</span>
            }
          </p>
        </div>
        @if (canDisengage()) {
          <button matButton type="button" class="disengage" [disabled]="busy()" (click)="disengage.emit()">
            Desengajar (gasta a ação)
          </button>
        }
      }
    }
    @if (summary().kind === 'ok' && summary().trap) {
      <div role="status" class="mr-notice mr-notice--warning">
        <mat-icon aria-hidden="true">warning</mat-icon>
        <p>Esse quadrado fica dentro do {{ summary().trap }}.</p>
      </div>
    }
  `,
  styleUrl: './move-status.scss',
})
export class MoveStatus {
  /** The glyph before the word of a swim and of a climb (the crawl has none): the map's own marks, for Água and Escalável. */
  protected readonly icons: Readonly<Record<ExtraPart['kind'], string>> = {
    swim: 'waves',
    climb: 'keyboard_double_arrow_up',
    crawl: '',
  };
  readonly summary = input.required<MoveSummary>();
  readonly canDisengage = input(false);
  readonly busy = input(false);
  /** A readied move spends no travel: its note ("É o movimento preparado…") stands in the card for "Depois restam…" (board 857-06). */
  readonly note = input('');
  /** Desengajar was chosen for a jump, to be spent with the jump: the notice says so and "Voltar atrás" takes it back. */
  readonly chosen = input(false);
  readonly disengage = output<void>();
  readonly undo = output<void>();
}
