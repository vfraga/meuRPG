import type { PickKind, PickRefusal } from '../../core/characters/character-errors';
import type { CharacterFormValue } from './character-editor.types';

/** The server's sentence for one pick of the draft, by its key: what the editor shows on the pick's own field. */
export interface PickSentence {
  readonly kind: PickKind;
  readonly key: string;
  readonly sentence: string;
}

/**
 * The refusal of a pick as the sentence of the key it points at: the server names the place in the list it was sent
 * (`skill_proficiency_keys[2]`), and the draft that was sent says which key stood there. `null` when the place holds no key
 * (the draft changed meanwhile).
 */
export function pickSentenceOf(
  refusal: PickRefusal,
  sent: Pick<CharacterFormValue, 'skillProficiencies' | 'expertiseSkillKeys' | 'featureChoiceKeys'>,
): PickSentence | null {
  const list =
    refusal.kind === 'skill'
      ? sent.skillProficiencies
      : refusal.kind === 'expertise'
        ? sent.expertiseSkillKeys
        : sent.featureChoiceKeys;
  const key = list[refusal.index];
  return key === undefined ? null : { kind: refusal.kind, key, sentence: refusal.sentence };
}

/** The sentences of one kind of pick, by key, for a picker. */
export function sentencesOf(
  pick: PickSentence | null,
  kind: PickKind,
  held: (key: string) => boolean,
): ReadonlyMap<string, string> {
  return pick && pick.kind === kind && held(pick.key)
    ? new Map([[pick.key, pick.sentence]])
    : new Map();
}
