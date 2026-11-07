import { ChangeDetectionStrategy, Component, computed, input, output, signal } from '@angular/core';

import type { EffectDraft, EffectMenuVm } from '../../../../core/content/effect-draft';
import {
  FEAT_OPTION_SETS,
  type OptionSource as Source,
  optionSource,
  withOptionSource,
} from '../../../../core/content/feat-options';
import { RadioCards, type RadioCard } from '../../../../shared/form-fields/radio-cards';
import { SelectField, type SelectOption } from '../../../../shared/form-fields/select-field';

/**
 * "De onde vêm as opções": the three places the options of a feat's choice come from, as radio cards, and the set the second one offers.
 * It edits the feat's one choice of kind "Uma opção de uma lista" (`set` and `from`); the choice's own fields stay in the effect.
 * The refusal of the pair lands on the cards, at the choice's `set` path.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-option-source',
  imports: [RadioCards, SelectField],
  templateUrl: './option-source.html',
  styleUrl: './option-source.scss',
})
export class OptionSource {
  /** The feat's choice of options. */
  readonly effect = input.required<EffectDraft>();
  readonly menu = input.required<EffectMenuVm>();
  /** The path of the choice, "table_feat.effects[0]". */
  readonly basePath = input.required<string>();
  /** How many options the feat's own panel holds. */
  readonly ownCount = input(0);
  readonly issuesOf = input<(path: string) => readonly string[]>(() => []);

  readonly effectChange = output<EffectDraft>();

  /** "Opções do SRD que eu marco" with nothing marked yet is the same draft as "As opções do talento": the card stays where it was put. */
  private readonly srdPicked = signal(false);

  protected readonly source = computed<Source>(() => {
    const own = optionSource(this.effect());
    return own === 'own' && this.srdPicked() ? 'srd' : own;
  });
  protected readonly sets = computed<SelectOption[]>(() =>
    this.menu()
      .list(FEAT_OPTION_SETS)
      .map((s) => ({ value: s.key, label: s.namePt })),
  );
  protected readonly setName = computed(() =>
    this.menu().nameOf(FEAT_OPTION_SETS, this.effect().set),
  );
  protected readonly setHint = computed(
    () => this.menu().optionOf(FEAT_OPTION_SETS, this.effect().set)?.hintPt ?? '',
  );
  protected readonly cards = computed<RadioCard[]>(() => {
    const n = this.ownCount();
    return [
      {
        value: 'own',
        label: 'As opções do talento',
        hint:
          n === 0 ? 'Escritas no painel abaixo.' : `${n === 1 ? 'A' : 'As'} ${n} do painel abaixo.`,
      },
      {
        value: 'set',
        label: 'Um conjunto inteiro',
        hint: this.effect().set === '' ? 'Todas de um conjunto do SRD.' : this.setName(),
      },
      { value: 'srd', label: 'Opções do SRD que eu marco', hint: 'Uma lista escolhida a dedo.' },
    ];
  });
  protected readonly path = computed(() => `${this.basePath()}.set`);
  protected readonly issues = computed(() => this.issuesOf()(this.path()));

  protected pick(value: string): void {
    const source = value as Source;
    this.srdPicked.set(source === 'srd');
    this.effectChange.emit(withOptionSource(this.effect(), source, this.sets()[0]?.value ?? ''));
  }

  protected setSet(set: string): void {
    this.effectChange.emit({ ...this.effect(), set, from: [] });
  }
}
