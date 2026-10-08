import { Component, effect, ElementRef, inject, signal } from '@angular/core';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatButtonModule } from '@angular/material/button';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';

import type { MapToken } from '../../../../gen/meurpg/maps/v1/maps_pb';
import { carriedErrorMessage } from '../../../core/maps/carried-light';
import { type LightOption, NO_LIGHT } from '../../../core/maps/light-presets';
import { MapsClient } from '../../../core/maps/maps-client';
import { SheetFrame } from '../combat/sheet-frame/sheet-frame';
import { injectSheet, openSheet } from '../combat/sheet-host';

/** What the sheet needs: who carries, what is carried now, the choices, and what to tell the page after each. */
export interface CarriedLightData {
  readonly campaignId: string;
  readonly mapId: string;
  readonly characterId: string;
  readonly characterName: string;
  readonly current: string;
  readonly options: readonly LightOption[];
  /** The light was set: the new token, and the option (`null` for none). */
  readonly changed: (token: MapToken, option: LightOption | null) => void;
}

/** Opens "Luz que você carrega": a bottom sheet on a phone, a dialog from a tablet up. It answers nothing: each choice is applied at once. */
export function openCarriedLight(
  dialog: MatDialog,
  bottomSheet: MatBottomSheet,
  data: CarriedLightData,
) {
  return openSheet<CarriedLightSheet, CarriedLightData, void>(
    dialog,
    bottomSheet,
    CarriedLightSheet,
    {
      data,
      ariaLabel: 'Luz que você carrega',
      labelledBy: 'sheet-t',
      width: '480px',
    },
  );
}

let nextId = 0;

/**
 * "Luz que você carrega" (MR-036, E9-04): Nenhuma, Tocha, Lanterna coberta and
 * Luz as radio cards with their radii in meters. A choice applies at once
 * (`SetCarriedLight`, no "Salvar"); "Pronto" only closes. The title takes focus on
 * open, Esc and the × close, and on a 320 × 568 phone the list scrolls inside the
 * frame, with the title and "Pronto" always in reach (`sheet-frame`). The page
 * gets each result (`changed`) to show the toast and read the map again.
 */
@Component({
  selector: 'app-carried-light-sheet',
  imports: [MatButtonModule, MatIconModule, SheetFrame],
  templateUrl: './carried-light-sheet.html',
  styleUrl: './carried-light-sheet.scss',
})
export class CarriedLightSheet {
  private readonly api = inject(MapsClient);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly sheet = injectSheet<CarriedLightData, void>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;
  protected readonly none = NO_LIGHT;
  protected readonly id = `carried-light-${nextId++}`;

  protected readonly chosen = signal(this.data.current);
  protected readonly busy = signal(false);
  /** A request in the air: Esc and the backdrop do not close the sheet under it. */
  protected readonly lockWhileBusy = effect(() => this.sheet.lock(this.busy()));
  protected readonly error = signal('');

  protected async pick(key: string): Promise<void> {
    if (key === this.chosen() || this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const token = await this.api.setCarriedLight(
        this.data.campaignId,
        this.data.mapId,
        this.data.characterId,
        key,
      );
      this.chosen.set(key);
      this.data.changed(token, this.data.options.find((o) => o.key === key) ?? null);
    } catch (err) {
      this.error.set(carriedErrorMessage(err));
      // The radio the person pressed is checked on screen; the choice they had is still the one in force.
      this.host.nativeElement
        .querySelectorAll<HTMLInputElement>('input[type=radio]')
        .forEach((r) => (r.checked = r.dataset['key'] === this.chosen()));
    } finally {
      this.busy.set(false);
    }
  }

  protected close(): void {
    this.sheet.close();
  }
}
