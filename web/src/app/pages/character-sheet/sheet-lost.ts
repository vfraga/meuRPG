import type { FullSheet as GenFullSheet } from '../../../gen/meurpg/characters/v1/characters_pb';
import type {
  DerivedSheet as GenDerivedSheet,
  Feature,
} from '../../../gen/meurpg/rules/v1/rules_pb';
import type { FeatureVm, FullSheetVm, IssueVm } from './character-sheet.types';
import { lostKeys } from './sheet-format';

/** The sheet's features, each marked when its prerequisite is lost (board 580-05, "Não vale agora"). */
export function featuresVm(full: GenFullSheet, derived: GenDerivedSheet): FeatureVm[] {
  const lost = lostKeys(full, derived.issues);
  return optionsUnderTheirFeature(derived.features).map((f) => ({
    name: f.namePt,
    sourcePt: f.sourcePt,
    description: f.description,
    descriptionPt: f.descriptionPt,
    descriptionPtMissing: f.descriptionPtMissing,
    descriptionPtOnly: f.descriptionPtOnly,
    summaryPt: f.summaryPt,
    optionOf: f.optionOfPt,
    lost: lost.has(f.key),
  }));
}

/**
 * The sheet's own issues, for "Pendências de regra": what "A classe mudou" tells above (the issue
 * `table_content_changed` and any sentence a changed entry already lists) is not repeated in the list.
 */
export function ownIssues(full: FullSheetVm): readonly IssueVm[] {
  if (full.changedContent.length === 0) {
    return full.issues;
  }
  const told = new Set(full.changedContent.flatMap((c) => c.messages));
  return full.issues.filter((i) => i.code !== 'table_content_changed' && !told.has(i.message));
}

/**
 * An option's row stands right under the row of the feature that offers it, the engine's order kept otherwise
 * (board 580-05: the options are indented under their feature), found by its key: two features can share a name (the
 * Fighter's and the Paladin's "Estilo de Luta"). An option whose feature is not on the sheet stays where it is.
 */
function optionsUnderTheirFeature(features: readonly Feature[]): Feature[] {
  const under = new Map<string, Feature[]>();
  const rest: Feature[] = [];
  for (const f of features) {
    const parent =
      f.optionOfKey !== ''
        ? features.find((p) => p.optionOfKey === '' && p.key === f.optionOfKey)
        : undefined;
    if (parent) {
      under.set(parent.key, [...(under.get(parent.key) ?? []), f]);
    } else {
      rest.push(f);
    }
  }
  return rest.flatMap((f) => [f, ...(under.get(f.key) ?? [])]);
}
