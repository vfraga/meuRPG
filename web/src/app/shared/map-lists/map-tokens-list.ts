import { Component, input, output } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import type { MapToken } from '../../../gen/meurpg/maps/v1/maps_pb';
import { tokenInitial, tokenKey } from '../map-view/map-geometry';
import { tokenKindLabel } from '../map-view/map-labels';

export interface TokenToggle {
  readonly token: MapToken;
  /** The state the master asked for. */
  readonly hidden: boolean;
}

/** What a row says under a token's name: a player's class line and player
 * ("Mago 3, de Vinicius"), "NPC, inimigo", or, for a character's creature, whose
 * it is ("Criatura de Pensantus": its `character_id` is the owner's). */
export function tokenSub(
  token: MapToken,
  info: ReadonlyMap<string, { classSummary: string; playerName: string | null }>,
  ownerName = '',
): string {
  if (token.creatureId) {
    return ownerName ? `Criatura de ${ownerName}` : 'Criatura de um personagem';
  }
  const known = info.get(token.characterId);
  if (token.kind !== 1) {
    return tokenKindLabel(token.kind);
  }
  const parts = [known?.classSummary, known?.playerName ? `de ${known.playerName}` : null];
  return parts.filter(Boolean).join(', ') || tokenKindLabel(token.kind);
}

/**
 * "Tokens no mapa" (E5-04, E5-06): each token with "Visível" or
 * "Escondido" and the same two outlined buttons as the points. The names
 * come from the party info the session page already has.
 */
@Component({
  selector: 'app-map-tokens-list',
  imports: [MatButtonModule, MatIconModule],
  templateUrl: './map-tokens-list.html',
  styleUrl: './map-lists.scss',
})
export class MapTokensList {
  readonly tokens = input.required<readonly MapToken[]>();
  readonly info = input<ReadonlyMap<string, { classSummary: string; playerName: string | null }>>(
    new Map(),
  );
  readonly pendingId = input<string | null>(null);
  readonly headingLevel = input<2 | 3>(2);
  readonly toggle = output<TokenToggle>();

  protected readonly key = tokenKey;
  protected readonly sub = (t: MapToken) => tokenSub(t, this.info(), this.ownerName(t));

  /** The owner's name for a creature's token, from the owner's own token on the map. */
  private ownerName(t: MapToken): string {
    return t.creatureId
      ? (this.tokens().find((o) => !o.creatureId && o.characterId === t.characterId)?.name ?? '')
      : '';
  }
  protected initial(t: MapToken): string {
    return tokenInitial(t, this.tokens());
  }
}
