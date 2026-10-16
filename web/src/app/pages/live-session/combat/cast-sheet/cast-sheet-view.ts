import {
  type Combatant,
  type Encounter,
  type PreviewSpellAreaResponse,
  type SpellTargets,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  type MetamagicOption,
  type SpellDetails,
  SpellDamageChoice,
  SpellRangeKind,
} from '../../../../../gen/meurpg/rules/v1/rules_pb';
import {
  type CastTargetRow,
  type SpellKind,
  type TargetRule,
  castTargetRows,
  dartsPlaced,
  dartsStatus,
} from '../../../../core/combat/cast-flow';
import { article } from '../../../../core/combat/combat-log';
import { groupFeminine, groupName, isCreature } from '../../../../core/combat/creature-names';
import { abilityMissing } from '../../../../core/effects/ability-choice';
import { type MetamagicPicks, metamagicMissing } from '../../../../core/resources/metamagic';
import type { Pool } from '../../../../core/resources/pools';
import type { SculptRow } from './sculpt-picker';

export {
  areaResultLine,
  doneSubtitle,
  castTitle,
  slotAfter,
  shieldLostText,
  castNotes,
  isPlainCast,
  nextRollOf,
  resumedCastOf,
  castShow,
  luckyShow,
  castTargetsOf,
} from './cast-result-view';

/** What the cast sheet shows and asks, as pure functions of the spell's details, the combat and the choices made. */

/** The damage types a spell lets the caster pick among; empty when it offers no choice. */
export function damageChoicesOf(d: SpellDetails | null): { key: string; name: string }[] {
  return d && d.damageChoice !== SpellDamageChoice.UNSPECIFIED && d.damage.length > 1
    ? d.damage.map((x) => ({ key: x.damageTypeKey, name: x.damageTypePt }))
    : [];
}

/** The picked damage type; the first until the caster picks another. */
export function damageTypeOf(choices: readonly { key: string }[], picked: string): string {
  return choices.find((c) => c.key === picked)?.key ?? choices[0]?.key ?? '';
}

/** How far a ranged spell reaches, `null` for any other. */
export function reachFtOf(details: SpellDetails | null): number | null {
  const r = details?.range;
  return r?.kind === SpellRangeKind.RANGED ? r.distanceFt : null;
}

/** The caster is a target only of a heal or a spell the app has no effect for (a blessing): a dart, a ray or a
 * fireball at oneself is never the choice. */
export function castRowsOf(
  targets: SpellTargets | undefined,
  kind: SpellKind,
  casterId: string,
  reachFt: number | null,
): CastTargetRow[] {
  const self = kind === 'heal' || kind === 'plain';
  const list = (targets?.targets ?? []).filter((t) => self || t.combatantId !== casterId);
  return castTargetRows(list, casterId, reachFt);
}

/** The ids of the combatants that pass `test`. */
export function idsWhere(
  combatants: readonly Combatant[],
  test: (c: Combatant) => boolean,
): Set<string> {
  return new Set(combatants.filter(test).map((c) => c.id));
}

/** "Constrição encerra a concentração em Conjurar Animais.", and "Os 2 Lobos atrozes somem." when that concentration
 * holds the caster's creatures: only when this spell needs concentration and the caster holds another one, from the
 * combat's own data. */
export function concentrationEnd(
  e: Encounter | null,
  spell: {
    readonly casterId: string;
    readonly concentration: boolean;
    readonly key: string;
    readonly name: string;
  },
): { ends: string; goes: string } | null {
  const caster = e?.combatants.find((c) => c.id === spell.casterId);
  if (
    !e ||
    !caster ||
    !spell.concentration ||
    !caster.concentrationSpell ||
    caster.concentrationSpell === spell.key
  ) {
    return null;
  }
  const held = e.combatants.filter(
    (c) =>
      isCreature(c) &&
      c.ownerCharacterId === caster.characterId &&
      !!c.summonGroupId &&
      !c.defeated,
  );
  let goes = '';
  if (held.length === 1) {
    goes = `${article(held[0].label) === 'a' ? 'A' : 'O'} ${held[0].label} some.`;
  } else if (held.length > 1) {
    goes = `${groupFeminine(held) ? 'As' : 'Os'} ${held.length} ${groupName(held)} somem.`;
  }
  return {
    ends: `${spell.name} encerra a concentração em ${caster.concentrationSpellNamePt || 'a magia'}.`,
    goes,
  };
}

/** What the target step still needs, in words; `''` when it is complete. */
export function targetsMissingText(
  rule: TargetRule,
  kind: SpellKind,
  have: {
    readonly chosen: number;
    readonly dealt: ReadonlyMap<string, number>;
    readonly darts: number;
  },
  canApp: boolean,
): string {
  if (rule.kind === 'darts') {
    return dartsPlaced(have.dealt) === have.darts ? '' : dartsStatus(have.darts, have.dealt);
  }
  if ((rule.kind === 'single' || rule.kind === 'multi') && have.chosen < rule.min) {
    return rule.kind === 'single' ? 'Escolha o alvo.' : 'Escolha pelo menos um alvo.';
  }
  if (kind === 'attack' && have.chosen > 1 && !canApp) {
    return 'Com dados físicos, escolha um alvo por vez.';
  }
  return '';
}

/** Why "Conjurar" is not ready yet, in words; empty when it is. */
export function castMissing(i: {
  readonly detailsRead: boolean;
  readonly detailsFailed: boolean;
  readonly level: number;
  readonly slotChosen: boolean;
  readonly spellKey: string;
  readonly ability: string;
  /** A placed area takes no target: the server works out who is inside from the point. */
  readonly placedArea: boolean;
  readonly targets: () => string;
  readonly metaOptions: readonly MetamagicOption[];
  readonly chosenMeta: readonly string[];
  readonly picks: MetamagicPicks;
  readonly sorceryPoints: Pool | null;
}): string {
  if (!i.detailsRead && !i.detailsFailed) {
    return 'Lendo a magia…';
  }
  if (i.detailsFailed) {
    return 'Não deu para ler a magia. Feche e tente de novo.';
  }
  if (i.level > 0 && !i.slotChosen) {
    return 'Escolha o espaço de magia.';
  }
  const ability = abilityMissing(i.spellKey, i.ability);
  if (ability) {
    return ability;
  }
  const target = i.placedArea ? '' : i.targets();
  return target || metamagicMissing(i.metaOptions, i.chosenMeta, i.picks, i.sorceryPoints);
}

/** The creatures the caster can spare with Sculpt Spells: the others in the placed area, or the ones ticked in the
 * list; allies first. */
export function sculptRowsOf(
  offered: boolean,
  area: PreviewSpellAreaResponse | null | undefined,
  placed: boolean,
  listed: readonly CastTargetRow[],
  chosen: readonly string[],
): SculptRow[] {
  if (!offered) {
    return [];
  }
  const rows: SculptRow[] = placed
    ? (area?.targets ?? [])
        .filter((t) => !t.self)
        .map((t) => ({ id: t.combatantId, label: t.label, ally: t.ally }))
    : listed
        .filter((t) => !t.blocked && chosen.includes(t.id))
        .map((t) => ({ id: t.id, label: t.label, ally: false }));
  return [...rows.filter((r) => r.ally), ...rows.filter((r) => !r.ally)];
}

/** What the cast sends: the ones marked that are still listed, no more than the limit. */
export function sculptedIdsOf(
  marked: readonly string[],
  rows: readonly SculptRow[],
  limit: number,
): string[] {
  const listed = new Set(rows.map((r) => r.id));
  return marked.filter((id) => listed.has(id)).slice(0, limit);
}

/** The marked creatures after a click: out when it was in, in while there is room under the limit. */
export function toggledSculpt(now: readonly string[], id: string, limit: number): string[] {
  if (now.includes(id)) {
    return now.filter((x) => x !== id);
  }
  return now.length < limit ? [...now, id] : [...now];
}

/** The targets still chosen when the list or the slot changed, or `null` when all of them still fit. */
export function keptTargets(
  chosen: readonly string[],
  fits: ReadonlySet<string>,
  max: number,
): string[] | null {
  const kept = chosen.filter((id) => fits.has(id)).slice(0, Math.max(max, 0));
  return kept.length !== chosen.length ? kept : null;
}

/** The darts after the slot or the targets changed, or `null` when they stand: the only reachable target takes them
 * all when none is placed yet, and darts past what the slot makes are taken back. */
export function dartsAfterChange(
  dealt: ReadonlyMap<string, number>,
  reachable: readonly string[],
  total: number,
  cast: boolean,
): Map<string, number> | null {
  let next: ReadonlyMap<string, number> = dealt;
  let changed = false;
  if (reachable.length === 1 && dartsPlaced(next) === 0 && total > 0 && !cast) {
    next = new Map([[reachable[0], total]]);
    changed = true;
  }
  if (dartsPlaced(next) > total) {
    next = new Map();
    changed = true;
  }
  return changed ? new Map(next) : null;
}

/** The Metamagic picks after the targets changed: a creature no longer a target cannot stay the second target or the
 * one with disadvantage. */
export function picksAfterTargets(p: MetamagicPicks, chosen: readonly string[]): MetamagicPicks {
  return {
    twinned: p.twinned === chosen[0] ? '' : p.twinned,
    careful: p.careful,
    heightened: chosen.includes(p.heightened) ? p.heightened : '',
  };
}

/** "A cobertura é medida do ponto de origem...": only when the area's preview counts cover. */
export function coverNoteOf(p: PreviewSpellAreaResponse | null | undefined, name: string): string {
  if (!p || p.targets.length === 0) {
    return '';
  }
  return p.coverCounts
    ? `A cobertura é medida do ponto de origem, não de você, e só conta quando o teste é de Destreza (como o ${article(name) === 'a' ? 'da' : 'do'} ${name}).`
    : '';
}
