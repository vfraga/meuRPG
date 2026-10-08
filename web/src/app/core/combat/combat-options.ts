import {
  type ActionOption,
  type Attack,
  AttackKind,
  type DisabledReason,
  DisabledReasonCode,
  type SpellOption,
  type TurnOptions,
} from '../../../gen/meurpg/rules/v1/rules_pb';
import { ActionEconomy } from '../../../gen/meurpg/rules/v1/rules_pb';
import { rechargeText } from './combat-errors';
import { circleLabel } from './combat-grid';
import { metersText } from '../units';
import { joinDots, tight } from '../format/text';

/**
 * What "Sua vez" says about the options the server works out
 * (`GetTurnOptions`): the Portuguese for every disabled reason, the line
 * under an attack, the groups by economy. Pure functions.
 */

export { circleLabel };

/** The reasons that read the same whatever the option, by code. */
const REASON_TEXT: Partial<Record<DisabledReasonCode, string>> = {
  [DisabledReasonCode.ACTION_USED]: 'Ação já usada',
  [DisabledReasonCode.BONUS_ACTION_USED]: 'Ação bônus já usada',
  [DisabledReasonCode.REACTION_USED]: 'Reação já usada',
  // Extra Attack: the attacks of this Attack action are all made.
  [DisabledReasonCode.ATTACKS_USED]: 'Ataques desta ação já usados',
  // Surto de ação: one use per turn, even with another use left.
  [DisabledReasonCode.ALREADY_USED_THIS_TURN]: 'Já usado neste turno',
  // Short on purpose: it repeats on every spell row, and the slot rows above
  // the list are the one explanation of which circles are out (E8-02).
  [DisabledReasonCode.NO_SLOT]: 'Sem espaço',
  // Escudo Arcano is listed on the character's own turn, where it cannot be
  // cast: it waits for a hit (the app asks then).
  [DisabledReasonCode.REACTION_ONLY_WHEN_HIT]: 'Só fora da sua vez',
  [DisabledReasonCode.REACTION_ONLY]: 'Só quando o gatilho acontecer',
  [DisabledReasonCode.CASTING_TIME_TOO_LONG]: 'Demora demais para um combate',
  [DisabledReasonCode.NOT_YOUR_TURN]: 'Não é a sua vez',
  [DisabledReasonCode.COMBAT_NOT_ACTIVE]: 'O combate não está em andamento',
  [DisabledReasonCode.COMBATANT_DOWN]: 'Caído: não pode agir',
  [DisabledReasonCode.COMBATANT_DEFEATED]: 'Derrotado: fora do combate',
};

/** Why an option is disabled, in words, by code. `''` when there is none. */
export function reasonText(reason: DisabledReason | undefined): string {
  if (!reason) {
    return '';
  }
  if (reason.code === DisabledReasonCode.NO_USES) {
    return `Sem usos: ${rechargeText(reason.recharge)}`;
  }
  return REASON_TEXT[reason.code] ?? 'Indisponível agora';
}

/** A reaction row has no button by design: "Escudo" waits to be hit. Its
 * reason is a hint, not a block, so the row doesn't read as disabled. */
export function isReactionHint(reason: DisabledReason | undefined): boolean {
  return (
    reason?.code === DisabledReasonCode.REACTION_ONLY_WHEN_HIT ||
    reason?.code === DisabledReasonCode.REACTION_ONLY
  );
}

/** "1d4 + 2 perfurante" for a weapon, "1d10 de fogo" for a cantrip: the dice
 * as the sheet writes them, spaced, and the damage type. */
export function damageText(attack: Attack): string {
  const dice = attack.damage.replace(
    /\s*([+-])\s*/g,
    (_, sign: string) => ` ${sign === '-' ? '−' : '+'} `,
  );
  const type = attack.damageTypePt;
  const adjective = /^(cortante|perfurante|contundente)$/.test(type);
  if (!type) {
    return dice;
  }
  // A cantrip's dice grow with the level and have no modifier: "de fogo".
  return /[+−]/.test(dice) || adjective ? `${dice} ${type}` : `${dice} de ${type}`;
}

/** "corpo a corpo" for a close attack, "alcance 36 m" for the rest. With
 * `reach`, a close attack says its reach too ("corpo a corpo, 1,5 m"). */
export function rangeText(attack: Attack, reach = false): string {
  if (isMelee(attack)) {
    return reach ? `corpo a corpo, ${metersText(attack.rangeFt || 5)}` : 'corpo a corpo';
  }
  return `alcance ${metersText(attack.rangeFt)}`;
}

/** A melee attack (a weapon with no range of its own): the only kind an
 * opportunity attack can be. */
export function isMelee(attack: Attack): boolean {
  return attack.rangeFt <= 5 && attack.longRangeFt === 0;
}

/** "+6 para acertar · 1d10 de fogo · alcance 36 m". */
export function attackDetail(attack: Attack, reach = false): string {
  // A cantrip that asks for a saving throw has no roll to hit: it has a DC.
  const bonus =
    attack.saveDc > 0
      ? `CD ${attack.saveDc}`
      : `${attack.attackBonus < 0 ? '−' : '+'}${Math.abs(attack.attackBonus)} para acertar`;
  return tight(joinDots([bonus, damageText(attack), rangeText(attack, reach)]));
}

/** "Cimitarra +5": the master's radio rows. */
export function attackTitle(attack: Attack): string {
  return `${attack.namePt || attack.name} ${attack.attackBonus < 0 ? '−' : '+'}${Math.abs(attack.attackBonus)}`;
}

/** An attack's name for a button and a sentence: the server's Portuguese name (a creature's attacks included), the English one when there is none. */
export function attackName(attack: Attack): string {
  return attack.namePt || attack.name;
}

export function isCantrip(attack: Attack): boolean {
  return attack.kind === AttackKind.SPELL;
}

/** The tags under a spell's name: its circle, and the economy when it is not
 * an action ("Reação", "Ação bônus"): the spells of every economy share one
 * list in the server's order (E8-02). */
export function spellTags(o: SpellOption): string[] {
  const tags = [circleLabel(o.spell?.level ?? 0)];
  if (o.spell?.concentration) {
    tags.push('Concentração');
  }
  if (o.spell?.ritual) {
    tags.push('Ritual');
  }
  if (o.economy === ActionEconomy.BONUS_ACTION) {
    tags.push('Ação bônus');
  } else if (o.economy === ActionEconomy.REACTION) {
    tags.push('Reação');
  }
  return tags;
}

/** What the "Reação" spell does for the line under its name: Escudo asks when
 * a hit lands, so it says so instead of its school. */
export function spellLine(o: SpellOption, summary = ''): string {
  if (o.spell?.key === 'spell:shield') {
    return 'Quando você for atingido, o app pergunta se quer usar.';
  }
  const parts = [summary || o.spell?.schoolNamePt || ''];
  const level = o.spell?.level ?? 0;
  const free = o.slots.reduce((sum, s) => sum + s.free, 0);
  if (level > 0 && o.enabled && free === 1) {
    parts.push(`gasta o último espaço de ${circleLabel(o.slots[0].level)}`);
  }
  return tight(joinDots(parts.filter(Boolean)));
}

/** The economy a group of the screen stands for. */
export type EconomyGroup = 'action' | 'bonus' | 'reaction';

export function groupOf(economy: ActionEconomy): EconomyGroup | null {
  switch (economy) {
    case ActionEconomy.ACTION:
      return 'action';
    case ActionEconomy.BONUS_ACTION:
      return 'bonus';
    case ActionEconomy.REACTION:
      return 'reaction';
    default:
      return null;
  }
}

/** The options of one economy, split the way the screen draws them. */
export interface GroupOptions {
  readonly spells: readonly SpellOption[];
  readonly features: readonly ActionOption[];
}

export function optionsFor(options: TurnOptions, group: EconomyGroup): GroupOptions {
  return {
    spells: options.spells.filter((s) => groupOf(s.economy) === group),
    // A feature that costs nothing (Surto de Ação) is used during the turn: it
    // stands with the Ação's abilities rather than in a group of its own.
    features: options.featureActions.filter((a) => {
      const economy = a.action?.economy ?? 0;
      return groupOf(economy) === group || (group === 'action' && economy === ActionEconomy.FREE);
    }),
  };
}

/** The word on a group's pill. */
export function groupState(used: boolean): string {
  return used ? 'Usada' : 'Disponível';
}

/** "Encerrar turno" is outlined while the action or the bonus action is
 * still available, and the filled button once both are used (timeline.md,
 * shared decision 2). */
export function endTurnIsPrimary(own: { actionUsed: boolean; bonusActionUsed: boolean }): boolean {
  return own.actionUsed && own.bonusActionUsed;
}
