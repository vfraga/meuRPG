import { ChangeDetectionStrategy, Component, computed, input, output } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import type { WallSide } from '../../../gen/meurpg/play/v1/combat_pb';
import { DIRECTIONS, type Direction, directionWord } from '../../core/combat/spell-area';
import {
  type WallChoice,
  lengthNote,
  lengthText,
  maxLengthFt,
  minLengthFt,
  sideChoices,
  sideNote,
  sidePicked,
  stepFt,
  reorient,
  stepLength,
  switchShape,
} from '../../core/combat/spell-wall';
import { metersText } from '../../core/units';

let nextId = 0;

/** The icons of the eight directions, in the order of `DIRECTIONS` (east first, clockwise). */
const ARROWS = [
  'east',
  'south_east',
  'south',
  'south_west',
  'west',
  'north_west',
  'north',
  'north_east',
];
/** The cells of the direction pad, row by row: the eight directions around the middle (`null`). */
const PAD: readonly (Direction | null)[] = [
  { dx: -1, dy: -1 },
  { dx: 0, dy: -1 },
  { dx: 1, dy: -1 },
  { dx: -1, dy: 0 },
  null,
  { dx: 1, dy: 0 },
  { dx: -1, dy: 1 },
  { dx: 0, dy: 1 },
  { dx: 1, dy: 1 },
];

/**
 * "Onde fica" of a spell that lays a wall (Wall of Fire): the shape, the direction of a line, the length (or the diameter of a
 * ring) a square at a time and the side that burns. It draws nothing on the map and asks nothing: it reports the wall it
 * would now be (`changed`) and the page sends it to the server. The direction pad is the keyboard's way to what a drag on the
 * map does. `part` splits it around the map on a phone: the shape above it, the rest below.
 */
@Component({
  selector: 'app-wall-controls',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [MatIconModule],
  templateUrl: './wall-controls.html',
  styleUrl: './wall-controls.scss',
})
export class WallControls {
  readonly wall = input.required<WallChoice>();
  /** The dice of the burning side at the slot ("5d8"); empty when the details are not read yet. */
  readonly dice = input('');
  readonly part = input<'shape' | 'size'>('size');
  /** The preview is on its way. */
  readonly loading = input(false);
  /** "A muralha para na parede: ...", or `null`. */
  readonly stops = input<string | null>(null);
  readonly disabled = input(false);
  readonly changed = output<WallChoice>();

  protected readonly id = `wall-controls-${nextId++}`;
  protected readonly pad = PAD.map((d) => (d ? this.cell(d) : null));
  protected readonly sides = computed(() => sideChoices(this.wall()));
  protected readonly picked = computed(() => sidePicked(this.wall()));
  protected readonly lengthText = computed(() => lengthText(this.wall()));
  protected readonly lengthNote = computed(() => lengthNote(this.wall()));
  protected readonly sideNote = computed(() => sideNote(this.wall(), this.dice()));
  protected readonly noun = computed(() => (this.wall().ring ? 'diâmetro' : 'comprimento'));
  protected readonly atLeast = computed(
    () => this.wall().lengthFt <= minLengthFt(this.wall().ring),
  );
  protected readonly atMost = computed(() => this.wall().lengthFt >= maxLengthFt(this.wall().ring));
  protected readonly step = computed(() =>
    metersText(stepFt(this.wall().ring)).replace(/\u00a0/g, ' '),
  );
  /** The group's name carries the way the wall runs now ("Direção da muralha: leste"). */
  protected readonly padLabel = computed(() => {
    const d = this.wall().direction;
    return d ? `Direção da muralha: ${directionWord(d)}` : 'Direção da muralha';
  });

  private cell(direction: Direction) {
    const i = DIRECTIONS.findIndex((d) => d.dx === direction.dx && d.dy === direction.dy);
    return { direction, icon: ARROWS[i], word: directionWord(direction) };
  }

  protected chosen(d: Direction): boolean {
    const w = this.wall().direction;
    return !!w && w.dx === d.dx && w.dy === d.dy;
  }

  protected shape(ring: boolean): void {
    if (!this.disabled() && ring !== this.wall().ring) {
      this.changed.emit(switchShape(this.wall(), ring));
    }
  }

  protected turn(direction: Direction): void {
    if (!this.disabled() && !this.chosen(direction)) {
      this.changed.emit(reorient(this.wall(), direction));
    }
  }

  protected longer(squares: number): void {
    const next = stepLength(this.wall(), squares);
    if (!this.disabled() && next.lengthFt !== this.wall().lengthFt) {
      this.changed.emit(next);
    }
  }

  protected side(side: WallSide): void {
    if (!this.disabled() && side !== this.wall().side) {
      this.changed.emit({ ...this.wall(), side });
    }
  }
}
