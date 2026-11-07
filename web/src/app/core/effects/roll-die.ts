import type { ManeuverDieOption } from '../../../gen/meurpg/play/v1/combat_pb';
import type { RollDie } from '../../../gen/meurpg/rules/v1/rules_pb';
import type { ExtraDieField } from './effects';

/** The roll a table die may join: an ability check (a skill or an ability), a saving throw of an ability, or the initiative. */
export type RollDieKind =
  | { readonly kind: 'check'; readonly skill?: string; readonly ability?: string }
  | { readonly kind: 'save'; readonly ability: string }
  | { readonly kind: 'initiative' };

/** The ability of each SRD skill, to tell a skill check apart from a die limited to the checks of an ability ("check.dex"). */
const SKILL_ABILITY: Readonly<Record<string, string>> = {
  athletics: 'str',
  acrobatics: 'dex',
  'sleight-of-hand': 'dex',
  stealth: 'dex',
  arcana: 'int',
  history: 'int',
  investigation: 'int',
  nature: 'int',
  religion: 'int',
  'animal-handling': 'wis',
  insight: 'wis',
  medicine: 'wis',
  perception: 'wis',
  survival: 'wis',
  deception: 'cha',
  intimidation: 'cha',
  performance: 'cha',
  persuasion: 'cha',
};

/** What a scene action rolls ("skill:stealth", "ability:str", "save:wis"), as the roll a die may join. */
export function sceneRollKind(actionKey: string): RollDieKind | null {
  const [kind, name = ''] = actionKey.split(':');
  switch (kind) {
    case 'skill':
      return { kind: 'check', skill: name, ability: SKILL_ABILITY[name] };
    case 'ability':
      return { kind: 'check', ability: name };
    case 'save':
      return { kind: 'save', ability: name };
    default:
      return null;
  }
}

function fits(die: RollDie, roll: RollDieKind): boolean {
  if (die.applies !== roll.kind) {
    return false;
  }
  if (roll.kind === 'check') {
    // No list: every ability check. A list names the skills ("skill.stealth") and abilities ("check.dex") it is limited to.
    return (
      die.targets.length === 0 ||
      die.targets.some(
        (t) =>
          (roll.skill !== undefined && t === `skill.${roll.skill}`) ||
          (roll.ability !== undefined && t === `check.${roll.ability}`),
      )
    );
  }
  return roll.kind === 'initiative' || die.ability === '' || die.ability === roll.ability;
}

/** The table dice the roll offers: only a die whose `applies` and limits fit it. */
export function rollDiceFor(
  dice: readonly RollDie[],
  roll: RollDieKind | null,
): readonly RollDie[] {
  return roll ? dice.filter((d) => fits(d, roll)) : [];
}

/** The field a physical roll asks for the die's face: always the last one of the roll's extra dice. */
export function rollDieField(die: RollDie): ExtraDieField {
  return {
    label: `Resultado do d${die.sides} (${die.namePt})`,
    name: die.namePt,
    faces: die.sides,
    sign: 1,
  };
}

/** "d8 · gasta 1 uso de dados de superioridade": what the tick says it costs. */
export function rollDieCost(die: RollDie): string {
  return die.resource === ''
    ? `d${die.sides}`
    : `d${die.sides} · gasta 1 uso de ${die.resource.replace(/_/g, ' ')}`;
}

/** The uses of the pool a table die spends from, as the turn options list them (`ManeuverDieOption`). */
export interface DieUses {
  readonly left: number;
  readonly resourceNamePt: string;
}

/** "d8 somado ao ataque · gasta 1 de Reserva de Golpes · restam 2": what the tick on a weapon attack says it costs. */
export function attackDieCost(die: RollDie, uses: DieUses): string {
  return `d${die.sides} somado ao ataque · gasta 1 de ${uses.resourceNamePt} · ${usesWord(uses.left)}`;
}

/** What a weapon-attack die listing needs: the die and its pool. */
export type ListedDie = Pick<ManeuverDieOption, 'die' | 'resourceNamePt' | 'usesLeft'>;

const cap = (t: string): string => t[0].toUpperCase() + t.slice(1);

function usesWord(left: number): string {
  if (left <= 0) {
    return 'sem usos';
  }
  return left === 1 ? 'resta 1' : `restam ${left}`;
}

/**
 * The one line that says a table die may join a weapon attack: "Golpe Certeiro (d8) pode entrar num ataque com arma:
 * você marca ao rolar. Reserva de Golpes: restam 2." (`where: 'list'`), or "... pode entrar neste ataque: ... Restam 2."
 * (`where: 'attack'`, the pool is named only when the dice come from more than one). The recharge is never named: the
 * options do not carry it. `''` without dice.
 */
export function maneuverLine(dice: readonly ListedDie[], where: 'list' | 'attack'): string {
  const listed = dice.filter((d) => d.die);
  if (!listed.length) {
    return '';
  }
  const names = listed.map((d) => `${d.die?.namePt} (d${d.die?.sides})`);
  const who =
    names.length === 1
      ? `${names[0]} pode`
      : `${names.slice(0, -1).join(', ')} e ${names[names.length - 1]} podem`;
  const pools = new Map<string, number>();
  for (const d of listed) {
    pools.set(d.resourceNamePt, d.usesLeft);
  }
  const lead = `${who} entrar ${where === 'list' ? 'num ataque com arma' : 'neste ataque'}: você marca ao rolar.`;
  if (where === 'attack' && pools.size === 1) {
    const left = [...pools.values()][0];
    return `${lead} ${cap(usesWord(left))}.`;
  }
  const tail = [...pools]
    .map(([pool, left]) => (pool ? `${pool}: ${usesWord(left)}.` : `${cap(usesWord(left))}.`))
    .join(' ');
  return `${lead} ${tail}`;
}
