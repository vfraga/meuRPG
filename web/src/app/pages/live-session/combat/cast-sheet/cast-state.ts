import { computed, signal } from '@angular/core';

import type { Square } from '../../../../core/combat/combat-grid';
import type { SpellCast } from '../../../../../gen/meurpg/play/v1/combat_pb';
import type { RollMode } from '../../../../../gen/meurpg/play/v1/combat_rolls_pb';
import type { SpellDetails } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { castLevelOf, type Dealt, type SlotRow } from '../../../../core/combat/cast-flow';
import { tableOptionPicks } from '../../../../core/combat/table-cast-options';
import { type MetamagicPicks, NO_PICKS } from '../../../../core/resources/metamagic';
import type { CastSheetData } from './cast-sheet-types';

/** What was chosen and what the cast sheet is showing, shared by the parts that read and write it. */
export class CastState {
  readonly details = signal<SpellDetails | null>(null);
  readonly detailsFailed = signal(false);
  readonly slot = signal<SlotRow | null>(null);
  /** A granted spell that also allows a slot is cast through the grant (no slot) until the caster asks for a slot. */
  readonly freeCast = signal(true);
  readonly chosen = signal<string[]>([]);
  readonly dealt = signal<Dealt>(new Map());
  readonly typing = signal(false);
  /** The mode of a spell attack's d20 the caster picked; null leaves it to the app's suggestion for each target. */
  readonly picked = signal<RollMode | null>(null);
  readonly reason = signal('');
  readonly busy = signal(false);
  readonly error = signal('');
  readonly cast = signal<SpellCast | null>(null);
  /** The Metamagic options marked: at most one, or Empowered Spell with one other; each may ask for creatures (`picks`). */
  readonly chosenMeta = signal<string[]>([]);
  readonly picks = signal<MetamagicPicks>(NO_PICKS);
  /** Aprimorar Habilidade names the ability it is cast for; empty until picked, and for every other spell. */
  readonly ability = signal('');
  /** "Estrela também": the cast reaches the steed the caster rides too; on until the caster takes it off. */
  readonly alsoSteed = signal(true);
  /** The steed's name, as the combat lists the combatant the caster rides; empty when it rides none, or a mount that is not its bonded steed. */
  readonly steedName = computed(() => {
    const all = this.data.state.encounter()?.combatants ?? [];
    const on = all.find((c) => c.id === this.data.casterId)?.mountedOnId;
    const mount = all.find((c) => c.id === on);
    const bonded = this.data.bondedSteedId ?? '';
    return mount && bonded !== '' && mount.creatureId === bonded ? mount.label : '';
  });
  /** The caster got off (or was thrown) while the sheet was open: the spell is the caster's alone again. */
  readonly steedOff = computed(() => {
    const all = this.data.state.encounter()?.combatants ?? [];
    return !!this.data.alsoSteed && !all.find((c) => c.id === this.data.casterId)?.mountedOnId;
  });
  /** The picked damage type; the first of the spell's choices until the caster picks another. */
  readonly pickedType = signal('');
  readonly sculpted = signal<string[]>([]);
  /** The table features' cast options ticked, and the damage type picked for each swap (by the option's key). */
  readonly tableTicked = signal<string[]>([]);
  readonly tableTypes = signal<Readonly<Record<string, string>>>({});
  /** What the cast sends for them. */
  readonly tablePicks = computed(() =>
    tableOptionPicks(this.data.tableOptions ?? [], this.tableTicked(), this.tableTypes()),
  );
  /** The square a teleport spell takes the caster to, picked on the map; `null` until picked. */
  readonly destination = signal<Square | null>(null);
  /** How far the spell teleports the caster on the combat's map, in feet; 0 for any other spell and without a grid. */
  readonly teleportFt: number;
  /** A list spell that may hit nobody, with nobody ticked: "Conjurar" asks in place first (PM-02d state 13). */
  readonly listAsking = signal(false);
  /** The cast goes through the grant, with no slot: a grant with no slot allowed, or one the caster did not ask a slot for. */
  readonly viaGrant = computed(
    () => !!this.data.grant && (!this.data.grant.alsoWithSlot || this.freeCast()),
  );
  /** The level the cast scales at: the slot's, else the level the grant casts at (a cast through the grant spends no slot), else the spell's own. */
  readonly slotLevel = computed(() =>
    castLevelOf(this.slot(), this.viaGrant() ? this.data.grant : null, this.data.level),
  );
  /** The level the targets of the sheet's entry were listed at: the grant's cast level for a grant, else the spell's own. */
  readonly entryLevel = computed(() => castLevelOf(null, this.data.grant, this.data.level));
  /** Whether the cast happened (or is being resumed): the result stage. */
  readonly done = computed(() => this.cast() !== null || !!this.data.resume);

  constructor(readonly data: CastSheetData) {
    this.teleportFt = data.map ? (data.targets?.teleportRangeFt ?? 0) : 0;
    if (data.preset) {
      this.chosen.set([...data.preset.chosen]);
      this.dealt.set(new Map(data.preset.dealt));
    }
  }
}
