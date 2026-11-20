import { clone } from '@bufbuild/protobuf';

import {
  type OptionPrerequisite,
  type OptionSwitchEntry,
  type TableEntry,
  type TableFeature,
  TableFeatureSchema,
} from '../../../gen/meurpg/rules/v1/table_content_pb';

/**
 * Whether a key is one the players never read: the table entry it names is off or archived (the server's players' copy drops
 * those keys before it sends an entry), or the master's switch list (`ListOptionSwitches`) says the players do not receive it:
 * an SRD option switched off, or one hidden through its owner (a subclass of a class that is off). A key that is in none of them
 * (a sibling option) is not hidden here.
 */
export function hiddenFrom(
  entries: readonly TableEntry[],
  switches: readonly OptionSwitchEntry[] = [],
): (key: string) => boolean {
  const gone = new Set(entries.filter((e) => e.off || e.archived).map((e) => e.key));
  for (const o of switches) {
    if (o.hidden || o.off || o.archived) {
      gone.add(o.key);
    }
  }
  return (key) => gone.has(key);
}

/** What an option asks, without the hidden keys: the same cuts the server makes in the players' copy. */
function stripAsks(p: OptionPrerequisite | undefined, hidden: (key: string) => boolean): void {
  if (!p) {
    return;
  }
  p.requiresOptionKeys = p.requiresOptionKeys.filter((k) => !hidden(k));
  if (hidden(p.requiresPact)) {
    p.requiresPact = '';
  }
  if (hidden(p.requiresSpellKey)) {
    p.requiresSpellKey = '';
  }
  if (p.classLevel && hidden(p.classLevel.classKey)) {
    p.classLevel = undefined;
  }
}

function strip(f: TableFeature, hidden: (key: string) => boolean): void {
  // The summon block is the master's to configure; the preview does not say it as a line the players read.
  f.summon = undefined;
  for (const e of f.effects) {
    e.spells = e.spells.filter((k) => !hidden(k));
    e.from = e.from.filter((k) => !hidden(k));
    if (hidden(e.spell)) {
      e.spell = '';
    }
  }
  stripAsks(f.prerequisite, hidden);
  f.options.forEach((o) => strip(o, hidden));
}

/** A copy of the feature as a player reads it: no hidden class, pact, spell or option is named in it, and no summon line. */
export function featureForPlayers(f: TableFeature, hidden: (key: string) => boolean): TableFeature {
  const copy = clone(TableFeatureSchema, f);
  strip(copy, hidden);
  return copy;
}
