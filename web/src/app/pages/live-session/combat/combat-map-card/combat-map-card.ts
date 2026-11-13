import { Component, computed, effect, input, model, output, signal } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import {
  CombatantKind,
  type Encounter,
  SunlightScope,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { PHONE_QUERY, mediaQuery } from '../../../../shared/map-view/media-query';
import { ownCombatant } from '../../../../core/combat/combat-view';
import { metersFixed } from '../../../../core/units';
import { fullscreenOfHost } from '../../../../core/ui/fullscreen';
import { type DoorSquare, type MapLayers, NO_LAYERS } from '../../../../core/maps/layers';
import {
  CombatMap,
  type CombatMapImage,
  type OfferMark,
  type PickMark,
  type PickNote,
  type Reach,
  type TokenDrop,
} from '../../../../shared/combat-map/combat-map';
import { CombatantToken } from '../../../../shared/combatant-token/combatant-token';
import { ZoneLayer } from '../../../../shared/combat-map/zone-layer/zone-layer';
import { ZoneLetter } from '../../../../shared/combat-map/zone-layer/zone-letter';
import { ZoneSwatch } from '../../../../shared/combat-map/zone-layer/zone-swatch';
import { SunlightLayer } from '../../../../shared/combat-map/sunlight-layer/sunlight-layer';
import { isTheatre } from '../../../../core/combat/theatre';
import type { PaintTool, SunChange } from '../../../../core/combat/sunlight-paint';
import { SunlightControls } from './sunlight-controls';
import { zoneLegend } from '../../../../core/combat/zone-pattern';
import { zoneViews } from '../../../../core/combat/zones';
import { AreaOverlay } from '../../../../shared/area-picker/area-overlay';
import { lastArea } from '../../../../core/combat/hidden-reveal';
import { MapLayersLegend } from '../../../../shared/map-layers/map-layers-legend';
import { pickWindow, turnWindow } from './pick-window';
import {
  type Vision,
  knownWindow,
  tileProgress,
  tileRects,
  visionLegend,
} from '../../../../core/maps/vision';

/**
 * The map panel of a running combat or of its setup (E6-04, E6-05, E6-11):
 * the battle map with its legend. The master drags any token ("O mestre anda
 * sem limite"); a player drags their own on their turn, which opens the "Mover" page on that square (the page asks, with its warnings; a drop never moves).
 * The legend names every shape the map uses, so no meaning is colour alone: the
 * layer marks the map has (`app-map-layers-legend`), then the tokens and, when
 * they are drawn, the movement marks and the square of an opportunity attack's
 * reactor. The master can turn the reach of whoever is on turn on and off
 * ("Mostrar o alcance do Toren no mapa", E9-05): the server's `GetMoveOptions`
 * answer, drawn as it comes. "Movimento forçado" (a teleport, a push or a pull)
 * is a box under it that makes the next drag skip the opportunity attacks.
 */
/** The squares kept around what the player knows when the card crops the map, and the share of the grid above which it does not. */
const CROP_MARGIN = 2;
const CROP_MAX_SHARE = 0.6;

@Component({
  selector: 'app-combat-map-card',
  imports: [
    AreaOverlay,
    CombatMap,
    CombatantToken,
    MapLayersLegend,
    MatButtonModule,
    MatIconModule,
    SunlightControls,
    SunlightLayer,
    ZoneLayer,
    ZoneLetter,
    ZoneSwatch,
  ],
  templateUrl: './combat-map-card.html',
  styleUrl: './combat-map-card.scss',
})
export class CombatMapCard {
  readonly encounter = input.required<Encounter>();
  readonly image = input.required<CombatMapImage>();
  readonly mapName = input('');
  readonly isMaster = input(false);
  /** The panel's title: "Mapa" for the master, the map's name for a player. */
  readonly title = input('Mapa');
  readonly reach = input<Reach | null>(null);
  /** The map's painted layers, once read. */
  readonly layers = input<MapLayers | null>(null);
  /** What the player sees of the map with the fog on (MR-036): the same drawing as the session's, with the combatants over it. */
  readonly fog = input<Vision | null>(null);
  /** The reactors of pending opportunity offers the caller answers or waits on (E9-13). */
  readonly offers = input<readonly OfferMark[]>([]);
  /** The master's switch for the reach of whoever is on turn: its name, and whether it is on. */
  readonly reachSwitch = input<{ readonly name: string; readonly on: boolean } | null>(null);
  /** The master's "Movimento forçado" box is offered (a running combat on a map): the next drag skips the opportunity attacks. */
  readonly forcedSwitch = input(false);
  /** The box is on: the next drag is forced. */
  readonly forced = input(false);
  /** What the last forced drag did, in words, for the live region. */
  readonly forcedNote = input('');
  /** An NPC dragged past its movement: "Mover mesmo assim?" with how much is missing; `null` when nothing is asked. */
  readonly overAsk = input<{ readonly label: string; readonly missing: string } | null>(null);
  /** The player's own combatant may be dragged to pick a square (their turn, desktop): the drop opens "Mover" there. */
  readonly ownMovable = input(false);
  /** The legend of the shapes; the setup's small map says it in a note. */
  readonly legend = input(true);
  /** A line under the map instead of the legend. */
  readonly note = input('');
  /** On a phone the master gets a zoomed, cropped preview centered on whoever
   * is on turn, and "Ver mapa" for the whole map (E6-12). */
  readonly cropOnPhone = input(false);
  /** The player's own turn (C4-03, C6-07): no head and no "Tela cheia"; the map's name is a chip and "Ver mapa" an icon over the map's top on a phone (a row above it from 768px), and what the page projects as `mapFoot` (the movement row) closes the card. */
  readonly compact = input(false);
  /** The player chooses a target on the map (C7-04, C7-05), or the master ringed the candidates of an opportunity offer (`mode: 'offer'`): the creatures offered are ringed buttons, the card has the accent border and, on a phone, the map is the part of it that holds them. */
  readonly pick = input<readonly PickMark[] | null>(null);
  /** Why the creature just tapped cannot be the target. */
  readonly pickNote = input<PickNote | null>(null);
  /** The master acts for this character (C8-01): the head says whose target is asked ("Toque no alvo · por Pensantus"), and "Mover" is in it while no target is. */
  readonly actingFor = input('');

  /** The master taps a door of the map (E10-05 10): the page opens the door's sheet. Not on the phone's cropped preview. */
  readonly doorPick = output<DoorSquare>();
  /** Paint mode of the sun (#704): the page keeps it so the bar's "Próximo turno" waits until "Pronto". */
  readonly painting = model(false);
  /** The master asked for another scope, or painted or erased squares: the page sends it. */
  readonly sunlightChange = output<{
    scope?: SunlightScope;
    addSquares?: readonly { col: number; row: number }[];
    removeSquares?: readonly { col: number; row: number }[];
  }>();
  readonly doorTaps = input(false);
  readonly tokenDrop = output<TokenDrop>();
  /** The master turned the reach on or off. */
  readonly reachChange = output<boolean>();
  /** The master turned "Movimento forçado" on or off. */
  readonly forcedChange = output<boolean>();
  readonly overAskConfirm = output<void>();
  readonly overAskCancel = output<void>();
  /** "Ver mapa": the player's full-screen, read-only map. */
  readonly openMap = output<void>();
  /** A creature offered as a target was tapped. */
  readonly pickTap = output<string>();
  /** "Mover" in the master's head, while he acts for a character. */
  readonly move = output<void>();

  private readonly phone = mediaQuery(PHONE_QUERY);
  private readonly settled = signal<ReadonlySet<string>>(new Set());
  protected readonly fogProgress = computed(() => {
    const v = this.fog();
    return v ? tileProgress(tileRects(v), this.settled()) : { total: 0, done: 0 };
  });
  protected readonly fogLoading = signal(false);
  protected readonly fogLegend = computed(() => {
    const v = this.fog();
    return v ? visionLegend(v) : null;
  });
  constructor() {
    // Paint mode ends with the sun, with the scope "Todo o mapa" or when the controls go.
    effect(() => {
      if (this.painting() && !this.sunSquares()) {
        this.painting.set(false);
      }
    });
  }

  protected onFogLoading(loading: boolean): void {
    this.fogLoading.set(loading);
  }

  protected onFogSettled(set: ReadonlySet<string>): void {
    this.settled.set(set);
  }
  /** "Tela cheia": the browser's full screen for the card (the map with its switches), for a table that shows the
   * combat on a big screen. */
  protected readonly fullscreen = fullscreenOfHost();
  /** The map's width over its height, so on full screen the map is as wide as the screen's height lets it be. */
  protected readonly mapAspect = computed(() => {
    const ratio = this.crop()?.ratio;
    if (ratio) {
      const [w, h] = ratio.split('/').map(Number);
      return w / Math.max(h, 1);
    }
    const { width, height } = this.image();
    return width / Math.max(1, height);
  });
  /** With the fog on, the map card shows the part of the grid the player knows (two squares around it), so the tokens are
   * readable when the explored area is a small part of the grid; "Ver mapa" has the whole map. */
  protected readonly crop = computed(() => {
    const vision = this.fog();
    const e = this.encounter();
    const columns = vision?.columns ?? e.gridColumns;
    const rows = vision?.rows ?? e.gridRows;
    const win = this.cropWindow(vision, columns, rows);
    if (!win) {
      return null;
    }
    const { width, height } = this.image();
    // The window is as wide as it is tall at least (a tall strip of corridor would be a tall card), and never wider than the grid.
    const cols = Math.min(columns, Math.max(win.cols, win.rows));
    const col = Math.min(Math.max(0, win.col - Math.floor((cols - win.cols) / 2)), columns - cols);
    return {
      // The inner map is `columns / cols` times the card's width; the shift is a share of the inner map.
      width: (columns / cols) * 100,
      shift: `translate(${(-col / columns) * 100}%, ${(-win.row / rows) * 100}%)`,
      ratio: `${(cols * width) / columns} / ${(win.rows * height) / rows}`,
    };
  });

  /** The squares the card shows: what the fog lets the player know, or, while they choose a target on a phone, the part of the map
   * that holds them and the creatures offered. */
  private cropWindow(vision: Vision | null, columns: number, rows: number) {
    const e = this.encounter();
    if (vision && !this.isMaster()) {
      const known = knownWindow(
        vision,
        e.combatants.filter((c) => c.placed),
        CROP_MARGIN,
      );
      return known && known.cols * known.rows < CROP_MAX_SHARE * columns * rows ? known : null;
    }
    const offered = new Set((this.pick() ?? []).map((m) => m.id));
    const mine = this.own()?.id;
    const own = e.combatants.find((c) => c.id === mine && c.placed);
    if (this.phone() && this.compact() && offered.size === 0 && own) {
      // The player's own turn view: wide and short, around their token (C4-03).
      return turnWindow(
        own,
        e.combatants.filter((c) => c.placed && c.id !== mine),
        columns,
        rows,
      );
    }
    if (!this.phone() || offered.size === 0) {
      return null;
    }
    return pickWindow(
      // The master's offer rings the candidates: the mover (who is on turn, never "own" for the master) must stay in the window.
      e.combatants.filter(
        (c) =>
          c.placed &&
          (offered.has(c.id) ||
            c.id === mine ||
            (this.offerRing() && c.id === e.currentCombatantId)),
      ),
      columns,
      rows,
    );
  }

  /** The ringed ones are the master's candidates for an opportunity offer: the legend names the ring. */
  protected readonly offerRing = computed(() =>
    (this.pick() ?? []).some((m) => m.mode === 'offer'),
  );

  /** The phone's cropped preview; painting the sun needs the whole map, so the preview gives way while the master paints. */
  protected readonly preview = computed(
    () => this.cropOnPhone() && this.isMaster() && this.phone() && !this.pick() && !this.painting(),
  );

  /** Where the 2x map sits inside the preview frame, so that the one on turn
   * is in the middle (clamped, so no empty edge shows). The frame is 326 x 224
   * like the artboard: its height over the map's is `frameOverMap`. */
  protected readonly shift = computed(() => {
    const e = this.encounter();
    const focus =
      e.combatants.find((c) => c.id === e.currentCombatantId && c.placed) ??
      e.combatants.find((c) => c.placed);
    const fx = focus ? (focus.col + 0.5) / e.gridColumns : 0.5;
    const fy = focus ? (focus.row + 0.5) / e.gridRows : 0.5;
    const ratio = this.image().width / Math.max(1, this.image().height);
    const frameOverMap = ratio / 2 / (326 / 224);
    const clamp = (v: number, lo: number, hi: number) => Math.min(hi, Math.max(lo, v));
    const x = clamp(-(fx - 0.25), -0.5, 0);
    const y = frameOverMap >= 1 ? 0 : clamp(-(fy - frameOverMap / 2), -(1 - frameOverMap), 0);
    return `translate(${x * 100}%, ${y * 100}%)`;
  });

  protected readonly own = computed(() => ownCombatant(this.encounter()));
  protected readonly size = computed(() => {
    const e = this.encounter();
    // The master's head is one line (C7-02): "24 × 16 · 1,5 m".
    return this.isMaster()
      ? `${e.gridColumns} × ${e.gridRows} · 1,5 m`
      : `${e.gridColumns} × ${e.gridRows} quadrados de 1,5 m`;
  });
  /** "Área da última magia": the area a player's spell reached, while its question waits (the master's map only). */
  protected readonly lastArea = computed(() =>
    this.isMaster() ? lastArea(this.encounter()) : null,
  );
  /** The zones the spells keep, as the server sent them for whoever looks (RN-37): drawn under the tokens and named in the legend. */
  protected readonly zones = computed(() => zoneViews(this.encounter(), this.isMaster()));
  protected readonly zoneLegend = computed(() => zoneLegend(this.zones()));
  /** The sun controls: the sun is on, the viewer is the master and the combat has a map (on the phone's cropped preview they are there too: "Pintar" swaps the preview for the whole map). */
  protected readonly sunControls = computed(
    () =>
      this.isMaster() &&
      this.encounter().sunlightOn &&
      !isTheatre(this.encounter()) &&
      !this.compact(),
  );
  protected readonly sunSquares = computed(
    () => this.sunControls() && this.encounter().sunlightScope === SunlightScope.SQUARES,
  );
  protected readonly sunWhole = computed(
    () => this.isMaster() && this.encounter().sunlightOn && !this.sunSquares(),
  );
  protected readonly tool = signal<PaintTool>('paint');
  protected paint(change: SunChange): void {
    this.sunlightChange.emit(change);
  }
  protected readonly emptyLayers = NO_LAYERS;
  protected readonly reachMeters = computed(() => metersFixed((this.reach()?.leftDft ?? 0) / 10));
  protected readonly hasHidden = computed(() => this.encounter().combatants.some((c) => c.hidden));
  /** A player's creature has a square on the map: the legend names the dashed round token. */
  protected readonly hasCreatures = computed(() =>
    this.encounter().combatants.some((c) => c.kind === CombatantKind.CREATURE && c.placed),
  );
  protected readonly hasDefeated = computed(() =>
    this.encounter().combatants.some((c) => c.defeated),
  );
}
