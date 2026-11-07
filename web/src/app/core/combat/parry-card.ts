import { type ReactionWindow } from '../../../gen/meurpg/play/v1/combat_pb';
import { article } from '../format/article';
import { ofThe } from './move-plan';

/**
 * The master's Parry window in words (#557): a melee weapon hit on a creature whose stat block has Parry waits for the
 * master, who answers for the creature. The window opens the same whether or not +AC would change the hit, so the text
 * says which it is. The numbers are the master's only (`ReactionWindow.trigger`, `ParryPrompt`). Pure, no DOM.
 */
export interface ParryCard {
  // The trigger has no natural-roll flag: the text never claims a natural 20 or 1 changes the result.
  readonly id: string;
  readonly overline: string;
  /** The creature that parries. */
  readonly reactor: string;
  /** "Aparar o golpe de Tor?". */
  readonly title: string;
  /** "Tor acertou com a Espada longa: 16 contra CA 15. Aparar soma 2: contra CA 17, vira erro.". */
  readonly text: string;
  /** Parry would turn the hit into a miss. */
  readonly stops: boolean | null;
  readonly passLabel: string;
  /** "Aparar (+2 CA)". */
  readonly useLabel: string;
}

/** The card of a window of kind PARRY, or `null` for a window without its prompt. `attackerIsPlayer`: the attacker is a
 * player's character, whose name takes no article. */
export function parryCard(w: ReactionWindow, attackerIsPlayer = false): ParryCard | null {
  if (w.prompt.case !== 'parry') {
    return null;
  }
  const p = w.prompt.value;
  const t = w.trigger;
  const attacker = p.attackerLabel || t?.actorLabel || 'O atacante';
  const hit = t?.attackTotal;
  const ac = t?.targetArmorClass;
  const stops = hit !== undefined ? hit < p.armorClassWithParry : null;
  const weapon = p.attackNamePt ? ` com ${article(p.attackNamePt)} ${p.attackNamePt}` : '';
  const numbers = hit !== undefined && ac !== undefined ? `: ${hit} contra CA ${ac}` : '';
  const verdict = stops === null ? '' : `, ${stops ? 'vira erro' : 'ainda acerta'}`;
  return {
    id: w.id,
    overline: 'Reação',
    reactor: w.reactorLabel,
    title: `Aparar o golpe ${attackerIsPlayer ? `de ${attacker}` : ofThe([attacker])}?`,
    text: `${attacker} acertou${weapon}${numbers}. Aparar soma ${p.acBonus}: contra CA ${p.armorClassWithParry}${verdict}.`,
    stops,
    passLabel: 'Deixar passar',
    useLabel: `Aparar (+${p.acBonus} CA)`,
  };
}
