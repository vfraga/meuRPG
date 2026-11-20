import type { EffectMenuField } from '../../../gen/meurpg/rules/v1/table_content_pb';
import {
  featureLine,
  featureOffers,
  filterLine,
  toolSourceOf,
  withFilterKinds,
  withFromFeature,
  withToolSource,
} from '../../core/content/effect-choice-source';
import type { EffectDraft, EffectMenuVm } from '../../core/content/effect-draft';
import { TOOL_SOURCE_COPY } from '../../core/content/effect-field-copy';
import type { CheckOption } from '../form-fields/check-group';
import type { SelectOption } from '../form-fields/select-field';

/** What the view reads of the picker: its inputs as functions, the fields it draws now, and where an effect goes. */
export interface ChoiceSourceHost {
  readonly effect: () => EffectDraft;
  readonly menu: () => EffectMenuVm;
  readonly offerFeatures: () => boolean;
  readonly featOption: () => boolean;
  readonly fields: () => readonly EffectMenuField[];
  readonly emit: (effect: EffectDraft) => void;
}

/**
 * Where the options of a choice come from, as the picker draws it (RN-33): for a choice of tools the cards "De onde vêm as opções" and
 * the kinds of the filter, for a choice of features the select of another feature. Only one source shows at a time, and changing it
 * clears the others (`effect-choice-source.ts`).
 */
export class ChoiceSourceView {
  readonly legend = TOOL_SOURCE_COPY.legend;
  readonly cards = TOOL_SOURCE_COPY.cards;

  constructor(private readonly host: ChoiceSourceHost) {}

  /** A field the source hides: the filter unless it is the source, the list and the set where another source stands in, another feature's
   * options unless the editor offers them (or one is stored). */
  hidden(f: EffectMenuField): boolean {
    const e = this.host.effect();
    const tool = e.type === 'choice' && e.choice === 'tool';
    const feature = e.type === 'choice' && e.choice === 'feature';
    switch (f.name) {
      case 'filter':
        return !(tool && e.filterOn);
      case 'from_feature':
        return !(
          feature &&
          !this.host.featOption() &&
          (this.host.offerFeatures() || e.fromFeature !== '')
        );
      case 'from':
        return (tool && e.filterOn) || (feature && e.fromFeature !== '');
      case 'set':
        return feature && e.fromFeature !== '';
      default:
        return false;
    }
  }

  /** The cards come before the first field of a choice of tools that names its options (the list or the filter). */
  cardsAt(f: EffectMenuField): boolean {
    const e = this.host.effect();
    return (
      e.type === 'choice' &&
      e.choice === 'tool' &&
      (f.name === 'from' || f.name === 'filter') &&
      this.host.fields().find((g) => g.name === 'from' || g.name === 'filter') === f
    );
  }

  tool(): string {
    return toolSourceOf(this.host.effect());
  }

  setTool(source: string): void {
    this.host.emit(withToolSource(this.host.effect(), source === 'filter' ? 'filter' : 'list'));
  }

  /** The kinds of proficiency the filter may name, as the menu lists them. */
  kinds(): CheckOption[] {
    return this.host
      .menu()
      .list('proficiency_kinds')
      .map((k) => ({ value: k.key, label: k.namePt }));
  }

  filterNote(): string {
    return filterLine(this.host.menu(), this.host.effect().filter);
  }

  setKinds(kinds: string[]): void {
    this.host.emit(withFilterKinds(this.host.effect(), this.host.menu(), kinds));
  }

  /** "Nenhuma", then the features that have options; a stored key the menu no longer lists stays on the select, said in words. */
  features(): SelectOption[] {
    const offers = featureOffers(this.host.menu());
    const key = this.host.effect().fromFeature;
    const kept =
      key !== '' && !offers.some((o) => o.key === key)
        ? [{ value: key, label: 'Uma característica que saiu da lista' }]
        : [];
    return [
      { value: '', label: 'Nenhuma' },
      ...kept,
      ...offers.map((o) => ({ value: o.key, label: o.label })),
    ];
  }

  /** The line beside the select: how many options the feature offers today. */
  featureNote(key: string): string {
    return featureLine(featureOffers(this.host.menu()), key);
  }

  setFeature(key: string): void {
    this.host.emit(withFromFeature(this.host.effect(), key));
  }
}
