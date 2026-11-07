import { computed } from '@angular/core';

import type { Combatant } from '../../../gen/meurpg/play/v1/combat_pb';
import { type HitBox, tappedCreature } from './pick-hit';
import { type Place, type PickPlan, planPick } from './pick-places';

/** A creature of the map while the player chooses a target (C7-04), or a candidate of the master's opportunity offer (`mode: 'offer'`): ringed and tappable, or dimmed with the reason a tap tells. */
export interface PickMark {
  readonly id: string;
  readonly label: string;
  /** Why it cannot be the target ("cobertura total"); `''` for a ringed one. */
  readonly reason: string;
  /** A valid target with a catch, written over it ("Longe: desvantagem"). */
  readonly note: string;
  /** The marks it has: the chosen target is 1, a missile's darts are as many as it took. */
  readonly count: number;
  /** What the marks are: the one target ("Alvo"), darts ("×2"), or the master's candidates for an opportunity offer (a dashed ring, the chosen one solid). */
  readonly mode: 'target' | 'darts' | 'offer';
}

/** The chip over the map that says why a creature cannot be the target. */
export interface PickNote {
  readonly id: string;
  readonly text: string;
}

/** The room the chips and tags take, in px (12 and 13px bold type): the check mark and "Alvo", "×2", a word of "Longe", the reason's icon and text. */
const MARK_WIDTH = 62;
const DART_WIDTH = 36;
/** The check mark and "Oferta": two letters more than "Alvo". */
const OFFER_WIDTH = 76;
const CHAR_WIDTH = 7;
const FAR_PADDING = 18;
const WHY_PADDING = 44;
const WHY_HEIGHT = 30;
const HALF = 2;
const PERCENT = 100;
/** How near a side edge (a share of the map's width) the "Vez" word leans inward. */
const TURN_LEAN = 0.08;
/** A map this wide (px) has room for the whole "Longe: desvantagem" when it is the only one. */
const WIDE_MAP = 600;

function markWidth(mode: PickMark['mode']): number {
  return mode === 'darts' ? DART_WIDTH : mode === 'offer' ? OFFER_WIDTH : MARK_WIDTH;
}

/** What the map reads to draw the choice of a target: the marks by creature, the dimmed ones, and where each tag and the reason chip go. */
export class PickLayer {
  private readonly byId = computed(() => new Map((this.marks() ?? []).map((m) => [m.id, m])));
  private readonly noted = computed(() => (this.marks() ?? []).filter((m) => m.note).length);
  /** The chip's text and the creature it is about. */
  readonly note = computed(() => {
    const note = this.chip();
    const c = note ? this.shown().find((x) => x.id === note.id) : undefined;
    return note && c ? { text: note.text, c } : null;
  });
  private readonly plan = computed<PickPlan>(() => {
    const unit = this.unit();
    const { columns, rows } = this.grid();
    const whole = this.noted() === 1 && unit * columns >= WIDE_MAP;
    const tags = this.shown()
      .map((c) => this.byId().get(c.id))
      .filter((m): m is PickMark => !!m && (m.count > 0 || !!m.note))
      .map((m) => {
        const far = this.farWidth(m.note);
        return {
          id: m.id,
          mark: m.count > 0 ? markWidth(m.mode) : 0,
          far: whole ? far : this.farWidth(this.far(m)),
          farShort: this.farWidth(this.far(m)),
          hasFar: !!m.note && far > 0,
        };
      });
    const note = this.note();
    return planPick({
      unit,
      columns,
      rows,
      tokens: this.shown().map((c) => ({ id: c.id, col: c.col, row: c.row, span: this.span(c) })),
      tags,
      why: note
        ? { id: note.c.id, w: note.text.length * CHAR_WIDTH + WHY_PADDING, h: WHY_HEIGHT }
        : null,
    });
  });

  constructor(
    private readonly marks: () => readonly PickMark[] | null,
    private readonly chip: () => PickNote | null,
    private readonly shown: () => readonly Combatant[],
    private readonly grid: () => { readonly columns: number; readonly rows: number },
    private readonly unit: () => number,
    private readonly span: (c: Combatant) => number,
  ) {}

  /** Tells `emit` the creature a click on the button of `id` meant, when it is one on offer (see `tappedCreature`). */
  tapped(
    event: MouseEvent,
    id: string,
    boxes: readonly HitBox[],
    emit: (hit: string) => void,
  ): void {
    const hit = tappedCreature(event, id, boxes, this.unit());
    if (hit && this.byId().has(hit)) {
      emit(hit);
    }
  }

  of(id: string): PickMark | undefined {
    return this.byId().get(id);
  }

  /** Dimmed while the player chooses: a creature the action cannot aim at. */
  dim(id: string): boolean {
    return !!this.byId().get(id)?.reason;
  }

  /** The button's name: "Escolher Goblin 1 como alvo", or why the creature cannot be one. */
  name(m: PickMark): string {
    if (m.reason) {
      return `${m.label}: ${m.reason}`;
    }
    if (m.mode === 'offer') {
      return `Oferecer a ${m.label}`;
    }
    const base =
      m.mode === 'darts' && m.count > 0
        ? `Pôr outro dardo em ${m.label} (${m.count} agora)`
        : `Escolher ${m.label} como alvo`;
    return m.note ? `${base}. ${m.note}` : base;
  }

  /** The "Longe" of "Longe: desvantagem". */
  far(m: PickMark): string {
    return m.note.split(':')[0];
  }

  /** The rest of a lone chip on a wide map (": desvantagem"); never on a narrow one, nor with several chips. */
  farRest(m: PickMark, place: Place | undefined): string {
    return !place || place.short || this.noted() > 1 ? '' : m.note.slice(this.far(m).length);
  }

  private farWidth(text: string): number {
    return text ? text.length * CHAR_WIDTH + FAR_PADDING : 0;
  }

  /** Where a token's tags hang (px from the map's top left), or `undefined` for a creature with none. */
  tagsAt(id: string): Place | undefined {
    return this.plan().tags.get(id);
  }

  /** Where the reason chip goes, or `null` where no free place is left (the bar's hint line says it). */
  whyAt(): Place | null {
    return this.plan().why;
  }

  /** A place's left and top as the map's own percentages (the token's centre until the map is measured). */
  left(c: Combatant, place: Place): number {
    const { columns } = this.grid();
    return this.unit() > 0
      ? (place.x / (columns * this.unit())) * PERCENT
      : ((c.col + this.span(c) / HALF) / columns) * PERCENT;
  }

  top(c: Combatant, place: Place): number {
    const { rows } = this.grid();
    return this.unit() > 0
      ? (place.y / (rows * this.unit())) * PERCENT
      : ((c.row + this.span(c) / HALF) / rows) * PERCENT;
  }

  /** A "Vez" word above or below a token at a side edge leans inward, so the map does not cut it. */
  turnLean(c: Combatant): string {
    const { columns } = this.grid();
    const x = (c.col + 1 / HALF) / columns;
    return x < TURN_LEAN
      ? ' cm__turn--lean-left'
      : x > 1 - TURN_LEAN
        ? ' cm__turn--lean-right'
        : '';
  }
}
