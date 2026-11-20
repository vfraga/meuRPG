import { Component, computed, input } from '@angular/core';

import { aidLabel, aidTag } from '../../../core/combat/combat-effects';
import { EffectPill } from '../../../shared/effect-pill/effect-pill';
import { formatModifier } from '../../../core/characters/character-labels';
import { metersText, squaresWithFeet } from '../../../core/units';

/**
 * The combat numbers of the paper sheet, as one `<dl>`: the Armour Class in
 * the garnet shield, the Iniciativa box, the Deslocamento box (full width, on
 * one line: "7,5 m · 5 quadrados (25 pés)", E8-01), then the hit point
 * maximum (garnet outline) and the hit dice. A basic NPC sheet has no
 * initiative or hit dice, so those inputs are `null` there and their boxes
 * are left out. While the campaign's session is live and Ajuda raises the maximum, the box shows the
 * current hit points over the sheet's own maximum, with the spell's tag and label (PM-03a).
 */
@Component({
  selector: 'app-combat-stats',
  imports: [EffectPill],
  templateUrl: './combat-stats.html',
  styleUrl: './combat-stats.scss',
})
export class CombatStats {
  readonly armorClass = input.required<number>();
  readonly initiative = input<number | null>(null);
  readonly speedFt = input.required<number>();
  /** The other speeds and "Respira na água", one line each under the walking speed. */
  readonly extras = input<readonly string[]>([]);
  readonly hitPointsMax = input.required<number>();
  /** The current hit points from the live session; `null` outside one (the box then says they appear in the session). */
  readonly hitPointsCurrent = input<number | null>(null);
  /** What Ajuda adds to `hitPointsMax` (0 without it); the effective numbers show only with it. */
  readonly aid = input(0);
  readonly hitDice = input<string | null>(null);
  /** The equipped shield does not count (a two-handed weapon is equipped): the sentence sits right under the shield's row. */
  readonly shieldNote = input(false);

  protected readonly formatModifier = formatModifier;
  protected readonly aidTag = aidTag;
  protected readonly aidLabel = aidLabel;
  /** The maximum Ajuda makes of the sheet's: "de 43" over "máximo 38 da ficha". */
  protected readonly effectiveMax = computed(() => this.hitPointsMax() + this.aid());
  /** The meters in the display type, then the squares and the feet in the small text after it. */
  protected readonly speed = computed(() => ({
    meters: metersText(this.speedFt()),
    rest: squaresWithFeet(this.speedFt()),
  }));
}
