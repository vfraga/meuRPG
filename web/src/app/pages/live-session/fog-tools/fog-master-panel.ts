import { Component, DestroyRef, effect, inject, input, output, untracked } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import type { Map as MapMessage } from '../../../../gen/meurpg/maps/v1/maps_pb';
import { MapsClient } from '../../../core/maps/maps-client';
import type { MapState } from '../../../core/maps/map-state';
import { ViewAsCounts } from '../../../core/maps/view-as';
import { ViewAsList, type ViewAsPerson } from '../../../shared/fog-map/view-as-list';
import { ViewAsMapView } from '../../../shared/fog-map/view-as-map';
import { LightPanel } from '../carried-light/light-panel';
import type { PartyMemberInfoVm } from '../../../core/play/live-session.types';

/**
 * The master's fog tools (MR-036, E9-03 and E9-04), next to the map on the session page and under the combat
 * during one: "Ver como" (the list, with the squares each character sees) and "Luz dos personagens". Which
 * character he looks as is the page's (`viewAs`), because the map that shows it is the session's own map out
 * of a combat, and, during one, this panel's own (the combat's map is the combat's). When that character
 * dies or leaves the campaign the read says `not_found`: the page goes back to "Todos" and this says so.
 */
@Component({
  selector: 'app-fog-master-panel',
  imports: [LightPanel, MatIconModule, ViewAsList, ViewAsMapView],
  template: `
    <app-view-as-list
      [people]="people()"
      [counts]="counts.counts()"
      [total]="counts.total()"
      [selected]="viewAs()"
      [groupVision]="map()?.groupVision ?? false"
      [note]="note()"
      (choose)="viewAsChange.emit($event)"
    />
    @if (goneNote()) {
      <p class="mr-notice mr-notice--warning gone" role="status">
        <mat-icon aria-hidden="true">info</mat-icon>
        <span>{{ goneNote() }}</span>
      </p>
    }
    @if (inCombat() && viewAs(); as characterId) {
      <app-view-as-map
        [campaignId]="campaignId()"
        [mapId]="map()!.id"
        [mapName]="map()?.name ?? ''"
        [characterId]="characterId"
        [name]="nameOf(characterId)"
        [playerName]="playerOf(characterId)"
        [imageWidth]="map()?.image?.width ?? 1"
        [imageHeight]="map()?.image?.height ?? 1"
        [tick]="tick()"
        (noteChange)="noteChange.emit($event)"
        (gone)="gone.emit()"
        (back)="viewAsChange.emit(null)"
      />
    }
    <app-light-panel [campaignId]="campaignId()" [state]="state()" [info]="info()" />
  `,
  styles: `
    :host {
      display: flex;
      flex-direction: column;
      gap: var(--mr-space-4);
      min-width: 0;
    }

    .gone {
      margin: 0;
    }
  `,
})
export class FogMasterPanel {
  private readonly api = inject(MapsClient);
  private timer: ReturnType<typeof setTimeout> | undefined;

  readonly campaignId = input.required<string>();
  readonly state = input.required<MapState>();
  readonly map = input<MapMessage | null>(null);
  readonly people = input<readonly ViewAsPerson[]>([]);
  readonly info = input<ReadonlyMap<string, PartyMemberInfoVm>>(new Map());
  /** Goes up when what a player sees may have changed. */
  readonly tick = input(0);
  readonly viewAs = input<string | null>(null);
  readonly note = input('');
  readonly goneNote = input('');
  readonly inCombat = input(false);

  readonly viewAsChange = output<string | null>();
  readonly noteChange = output<string>();
  readonly gone = output<void>();

  protected readonly counts = new ViewAsCounts((mapId, characterId) =>
    this.api.vision(this.campaignId(), mapId, characterId),
  );

  constructor() {
    inject(DestroyRef).onDestroy(() => clearTimeout(this.timer));
    // The list counts the squares each character sees, and again when it may have changed (one read for a burst).
    effect(() => {
      const map = this.map();
      const ids = this.people().map((p) => p.id);
      this.tick();
      if (!map?.fogEnabled) {
        return;
      }
      untracked(() => {
        clearTimeout(this.timer);
        this.timer = setTimeout(() => void this.counts.read(map.id, ids), 250);
      });
    });
  }

  protected nameOf(id: string): string {
    return this.people().find((p) => p.id === id)?.name ?? 'o personagem';
  }

  protected playerOf(id: string): string | null {
    return this.people().find((p) => p.id === id)?.sub || null;
  }
}
