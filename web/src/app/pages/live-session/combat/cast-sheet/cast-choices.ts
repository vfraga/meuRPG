import { computed, effect, untracked } from '@angular/core';

import {
  type SlotRow,
  dartsAt,
  dartsStatus,
  dealOne,
  defaultSlot,
  lastSlotWarning,
  slotRows,
  spellKind,
  targetRule,
  toggled,
} from '../../../../core/combat/cast-flow';
import type { Square } from '../../../../core/combat/combat-grid';
import { isPlayer } from '../../../../core/combat/combat-view';
import { isCreature } from '../../../../core/combat/creature-names';
import { poolDice } from '../../../../core/combat/hp-effects';
import { needsAbility } from '../../../../core/effects/ability-choice';
import {
  type MetamagicPicks,
  NO_PICKS,
  chosenLine,
  metamagicCost,
  metamagicRows,
  toggledOption,
} from '../../../../core/resources/metamagic';
import { tableOptionRows, toggledTable } from '../../../../core/combat/table-cast-options';
import type { Pool } from '../../../../core/resources/pools';
import type { CastArea } from './cast-area';
import {
  castMissing,
  castRowsOf,
  damageChoicesOf,
  damageTypeOf,
  dartsAfterChange,
  idsWhere,
  keptTargets,
  picksAfterTargets,
  reachFtOf,
  sculptRowsOf,
  sculptedIdsOf,
  targetsMissingText,
  toggledSculpt,
} from './cast-sheet-view';
import type { CastState } from './cast-state';
import type { SculptRow } from './sculpt-picker';

/** What a change of the choices does to the cast being built. */
export interface CastHooks {
  /** The choice changed: the key is a new cast's and the typed text is dropped (RN-18). */
  changed(): void;
  /** The mode of the d20 changed: a new cast, though the typed text stays. */
  newCast(): void;
}

/** The choices of a cast before it is sent: the slot, the targets or darts, the damage type, the ability, the
 * Metamagic options and the creatures Sculpt Spells spares; and what is still missing for the button. */
export class CastChoices {
  readonly asksAbility: boolean;
  readonly metaOptions: NonNullable<CastState['data']['metamagic']>;
  readonly sorceryPoints: Pool | null;
  readonly sculptOffered: boolean;
  /** The grant the spell is cast through, if any. */
  readonly grant: NonNullable<CastState['data']['grant']> | null;
  /** The slots to pick from: a granted spell has none to pick unless its grant allows one. */
  readonly rows = computed(() =>
    this.grant && !this.grant.alsoWithSlot
      ? []
      : slotRows(this.s.data.level, this.s.data.slots, this.s.data.usage, this.s.data.pact),
  );
  readonly viaGrant = computed(() => this.s.viaGrant());
  readonly kind = computed(() => spellKind(this.s.data.spellKey, this.s.details()));
  /** Sono and Leque Cromático roll a pool of dice: the dice at this slot. */
  readonly pool = computed(() =>
    this.kind() === 'pool' ? poolDice(this.s.details(), this.s.slotLevel()) : null,
  );
  /** The roll the sheet asks for before the cast: the d20 of a spell attack, or the pool when the
   * table's dice may be typed (with the app rolling every die, "Conjurar" is enough). */
  readonly usesPicker = computed(
    () => this.kind() === 'attack' || (this.kind() === 'pool' && this.canTypeDamage),
  );
  /** A spell that reads hit points says its area: who is in it is for the caster to say. */
  readonly hpArea = computed(() => this.kind() === 'pool');
  readonly rule = computed(() =>
    targetRule(this.s.data.targets, this.s.data.casterId, this.s.entryLevel(), this.s.slotLevel()),
  );
  readonly dartsTotal = computed(() =>
    dartsAt(this.s.data.targets?.darts ?? [], this.s.slotLevel()),
  );
  private readonly reachFt = computed(() => reachFtOf(this.s.details()));
  readonly targetRows = computed(() =>
    castRowsOf(this.s.data.targets, this.kind(), this.s.data.casterId, this.reachFt()),
  );
  readonly npcs = computed(() =>
    idsWhere(this.s.data.state.encounter()?.combatants ?? [], (c) => !isPlayer(c)),
  );
  readonly creatures = computed(() =>
    idsWhere(this.s.data.state.encounter()?.combatants ?? [], isCreature),
  );
  /** The damage types a spell lets the caster pick among; empty when it offers no choice. */
  readonly damageChoices = computed(() => damageChoicesOf(this.s.details()));
  readonly damageType = computed(() => damageTypeOf(this.damageChoices(), this.s.pickedType()));
  readonly warning = computed(() =>
    lastSlotWarning(this.s.slot(), this.s.data.shieldFree, this.s.data.shieldName),
  );
  /** The cast options of the table features, with their state. */
  readonly tableRows = computed(() =>
    tableOptionRows(this.s.data.tableOptions ?? [], this.s.tableTicked(), this.s.tableTypes()),
  );
  readonly metaRows = computed(() => metamagicRows(this.metaOptions, this.s.chosenMeta()));
  readonly metaCost = computed(() => metamagicCost(this.metaOptions, this.s.chosenMeta()));
  readonly metaLine = computed(() => chosenLine(this.metaOptions, this.s.chosenMeta()));
  /** The creatures the options of Metamagic can pick among: the spell's own targets that can be chosen. */
  readonly metaCandidates = computed(() => this.targetRows().filter((t) => !t.blocked));
  /** 1 + the level the spell is cast at (a cantrip is level 0). */
  readonly sculptLimit = computed(() => 1 + (this.s.data.level > 0 ? this.s.slotLevel() : 0));
  /** The creatures the caster can spare: the others in the placed area, or the ones ticked in the list; allies first. */
  readonly sculptRows = computed<SculptRow[]>(() =>
    sculptRowsOf(
      this.sculptOffered,
      this.area.flow?.preview(),
      !!this.area.flow,
      this.targetRows(),
      this.s.chosen(),
    ),
  );
  /** Why "Conjurar" is not ready yet, in words; empty when it is. */
  readonly missing = computed(() =>
    castMissing({
      detailsRead: !!this.s.details(),
      detailsFailed: this.s.detailsFailed(),
      level: this.s.data.level,
      slotChosen: !!this.s.slot() || this.viaGrant(),
      spellKey: this.s.data.spellKey,
      ability: this.s.ability(),
      placedArea: !!this.area.flow,
      destinationMissing:
        this.s.teleportFt > 0 && !this.s.data.master && this.s.destination() === null,
      targets: () =>
        targetsMissingText(
          this.rule(),
          this.kind(),
          { chosen: this.s.chosen().length, dealt: this.s.dealt(), darts: this.dartsTotal() },
          this.canApp,
        ),
      metaOptions: this.metaOptions,
      chosenMeta: this.s.chosenMeta(),
      picks: this.s.picks(),
      sorceryPoints: this.sorceryPoints,
    }),
  );
  readonly ready = computed(() => this.missing() === '');
  /** Magic Missile's count, in the footer next to the button: it states what is missing, and when all are placed. */
  readonly dartsLine = computed(() => {
    if (this.rule().kind !== 'darts' || this.dartsTotal() === 0) {
      return '';
    }
    return dartsStatus(this.dartsTotal(), this.s.dealt(), this.rule().rays);
  });

  constructor(
    private readonly s: CastState,
    private readonly area: CastArea,
    private readonly hooks: CastHooks,
    private readonly canApp: boolean,
    private readonly canTypeDamage: boolean,
  ) {
    this.grant = s.data.grant ?? null;
    this.asksAbility = needsAbility(s.data.spellKey);
    this.metaOptions = s.data.metamagic ?? [];
    this.sorceryPoints = s.data.sorceryPoints ?? null;
    this.sculptOffered = s.data.sculptSpells ?? false;
    // The lowest free slot is chosen, as the design draws it; an only target
    // and an only reachable dart target are chosen too.
    const first = this.grant ? null : defaultSlot(this.rows());
    if (first) {
      s.slot.set(first);
    }
    effect(() => this.keepFitting());
  }

  /** When the targets list or the slot changes the choice may no longer fit. */
  private keepFitting(): void {
    const r = this.rule();
    const ok = new Set(
      this.targetRows()
        .filter((t) => !t.blocked)
        .map((t) => t.id),
    );
    const kept = keptTargets(this.s.chosen(), ok, r.max);
    if (kept) {
      this.s.chosen.set(kept);
      untracked(() => this.hooks.changed());
    }
    if (r.kind === 'darts') {
      const dealt = dartsAfterChange(this.s.dealt(), [...ok], this.dartsTotal(), !!this.s.cast());
      if (dealt) {
        this.s.dealt.set(dealt);
        untracked(() => this.hooks.changed());
      }
    }
  }

  /** The Sculpt Spells ids the cast sends. */
  sculptedIds(): string[] {
    return sculptedIdsOf(this.s.sculpted(), this.sculptRows(), this.sculptLimit());
  }

  /** A choice was made: the cast being built changes, and the last error is no longer true. */
  private chose(): void {
    this.hooks.changed();
    this.s.error.set('');
  }

  toggleSculpt(id: string): void {
    this.s.sculpted.set(toggledSculpt(this.s.sculpted(), id, this.sculptLimit()));
    this.chose();
  }

  pickAbility(key: string): void {
    this.s.ability.set(key);
    this.chose();
  }

  pickDamageType(key: string): void {
    this.s.pickedType.set(key);
    this.chose();
  }

  /** The square the teleport takes the caster to, or `null` when the last one tried cannot be used. */
  pickDestination(square: Square | null): void {
    this.s.destination.set(square);
    this.chose();
  }

  /** "Sem espaço (usa a concessão)" on or off: off asks for a slot, the lowest free one first. */
  setFreeCast(on: boolean): void {
    this.s.freeCast.set(on);
    this.s.slot.set(on ? null : defaultSlot(this.rows()));
  }

  pickSlot(row: SlotRow): void {
    this.s.slot.set(row);
    this.chose();
  }

  toggleTable(key: string): void {
    this.s.tableTicked.set(toggledTable(this.s.tableTicked(), key));
    this.chose();
  }

  pickTableType(key: string, type: string): void {
    this.s.tableTypes.update((t) => ({ ...t, [key]: type }));
    this.chose();
  }

  toggleMeta(key: string): void {
    this.s.chosenMeta.set(toggledOption(this.s.chosenMeta(), key));
    this.chose();
  }

  setPicks(picks: MetamagicPicks): void {
    this.s.picks.set(picks);
    this.chose();
  }

  /** "Sem Metamagia": the spell is cast as it is. */
  noMetamagic(): void {
    this.s.chosenMeta.set([]);
    this.s.picks.set(NO_PICKS);
    this.hooks.changed();
  }

  toggle(id: string): void {
    this.s.listAsking.set(false);
    this.s.chosen.set(toggled(this.rule(), this.s.chosen(), id));
    // A creature no longer a target cannot stay the second target or the one with disadvantage.
    this.s.picks.update((p) => picksAfterTargets(p, this.s.chosen()));
    this.chose();
  }

  deal(change: { id: string; delta: 1 | -1 }): void {
    this.s.dealt.set(dealOne(this.s.dealt(), change.id, change.delta, this.dartsTotal()));
    this.chose();
  }
}
