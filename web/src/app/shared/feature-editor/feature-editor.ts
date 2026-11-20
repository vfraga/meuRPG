import {
  Component,
  ElementRef,
  Injector,
  TemplateRef,
  afterNextRender,
  computed,
  effect,
  inject,
  input,
  output,
  signal,
  untracked,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { MatMenuModule } from '@angular/material/menu';

import type { TableEntry } from '../../../gen/meurpg/rules/v1/table_content_pb';
import type { EffectDraft, EffectMenuVm, FeatChoice } from '../../core/content/effect-draft';
import { emptyEffect } from '../../core/content/effect-draft';
import {
  emptyFeature,
  type FeatureDraft,
  featureAsks,
  optionRowPaths,
  NAME_MAX,
  withAsks,
} from '../../core/content/feature-draft';
import { askKeyNamer } from '../../core/content/ask-names';
import { focusAfterRender } from '../../core/content/entry-saver';
import { optionSummary } from '../../core/content/feat-options';
import { optionAsksPhrase } from '../../core/content/option-text';
import { followResource, resourcesOf, type SummonDraft } from '../../core/content/summon-draft';
import type { Asks } from '../../core/content/asks';
import {
  pactChoices,
  requirableOptions,
  siblingOptions,
} from '../../core/content/requirable-options';
import { AskRows } from '../ask-rows/ask-rows';
import { latestInput } from '../latest-input';
import { EffectPicker } from '../effect-picker/effect-picker';
import { FieldNote } from '../form-fields/field-note';
import { TextField } from '../form-fields/text-field';
import type { SelectOption } from '../form-fields/select-field';
import type { CatalogVm } from '../../core/content/catalog';
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
    AskRows,
    EffectPicker,
    FieldNote,
    FeatureEditor,
    MatButtonModule,
    MatIconModule,
    MatMenuModule,
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
  /** The catalog an aura effect names skills, damage types and abilities from. */
  readonly catalog = input<CatalogVm | null>(null);
  readonly nameLabel = input('Nome do traço');
  readonly textLabel = input('Texto');
  /** "Remover traço": absent for a feature that must exist (a background's). */
  readonly removeLabel = input('');
  /** The place in a list, for "Subir" and "Descer": -1 when it is not in a list. */
  readonly index = input(-1);
  readonly count = input(0);
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
  /** An option of a class or subclass feature may ask something of who picks it (a level, a pact, a spell, other options; board 580-03). */
  readonly allowAsks = input(false);
  protected readonly asks = computed(() => featureAsks(this.feature()));
  /** The editor is the body of an option row of its parent: the row is the group and has the head, the arrows and the menu. */
  readonly inRow = input(false);
  /** The options of the feature this one belongs to, itself among them: "Outras opções" offers the saved ones. */
  readonly peers = input<readonly FeatureDraft[]>([]);
  protected readonly classOptions = computed<SelectOption[]>(() => [
    { value: '', label: 'Nenhuma' },
    ...(this.catalog()?.classes ?? []),
  ]);
  protected readonly pactOptions = computed<SelectOption[]>(() =>
    pactChoices(this.menu(), this.entries()),
  );
  protected readonly requirable = computed<SelectOption[]>(() =>
    requirableOptions(
      this.menu(),
      this.entries(),
      this.feature().key,
      siblingOptions(this.peers(), this.feature().key),
    ),
  );
  /** How many of the other options of the list have no key yet (the hint says they can be asked for after the save). */
  protected readonly unsavedPeers = computed(
    () => this.peers().filter((o) => o.key === '' && o.id !== this.feature().id).length,
  );
  /**
   * The chips of "Outras opções" a refusal is about, while the row has one: a sibling that asks for this option in turn (the two
   * wait for each other), and a key no option has any more.
   */
  protected readonly flaggedOptions = computed<string[]>(() => {
    if (this.issuesOf()(this.path('prerequisite.requires_option_keys')).length === 0) {
      return [];
    }
    const own = this.feature().key;
    const known = new Set(this.requirable().map((o) => o.value));
    return this.asks().requiresOptions.filter((k) => {
      const peer = this.peers().find((p) => p.key === k);
      const waits = own !== '' && !!peer && featureAsks(peer).requiresOptions.includes(own);
      return waits || !known.has(k);
    });
  });
  protected readonly spellSearch = computed(() => this.catalog()?.spellSearch ?? []);

  protected setAsks(change: Partial<Asks>): void {
    const next = withAsks(this.latest.current(), change);
    this.latest.sent(next);
    this.featureChange.emit(next);
  }
  /** An option that has content but no name: the save leaves it out, and the screen says so. */
  protected readonly nameMax = NAME_MAX;
  protected readonly unsaved = computed(() => {
    const f = this.feature();
    return (
      this.isOption() && f.name.trim() === '' && (f.text.trim() !== '' || f.effects.length > 0)
    );
  });

  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly injector = inject(Injector);

  constructor() {
    effect(() => {
      if (this.refusedIds() !== '') {
        const first = untracked(() => this.rows().find((r) => r.bad)?.option.id);
        if (first) {
          untracked(() => this.openOption.set(first));
        }
      }
    });
    // A new answer of the save (or an option added or taken away) puts the refusals back at the places the save named: the rows
    // are in that order again until the next move.
    effect(() => {
      this.issuesOf()(this.basePath());
      this.optionCount();
      untracked(() => this.moves.set([]));
    });
  }

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
  protected readonly groupLabel = computed(() =>
    this.index() >= 0 ? `Traço ${this.index() + 1}` : 'Característica',
  );

  /** What the arrows name: the feature, else the field's label. */
  protected readonly moveName = computed(() => this.feature().name.trim() || this.nameLabel());

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

  private readonly optionCount = computed(() => this.feature().options.length);
  /**
   * The refusals of a save are told by place ("options[2]"), and the arrows move the option from its place at once. `moves[k]` is
   * the place the save named for the option now at row `k` (empty: no move since the save), so a refusal stays on the option it was
   * about while it moves; a new save, or an option added or taken away, clears it.
   */
  private readonly moves = signal<readonly number[]>([]);
  private placeOf(k: number): number {
    return this.moves()[k] ?? k;
  }
  /** The messages of a path of the option now at row `k`, read at the place the save named for it. */
  private readonly rowIssues = computed(() =>
    this.feature().options.map((_, k) => {
      const issues = this.issuesOf();
      const was = this.placeOf(k);
      if (was === k) {
        return issues;
      }
      const now = this.path(`options[${k}]`);
      const named = this.path(`options[${was}]`);
      const inRow = (p: string): boolean =>
        p === now || p.startsWith(`${now}.`) || p.startsWith(`${now}[`);
      return (path: string): readonly string[] =>
        issues(inRow(path) ? named + path.slice(now.length) : path);
    }),
  );

  /** The option whose row is open: one at a time (board 580-03). */
  protected readonly openOption = signal('');
  /** The rows of "Opções para escolher": the option, its title, the line under it and whether it is open or has a refusal. */
  protected readonly rows = computed(() => {
    const asking = !this.oneOf() && !this.featEditor() && !this.featOption();
    const nameOf = this.askName();
    return this.feature().options.map((option, index) => {
      const place = `Opção ${index + 1}`;
      const named = option.name.trim() !== '';
      const asks = optionAsksPhrase(option.prerequisite, nameOf);
      // The text of an option ends in a stop; the line is a label, so the stop goes before the " · " (board 580-03).
      const lead = optionSummary(option, this.menu()).replace(/\.$/, '');
      const tail = !asking ? '' : asks ? asks.replace(/^P/, 'p') : 'não pede nada';
      const summary =
        lead && tail ? `${lead} · ${tail}` : lead || tail.replace(/^./, (c) => c.toUpperCase());
      return {
        option,
        index,
        place,
        name: option.name.trim() || place,
        /** What the icon buttons of the row say they move: "a opção Golpe do Corvo", "a opção 3" for one with no name. */
        ref: named ? `a opção ${option.name.trim()}` : `a opção ${index + 1}`,
        issues: this.rowIssues()[index],
        title: option.name.trim() ? `${place} · ${option.name.trim()}` : place,
        summary,
        open: this.openOption() === option.id,
        bodyId: `opt-${option.id}`,
        bad: this.refused(index),
      };
    });
  });
  /** The names of the keys an option asks for: its siblings (as they are named now), the catalog, the options of the lists. */
  private readonly askName = computed(() =>
    askKeyNamer(this.feature().options, this.catalog(), this.requirableAll()),
  );
  private readonly requirableAll = computed(() =>
    this.allowOptions() ? requirableOptions(this.menu(), this.entries(), '') : [],
  );
  /** Whether a field of option `k`, or the option, has a refusal on it. */
  private refused(k: number): boolean {
    const issues = this.issuesOf();
    return optionRowPaths(
      this.path(`options[${this.placeOf(k)}]`),
      this.feature().options[k],
      this.menu(),
    ).some((p) => issues(p).length > 0);
  }
  /** The first row with a refusal opens when the set of such rows changes (a save that was refused): the field is there to be reached. */
  private readonly refusedIds = computed(() =>
    this.rows()
      .filter((r) => r.bad)
      .map((r) => r.option.id)
      .sort()
      .join(','),
  );

  protected toggleOption(id: string): void {
    this.openOption.update((open) => (open === id ? '' : id));
  }

  protected addOption(): void {
    const option = emptyFeature();
    const at = this.latest.current().options.length;
    this.openOption.set(option.id);
    this.patch({ options: [...this.latest.current().options, option] });
    focusAfterRender(this.host.nativeElement, this.injector, this.path(`options[${at}].name_pt`));
  }

  /**
   * "Subir" and "Descer" on an option row: the option goes with its key (a pick keeps its option). The row is drawn again at its
   * new place, so the focus goes back to the arrow that was pressed there (to the other one when that is now disabled).
   */
  protected moveOption(i: number, by: -1 | 1): void {
    const options = [...this.latest.current().options];
    const j = i + by;
    if (j < 0 || j >= options.length) {
      return;
    }
    [options[i], options[j]] = [options[j], options[i]];
    const places = options.map((_, k) => this.placeOf(k));
    [places[i], places[j]] = [places[j], places[i]];
    this.moves.set(places);
    this.patch({ options });
    afterNextRender(
      () => {
        const row = this.host.nativeElement.querySelector(`section.options .opt[data-opt="${j}"]`);
        const verbs = by === 1 ? ['down', 'up'] : ['up', 'down'];
        for (const verb of verbs) {
          const button = row?.querySelector<HTMLButtonElement>(`button[data-move="${verb}"]`);
          if (button && !button.disabled) {
            button.focus();
            return;
          }
        }
      },
      { injector: this.injector },
    );
  }

  /** Takes the option away; its row's place goes to the next row's button (the last one's to the one before, else to "Adicionar uma opção"). */
  protected removeOption(i: number): void {
    const gone = this.latest.current().options[i];
    this.patch({ options: this.latest.current().options.filter((_, k) => k !== i) });
    if (this.openOption() === gone?.id) {
      this.openOption.set('');
    }
    afterNextRender(
      () => {
        const toggles = this.host.nativeElement.querySelectorAll<HTMLButtonElement>(
          'section.options .opt__toggle',
        );
        const target =
          toggles[Math.min(i, toggles.length - 1)] ??
          this.host.nativeElement.querySelector<HTMLButtonElement>('section.options .options__add');
        target?.focus();
      },
      { injector: this.injector },
    );
  }

  protected removeEffect(i: number): void {
    this.patch({ effects: this.latest.current().effects.filter((_, k) => k !== i) });
  }
}
