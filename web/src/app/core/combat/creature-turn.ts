import { Ability } from '../../../gen/meurpg/rules/v1/rules_pb';
import {
  type CreatureAction,
  type CreatureDamagePart,
  type CreatureDamageRoll,
  type CreatureSavePlan,
  CreatureActionKind,
  CreatureActionStepKind,
  CreatureUsageKind,
} from '../../../gen/meurpg/play/v1/creatures_pb';
import { joinDots, tight } from '../format/text';
import { metersText } from '../units';

/**
 * What a monster's turn says (W7-M): the line under each action of the stat block, its limit
 * ("Recarga 5–6 · Disponível"), the legendary options and the damage of an attack part by part.
 * Words only: what an action does, and whether it can be used, come from the server
 * (`CreatureAction`).
 */

/** The faces of the d6 a recharge rolls. */
const D6_FACES = 6;

const ABILITY_PT: Partial<Record<Ability, string>> = {
  [Ability.STRENGTH]: 'Força',
  [Ability.DEXTERITY]: 'Destreza',
  [Ability.CONSTITUTION]: 'Constituição',
  [Ability.INTELLIGENCE]: 'Inteligência',
  [Ability.WISDOM]: 'Sabedoria',
  [Ability.CHARISMA]: 'Carisma',
};

export function abilityPt(a: Ability): string {
  return ABILITY_PT[a] ?? '';
}

/** The damage types said as an adjective after the dice ("2d6 + 8 cortante"); the others take "de" ("2d6 de fogo"). */
const ADJECTIVE_TYPES = new Set([
  'damage-type:bludgeoning',
  'damage-type:piercing',
  'damage-type:slashing',
]);

/** "2d10+8" as the table writes it, "2d10 + 8". */
export function diceText(dice: string): string {
  return dice
    .replace(/([+-])/g, ' $1 ')
    .replace(/\s+/g, ' ')
    .trim();
}

/** A damage part: "2d10 + 8 perfurante", "2d6 de fogo". */
export function partText(
  p: Pick<CreatureDamagePart, 'dice' | 'damageTypeKey' | 'damageTypePt'>,
): string {
  const type = p.damageTypePt.toLowerCase();
  if (!type) {
    return diceText(p.dice);
  }
  return `${diceText(p.dice)} ${ADJECTIVE_TYPES.has(p.damageTypeKey) ? '' : 'de '}${type}`;
}

/** The parts as a list: "2d10 + 8 perfurante e 2d6 de fogo". */
export function partsText(parts: readonly CreatureDamagePart[]): string {
  const texts = parts.map(partText);
  return texts.length > 1
    ? `${texts.slice(0, -1).join(', ')} e ${texts[texts.length - 1]}`
    : (texts[0] ?? '');
}

const SHAPE_PT: Record<string, string> = {
  cone: 'Cone',
  line: 'Linha',
  sphere: 'Esfera',
  cube: 'Cubo',
  cylinder: 'Cilindro',
  radius: 'Raio',
};

function areaText(save: CreatureSavePlan): string {
  const area = save.area;
  if (!area) {
    return '';
  }
  const shape = SHAPE_PT[area.shape] ?? area.shape;
  const width = area.widthFt > 0 ? ` × ${metersText(area.widthFt)}` : '';
  return tight(`${shape} de ${metersText(area.lengthFt)}${width}`);
}

/** The saving throw an action asks: "teste de resistência de Destreza CD 21 · 18d6 de fogo, metade se passar". */
export function saveText(save: CreatureSavePlan): string {
  const parts = [
    areaText(save),
    `teste de resistência de ${abilityPt(save.ability)} CD ${save.dc}`,
  ];
  if (save.damage.length) {
    parts.push(`${partsText(save.damage)}${save.halfOnSuccess ? ', metade se passar' : ''}`);
  }
  if (save.conditionKey) {
    parts.push(
      `${save.conditionPt}${save.duration ? ` por ${save.duration}` : ''}${save.repeatSave ? ' (repete o teste ao fim de cada vez dele)' : ''}`,
    );
  }
  return joinDots(parts.filter(Boolean));
}

function reachText(a: CreatureAction): string {
  if (a.melee) {
    return `alcance ${metersText(a.reachFt)}`;
  }
  return a.longRangeFt > 0
    ? `alcance ${metersText(a.rangeFt)}/${metersText(a.longRangeFt)}`
    : `alcance ${metersText(a.rangeFt)}`;
}

/** The line under an action: what it rolls and what it deals, from the stat block. */
export function actionLine(a: CreatureAction): string {
  switch (a.kind) {
    case CreatureActionKind.ATTACK: {
      const bonus = `${a.attackBonus < 0 ? '−' : '+'}${Math.abs(a.attackBonus)} para acertar`;
      const rider = a.rider?.save
        ? `; ${saveText(a.rider.save)}`
        : a.rider?.conditionKey
          ? `; ${a.rider.conditionPt}${a.rider.escapeDc ? ` (escapar CD ${a.rider.escapeDc})` : ''}`
          : '';
      return `${joinDots([bonus, reachText(a), partsText(a.damageParts)])}${rider}`;
    }
    case CreatureActionKind.SAVE:
      return a.save ? saveText(a.save) : '';
    default:
      return '';
  }
}

/** The word of the button that uses an action. */
export function actionButton(a: CreatureAction): string {
  if (a.kind === CreatureActionKind.ATTACK) {
    return 'Atacar';
  }
  if (a.kind === CreatureActionKind.SAVE && a.save?.area?.shape === 'cone') {
    return 'Soprar';
  }
  return 'Usar';
}

/** The limit of an action as the badge reads it, or `''` for an action with none. */
export function usageText(a: CreatureAction): string {
  const u = a.usage;
  switch (u?.kind) {
    case CreatureUsageKind.RECHARGE: {
      const range = u.rechargeMin >= D6_FACES ? String(D6_FACES) : `${u.rechargeMin}–${D6_FACES}`;
      return `Recarga ${range} · ${u.recharging ? 'Recarregando' : 'Disponível'}`;
    }
    case CreatureUsageKind.PER_DAY:
      return `${u.usesMax}/dia · ${u.usesLeft} de ${u.usesMax}`;
    case CreatureUsageKind.REST:
      return `Uma vez · ${u.usesLeft > 0 ? 'Disponível' : 'Usada'}`;
    default:
      return '';
  }
}

/** Why a button is grey, in a sentence; `''` when the action can be used. */
export function unavailableWhy(a: CreatureAction): string {
  if (a.available) {
    return '';
  }
  const u = a.usage;
  if (u?.kind === CreatureUsageKind.RECHARGE) {
    return 'Recarregando: o servidor rola o d6 no início da vez dele.';
  }
  return 'Sem usos: volta quando o encontro termina.';
}

/** "Usar (2)": a legendary option's button names its cost. */
export function legendaryButton(cost: number): string {
  return `Usar (${cost})`;
}

/** "Detectar (1)": a legendary option on the sheet. */
export function legendaryName(
  a: Pick<CreatureAction, 'name' | 'namePt' | 'legendaryCost'>,
): string {
  return `${a.namePt || a.name} (${a.legendaryCost})`;
}

/** "Resta só 1: não cabe", for an option that costs more than what is left. */
export function doesNotFit(left: number): string {
  return `Resta só ${left}: não cabe`;
}

/** The d6 line of a recharge: "Não recarregou." / "Recarregou." with the number. */
export function rechargeText(roll: number, min: number): { word: string; detail: string } {
  return {
    word: roll >= min ? 'Recarregou.' : 'Não recarregou.',
    detail: `No começo da vez o servidor rolou 1d6: ${roll}. A recarga é ${min}–${D6_FACES}.`,
  };
}

/** What a step did to the damage of a part. */
export function stepWord(kind: CreatureActionStepKind): string {
  switch (kind) {
    case CreatureActionStepKind.RESISTANCE:
      return 'Resistência: metade, arredondada para baixo';
    case CreatureActionStepKind.VULNERABILITY:
      return 'Vulnerável: dobra';
    case CreatureActionStepKind.IMMUNITY:
      return 'Imune: nenhum dano';
    default:
      return '';
  }
}

/** The sum the part's steps leave, as the line says it: "19 → 9". */
export function stepArrow(before: number, after: number): string {
  return `${before} → ${after}`;
}

/** The rows of a damage part: the roll and each step, ending with what lands. */
export function partRows(p: CreatureDamageRoll): { label: string; value: string }[] {
  const rows: { label: string; value: string }[] = [];
  rows.push({ label: p.damageTypePt, value: String(p.rolled?.total ?? 0) });
  for (const s of p.steps) {
    rows.push({ label: stepWord(s.kind), value: stepArrow(s.before, s.after) });
  }
  return rows;
}
