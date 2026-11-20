import type { SpellKind } from '../../../../core/combat/cast-flow';
import {
  type TargetRule,
  castLevelOf,
  defaultSlot,
  slotRows,
  targetRule,
} from '../../../../core/combat/cast-flow';
import { attackName } from '../../../../core/combat/combat-options';
import {
  type PickRow,
  attackNumbers,
  attackPickRows,
  castPickRows,
  counts,
} from '../../../../core/combat/target-pick';
import {
  type ContestAttackOption,
  ContestTargetReason,
} from '../../../../../gen/meurpg/play/v1/contest_types_pb';
import { AreaPlacement } from '../../../../../gen/meurpg/play/v1/combat_pb';
import type { AttackSheetData } from '../attack-sheet/attack-sheet-types';
import type { CastSheetData } from '../cast-sheet/cast-sheet-types';
import { type PickSession, TargetMode } from './target-mode';

/** The attack that opens its sheet with the target named. */
const ONE: TargetRule = { kind: 'single', max: 1, min: 1 };

/** What the flow reads from the combat screen. */
export interface TargetFlowDeps {
  /** The player's own turn on a drawn battle map (not the master, not the theatre of the mind). */
  readonly onMap: () => boolean;
  /** The combatant has a square on the map, so a token to tap. */
  readonly placed: (id: string) => boolean;
  /** The map comes into view (and into the highlight). */
  readonly reveal: () => void;
  /** The element the player pressed, where the focus goes back to. */
  readonly origin: () => HTMLElement | null;
  /** The character the master acts for, or `''`. */
  readonly by?: () => string;
}

/** The spell's targets chosen on the map, as the cast sheet takes them. */
export interface CastPreset {
  readonly chosen: readonly string[];
  readonly dealt: ReadonlyMap<string, number>;
}

/**
 * Starts the choice of a target on the map for the actions of the player's own turn (C7-04, C7-05): an attack with its target
 * list, a spell that aims at one or several creatures, "Agarrar" and "Empurrar". Each `start` returns `false` when the map
 * cannot be the way (no map, nobody on it to tap, an area spell that is placed instead): the page then opens the sheet as it
 * always did. The targets, their reach, cover and disadvantage are what the server sent for the sheet; nothing is judged here.
 */
export class TargetFlow {
  constructor(
    readonly mode: TargetMode,
    private readonly deps: TargetFlowDeps,
  ) {}

  /** An attack with a weapon or a spell attack roll. `open` is the attack sheet (with the target, or without). */
  attack(data: AttackSheetData, open: (target: string | null) => void): boolean {
    const rows = attackPickRows(data.targets, data.attack);
    return this.begin({
      key: data.attack.key,
      name: attackName(data.attack),
      rows,
      rule: ONE,
      detail: attackNumbers(data.attack),
      foe: true,
      open: (picks) => open(picks?.[0] ?? null),
    });
  }

  /** A spell with one target, several, or darts or rays to hand out; an area, a teleport and a spell on the caster keep the sheet. */
  cast(data: CastSheetData, kind: SpellKind, open: (preset: CastPreset | null) => void): boolean {
    const rule = this.castRule(data);
    const targets = data.targets;
    if (!targets || rule.kind === 'none' || (rule.kind === 'multi' && rule.min === 0)) {
      return false;
    }
    if (targets.teleportRangeFt > 0 || kind === 'pool' || areaPlaced(data)) {
      return false;
    }
    const self = kind === 'heal' || kind === 'plain';
    return this.begin({
      key: data.spellKey,
      name: data.name,
      rows: castPickRows(targets.targets, data.casterId, self),
      rule,
      detail: castNumbers(data, kind),
      foe: !self,
      open: (picks) =>
        open(
          picks
            ? {
                chosen: rule.kind === 'darts' ? [] : picks,
                dealt: rule.kind === 'darts' ? counts(picks) : new Map(),
              }
            : null,
        ),
    });
  }

  /** "Agarrar" and "Empurrar": the creatures at reach no more than a size above the player's. */
  contest(
    option: ContestAttackOption,
    name: string,
    labelOf: (id: string) => string,
    open: (target: string | null) => void,
  ): boolean {
    const rows: PickRow[] = option.targets
      .filter(
        (t) =>
          t.reason !== ContestTargetReason.OUT_OF_REACH &&
          t.reason !== ContestTargetReason.NOT_PLACED,
      )
      .map((t) => ({
        id: t.combatantId,
        label: labelOf(t.combatantId),
        reason: t.eligible ? '' : 'grande demais',
        note: '',
      }));
    return this.begin({
      key: name,
      name,
      rows,
      rule: ONE,
      detail: '',
      foe: true,
      open: (picks) => open(picks?.[0] ?? null),
    });
  }

  private castRule(data: CastSheetData): TargetRule {
    const slot = defaultSlot(slotRows(data.level, data.slots, data.usage, data.pact));
    // A granted spell opens cast through the grant, at the level the grant casts at.
    const level = castLevelOf(data.grant ? null : slot, data.grant, data.level);
    return targetRule(
      data.targets,
      data.casterId,
      castLevelOf(null, data.grant, data.level),
      level,
    );
  }

  /** Starts the mode when there is a creature to tap; the map comes into view. */
  private begin(spec: Omit<PickSession, 'origin'>): boolean {
    const tappable = spec.rows.filter((r) => !r.reason && this.deps.placed(r.id));
    if (!this.deps.onMap() || tappable.length === 0) {
      return false;
    }
    this.mode.begin({
      ...spec,
      rows: spec.rows.filter((r) => this.deps.placed(r.id)),
      origin: this.deps.origin(),
      by: this.deps.by?.() ?? '',
    });
    this.deps.reveal();
    return true;
  }
}

/** A spell that is placed on the map (an area) keeps the picker of its area. */
function areaPlaced(data: CastSheetData): boolean {
  const placement = data.targets?.placement ?? AreaPlacement.UNSPECIFIED;
  return (
    !!data.map && placement !== AreaPlacement.UNSPECIFIED && placement !== AreaPlacement.CASTER
  );
}

/** The spell attack's numbers before the roll, when it has any ("+7 para acertar · 1d10 de fogo"). */
function castNumbers(data: CastSheetData, kind: SpellKind): string {
  const bonus =
    kind === 'attack'
      ? `${data.attackBonus < 0 ? '−' : '+'}${Math.abs(data.attackBonus)} para acertar`
      : '';
  return [bonus, data.cantripDice].filter(Boolean).join(' · ');
}
