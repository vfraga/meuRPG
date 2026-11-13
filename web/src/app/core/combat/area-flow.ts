import { computed, signal } from '@angular/core';

import {
  AreaPlacement,
  type PreviewSpellAreaResponse,
} from '../../../gen/meurpg/play/v1/combat_pb';
import type { AreaChoice } from './combat-client-types';
import { combatErrorMessage, encounterBlocked } from './combat-errors';
import type { Square } from './combat-grid';
import { type Direction, sameSquare } from './spell-area';
import { type WallChoice, askable, newWall, reorient, sidePicked, stopsText } from './spell-wall';

/** Where the caster put the area: a point (a sphere, a cylinder) or a direction (a cone, a line, a cube). */
export type AreaPick =
  | { readonly kind: 'point'; readonly square: Square }
  | { readonly kind: 'direction'; readonly direction: Direction };

/** Whether two picks are the same place: a second tap on it confirms. */
export function samePick(a: AreaPick | null, b: AreaPick | null): boolean {
  if (!a || !b) {
    return false;
  }
  if (a.kind === 'point' && b.kind === 'point') {
    return sameSquare(a.square, b.square);
  }
  return (
    a.kind === 'direction' &&
    b.kind === 'direction' &&
    a.direction.dx === b.direction.dx &&
    a.direction.dy === b.direction.dy
  );
}

/** The area of a pick, as CastSpell and PreviewSpellArea take it. */
export function choiceOf(pick: AreaPick | null): AreaChoice | null {
  if (!pick) {
    return null;
  }
  return pick.kind === 'point' ? { origin: pick.square } : { direction: pick.direction };
}

/**
 * The two steps of placing an area (PM-02a, PM-02b, PM-02d), shared by the player's cast sheet and the master's dialog.
 *
 * - Step 1 ("Onde ela explode", "Para onde?"): the browser draws the outline while the point moves, and asks nothing. Once a
 *   point or a direction is placed (a tap, Enter, a pick in "Centrar em…"), the server is asked who is inside
 *   (`PreviewSpellArea`, read-only): its squares replace the outline, its point is the effective one (the near side of a
 *   wall), and the warnings come from its list. Only the answer for the latest place counts.
 * - Confirming opens step 2 ("Quem está na área") with that answer; "Mudar o local" goes back with the point where it was.
 *
 * Nothing is spent until the cast itself, and the server works the area out again then. A sphere centered on the caster has
 * nothing to place: `start()` goes straight to its list.
 */
export class AreaFlow {
  readonly step = signal<'place' | 'list'>('place');
  /** The point or direction placed on the map (the filled template), or `null`. */
  readonly placed = signal<AreaPick | null>(null);
  /** The server's answer for the placed area. */
  readonly preview = signal<PreviewSpellAreaResponse | null>(null);
  /** A preview is on its way. */
  readonly loading = signal(false);
  /** "Confirmar local" waits for the preview. */
  readonly busy = signal(false);
  readonly error = signal('');
  /** The wall being laid (Wall of Fire: shape, direction, length, side), sent with every preview and with the cast; `null`
   * for any other area. It changes only while the map step is open. */
  readonly wall = signal<WallChoice | null>(null);
  /** "Conjurar mesmo assim" was chosen for an area with nobody the caster sees (a spell that still rolls a pool asks it first). */
  readonly nobodyAccepted = signal(false);

  private seq = 0;
  private pending: Promise<PreviewSpellAreaResponse | null> | null = null;

  /** Nobody the caller sees is inside: step 2 asks before casting. */
  readonly nobody = computed(() => {
    const p = this.preview();
    return this.step() === 'list' && !!p && p.targets.length === 0;
  });

  /** Whether "Confirmar local" can be pressed. */
  readonly ready = computed(() => {
    const wall = this.wall();
    if (wall) {
      return this.placed() !== null && sidePicked(wall);
    }
    return this.placement === AreaPlacement.CASTER || this.placed() !== null;
  });

  /** "A muralha para na parede: ficou com 9 m (6 quadrados) em vez de 12 m.", when the answer for this wall is shorter than the
   * open floor; `null` otherwise. */
  readonly stops = computed(() => {
    const wall = this.wall();
    const p = this.preview();
    return wall && p && this.placed() ? stopsText(wall, p.squares.length) : null;
  });

  /** The area to send with the cast: the server's effective point when the preview moved it (a wall), so the cast is what the
   * caster saw; the direction as placed. `null` for a sphere on the caster. */
  readonly choice = computed<AreaChoice | null>(() => {
    const pick = this.placed();
    const origin = this.preview()?.origin;
    if (pick?.kind === 'point' && origin) {
      return { origin: { col: origin.col, row: origin.row } };
    }
    return choiceOf(pick);
  });

  constructor(
    readonly placement: AreaPlacement,
    private readonly ask: (
      area: AreaChoice | null,
      wall: WallChoice | null,
    ) => Promise<PreviewSpellAreaResponse>,
    laysWall = false,
  ) {
    if (laysWall) {
      this.wall.set(newWall(false));
    }
  }

  /** A sphere around the caster: nothing to place, the list at once. */
  async start(): Promise<void> {
    if (this.placement === AreaPlacement.CASTER) {
      await this.confirm();
    }
  }

  /** A new place (or none): the old answer is dropped and the new one asked for. */
  place(pick: AreaPick | null): void {
    if (this.step() !== 'place') {
      return;
    }
    this.placed.set(pick);
    this.preview.set(null);
    this.error.set('');
    if (pick) {
      void this.load();
    } else {
      this.seq++;
      this.loading.set(false);
    }
  }

  /** The wall changed (shape, direction, length or side): the answer is asked again when a point is placed. */
  setWall(wall: WallChoice): void {
    if (this.step() !== 'place' || !this.wall()) {
      return;
    }
    this.wall.set(wall);
    this.preview.set(null);
    this.error.set('');
    if (this.placed()) {
      void this.load();
    }
  }

  /** A drag on the map: the start square and the way it runs, with one question to the server. */
  placeWall(square: Square, direction: Direction): void {
    const wall = this.wall();
    if (!wall || wall.ring || this.step() !== 'place') {
      return;
    }
    this.wall.set(reorient(wall, direction));
    this.place({ kind: 'point', square });
  }

  private wallAsked(): WallChoice | null {
    const wall = this.wall();
    return wall ? askable(wall) : null;
  }

  private load(): Promise<PreviewSpellAreaResponse | null> {
    const seq = ++this.seq;
    this.loading.set(true);
    const run = this.ask(choiceOf(this.placed()), this.wallAsked()).then(
      (res) => {
        if (seq === this.seq) {
          this.preview.set(res);
        }
        return seq === this.seq ? res : null;
      },
      (err: unknown) => {
        if (seq === this.seq) {
          // A point the server refuses (out of range) is placed again by the caster, as a tap outside the range is.
          if (encounterBlocked(err)) {
            this.placed.set(null);
          }
          this.error.set(combatErrorMessage(err, 'ver quem está na área'));
        }
        return null;
      },
    );
    this.pending = run.finally(() => {
      if (seq === this.seq) {
        this.loading.set(false);
      }
    });
    return this.pending;
  }

  /** "Confirmar local" (or the second tap): the list of who is inside, once the server answered for this place. */
  async confirm(): Promise<boolean> {
    if (this.busy() || !this.ready() || this.step() !== 'place') {
      return false;
    }
    let res = this.preview();
    if (!res) {
      this.busy.set(true);
      this.error.set('');
      try {
        res = await (this.loading() && this.pending ? this.pending : this.load());
      } finally {
        this.busy.set(false);
      }
    }
    if (!res || this.preview() !== res) {
      return false;
    }
    this.nobodyAccepted.set(false);
    this.step.set('list');
    return true;
  }

  /** "Mudar o local": back to the map, the point and its answer where they were; nothing was spent. */
  back(): void {
    if (this.busy()) {
      return;
    }
    this.step.set('place');
    this.nobodyAccepted.set(false);
    this.error.set('');
  }
}
