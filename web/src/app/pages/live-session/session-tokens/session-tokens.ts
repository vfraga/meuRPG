import { Component, inject, input } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import { MapReveals } from '../../../core/maps/map-reveals';
import { MapState } from '../../../core/maps/map-state';
import { MapsClient } from '../../../core/maps/maps-client';
import { MapTokensList } from '../../../shared/map-lists/map-tokens-list';
import type { PartyMemberInfoVm } from '../../../core/play/live-session.types';

/**
 * The master's "Tokens no mapa" panel (E5-04, E5-06): each token of the
 * current map with "Visível" or "Escondido" and an outlined "Esconder" or
 * "Revelar aos jogadores" (RN-10). Without it the master could not reveal a
 * hidden NPC from the session page.
 */
@Component({
  selector: 'app-session-tokens',
  imports: [MapTokensList, MatIconModule],
  template: `
    <section class="mr-panel" aria-labelledby="tokens-heading">
      @if (state().map()) {
        <app-map-tokens-list
          [tokens]="state().tokens()"
          [info]="info()"
          [pendingId]="reveals.pendingId()"
          (toggle)="reveals.toggleToken($event.token, $event.hidden)"
        />
        @if (reveals.error(); as message) {
          <div class="mr-notice mr-notice--danger" role="alert">
            <mat-icon aria-hidden="true">error</mat-icon>
            <p>{{ message }}</p>
          </div>
        }
      } @else {
        <h2 class="title" id="tokens-heading">Tokens no mapa</h2>
        <p class="mr-muted">Escolha um mapa atual para ver os tokens dele.</p>
      }
      <p class="mr-visually-hidden" role="status" aria-live="polite">{{ reveals.announcement() }}</p>
    </section>
  `,
  styles: `
    :host {
      display: block;
    }

    .title {
      margin: 0 0 var(--mr-space-1);
      font-family: var(--mr-font-display);
      font-size: 21px;
      font-weight: 700;
      line-height: 26px;
    }

    p {
      margin: 0;
    }
  `,
})
export class SessionTokens {
  readonly campaignId = input.required<string>();
  readonly state = input.required<MapState>();
  readonly info = input<ReadonlyMap<string, PartyMemberInfoVm>>(new Map());

  protected readonly reveals = new MapReveals(
    inject(MapsClient),
    () => this.state(),
    () => this.campaignId(),
  );
}
