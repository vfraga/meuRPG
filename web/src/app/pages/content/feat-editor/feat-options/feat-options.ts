import { ChangeDetectionStrategy, Component, computed, input, output, signal } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { MatMenuModule } from '@angular/material/menu';

import type { EffectMenuVm } from '../../../../core/content/effect-draft';
import { MAX_FEAT_OPTIONS } from '../../../../core/content/feat-draft';
import { optionSummary } from '../../../../core/content/feat-options';
import { type FeatureDraft, emptyFeature } from '../../../../core/content/feature-draft';
import { FeatureEditor } from '../../../../shared/feature-editor/feature-editor';
import { FieldNote } from '../../../../shared/form-fields/field-note';
import type { SelectOption } from '../../../../shared/form-fields/select-field';

/** One row of the panel: the option, its place, its name and what it does. */
interface Row {
  readonly option: FeatureDraft;
  readonly index: number;
  /** "Opção 2", for the buttons' names. */
  readonly place: string;
  /** "Opção 2 · Mão Firme". */
  readonly title: string;
  readonly summary: string;
  readonly open: boolean;
}

/**
 * "Opções do talento": the feat's own option list, one folded row per option with its fields inside.
 * An option is a feature of its own (name, text, effects); an option with no name is not sent.
 * The panel holds up to `MAX_FEAT_OPTIONS`; at the limit the add button waits and says why.
 * While the feat's choice offers a whole set the list stays as it is and takes no new option.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-feat-options',
  imports: [FeatureEditor, FieldNote, MatButtonModule, MatIconModule, MatMenuModule],
  templateUrl: './feat-options.html',
  styleUrl: './feat-options.scss',
})
export class FeatOptions {
  readonly options = input.required<readonly FeatureDraft[]>();
  readonly menu = input.required<EffectMenuVm>();
  readonly issuesOf = input<(path: string) => readonly string[]>(() => []);
  readonly spellOptions = input<readonly SelectOption[]>([]);
  /** The number of the feat's choice of options among its effects (from 1), or 0 when it has none. */
  readonly choiceNumber = input(0);
  /** The name of the set the choice offers, or empty when it offers the options of the feat. */
  readonly setName = input('');

  /** The paths the server refused: a row with a refusal inside stays open. */
  readonly refused = input<readonly string[]>([]);

  readonly optionsChange = output<FeatureDraft[]>();

  protected readonly basePath = 'table_feat.options';
  protected readonly max = MAX_FEAT_OPTIONS;
  protected readonly openIds = signal<ReadonlySet<string>>(new Set());
  protected readonly count = computed(() => this.options().length);
  protected readonly over = computed(() => this.count() > MAX_FEAT_OPTIONS);
  protected readonly full = computed(() => this.count() >= MAX_FEAT_OPTIONS);
  protected readonly offersSet = computed(() => this.setName() !== '');
  /** Why "Adicionar uma opção" waits, or empty. */
  protected readonly lock = computed(() => {
    if (this.offersSet()) {
      return 'A escolha oferece um conjunto: um talento oferece as opções dele ou um conjunto, não os dois.';
    }
    return this.full() ? `No máximo ${MAX_FEAT_OPTIONS} opções. Remova uma para pôr outra.` : '';
  });
  protected readonly rows = computed<Row[]>(() =>
    this.options().map((option, index) => ({
      option,
      index,
      place: `Opção ${index + 1}`,
      title:
        option.name.trim() === ''
          ? `Opção ${index + 1}`
          : `Opção ${index + 1} · ${option.name.trim()}`,
      summary: optionSummary(option, this.menu()),
      open: this.openIds().has(option.id) || this.refusedAt(index),
    })),
  );
  protected readonly listIssues = computed(() => this.issuesOf()(this.basePath));

  private refusedAt(index: number): boolean {
    const at = `${this.basePath}[${index}]`;
    return this.refused().some((p) => p === at || p.startsWith(`${at}.`));
  }

  protected toggle(id: string): void {
    const next = new Set(this.openIds());
    if (!next.delete(id)) {
      next.add(id);
    }
    this.openIds.set(next);
  }

  protected add(): void {
    if (this.lock() !== '') {
      return;
    }
    const option = emptyFeature();
    this.openIds.update((ids) => new Set(ids).add(option.id));
    this.optionsChange.emit([...this.options(), option]);
  }

  protected set(i: number, option: FeatureDraft): void {
    const next = [...this.options()];
    next[i] = option;
    this.optionsChange.emit(next);
  }

  protected move(i: number, by: -1 | 1): void {
    const next = [...this.options()];
    const to = i + by;
    if (to < 0 || to >= next.length) {
      return;
    }
    [next[i], next[to]] = [next[to], next[i]];
    this.optionsChange.emit(next);
  }

  protected remove(i: number): void {
    this.optionsChange.emit(this.options().filter((_, k) => k !== i));
  }
}
