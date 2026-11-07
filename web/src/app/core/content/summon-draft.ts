import { clone, create } from '@bufbuild/protobuf';

import {
  CastingTimeUnit,
  SpellDurationKind,
  type SpellDurationUnit,
} from '../../../gen/meurpg/rules/v1/rules_pb';
import {
  TableSpellCastingTimeSchema,
  TableSpellDurationSchema,
  type TableSummon,
  TableSummonReplaces,
  TableSummonRole,
  TableSummonSchema,
  TableSummonTurnMode,
} from '../../../gen/meurpg/rules/v1/table_content_pb';
import type { EffectDraft } from './effect-draft.types';

/**
 * What a feature's "Invoca criaturas" says (#578): the creatures a holder brings with a use of the feature's resource.
 * The draft keeps what the block edits as typed (the count formula, the most at a time); everything the block has no control for
 * (the role, the creatures per slot level, the cost, fields a newer server adds) goes back as it was loaded (`base`). A feature
 * with no summon has `null`: switching it off sends none.
 */

export interface SummonTime {
  unit: CastingTimeUnit;
  amount: number;
  triggerPt: string;
}

export interface SummonDuration {
  kind: SpellDurationKind;
  amount: number;
  unit: SpellDurationUnit | 0;
  upTo: boolean;
}

export interface SummonDraft {
  /** Table creatures ("creature:<slug>@mesa") and the SRD's ("monster:wolf"), in the order picked. */
  creatureKeys: string[];
  /** The formula as typed. */
  count: string;
  /** The most creatures at a time as typed ("0" is no limit). */
  maxLive: string;
  /** The creatures a circle above the spell's own adds, as typed (a spell's summon only: a feature's keeps the stored 0). */
  perSlot: string;
  turnMode: TableSummonTurnMode;
  replaces: TableSummonReplaces;
  /** The name of the resource a use is spent from; empty until one is chosen. */
  resourceKey: string;
  time: SummonTime;
  duration: SummonDuration;
  concentration: boolean;
  /** The `resource` effect of the feature whose name `resourceKey` follows (the editor's row id, never sent); unset for a key the feature does not define. */
  resourceEffect?: string;
  /** The stored message: its other fields (role, cost, count per slot level, unknown ones) go back untouched. */
  base?: TableSummon;
}

/** The most creatures a summon lists, and the most it keeps at a time (the server's limits). */
export const MAX_SUMMON_CREATURES = 8;
export const MAX_SUMMON_LIVE = 40;

export function emptySummon(resourceKey = ''): SummonDraft {
  return {
    creatureKeys: [],
    count: '1',
    maxLive: '0',
    perSlot: '0',
    turnMode: TableSummonTurnMode.OWN_INITIATIVE,
    replaces: TableSummonReplaces.NONE,
    resourceKey,
    time: { unit: CastingTimeUnit.ACTION, amount: 1, triggerPt: '' },
    duration: { kind: SpellDurationKind.INSTANTANEOUS, amount: 0, unit: 0, upTo: false },
    concentration: false,
  };
}

export function summonToDraft(s: TableSummon): SummonDraft {
  return {
    creatureKeys: [...s.creatureKeys],
    count: s.count,
    maxLive: String(s.maxLive),
    perSlot: String(s.countPerSlotLevel),
    turnMode: s.turnMode,
    replaces: s.replaces,
    resourceKey: s.resourceKey,
    time: {
      unit: s.castingTime?.unit ?? CastingTimeUnit.UNSPECIFIED,
      amount: s.castingTime?.amount ?? 0,
      triggerPt: s.castingTime?.triggerPt ?? '',
    },
    duration: {
      kind: s.duration?.kind ?? SpellDurationKind.UNSPECIFIED,
      amount: s.duration?.amount ?? 0,
      unit: s.duration?.unit ?? 0,
      upTo: s.duration?.upTo ?? false,
    },
    concentration: s.concentration,
    base: s,
  };
}

/** "12" is 12; "", " " is 0 (no limit); text that is no whole number is -1, which the server refuses at the field with its own words. */
export function wholeOf(text: string): number {
  const t = text.trim();
  if (t === '') {
    return 0;
  }
  return /^\d+$/.test(t) ? Number(t) : -1;
}

/** The summon as the save sends it: the stored message with what the block edits put over it. */
export function draftToSummon(d: SummonDraft): TableSummon {
  const out = d.base ? clone(TableSummonSchema, d.base) : create(TableSummonSchema);
  out.creatureKeys = [...d.creatureKeys];
  out.count = d.count;
  out.maxLive = wholeOf(d.maxLive);
  out.turnMode = d.turnMode;
  out.replaces = d.replaces;
  out.resourceKey = d.resourceKey;
  out.resourceCost = d.base?.resourceCost || 1;
  // An empty time and an empty duration stay as stored (unset is 1 action, instantaneous).
  out.castingTime =
    d.time.unit === CastingTimeUnit.UNSPECIFIED && d.time.amount === 0 && d.time.triggerPt === ''
      ? d.base?.castingTime
      : create(TableSpellCastingTimeSchema, d.time);
  out.duration =
    d.duration.kind === SpellDurationKind.UNSPECIFIED &&
    d.duration.amount === 0 &&
    d.duration.unit === 0 &&
    !d.duration.upTo
      ? d.base?.duration
      : create(TableSpellDurationSchema, d.duration);
  out.concentration = d.concentration;
  return out;
}

/** The paths of the inputs the block draws for a feature at `base`; a refusal at another path under the summon lands on the block's own. */
export function summonPaths(base: string): string[] {
  const at = `${base}.summon`;
  return [
    at,
    `${at}.creature_keys`,
    `${at}.count`,
    `${at}.max_live`,
    `${at}.resource_key`,
    `${at}.turn_mode`,
    `${at}.replaces`,
    `${at}.casting_time`,
    `${at}.duration`,
    `${at}.concentration`,
  ];
}

/** A resource the feature defines with a `resource` effect: the name a summon spends from, and how the select says it. */
export interface SummonResource {
  /** The id of the effect that defines it. */
  readonly id: string;
  readonly key: string;
  readonly label: string;
}

/** The resources a feature defines ("Companheiro do Vale (1)": the feature's name and the uses; the resource's own name when it has several). */
export function resourcesOf(f: {
  readonly name: string;
  readonly effects: readonly EffectDraft[];
}): SummonResource[] {
  const own = f.effects.filter((e) => e.type === 'resource' && e.resource.trim() !== '');
  return own.map((e) => {
    const name = own.length === 1 && f.name.trim() !== '' ? f.name.trim() : e.resource.trim();
    const uses = e.max.trim();
    return { id: e.id, key: e.resource.trim(), label: uses === '' ? name : `${name} (${uses})` };
  });
}

/** The path of the resource field while the switch is on and no resource is chosen at all: saving waits, the block says why. A key the feature does not define is the class's (the server checks it), so it is not blocked. */
export function summonBlockedPaths(
  base: string,
  f: { readonly summon: SummonDraft | null },
): string[] {
  return f.summon && f.summon.resourceKey.trim() === '' ? [`${base}.summon.resource_key`] : [];
}

/** What the block asks of the feature's resources: the one the summon spends must be one the feature defines. */
export const NO_RESOURCE_TEXT =
  'Esta característica não define um recurso. Ponha nela um efeito Recurso para dar os usos da invocação.';
export const FIXED_OFF_TEXT =
  'Esta invocação tem um papel, um custo ou criaturas por nível de espaço que o editor não muda, e o servidor não aceita.';

/** The `resource` effect of `f` that defines `key`, if one does. */
function effectOfKey(
  f: { readonly effects: readonly EffectDraft[] },
  key: string,
): EffectDraft | undefined {
  return f.effects.find((e) => e.type === 'resource' && e.resource.trim() === key);
}

/**
 * The summon bound to the feature's resource that defines `key` (or to none, for a key the feature does not define: the class's,
 * written through the API), so a later rename of that effect is followed.
 */
export function bindResource<F extends { readonly effects: readonly EffectDraft[] }>(
  f: F,
  s: SummonDraft,
): SummonDraft {
  const own = s.resourceKey.trim() === '' ? undefined : effectOfKey(f, s.resourceKey.trim());
  return own?.id === s.resourceEffect ? s : { ...s, resourceEffect: own?.id };
}

/**
 * The feature after its effects changed: a summon that spends one of the feature's resources follows that effect's new name, by the
 * effect (so a feature with two resources, or a name cleared and typed again, still follows). A name cleared to nothing leaves the
 * last key in place. A key the feature never defined (the class's resource, written through the API) is never rewritten.
 */
export function followResource<
  F extends {
    readonly name: string;
    readonly effects: readonly EffectDraft[];
    readonly summon: SummonDraft | null;
  },
>(before: F, after: F): F {
  const s = after.summon;
  if (!s) {
    return after;
  }
  // A summon the person just pointed at another resource (or set up) is bound again; otherwise it follows the effect it was bound to.
  const bound =
    before.summon?.resourceKey === s.resourceKey && s.resourceEffect !== undefined
      ? s
      : bindResource(before.summon?.resourceKey === s.resourceKey ? before : after, s);
  const own = after.effects.find((e) => e.id === bound.resourceEffect && e.type === 'resource');
  const key = own?.resource.trim() ?? '';
  const next = key !== '' && key !== bound.resourceKey ? { ...bound, resourceKey: key } : bound;
  return next === s ? after : { ...after, summon: next };
}

/**
 * A summon switched off and on again, against the feature's resources as they are now: one bound to a resource that was renamed
 * follows it, and one whose resource is gone takes the first one left (or none). A key the feature never defined stays.
 */
export function restoredSummon(s: SummonDraft, resources: readonly SummonResource[]): SummonDraft {
  if (s.resourceEffect === undefined) {
    return s;
  }
  const own = resources.find((r) => r.id === s.resourceEffect) ?? resources[0];
  return { ...s, resourceKey: own?.key ?? '', resourceEffect: own?.id };
}

/** The role, the cost and the creatures per slot level the block cannot change: whether the stored summon has something else than the defaults. */
export function hasFixedOff(d: SummonDraft): boolean {
  const b = d.base;
  return (
    !!b &&
    ((b.role !== TableSummonRole.UNSPECIFIED && b.role !== TableSummonRole.COMPANION) ||
      (b.resourceCost !== 0 && b.resourceCost !== 1) ||
      b.countPerSlotLevel !== 0)
  );
}

/** "Usar o padrão": a companion, one use, no creature per slot level. */
export function resetFixed(d: SummonDraft): SummonDraft {
  if (!d.base) {
    return d;
  }
  const base = clone(TableSummonSchema, d.base);
  base.role = TableSummonRole.COMPANION;
  base.resourceCost = 1;
  base.countPerSlotLevel = 0;
  return { ...d, base };
}
