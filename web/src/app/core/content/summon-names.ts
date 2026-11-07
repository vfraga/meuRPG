import { signal } from '@angular/core';

import { creatureSlug } from '../creatures/bestiary-format';

/**
 * The names of the creatures a feature's summon lists. A table creature is named by the catalog (its entry), an SRD one only by
 * the bestiary: the summon block remembers each SRD name it reads (a pick, a stat block) here, and the preview and the master's read
 * take it from here. A signal, so a preview that is already drawn follows a name that arrives late; before it arrives (or when
 * the bestiary does not answer) the SRD's own slug stands in ("Giant wolf spider").
 */
const known = signal<ReadonlyMap<string, string>>(new Map());

export function rememberCreatureName(key: string, namePt: string): void {
  // Only the SRD's names: a table creature is named by its entry, and the names of one campaign never reach another.
  if (key.startsWith('monster:') && namePt !== '' && known().get(key) !== namePt) {
    known.update((m) => new Map(m).set(key, namePt));
  }
}

export function creatureNameOf(key: string, nameOf: (key: string) => string): string {
  const named = nameOf(key);
  if (named !== key) {
    return named;
  }
  const remembered = known().get(key);
  if (remembered) {
    return remembered;
  }
  const slug = creatureSlug(key).replace(/-/g, ' ');
  return slug.charAt(0).toUpperCase() + slug.slice(1);
}

/** "Invoca: Lobo, Urso." — empty when the feature summons nothing. */
export function summonLine(
  summon: { readonly creatureKeys: readonly string[] } | undefined,
  nameOf: (key: string) => string,
): string {
  const keys = summon?.creatureKeys ?? [];
  return keys.length === 0
    ? ''
    : `Invoca: ${keys.map((k) => creatureNameOf(k, nameOf)).join(', ')}.`;
}
