import {
  Component,
  computed,
  effect,
  input,
  output,
  signal,
  untracked,
  viewChild,
} from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import { CharacterKind } from '../../../gen/meurpg/characters/v1/characters_pb';
import type { MapPoint, MapToken } from '../../../gen/meurpg/maps/v1/maps_pb';
import { isPinKind } from '../../core/traps/trap-text';
import { combatantInitial } from '../../core/combat/combat-view';
import type { FogError, FogStatus } from '../../core/maps/fog-view';
import { type DoorSquare, hasLayers, type MapLayers, NO_LAYERS } from '../../core/maps/layers';
import { tokenKey } from '../../core/maps/map-state';
import { type Vision, Sight, seenCount, tileRects, visionLegend } from '../../core/maps/vision';
import { CombatantToken } from '../combatant-token/combatant-token';
import { DoorPicks } from '../map-layers/door-picks';
import { MapLayersLegend } from '../map-layers/map-layers-legend';
import { centroid, tokenInitial, ViewToken } from '../map-view/map-geometry';
import { type MapMove, MapView } from '../map-view/map-view';
import { PHONE_QUERY, mediaQuery } from '../map-view/media-query';
import { MapPins } from '../map-pins/map-pins';
import { MapPinsLegend } from '../map-pins/map-pins-legend';
import { DarkMarks } from './dark-marks';
import { FogBase, type FogImage } from './fog-base';
import { fullscreenOfHost } from '../../core/ui/fullscreen';

/** How a viewer is addressed: "Seu personagem" (their own), or, for the master, the character's name. */
export interface FogViewer {
  readonly name: string;
  readonly own: boolean;
}

/**
 * The map of the session (and of the combat, and of the map page) when the fog of war is on (MR-036,
 * RN-10; E9-03), as one viewer sees it: the tiles, the layers and the shading of `app-fog-base` inside the
 * map view's pan and zoom, the party's tokens always, the NPCs and creatures as the server's reads give
 * them, a legend under the map that names every mark it draws, and the notices.
 *
 * The same piece is the player's map, the master's own map ("Todos": the whole image and every square
 * seen, with his tokens to drag) and his "Ver como" a player (`forCharacter`: that player's tiles), and
 * the wolf's view of Wild Shape (9.17). It decides nothing: the server sent each square's state and each
 * token it may show.
 *
 * - **Loading is for the first load only** (a map and a viewer): "Carregando o mapa" (`role="status"`,
 *   read once) with "parte N de M", still stripes where a tile has not come. A tile that arrives later
 *   brings back neither. **The party never vanishes** (docs/design/README.md): its tokens are drawn over a
 *   place still on its way; only an NPC's waits for its tile.
 * - **Tokens:** NPCs first, the viewer's own last, on top. On a phone the map opens at 2x on the party,
 *   and the party's chips (44 px) take the view to a character.
 * - **Legend:** only the marks this map draws; the fog's states come from the squares (a viewer without
 *   darkvision has no "No escuro, em cinza"), the companion's initial is the companion's own.
 * - **"Você vê" (players):** the character's senses, the light carried, who is in sight, and what grey
 *   and the remembered squares mean. Not a count of squares: the master's list has it.
 */
@Component({
  selector: 'app-fog-map',
  imports: [
    CombatantToken,
    DarkMarks,
    DoorPicks,
    FogBase,
    MapLayersLegend,
    MapPins,
    MapPinsLegend,
    MapView,
    MatIconModule,
  ],
  templateUrl: './fog-map.html',
  styleUrl: './fog-map.scss',
})
export class FogMap {
  protected readonly fullscreen = fullscreenOfHost();
  readonly mapName = input('');
  /** The map's picture size, to reserve the frame (a player gets no image, only its size). */
  readonly imageWidth = input.required<number>();
  readonly imageHeight = input.required<number>();
  readonly status = input<FogStatus>('ready');
  /** Why the last read failed, when it did. */
  readonly error = input<FogError | null>(null);
  readonly vision = input<Vision | null>(null);
  readonly layers = input<MapLayers | null>(null);
  readonly tokens = input<readonly MapToken[]>([]);
  readonly points = input<readonly MapPoint[]>([]);
  /**
   * Draw the trap, treasure and light marks (`app-map-pins`) over the map, from `points` as the server sent them (a player
   * gets only what they see or remember, `MapPoint.remembered`), and leave them out of the plain markers (9.14).
   */
  readonly pins = input(false);
  /** The master reading as this character ("Ver como"): the tiles' URLs say so. */
  readonly forCharacter = input<string | null>(null);
  /** The whole picture for a viewer that reads it whole; a player's map has none. */
  readonly image = input<FogImage | null>(null);
  /** Who sees: names the character in the off-map notice, the legend and the chips. */
  readonly viewer = input<FogViewer | null>(null);
  /** The master's own map: hidden tokens and points are drawn (dashed), and the legend names his marks. */
  readonly isMaster = input(false);
  /** `tokens`: the master drags a token; `view`: pan and zoom only. */
  readonly mode = input<'view' | 'tokens'>('view');
  /** The master's map: each door is a button that opens its sheet (E10-05 10). */
  readonly doorTaps = input(false);
  /** A click on a point opens it (the map page). */
  readonly selectablePoints = input(false);
  /** The card "Você vê ..." under the legend (players). */
  readonly caption = input(true);
  /** The master's token marks (Personagem, NPC, Escondido) in the legend: off where the page draws its own row of them under the map (no word twice). */
  readonly tokenMarks = input(true);
  /** The character's senses ("Visão no escuro: 18 m") for that card. */
  readonly senses = input<readonly string[]>([]);
  /** What the character carries ("Tocha"), when it carries something. */
  readonly carried = input('');
  /** The familiar whose eyes these are ("Nanquim"): the card is "O que o Nanquim vê". */
  readonly familiar = input<string | null>(null);
  /** A label for the master's "Ver como" on a phone: "Vendo como Toren (Caio)". */
  readonly badge = input<string | null>(null);
  /** The zoom, fit and full-screen buttons under the map; the table screen (a TV nobody touches) leaves them out. */
  readonly tools = input(true);
  /** The notice is above the map; a screen that has its own banner leaves it. */
  readonly notices = input(true);
  /** The master's light warning: the ids of the characters who see nothing where they stand (a dashed ring each). */
  readonly dark = input<ReadonlySet<string>>(new Set());
  /** The one "Ver no mapa" chose: the only one with the label "<Nome> · no escuro". */
  readonly darkChosen = input<string | null>(null);

  /** A player's own token, the only one they may drag (the master has allowed it on this map). */
  readonly onlyToken = input<string | null>(null);

  readonly pointSelect = output<string>();
  /** The master tapped a door (`doorTaps`). */
  readonly doorPick = output<DoorSquare>();
  readonly moved = output<MapMove>();
  /** The master's click on a token, with Shift or not, and on an empty spot (B2: choose to move, and where to). */
  readonly tokenPick = output<{ id: string; additive: boolean }>();
  readonly emptyClick = output<{ xBp: number; yBp: number }>();

  protected readonly phone = mediaQuery(PHONE_QUERY);
  protected readonly view = viewChild(MapView);
  private readonly settled = signal<ReadonlySet<string>>(new Set());
  private readonly firstLoad = signal(true);

  protected readonly frame = computed<FogImage>(() => ({
    url: '',
    width: this.imageWidth(),
    height: this.imageHeight(),
  }));
  protected readonly noLayers = NO_LAYERS;
  protected readonly layerSet = computed(() => this.layers() ?? NO_LAYERS);

  /** The points the plain markers draw: with the pins on, the traps, chests and lights have their own drawing. */
  protected readonly markerPoints = computed(() =>
    this.pins() ? this.points().filter((p) => !isPinKind(p.kind)) : this.points(),
  );

  protected readonly rects = computed(() => {
    const v = this.vision();
    return v ? tileRects(v) : [];
  });
  /** The tile at the first load: "parte N de M". */
  protected readonly part = computed(() =>
    Math.min(this.rects().length, this.rects().filter((r) => this.settled().has(r.key)).length + 1),
  );
  private readonly route = computed(() => this.vision()?.tilesPath ?? '');
  protected readonly loading = computed(
    () => this.status() === 'loading' || (this.vision() !== null && this.firstLoad()),
  );

  /** The tokens the map draws. The party always; an NPC's only over a place whose tile has arrived (a square with no tile is black and has no wait). */
  protected readonly shownTokens = computed(() => {
    const v = this.vision();
    if (!v) {
      return [];
    }
    const waiting = (t: MapToken): boolean => {
      if (v.tilesPath === '' || !isNpc(t)) {
        return false;
      }
      const col = Math.min(v.columns - 1, Math.floor((t.xBp / 10000) * v.columns));
      const row = Math.min(v.rows - 1, Math.floor((t.yBp / 10000) * v.rows));
      const key = `${Math.floor(col / v.tileSquares)}:${Math.floor(row / v.tileSquares)}`;
      return !this.settled().has(key) && this.rects().some((r) => r.key === key);
    };
    // The viewer's own token last, so it is drawn on top of its neighbours.
    const rank = (t: MapToken) =>
      t.mine && !t.creatureId ? 3 : t.creatureId ? 2 : isParty(t) ? 1 : 0;
    return this.tokens()
      .filter((t) => !waiting(t))
      .sort((a, b) => rank(a) - rank(b));
  });

  protected readonly party = computed(() =>
    this.tokens().filter((t) => isParty(t) && !t.creatureId),
  );
  protected readonly enemies = computed(() => this.shownTokens().filter((t) => isNpc(t)));
  protected readonly creatures = computed(() => this.shownTokens().filter((t) => !!t.creatureId));
  protected readonly mine = computed(
    () => this.tokens().find((t) => t.mine && !t.creatureId) ?? null,
  );
  protected readonly companions = computed(() => this.party().filter((t) => !t.mine));
  protected readonly hiddenTokens = computed(() => this.tokens().filter((t) => t.hidden));
  protected readonly offMap = computed(() => {
    const v = this.vision();
    return v !== null && v.fogEnabled && !v.characterOnMap && !this.isMaster();
  });
  /** Nothing to show at all (a character off the map who never saw anything): a small box, not a black map. */
  protected readonly empty = computed(() => {
    const v = this.vision();
    return this.offMap() && v !== null && v.states.every((s) => s === Sight.Unseen);
  });
  protected readonly legend = computed(() => {
    const v = this.vision();
    if (!v) {
      return null;
    }
    const l = visionLegend(v);
    // The viewer's own square is always seen, in grey in the dark: one square under the viewer's own token is not "No escuro, em cinza".
    const me = this.mine();
    const ownSquare = me
      ? Math.min(v.rows - 1, Math.floor((me.yBp / 10000) * v.rows)) * v.columns +
        Math.min(v.columns - 1, Math.floor((me.xBp / 10000) * v.columns))
      : -1;
    const grey = v.states.some((s, n) => s === Sight.Grey && n !== ownSquare);
    const shown = { ...l, grey };
    // A map with nothing but "Visto" (the master's own) has no shading to explain.
    return shown.dim || shown.grey || shown.remembered || shown.unseen ? shown : null;
  });
  protected readonly hasLayerMarks = computed(() => hasLayers(this.layerSet()));
  protected readonly youLabel = computed(() =>
    this.viewer()?.own === false ? this.viewer()!.name : 'Você',
  );
  protected readonly ownView = computed(() => this.viewer()?.own !== false);
  /** The spot a phone opens on: the party, at 2x (the person pans and zooms from there). */
  protected readonly startAt = computed(() => {
    const v = this.vision();
    return this.phone() && v && !this.isMaster() && !this.empty() ? centroid(this.party()) : null;
  });

  protected readonly cardTitle = computed(() =>
    this.familiar() ? `O que o ${this.familiar()} vê` : 'Você vê',
  );
  /** What the player can use: senses, light, who is in sight, and what grey and remembered mean. */
  protected readonly lines = computed<readonly string[]>(() => {
    const v = this.vision();
    if (!v || this.loading() || this.offMap()) {
      return [];
    }
    const lines: string[] = [];
    if (!this.familiar()) {
      lines.push(...this.senses());
      if (this.carried()) {
        lines.push(`Luz que você carrega: ${this.carried()}.`);
      }
    }
    const names = this.enemies().map((t) => t.name);
    lines.push(
      names.length > 0 ? `Inimigos à vista: ${names.join(', ')}.` : 'Nenhum inimigo à vista.',
    );
    const l = this.legend();
    if (l?.grey) {
      lines.push('Em cinza: visto no escuro, pela visão no escuro.');
    }
    if (l?.remembered) {
      lines.push('O que você já viu fica escurecido e sem inimigos: pode ter mudado.');
    }
    return lines;
  });

  protected readonly offMapTitle = computed(() => {
    const viewer = this.viewer();
    return viewer ? `${viewer.name} fora do mapa` : 'Personagem fora do mapa';
  });
  protected readonly offMapText = computed(() =>
    this.viewer()?.own === false
      ? 'O personagem não está neste mapa: o jogador vê só o que já tinha visto.'
      : 'Seu personagem não está neste mapa. Você vê só o que já tinha visto.',
  );
  protected readonly seenNow = computed(() => {
    const v = this.vision();
    return v ? seenCount(v) : 0;
  });

  protected readonly initialOf = (token: ViewToken, all: readonly ViewToken[]): string =>
    isNpc(token) ? combatantInitial(token.name) : tokenInitial(token, all);
  protected readonly key = tokenKey;

  constructor() {
    // A new map or viewer starts its first load again.
    effect(() => {
      this.route();
      this.forCharacter();
      untracked(() => this.firstLoad.set(true));
    });
  }

  protected initial(t: MapToken): string {
    return this.initialOf(t, this.tokens());
  }

  protected onSettled(set: ReadonlySet<string>): void {
    this.settled.set(set);
  }

  protected onLoading(loading: boolean): void {
    this.firstLoad.set(loading);
  }

  protected goTo(t: MapToken): void {
    this.view()?.focusOn({ xBp: t.xBp, yBp: t.yBp });
  }

  /** "Ver no mapa" (the master's light warning): the map centers on the character. */
  showCharacter(characterId: string): void {
    const t = this.tokens().find((x) => !x.creatureId && x.characterId === characterId);
    // On a phone the map zooms in on the character; on a computer it keeps the zoom it has (the whole map, unless the master zoomed).
    if (t) {
      this.view()?.focusOn(
        { xBp: t.xBp, yBp: t.yBp },
        this.phone() ? undefined : this.view()?.scale(),
      );
    }
  }

  zoomIn(): void {
    this.view()?.zoomIn();
  }

  zoomOut(): void {
    this.view()?.zoomOut();
  }

  fit(): void {
    this.view()?.fit();
  }
}

/** A player's character (a creature of it too): the party, which the fog never hides. */
function isParty(t: { kind?: number; creatureId?: string }): boolean {
  return t.kind === CharacterKind.PLAYER || !!t.creatureId;
}

function isNpc(t: { kind?: number; creatureId?: string }): boolean {
  return (
    !t.creatureId &&
    t.kind !== undefined &&
    t.kind !== CharacterKind.PLAYER &&
    t.kind !== CharacterKind.UNSPECIFIED
  );
}
