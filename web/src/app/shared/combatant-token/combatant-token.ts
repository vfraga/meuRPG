import { Component, computed, input } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

/**
 * A combatant's token (README-A, "Combatant token"): a player's character
 * is a round disc, filled `ink`; an NPC is a rounded square, with a 2px
 * `ink` border. The two differ by shape and fill, never by colour. A
 * defeated one is grey with a ✕; a hidden one (the master only) is dashed
 * with the eye-off badge. A player's creature (a familiar, summoned animals) is
 * round like a character with a dashed outline (docs/design/README.md). A combatant with conditions has a dot at its lower
 * right corner (E6-29): the names are in the lists and the map's text list.
 * `current` adds the 3px accent ring and `mine` the
 * player's own accent halo. The "Vez" word above a token on the map belongs
 * to the map, not here. Decorative: the lists and the map say the names, so
 * it is hidden from assistive tech.
 */
@Component({
  selector: 'app-combatant-token',
  imports: [MatIconModule],
  template: `
    @if (defeated()) {
      <mat-icon class="tk__x" aria-hidden="true">close</mat-icon>
    } @else {
      {{ initial() }}
    }
    @if (hidden()) {
      <span class="tk__badge"><mat-icon>visibility_off</mat-icon></span>
    }
    @if (marked()) {
      <span class="tk__dot"></span>
    }
  `,
  styleUrl: './combatant-token.scss',
  host: {
    'aria-hidden': 'true',
    '[class.tk--npc]': 'npc()',
    '[class.tk--creature]': 'creature()',
    '[class.tk--hidden]': 'hidden()',
    '[class.tk--defeated]': 'defeated()',
    '[class.tk--current]': 'current()',
    '[class.tk--mine]': 'mine()',
    '[class.tk--map]': 'onMap()',
    '[class.tk--seal-left]': 'sealLeft()',
    '[class.tk--long]': 'initial().length > 1',
    '[style.--tk]': 'css()',
  },
})
export class CombatantToken {
  readonly initial = input.required<string>();
  /** An NPC (a rounded square) rather than a player's character (a disc). */
  readonly npc = input(false);
  /** A player's creature: round, with a dashed ink outline (E9-12). */
  readonly creature = input(false);
  readonly hidden = input(false);
  readonly defeated = input(false);
  readonly current = input(false);
  /** The combatant has conditions marked: the dot of E6-29. */
  readonly marked = input(false);
  readonly mine = input(false);
  /** Drawn on a battle map: fixed colours, since the picture is not themed. */
  readonly onMap = input(false);
  /** The eye-off seal sits top left: the mount of a pair on the map, whose top right holds its rider's disc (588-04). */
  readonly sealLeft = input(false);
  /** The side: pixels as a number (28 in lists, 48 in the bar), or any CSS
   * length (the map sizes it from its square). */
  readonly size = input<number | string>(28);

  protected readonly css = computed(() =>
    typeof this.size() === 'number' ? `${this.size()}px` : (this.size() as string),
  );
}
