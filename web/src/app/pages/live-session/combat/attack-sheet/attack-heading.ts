import type { AttackSheetData } from './attack-sheet-types';

/** What the head of the attack sheet is made from. */
export interface Heading {
  /** The sheet was opened at the sum the master's take-out asks again (#835): it says so instead of "Atacar com". */
  readonly again: boolean;
  readonly typing: boolean;
  readonly name: string;
  readonly detail: string;
  readonly target: string;
  readonly data: AttackSheetData;
}

export function headingTitle(h: Heading): string {
  if (h.again) {
    return 'Digitar o dano de novo';
  }
  if (h.typing) {
    return 'Digite o resultado do dado';
  }
  if (h.data.opportunity) {
    return `Ataque de oportunidade\u00a0· ${h.name}`;
  }
  return h.data.legendary?.title ?? `Atacar com ${h.name}`;
}

/** An opportunity attack names who attacks and what it costs; the numbers of the weapon are on the list. */
export function headingSubtitle(h: Heading): string {
  const who = h.data.state.encounter()?.combatants.find((c) => c.id === h.data.attackerId)?.label;
  if (h.again) {
    return `${who ?? ''} · ${h.name} no ${h.target}`;
  }
  if (h.typing) {
    return `${h.name} contra ${h.target} · Rodada ${h.data.round}`;
  }
  if (h.data.opportunity) {
    return who ? `${who} · gasta a sua reação` : 'Gasta a sua reação';
  }
  return h.data.legendary?.subtitle ?? h.detail;
}
