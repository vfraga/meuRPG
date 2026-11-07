import {
  CastingTimeUnit,
  SpellDurationKind,
  SpellDurationUnit,
  SpellRangeKind,
  SpellSaveSuccess,
} from '../../../gen/meurpg/rules/v1/rules_pb';
import type { MessageInitShape } from '@bufbuild/protobuf';

import {
  TableAreaShape,
  type TableSpell,
  type TableSpellDamageSchema,
  TableSpellTargetKind,
} from '../../../gen/meurpg/rules/v1/table_content_pb';
import { rangeFeet, rangeMeters } from './effect-draft';
import { draftToSpellSummon } from './spell-summon';
import { emptySummon, summonToDraft } from './summon-draft';
import {
  type DurationChoice,
  type MechanicChoice,
  type RangeChoice,
  type SpellDraft,
  type SpellInit,
  type TargetChoice,
  type TimeChoice,
  emptySpell,
} from './spell-types';

const TIME_TO_UNIT: Record<TimeChoice, CastingTimeUnit> = {
  action: CastingTimeUnit.ACTION,
  bonus_action: CastingTimeUnit.BONUS_ACTION,
  reaction: CastingTimeUnit.REACTION,
  minute: CastingTimeUnit.MINUTE,
  hour: CastingTimeUnit.HOUR,
};
const RANGE_TO_KIND: Record<RangeChoice, SpellRangeKind> = {
  ranged: SpellRangeKind.RANGED,
  self: SpellRangeKind.SELF,
  touch: SpellRangeKind.TOUCH,
  sight: SpellRangeKind.SIGHT,
  unlimited: SpellRangeKind.UNLIMITED,
};
const DURATION_UNIT: Record<SpellDraft['durationUnit'], SpellDurationUnit> = {
  round: SpellDurationUnit.ROUND,
  minute: SpellDurationUnit.MINUTE,
  hour: SpellDurationUnit.HOUR,
  day: SpellDurationUnit.DAY,
};
const SHAPE_TO_ENUM: Record<SpellDraft['shape'], TableAreaShape> = {
  cone: TableAreaShape.CONE,
  cube: TableAreaShape.CUBE,
  cylinder: TableAreaShape.CYLINDER,
  line: TableAreaShape.LINE,
  sphere: TableAreaShape.SPHERE,
};

const TARGET_KIND: Record<TargetChoice, TableSpellTargetKind> = {
  creature: TableSpellTargetKind.CREATURE,
  creatures: TableSpellTargetKind.CREATURES,
  area: TableSpellTargetKind.AREA,
  self: TableSpellTargetKind.SELF,
};

/** The key of a choice map whose value is `value`: the reverse of the maps above, `fallback` for a value they do not know. */
function choiceOf<K extends string, V>(map: Record<K, V>, value: V | undefined, fallback: K): K {
  return (Object.entries(map).find(([, v]) => v === value)?.[0] as K | undefined) ?? fallback;
}

function durationChoice(kind: SpellDurationKind | undefined): DurationChoice {
  switch (kind) {
    case SpellDurationKind.TIMED:
      return 'timed';
    case SpellDurationKind.UNTIL_DISPELLED:
      return 'until_dispelled';
    default:
      return 'instant';
  }
}

/** The casting time, the range and the duration of a stored spell, as the form's fields. */
function timingToDraft(s: TableSpell): Partial<SpellDraft> {
  const ct = s.castingTime;
  const du = s.duration;
  return {
    time: choiceOf(TIME_TO_UNIT, ct?.unit, 'action'),
    timeAmount: ct?.amount || 1,
    trigger: ct?.triggerPt ?? '',
    range: choiceOf(RANGE_TO_KIND, s.range?.kind, 'ranged'),
    rangeM: rangeMeters(s.range?.distanceFt ?? 0),
    duration: durationChoice(du?.kind),
    durationAmount: du?.amount || 1,
    durationUnit: choiceOf(DURATION_UNIT, du?.unit, 'minute'),
    upTo: du?.upTo ?? false,
  };
}

function targetToDraft(s: TableSpell): Partial<SpellDraft> {
  const t = s.target;
  return {
    target: choiceOf(TARGET_KIND, t?.kind, 'creature'),
    count: t?.count || 2,
    perSlot: t?.perSlotLevel ?? 0,
    shape: choiceOf(SHAPE_TO_ENUM, t?.shape, 'cone'),
    sizeM: rangeMeters(t?.sizeFt ?? 0),
  };
}

/** The attack, the saving throw or the healing the spell has (one of them), as the form's mechanic. */
function mechanicToDraft(s: TableSpell): Partial<SpellDraft> {
  if (s.summon) {
    return { mechanic: 'summon', summon: summonToDraft(s.summon) };
  }
  if (s.attack) {
    return { mechanic: 'attack', attack: s.attack === 'melee' ? 'melee' : 'ranged' };
  }
  if (s.save) {
    return {
      mechanic: 'save',
      saveAbility: s.save.ability,
      saveOnSuccess: s.save.onSuccess === SpellSaveSuccess.NONE ? 'none' : 'half',
    };
  }
  return s.heal ? { mechanic: 'heal' } : {};
}

/** The form edits the first damage entry; the others are kept as they are. */
function damageToDraft(s: TableSpell): Partial<SpellDraft> {
  const [first, ...rest] = s.damage;
  const out: Partial<SpellDraft> = {};
  if (first) {
    out.dice = first.dice;
    out.damageType = first.damageTypeKey;
    out.more = s.level === 0 ? first.perTier : first.perSlotLevel;
    out.otherDamage = rest.map((r) => ({
      damageTypeKey: r.damageTypeKey,
      dice: r.dice,
      perSlotLevel: r.perSlotLevel,
      perTier: r.perTier,
    }));
  }
  if (s.heal) {
    out.healDice = s.heal.dice;
    out.healMore = s.heal.perSlotLevel;
    out.healModifier = s.heal.addsModifier;
  }
  return out;
}

/** The effect and the zone of a stored spell: no control shows them, so the draft carries them through. */
function keptToDraft(s: TableSpell): Partial<SpellDraft> {
  if (!s.effect && !s.zone) {
    return {};
  }
  return { kept: { effect: s.effect, zone: s.zone } };
}

/** A stored spell, as a form. */
export function spellToDraft(s: TableSpell): SpellDraft {
  return {
    ...emptySpell(),
    name: s.namePt,
    level: s.level,
    schoolKey: s.schoolKey,
    ...timingToDraft(s),
    verbal: s.components?.verbal ?? false,
    somatic: s.components?.somatic ?? false,
    material: s.components?.material ?? false,
    materialPt: s.components?.materialPt ?? '',
    concentration: s.concentration,
    ritual: s.ritual,
    classKeys: [...s.classKeys],
    text: s.descPt.join('\n\n'),
    higher: s.higherLevelPt.join('\n\n'),
    ...targetToDraft(s),
    ...mechanicToDraft(s),
    ...damageToDraft(s),
    ...keptToDraft(s),
  };
}

/** The paragraphs of a textarea: blank lines between them. */
export function paragraphs(text: string): string[] {
  return text
    .split(/\n\s*\n/)
    .map((p) => p.trim())
    .filter((p) => p !== '');
}

function castingTimeInit(d: SpellDraft): SpellInit['castingTime'] {
  const oneShot = d.time === 'action' || d.time === 'bonus_action' || d.time === 'reaction';
  return {
    unit: TIME_TO_UNIT[d.time],
    amount: oneShot ? 1 : d.timeAmount,
    ...(d.time === 'reaction' && d.trigger.trim() ? { triggerPt: d.trigger.trim() } : {}),
  };
}

function durationInit(d: SpellDraft): SpellInit['duration'] {
  switch (d.duration) {
    case 'instant':
      return { kind: SpellDurationKind.INSTANTANEOUS };
    case 'until_dispelled':
      return { kind: SpellDurationKind.UNTIL_DISPELLED };
    default:
      return {
        kind: SpellDurationKind.TIMED,
        amount: d.durationAmount,
        unit: DURATION_UNIT[d.durationUnit],
        upTo: d.upTo || d.concentration,
      };
  }
}

/** The target with only what its kind reads: a count for several creatures, the shape and size of an area. */
function targetInit(d: SpellDraft): NonNullable<SpellInit['target']> {
  const target: NonNullable<SpellInit['target']> = { kind: TARGET_KIND[d.target] };
  if (d.target === 'creatures') {
    target.count = d.count;
    target.perSlotLevel = Number.isNaN(d.perSlot) ? 0 : d.perSlot;
  } else if (d.target === 'creature' && d.perSlot > 0) {
    target.perSlotLevel = d.perSlot;
  } else if (d.target === 'area') {
    target.shape = SHAPE_TO_ENUM[d.shape];
    target.sizeFt = rangeFeet(d.sizeM);
  }
  return target;
}

/** The damage entries: only an attack or a saving throw with dice has any; the form's, then the kept ones. */
function damageInit(d: SpellDraft): MessageInitShape<typeof TableSpellDamageSchema>[] {
  const hasDamage = (d.mechanic === 'attack' || d.mechanic === 'save') && d.dice.trim() !== '';
  if (!hasDamage) {
    return [];
  }
  return [
    {
      damageTypeKey: d.damageType,
      dice: d.dice.trim(),
      ...(d.level === 0 ? { perTier: d.more.trim() } : { perSlotLevel: d.more.trim() }),
    },
    ...d.otherDamage.map((o) => ({ ...o })),
  ];
}

/** The attack, the saving throw or the healing the chosen mechanic sends; "Só texto" sends none. */
function mechanicInit(d: SpellDraft): Pick<SpellInit, 'attack' | 'save' | 'heal'> {
  switch (d.mechanic) {
    case 'attack':
      return { attack: d.attack };
    case 'save':
      return {
        save: {
          ability: d.saveAbility,
          onSuccess: d.saveOnSuccess === 'none' ? SpellSaveSuccess.NONE : SpellSaveSuccess.HALF,
        },
      };
    case 'heal':
      return {
        heal: {
          dice: d.healDice.trim(),
          ...(d.healMore.trim() ? { perSlotLevel: d.healMore.trim() } : {}),
          addsModifier: d.healModifier,
        },
      };
    default:
      return {};
  }
}

/** The body of a request, from the form. Only what the chosen mechanic uses goes: a spell of "Só texto" has no attack, no
 * saving throw, no damage and no healing; a spell that is not an area has no shape. */
export function draftToSpell(d: SpellDraft): SpellInit {
  return {
    namePt: d.name.trim(),
    level: d.level,
    schoolKey: d.schoolKey,
    castingTime: castingTimeInit(d),
    range: {
      kind: RANGE_TO_KIND[d.range],
      ...(d.range === 'ranged' ? { distanceFt: rangeFeet(d.rangeM) } : {}),
    },
    duration: durationInit(d),
    components: {
      verbal: d.verbal,
      somatic: d.somatic,
      material: d.material,
      ...(d.material && d.materialPt.trim() ? { materialPt: d.materialPt.trim() } : {}),
    },
    concentration: d.concentration,
    ritual: d.ritual,
    classKeys: [...d.classKeys],
    descPt: paragraphs(d.text),
    higherLevelPt: paragraphs(d.higher),
    target: targetInit(d),
    damage: damageInit(d),
    ...mechanicInit(d),
    ...(d.mechanic === 'summon' && d.summon ? { summon: draftToSpellSummon(d.summon) } : {}),
    ...(d.kept?.effect ? { effect: d.kept.effect } : {}),
    ...(d.kept?.zone ? { zone: d.kept.zone } : {}),
  };
}

/**
 * Choosing a mechanic. "Invocar" puts the caster as the target (the creatures appear next to it, and the range follows, as the
 * server asks) and gives back the creatures it had; the level and the ritual stay as they are, and the form says what the engine
 * will refuse about them (`summonRefusals`). The other mechanics keep what the summon had in the draft, and send none of it.
 */
export function withMechanic(d: SpellDraft, mechanic: MechanicChoice): SpellDraft {
  if (mechanic !== 'summon') {
    const { beforeSummon: was, ...rest } = d;
    const back = d.mechanic === 'summon' && was && d.target === 'self' && d.range === 'self';
    return { ...rest, mechanic, ...(back ? { target: was.target, range: was.range } : {}) };
  }
  if (d.mechanic === 'summon') {
    return d;
  }
  const next = d.target === 'self' ? d : withTarget(d, 'self');
  const beforeSummon = d.target === 'self' ? undefined : { target: d.target, range: d.range };
  return { ...next, mechanic, summon: d.summon ?? emptySummon(), beforeSummon };
}

/** Choosing "Só quem conjura" makes the range "Pessoal" (the server asks for it); leaving it gives the distance back. */
export function withTarget(d: SpellDraft, target: TargetChoice): SpellDraft {
  const { beforeSummon: _, ...kept } = d;
  const next = { ...kept, target };
  if (target === 'self') {
    next.range = 'self';
  } else if (d.target === 'self' && d.range === 'self' && target !== 'area') {
    // An area may come out of the caster (a cone from "Pessoal"): it keeps the range; picking creatures gives the distance back.
    next.range = 'ranged';
  }
  return next;
}

/** The paths of the count fields on screen whose text is no whole number the save can send (a NaN in the draft), in the order of the form. */
export function unreadableSpellPaths(d: SpellDraft): string[] {
  const bad = (n: number) => Number.isNaN(n);
  return [
    ...(bad(d.timeAmount) && (d.time === 'minute' || d.time === 'hour')
      ? ['table_spell.casting_time.amount']
      : []),
    ...(bad(d.durationAmount) && d.duration === 'timed' ? ['table_spell.duration.amount'] : []),
    ...(bad(d.count) && d.target === 'creatures' ? ['table_spell.target.count'] : []),
    ...(bad(d.perSlot) && d.level > 0 && (d.target === 'creatures' || d.target === 'creature')
      ? ['table_spell.target.per_slot_level']
      : []),
  ];
}

/** The draft with a NaN count read as not set: what the preview draws while the form says the text is unreadable. */
export function readableSpell(d: SpellDraft): SpellDraft {
  const n = (v: number) => (Number.isNaN(v) ? 0 : v);
  return {
    ...d,
    timeAmount: n(d.timeAmount),
    durationAmount: n(d.durationAmount),
    count: n(d.count),
    perSlot: n(d.perSlot),
  };
}
