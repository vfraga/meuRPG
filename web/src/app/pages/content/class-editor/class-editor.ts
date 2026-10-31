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
import { RouterLink } from '@angular/router';

import { Ability } from '../../../../gen/meurpg/rules/v1/rules_pb';
import { PlayersSwitch } from '../players-switch/players-switch';
import type {
  GetClassTableDefaultsResponse,
  TableEntry,
} from '../../../../gen/meurpg/rules/v1/table_content_pb';
import type { CatalogVm } from '../../../core/content/catalog';
import {
  type CastingDraft,
  type CastingKind,
  type ClassDraft,
  type LevelFeature,
  type Preparation,
  castingOfKind,
  classFeatureBase,
  classToDraft,
  defaultTableOf,
  draftToClass,
  emptyClass,
  featureIdOfPath,
  gridColumns,
  newLevelFeatureId,
  rowsEdited,
  rowsOfTable,
  sortedFeatures,
} from '../../../core/content/class-draft';
import { type EntryBody, TableContentClient } from '../../../core/content/content-client';
import type { EffectMenuVm } from '../../../core/content/effect-draft';
import { EntrySaver, focusAfterRender } from '../../../core/content/entry-saver';
import {
  ABILITY_FIELDS,
  type AbilityField,
  emptyFeature,
} from '../../../core/content/feature-draft';
import { CheckRow } from '../../../shared/form-fields/check-row';
import { FieldNote } from '../../../shared/form-fields/field-note';
import { NumberStepper } from '../../../shared/form-fields/number-stepper';
import { PickList } from '../../../shared/form-fields/pick-list';
import { SelectField, type SelectOption } from '../../../shared/form-fields/select-field';
import { TextField } from '../../../shared/form-fields/text-field';
import { type GridEdit, type GridRowVm, LevelGrid } from '../../../shared/level-grid/level-grid';
import { CastingFields } from '../class-parts/casting-fields';
import { ClassFeatures } from '../class-parts/class-features';
import { type NavSection, SectionNav } from '../class-parts/section-nav';
import { TableQuestion } from '../class-parts/table-question';
import { EditorAlerts, EditorBar } from '../editor-bar/editor-bar';
import type { EditorSaved } from '../spell-editor/spell-editor';
import {
  ARMOR_KEYS,
  ASI_OPTIONS,
  HIT_DICE,
  LEVEL_NUMBERS,
  LEVEL_OPTIONS,
  subclassesOf,
  GROUP_KEYS,
  REQUIREMENT_GROUPS,
  SECTIONS,
  WEAPON_KEYS,
  type RequirementField,
  type TableAsk,
  addedRequirement,
  answeredQuestion,
  classViolationContext,
  editedRows,
  emptyDefaults,
  gridRowsOf,
  isKnownPath,
  movedRequirement,
  questionTextOf,
  removedRequirement,
  sectionHasIssue,
  tableNoteOf,
  toggled,
  typedRequirement,
  unreadableClassPaths,
  withProficiency,
} from './class-editor-model';

/**
 * The class editor (MR-025, RN-23, ADR-0018; E10-02 states 1 to 3): one page with sections and a list of them at the side. The
 * basics, the proficiencies, how it casts, the 20-level table (the server's defaults, edited cell by cell), the features with
 * their effects from the server's menu, the level the subclass is chosen at and the subclasses the class has. One "Salvar
 * classe" sends the whole entry; a refusal comes back on the input it names, a grid cell included. The class's switch "Disponível
 * para os jogadores" is slice 10.11c's: it goes in the "Para os jogadores" panel at the foot of the side list (see the seam in
 * the template).
 */
@Component({
  selector: 'app-class-editor',
  imports: [
    CastingFields,
    CheckRow,
    ClassFeatures,
    EditorAlerts,
    EditorBar,
    FieldNote,
    LevelGrid,
    MatButtonModule,
    MatIconModule,
    NumberStepper,
    PickList,
    PlayersSwitch,
    RouterLink,
    SectionNav,
    SelectField,
    TableQuestion,
    TextField,
  ],
  templateUrl: './class-editor.html',
  styleUrl: './class-editor.scss',
})
export class ClassEditor {
  private readonly client = inject(TableContentClient);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly injector = inject(Injector);

  readonly campaignId = input.required<string>();
  readonly entry = input<TableEntry | null>(null);
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

  protected readonly draft = signal<ClassDraft>(emptyClass(emptyDefaults()));
  protected readonly openFeature = signal('');
  protected readonly question = signal<TableAsk | null>(null);
  protected readonly allCircles = signal(false);
  protected readonly status = signal('');

  protected readonly saver = new EntrySaver(
    classViolationContext({ entries: this.entries, menu: this.menu, draft: this.draft }),
    'a classe',
  );

  /** The list of sections, each with whether a refusal is inside it. */
  protected readonly sections = computed<NavSection[]>(() =>
    SECTIONS.map((s) => ({ ...s, issue: sectionHasIssue(s.id, this.saver.placement().fields) })),
  );

  protected readonly abilities = computed(() => this.catalog().abilities);
  protected readonly abilityOptions = computed<SelectOption<number>[]>(() =>
    this.abilities().map((a) => ({ value: a.ability as number, label: a.name })),
  );
  protected readonly hitDice = HIT_DICE;
  protected readonly levelOptions = LEVEL_OPTIONS;
  protected readonly levelNumbers = LEVEL_NUMBERS;
  protected readonly asiOptions = ASI_OPTIONS;
  protected readonly skills = computed(() => this.catalog().skills);
  protected readonly spellOptions = computed<SelectOption[]>(() => [...this.catalog().spells]);

  protected readonly armor = computed(() => this.groupOptions(ARMOR_KEYS));
  protected readonly weapons = computed(() => this.groupOptions(WEAPON_KEYS));
  /** The proficiencies that are not one of the six groups: single weapons, armors and tools, from the menu. */
  protected readonly otherProficiencies = computed<SelectOption[]>(() =>
    this.menu()
      .list('proficiency_targets')
      .filter((p) => p.key.startsWith('proficiency:') && !GROUP_KEYS.includes(p.key))
      .map((p) => ({ value: p.key, label: p.namePt })),
  );

  protected readonly casting = computed(() => this.draft().casting);
  protected readonly columns = computed(() =>
    gridColumns(this.casting(), this.draft().rows, this.defaults(), this.allCircles()),
  );
  protected readonly canAllCircles = computed(
    () => this.casting().kind !== '' && this.columns().circles < 9,
  );
  protected readonly isNew = computed(() => this.entry() === null);
  protected readonly selfKey = computed(() => this.entry()?.key ?? '');
  protected readonly subclasses = computed(() => subclassesOf(this.entries(), this.selfKey()));

  protected readonly gridRows = computed<GridRowVm[]>(() => gridRowsOf(this.draft()));

  /** "Preenchida com a tabela do SRD de Paladino": says where the numbers come from, in the server's words. */
  protected readonly tableNote = computed(() =>
    tableNoteOf(this.casting(), this.defaults(), (k) => this.catalog().nameOf(k)),
  );

  protected readonly questionText = computed(() =>
    questionTextOf(this.question(), this.casting(), this.defaults(), (k) =>
      this.catalog().nameOf(k),
    ),
  );

  constructor() {
    // Keyed on the entry's key and revision, never on the object: archiving hands the page a new object with the same revision,
    // and the master's unsaved draft must stay. A new class waits for the defaults.
    const source = computed(() => {
      const e = this.entry();
      return e ? `${e.key}@${e.revision}` : 'new';
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
      e?.body.case === 'tableClass' ? classToDraft(e.body.value, defaults) : emptyClass(defaults),
    );
    this.openFeature.set('');
    this.question.set(null);
    this.saver.clear();
  }

  private groupOptions(keys: readonly string[]): SelectOption[] {
    const list = this.menu().list('proficiency_targets');
    return keys.map((k) => ({
      value: k,
      label: list.find((p) => p.key === k)?.namePt ?? this.catalog().nameOf(k),
    }));
  }

  protected patch(p: Partial<ClassDraft>): void {
    this.draft.update((d) => ({ ...d, ...p }));
  }

  protected readonly issuesOf = (path: string): readonly string[] => this.saver.issues(path);

  private scrollTo(id: string): void {
    const el = this.host.nativeElement.querySelector<HTMLElement>(`#sec-${id}`);
    el?.scrollIntoView?.({ block: 'start', behavior: 'auto' });
  }

  // ---- The basics.

  protected setSave(i: 0 | 1, ability: number): void {
    const saves: [Ability, Ability] = [...this.draft().saves];
    saves[i] = ability as Ability;
    this.patch({ saves });
  }

  protected toggleSkill(key: string): void {
    this.patch({ skillFrom: toggled(this.draft().skillFrom, key) });
  }

  protected setSkillChoose(n: number): void {
    this.patch({ skillChoose: Math.max(0, n) });
  }

  // ---- The proficiencies.

  protected toggleProficiency(key: string, on: boolean): void {
    this.patch({ proficiencies: withProficiency(this.draft().proficiencies, key, on) });
  }

  protected has(key: string): boolean {
    return this.draft().proficiencies.includes(key);
  }

  protected othersOf(list: readonly string[]): string[] {
    return list.filter((k) => !GROUP_KEYS.includes(k));
  }

  protected addOther(key: string): void {
    this.patch({ proficiencies: [...this.draft().proficiencies, key] });
  }

  protected removeOther(key: string): void {
    this.patch({ proficiencies: this.draft().proficiencies.filter((k) => k !== key) });
  }

  protected addMulticlassProficiency(key: string): void {
    this.patch({ multiclassProficiencies: [...this.draft().multiclassProficiencies, key] });
  }

  protected removeMulticlassProficiency(key: string): void {
    this.patch({
      multiclassProficiencies: this.draft().multiclassProficiencies.filter((k) => k !== key),
    });
  }

  protected readonly requirementGroups = REQUIREMENT_GROUPS;

  protected requirementPath(field: RequirementField, ability?: AbilityField): string {
    return `table_class.${field === 'anyOf' ? 'any_of' : 'minimums'}${ability ? '.' + ability : ''}`;
  }

  protected requirementOptions(): SelectOption[] {
    return this.abilities().map((a) => ({ value: a.field, label: a.name }));
  }

  protected requirementValues(field: RequirementField): AbilityField[] {
    const m = this.draft()[field];
    return ABILITY_FIELDS.filter((a) => m[a] > 0);
  }

  protected abilityName(field: AbilityField): string {
    return this.abilities().find((a) => a.field === field)?.name ?? field;
  }

  protected addRequirement(field: RequirementField, ability: string): void {
    this.patch({ [field]: addedRequirement(this.draft(), field, ability) });
  }

  protected removeRequirement(field: RequirementField, ability: string): void {
    this.patch({ [field]: removedRequirement(this.draft(), field, ability) });
  }

  /** Another ability for a row: the minimum moves with it (a row never takes an ability another row has). */
  protected moveRequirement(field: RequirementField, from: AbilityField, to: string): void {
    const moved = movedRequirement(this.draft(), field, from, to);
    if (moved) this.patch({ [field]: moved });
  }

  protected freeRequirementOptions(field: RequirementField): SelectOption[] {
    const m = this.draft()[field];
    return this.requirementOptions().filter((o) => !(m[o.value as AbilityField] > 0));
  }

  protected setRequirement(field: RequirementField, ability: AbilityField, text: string): void {
    this.patch({ [field]: typedRequirement(this.draft(), field, ability, text) });
  }

  // ---- The casting and the table.

  protected setKind(kind: CastingKind): void {
    this.changeCasting(
      castingOfKind(kind, this.casting(), this.defaults()),
      kind !== this.casting().kind,
    );
  }

  protected setPreparation(preparation: Preparation): void {
    if (preparation === this.casting().preparation) return;
    this.changeCasting(
      castingOfKind(this.casting().kind, { ...this.casting(), preparation }, this.defaults()),
      true,
    );
  }

  protected patchCasting(p: Partial<CastingDraft>): void {
    this.patch({ casting: { ...this.casting(), ...p } });
  }

  /** A new way of casting has a new default table: it replaces the table, unless the master edited it; then it asks first. */
  private changeCasting(next: CastingDraft, tableChanges: boolean): void {
    const d = this.draft();
    if (tableChanges && rowsEdited(d.rows, d.casting, this.defaults())) {
      this.question.set({ kind: 'casting', next });
      return;
    }
    this.patch({
      casting: next,
      rows: tableChanges
        ? rowsOfTable(defaultTableOf(next, this.defaults()), this.defaults())
        : d.rows,
    });
  }

  protected answerQuestion(replace: boolean): void {
    const q = this.question();
    this.question.set(null);
    const answer = q && answeredQuestion(q, replace, this.draft(), this.defaults());
    if (answer) {
      this.patch(answer.patch);
      this.status.set(answer.status);
    }
  }

  protected restore(): void {
    if (rowsEdited(this.draft().rows, this.casting(), this.defaults())) {
      this.question.set({ kind: 'restore' });
      return;
    }
    this.status.set('A tabela já está no padrão.');
  }

  protected editCell(e: GridEdit): void {
    this.patch({ rows: editedRows(this.draft().rows, e) });
  }

  protected setAsi(levels: string[]): void {
    this.patch({ asiLevels: levels.map(Number).sort((a, b) => a - b) });
  }

  protected asiValues(): string[] {
    return this.draft().asiLevels.map(String);
  }

  // ---- The features.

  protected setFeatures(features: LevelFeature[]): void {
    this.patch({ features });
  }

  protected readonly featureBase = (i: number): string =>
    classFeatureBase(this.draft().features, i);

  protected openFromGrid(id: string): void {
    this.openFeature.set(id);
    this.scrollTo('features');
  }

  protected addFromGrid(level: number): void {
    const id = newLevelFeatureId();
    this.patch({
      features: sortedFeatures([...this.draft().features, { id, level, feature: emptyFeature() }]),
    });
    this.openFeature.set(id);
    this.scrollTo('features');
  }

  // ---- Saving.

  protected subclassLink(e: TableEntry): string[] {
    return ['/campaigns', this.campaignId(), 'content', 'entries', e.key];
  }

  protected newSubclassLink(): string[] {
    return ['/campaigns', this.campaignId(), 'content', 'new', 'subclass'];
  }

  private readonly known = (path: string): boolean =>
    isKnownPath(path, this.draft(), this.columns(), this.menu());

  protected async save(): Promise<void> {
    if (this.saver.saving() || this.saveBlocked()) {
      return;
    }
    if (
      this.saver.blockOn(
        unreadableClassPaths(this.draft().casting, this.draft().features, this.menu()),
        (path) => this.reveal(path),
      )
    ) {
      return;
    }
    const body: EntryBody = { case: 'tableClass', value: draftToClass(this.draft(), this.menu()) };
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
      (k) => classFeatureBase(this.draft().features, k),
      path,
    );
    if (feature) {
      this.openFeature.set(feature);
    }
    focusAfterRender(this.host.nativeElement, this.injector, path);
  }
}
