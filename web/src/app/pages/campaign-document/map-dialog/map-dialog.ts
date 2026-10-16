import { Component, OnInit, computed, inject, input, output, signal } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { RouterLink } from '@angular/router';

import { MapLegend } from '../../../shared/map-view/map-legend/map-legend';
import { pointAriaLabel } from '../../../shared/map-view/map-labels';
import { MapPinsLegend } from '../../../shared/map-pins/map-pins-legend';
import { MapPins } from '../../../shared/map-pins/map-pins';
import { MapView as MapPicture } from '../../../shared/map-view/map-view';
import { DocDialog } from '../doc-dialog/doc-dialog';
import {
  DocumentLinks,
  type MapView,
  type OpenSessionMap,
} from '../../../shared/campaign-document/document-clients';
import { type LookupState, lookupFailure } from '../document-copy';

/**
 * The map link's dialog (E5-29): the map's name, whether it is revealed
 * (only the master reads the document, so the line says it either way) and
 * whether it is the open session's current map, the map with every point
 * (hidden ones dashed, "Escondido"), the legend and "Abrir no editor de
 * mapas". Read-only: the points do not open here. The details come from
 * `GetMap`, the same authorized call as the maps screens: a map deleted since
 * the link was written answers `not_found` and shows "Este mapa foi apagado."
 */
@Component({
  selector: 'app-document-map-dialog',
  imports: [
    DocDialog,
    MapLegend,
    MapPicture,
    MapPins,
    MapPinsLegend,
    MatButtonModule,
    MatIconModule,
    RouterLink,
  ],
  templateUrl: './map-dialog.html',
  styleUrl: './map-dialog.scss',
})
export class DocumentMapDialog implements OnInit {
  private readonly links = inject(DocumentLinks);

  readonly campaignId = input.required<string>();
  readonly mapId = input.required<string>();
  /** The link's own text, for the title while the map loads. */
  readonly text = input('');
  readonly closed = output<void>();

  protected readonly state = signal<LookupState<MapView>>({ status: 'loading' });
  private readonly session = signal<OpenSessionMap | null>(null);

  /** "Revelado aos jogadores. Mapa atual da Sessão 4." */
  protected readonly subtitle = computed(() => {
    const s = this.state();
    if (s.status !== 'ready') {
      return '';
    }
    const visibility = s.value.revealed ? 'Revelado aos jogadores.' : 'Escondido dos jogadores.';
    const session = this.session();
    return session?.mapId === s.value.id
      ? `${visibility} Mapa atual da Sessão ${session.sessionNumber}.`
      : visibility;
  });

  protected readonly pointLabel = pointAriaLabel;

  ngOnInit(): void {
    this.load();
  }

  protected load(): void {
    this.state.set({ status: 'loading' });
    // The session line is a detail: it never holds the map back, and a
    // campaign without an open session just leaves it out.
    void this.links.openSessionMap(this.campaignId()).then((s) => this.session.set(s));
    this.links.getMap(this.campaignId(), this.mapId()).then(
      (value) => this.state.set({ status: 'ready', value }),
      (err: unknown) => this.state.set(lookupFailure(err)),
    );
  }

  /** Keeps a tall map inside the window: the box is never wider than the
   * room left under the dialog's head and foot allows, at the image's ratio. */
  protected maxWidth(image: { width: number; height: number }): string {
    return `calc((100dvh - 260px) * ${image.width / image.height})`;
  }
}
