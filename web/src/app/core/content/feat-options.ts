import type { EffectDraft, EffectMenuVm } from './effect-draft';
import type { FeatureDraft } from './feature-draft';

/**
 * What the feat editor's option panel and its "De onde vêm as opções" cards read of a draft.
 * The rules are the server's (`rules/overlay_features.go`, `rules/overlay_feat.go`).
 */

/** Where the options of a feat's choice come from: its own option list, a whole SRD set, or SRD options the master marks. */
export type OptionSource = 'own' | 'set' | 'srd';

/** The list the menu names the sets a feat may offer in. */
export const FEAT_OPTION_SETS = 'feat_option_sets';

/** The place of the feat's choice of options among its effects, or -1: a feat has one such choice. */
export function optionChoiceIndex(effects: readonly EffectDraft[]): number {
  return effects.findIndex((e) => e.type === 'choice' && e.choice === 'feature');
}

/** What the choice offers now, by what it holds. */
export function optionSource(e: Pick<EffectDraft, 'set' | 'from'>): OptionSource {
  if (e.set !== '') {
    return 'set';
  }
  return e.from.length > 0 ? 'srd' : 'own';
}

/** The choice after the master picked a source: the other two sources' fields go, so the server never gets both. */
export function withOptionSource(
  e: EffectDraft,
  source: OptionSource,
  firstSet: string,
): EffectDraft {
  switch (source) {
    case 'own':
      return { ...e, set: '', from: [] };
    case 'set':
      return { ...e, set: e.set || firstSet, from: [] };
    case 'srd':
      return { ...e, set: '' };
  }
}

const SUMMARY_MAX = 60;

function signed(mode: string, value: string): string {
  const v = value.trim();
  return mode === 'add' && /^\d/.test(v) ? `+${v}` : v;
}

/**
 * The line under an option's name: what its first effect does, or the start of its text.
 * The short form leaves out the effect's type ("Classe de Armadura +1" for "Modificador: Classe de Armadura +1"), for the row of a read.
 */
export function optionSummary(o: FeatureDraft, menu: EffectMenuVm, short = false): string {
  const e = o.effects[0];
  if (!e) {
    const text = o.text.trim().split('\n')[0];
    return text.length > SUMMARY_MAX ? `${text.slice(0, SUMMARY_MAX).trimEnd()}…` : text;
  }
  const type = menu.typeOf(e.type)?.namePt ?? '';
  switch (e.type) {
    case 'modifier':
      return e.target === ''
        ? type
        : `${short ? '' : `${type}: `}${menu.nameOf('modifier_targets', e.target)} ${signed(e.mode, e.value)}`.trim();
    case 'proficiency':
      return e.proficiency === ''
        ? type
        : `Proficiência em ${menu.nameOf('proficiency_targets', e.proficiency)}`;
    default:
      return type;
  }
}
