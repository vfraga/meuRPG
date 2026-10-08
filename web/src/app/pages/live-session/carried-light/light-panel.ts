import { Component, computed, effect, inject, input, signal, untracked } from '@angular/core';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatIconModule } from '@angular/material/icon';
import { MatInputModule } from '@angular/material/input';

import { CharacterKind } from '../../../../gen/meurpg/characters/v1/characters_pb';
import type { MapToken } from '../../../../gen/meurpg/maps/v1/maps_pb';
import { carriedErrorMessage, carriedLine, carriedOptions } from '../../../core/maps/carried-light';
import { type LightOption, LightPresets } from '../../../core/maps/light-presets';
import type { MapState } from '../../../core/maps/map-state';
import { MapsClient } from '../../../core/maps/maps-client';
import type { PartyMemberInfoVm } from '../live-session.types';

/**
 * "Luz dos personagens" (MR-036, E9-04 state 2): the master's panel on the session
 * page, one 48 px select for each player character on the map (Nenhuma, Tocha,
 * Lanterna coberta, Luz). A choice applies at once (`SetCarriedLight`, the master
 * for anyone) and the line under the list says what it did; the player's own row
 * changes the same light, and the last change stands. Only on a map with the fog
 * of war on, since a light matters nowhere else.
 */
@Component({
  selector: 'app-light-panel',
  imports: [MatFormFieldModule, MatIconModule, MatInputModule],
  templateUrl: './light-panel.html',
  styleUrl: './light-panel.scss',
})
export class LightPanel {
  private readonly api = inject(MapsClient);
  private readonly presets = inject(LightPresets);

  readonly campaignId = input.required<string>();
  readonly state = input.required<MapState>();
  readonly info = input<ReadonlyMap<string, PartyMemberInfoVm>>(new Map());

  private readonly all = signal<readonly LightOption[]>([]);
  protected readonly busyId = signal<string | null>(null);
  protected readonly error = signal('');
  protected readonly line = signal('');

  /** The player characters on the map (their own tokens; creatures carry nothing). */
  protected readonly rows = computed(() =>
    this.state()
      .tokens()
      .filter((t) => t.kind === CharacterKind.PLAYER && !t.creatureId)
      .map((t) => ({
        token: t,
        sub: this.info().get(t.characterId)?.playerName ?? 'Jogador sem nome',
        options: carriedOptions(this.all(), t.carriedLight),
      })),
  );

  constructor() {
    // A required input has no value in the constructor: read the presets once it has.
    effect(() => {
      const campaignId = this.campaignId();
      void untracked(() =>
        this.presets.list(campaignId).then(
          (list) => this.all.set(list),
          () => this.error.set('Não deu para ler as luzes. Recarregue a página.'),
        ),
      );
    });
  }

  protected async choose(token: MapToken, key: string, select: HTMLSelectElement): Promise<void> {
    const mapId = this.state().map()?.id;
    if (!mapId || this.busyId() !== null) {
      return;
    }
    this.busyId.set(token.characterId);
    this.error.set('');
    this.line.set('');
    try {
      const next = await this.api.setCarriedLight(this.campaignId(), mapId, token.characterId, key);
      this.state().upsertToken(next);
      const option = this.all().find((o) => o.key === key) ?? null;
      this.line.set(
        key === ''
          ? `${token.name} deixou de carregar luz.`
          : `${carriedLine(token.name, option)} Muda na hora no mapa de todos que enxergam esse lugar.`,
      );
    } catch (err) {
      select.value = token.carriedLight;
      this.error.set(carriedErrorMessage(err));
    } finally {
      this.busyId.set(null);
    }
  }
}
