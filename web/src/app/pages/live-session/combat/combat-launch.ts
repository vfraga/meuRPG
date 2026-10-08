import { Component, computed, inject, input, output } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';
import type { Observable } from 'rxjs';

import type { Encounter } from '../../../../gen/meurpg/play/v1/combat_pb';
import type { MapState } from '../../../core/maps/map-state';
import { PHONE_QUERY, mediaQuery } from '../../../shared/map-view/media-query';
import {
  type CombatMapInfo,
  StartCombatDialog,
  type StartCombatData,
} from './start-combat/start-combat-dialog';

/** The session's current map as the start dialog wants it, or `null` while there is none or it has no image. */
export function combatMapInfo(state: MapState): CombatMapInfo | null {
  const map = state.map();
  return map?.image
    ? {
        id: map.id,
        name: map.name,
        image: { url: map.image.url, width: map.image.width, height: map.image.height },
        columns: map.gridColumns,
        rows: map.gridRows,
      }
    : null;
}

/** Opens "Iniciar combate" (a full-screen dialog on a phone) and answers with the combat it started, or `undefined`. */
export function openStartCombat(
  dialog: MatDialog,
  phone: boolean,
  data: StartCombatData,
): Observable<Encounter | undefined> {
  return dialog
    .open<StartCombatDialog, StartCombatData, Encounter>(StartCombatDialog, {
      data,
      width: phone ? '100vw' : '760px',
      maxWidth: phone ? '100vw' : 'calc(100vw - 32px)',
      height: phone ? '100dvh' : undefined,
      maxHeight: phone ? '100dvh' : '92dvh',
      ariaLabelledBy: 'start-title',
      autoFocus: 'first-tabbable',
    })
    .afterClosed();
}

/**
 * "Combate" on the master's session page while there is none (MR-013): one
 * line and the button that opens "Iniciar combate" (E6-01). The map the
 * fight is on is the session's current map. "Iniciar combate" is an outline:
 * the session page has no filled button of its own, and the combat's own
 * screens have theirs.
 */
@Component({
  selector: 'app-combat-launch',
  imports: [MatButtonModule, MatIconModule],
  template: `
    <section class="mr-panel launch" aria-labelledby="launch-title">
      <div class="launch__text">
        <h2 class="mr-panel__title" id="launch-title">Combate</h2>
        <p class="mr-muted">
          @if (loading()) {
            Lendo o mapa atual…
          } @else if (unreadable()) {
            Não deu para ler o mapa atual. Sem ele o combate começaria no teatro da mente, e isso não muda depois de iniciado.
          } @else if (state().map()) {
            Escolha quem luta e o app pede a iniciativa de todos.
          } @else {
            Sem um mapa atual, o combate é no teatro da mente: só a ordem, o movimento por número e a sua palavra.
          }
        </p>
      </div>
      <!-- Disabled while the current map is read: the dialog takes the map when it opens, and a click in that moment
           would offer only the theatre of the mind for a session that has a map. -->
      <button
        mat-stroked-button
        type="button"
        class="launch__button"
        [disabled]="loading() || unreadable()"
        (click)="open()"
      >
        <mat-icon aria-hidden="true">swords</mat-icon>Iniciar combate
      </button>
      @if (unreadable()) {
        <button mat-stroked-button type="button" class="launch__button" (click)="retry()">
          <mat-icon aria-hidden="true">refresh</mat-icon>Tentar de novo
        </button>
      }
    </section>
  `,
  styles: `
    :host {
      display: block;
    }

    .launch {
      display: flex;
      flex-direction: column;
      gap: var(--mr-space-3);

      @media (min-width: 768px) {
        flex-direction: row;
        align-items: center;
        justify-content: space-between;
      }
    }

    .mr-panel__title {
      margin: 0 0 2px;
    }

    p {
      margin: 0;
    }

    .launch__button {
      --mat-button-outlined-label-text-color: var(--mr-ink);
      flex: none;
    }
  `,
})
export class CombatLaunch {
  private readonly dialog = inject(MatDialog);
  private readonly phone = mediaQuery(PHONE_QUERY);

  readonly campaignId = input.required<string>();
  readonly state = input.required<MapState>();
  /** The combat that was started: the page shows it. */
  readonly started = output<Encounter>();

  protected readonly map = computed<CombatMapInfo | null>(() => combatMapInfo(this.state()));
  /** The session's current map is still being read: "Iniciar combate" waits for it. */
  protected readonly loading = computed(() => this.state().status() === 'loading');

  /** The read of the current map failed: unlike a session without a map, its answer is unknown. */
  protected readonly unreadable = computed(() => this.state().status() === 'error');

  protected retry(): void {
    void this.state().refresh();
  }

  protected open(): void {
    if (this.loading() || this.unreadable()) {
      return;
    }
    const map = this.map();
    // Without a map the combat starts without one (the theatre of the mind, RN-25): the dialog offers the choice.
    openStartCombat(this.dialog, this.phone(), {
      campaignId: this.campaignId(),
      mode: 'start',
      map,
    }).subscribe((encounter) => {
      if (encounter) {
        this.started.emit(encounter);
      }
    });
  }
}
