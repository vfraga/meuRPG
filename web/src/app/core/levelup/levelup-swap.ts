import { computed, signal } from '@angular/core';

import type {
  LevelUpNamedKey,
  LevelUpOptions,
  LevelUpSwapGroup,
} from '../../../gen/meurpg/characters/v1/characters_pb';
import type { LevelUpChoicesInit } from './levelup-client';
import type { ChangeRow } from './levelup-summary-rows';

/** One swap of one group: the pick to give up and the pick to take (either may still be empty). */
interface Selection {
  readonly from: string;
  readonly to: string;
}

/** A swap whole enough to send: the group, the pick given up and the pick taken. */
export interface WholeSwap {
  readonly group: string;
  readonly fromKey: string;
  readonly toKey: string;
}

/**
 * Whether a group is one of a table feature (`TableFeature.swap_levels`, sent in `swap_feature_options`) and not one
 * of the table rule's groups (`swap_pick`). The server names a feature group by the feature's key, and every feature
 * key starts with `feature:`; the rule's groups are fixed words (`cantrip`, `fighting_style`, `metamagic`,
 * `invocation`, `pact_boon`) that never do.
 */
export function isFeatureGroup(group: string): boolean {
  return group.startsWith('feature:');
}

/**
 * "Trocar uma escolha" of a level-up: one earlier pick of a group given up for another of the same pool.
 *
 * The groups come from the server and are of two kinds: the table rule "Trocar uma escolha no aumento de atributo"
 * (MR-025), which lets the level swap one pick in only one of its groups, and the table features that allow a swap at
 * the level (`swap_levels`), each of which lets one pick of its own group be swapped. So the draft holds one selection
 * per group; the rule's groups share one swap (starting a second one drops the first), the feature groups are
 * independent. The swap is optional and each one is sent only once whole.
 */
export class LevelUpSwap {
  /** The group open in the panel ('' for "Não trocar"). */
  readonly groupKey = signal('');
  private readonly selections = signal<Readonly<Record<string, Selection>>>({});

  readonly groups: () => readonly LevelUpSwapGroup[];
  readonly group = computed(() => this.groups().find((g) => g.group === this.groupKey()) ?? null);

  /** The pick to give up of the open group. */
  readonly fromKey = computed(() => this.selections()[this.groupKey()]?.from ?? '');
  /** The pick to take of the open group. */
  readonly toKey = computed(() => this.selections()[this.groupKey()]?.to ?? '');

  /** Every group whose swap is whole, in the order the server lists the groups. */
  private readonly whole = computed<WholeSwap[]>(() => {
    const out: WholeSwap[] = [];
    for (const g of this.groups()) {
      const s = this.selections()[g.group];
      if (s && g.picks.some((p) => p.key === s.from) && g.pool.some((p) => p.key === s.to)) {
        out.push({ group: g.group, fromKey: s.from, toKey: s.to });
      }
    }
    return out;
  });

  /** The swap of the table rule's groups to send (`swap_pick`), or `undefined` while it is not whole. */
  readonly pick = computed<LevelUpChoicesInit['swapPick']>(() =>
    this.whole().find((w) => !isFeatureGroup(w.group)),
  );

  /** The swaps of the table features' groups to send (`swap_feature_options`), one per group. */
  readonly featurePicks = computed<NonNullable<LevelUpChoicesInit['swapFeatureOptions']>>(() =>
    this.whole().filter((w) => isFeatureGroup(w.group)),
  );

  /** Both swaps, in the fields of the request's source. */
  readonly request = computed(() => ({
    swapPick: this.pick(),
    swapFeatureOptions: this.featurePicks(),
  }));

  /** One row for each whole swap, for the lists "O que muda até aqui" and the summary: the group, the pick given up → the pick taken. */
  readonly rows = computed<ChangeRow[]>(() =>
    this.whole().flatMap((w) => {
      const g = this.groups().find((x) => x.group === w.group);
      return g
        ? [
            {
              key: `swap:${w.group}`,
              label: g.namePt,
              before: nameIn(g.picks, w.fromKey, g.namePt),
              after: nameIn(g.pool, w.toKey, g.namePt),
              sub: '',
            },
          ]
        : [];
    }),
  );

  /** The line of the open group's swap, or '' while it is not whole. */
  readonly summary = computed(() => {
    const g = this.group();
    const w = this.whole().find((x) => x.group === g?.group);
    return g && w
      ? `Trocar ${nameIn(g.picks, w.fromKey, g.namePt)} por ${nameIn(g.pool, w.toKey, g.namePt)}`
      : '';
  });

  /** The groups with a part of a swap chosen and not the rest: a missing choice of the step, until it is whole or cleared. */
  readonly halfGroups = computed<LevelUpSwapGroup[]>(() =>
    this.groups().filter((g) => this.selections()[g.group] !== undefined && !this.isWhole(g.group)),
  );

  /** Whether the swap of this group is whole (the panel marks the group). */
  isWhole(group: string): boolean {
    return this.whole().some((w) => w.group === group);
  }

  /** The pick is already given up in another group: the same pick cannot leave twice. */
  givenUpElsewhere(key: string): boolean {
    const open = this.groupKey();
    return Object.entries(this.selections()).some(([g, s]) => g !== open && s.from === key);
  }

  /** Some part of a swap was chosen: leaving the level then asks first. */
  readonly started = computed(
    () => this.groupKey() !== '' || Object.keys(this.selections()).length > 0,
  );

  constructor(options: LevelUpOptions) {
    this.groups = () => options.swapGroups;
  }

  /** Opens a group in the panel; what was chosen in the other groups stays. */
  setGroup(key: string): void {
    this.groupKey.set(this.groups().some((g) => g.group === key) ? key : '');
  }

  setFrom(key: string): void {
    if (this.givenUpElsewhere(key)) {
      return;
    }
    this.choose('from', this.group()?.picks.some((p) => p.key === key) ? key : '');
  }

  setTo(key: string): void {
    this.choose('to', this.group()?.pool.some((p) => p.key === key) ? key : '');
  }

  /** "Não trocar": nothing is swapped in any group. */
  clear(): void {
    this.groupKey.set('');
    this.selections.set({});
  }

  /** Takes over what `other` had chosen, if these options still offer it. */
  adopt(other: LevelUpSwap): void {
    this.clear();
    for (const [group, s] of Object.entries(other.selections())) {
      this.setGroup(group);
      if (this.groupKey() === group) {
        this.setFrom(s.from);
        this.setTo(s.to);
      }
    }
    this.setGroup(other.groupKey());
  }

  private choose(side: 'from' | 'to', key: string): void {
    const group = this.groupKey();
    if (group === '') {
      return;
    }
    const next: Record<string, Selection> = {};
    for (const [g, s] of Object.entries(this.selections())) {
      // The table rule lets one swap be made among its groups: a new one in another of them replaces it.
      if (g === group || isFeatureGroup(g) || isFeatureGroup(group)) {
        next[g] = s;
      }
    }
    const was = next[group] ?? { from: '', to: '' };
    const now = { ...was, [side]: key };
    if (now.from === '' && now.to === '') {
      delete next[group];
    } else {
      next[group] = now;
    }
    this.selections.set(next);
  }
}

/** A pick's name without its group's name ("Estilo de Luta: Defesa" is "Defesa" under "Estilo de luta"; the case of the lead is not compared). */
export function plainName(groupName: string, name: string): string {
  const lead = `${groupName}: `;
  const same = name.toLocaleLowerCase('pt-BR').startsWith(lead.toLocaleLowerCase('pt-BR'));
  return same && name.length > lead.length ? name.slice(lead.length) : name;
}

function nameIn(list: readonly LevelUpNamedKey[], key: string, groupName = ''): string {
  return plainName(groupName, list.find((p) => p.key === key)?.namePt ?? key);
}
