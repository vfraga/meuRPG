import { ChangeDetectionStrategy, Component, computed, input, output } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import type { SessionSummary } from '../../../gen/meurpg/play/v1/summary_pb';
import {
  COMBAT_COLUMNS,
  combatRows,
  durationSeconds,
  formatDuration,
  masterTiles,
  summaryOwn,
  summaryRows,
  summaryTiles,
  treasureRows,
} from '../../core/play/session-summary';
import { HighlightTiles } from '../highlights/highlight-tiles';
import { HighlightsFrame } from '../highlights/highlights-frame';
import { HighlightsTable } from '../highlights/highlights-table';

/**
 * What an ended session's summary says (MR-032), drawn the same by the screen
 * the session ends on and by the page that keeps it (`/campaigns/:id/sessions/:n`).
 * It draws only what the server sent the reader (RN-20, RN-10); it hides nothing
 * and adds nothing.
 *
 * - **The master** gets "Resumo da sessão": the highlights of the whole session,
 *   "Testes passados fora do combate" per player (the caption says only scenes that
 *   showed their DC count) and "Mais tesouro encontrado" (everyone who found
 *   something, or the line that nobody did). The archive adds "Números de cada
 *   jogador" (the combat numbers, only when a combat happened) and says so when
 *   no combat did.
 * - **A player** gets the card "A sessão acabou": the winners with their numbers
 *   (no "de N"), "Você" on what their character won and "Seu resultado". The
 *   archive's card has no tag and nothing to close (it is a page, not a notice);
 *   when the player's character took no part it says so in one neutral line.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-session-summary-body',
  imports: [HighlightTiles, HighlightsFrame, HighlightsTable, MatIconModule],
  templateUrl: './session-summary-body.html',
  styleUrl: './session-summary-body.scss',
})
export class SessionSummaryBody {
  readonly summary = input.required<SessionSummary>();
  readonly isMaster = input(false);
  /** The players' names by character, for "de Caio" on the master's tiles. */
  readonly players = input<ReadonlyMap<string, string>>(new Map());
  /** The reader's own character: marks "Você" and names "Seu resultado". */
  readonly characterId = input('');
  readonly characterName = input('');
  /** The kept page: no notice (live region, tag, "Fechar"), and the extras of the archive. */
  readonly archive = input(false);

  /** The player closed the card (the ending screen only). */
  readonly closed = output<void>();

  protected readonly COMBAT_COLUMNS = COMBAT_COLUMNS;
  protected readonly tiles = computed(() => summaryTiles(this.summary()));
  /** The master's tiles: the treasure has its own block. */
  protected readonly masterTiles = computed(() => masterTiles(this.summary()));
  protected readonly treasure = computed(() => treasureRows(this.summary()));
  protected readonly rows = computed(() => summaryRows(this.summary()));
  protected readonly combat = computed(() => combatRows(this.summary()));
  protected readonly own = computed(() => summaryOwn(this.summary().mine));
  protected readonly cardSub = computed(() => {
    const d = formatDuration(durationSeconds(this.summary()));
    return d ? `Durou ${d}` : '';
  });
}
