import { unreadableCastingPaths } from './whole-number';
import type {
  GetClassTableDefaultsResponse,
  TableSubclass,
} from '../../../gen/meurpg/rules/v1/table_content_pb';
import {
  type AlwaysPreparedGroup,
  LEVEL_COUNT,
  type LevelFeature,
  type SubclassDraft,
  type SubclassInit,
  castingInit,
  castingToDraft,
  emptyCasting,
  emptyRow,
  featuresAt,
  newLevelFeatureId,
  rowOf,
  slotsOf,
} from './class-draft';
import type { EffectMenuVm } from './effect-draft';
import { featureOwnPaths, featureToDraft, unreadableFeaturePaths } from './feature-draft';

// The subclass form's conversions and the paths of its inputs; the class form's are in class-draft.

/** The levels the request has a row for: all 20 for a class; for a subclass, the ones that cast or have a feature, ascending. */
export function subclassLevels(d: SubclassDraft): number[] {
  const start = d.casting.startLevel > 0 ? d.casting.startLevel : 1;
  const out: number[] = [];
  for (let level = 1; level <= LEVEL_COUNT; level++) {
    if ((d.conjures && level >= start) || d.features.some((f) => f.level === level)) {
      out.push(level);
    }
  }
  return out;
}

export function subclassFeatureBase(d: SubclassDraft, index: number): string {
  const f = d.features[index];
  const levels = subclassLevels(d);
  const k = d.features.slice(0, index).filter((x) => x.level === f.level).length;
  return `table_subclass.levels[${levels.indexOf(f.level)}].features[${k}]`;
}

export function subclassFeaturePaths(d: SubclassDraft, menu: EffectMenuVm): string[] {
  return d.features.flatMap((f, i) => featureOwnPaths(subclassFeatureBase(d, i), f.feature, menu));
}

/** The paths of the casting's start level and of the number fields, in every feature, whose text is no whole number. */
export function unreadableSubclassPaths(d: SubclassDraft, menu: EffectMenuVm): string[] {
  return [
    ...(d.conjures ? unreadableCastingPaths('table_subclass.casting', d.casting) : []),
    ...d.features.flatMap((f, i) =>
      unreadableFeaturePaths(subclassFeatureBase(d, i), f.feature, menu),
    ),
  ];
}

/** "table_subclass.levels[2]": the row of a class level of a third caster, or '' when the request has none for it. */
export function subclassRowBase(d: SubclassDraft, level: number): string {
  const i = subclassLevels(d).indexOf(level);
  return i < 0 ? '' : `table_subclass.levels[${i}]`;
}

export function emptySubclass(classKey: string): SubclassDraft {
  return {
    name: '',
    classKey,
    level: 0,
    text: '',
    conjures: false,
    casting: emptyCasting(''),
    rows: Array.from({ length: LEVEL_COUNT }, emptyRow),
    features: [],
    alwaysPrepared: [],
    spellsExpanded: false,
  };
}

export function subclassToDraft(
  s: TableSubclass,
  defaults: GetClassTableDefaultsResponse,
): SubclassDraft {
  const rows = Array.from({ length: LEVEL_COUNT }, emptyRow);
  const features: LevelFeature[] = [];
  for (const lv of s.levels) {
    if (lv.level >= 1 && lv.level <= LEVEL_COUNT) {
      rows[lv.level - 1] = rowOf(lv);
    }
    lv.features.forEach((f) =>
      features.push({ id: newLevelFeatureId(), level: lv.level, feature: featureToDraft(f) }),
    );
  }
  const groups = new Map<number, string[]>();
  for (const a of s.alwaysPrepared) {
    groups.set(a.classLevel, [...(groups.get(a.classLevel) ?? []), a.spellKey]);
  }
  return {
    name: s.namePt,
    classKey: s.classKey,
    level: s.level,
    text: s.descPt.join('\n\n'),
    conjures: s.casting !== undefined && s.casting.kind !== '',
    casting: castingToDraft(s.casting, defaults),
    rows,
    features,
    alwaysPrepared: [...groups]
      .sort((a, b) => a[0] - b[0])
      .map(([level, spells]) => ({ level, spells })),
    spellsExpanded: s.spellsExpanded,
  };
}

/** The paragraphs of a text: blank lines split them. */
function paragraphsOf(text: string): string[] {
  return text
    .split(/\n\s*\n/)
    .map((p) => p.trim())
    .filter((p) => p !== '');
}

export function draftToSubclass(d: SubclassDraft, menu: EffectMenuVm): SubclassInit {
  return {
    namePt: d.name.trim(),
    classKey: d.classKey,
    level: d.level,
    descPt: paragraphsOf(d.text),
    casting: d.conjures ? castingInit(d.casting) : undefined,
    levels: subclassLevels(d).map((level) => {
      const r = d.rows[level - 1];
      const cast = d.conjures && level >= (d.casting.startLevel > 0 ? d.casting.startLevel : 1);
      return {
        level,
        features: featuresAt(d.features, level, menu),
        cantripsKnown: cast ? r.cantrips : 0,
        spellsKnown: cast ? r.spells : 0,
        slots: cast ? slotsOf(r) : [],
      };
    }),
    alwaysPrepared: flattenAlwaysPrepared(d.alwaysPrepared),
    spellsExpanded: d.spellsExpanded && d.alwaysPrepared.some((g) => g.spells.length > 0),
  };
}

/** The request's list: a class level and a spell each, in the order of the groups. */
export function flattenAlwaysPrepared(
  groups: readonly AlwaysPreparedGroup[],
): { classLevel: number; spellKey: string }[] {
  return groups.flatMap((g) => g.spells.map((spellKey) => ({ classLevel: g.level, spellKey })));
}

/** The refusals of one always-prepared group: the request lists the spells flat (`always_prepared[3].spell_key`), the screen by
 * class level; a path in the flat list belongs to the group the index falls in. Returns the group's level, or -1. */
export function groupOfAlwaysPrepared(
  groups: readonly AlwaysPreparedGroup[],
  path: string,
): number {
  const m = /^table_subclass\.always_prepared\[(\d+)\]/.exec(path);
  if (!m) return -1;
  let n = Number(m[1]);
  for (const g of groups) {
    if (n < g.spells.length) return g.level;
    n -= g.spells.length;
  }
  return -1;
}
