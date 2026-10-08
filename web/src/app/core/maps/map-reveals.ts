import { signal } from '@angular/core';

import type { MapPoint, MapToken } from '../../../gen/meurpg/maps/v1/maps_pb';
import { mapErrorMessage } from './map-errors';
import type { MapsClient } from './maps-client';
import type { MapState } from './map-state';

/**
 * The reveal and hide buttons of "Pontos do mapa" and "Tokens no mapa"
 * (MR-009, RN-10), shared by the master's phone map and the session page:
 * one call per click (`SetMapPointRevealed`, `SetMapTokenHidden`), the
 * answer goes into the map's state (when that map is still the open one),
 * and the button waits while its call is in flight. A failure says so in `error` and leaves the state alone.
 */
export class MapReveals {
  /** The point or the character whose call is in flight. */
  readonly pendingId = signal<string | null>(null);
  readonly error = signal<string | null>(null);
  /** What a screen reader hears after a click ("Taverna do Javali foi revelada aos jogadores."). */
  readonly announcement = signal('');

  constructor(
    private readonly api: Pick<MapsClient, 'setPointRevealed' | 'setTokenHidden'>,
    private readonly stateOf: () => MapState,
    private readonly campaignId: () => string,
  ) {}

  async togglePoint(point: MapPoint, revealed: boolean): Promise<void> {
    const mapId = this.stateOf().map()?.id;
    if (!mapId || this.pendingId() !== null) {
      return;
    }
    this.pendingId.set(point.id);
    this.error.set(null);
    try {
      const saved = await this.api.setPointRevealed(this.campaignId(), mapId, point.id, revealed);
      if (this.stateOf().map()?.id === mapId) {
        this.stateOf().upsertPoint(saved);
      }
      this.announcement.set(
        revealed
          ? `${point.name} foi revelado aos jogadores.`
          : `${point.name} foi escondido dos jogadores.`,
      );
    } catch (err) {
      this.error.set(mapErrorMessage(err, 'mudar o ponto'));
    } finally {
      this.pendingId.set(null);
    }
  }

  async toggleToken(token: MapToken, hidden: boolean): Promise<void> {
    const mapId = this.stateOf().map()?.id;
    // A creature's token is a party token, never hidden, and its `characterId` is its owner's.
    if (!mapId || token.creatureId || this.pendingId() !== null) {
      return;
    }
    this.pendingId.set(token.characterId);
    this.error.set(null);
    try {
      const saved = await this.api.setTokenHidden(
        this.campaignId(),
        mapId,
        token.characterId,
        hidden,
      );
      if (this.stateOf().map()?.id === mapId) {
        this.stateOf().upsertToken(saved);
      }
      this.announcement.set(
        hidden
          ? `${token.name} foi escondido dos jogadores.`
          : `${token.name} foi revelado aos jogadores.`,
      );
    } catch (err) {
      this.error.set(mapErrorMessage(err, 'mudar o token'));
    } finally {
      this.pendingId.set(null);
    }
  }
}
