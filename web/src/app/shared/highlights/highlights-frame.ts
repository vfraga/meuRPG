import { Component, computed, input, output } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import type { HighlightTile, OwnNumber } from '../../core/combat/combat-highlights';

/** The most tiles of "Seu resultado" that share one row in a wide card. */
const WIDE_ROW_MAX = 5;

/**
 * The players' card of an ending (MR-032, E8-11 states 3 and 5): "O combate
 * acabou" for a combat and "A sessão acabou" for the session. At the top of
 * the page, until the player closes it with "Fechar" or the ✕ (44px, so the way
 * out stays in sight even on a 320x568 screen where the card scrolls with the
 * page).
 *
 * It shows what the master's panel shows, without the table: every category that
 * has a winner, with the number and the names (a tie names everyone), and "Você"
 * on a tile the reader's own character won. "Seu resultado, Pensantus" gives the
 * reader's own numbers, zeros included: the server sends a player their own row,
 * never anyone else's (RN-20).
 *
 * The page does not move focus to it (the player is already reading it): the
 * title sits in a polite live region, so a screen reader says "O combate acabou"
 * and the rest is a tab away. Motion (a 200ms fade) only with
 * `prefers-reduced-motion: no-preference`.
 */
@Component({
  selector: 'app-highlights-frame',
  imports: [MatButtonModule, MatIconModule],
  templateUrl: './highlights-frame.html',
  styleUrl: './highlights-frame.scss',
})
export class HighlightsFrame {
  readonly ariaLabel = input.required<string>();
  /** The green word with the check: "Combate encerrado". Empty for a card that is a page, with no tag. */
  readonly tag = input('');
  readonly title = input.required<string>();
  /** "Emboscada na estrada · 4 rodadas". */
  readonly subtitle = input('');
  readonly tiles = input.required<readonly HighlightTile[]>();
  /** The title over the tiles: "Destaques" for a combat, "Resumo da sessão" for the session. */
  readonly heading = input('Destaques');
  /** Said when no category has a winner. */
  readonly none = input('');
  /** The reader's own character: marks "Você". */
  readonly characterId = input('');
  readonly ownTitle = input('');
  readonly own = input<readonly OwnNumber[]>([]);
  /** Said in place of "Seu resultado" when the reader's character has no numbers (the kept page). */
  readonly ownNone = input('');
  /** False for a card that is a page: nothing to close, no ✕ and no "Fechar". */
  readonly closable = input(true);
  /** True for the notice of an ending, whose title a screen reader says (polite live region). */
  readonly notice = input(true);

  /** How many tiles share a row when the card is wide: all of them up to five,
   * else two even rows, so no tile stands alone on its row. */
  protected readonly wideColumns = computed(() => {
    const n = this.own().length;
    return n <= WIDE_ROW_MAX ? Math.max(1, n) : Math.ceil(n / 2);
  });

  readonly closed = output<void>();

  protected mine(tile: HighlightTile): boolean {
    return !!this.characterId() && tile.characterIds.includes(this.characterId());
  }
}
