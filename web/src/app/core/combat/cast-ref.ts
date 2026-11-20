import type { SpellTargets } from '../../../gen/meurpg/play/v1/combat_pb';
import type { SpellOption } from '../../../gen/meurpg/rules/v1/rules_pb';

/**
 * The reference the screen passes around for a spell it can cast. A spell of the character's classes is its key
 * ("spell:bless"); a spell a trait, feat or feature grants (`SpellOption.grant`) is the key plus the grant's source
 * ("spell:bless#granted=feat:graca@mesa"): the character may have the same spell both ways, and they are two options with
 * their own costs. Everything past the option list reads the real key and the source apart (`splitCastRef`).
 */
const GRANTED = '#granted=';

/** The reference of an option: its key, with the grant's source when it is a granted one. */
export function castRefOf(o: SpellOption): string {
  return castRefOfKey(o.spell?.key ?? '', o.grant?.sourceKey ?? '');
}

/** The reference of a spell key and the source it is cast through (`''` for the classes'). */
export function castRefOfKey(key: string, grantedBy: string): string {
  return grantedBy ? `${key}${GRANTED}${grantedBy}` : key;
}

/** A reference taken apart: the spell's key, and the source of the grant it is cast through (`''` for the classes'). */
export function splitCastRef(ref: string): { key: string; grantedBy: string } {
  const at = ref.indexOf(GRANTED);
  return at < 0
    ? { key: ref, grantedBy: '' }
    : { key: ref.slice(0, at), grantedBy: ref.slice(at + GRANTED.length) };
}

/** The option a reference names; for a plain key, the class's option and not a granted one of the same spell. */
export function optionOfRef(spells: readonly SpellOption[], ref: string): SpellOption | undefined {
  const { key, grantedBy } = splitCastRef(ref);
  return spells.find((s) => s.spell?.key === key && (s.grant?.sourceKey ?? '') === grantedBy);
}

/**
 * The targets the turn lists for an option of the spells. The server lists one entry per enabled option of a spell, in the
 * order of the options (the class's first, then the grants), so the entry is the n-th of its key, n being the place of the
 * option among the enabled ones of that key. `undefined` for an option that is not enabled or has no entry.
 */
export function targetsOfOption(
  spells: readonly SpellOption[],
  targets: readonly SpellTargets[],
  option: SpellOption | undefined,
): SpellTargets | undefined {
  const key = option?.spell?.key;
  if (!option || !key || !option.enabled) {
    return undefined;
  }
  const n = spells.filter((s) => s.enabled && s.spell?.key === key).indexOf(option);
  return n < 0 ? undefined : targets.filter((t) => t.spellKey === key)[n];
}

/** The targets of the option a reference names (`optionOfRef`). */
export function targetsOfRef(
  spells: readonly SpellOption[],
  targets: readonly SpellTargets[],
  ref: string,
): SpellTargets | undefined {
  return targetsOfOption(spells, targets, optionOfRef(spells, ref));
}
