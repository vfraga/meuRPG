import {
  type ActionOption,
  type Attack,
  type AttackOption,
  AttackKind,
  BonusAttackRule,
  type DisabledReason,
  DisabledReasonCode,
  type SpellGrantOption,
  type SpellOption,
  type TurnOptions,
} from '../../../gen/meurpg/rules/v1/rules_pb';
import { ActionEconomy } from '../../../gen/meurpg/rules/v1/rules_pb';
import { type MarksVm, markWords } from '../characters/gear';
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
  // A bonus action spell leaves no other spell this turn but a cantrip of 1 action.
  [DisabledReasonCode.BONUS_ACTION_SPELL_LIMIT]: 'Só um truque junto com uma magia de ação bônus',
  // Rajada de Golpes comes right after the Attack action.
  [DisabledReasonCode.ATTACK_ACTION_FIRST]: 'Só depois de atacar com a ação',
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
  // A surprised combatant does not move, act or react until its first turn ends (W7-X, SRD 5.1, Surprise).
  [DisabledReasonCode.SURPRISED]: 'Surpresa',
  // A grappled combatant's speed is 0 (SRD 5.1, Conditions); the turn's own line says it by the character's gender.
  [DisabledReasonCode.GRAPPLED]: 'Em um agarrão',
  // An effect or a condition writes its own sentence (`text_pt`); these are the words if it came without.
  [DisabledReasonCode.INCAPACITATED]: 'Você não pode agir.',
  [DisabledReasonCode.EFFECT_LETHARGY]: 'Indisponível por um efeito',
  [DisabledReasonCode.SPEED_ZERO]: 'Indisponível: deslocamento 0',
  // Armor or a shield without proficiency stops every spell (SRD 5.1, "Armor").
  [DisabledReasonCode.ARMOR_NOT_PROFICIENT]: 'Armadura ou escudo sem proficiência',
  // A wall needs its shape, size and damaging side; the cast sheet's "Onde fica" sends them, so the server no longer lists the
  // spell with this reason. It stays so an answer that carries it still reads.
  [DisabledReasonCode.WALL_PLACEMENT_REQUIRED]: 'Falta dizer onde fica a muralha',
};

/** Why an option is disabled, in words, by code. `''` when there is none. */
export function reasonText(reason: DisabledReason | undefined, master = false): string {
  if (!reason) {
    return '';
  }
  // An effect, a condition or a spell writes its own sentence (RN-22), already without a cause the caller may not read.
  const own = master ? reason.textMasterPt || reason.textPt : reason.textPt;
  if (own) {
    return own;
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

/** Whether an attack is made with the bonus action now (off hand, Artes Marciais, Rajada de Golpes, Frenesi). */
export function isBonusAttack(o: AttackOption): boolean {
  return o.bonusRule !== BonusAttackRule.UNSPECIFIED;
}

/** The line under a bonus action attack that can be made now: which rule makes
 * it one. `''` for an attack of the action, and for one that cannot be made
 * (its reason says why). */
export function bonusAttackLine(o: AttackOption): string {
  if (!o.enabled) {
    return '';
  }
  switch (o.bonusRule) {
    case BonusAttackRule.OFF_HAND:
      return o.bonusDropsModifier
        ? 'Ataque com a outra mão, sem o modificador no dano'
        : 'Ataque com a outra mão';
    case BonusAttackRule.MARTIAL_ARTS:
      return 'Golpe desarmado das Artes Marciais';
    case BonusAttackRule.FLURRY_OF_BLOWS:
      return `Rajada de Golpes: ${flurryLeftText(o.bonusAttacksLeft)}`;
    case BonusAttackRule.FRENZY:
      return 'Frenesi: ataque corpo a corpo como ação bônus';
    default:
      return '';
  }
}

/** What a bonus action attack spent, once it is made: the bonus action, or one
 * of the strikes of Rajada de Golpes (`left` is how many there were before it).
 * `''` for an attack of the action. */
export function bonusSpentText(rule: BonusAttackRule | undefined, left = 1): string {
  switch (rule) {
    case BonusAttackRule.FLURRY_OF_BLOWS:
      return left > 1
        ? `Rajada de Golpes: ${flurryLeftText(left - 1)}.`
        : 'Rajada de Golpes: acabaram os golpes.';
    case BonusAttackRule.OFF_HAND:
    case BonusAttackRule.MARTIAL_ARTS:
    case BonusAttackRule.FRENZY:
      return 'Sua ação bônus foi usada.';
    default:
      return '';
  }
}

/** "Rajada Mística: 2 raios restantes": the beams of a cantrip cast (Eldritch
 * Blast) still to fire, once the first one spent the action. `''` otherwise. */
export function beamsLeftLine(o: AttackOption): string {
  if (!o.enabled || o.beamsLeft <= 0 || !o.attack) {
    return '';
  }
  return `${attackName(o.attack)}: ${o.beamsLeft === 1 ? '1 raio restante' : `${o.beamsLeft} raios restantes`}`;
}

/** What a beam spent, once it is fired: the beams still to fire, or the action
 * with the last one. `before` is how many beams were to fire before this one
 * (the option's `beamsLeft`, or the attack's `beams` for the first). */
export function beamSpentText(name: string, before: number): string {
  const left = before - 1;
  if (left <= 0) {
    return 'Sua ação foi usada.';
  }
  return `${name}: ${left === 1 ? '1 raio restante' : `${left} raios restantes`}.`;
}

/** The key of the monk's ki resource (`Action.resource_key`): its features spend 1 ki point each use. */
const KI_RESOURCE = 'ki';

/**
 * What a feature row says about its resource. A ki feature (Rajada de Golpes, Defesa Paciente, Passo do Vento)
 * says what one use costs and what is left, in ki points, not "N usos"; any other feature says its uses left.
 * `''` when there is nothing to say.
 */
export function featureUsesText(resourceKey: string, usesLeft: number): string {
  if (resourceKey === KI_RESOURCE) {
    return usesLeft > 0 ? `Gasta 1 ki · restam ${usesLeft}` : 'Gasta 1 ki · sem ki';
  }
  return usesLeft > 0 ? `${usesLeft} ${usesLeft === 1 ? 'uso' : 'usos'}` : '';
}

/** "2 golpes restantes", "1 golpe restante": the unarmed strikes of Rajada de Golpes still to make. */
export function flurryLeftText(left: number): string {
  return left === 1 ? '1 golpe restante' : `${left} golpes restantes`;
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

/** Where a granted spell comes from and what it costs: "Graça · À vontade", "Graça · 1 uso", "Graça · gasta 2 · 3 usos". */
export function grantLine(g: SpellGrantOption): string {
  const left = g.usesLeft === 1 ? '1 uso' : `${g.usesLeft} usos`;
  const uses = g.atWill ? 'À vontade' : g.usesCost > 1 ? `gasta ${g.usesCost} · ${left}` : left;
  return `${g.sourceNamePt} · ${uses}`;
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
  // A granted spell says where it comes from before anything else: the same spell may stand twice in the list.
  const parts = [o.grant ? grantLine(o.grant) : '', summary || o.spell?.schoolNamePt || ''];
  const level = o.spell?.level ?? 0;
  const free = o.slots.reduce((sum, s) => sum + s.free, 0);
  if (level > 0 && o.enabled && free === 1 && !o.grant) {
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

/** The tags under an attack's name: "Truque" for a cantrip (its economy has its own mark). */
export function attackTagsOf(o: AttackOption): string[] {
  return o.attack && isCantrip(o.attack) ? ['Truque'] : [];
}

/** The marks of a weapon attack, for its chip beside the name; none for a spell or a creature's attack. */
export function attackMarksOf(a: Attack | undefined): MarksVm | null {
  return a?.kind === AttackKind.WEAPON && markWords(a).length > 0
    ? { silvered: a.silvered, adamantine: a.adamantine, magical: a.magical }
    : null;
}
