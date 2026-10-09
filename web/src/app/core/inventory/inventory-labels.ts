import type { Coins } from '../../../gen/meurpg/characters/v1/characters_pb';
import {
  ItemKind,
  type InventoryEntry,
} from '../../../gen/meurpg/characters/v1/inventory_service_pb';

/** The five coins, in the order of the paper sheet, with the abbreviation the table uses. */
export const COIN_FIELDS = [
  { key: 'copper', label: 'Cobre (PC)', short: 'PC' },
  { key: 'silver', label: 'Prata (PP)', short: 'PP' },
  { key: 'electrum', label: 'Electro (PE)', short: 'PE' },
  { key: 'gold', label: 'Ouro (PO)', short: 'PO' },
  { key: 'platinum', label: 'Platina (PL)', short: 'PL' },
] as const;

export type CoinKey = (typeof COIN_FIELDS)[number]['key'];

/** The largest amount of one coin the server accepts. */
export const COIN_MAX = 1_000_000;

const RARITY: Record<string, string> = {
  common: 'comum',
  uncommon: 'incomum',
  rare: 'raro',
  'very rare': 'muito raro',
  legendary: 'lendário',
  artifact: 'artefato',
  varies: 'varia',
};

/** "very rare" → "muito raro"; an unknown word is shown as it came. */
export function rarityLabel(rarity: string): string {
  return RARITY[rarity.toLowerCase()] ?? rarity;
}

/** "Sintonia: 2 de 3". */
export function attunementLine(count: number, max: number): string {
  return `Sintonia: ${count} de ${max}`;
}

/** "2d4 + 2" from the server's "2d4+2". */
export function diceText(dice: string): string {
  return dice.replace(/([+-])/g, ' $1 ').replace(/\s+/g, ' ').trim();
}

/** The second line of an item in the list: category, rarity and what it does, with the numbers the server worked out. */
export function itemDetail(entry: InventoryEntry, attackLine = ''): string {
  if (entry.unidentified) {
    return 'Você ainda não sabe o que ela faz.';
  }
  const parts: string[] = [];
  if (entry.kind === ItemKind.FREE_TEXT) {
    return 'Texto livre';
  }
  if (entry.category) {
    parts.push(entry.category);
  }
  if (entry.rarity) {
    parts.push(rarityLabel(entry.rarity));
  }
  if (entry.notePt) {
    parts.push(entry.notePt);
  }
  if (entry.healDice) {
    parts.push(diceText(entry.healDice));
  }
  if (entry.chargesMax > 0) {
    parts.push(`${entry.chargesMax - entry.chargesUsed} de ${entry.chargesMax} cargas`);
  }
  if (attackLine) {
    parts.push(attackLine);
  }
  return parts.join(' · ');
}

/** "Poção de cura" with its quantity beside it only when there is more than one. */
export function quantityText(entry: InventoryEntry): string {
  return entry.kind === ItemKind.FREE_TEXT ? '—' : String(entry.quantity);
}

/** The coins as a list of the ones the character has, or all zeros as the board draws them. */
export function coinRows(coins: Coins | undefined): { label: string; short: string; amount: number }[] {
  return COIN_FIELDS.map((f) => ({ label: f.label, short: f.short, amount: coins?.[f.key] ?? 0 }));
}
