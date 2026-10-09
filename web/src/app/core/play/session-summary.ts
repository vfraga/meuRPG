import { Injectable, inject } from '@angular/core';
import { createClient } from '@connectrpc/connect';
import { timestampDate } from '@bufbuild/protobuf/wkt';

import { CharacterService } from '../../../gen/meurpg/characters/v1/characters_pb';
import { PlayService } from '../../../gen/meurpg/play/v1/play_pb';
import { HighlightKind } from '../../../gen/meurpg/play/v1/combat_pb';
import type {
  SessionCharacterSummary,
  SessionSummary,
} from '../../../gen/meurpg/play/v1/summary_pb';
import {
  type OwnNumber,
  highlightRows,
  highlightTiles,
  ownNumbers,
} from '../combat/combat-highlights';
import type { HighlightTile } from '../combat/combat-highlights';
import { CONNECT_TRANSPORT } from '../connect/transport';
import { formatInt, tight } from '../format/text';
import { formatClock } from '../../shared/session-time/session-time';
import type { HighlightsTableRow } from '../../shared/highlights/highlights-table';

const NBSP = '\u00a0';

/**
 * `PlayService.GetSessionSummary` (MR-032): how an ended session went, as the
 * caller may see it (RN-20). The page reads it after the stream's
 * `session_ended`; an open session is refused (`SESSION_NOT_ENDED`).
 * `providedIn: 'root'`, imported only by lazy code.
 */
@Injectable({ providedIn: 'root' })
export class SessionSummaryClient {
  private readonly transport = inject(CONNECT_TRANSPORT);
  private readonly client = createClient(PlayService, this.transport);
  private readonly characters = createClient(CharacterService, this.transport);

  /** The players' display names by character (`ListCharacters`), for "de Caio" on the master's tiles.
   * A player without a display name is left out. */
  async playerNames(campaignId: string): Promise<ReadonlyMap<string, string>> {
    const res = await this.characters.listCharacters({ campaignId });
    return new Map(
      res.characters.flatMap((c) => {
        const name = c.playerDisplayName.trim();
        return name ? [[c.id, name] as const] : [];
      }),
    );
  }

  async get(campaignId: string, gameSessionId: string): Promise<SessionSummary> {
    const res = await this.client.getSessionSummary({ campaignId, gameSessionId });
    if (!res.summary) {
      throw new Error('GetSessionSummary answered without its result');
    }
    return res.summary;
  }
}

/** "3 h 5 min", "45 min", "2 h": the minutes of a session, hours and minutes
 * tied to their numbers. Under a minute says so. */
export function formatDuration(seconds: number): string {
  const minutes = Math.round(seconds / 60);
  if (minutes < 1) {
    return `menos de 1${NBSP}min`;
  }
  const h = Math.floor(minutes / 60);
  const m = minutes % 60;
  return [h > 0 ? `${h}${NBSP}h` : '', m > 0 || h === 0 ? `${m}${NBSP}min` : '']
    .filter(Boolean)
    .join(' ');
}

/** "9 de 12": passed of tried, the number never split from its "de". */
export function checksRatio(passed: number, tried: number): string {
  return `${passed}${NBSP}de${NBSP}${tried}`;
}

/** "das 20:05 às 23:10". */
export function sessionSpan(summary: SessionSummary): string {
  if (!summary.startedAt || !summary.endedAt) {
    return '';
  }
  return tight(
    `das ${formatClock(timestampDate(summary.startedAt))} às ${formatClock(timestampDate(summary.endedAt))}`,
  );
}

/** How long the session lasted, in seconds (the server's `duration`). */
export function durationSeconds(summary: SessionSummary): number {
  return summary.duration ? Number(summary.duration.seconds) : 0;
}

/** The session's highlight tiles: the combats' categories summed, then the
 * checks passed. The master's tile also says how many the winner tried. */
export function summaryTiles(summary: SessionSummary): HighlightTile[] {
  const tried = new Map(
    summary.players.map((p) => [p.highlights?.characterId ?? '', p.checksTried] as const),
  );
  return highlightTiles(summary, tried);
}

/** The master's tiles leave out "Mais tesouro encontrado": it has its own block
 * (`treasureRows`), with everyone who found something and not only the top. */
export function masterTiles(summary: SessionSummary): HighlightTile[] {
  return summaryTiles(summary).filter((t) => t.kind !== HighlightKind.TREASURE_FOUND);
}

/** The master's "Mais tesouro encontrado" block (E9-09): one row for every
 * character that found treasure in the session, in the order they first
 * appeared, with the PO (a find by two splits its value, rounded down). */
export function treasureRows(summary: SessionSummary): HighlightsTableRow[] {
  // The most first; a tie by name, and the server's order otherwise (the sort is stable).
  return summary.players
    .filter((p) => p.treasureFoundPo > 0)
    .sort(
      (a, b) =>
        b.treasureFoundPo - a.treasureFoundPo ||
        (a.highlights?.name ?? '').localeCompare(b.highlights?.name ?? '', 'pt-BR'),
    )
    .map((p) => ({
      id: p.highlights?.characterId ?? '',
      name: p.highlights?.name ?? '',
      cells: [`${formatInt(p.treasureFoundPo)}${NBSP}PO`],
    }));
}

/** The master's per-player table: the checks each player passed of the ones tried,
 * zeros included. One row for every player the server lists (they fought or rolled a
 * check); one who tried no test reads "nenhum teste", muted, not "0 de 0". */
export function summaryRows(summary: SessionSummary): HighlightsTableRow[] {
  // A character that only found treasure fought and tried nothing: no row of zeros
  // for it here ("Mais tesouro encontrado" is its own block).
  return summary.players
    .filter((p) => p.checksTried > 0 || foughtIn(p))
    .map((p) => ({
      id: p.highlights?.characterId ?? '',
      name: p.highlights?.name ?? '',
      cells: [p.checksTried === 0 ? 'nenhum teste' : checksRatio(p.checksPassed, p.checksTried)],
      muted: p.checksTried === 0,
    }));
}

/** The columns of the master's "Números de cada jogador", the combat's own order. */
export const COMBAT_COLUMNS = [
  'Dano causado',
  'Cura',
  'Dano recebido',
  'Golpes finais',
  'Acertos críticos',
] as const;

/** The master's "Números de cada jogador": the combat numbers of every player's
 * character that fought in the session, zeros included. A character that only
 * rolled a check or found treasure has no row here. */
export function combatRows(summary: SessionSummary): HighlightsTableRow[] {
  const fought = summary.players
    .filter(foughtIn)
    .flatMap((p) => (p.highlights ? [p.highlights] : []));
  return highlightRows(fought).map((r) => ({
    id: r.characterId,
    name: r.name,
    cells: [r.damageDealt, r.healingDone, r.damageTaken, r.finalBlows, r.criticalHits].map(String),
  }));
}

function foughtIn(p: SessionCharacterSummary): boolean {
  const h = p.highlights;
  return !!h && h.damageDealt + h.damageTaken + h.healingDone + h.finalBlows + h.criticalHits > 0;
}

/** "Seu resultado": the player's own numbers. The four of the combat when any is above 0, and the
 * checks they passed of the ones they tried, when they tried any. */
export function summaryOwn(mine: SessionCharacterSummary | undefined): OwnNumber[] {
  if (!mine) {
    return [];
  }
  const h = mine.highlights;
  const own: OwnNumber[] = foughtIn(mine) && h ? ownNumbers([h], h.characterId) : [];
  if (mine.checksTried > 0) {
    own.push({
      label: 'Testes passados fora do combate',
      value: checksRatio(mine.checksPassed, mine.checksTried),
    });
  }
  if (mine.treasureFoundPo > 0) {
    own.push({ label: 'Tesouro encontrado', value: `${formatInt(mine.treasureFoundPo)}${NBSP}PO` });
  }
  return own;
}
