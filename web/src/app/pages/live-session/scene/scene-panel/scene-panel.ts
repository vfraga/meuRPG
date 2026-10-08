import {
  Component,
  ElementRef,
  Injector,
  afterNextRender,
  computed,
  effect,
  inject,
  input,
  untracked,
  viewChild,
} from '@angular/core';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatButtonModule } from '@angular/material/button';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';

import { MapPointKind } from '../../../../../gen/meurpg/maps/v1/maps_pb';
import type { MapState } from '../../../../core/maps/map-state';
import type { SceneState } from '../../../../core/play/scene-state';
import { openScenePicker } from '../scene-picker/scene-picker';

/**
 * "Cena de RP" on the master's page (E7-02 state 1, E7-05 state 2): the
 * panel above "Grupo" while no scene is open, with "Abrir cena", which
 * opens the picker. When the current map has no scene point it says so and
 * offers no button; when no map is chosen it asks for one. It leaves the
 * page when a scene opens (the open scene takes its place, in the map's
 * column). After the master closes a scene, "Abrir cena" takes focus.
 */
@Component({
  selector: 'app-scene-panel',
  imports: [MatButtonModule, MatIconModule],
  templateUrl: './scene-panel.html',
  styleUrl: './scene-panel.scss',
})
export class ScenePanel {
  private readonly dialog = inject(MatDialog);
  private readonly bottomSheet = inject(MatBottomSheet);
  private readonly injector = inject(Injector);

  readonly campaignId = input.required<string>();
  readonly state = input.required<SceneState>();
  readonly mapState = input.required<MapState>();

  protected readonly hasMap = computed(() => this.mapState().map() !== null);
  protected readonly hasScenes = computed(() =>
    this.mapState()
      .points()
      .some((p) => p.kind === MapPointKind.SCENE),
  );

  private readonly openButton = viewChild('openButton', { read: ElementRef<HTMLButtonElement> });

  constructor() {
    effect(() => {
      if (this.state().focusNext() === 'open' && this.openButton()) {
        const state = untracked(() => this.state());
        afterNextRender(
          () => {
            this.openButton()?.nativeElement.focus();
            state.focusNext.set(null);
          },
          { injector: this.injector },
        );
      }
    });
  }

  protected open(): void {
    const map = this.mapState();
    // The editor may have added actions since the page read the map.
    void map.refresh();
    openScenePicker(this.dialog, this.bottomSheet, {
      campaignId: this.campaignId(),
      mapName: map.map()?.name ?? '',
      points: map.points,
      openPointId: null,
      state: this.state(),
    }).subscribe();
  }
}
