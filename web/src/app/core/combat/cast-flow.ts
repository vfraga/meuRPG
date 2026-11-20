import {
  type DartsAtSlot,
  type SpellTargets,
  type TargetInReach,
} from '../../../gen/meurpg/play/v1/combat_pb';
import {
  type SlotChoice,
  type SpellDetails,
  SpellAttackType,
} from '../../../gen/meurpg/rules/v1/rules_pb';
import { metersFixed, metersText } from '../units';
import { joinDots, tight } from '../format/text';
import { circleLabel } from './combat-options';
import { listing } from './cover';
import { hpSpellKind } from './hp-effects';
import { stateWord } from './combat-view';
import type { SpellKind } from './cast-flow-text';

/**
 * The steps of the cast sheet (E6-09): the slot, the targets (and Magic
 * Missile's darts), what the cast did and the damage still to roll. The
 * sheet's logic is kept here as small pure functions, so the choices, the
 * counters and the result lines are tested without a DOM.
 */

export function spellKind(key: string, details: SpellDetails | null): SpellKind {
  if (key === 'spell:magic-missile') {
    return 'darts';
  }
  // The spells that read hit points: Sono and Leque Cromático roll a pool before
  // the cast, the others only name who they touch (E8-03).
  const hp = hpSpellKind(details);
  if (hp) {
    return hp === 'pool' ? 'pool' : 'hp';
  }
  if (!details) {
    return 'plain';
  }
  if (
    details.attackType === SpellAttackType.MELEE ||
    details.attackType === SpellAttackType.RANGED
  ) {
    return 'attack';
  }
  if (details.save) {
    return 'save';
  }
  return Object.keys(details.healBySlotLevel).length > 0 ? 'heal' : 'plain';
}

// The wording of the sheet and the rows of its result live in `cast-flow-text` and `cast-flow-rows`; the sheets read them here.
export { damageDice, damageText, castSubtitle, saveAbility } from './cast-flow-text';
export type { SpellKind } from './cast-flow-text';
export { castRows, dartLines, effectSentence, rollGroups, spentLine } from './cast-flow-rows';
export type { CastD20, CastRow } from './cast-flow-rows';

// ---- the slot ----

/** A row of the slot step: "1º nível", "1 livre de 4". */
export interface SlotRow {
  readonly level: number;
  readonly pact: boolean;
  readonly free: number;
  /** How many slots of this level the character has; `null` when unknown. */
  readonly total: number | null;
  readonly used: number;
  readonly enabled: boolean;
  readonly title: string;
  /** "1 livre de 4". */
  readonly count: string;
}

/** "1 livre de 4", "0 livres de 2"; without the total, "1 livre". */
export function freeText(free: number, total: number | null): string {
  const word = free === 1 ? 'livre' : 'livres';
  return total === null ? `${free} ${word}` : `${free} ${word} de ${total}`;
}

/**
 * The rows a spell of `level` can be cast with: every circle the character
 * has from the spell's own up (the ones with no free slot are listed too,
 * disabled, so "2º nível: Sem espaço livre" is seen), and the pact slots.
 * `choices` are the server's free slots (`SpellOption.slots`), who decides
 * what is enabled; `usage` has the totals, for "de 4". A cantrip (level 0)
 * uses no slot, so it has no rows at all.
 */
export function slotRows(
  level: number,
  choices: readonly SlotChoice[],
  usage: readonly SlotUsage[],
  pact: PactUsage | null = null,
): SlotRow[] {
  if (level < 1) {
    return [];
  }
  const rows = circleLevels(level, choices, usage).map((l) => circleRow(l, choices, usage));
  const pactRow = pactSlotRow(level, choices, pact);
  return pactRow ? [...rows, pactRow] : rows;
}

/** The slots a character has of one circle, as the usage reads them. */
interface SlotUsage {
  readonly level: number;
  readonly total: number;
  readonly used: number;
}

/** The pact slots, all of one level. */
interface PactUsage {
  readonly slotLevel: number;
  readonly total: number;
  readonly used: number;
}

/** The circles listed: the ones the server offers and the ones the character has, from the spell's own up. */
function circleLevels(
  level: number,
  choices: readonly SlotChoice[],
  usage: readonly SlotUsage[],
): number[] {
  const levels = new Set<number>(choices.filter((c) => !c.pact).map((c) => c.level));
  for (const u of usage) {
    if (u.level >= level) {
      levels.add(u.level);
    }
  }
  return [...levels].sort((a, b) => a - b);
}

function circleRow(
  l: number,
  choices: readonly SlotChoice[],
  usage: readonly SlotUsage[],
): SlotRow {
  const choice = choices.find((c) => c.level === l && !c.pact);
  const u = usage.find((x) => x.level === l);
  const free = choice?.free ?? (u ? u.total - u.used : 0);
  return row(l, false, free, u?.total ?? null, u?.used ?? 0, !!choice && choice.free > 0);
}

/** The pact row: the server's own slot, or the character's pact slots when they reach the spell's level. */
function pactSlotRow(
  level: number,
  choices: readonly SlotChoice[],
  pact: PactUsage | null,
): SlotRow | null {
  const choice = choices.find((c) => c.pact);
  if (choice) {
    return row(
      choice.level,
      true,
      choice.free,
      pact?.total ?? null,
      pact?.used ?? 0,
      choice.free > 0,
    );
  }
  if (!pact || pact.slotLevel < level) {
    return null;
  }
  return row(pact.slotLevel, true, pact.total - pact.used, pact.total, pact.used, false);
}

function row(
  level: number,
  pact: boolean,
  free: number,
  total: number | null,
  used: number,
  enabled: boolean,
): SlotRow {
  return {
    level,
    pact,
    free,
    total,
    used,
    enabled,
    title: pact ? `${circleLabel(level)} (pacto)` : circleLabel(level),
    count: freeText(free, total),
  };
}

/** The row a spell opens with: the lowest free one. */
export function defaultSlot(rows: readonly SlotRow[]): SlotRow | null {
  return rows.find((r) => r.enabled) ?? null;
}

/** "É o seu último espaço de 1º nível: depois dele, o Escudo Arcano fica sem
 * espaço." The shield part only when Escudo is prepared and this slot is the
 * last one it could be cast with (`shieldFree` is the free slots of Escudo's
 * own options; `null` when the character has no Escudo). */
export function lastSlotWarning(
  slot: SlotRow | null,
  shieldFree: number | null,
  shieldName = 'Escudo Arcano',
): string {
  if (!slot || slot.free !== 1) {
    return '';
  }
  const base = `É o seu último espaço de ${circleLabel(slot.level)}`;
  return shieldFree === 1 ? `${base}: depois dele, o ${shieldName} fica sem espaço.` : `${base}.`;
}

// ---- the targets ----

/** How many targets a cast takes and whether the step is there at all. */
export interface TargetRule {
  /** `none`: the spell stays on the caster (or has no one to aim at). */
  readonly kind: 'none' | 'single' | 'multi' | 'darts';
  /** The most targets (10 is the server's limit for any cast). */
  readonly max: number;
  /** At least this many: 0 for an area, which may meet nobody. */
  readonly min: number;
  /** The shares to hand out are rays (Raio Ardente), not darts: the words change, the stepper does not. */
  readonly rays?: boolean;
}

/** Raio Ardente: its rays go to the targets as the caster shares them out, all to one if they want. */
const SCORCHING_RAY_KEY = 'spell:scorching-ray';

export const MAX_CAST_TARGETS = 10;

/** Magic Missile's darts at a slot level (3 at the 1st, one more for each level above). */
export function dartsAt(darts: readonly DartsAtSlot[], level: number): number {
  return darts.find((d) => d.slotLevel === level)?.darts ?? 0;
}

/**
 * The level a cast scales at: the slot's, else the level the grant casts at, else the spell's own (`own`; 0 for a
 * cantrip), so a grant's level never replaces a slot's.
 */
export function castLevelOf(
  slot: { readonly level: number } | null | undefined,
  grant: { readonly castLevel: number } | null | undefined,
  own: number,
): number {
  return slot?.level ?? (grant && grant.castLevel > own ? grant.castLevel : own);
}

/**
 * How many targets a cast takes and who can be picked. `entryLevel` is the level the entry `st` was listed at: the spell's
 * own for the class's cast, the grant's cast level for a grant's; `slotLevel` is the level the cast scales at.
 */
export function targetRule(
  st: SpellTargets | undefined,
  casterId: string,
  entryLevel: number,
  slotLevel: number,
): TargetRule {
  if (!st) {
    return { kind: 'none', max: 0, min: 0 };
  }
  if (st.darts.length > 0) {
    const n = dartsAt(st.darts, slotLevel);
    return {
      kind: 'darts',
      max: Math.min(n, MAX_CAST_TARGETS),
      min: 1,
      ...(st.spellKey === SCORCHING_RAY_KEY ? { rays: true } : {}),
    };
  }
  // A spell that stays on the caster lists only the caster.
  if (st.maxTargets === 0 && st.targets.every((t) => t.combatantId === casterId)) {
    return { kind: 'none', max: 0, min: 0 };
  }
  if (st.maxTargets === 0) {
    return { kind: 'multi', max: MAX_CAST_TARGETS, min: 0 }; // an area: any number, even none
  }
  // The server says how many more targets each level above the entry's adds; a server that sets only the flag means one.
  // A slot below the level of a grant's entry takes as many away.
  const perLevel = st.targetsPerLevel > 0 ? st.targetsPerLevel : st.extraTargetPerLevel ? 1 : 0;
  const extra = perLevel * (slotLevel - entryLevel);
  const max = Math.max(Math.min(st.maxTargets + extra, MAX_CAST_TARGETS), 1);
  return { kind: max === 1 ? 'single' : 'multi', max, min: 1 };
}

/** A target row: "Ferido · a 7,5 m", and why it can't be chosen. */
export interface CastTargetRow {
  readonly id: string;
  readonly label: string;
  readonly sub: string;
  readonly blocked: string;
  /** The cover the target has against this caster ("Meia cobertura (do mapa)") and its pictogram. */
  readonly cover: string;
  readonly coverMark: 'half' | 'three' | null;
}

/** The rows of the target step. `reach` is "Longe demais: alcance de 36 m"
 * for a target the spell cannot reach. */
export function castTargetRows(
  targets: readonly TargetInReach[],
  casterId: string,
  reachFt: number | null,
): CastTargetRow[] {
  return targets.flatMap((t) => {
    const cover = listing(t);
    if (cover.kind === 'left-out') {
      return [];
    }
    const parts: string[] = [];
    if (t.combatantId === casterId) {
      parts.push('você');
    } else {
      const word = stateWord(t.state);
      if (word) {
        parts.push(word);
      }
      if (t.distanceFt !== undefined) {
        parts.push(`a ${metersFixed(t.distanceFt)}`);
      }
    }
    return [
      {
        id: t.combatantId,
        label: t.combatantId === casterId ? `${t.label} (você)` : t.label,
        sub: tight(joinDots(parts)),
        blocked:
          cover.kind === 'blocked'
            ? cover.text
            : t.tooFar
              ? tight(reachFt ? `Longe demais: alcance de ${metersText(reachFt)}` : 'Longe demais')
              : '',
        cover: cover.kind === 'listed' ? cover.text : '',
        coverMark: cover.kind === 'listed' ? cover.mark : null,
      },
    ];
  });
}

/** Whether choosing `id` is allowed on top of the ones already chosen. */
export function canPick(rule: TargetRule, chosen: readonly string[], id: string): boolean {
  return chosen.includes(id) || chosen.length < rule.max;
}

/** The ids with `id` toggled (a single target replaces the choice). */
export function toggled(rule: TargetRule, chosen: readonly string[], id: string): string[] {
  if (rule.kind === 'single') {
    return [id];
  }
  if (chosen.includes(id)) {
    return chosen.filter((c) => c !== id);
  }
  return canPick(rule, chosen, id) ? [...chosen, id] : [...chosen];
}

// ---- the darts ----

/** The darts handed out so far: by target. */
export type Dealt = ReadonlyMap<string, number>;

export function dartsPlaced(dealt: Dealt): number {
  let n = 0;
  for (const v of dealt.values()) {
    n += v;
  }
  return n;
}

/** The counter in the footer next to "Conjurar": what is missing, and when all are placed. */
export function dartsStatus(total: number, dealt: Dealt, rays = false): string {
  const placed = dartsPlaced(dealt);
  const done = rays
    ? ['raio distribuído', 'raios distribuídos']
    : ['dardo distribuído', 'dardos distribuídos'];
  return placed === total
    ? `${total} de ${total} ${total === 1 ? done[0] : done[1]}.`
    : `Distribua ${rays ? 'todos os raios' : 'todos os dardos'}: ${placed} de ${total}.`;
}

/** The dealt darts with one more or one less on a target, inside 0 and the total. */
export function dealOne(
  dealtNow: Dealt,
  id: string,
  delta: 1 | -1,
  total: number,
): Map<string, number> {
  const next = new Map(dealtNow);
  const now = next.get(id) ?? 0;
  const value = now + delta;
  if (value < 0 || (delta > 0 && dartsPlaced(dealtNow) >= total)) {
    return next;
  }
  next.set(id, value);
  return next;
}

/** The targets of the request: who got at least one dart, each with its darts. */
export function dartTargets(dealtNow: Dealt): { combatantId: string; darts: number }[] {
  return [...dealtNow]
    .filter(([, n]) => n > 0)
    .map(([combatantId, darts]) => ({ combatantId, darts }));
}
