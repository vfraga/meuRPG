import { Component, TemplateRef, computed, input, output } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import type { TableEntry } from '../../../gen/meurpg/rules/v1/table_content_pb';
import type { EffectDraft, EffectMenuVm, FeatChoice } from '../../core/content/effect-draft';
import { emptyEffect } from '../../core/content/effect-draft';
import { emptyFeature, type FeatureDraft, NAME_MAX } from '../../core/content/feature-draft';
import { followResource, resourcesOf, type SummonDraft } from '../../core/content/summon-draft';
import { latestInput } from '../latest-input';
import { EffectPicker } from '../effect-picker/effect-picker';
import { FieldNote } from '../form-fields/field-note';
import { TextField } from '../form-fields/text-field';
import type { SelectOption } from '../form-fields/select-field';
import { SummonBlock } from '../summon-block/summon-block';

/**
 * One feature of a race, a subrace, a background (and, in 10.12, a class or a subclass): a name, a text and effects from the
 * server's menu (`app-effect-picker`). With no effect it is text only. The first effect is chosen in the "Efeito" select
 * ("Só texto" takes it away); a feature may have as many as the menu says (`maxEffects`). `basePath` is the feature's own path
 * ("table_race.traits[1]"): its inputs carry it, so a refusal lands on them.
 */
@Component({
  selector: 'app-feature-editor',
  imports: [
    EffectPicker,
    FieldNote,
    FeatureEditor,
    MatButtonModule,
    MatIconModule,
    SummonBlock,
    TextField,
  ],
  templateUrl: './feature-editor.html',
  styleUrl: './feature-editor.scss',
})
export class FeatureEditor {
  readonly feature = input.required<FeatureDraft>();
  readonly menu = input.required<EffectMenuVm>();
  readonly basePath = input.required<string>();
  readonly issuesOf = input<(path: string) => readonly string[]>(() => []);
  readonly spellOptions = input<readonly SelectOption[]>([]);
  readonly nameLabel = input('Nome do traço');
  readonly textLabel = input('Texto');
  /** "Remover traço": absent for a feature that must exist (a background's). */
  readonly removeLabel = input('');
  /** The place in a list, for "Subir" and "Descer": -1 when it is not in a list. */
  readonly index = input(-1);
  readonly count = input(0);
  /** The heading above the fields, when the feature has one of its own ("Característica"). */
  readonly heading = input('');
  /** No card of its own (no border, fill or padding): the page already frames it, as the open feature of a class does. */
  readonly flat = input(false);
  /** The feat editor: its effects may be the types only a feat has. */
  readonly featEditor = input(false);
  /**
   * The named choices of the feat these effects belong to (`featChoices` of its effects), set by the feat's own editor: a field
   * offers the ones before its effect. With none (null), no field offers a choice.
   */
  readonly choices = input<readonly FeatChoice[] | null>(null);
  /** The feature is an option of a feat: its fields offer fixed values only, and say so. */
  readonly featOption = input(false);
  /** The feat editor holds where a choice of options comes from in its own cards: the picker leaves those fields out. */
  readonly sourceInCards = input(false);
  /** A block drawn inside the picker of the effect at `afterFieldsAt`, under its fields and above "Mais opções". */
  readonly afterFields = input<TemplateRef<unknown> | null>(null);
  readonly afterFieldsAt = input(-1);
  /** A narrow field projected between the name and the effect (`<… lead>`): the class editor's "Nível". */
  readonly withLead = input(false);
  /**
   * "Opções para escolher": a class or subclass feature may carry a list of options the player picks (a table's maneuvers),
   * each edited here as a feature of its own (name, text, effects). A feature that has options needs a "Uma opção de uma
   * lista" choice, whose empty list offers them.
   */
  readonly allowOptions = input(false);
  /**
   * "Invoca criaturas": a class, subclass, race or background feature (and a class-feature entry) may bring creatures to its
   * holder; an option and a feat do not. Needs the campaign (the bestiary's search) and, to name the table's creatures, its entries.
   */
  readonly allowSummon = input(false);
  readonly campaignId = input('');
  readonly entries = input<readonly TableEntry[]>([]);

  /** The options are a race or subrace trait's one-time choice of one, picked at creation; no "Escolha" effect offers them. */
  readonly oneOf = input(false);
  /** The feature is one of a list of options: an option with no name is not sent. */
  readonly isOption = input(false);
  /** An option that has content but no name: the save leaves it out, and the screen says so. */
  protected readonly nameMax = NAME_MAX;
  protected readonly unsaved = computed(() => {
    const f = this.feature();
    return (
      this.isOption() && f.name.trim() === '' && (f.text.trim() !== '' || f.effects.length > 0)
    );
  });

  readonly featureChange = output<FeatureDraft>();
  private readonly latest = latestInput(this.feature);
  readonly removed = output<void>();
  readonly moved = output<-1 | 1>();

  protected readonly canAddEffect = computed(
    () =>
      this.feature().effects.length > 0 && this.feature().effects.length < this.menu().maxEffects,
  );
  /** The effects after the first, with their place in the list (the first sits beside the name). */
  protected readonly otherEffects = computed(() =>
    this.feature()
      .effects.map((effect, index) => ({ id: effect.id, effect, index }))
      .slice(1),
  );
  /** With no effect, the picker shows "Só texto". */
  protected readonly firstEffect = computed<EffectDraft>(
    () => this.feature().effects[0] ?? emptyEffect(''),
  );

  /** The group's own name, never the name field's label (two things with one name confuse a screen reader and a test). */
  protected readonly groupLabel = computed(
    () => this.heading() || (this.index() >= 0 ? `Traço ${this.index() + 1}` : 'Característica'),
  );

  protected path(field: string): string {
    return `${this.basePath()}.${field}`;
  }

  /** The resources the feature defines: what its summon can spend. */
  protected readonly resources = computed(() => resourcesOf(this.feature()));

  protected patch(p: Partial<FeatureDraft>): void {
    const was = this.latest.current();
    const next = followResource(was, { ...was, ...p });
    this.latest.sent(next);
    this.featureChange.emit(next);
  }

  protected setSummon(summon: SummonDraft | null): void {
    this.patch({ summon });
  }

  protected setEffect(i: number, e: EffectDraft): void {
    const effects = [...this.latest.current().effects];
    if (e.type === '') {
      // "Só texto" takes this effect away; the others stay.
      this.patch({ effects: effects.filter((_, k) => k !== i) });
      return;
    }
    effects[i] = e;
    this.patch({ effects });
  }

  protected addEffect(): void {
    this.patch({ effects: [...this.latest.current().effects, emptyEffect('note')] });
  }

  protected setOption(i: number, o: FeatureDraft): void {
    const options = [...this.latest.current().options];
    options[i] = o;
    this.patch({ options });
  }

  protected addOption(): void {
    this.patch({ options: [...this.latest.current().options, emptyFeature()] });
  }

  protected removeOption(i: number): void {
    this.patch({ options: this.latest.current().options.filter((_, k) => k !== i) });
  }

  protected removeEffect(i: number): void {
    this.patch({ effects: this.latest.current().effects.filter((_, k) => k !== i) });
  }
}
