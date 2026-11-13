import type { Square } from './combat-grid';

/** Painting the sun on the map (#704): what the master's strokes add or remove, and the words that say it. Pure, no DOM. */
export type PaintTool = 'paint' | 'erase';

/** What a stroke changes: the squares that gain the sun and the ones that lose it. */
export interface SunChange {
  readonly addSquares: readonly Square[];
  readonly removeSquares: readonly Square[];
}

const key = (s: Square): string => `${s.col},${s.row}`;

/** "1 quadrado" / "5 quadrados". */
export function squaresCount(n: number): string {
  return `${n} ${n === 1 ? 'quadrado' : 'quadrados'}`;
}

/** The status line while painting: "Pintando luz do sol · 3 quadrados". */
export function paintStatus(n: number): string {
  return `Pintando luz do sol · ${squaresCount(n)}`;
}

/** What the keyboard cursor announces: "Coluna 3, linha 5: luz do sol" (squares counted from 1, as the master counts them). */
export function cursorNote(at: Square, lit: boolean): string {
  return `Coluna ${at.col + 1}, linha ${at.row + 1}: ${lit ? 'luz do sol' : 'sem luz do sol'}`;
}

/** The squares on the straight line from `a` to `b`, both included, so a fast drag leaves no gap. */
export function lineSquares(a: Square, b: Square): Square[] {
  const out: Square[] = [];
  const dc = Math.abs(b.col - a.col);
  const dr = Math.abs(b.row - a.row);
  const sc = a.col < b.col ? 1 : -1;
  const sr = a.row < b.row ? 1 : -1;
  let err = dc - dr;
  let { col, row } = a;
  for (;;) {
    out.push({ col, row });
    if (col === b.col && row === b.row) {
      return out;
    }
    const e2 = 2 * err;
    if (e2 > -dr) {
      err -= dr;
      col += sc;
    }
    if (e2 < dc) {
      err += dc;
      row += sr;
    }
  }
}

/** The change a stroke makes against the squares already lit: only what really changes, each square once. */
export function strokeChange(
  lit: readonly Square[],
  tool: PaintTool,
  touched: readonly Square[],
): SunChange {
  const on = new Set(lit.map(key));
  const seen = new Set<string>();
  const picked: Square[] = [];
  for (const s of touched) {
    const k = key(s);
    if (!seen.has(k) && on.has(k) === (tool === 'erase')) {
      seen.add(k);
      picked.push({ col: s.col, row: s.row });
    }
  }
  return tool === 'paint'
    ? { addSquares: picked, removeSquares: [] }
    : { addSquares: [], removeSquares: picked };
}

/** What the map shows while a stroke is under the pointer: the lit squares with the stroke's change applied (the server's answer replaces it). */
export function withStroke(
  lit: readonly Square[],
  tool: PaintTool,
  touched: readonly Square[],
): Square[] {
  const change = strokeChange(lit, tool, touched);
  if (tool === 'paint') {
    return [...lit, ...change.addSquares];
  }
  const gone = new Set(change.removeSquares.map(key));
  return lit.filter((s) => !gone.has(key(s)));
}

/**
 * Strokes in the order they come, sent one call at a time: what arrives while a call is in the air is merged (a square added and
 * then erased cancels out) and sent when it returns. A call that fails drops what it carried; the screen says why and shows the
 * server's squares.
 */
export class PaintQueue {
  private add = new Map<string, Square>();
  private remove = new Map<string, Square>();
  private flying = false;

  constructor(private readonly send: (change: SunChange) => Promise<boolean>) {}

  push(change: SunChange): Promise<void> {
    for (const s of change.addSquares) {
      this.remove.delete(key(s));
      this.add.set(key(s), s);
    }
    for (const s of change.removeSquares) {
      this.add.delete(key(s));
      this.remove.set(key(s), s);
    }
    return this.flush();
  }

  private async flush(): Promise<void> {
    if (this.flying) {
      return;
    }
    this.flying = true;
    try {
      while (this.add.size + this.remove.size > 0) {
        const change = {
          addSquares: [...this.add.values()],
          removeSquares: [...this.remove.values()],
        };
        this.add = new Map();
        this.remove = new Map();
        await this.send(change);
      }
    } finally {
      this.flying = false;
    }
  }
}
