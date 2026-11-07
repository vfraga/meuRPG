import type {
  TableEffect,
  TableFeat,
  TableFeature,
} from '../../../gen/meurpg/rules/v1/table_content_pb';
import { type NameOf, type ReadItem, type ReadRow, featureItem } from './content-read-parts';
import { CHOICE_KINDS, choiceKindOf } from './effect-choices';
import { optionSetName } from './option-text';

/** How much a choice of options asks: "1 de 4 opções" for the feat's own or marked ones, "1 estilo de luta" for a whole set. */
function optionsAsked(e: TableEffect, own: number): string {
  if (e.set !== '') {
    return `${e.count} ${optionSetName(e.set).toLowerCase()}`;
  }
  const offered = e.from.length > 0 ? e.from.length : own;
  return offered > 0 ? `${e.count} de ${offered} ${offered === 1 ? 'opção' : 'opções'}` : '';
}

/** The "Escolha" row of each choice of the feat that the player makes when taking it. */
export function featChoiceRows(f: TableFeat): ReadRow[] {
  return f.effects.flatMap((e): ReadRow[] => {
    if (e.type !== 'choice') {
      return [];
    }
    const kind = choiceKindOf(e.choice);
    const value = kind
      ? `${e.count} ${CHOICE_KINDS[kind].words[e.count === 1 ? 0 : 1]}`
      : e.choice === 'feature'
        ? optionsAsked(e, f.options.length)
        : '';
    return value === '' ? [] : [{ label: 'Escolha', value }];
  });
}

/**
 * The options the feat lists of its own, each with its text and what its effects say.
 * With `sideOf` (the master, who has the effect menu) each also carries a few words of what it does, for a row.
 */
export function featOptionItems(
  f: TableFeat,
  nameOf: NameOf,
  sideOf?: (o: TableFeature) => string,
): ReadItem[] {
  return f.options
    .filter((o) => o.namePt !== '')
    .map((o) => {
      const item = featureItem(o, nameOf);
      const side = sideOf?.(o) ?? '';
      return side === '' ? item : { ...item, side };
    });
}
