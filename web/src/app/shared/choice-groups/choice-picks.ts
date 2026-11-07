import {
  type Choice,
  ChoiceKind,
  type ChoiceGroup,
  ChoiceOrigin,
  type ChoiceOption,
  ChoicePrerequisiteKind,
} from '../../../gen/meurpg/characters/v1/characters_pb';

/**
 * The pure half of the class and race choices (PM-05): turning a pick on a choice into the
 * keys the sheet stores, and naming a group. The rules are the server's (`PreviewChoices`
 * says which choices there are, their options, what each asks for and what is picked):
 * nothing here knows a fighting style from an invocation.
 */

/** The key of the n-th free text (from 1) of a choice, in `FullSheet.feature_choice_text`. */
export function choiceTextKey(choiceKey: string, n: number): string {
  return `${choiceKey}#${n}`;
}

/**
 * The stored keys after a choice's selection became `nextOptionKeys`: the stored key of an option
 * that is no longer picked goes, the one of a newly picked option comes at the end, and every other
 * key is left alone (the other choices', and the picks of other choices that share the same options
 * stay). The order of the keys is the order the picks were made, which is what the server reads.
 */
export function applySelection(
  keys: readonly string[],
  choice: Choice,
  nextOptionKeys: readonly string[],
): string[] {
  const stored = new Map(choice.options.map((o) => [o.key, o.storedKey]));
  const before = new Set(choice.picked);
  const after = new Set(nextOptionKeys);
  const gone = new Set(choice.picked.filter((k) => !after.has(k)).map((k) => stored.get(k) ?? k));
  const next = keys.filter((k) => !gone.has(k));
  for (const k of nextOptionKeys) {
    const key = stored.get(k) ?? k;
    if (!before.has(k) && !next.includes(key)) {
      next.push(key);
    }
  }
  return next;
}

/** The texts of the sheet with the n-th (from 1) of a choice set; an empty text takes the entry out. */
export function withChoiceText(
  texts: Readonly<Record<string, string>>,
  choiceKey: string,
  n: number,
  value: string,
): Record<string, string> {
  const out = { ...texts };
  const key = choiceTextKey(choiceKey, n);
  if (value.trim() === '') {
    delete out[key];
  } else {
    out[key] = value;
  }
  return out;
}

/** The line under a choice's title that says where it comes from: "Raça · Draconato · 1 escolha". */
export function originLine(group: ChoiceGroup, choice: Choice): string {
  const count = choice.picks === 1 ? '1 escolha' : `${choice.picks} escolhas`;
  switch (group.origin) {
    case ChoiceOrigin.RACE:
      if (isTraitChoice(group, choice)) {
        return `Traço de ${group.sourceNamePt} · ${choice.picks === 1 ? 'escolha uma vez' : count}`;
      }
      return `Raça · ${group.sourceNamePt} · ${count}`;
    case ChoiceOrigin.BACKGROUND:
      return `Antecedente · ${group.sourceNamePt} · ${count}`;
    case ChoiceOrigin.FEAT:
      return `Talento · ${group.sourceNamePt} · ${count}`;
    case ChoiceOrigin.SUBCLASS:
      return `${group.classNamePt} · nível ${group.level} · ${group.sourceNamePt}`;
    default:
      return `${group.sourceNamePt} · nível ${group.level} · ${count}`;
  }
}

/** A race's or subrace's trait that offers options (a table's "one of", the dragonborn's ancestry): its card reads "Traço de Corujeiro". */
export function isTraitChoice(group: ChoiceGroup, choice: Choice): boolean {
  return (
    group.origin === ChoiceOrigin.RACE &&
    choice.kind === ChoiceKind.OPTIONS &&
    choice.featureKey.startsWith('trait:') &&
    // The choice of the trait's own options; a part of a choice is "<trait>#<part>".
    choice.key === choice.featureKey
  );
}

/** One card of the step: a choice, or the choices a feature asks together (a favored enemy's type and language). */
export interface ChoiceSection {
  readonly group: ChoiceGroup;
  readonly choices: readonly Choice[];
}

/** The cards of the groups, in order: consecutive choices of the same feature that ask in parts share one. */
export function sectionsOf(groups: readonly ChoiceGroup[]): ChoiceSection[] {
  const out: ChoiceSection[] = [];
  for (const group of groups) {
    let last: { group: ChoiceGroup; choices: Choice[] } | undefined;
    for (const choice of group.choices) {
      if (last && choice.partPt !== '' && last.choices[0].featureKey === choice.featureKey) {
        last.choices.push(choice);
        continue;
      }
      last = { group, choices: [choice] };
      out.push(last);
    }
  }
  return out;
}

/**
 * The labels of the choices with something left to pick, in the order of the groups. An advisory choice (the
 * languages and tools a race or background lets the player pick) warns while it is open but does not stop the sheet:
 * it counts only with `advisory`.
 */
export function pendingLabels(groups: readonly ChoiceGroup[], advisory = true): string[] {
  return groups.flatMap((g) =>
    g.choices.filter((c) => c.missing > 0 && (advisory || !c.advisory)).map((c) => c.labelPt),
  );
}

/** The sentence under "Criar personagem" when choices are open: "Falta uma escolha: ..." (only the ones that stop the sheet). */
export function missingSentence(groups: readonly ChoiceGroup[]): string {
  const labels = pendingLabels(groups, false);
  if (labels.length === 0) {
    return '';
  }
  return labels.length === 1
    ? `Falta uma escolha: ${labels[0]}.`
    : `Faltam escolhas: ${labels.join('; ')}.`;
}

/** Whether an option cannot be taken now (a prerequisite is unmet, or another choice took it). */
export function isBlocked(option: ChoiceOption): boolean {
  return option.reasonPt !== '';
}

/** The spell an option still asks the sheet to know (an invocation that needs Eldritch Blast), if any. */
export function missingSpell(option: ChoiceOption): { key: string; namePt: string } | undefined {
  const p = option.prerequisites.find((x) => x.kind === ChoicePrerequisiteKind.SPELL && !x.met);
  return p ? { key: p.key, namePt: p.namePt } : undefined;
}

/** Whether a choice is drawn as a pick-one list (radio) rather than as check boxes. */
export function isRadio(choice: Choice): boolean {
  return choice.picks === 1 && choice.kind === ChoiceKind.OPTIONS;
}
