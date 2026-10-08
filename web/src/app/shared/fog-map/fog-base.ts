import {
  ChangeDetectionStrategy,
  Component,
  computed,
  DestroyRef,
  effect,
  inject,
  input,
  output,
  signal,
  untracked,
} from '@angular/core';

import { type MapLayers, hasLayers } from '../../core/maps/layers';
import { Sight } from '../../core/maps/vision';
import {
  seenCount,
  type TileRect,
  type Vision,
  shadeRects,
  tileProgress,
  tileRects,
  tileUrl,
} from '../../core/maps/vision';
import { MapLayersOverlay } from '../map-layers/map-layers';

const MS_PER_SECOND = 1000;
/** A tile that did not come (the server throttles tiles with a 429 or a 503; an `<img>` cannot read the `Retry-After`)
 * is asked for again after 2, 4, 8, 16 and 30 s; after the last attempt the place stops waiting. */
const TILE_RETRIES = 5;
const TILE_RETRY_CAP_SECONDS = 30;

/** The whole picture of a map the viewer reads whole (a master): its URL and size. */
export interface FogImage {
  readonly url: string;
  readonly width: number;
  readonly height: number;
}

/**
 * The picture of a map with the fog of war on, as one viewer sees it (MR-036,
 * RN-10, MAP-LANGUAGE.md): the tiles the server made for them, the map's layers
 * (walls, terrain, cover) over them, and every square shaded by what the viewer
 * sees of it. It sits inside a surface that is as big as the map (the map view's
 * stage, the combat map's surface) and fills it.
 *
 * - **Tiles.** A player never gets the image whole, only tiles of 16 × 16
 *   squares. Each is an `<img>` at its place; the URL carries the tile's revision
 *   (`r`), so a tile that did not change keeps its URL and the browser keeps its
 *   copy, and a changed one is fetched again while the old pixels stay on screen
 *   (a tile only ever gains squares). A tile that has not arrived is a grey striped
 *   place with a dashed border, still (no animation). A tile the viewer does not
 *   have is not asked for: its squares are solid black. The master reading the
 *   whole image (no tiles) passes `image` instead.
 *   A tile the server did not give (throttled, or busy) is asked for again after a growing wait; it counts as
 *   arrived only when it does, or when the last attempt has failed.
 * - **Shading.** One state per square (`GetMapVision`): penumbra under 25 % black,
 *   "no escuro, em cinza" without colour (a backdrop filter, so the layers go grey
 *   too), "já visto" under 60 % black with fine dots, "não visto" solid black.
 *   Squares of a kind are joined into blocks. The pieces carry `data-shade`, and
 *   the host carries the counts, so a test reads the shading without pixels.
 * - **Loading is for the first load only.** Until every tile of a map and viewer has arrived once, a place
 *   waiting is striped (`loadingChange` says so, for the notice "parte N de M"). A tile that arrives later,
 *   because the viewer saw more, never brings the stripes or the notice back; `settledChange` tells which
 *   tiles are in, so the screen can keep an NPC's token off a place still on its way.
 * - **What was seen keeps its marks.** The layers are drawn under the shading, and the ones on remembered squares again over
 *   it, faded, so a remembered wall keeps its hatch, darkened with the picture.
 *
 * Presentational: it never calls the API. Decorative for assistive tech: the
 * legend, the caption and the lists say what the map holds.
 */
@Component({
  selector: 'app-fog-base',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [MapLayersOverlay],
  templateUrl: './fog-base.html',
  styleUrl: './fog-base.scss',
  host: {
    'aria-hidden': 'true',
    '[style.--cols]': 'vision().columns',
    '[style.--rows]': 'vision().rows',
    '[attr.data-tiles]': 'rects().length',
    '[attr.data-tiles-ready]': 'progress().done',
    '[attr.data-shaded]': 'shaded()',
    '[attr.data-seen]': 'seen()',
  },
})
export class FogBase {
  readonly vision = input.required<Vision>();
  readonly layers = input<MapLayers | null>(null);
  /** The whole picture, when the viewer reads it whole (the master, or a map without the fog). */
  readonly image = input<FogImage | null>(null);
  /** The master reading as a player's character ("Ver como"): the tile URLs say so. */
  readonly forCharacter = input<string | null>(null);

  /** Whether the first load is still going: some tile of this map and viewer has not arrived yet. */
  readonly loadingChange = output<boolean>();
  /** The places whose tile has arrived (or failed every attempt: a place that will never come is not waited for). */
  readonly settledChange = output<ReadonlySet<string>>();

  /** The tiles that have settled at least once, by place. */
  private readonly settled = signal<ReadonlySet<string>>(new Set());
  /** True until every tile has arrived once; a map and a viewer start again at true. */
  private readonly initial = signal(true);
  /** The attempts made again, by place: part of the tile's URL, so the browser asks again. */
  private readonly retries = signal<ReadonlyMap<string, number>>(new Map());
  private readonly timers = new Set<ReturnType<typeof setTimeout>>();

  /** The route of the tiles: another map is another set of tiles (a changed revision is not). */
  private readonly route = computed(() => this.vision().tilesPath);
  protected readonly rects = computed(() => tileRects(this.vision()));
  protected readonly progress = computed(() => tileProgress(this.rects(), this.settled()));
  protected readonly blocks = computed(() => shadeRects(this.vision()));
  /** The layers on the squares seen before and not now, to draw again over their shading. */
  protected readonly remembered = computed<MapLayers | null>(() => {
    const l = this.layers();
    const v = this.vision();
    if (!l || l.columns !== v.columns) {
      return null;
    }
    const on = (s: { col: number; row: number }) =>
      v.states[s.row * v.columns + s.col] === Sight.Remembered;
    const out = {
      ...l,
      walls: l.walls.filter(on),
      terrain: l.terrain.filter(on),
      half: l.half.filter(on),
      threeQuarters: l.threeQuarters.filter(on),
      doors: (l.doors ?? []).filter(on),
    };
    return hasLayers(out) ? out : null;
  });
  protected readonly grey = computed(() => this.blocks().filter((b) => b.shade === 'grey'));
  protected readonly dark = computed(() => this.blocks().filter((b) => b.shade !== 'grey'));
  protected readonly seen = computed(() => seenCount(this.vision()));
  protected readonly shaded = computed(() => {
    const counts: Record<string, number> = { dim: 0, grey: 0, remembered: 0, unseen: 0 };
    for (const b of this.blocks()) {
      counts[b.shade] += b.cols * b.rows;
    }
    return `dim:${counts['dim']} grey:${counts['grey']} remembered:${counts['remembered']} unseen:${counts['unseen']}`;
  });

  constructor() {
    inject(DestroyRef).onDestroy(() => this.cancelRetries());
    // Another route, map or viewer is another set of tiles: nothing has arrived yet.
    effect(() => {
      this.route();
      this.forCharacter();
      untracked(() => {
        this.cancelRetries();
        this.retries.set(new Map());
        this.settled.set(new Set());
        this.initial.set(true);
      });
    });
    // Every tile in once: the first load is over for good.
    effect(() => {
      const { done, total } = this.progress();
      if (done >= total) {
        untracked(() => this.initial.set(false));
      }
    });
    effect(() =>
      this.loadingChange.emit(this.initial() && this.progress().done < this.progress().total),
    );
    effect(() => this.settledChange.emit(this.settled()));
  }

  protected src(rect: TileRect): string {
    const url = tileUrl(this.vision(), rect, this.forCharacter());
    const retry = this.retries().get(rect.key);
    return retry ? `${url}&retry=${retry}` : url;
  }

  protected pending(rect: TileRect): boolean {
    return this.initial() && !this.settled().has(rect.key);
  }

  protected settle(rect: TileRect): void {
    if (!this.settled().has(rect.key)) {
      this.settled.update((set) => new Set(set).add(rect.key));
    }
  }

  /** The tile did not come: ask again after a wait, and give up (stop waiting for it) after the last attempt. */
  protected fail(rect: TileRect): void {
    const attempt = this.retries().get(rect.key) ?? 0;
    if (attempt >= TILE_RETRIES) {
      this.settle(rect);
      return;
    }
    const timer = setTimeout(
      () => {
        this.timers.delete(timer);
        this.retries.update((map) => new Map(map).set(rect.key, attempt + 1));
      },
      Math.min(2 * 2 ** attempt, TILE_RETRY_CAP_SECONDS) * MS_PER_SECOND,
    );
    this.timers.add(timer);
  }

  private cancelRetries(): void {
    this.timers.forEach(clearTimeout);
    this.timers.clear();
  }

  protected pct(value: number, of: number): number {
    return (value / of) * 100;
  }
}
