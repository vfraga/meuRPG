import {
  Component,
  ElementRef,
  Injector,
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

import type {
  GetClassTableDefaultsResponse,
  TableEntry,
} from '../../../../gen/meurpg/rules/v1/table_content_pb';
import type { CatalogVm } from '../../../core/content/catalog';
import {
  type CastingDraft,
  LEVEL_COUNT,
  type LevelFeature,
  type Preparation,
  type SubclassDraft,
  castingOfKind,
  defaultTableOf,
  emptyCasting,
  emptyRow,
  featureIdOfPath,
  gridColumns,
  isGridPath,
  rowsEdited,
  rowsOfTable,
} from '../../../core/content/class-draft';
import {
  draftToSubclass,
  emptySubclass,
  groupOfAlwaysPrepared,
  subclassFeatureBase,
  subclassFeaturePaths,
  subclassRowBase,
  subclassToDraft,
  unreadableSubclassPaths,
} from '../../../core/content/subclass-draft';
import { type EntryBody, TableContentClient } from '../../../core/content/content-client';
import type { EffectMenuVm } from '../../../core/content/effect-draft';
import { EntrySaver, focusAfterRender } from '../../../core/content/entry-saver';
import { FieldNote } from '../../../shared/form-fields/field-note';
import { PickList } from '../../../shared/form-fields/pick-list';
import { SelectField, type SelectOption } from '../../../shared/form-fields/select-field';
import { SwitchField } from '../../../shared/form-fields/switch-field';
import { TextField } from '../../../shared/form-fields/text-field';
import { type GridEdit, type GridRowVm, LevelGrid } from '../../../shared/level-grid/level-grid';
import { CastingFields } from '../class-parts/casting-fields';
import { TableQuestion } from '../class-parts/table-question';
import { ClassFeatures } from '../class-parts/class-features';
import { PlayersSwitch } from '../players-switch/players-switch';
import { EditorAlerts, EditorBar } from '../editor-bar/editor-bar';
import type { EditorSaved } from '../spell-editor/spell-editor';

/**
 * The subclass editor (MR-025, RN-23; E10-02 state 4): the name, the class it belongs to (chosen when it is created and never
 * changed), its features by level, the switch "Esta subclasse conjura" (a third caster: the casting, the list it casts from and
 * the table rows from the level casting starts, all from the server's defaults) and the spells it always has prepared, by class
 * level. A subclass of an SRD class and of a table class are the same form. Its switch "Disponível para os jogadores" is slice
 * 10.11c's (the seam is the comment in the template).
 */
@Component({
  selector: 'app-subclass-editor',
  imports: [
    CastingFields,
    ClassFeatures,
    TableQuestion,
    EditorAlerts,
    EditorBar,
    FieldNote,
    LevelGrid,
    MatButtonModule,
    MatIconModule,
    PickList,
    PlayersSwitch,
    SelectField,
    SwitchField,
    TextField,
  ],
  templateUrl: './subclass-editor.html',
  styleUrl: './subclass-editor.scss',
})
export class SubclassEditor {
  private readonly client = inject(TableContentClient);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly injector = inject(Injector);

  readonly campaignId = input.required<string>();
  readonly entry = input<TableEntry | null>(null);
  /** A new subclass: the class it is for (from the link on the class's page). */
  readonly parentKey = input('');
  readonly menu = input.required<EffectMenuVm>();
  readonly catalog = input.required<CatalogVm>();
  readonly defaults = input.required<GetClassTableDefaultsResponse>();
  readonly entries = input<readonly TableEntry[]>([]);
  readonly saveBlocked = input('');

  readonly saved = output<EditorSaved>();
  /** The entry's switch "Disponível para os jogadores" was turned (it saves at once, apart from the form). */
  readonly switched = output<TableEntry>();
  readonly reload = output<void>();
  readonly cancelled = output<void>();

  protected readonly draft = signal<SubclassDraft>(emptySubclass(''));
  protected readonly openFeature = signal('');
  protected readonly question = signal<CastingDraft | null>(null);
  protected readonly status = signal('');

  protected readonly saver = new EntrySaver(
    ((editor: SubclassEditor) => ({
      aOne: 'uma subclasse',
      nameOf: (key: string) => editor.entries().find((e) => e.key === key)?.namePt ?? '',
      get maxFeatures() {
        return editor.menu().maxFeatures;
      },
      get featureCount() {
        return editor.draft().features.length;
      },
      // "Too many features" is the panel head's; a refused spell of the flat list belongs to its group.
      redirect: (field: string, reason: string) => {
        if (reason === 'limit' && /^table_subclass\.levels\[\d+\]\.features\[\d+\]$/.test(field))
          return 'table_subclass.features';
        const group = groupOfAlwaysPrepared(editor.draft().alwaysPrepared, field);
        return group >= 0 ? `table_subclass.always_prepared#${group}` : field;
      },
    }))(this),
    'a subclasse',
  );

  protected readonly isNew = computed(() => this.entry() === null);
  protected readonly classOptions = computed<SelectOption[]>(() => [...this.catalog().classes]);
  protected readonly className = computed(() => this.catalog().nameOf(this.draft().classKey));
  /** The class level the choice happens at: the subclass's own, or the class's (the catalog says it). */
  protected readonly chosenAt = computed(
    () => this.draft().level || this.catalog().subclassLevelOf(this.draft().classKey),
  );
  protected readonly featureLevels = computed(() => {
    const from = Math.max(1, this.chosenAt());
    return Array.from({ length: LEVEL_COUNT - from + 1 }, (_, i) => from + i);
  });
  protected readonly spellOptions = computed<SelectOption[]>(() => [...this.catalog().spells]);
  /** The switch "As magias da subclasse entram na lista da classe" is for a subclass that names spells. */
  protected readonly hasSpells = computed(() =>
    this.draft().alwaysPrepared.some((g) => g.spells.length > 0),
  );
  protected readonly levelOptions = Array.from({ length: LEVEL_COUNT }, (_, i) => ({
    value: String(i + 1),
    label: `Nível ${i + 1}`,
  }));
  protected readonly columns = computed(() =>
    gridColumns(this.draft().casting, this.draft().rows, this.defaults(), false),
  );

  /** The rows of a third caster's table: the levels from the one casting starts at. */
  protected readonly gridRows = computed<GridRowVm[]>(() => {
    const d = this.draft();
    if (!d.conjures) return [];
    const start = d.casting.startLevel > 0 ? d.casting.startLevel : 1;
    const out: GridRowVm[] = [];
    for (let level = start; level <= LEVEL_COUNT; level++) {
      out.push({ level, base: subclassRowBase(d, level), row: d.rows[level - 1], chips: [] });
    }
    return out;
  });

  protected readonly tableNote = computed(() => {
    const prep =
      this.draft().casting.preparation === 'prepared'
        ? 'As magias são preparadas: o número vem da habilidade e do nível, não da tabela.'
        : 'As magias são conhecidas: a coluna “Magias” diz quantas.';
    return `Preenchida com o padrão de um terço, a partir do nível em que a conjuração começa. Edite o que quiser. ${prep}`;
  });

  protected readonly questionText = computed(
    () =>
      'Você editou a tabela da conjuração. Refazê-la com o padrão para este jeito de conjurar? O que você editou nela se perde.',
  );

  constructor() {
    const source = computed(() => {
      const e = this.entry();
      return e ? `${e.key}@${e.revision}` : `new:${this.parentKey()}`;
    });
    effect(() => {
      source();
      const defaults = this.defaults();
      untracked(() => this.start(defaults));
    });
  }

  private start(defaults: GetClassTableDefaultsResponse): void {
    const e = this.entry();
    this.draft.set(
      e?.body.case === 'tableSubclass'
        ? subclassToDraft(e.body.value, defaults)
        : emptySubclass(this.parentKey()),
    );
    this.openFeature.set('');
    this.question.set(null);
    this.saver.clear();
  }

  protected patch(p: Partial<SubclassDraft>): void {
    this.draft.update((d) => ({ ...d, ...p }));
  }

  protected readonly issuesOf = (path: string): readonly string[] => this.saver.issues(path);

  // ---- The third caster.

  protected setConjures(on: boolean): void {
    if (!on) {
      this.patch({
        conjures: false,
        casting: emptyCasting(''),
        rows: Array.from({ length: LEVEL_COUNT }, emptyRow),
      });
      return;
    }
    const casting = castingOfKind(
      'third',
      { ...emptyCasting('third'), preparation: 'known' },
      this.defaults(),
    );
    this.patch({
      conjures: true,
      casting,
      rows: rowsOfTable(defaultTableOf(casting, this.defaults()), this.defaults()),
    });
  }

  protected setPreparation(preparation: Preparation): void {
    const d = this.draft();
    if (preparation === d.casting.preparation) return;
    const next = castingOfKind('third', { ...d.casting, preparation }, this.defaults());
    if (rowsEdited(d.rows, d.casting, this.defaults())) {
      this.question.set(next);
      return;
    }
    this.patch({
      casting: next,
      rows: rowsOfTable(defaultTableOf(next, this.defaults()), this.defaults()),
    });
  }

  protected answerQuestion(replace: boolean): void {
    const next = this.question();
    this.question.set(null);
    if (!next) return;
    this.patch({
      casting: next,
      rows: replace
        ? rowsOfTable(defaultTableOf(next, this.defaults()), this.defaults())
        : this.draft().rows,
    });
    this.status.set(
      replace
        ? 'A tabela foi refeita com o padrão.'
        : 'A conjuração mudou; a tabela ficou como estava.',
    );
  }

  protected patchCasting(p: Partial<CastingDraft>): void {
    this.patch({ casting: { ...this.draft().casting, ...p } });
  }

  protected editCell(e: GridEdit): void {
    const rows = this.draft().rows.map((r, i) => {
      if (i !== e.level - 1) return r;
      switch (e.field) {
        case 'cantrips':
          return { ...r, cantrips: e.value };
        case 'spells':
          return { ...r, spells: e.value };
        case 'slot':
          return { ...r, slots: r.slots.map((n, k) => (k === e.slot ? e.value : n)) };
        default:
          return r;
      }
    });
    this.patch({ rows });
  }

  // ---- The features and the always-prepared spells.

  protected setFeatures(features: LevelFeature[]): void {
    this.patch({ features });
  }

  protected readonly featureBase = (i: number): string => subclassFeatureBase(this.draft(), i);

  protected addGroup(level: string): void {
    const n = Number(level);
    const groups = [...this.draft().alwaysPrepared, { level: n, spells: [] as string[] }].sort(
      (a, b) => a.level - b.level,
    );
    this.patch({ alwaysPrepared: groups });
  }

  protected removeGroup(level: number): void {
    this.patch({ alwaysPrepared: this.draft().alwaysPrepared.filter((g) => g.level !== level) });
  }

  protected addSpell(level: number, spell: string): void {
    this.patch({
      alwaysPrepared: this.draft().alwaysPrepared.map((g) =>
        g.level === level ? { ...g, spells: [...g.spells, spell] } : g,
      ),
    });
  }

  protected removeSpell(level: number, spell: string): void {
    this.patch({
      alwaysPrepared: this.draft().alwaysPrepared.map((g) =>
        g.level === level ? { ...g, spells: g.spells.filter((s) => s !== spell) } : g,
      ),
    });
  }

  protected freeLevels(): SelectOption[] {
    const used = new Set(this.draft().alwaysPrepared.map((g) => g.level));
    return this.levelOptions.filter((o) => !used.has(Number(o.value)));
  }

  // ---- Saving.

  private readonly known = (path: string): boolean => {
    const p = 'table_subclass';
    const d = this.draft();
    const fixed = [
      `${p}.name_pt`,
      `${p}.class_key`,
      `${p}.level`,
      `${p}.desc_pt`,
      `${p}.always_prepared`,
      `${p}.spells_expanded`,
      `${p}.casting`,
      `${p}.casting.ability`,
      `${p}.casting.list_from`,
      `${p}.casting.preparation`,
      `${p}.casting.start_level`,
      `${p}.casting.prepared_max`,
      `${p}.casting.kind`,
      `${p}.features`,
      ...d.alwaysPrepared.map((g) => `${p}.always_prepared#${g.level}`),
    ];
    if (fixed.includes(path)) return true;
    if (isGridPath(path, `${p}.levels`, this.columns(), false)) return true;
    return subclassFeaturePaths(d, this.menu()).includes(path);
  };

  protected async save(): Promise<void> {
    if (this.saver.saving() || this.saveBlocked()) {
      return;
    }
    if (
      this.saver.blockOn(unreadableSubclassPaths(this.draft(), this.menu()), (path) =>
        this.reveal(path),
      )
    ) {
      return;
    }
    const body: EntryBody = {
      case: 'tableSubclass',
      value: draftToSubclass(this.draft(), this.menu()),
    };
    const res = await this.saver.run(
      () => this.client.save(this.campaignId(), this.entry(), body, this.saver.keyFor(body)),
      this.known,
    );
    if (res) {
      this.saved.emit(res);
      return;
    }
    const first = this.saver.placement().fields[0];
    if (first) {
      this.reveal(first);
    }
  }

  /** Opens the feature that holds the path, so its input exists, and focuses the input. */
  private reveal(path: string): void {
    const feature = featureIdOfPath(
      this.draft().features,
      (k) => subclassFeatureBase(this.draft(), k),
      path,
    );
    if (feature) {
      this.openFeature.set(feature);
    }
    focusAfterRender(this.host.nativeElement, this.injector, path);
  }
}
