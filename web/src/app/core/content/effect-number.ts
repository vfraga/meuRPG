import { draftKey } from './effect-draft';
import type { EffectDraft } from './effect-draft.types';
import { readWhole, wholeText } from './whole-number';

/** The draft after a whole number was typed in the field `name`, with a sign where `signed` allows one ("+1" reads as 1). An empty field is
 * 0, "not set". What was typed stays as typed (`typed`, never sent); the draft holds the number, or a NaN for text that is no whole number
 * the server stores, so an editor does not save instead of sending 0. */
export function withNumber(
  d: EffectDraft,
  name: string,
  text: string,
  signed: boolean,
): EffectDraft {
  const n = readWhole(text, signed);
  const { [name]: _, ...kept } = d.typed ?? {};
  const typed = text === wholeText(n) || text.trim() === '' ? kept : { ...kept, [name]: text };
  return {
    ...d,
    [draftKey(name)]: n,
    typed: Object.keys(typed).length > 0 ? typed : undefined,
  };
}
