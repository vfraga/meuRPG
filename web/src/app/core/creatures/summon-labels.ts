import type { ReplacedCreature } from '../../../gen/meurpg/characters/v1/characters_pb';
import { tight } from '../format/text';

/** Convocar Familiar's key: the one summon that makes a named companion the table talks to. */
export const FIND_FAMILIAR = 'spell:find-familiar';

/** The button's words for what a casting does, the same in every state ("Convocar o familiar"). Copy only:
 * what a spell may bring comes from `GetSummonOptions`. */
export function castVerb(spellKey: string, namePt: string): string {
  switch (spellKey) {
    case FIND_FAMILIAR:
      return 'Convocar o familiar';
    case 'spell:animate-dead':
      return 'Animar os mortos';
    default:
      // A spell whose name already starts with the verb ("Conjurar Animais") is the button as it stands.
      return namePt.startsWith('Conjurar ') ? namePt : `Conjurar ${namePt}`;
  }
}

/** "3 criaturas", "1 criatura". */
export function creaturesText(n: number): string {
  return tight(`${n} ${n === 1 ? 'criatura' : 'criaturas'}`);
}

/**
 * What a casting sends away, said before it happens: a familiar leaves ("Nanquim sai da ficha: um novo
 * familiar toma o lugar."), the creatures a concentration holds are dismissed ("Isso encerra Conjurar Animais e
 * dispensa 8 criaturas: Lobo 1, Lobo 2 e mais 6."). Empty when nothing goes.
 */
export function replacesText(
  spellNamePt: string,
  concentration: boolean,
  replaces: readonly ReplacedCreature[],
): string {
  if (replaces.length === 0) {
    return '';
  }
  if (!concentration) {
    const names = replaces.map((r) => r.name);
    return `${names.join(' e ')} sai da ficha: um novo familiar toma o lugar.`;
  }
  const shown = replaces.slice(0, 3).map((r) => r.name);
  const rest = replaces.length - shown.length;
  const list =
    rest > 0
      ? `${shown.join(', ')} e mais ${rest}`
      : shown.length > 1
        ? `${shown.slice(0, -1).join(', ')} e ${shown[shown.length - 1]}`
        : shown[0];
  return tight(
    `Isso encerra ${spellNamePt} e dispensa ${creaturesText(replaces.length)}: ${list}.`,
  );
}

/** The live notice after a cast: what arrived, what it cost, what left. */
export function castNotice(
  arrived: string,
  spellNamePt: string,
  ritual: boolean,
  time: string,
  dismissed: number,
): string {
  const cost = ritual
    ? `${spellNamePt}, ritual de ${time}. Nenhum espaço de magia foi gasto.`
    : `${spellNamePt}: o espaço de magia foi gasto.`;
  const left =
    dismissed > 0
      ? ` ${dismissed === 1 ? 'Uma criatura foi dispensada' : creaturesText(dismissed) + ' foram dispensadas'} no lugar.`
      : '';
  return tight(`${arrived} ${cost}${left}`);
}

/** Under a cast button of the "Criaturas" panel: what the cast spends, and that it needs a session and no combat. */
export function castCostText(spell: {
  readonly grantedBy: string;
  readonly usesLeft: number;
  readonly canRitual: boolean;
  readonly castingTimePt: string;
  readonly level: number;
}): string {
  const where = 'Só durante uma sessão, fora de combate.';
  if (spell.grantedBy) {
    return tight(`Gasta um uso, restam ${spell.usesLeft}. ${where}`);
  }
  if (spell.canRitual) {
    return tight(`Ritual de ${spell.castingTimePt}: não gasta espaço de magia. ${where}`);
  }
  return tight(`Gasta um espaço de ${spell.level}º nível ou maior. ${where}`);
}

/** The live notice of a cast made from the panel: who arrived (by name, or by count), then what it cost. */
export function castConfirmation(r: {
  readonly names: readonly string[];
  readonly count: number;
  readonly spellName: string;
  readonly ritual: boolean;
  readonly castingTime: string;
  readonly dismissed: number;
}): string {
  const arrived =
    r.names.length === 1
      ? `${r.names[0]} chegou.`
      : r.count === 1
        ? 'A criatura chegou.'
        : `${creaturesText(r.count)} chegaram.`;
  return castNotice(arrived, r.spellName, r.ritual, r.castingTime, r.dismissed);
}
