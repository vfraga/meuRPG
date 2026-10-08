import { Component, computed, effect, inject, signal, type Signal, viewChild } from '@angular/core';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatButtonModule } from '@angular/material/button';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';
import type { Observable } from 'rxjs';

import { MapPointKind, type MapPoint } from '../../../../../gen/meurpg/maps/v1/maps_pb';
import { joinDots } from '../../../../core/format/text';
import { SceneClient } from '../../../../core/play/scene-client';
import { sceneErrorMessage } from '../../../../core/play/scene-errors';
import type { SceneState } from '../../../../core/play/scene-state';
import { clueCount } from '../../../../core/maps/scene-clues';
import { actionCount } from '../../../../core/play/scene-view';
import { SheetFrame } from '../../combat/sheet-frame/sheet-frame';
import { injectSheet, openSheet } from '../../combat/sheet-host';

/** What the session page hands the picker. */
export interface ScenePickerData {
  readonly campaignId: string;
  readonly mapName: string;
  /** The current map's points, as the page's map state holds them now (a refresh in flight lands in the open
   * picker); the picker keeps the scene ones. */
  readonly points: Signal<readonly MapPoint[]>;
  /** The scene open now, marked when the picker opens ("Trocar cena"). */
  readonly openPointId: string | null;
  readonly state: SceneState;
}

/** "Abrir uma cena": a dialog from a tablet up and a sheet on a phone. It
 * answers `true` once the scene is open. */
export function openScenePicker(
  dialog: MatDialog,
  bottomSheet: MatBottomSheet,
  data: ScenePickerData,
): Observable<boolean | undefined> {
  return openSheet<ScenePicker, ScenePickerData, boolean>(dialog, bottomSheet, ScenePicker, {
    data,
    ariaLabel: 'Abrir uma cena',
    labelledBy: 'scene-picker-t',
    width: '560px',
  });
}

/**
 * "Abrir uma cena" (E7-02, E7-05): the scene points of the current map as a
 * list of radios. Any of them opens, even one with no actions (question 63:
 * the description, the clues and the NPCs are enough to run a scene); a hidden
 * point can be opened and stays hidden on the map. The footer is "Cancelar" (outlined) and
 * "Abrir cena" (filled), right-aligned on a computer and two equal 48px
 * buttons on a phone. The call is made here; a refusal is said at the top of
 * the list, where it is seen.
 */
@Component({
  selector: 'app-scene-picker',
  imports: [MatButtonModule, MatIconModule, SheetFrame],
  templateUrl: './scene-picker.html',
  styleUrl: './scene-picker.scss',
})
export class ScenePicker {
  private readonly api = inject(SceneClient);
  private readonly sheet = injectSheet<ScenePickerData, boolean>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;

  protected readonly rows = computed(() =>
    this.data
      .points()
      .filter((p) => p.kind === MapPointKind.SCENE)
      .map((p) => ({
        id: p.id,
        name: p.name,
        revealed: p.revealed,
        detail: joinDots([
          p.revealed ? 'Revelado no mapa' : 'Escondido no mapa',
          ...(p.sceneActions.length > 0 ? [actionCount(p.sceneActions.length)] : []),
          ...(p.clues.length > 0 ? [clueCount(p.clues.length)] : []),
        ]),
      })),
  );
  protected readonly anyHidden = computed(() => this.rows().some((r) => !r.revealed));
  private readonly picked = signal<string | null>(this.initialChoice());
  /** The ticked point, or none when a refresh took it off the map. */
  protected readonly choice = computed(() => {
    const id = this.picked();
    return this.rows().some((r) => r.id === id) ? id : null;
  });
  protected readonly busy = signal(false);
  /** A request in the air: Esc and the backdrop do not close the sheet under it. */
  protected readonly lockWhileBusy = effect(() => this.sheet.lock(this.busy()));
  protected readonly error = signal('');

  private readonly frame = viewChild(SheetFrame);

  private initialChoice(): string | null {
    const open = this.data.points().find((p) => p.id === this.data.openPointId);
    const first = this.data.points().find((p) => p.kind === MapPointKind.SCENE);
    return (open ?? first)?.id ?? null;
  }

  protected pick(id: string): void {
    this.picked.set(id);
  }

  protected close(): void {
    this.sheet.close(false);
  }

  protected async confirm(): Promise<void> {
    const pointId = this.choice();
    if (pointId === null || this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const scene = await this.api.open(this.data.campaignId, pointId);
      this.data.state.openedHere(scene);
      this.sheet.close(true);
    } catch (err) {
      this.error.set(sceneErrorMessage(err, 'abrir a cena'));
      this.frame()?.scrollToTop();
    } finally {
      this.busy.set(false);
    }
  }
}
