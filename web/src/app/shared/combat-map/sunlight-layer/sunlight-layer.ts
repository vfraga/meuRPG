import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  afterNextRender,
  computed,
  effect,
  Injector,
  inject,
  input,
  output,
  signal,
} from '@angular/core';

import type { Square } from '../../../core/combat/combat-grid';
import { squareAt, stepSquare } from '../../../core/combat/combat-grid';
import { cellsPath, outlinePath, sameSquare } from '../../../core/combat/spell-area';
import {
  type PaintTool,
  type SunChange,
  cursorNote,
  lineSquares,
  strokeChange,
  withStroke,
} from '../../../core/combat/sunlight-paint';

let nextId = 0;

/** The light dots of the sunlit squares: one dot per tile of 0.2 x 0.2 squares, a light dot over a dark halo (the legend's swatch is the same in CSS). */
export const SUN_TILE = 0.2;
export const SUN_DOT = 0.032;
export const SUN_DOT_HALO = 0.045;

/**
 * The sun on the master's map (#704): the squares the master painted, drawn as light dots as a light dotted pattern with a dotted white edge (never the line patterns
 * of the spell zones), or, when the sun is over the whole map, a dotted frame around it. While `painting` it takes the pointer
 * over the map: a click or a drag adds (`paint`) or removes (`erase`) squares, the stroke is sent once, when the pointer is
 * released; the arrow keys move a cursor and Space applies the tool to its square, announced in a live region. It draws what the
 * server sent (`squares`) and previews only the stroke under the pointer.
 *
 * Two fingers scroll the map, one paints. The pad is `touch-action: pinch-zoom`: one finger never makes the browser scroll (so a
 * drag is a stroke), a pinch is still the browser's (it then sends `pointercancel` and the stroke is dropped). A second pointer
 * that goes down cancels the stroke and, as pointer events give no native two-finger pan on a surface that does not pan, the
 * layer scrolls the nearest scrollable ancestor (or the page) by the movement of the midpoint of the two fingers. Limits: the pan
 * is done by script (no momentum, no overscroll), and a finger left down after the other lifts paints nothing until it lifts too.
 */
@Component({
  selector: 'app-sunlight-layer',
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './sunlight-layer.html',
  styleUrl: './sunlight-layer.scss',
})
export class SunlightLayer {
  readonly columns = input.required<number>();
  readonly rows = input.required<number>();
  /** The squares the server says are in the sun. */
  readonly squares = input<readonly Square[]>([]);
  /** The sun is over the whole map: no squares, a frame. */
  readonly wholeMap = input(false);
  readonly painting = input(false);
  readonly tool = input<PaintTool>('paint');

  /** What a stroke or a key changed. */
  readonly paint = output<SunChange>();

  protected readonly uid = `sl-${nextId++}`;
  protected readonly tile = SUN_TILE;
  protected readonly dot = SUN_DOT;
  protected readonly dotHalo = SUN_DOT_HALO;
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private stroke: { pointer: number; last: Square; touched: Square[] } | null = null;
  /** Where each finger or pointer on the pad is; two or more means the map is being scrolled, not painted. */
  private readonly fingers = new Map<number, { x: number; y: number }>();
  private mid: { x: number; y: number } | null = null;
  private readonly touched = signal<readonly Square[]>([]);
  protected readonly cursor = signal<Square | null>(null);
  private readonly injector = inject(Injector);
  /** What the live region says: the cursor's square, and whether it is in the sun (after the server's answer). */
  protected readonly note = computed(() => {
    const at = this.cursor();
    return this.painting() && at ? cursorNote(at, this.lit(at)) : '';
  });

  /** The squares as drawn: the server's, with the stroke under the pointer previewed. */
  protected readonly shown = computed(() =>
    this.touched().length
      ? withStroke(this.squares(), this.tool(), this.touched())
      : this.squares(),
  );
  protected readonly fill = computed(() => cellsPath(this.shown()));
  protected readonly edge = computed(() => outlinePath(this.shown()));
  /** The squares under the pointer in this stroke, marked in the accent until the server answers. */
  protected readonly preview = computed(() =>
    this.touched().length ? cellsPath(this.touched()) : '',
  );

  constructor() {
    effect(() => {
      if (!this.painting()) {
        this.stroke = null;
        this.fingers.clear();
        this.mid = null;
        this.touched.set([]);
        this.cursor.set(null);
        return;
      }
      afterNextRender(
        () => this.host.nativeElement.querySelector<HTMLElement>('.sl__pad')?.focus(),
        { injector: this.injector },
      );
    });
  }

  private under(event: PointerEvent | MouseEvent): Square {
    const rect = this.host.nativeElement.getBoundingClientRect();
    return squareAt(
      (event.clientX - rect.left) / Math.max(1, rect.width),
      (event.clientY - rect.top) / Math.max(1, rect.height),
      this.columns(),
      this.rows(),
    );
  }

  protected down(event: PointerEvent): void {
    event.stopPropagation();
    if (event.button > 0) {
      return;
    }
    this.fingers.set(event.pointerId, { x: event.clientX, y: event.clientY });
    (event.currentTarget as HTMLElement).setPointerCapture?.(event.pointerId);
    if (this.fingers.size > 1) {
      // A second finger: no stroke, the map scrolls.
      this.stroke = null;
      this.touched.set([]);
      this.mid = this.midpoint();
      return;
    }
    const at = this.under(event);
    this.stroke = { pointer: event.pointerId, last: at, touched: [at] };
    this.touched.set([at]);
    this.cursor.set(at);
  }

  protected move(event: PointerEvent): void {
    event.stopPropagation();
    if (this.fingers.has(event.pointerId)) {
      this.fingers.set(event.pointerId, { x: event.clientX, y: event.clientY });
    }
    if (this.fingers.size > 1) {
      this.scrollBy();
      return;
    }
    const s = this.stroke;
    if (!s || s.pointer !== event.pointerId) {
      return;
    }
    const at = this.under(event);
    if (sameSquare(at, s.last)) {
      return;
    }
    s.touched.push(...lineSquares(s.last, at).slice(1));
    s.last = at;
    this.touched.set([...s.touched]);
  }

  protected up(event: PointerEvent): void {
    event.stopPropagation();
    const wasPanning = this.fingers.size > 1;
    this.fingers.delete(event.pointerId);
    this.mid = this.fingers.size > 1 ? this.midpoint() : null;
    const s = this.stroke;
    if (wasPanning || !s || s.pointer !== event.pointerId) {
      return;
    }
    this.stroke = null;
    this.touched.set([]);
    if (event.type === 'pointerup') {
      this.send(s.touched);
    }
  }

  private midpoint(): { x: number; y: number } {
    const all = [...this.fingers.values()];
    return {
      x: all.reduce((n, f) => n + f.x, 0) / all.length,
      y: all.reduce((n, f) => n + f.y, 0) / all.length,
    };
  }

  /** Two fingers: the map follows the midpoint, as a finger pans a page. */
  private scrollBy(): void {
    const now = this.midpoint();
    const before = this.mid ?? now;
    this.mid = now;
    const dx = before.x - now.x;
    const dy = before.y - now.y;
    let node: HTMLElement | null = this.host.nativeElement.parentElement;
    while (node) {
      const style = getComputedStyle(node);
      const x = /(auto|scroll)/.test(style.overflowX) && node.scrollWidth > node.clientWidth;
      const y = /(auto|scroll)/.test(style.overflowY) && node.scrollHeight > node.clientHeight;
      if (x || y) {
        node.scrollBy(x ? dx : 0, y ? dy : 0);
        return;
      }
      node = node.parentElement;
    }
    window.scrollBy(dx, dy);
  }

  protected key(event: KeyboardEvent): void {
    event.stopPropagation();
    const cols = this.columns();
    const rows = this.rows();
    if (event.key.startsWith('Arrow')) {
      event.preventDefault();
      const next = stepSquare(this.cursor() ?? { col: 0, row: 0 }, event.key, cols, rows);
      this.cursor.set(next);
    } else if (event.key === ' ' || event.key === 'Spacebar') {
      event.preventDefault();
      const at = this.cursor() ?? { col: 0, row: 0 };
      this.cursor.set(at);
      this.send([at]);
    }
  }

  protected focused(): void {
    this.cursor.update((at) => at ?? { col: 0, row: 0 });
  }

  private lit(at: Square): boolean {
    return this.squares().some((s) => sameSquare(s, at));
  }

  private send(touched: readonly Square[]): void {
    // The server's squares may be older than a stroke still in the air (a square painted and erased within one round trip), so
    // the stroke is sent whole, each square once; the server ignores what is already so.
    const change = strokeChange(this.tool() === 'erase' ? touched : [], this.tool(), touched);
    if (change.addSquares.length + change.removeSquares.length > 0) {
      this.paint.emit(change);
    }
  }
}
