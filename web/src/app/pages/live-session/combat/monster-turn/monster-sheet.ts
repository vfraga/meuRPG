import { ChangeDetectionStrategy, Component, computed, input } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import type { Combatant } from '../../../../../gen/meurpg/play/v1/combat_pb';
import type { CreatureTurn } from '../../../../../gen/meurpg/play/v1/creatures_pb';
import type {
  CreatureBonus,
  CreatureDamageModifier,
} from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { alignmentPt, capitalized } from '../../../../core/creatures/bestiary-format';
import { challengeText, speedsText } from '../../../../core/creatures/creature-format';
import { joinDots, formatXp, tieNumbers } from '../../../../core/format/text';
import { legendaryName } from '../../../../core/combat/creature-turn';
import { metersText } from '../../../../core/units';
import { CombatantToken } from '../../../../shared/combatant-token/combatant-token';

/** The letters of an ability's abbreviation ("FOR", "DES"). */
const ABBREVIATION = 3;

const signed = (n: number): string => `${n < 0 ? '−' : '+'}${Math.abs(n)}`;

const bonuses = (list: readonly CreatureBonus[]): string =>
  joinDots(list.map((b) => `${b.namePt} ${signed(b.bonus)}`));

const modifiers = (list: readonly CreatureDamageModifier[]): string =>
  list.length
    ? list
        .map(
          (m) =>
            m.types.map((t) => t.namePt.toLowerCase()).join(', ') + (m.note ? ` ${m.note}` : ''),
        )
        .join('; ')
    : 'nenhuma';

/**
 * "Ficha do monstro" (W7-M): the stat block of the monster on turn, for the master. It reads
 * what `GetCreatureTurn` sends (the SRD block and the sheet the rolls read), with the hit points
 * of the combat; what the engine does not apply is a dashed card ("Lembrete: o app não aplica
 * este texto"), never hidden. A player never gets any of it (RN-10, RN-20).
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-monster-sheet',
  imports: [CombatantToken, MatIconModule],
  templateUrl: './monster-sheet.html',
  styleUrl: './monster-sheet.scss',
})
export class MonsterSheet {
  readonly turn = input.required<CreatureTurn>();
  readonly subject = input.required<Combatant>();

  protected readonly creature = computed(() => this.turn().creature);
  protected readonly initial = computed(() =>
    (this.subject().label || '?').charAt(0).toUpperCase(),
  );
  protected readonly name = computed(
    () =>
      this.creature()?.summary?.namePt || this.creature()?.summary?.name || this.subject().label,
  );
  protected readonly subtitle = computed(() => {
    const c = this.creature();
    const s = c?.summary;
    if (!c || !s) {
      return '';
    }
    return joinDots(
      [
        capitalized(s.sizePt),
        s.typePt,
        alignmentPt(c.alignment).text,
        `${challengeText(s.challengeRating)} (${formatXp(s.xp).replace(/ XP$/, '')} XP)`,
      ].filter(Boolean),
    );
  });
  protected readonly hp = computed(() => ({
    now: this.subject().hitPointsCurrent ?? this.creature()?.hitPoints ?? 0,
    max: this.subject().hitPointsMax ?? this.creature()?.hitPoints ?? 0,
  }));
  protected readonly speeds = computed(() => {
    const c = this.creature();
    return c ? speedsText(c) : '';
  });
  protected readonly saves = computed(() => bonuses(this.creature()?.savingThrows ?? []));
  protected readonly skills = computed(() => bonuses(this.creature()?.skills ?? []));
  protected readonly immunities = computed(() => modifiers(this.turn().sheet?.immunities ?? []));
  protected readonly resistances = computed(() => modifiers(this.turn().sheet?.resistances ?? []));
  protected readonly vulnerabilities = computed(() =>
    modifiers(this.turn().sheet?.vulnerabilities ?? []),
  );
  protected readonly conditionImmunities = computed(() => {
    const list = this.turn().sheet?.conditionImmunities ?? [];
    return list.length ? list.map((c) => c.namePt.toLowerCase()).join(', ') : 'nenhuma';
  });
  protected readonly senses = computed(() => {
    const c = this.creature();
    if (!c) {
      return '';
    }
    return joinDots([
      ...c.senses.map((s) => tieNumbers(`${s.namePt.toLowerCase()} ${metersText(s.rangeFt)}`)),
      `Percepção passiva ${c.passivePerception}`,
    ]);
  });
  protected readonly legendary = computed(() => this.turn().legendary);
  protected readonly legendaryOptions = computed(() =>
    (this.legendary()?.options ?? []).map((o) => ({
      key: o.key,
      name: legendaryName(o),
      text: o.text,
    })),
  );
  protected readonly resistance = computed(() => {
    const st = this.legendary()?.state;
    return st && st.resistanceUses > 0 ? st : null;
  });
  protected readonly traits = computed(() => this.turn().sheet?.traits ?? []);
  protected readonly casting = computed(() => this.turn().sheet?.spellcasting ?? []);
  protected readonly abilities = computed(() => this.creature()?.abilities ?? []);

  protected spellNames(list: readonly { name: string; namePt: string }[]): string {
    return list.map((s) => s.namePt || s.name).join(', ');
  }

  protected abbreviation(namePt: string): string {
    return namePt.slice(0, ABBREVIATION).toUpperCase();
  }
}
