import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  Injector,
  afterNextRender,
  computed,
  inject,
  input,
  output,
  signal,
  viewChild,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import type { GetDungeonRoomsResponse } from '../../../../gen/meurpg/maps/v1/dungeons_pb';
import type { Map as MapMessage } from '../../../../gen/meurpg/maps/v1/maps_pb';
import { redrawFailure } from '../../../core/maps/dungeon-errors';
import { doorCountText, dungeonSizeText } from '../../../core/maps/dungeon-layout';
import { DungeonsClient } from '../../../core/maps/dungeons-client';
import { MapAsk } from '../../../shared/map-ask/map-ask';

/**
 * The "Imagem" panel of a generated dungeon's map (E10-05 5 and 6): the map's name with "Gerada pelo app", the seed and the size, and
 * "Redesenhar", asked in place ("Redesenhar a imagem?" with "Voltar" first and the one filled button under it, nothing is drawn before the
 * second click). `RedrawDungeonMap` draws a new image from the walls and doors the map has now and keeps every layer, point, token and
 * what the players remember.
 *
 * "Redesenhar" is offered only while the server says the image is still the generator's (`image_is_generated`); after the master put
 * another image on the map the panel says so and has no button. A combat running does not block it (it moves no one). The strokes the
 * editor still holds are sent first (`prepare`), so the new image has the walls the master just painted. Each refusal is said by its
 * reason, and the failed question stays open with the reason above its buttons.
 */
@Component({
  selector: 'app-dungeon-image',
  imports: [MapAsk, MatButtonModule, MatIconModule],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './dungeon-image.html',
  styleUrl: './dungeon-image.scss',
})
export class DungeonImage {
  private readonly api = inject(DungeonsClient);
  private readonly injector = inject(Injector);

  readonly campaignId = input.required<string>();
  readonly map = input.required<MapMessage>();
  readonly info = input.required<GetDungeonRoomsResponse>();
  /** Sends what the editor still holds (painted strokes) before the image is drawn; returns the reason it could not, or `null`. */
  readonly prepare = input<() => Promise<string | null>>(() => Promise.resolve(null));
  /** The map with its new image. */
  readonly redrawn = output<MapMessage>();
  /** The server refused: the page's reading of the map may be stale (the image changed, a combat started). */
  readonly refused = output<void>();

  protected readonly asking = signal(false);
  protected readonly busy = signal(false);
  protected readonly error = signal<string | null>(null);
  protected readonly done = signal('');

  private readonly opener = viewChild('opener', { read: ElementRef<HTMLButtonElement> });

  protected readonly summary = computed(() => {
    const i = this.info();
    return [
      `Semente ${i.seed}`,
      `${i.width} × ${i.height} quadrados (${dungeonSizeText(i.width, i.height)})`,
      doorCountText(i.doors),
    ];
  });

  protected start(): void {
    this.error.set(null);
    this.done.set('');
    this.asking.set(true);
  }

  protected back(): void {
    this.asking.set(false);
    this.error.set(null);
    this.focusOpener();
  }

  /** The focus goes back to the button that asked, once the panel has drawn it again. */
  private focusOpener(): void {
    afterNextRender(() => this.opener()?.nativeElement.focus(), { injector: this.injector });
  }

  protected async confirm(): Promise<void> {
    if (this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set(null);
    try {
      const reason = await this.prepare()();
      if (reason !== null) {
        this.error.set(reason);
        return;
      }
      const map = await this.api.redraw(this.campaignId(), this.map().id);
      this.asking.set(false);
      this.done.set(
        'A imagem foi desenhada de novo. As camadas, os pontos e os tokens continuam como estavam.',
      );
      this.redrawn.emit(map);
      this.focusOpener();
    } catch (err) {
      this.error.set(redrawFailure(err));
      this.refused.emit();
    } finally {
      this.busy.set(false);
    }
  }
}
