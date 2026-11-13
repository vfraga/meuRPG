import {
  ChangeDetectionStrategy,
  Component,
  DestroyRef,
  ElementRef,
  afterNextRender,
  computed,
  inject,
  input,
  signal,
} from '@angular/core';

import { cellsPath, outlinePath } from '../../../core/combat/spell-area';
import {
  badgeSpots,
  FALLBACK_CELL_PX,
  ZONE_BADGE_BORDER_PX,
  ZONE_BADGE_INSET_PX,
  ZONE_BADGE_PX,
} from '../../../core/combat/zone-badge';
import { overlapPaths, withoutShared } from '../../../core/combat/zone-overlap';
import {
  arrowTurn,
  BURNING_PATTERN,
  ZONE_PATTERNS,
  ZONE_TILE,
  zonePattern,
} from '../../../core/combat/zone-pattern';
import type { ZoneView } from '../../../core/combat/zones';

let nextId = 0;

interface Burning {
  readonly fill: string;
  readonly edge: string;
  /** Degrees the arrows turn so they point from the wall to this side; null when the wall's way is unknown: no arrows then. */
  readonly turn: number | null;
}

interface Drawn {
  readonly id: string;
  readonly fill: string;
  readonly edge: string;
  readonly pattern: number;
  readonly dashed: boolean;
  readonly burning: Burning | null;
  readonly letter: string;
  /** The badge's corner of the footprint, in percent of the map. */
  readonly x: number;
  readonly y: number;
}

/**
 * The spell zones on the battle map (RN-37): each footprint drawn from the squares the server sent (already cut by walls),
 * with a veil (what the zone does to sight), a pattern of its own per spell, an outline with a white halo and, for a wall, the
 * burning side as its own layer drawn from `burningSquares`. No name floats on the map: a letter in a white corner diamond ties
 * each zone to the legend and the list, which carry the name, the caster and the concentration. The badge is kept inside the map.
 * Where zones lie over each other each shared square is cut on its diagonal (older pattern upper left, newer lower right) and both outlines stay. A zone of a hidden caster has a dashed outline. It sits among the map's
 * own marks, under the tokens, and takes no pointer. The drawing is decorative (`aria-hidden`): the zones list in the combat panel
 * is the accessible version. It draws what the server sent, nothing more.
 */
@Component({
  selector: 'app-zone-layer',
  changeDetection: ChangeDetectionStrategy.OnPush,
  host: {
    'aria-hidden': 'true',
    '[style.--zl-badge]': "badgePx + 'px'",
    '[style.--zl-badge-inset]': "badgeInsetPx + 'px'",
    '[style.--zl-badge-border]': "badgeBorderPx + 'px'",
  },
  templateUrl: './zone-layer.html',
  styleUrl: './zone-layer.scss',
})
export class ZoneLayer {
  readonly columns = input.required<number>();
  readonly rows = input.required<number>();
  readonly zones = input.required<readonly ZoneView[]>();
  /** The corner letters tie each zone to the legend and the list; a zone drawn alone (the wall being laid) has none. */
  readonly badges = input(true);

  protected readonly uid = `zl-${nextId++}`;
  protected readonly patterns = ZONE_PATTERNS;
  protected readonly tile = ZONE_TILE;
  protected readonly burningLook = ZONE_PATTERNS[BURNING_PATTERN];
  protected readonly badgePx = ZONE_BADGE_PX;
  protected readonly badgeInsetPx = ZONE_BADGE_INSET_PX;
  protected readonly badgeBorderPx = ZONE_BADGE_BORDER_PX;
  private readonly width = signal(0);

  constructor() {
    const host = inject<ElementRef<HTMLElement>>(ElementRef).nativeElement;
    const destroy = inject(DestroyRef);
    afterNextRender(() => {
      this.width.set(host.clientWidth);
      if (typeof ResizeObserver !== 'undefined') {
        const ro = new ResizeObserver(() => this.width.set(host.clientWidth));
        ro.observe(host);
        destroy.onDestroy(() => ro.disconnect());
      }
    });
  }

  /** The badge's size in squares: its px over the square's px, measured once the map is on screen. */
  private readonly badgeSquares = computed(
    () => ZONE_BADGE_PX / (this.width() > 0 ? this.width() / this.columns() : FALLBACK_CELL_PX),
  );
  private readonly shown = computed(() => this.zones().filter((z) => z.squares.length > 0));
  /** Where zones lie over each other: each shared square split on its diagonal, the older pattern upper left, the newer lower right. */
  protected readonly overlaps = computed(() =>
    overlapPaths(this.shown().map((z) => z.squares)).map((o) => ({
      ...o,
      olderPattern: zonePattern(this.shown()[o.older].spellKey),
      newerPattern: zonePattern(this.shown()[o.newer].spellKey),
    })),
  );
  protected readonly drawn = computed<Drawn[]>(() => {
    const shown = this.shown();
    const spots = badgeSpots(
      shown.map((z) => z.squares),
      this.columns(),
      this.rows(),
      this.badgeSquares(),
    );
    const own = withoutShared(shown.map((z) => z.squares));
    return shown.map((z, i) => {
      const pattern = zonePattern(z.spellKey);
      return {
        id: z.id,
        fill: cellsPath(own[i]),
        edge: outlinePath(z.squares),
        pattern,
        dashed: z.dashed,
        burning:
          z.burningSquares.length > 0
            ? {
                fill: cellsPath(z.burningSquares),
                edge: outlinePath(z.burningSquares),
                turn: arrowTurn(z.wallSide, z.squares, z.origin),
              }
            : null,
        letter: z.letter,
        x: (spots[i].col / this.columns()) * 100,
        y: (spots[i].row / this.rows()) * 100,
      };
    });
  });
}
