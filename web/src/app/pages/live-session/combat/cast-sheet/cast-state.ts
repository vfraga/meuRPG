import { computed, signal } from '@angular/core';

import type { Square } from '../../../../core/combat/combat-grid';
import type { SpellCast } from '../../../../../gen/meurpg/play/v1/combat_pb';
import type { RollMode } from '../../../../../gen/meurpg/play/v1/combat_rolls_pb';
import type { SpellDetails } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import type { Dealt, SlotRow } from '../../../../core/combat/cast-flow';
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
  /** "Estrela também" (#588): the cast reaches the steed the caster rides too; on until the caster takes it off. */
  readonly alsoSteed = signal(true);
  /** The steed's name, as the combat lists the combatant the caster rides; empty when it rides none. */
  readonly steedName = computed(() => {
    const all = this.data.state.encounter()?.combatants ?? [];
    const on = all.find((c) => c.id === this.data.casterId)?.mountedOnId;
    return all.find((c) => c.id === on)?.label ?? '';
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
  readonly slotLevel = computed(() => this.slot()?.level ?? this.data.level);
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
