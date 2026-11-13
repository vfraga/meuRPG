import { computed } from '@angular/core';

import { AreaPlacement, type Combatant } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { ActionEconomy, SpellAreaShape } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { damageDice, saveAbility } from '../../../../core/combat/cast-flow';
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
import type { CastExtras } from '../../../../core/combat/combat-client-types';
import type { AreaShape } from '../../../../core/combat/spell-area';
import { WALL_NEXT, WALL_STEPS, laysWall } from '../../../../core/combat/spell-wall';
import { wallSummary } from '../../../../core/combat/wall-summary';
import { ActionKey } from '../../../../core/connect/idempotency';
import type { CastState } from './cast-state';
import { coverNoteOf } from './cast-sheet-view';

/** What the two steps say: a wall has its own words and a strip of both steps; any other area keeps one heading at a time. */
function stepWords(wall: boolean, placement: AreaPlacement) {
  if (wall) {
    return {
      one: WALL_STEPS[0],
      two: WALL_STEPS[1],
      all: WALL_STEPS,
      next: WALL_NEXT,
      notPlaced: 'Toque no mapa para marcar onde a muralha fica.',
    };
  }
  return {
    one: stepOneTitle(placement),
    two: 'Quem está na área',
    all: [] as readonly string[],
    next: placement === AreaPlacement.CASTER ? 'Ver quem está na área' : confirmLabel(placement),
    notPlaced: notPlacedReason(placement),
  };
}

/** The area of a spell placed on the map (PM-02a, PM-02b): where it goes, who is inside and what the sheet says about it. */
export class CastArea {
  /** How the spell's area is placed: only on a map with a grid; without one every area spell keeps its list. */
  readonly placement: AreaPlacement;
  /** The two steps of the area, for a placed area spell; `null` for any other cast. */
  readonly flow: AreaFlow | null;
  readonly shape: AreaShape;
  readonly stepOne: string;
  /** The second step's title: "Quem está nela" for a wall, "Quem está na área" for any other area. */
  readonly stepTwo: string;
  /** Every step's title, for the wall's strip of two steps; empty for the other areas, which show one heading at a time. */
  readonly steps: readonly string[];
  readonly confirmText: string;
  readonly notPlaced: string;
  /** Why the step's button is off: nothing placed yet, or (a wall) no side picked. */
  readonly notReady = computed(() =>
    this.flow?.wall() && this.flow.placed() ? 'Escolha o lado que queima.' : this.notPlaced,
  );
  /** The ring's card under its controls, from the server's preview; not while a newer one is on its way. */
  readonly summary = computed(() =>
    this.flow?.loading()
      ? null
      : wallSummary(
          this.flow?.wall() ?? null,
          this.flow?.preview() ?? null,
          saveAbility(this.s.details()),
        ),
  );
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
  /** The dice of the burning side at the slot ("5d8"), for the wall's note. */
  readonly dice = computed(() =>
    damageDice(this.s.details(), this.s.data.level > 0 ? this.s.slotLevel() : 0),
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
        : new AreaFlow(
            this.placement,
            (area, wall) =>
              api.casts.previewSpellArea(
                data.campaignId,
                data.encounterId,
                data.casterId,
                data.spellKey,
                this.slotRef(),
                area,
                ...(data.grant || wall ? [data.grant?.sourceKey ?? ''] : []),
                ...(wall ? [wall] : []),
              ),
            laysWall(data.spellKey),
          );
    this.shape = {
      shape: data.targets?.areaShape ?? SpellAreaShape.UNSPECIFIED,
      sizeFt: data.targets?.areaSizeFt ?? 0,
      widthFt: data.targets?.areaWidthFt ?? 0,
    };
    const words = stepWords(!!this.flow?.wall(), this.placement);
    this.stepOne = words.one;
    this.stepTwo = words.two;
    this.steps = words.all;
    this.confirmText = words.next;
    this.notPlaced = words.notPlaced;
  }

  /** What the cast carries for a placed area: the area, and the wall of a spell that lays one. */
  extras(): CastExtras {
    const wall = this.flow?.wall();
    return this.flow ? { area: this.flow.choice(), ...(wall ? { wall } : {}) } : {};
  }

  /** The slot of the cast, as the requests take it. */
  slotRef(): { level: number; pact: boolean } | null {
    const slot = this.s.slot();
    return this.s.data.level > 0 && slot ? { level: slot.level, pact: slot.pact } : null;
  }
}
