import { computed } from '@angular/core';

import { AreaPlacement, type Combatant } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { ActionEconomy, SpellAreaShape } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { saveAbility } from '../../../../core/combat/cast-flow';
import { AreaFlow } from '../../../../core/combat/area-flow';
import {
  allyWarning,
  areaRows,
  confirmLabel,
  countText,
  nobodyText,
  notPlacedReason,
  stepOneTitle,
} from '../../../../core/combat/area-text';
import type { CombatClient } from '../../../../core/combat/combat-client';
import type { AreaShape } from '../../../../core/combat/spell-area';
import { ActionKey } from '../../../../core/connect/idempotency';
import type { CastState } from './cast-state';
import { coverNoteOf } from './cast-sheet-view';

/** The area of a spell placed on the map (PM-02a, PM-02b): where it goes, who is inside and what the sheet says about it. */
export class CastArea {
  /** How the spell's area is placed: only on a map with a grid; without one every area spell keeps its list. */
  readonly placement: AreaPlacement;
  /** The two steps of the area, for a placed area spell; `null` for any other cast. */
  readonly flow: AreaFlow | null;
  readonly shape: AreaShape;
  readonly stepOne: string;
  readonly confirmText: string;
  readonly notPlaced: string;
  /** One key for the cast of a placed area: its request is the same while the slot, place and roll are. */
  readonly key = new ActionKey();
  readonly combatants = computed<readonly Combatant[]>(
    () => this.s.data.state.encounter()?.combatants ?? [],
  );
  private readonly casterLabel = computed(
    () => this.combatants().find((c) => c.id === this.s.data.casterId)?.label ?? '',
  );
  readonly rows = computed(() => {
    const p = this.flow?.preview();
    return p
      ? areaRows(p.targets, p.coverCounts, saveAbility(this.s.details()), this.placement)
      : [];
  });
  readonly count = computed(() => countText(this.flow?.preview()?.targets ?? []));
  readonly warning = computed(() => {
    const p = this.flow?.preview();
    return p ? allyWarning(p.targets, this.s.data.name, this.casterLabel()) : null;
  });
  /** "Ninguém que você vê está na área." (on the map) or "Ninguém está marcado." (the list), with what it spends. */
  readonly nobodyLine = computed(() =>
    nobodyText(
      !!this.flow,
      this.s.data.level > 0 ? this.s.slotLevel() : 0,
      this.s.data.economy === ActionEconomy.BONUS_ACTION,
    ),
  );
  readonly coverNote = computed(() => coverNoteOf(this.flow?.preview(), this.s.data.name));
  /** The area waits for the caster's "Conjurar mesmo assim" (nobody in it) before the cast can be sent. */
  readonly asks = computed(() => !!this.flow && this.flow.nobody() && !this.flow.nobodyAccepted());

  constructor(
    private readonly s: CastState,
    api: CombatClient,
  ) {
    const { data } = s;
    this.placement = data.map && data.targets ? data.targets.placement : AreaPlacement.UNSPECIFIED;
    this.flow =
      this.placement === AreaPlacement.UNSPECIFIED
        ? null
        : new AreaFlow(this.placement, (area) =>
            api.casts.previewSpellArea(
              data.campaignId,
              data.encounterId,
              data.casterId,
              data.spellKey,
              this.slotRef(),
              area,
            ),
          );
    this.shape = {
      shape: data.targets?.areaShape ?? SpellAreaShape.UNSPECIFIED,
      sizeFt: data.targets?.areaSizeFt ?? 0,
      widthFt: data.targets?.areaWidthFt ?? 0,
    };
    this.stepOne = stepOneTitle(this.placement);
    this.confirmText =
      this.placement === AreaPlacement.CASTER
        ? 'Ver quem está na área'
        : confirmLabel(this.placement);
    this.notPlaced = notPlacedReason(this.placement);
  }

  /** The slot of the cast, as the requests take it. */
  slotRef(): { level: number; pact: boolean } | null {
    const slot = this.s.slot();
    return this.s.data.level > 0 && slot ? { level: slot.level, pact: slot.pact } : null;
  }
}
