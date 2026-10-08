import {
  Component,
  DestroyRef,
  ElementRef,
  Injector,
  ViewChild,
  afterNextRender,
  computed,
  effect,
  inject,
  signal,
} from '@angular/core';
import { takeUntilDestroyed, toSignal } from '@angular/core/rxjs-interop';
import { FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';
import { CdkStep } from '@angular/cdk/stepper';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatButtonModule } from '@angular/material/button';
import { MatCheckboxModule } from '@angular/material/checkbox';
import { MatDialog } from '@angular/material/dialog';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatIconModule } from '@angular/material/icon';
import { MatInputModule } from '@angular/material/input';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';
import { MatRadioModule } from '@angular/material/radio';
import { MatSelectModule } from '@angular/material/select';
import { ActivatedRoute, ParamMap, Router, RouterLink } from '@angular/router';

import {
  characterBlockedMessage,
  contentRef,
  describeCharacterError,
  invalidFieldPath,
  switchedOffKey,
} from '../../core/characters/character-errors';
import { ContentWatcher } from '../../core/content/content-watcher';
import { LiveSessionSourceLive } from '../live-session/live-session-source.live';
import { catalogChanged, offControlOf, offersChanged } from './catalog-changes';
import {
  abilityLabel,
  characterKindLabel,
  spellLevelLabel,
} from '../../core/characters/character-labels';
import { CharacterKind, isFullSheetKind } from '../../core/characters/characters.types';
import { ActionKey } from '../../core/connect/idempotency';
import { formatXp } from '../../core/format/text';
import { FictionNotice } from '../../shared/fiction-notice/fiction-notice';
import { TableMark } from '../../shared/table-mark/table-mark';
import { AbilityFields } from './ability-fields/ability-fields';
import { AbilityScores } from './ability-scores/ability-scores';
import { TableAbilityScores } from './table-ability-scores/table-ability-scores';
import {
  ALIGNMENT_LABELS,
  AbilityMethodKey,
  AbilityTableVm,
  AlignmentKey,
  CharacterEditorMode,
  CharacterEditorSource,
  CharacterForEdit,
  CharacterFormValue,
  ExtraClassValue,
  HitPointsMethod,
  RulesCatalogVm,
  SpellOptionVm,
} from './character-editor.types';
import { ClassBlockFields } from './class-block/class-block';
import { GrantedSpells } from './granted-spells/granted-spells';
import {
  ClassBlock,
  MAX_TOTAL_LEVEL,
  type CasterSection,
  allOfSection,
  blocksOf,
  cantripsOf,
  casterSections,
  hitDiceAfterFirst,
  leveledOf,
  offered,
  unlisted,
  outsideTheLists,
  sectionName,
  totalLevel,
} from './class-blocks';
import type { SpellDetailsData } from '../../shared/spell-details/spell-details';
import { openSpellDetails } from '../../shared/spell-details/open-spell-details';
import type { SpellDetailsVm } from '../../shared/spell-details/spell-details.types';
import { EditorStepper } from './editor-stepper/editor-stepper';
import { HitPointsRolls } from './hit-points-rolls/hit-points-rolls';
import { validRoll } from './hit-points-preview';
import {
  EDITOR_STEP_LABELS,
  EditorField,
  EditorStepKey,
  FULL_SHEET_FIELDS,
  UNPLACED_RESULTS_FIELD,
  countLabel,
  describeBonusesInUse,
  describeInvalidFields,
  invalidFields,
} from './editor-labels';
import {
  basicFormToValue,
  createBasicForm,
  invalidBasicFields,
  patchBasicForm,
} from './npc-short-form/basic-form';
import { DefeatXp } from './defeat-xp/defeat-xp';
import { PortraitField } from './portrait-field/portrait-field';
import { NpcShortForm } from './npc-short-form/npc-short-form';
import { SkillPicker } from './skill-picker/skill-picker';
import { SpellPicker } from './spell-picker/spell-picker';

/** The NPC route's `:kind` segment (plan §5) to `CharacterKind`. */
const NPC_ROUTE_KINDS: Record<string, CharacterKind> = {
  enemy: 'enemy',
  boss: 'boss',
  minion: 'minion',
  story: 'story',
};

type ReadyState = {
  status: 'ready';
  mode: CharacterEditorMode;
  kind: CharacterKind;
  campaignId: string;
  characterId: string | null;
  revision: number;
  catalog: RulesCatalogVm;
  /** The sheet is locked (a session started, RN-01): the XP is no longer
   * typed here, only the master's awards change it (MR-016). */
  xpLocked: boolean;
};

type ErrorState = {
  status: 'error';
  message: string;
  /** Where "Voltar" goes: the sheet when editing one, else the campaign. */
  backLink: string[];
  backLabel: string;
  /** Set when nothing went wrong, but the sheet may not be edited now
   * (locked or dead): the page names that state instead of an error. */
  blocked?: { title: string };
};

type PageState = { status: 'loading'; title: string } | ErrorState | ReadyState;

/** The page title: known from the route alone, so the loading state shows
 * it too. */
function titleFor(mode: CharacterEditorMode, kind: CharacterKind): string {
  if (mode === 'edit') {
    return 'Editar ficha';
  }
  return kind === 'player' ? 'Criar personagem' : 'Criar NPC';
}

type SavingState = { status: 'idle' } | { status: 'saving' } | { status: 'error'; message: string };

/** The "search box" the catalog-backed pickers use to narrow a long list
 * (cantrips, spells) — a plain case-insensitive substring match on the
 * Portuguese name, no new dependency. */
const BONUS_LIST = new Intl.ListFormat('pt-BR', { type: 'conjunction' });

/** "+2 e +1". */
function bonusWords(bonuses: readonly number[]): string {
  return BONUS_LIST.format(bonuses.map((b) => `+${b}`));
}

function filterByName<T extends { readonly namePt: string }>(
  items: readonly T[],
  query: string,
): readonly T[] {
  const q = query.trim().toLowerCase();
  return q ? items.filter((item) => item.namePt.toLowerCase().includes(q)) : items;
}

/**
 * The character editor (MR-003, MR-005, MR-006): a stepper for a full
 * sheet (player, enemy, boss) — Básico, Habilidades, Perícias, Magias (only
 * for a caster class), Equipamento, `EditorStepper` over the CDK stepper —
 * or a single short form for a basic sheet (minion, story, `NpcShortForm`),
 * routed from three places (plan §5):
 *
 * - `/campaigns/:id/characters/new` — a player creates their character.
 * - `/campaigns/:id/npcs/new/:kind` — a master creates an NPC.
 * - `/campaigns/:id/characters/:characterId/edit` — either edits.
 *
 * The submit action sits outside the stepper, right under the open step,
 * so the whole form can be saved from any step. A submit with an invalid
 * field lists what to fix (`invalidSummary`), marks the steps that have
 * one, and opens the first of them. RN-01 is enforced on the server: this
 * page opens an existing character only when `Character.can_edit` says the
 * caller may save it (otherwise it shows why, as `character-sheet` does),
 * and still renders whatever `describeCharacterError` maps a
 * `failed_precondition` / `SHEET_LOCKED` response to, for a sheet locked
 * while the form was open. No D&D rule runs here: the page never shows a
 * modifier, CA or PV it computed itself.
 *
 * A custom background's two granted skills (`CustomBackground.skill_keys`)
 * are collected separately from the player's own skill proficiencies —
 * `customBackgroundSkills`, capped at 2 (`toggleCustomBackgroundSkill`) —
 * and sent only once exactly two are chosen; the server's advisory check
 * (`DerivedSheet.issues`) covers 0 or 1. Expertise (`expertiseSkillKeys`)
 * is a further, separate selection: always a subset of the proficient
 * skills (`toggleExpertise`, disabled on a skill that is not proficient).
 * `feature_choice_keys` has no field at all: see
 * `character-editor.types.ts`'s doc comment on `CharacterFormValue` for
 * why — everything else `FullSheet` has, this form now covers.
 *
 * **No data loss on save** (integrator fix, phase 2b): on an edit,
 * `CharacterEditorSourceLive.updateCharacter` starts from the `FullSheet`
 * it loaded and overwrites only the fields this form actually edits —
 * `coins` and `feature_choice_keys` (which this form has no UI for) and
 * any class beyond the first (multiclassing; this form only ever edits
 * one) survive a save unchanged instead of being silently wiped. See
 * `character-editor-source.live.spec.ts`'s round-trip test.
 *
 * **Never a typed content key** (integrator fix): race, subrace, class,
 * subclass, background, skills, armor, weapons, cantrips and spells are
 * all picked from `RulesCatalogVm` (`ContentService.ListContent`) —
 * `namePt` shown, `key` sent — never typed as free text, since a typed
 * name almost never matches the real key and the server rejects it with
 * `invalid_argument`. Cantrips and the known/prepared spell lists are
 * filterable checkbox pickers (`availableCantrips`/`availableSpells`,
 * `filteredCantrips`/`filteredSpellsKnown`/`filteredSpellsPrepared`),
 * narrowed to the chosen class's spell list; armor is a select (with "Sem
 * armadura"); weapons is a `<mat-select multiple>`. Only genuinely free
 * text stays free text — see `CharacterFormValue`'s doc comment.
 */
@Component({
  selector: 'app-character-editor',
  imports: [
    ClassBlockFields,
    GrantedSpells,
    TableMark,
    AbilityFields,
    AbilityScores,
    TableAbilityScores,
    CdkStep,
    EditorStepper,
    FictionNotice,
    HitPointsRolls,
    MatButtonModule,
    MatCheckboxModule,
    MatFormFieldModule,
    MatIconModule,
    MatInputModule,
    MatProgressSpinnerModule,
    MatRadioModule,
    MatSelectModule,
    DefeatXp,
    NpcShortForm,
    PortraitField,
    ReactiveFormsModule,
    RouterLink,
    SkillPicker,
    SpellPicker,
  ],
  providers: [ContentWatcher, LiveSessionSourceLive],
  templateUrl: './character-editor.html',
  styleUrl: './character-editor.scss',
})
export class CharacterEditor {
  private readonly source = inject(CharacterEditorSource);
  private readonly createKey = new ActionKey();
  private readonly route = inject(ActivatedRoute);
  private readonly router = inject(Router);
  private readonly destroyRef = inject(DestroyRef);
  private readonly fb = inject(FormBuilder);
  private readonly dialog = inject(MatDialog);
  private readonly bottomSheet = inject(MatBottomSheet);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly injector = inject(Injector);
  // The session's stream says when the table's content changes (10.1d, RN-23): one stream, one read per hint, with the debounce.
  private readonly watcher = inject(ContentWatcher);
  /** The spell descriptions already fetched, by spell key, for the life of
   * the page (a second "?" on the same spell is instant; nothing is stored
   * in the browser). A failed fetch is dropped so "Tentar de novo" asks again. */
  private readonly spellDetails = new Map<string, Promise<SpellDetailsVm>>();

  @ViewChild(EditorStepper) private stepper?: EditorStepper;

  protected readonly state = signal<PageState>({ status: 'loading', title: 'Ficha' });
  /** The class block a refusal points at (`full.classes[i]`), so the block says so. */
  protected readonly serverClassProblem = signal<number | null>(null);
  protected readonly saveState = signal<SavingState>({ status: 'idle' });
  protected readonly selectedSkills = signal<ReadonlySet<string>>(new Set());
  /** The custom background's two granted skills (`CustomBackground.skills`,
   * integrator fix, phase 2) — a separate, capped-at-2 selection from
   * `selectedSkills`, only shown when `background === 'custom'`. */
  protected readonly customBackgroundSkills = signal<ReadonlySet<string>>(new Set());
  /** The "Outro" background's two tools or languages (content keys), capped at 2 like its skills. */
  protected readonly customBackgroundProficiencies = signal<readonly string[]>([]);
  /** The classes after the first (multiclass at creation): one block each, see `ClassBlockFields`. */
  protected readonly extraClasses = signal<readonly ExtraClassValue[]>([]);
  /** Expertise doubles a skill's proficiency bonus — always a subset of
   * `selectedSkills` (integrator fix, phase 2b). */
  protected readonly expertiseSkills = signal<ReadonlySet<string>>(new Set());
  /** One roll per level after the first, index 0 = level 2. Only sent when
   * `hitPointsMethod` is "rolled" (integrator fix, phase 2b). */
  protected readonly hitPointsRolls = signal<readonly number[]>([]);

  /** Catalog-backed pickers (integrator fix: the editor must never make a
   * person type a content key) — cantrips and the known/prepared spell
   * lists, each a filterable checkbox list over `RulesCatalogVm.spells`. */
  protected readonly selectedCantrips = signal<ReadonlySet<string>>(new Set());
  protected readonly selectedSpellsKnown = signal<ReadonlySet<string>>(new Set());
  protected readonly selectedSpellsPrepared = signal<ReadonlySet<string>>(new Set());
  /** The search of each list, by `<section>:<list>`: "0:cantrips", "1:known", "1:prepared". */
  private readonly spellFilters = signal<Readonly<Record<string, string>>>({});
  /** Spells the sheet has that neither a pick of this form nor its subclass gave it (a race's or a feature's spell):
   * only known on an edit, from the server's own derived sheet, as it was when the sheet was opened. */
  protected readonly grantedSpells = signal<readonly SpellOptionVm[]>([]);

  protected readonly isFullSheetKind = isFullSheetKind;
  protected readonly stepLabels = EDITOR_STEP_LABELS;
  protected readonly alignmentKeys: readonly AlignmentKey[] = [
    '',
    'lawful_good',
    'neutral_good',
    'chaotic_good',
    'lawful_neutral',
    'neutral',
    'chaotic_neutral',
    'lawful_evil',
    'neutral_evil',
    'chaotic_evil',
  ];
  protected readonly alignmentLabel = (key: AlignmentKey) => ALIGNMENT_LABELS[key];

  protected readonly fullForm = this.fb.nonNullable.group({
    name: ['', [Validators.required, Validators.maxLength(80)]],
    race: ['', Validators.required],
    subrace: [''],
    className: ['', Validators.required],
    subclassName: [''],
    customSubclassName: [''],
    level: [1, [Validators.required, Validators.min(1), Validators.max(20)]],
    background: ['', Validators.required],
    customBackgroundName: ['', Validators.maxLength(40)],
    // The "Outro" background's feature (a name and a text) and its equipment, in the player's words.
    customBackgroundFeatureName: ['', Validators.maxLength(40)],
    customBackgroundFeatureText: ['', Validators.maxLength(1000)],
    customBackgroundEquipment: ['', Validators.maxLength(500)],
    /** A content key from the catalog, or `''` for "Sem armadura" — never
     * typed (integrator fix). */
    armor: [''],
    shield: [false],
    /** Content keys, `<mat-select multiple>` — never typed. */
    weaponKeys: [[] as string[]],
    equipmentText: ['', Validators.maxLength(4000)],
    languagesText: ['', Validators.maxLength(2000)],
    toolProficienciesText: ['', Validators.maxLength(2000)],
    experiencePoints: [0, [Validators.required, Validators.min(0), Validators.max(1000000)]],
    // An enemy's or boss's ND and the XP it gives when defeated (E7-11); a
    // player's stay empty and 0 and are never shown.
    challengeRating: [''],
    xpValue: [0, [Validators.required, Validators.min(0), Validators.max(1000000)]],
    // An enemy's or boss's portrait (MR-031): a gallery image's ID, or empty.
    portraitImageId: [''],
    alignment: ['' as AlignmentKey],
    customFeaturesText: ['', Validators.maxLength(5000)],
    hitPointsMethod: ['average' as HitPointsMethod],
    abilities: this.fb.nonNullable.group({
      str: [10, [Validators.required, Validators.min(1), Validators.max(30)]],
      dex: [10, [Validators.required, Validators.min(1), Validators.max(30)]],
      con: [10, [Validators.required, Validators.min(1), Validators.max(30)]],
      int: [10, [Validators.required, Validators.min(1), Validators.max(30)]],
      wis: [10, [Validators.required, Validators.min(1), Validators.max(30)]],
      cha: [10, [Validators.required, Validators.min(1), Validators.max(30)]],
    }),
    extraAbilityBonuses: this.fb.nonNullable.group({
      str: [0, [Validators.required, Validators.min(-10), Validators.max(10)]],
      dex: [0, [Validators.required, Validators.min(-10), Validators.max(10)]],
      con: [0, [Validators.required, Validators.min(-10), Validators.max(10)]],
      int: [0, [Validators.required, Validators.min(-10), Validators.max(10)]],
      wis: [0, [Validators.required, Validators.min(-10), Validators.max(10)]],
      cha: [0, [Validators.required, Validators.min(-10), Validators.max(10)]],
    }),
  });

  protected readonly basicForm = createBasicForm(this.fb);

  /** The locked sheet's XP, to read ("2.716 XP"): only the master's awards change it. */
  private readonly experienceXp = toSignal(this.fullForm.controls.experiencePoints.valueChanges, {
    initialValue: 0,
  });
  protected readonly experienceLabel = computed(() => formatXp(this.experienceXp()));

  /** "Rolar 4d6" or "Conjunto padrão" with results still to place: saving
   * waits, so a half-placed roll never turns into six default 10s. */
  protected readonly abilitiesIncomplete = signal(false);
  /** What the table's way of making scores still lacks, in words ("role as habilidades"), `''` when it is complete. */
  protected readonly abilitiesProblem = signal('');
  /** The table's ways of making scores, when a player makes a new sheet (RN-24); `null` is the free editor of the master's NPCs and of an edit. */
  protected readonly abilityTable = signal<AbilityTableVm | null>(null);
  /** The way the player chose; the server checks the scores against it. */
  protected readonly abilityMethod = signal<AbilityMethodKey>('typed');
  /** A player editing their own draft: the way the server recorded the scores were made, which the step keeps (RN-24). */
  protected readonly lockedOrigin = signal<NonNullable<CharacterForEdit['abilityOrigin']> | null>(
    null,
  );
  /** What the table leaves of the hit points of a new sheet: both ways, only "Rolado" or only "Média". */
  protected readonly hpRule = computed<'player_chooses' | 'roll' | 'average'>(() => {
    const s = this.state();
    return s.status === 'ready' && s.mode === 'create'
      ? (this.abilityTable()?.hitPoints ?? 'player_chooses')
      : 'player_chooses';
  });

  protected readonly selectedRaceKey = toSignal(this.fullForm.controls.race.valueChanges, {
    initialValue: '',
  });
  protected readonly selectedClassKey = toSignal(this.fullForm.controls.className.valueChanges, {
    initialValue: '',
  });
  protected readonly selectedBackground = toSignal(this.fullForm.controls.background.valueChanges, {
    initialValue: '',
  });
  private readonly selectedLevel = toSignal(this.fullForm.controls.level.valueChanges, {
    initialValue: 1,
  });
  protected readonly selectedSubclassKey = toSignal(
    this.fullForm.controls.subclassName.valueChanges,
    {
      initialValue: '',
    },
  );
  private readonly selectedCustomSubclass = toSignal(
    this.fullForm.controls.customSubclassName.valueChanges,
    { initialValue: '' },
  );
  protected readonly selectedHitPointsMethod = toSignal(
    this.fullForm.controls.hitPointsMethod.valueChanges,
    { initialValue: 'average' as HitPointsMethod },
  );
  /** Every class of the sheet, in the order of the blocks: the first one is the form's own fields. */
  protected readonly blocks = computed<ClassBlock[]>(() =>
    blocksOf(
      {
        classKey: this.selectedClassKey(),
        level: this.selectedLevel(),
        subclassKey: this.selectedSubclassKey(),
        customSubclassName: this.selectedCustomSubclass(),
      },
      this.extraClasses(),
    ),
  );
  /** More than one class: the Básico step shows one block per class and the total level. */
  protected readonly multiclass = computed(() => this.extraClasses().length > 0);
  protected readonly totalLevelValue = computed(() => totalLevel(this.blocks()));
  /** The level the whole sheet has, up to 20, or why it does not fit. */
  protected readonly totalLevelProblem = computed(() =>
    this.totalLevelValue() > MAX_TOTAL_LEVEL ? `O nível total vai até ${MAX_TOTAL_LEVEL}.` : '',
  );
  /** How many rolls "Dados de Vida" needs: one per level after the first, of all the classes together. */
  protected readonly rollsNeeded = computed(() => Math.max(0, this.totalLevelValue() - 1));
  /** The die of each of those levels, which differs between the classes of a multiclass. */
  protected readonly levelDice = computed(() => {
    const s = this.state();
    return s.status === 'ready' ? hitDiceAfterFirst(s.catalog, this.blocks()) : [];
  });
  protected readonly selectedSubraceKey = toSignal(this.fullForm.controls.subrace.valueChanges, {
    initialValue: '',
  });
  private readonly baseScores = toSignal(this.fullForm.controls.abilities.valueChanges, {
    initialValue: this.fullForm.controls.abilities.getRawValue(),
  });

  protected readonly availableSubraces = computed(() => {
    const s = this.state();
    if (s.status !== 'ready') {
      return [];
    }
    return s.catalog.races.find((r) => r.key === this.selectedRaceKey())?.subraces ?? [];
  });

  /** The Portuguese name of any entry of the catalog, for a message that names one (never the key itself). */
  private nameOfKey(key: string): string | undefined {
    const s = this.state();
    if (s.status !== 'ready') {
      return undefined;
    }
    const c = s.catalog;
    return [
      ...c.races,
      ...c.classes,
      ...c.backgrounds,
      ...c.spells,
      ...c.races.flatMap((r) => r.subraces),
      ...c.classes.flatMap((k) => k.subclasses),
    ].find((e) => e.key === key)?.namePt;
  }

  /** The caller is the master: the one who is offered what is switched off for the players. */
  protected readonly master = computed(() => {
    const s = this.state();
    return s.status === 'ready' && s.catalog.viewerIsMaster;
  });
  /** What each list offers as a NEW choice: never a retired entry, unless it is the value the form already has. */
  protected readonly raceOptions = computed(() => {
    const s = this.state();
    return s.status === 'ready'
      ? offered(s.catalog.races, [this.selectedRaceKey()], this.master())
      : [];
  });
  protected readonly subraceOptions = computed(() =>
    offered(this.availableSubraces(), [this.selectedSubraceKey()], this.master()),
  );
  protected readonly classOptions = computed(() => {
    const s = this.state();
    return s.status === 'ready'
      ? offered(s.catalog.classes, [this.selectedClassKey()], this.master())
      : [];
  });
  protected readonly subclassOptions = computed(() =>
    offered(this.selectedClass()?.subclasses ?? [], [this.selectedSubclassKey()], this.master()),
  );
  protected readonly backgroundOptions = computed(() => {
    const s = this.state();
    return s.status === 'ready'
      ? offered(s.catalog.backgrounds, [this.selectedBackground()], this.master())
      : [];
  });
  /** A value the catalog does not list at all (a key from an old sheet): the select says so instead of staying blank. */
  protected readonly raceUnlisted = computed(() =>
    unlisted(
      this.state().status === 'ready' ? (this.state() as ReadyState).catalog.races : [],
      this.selectedRaceKey(),
    ),
  );
  protected readonly classUnlisted = computed(() =>
    unlisted(
      this.state().status === 'ready' ? (this.state() as ReadyState).catalog.classes : [],
      this.selectedClassKey(),
    ),
  );
  protected readonly backgroundUnlisted = computed(
    () =>
      this.selectedBackground() !== 'custom' &&
      unlisted(
        this.state().status === 'ready' ? (this.state() as ReadyState).catalog.backgrounds : [],
        this.selectedBackground(),
      ),
  );

  protected readonly selectedClass = computed(() => {
    const s = this.state();
    if (s.status !== 'ready') {
      return undefined;
    }
    return s.catalog.classes.find((c) => c.key === this.selectedClassKey());
  });

  /** Faces of the chosen class's hit die, or 0 before a class is chosen. */
  protected readonly hitDie = computed(() => this.selectedClass()?.hitDie ?? 0);

  /** The Constitution the rolled hit points use: the typed score plus what
   * the race and subrace give plus the manual bonus (the same sum the
   * server makes; this one only feeds the preview). */
  protected readonly finalConstitution = computed(() => {
    const s = this.state();
    const race =
      s.status === 'ready'
        ? s.catalog.races.find((r) => r.key === this.selectedRaceKey())
        : undefined;
    const subrace = race?.subraces.find((sr) => sr.key === this.selectedSubraceKey());
    return (
      (this.baseScores().con ?? 0) +
      (race?.constitutionBonus ?? 0) +
      (subrace?.constitutionBonus ?? 0) +
      (this.bonusesValue().con ?? 0)
    );
  });

  /** The spell lists the sheet reads from: one section per class that casts, or per subclass that
   * casts (a third caster), from its level. Empty for a sheet that casts nothing: no "Magias" step. */
  protected readonly sections = computed<CasterSection[]>(() => {
    const s = this.state();
    return s.status === 'ready' ? casterSections(s.catalog, this.blocks()) : [];
  });
  protected readonly isCaster = computed(() => this.sections().length > 0);
  /** The heading of a section when there is more than one: "Clérigo · 1º nível". */
  protected sectionHeading(section: CasterSection): string {
    const name = section.subclassNamePt
      ? `${section.namePt} · ${section.subclassNamePt}`
      : section.namePt;
    const max = section.maxCircle;
    return max === null
      ? name
      : max === 0
        ? `${name} · só truques`
        : `${name} · até o ${max}º nível de magia`;
  }
  protected sectionLabel = sectionName;

  /** Every cantrip and spell of the sheet's lists, for "the chosen line" and for what a save may send. */
  protected readonly availableCantrips = computed(() =>
    this.unionOf((sec, spells) => cantripsOf(spells, sec, new Set(this.selectedCantrips()), true)),
  );
  protected readonly availableSpells = computed(() =>
    this.unionOf((sec, spells) => allOfSection(spells, sec).filter((sp) => sp.level >= 1)),
  );
  private unionOf(
    pick: (section: CasterSection, spells: readonly SpellOptionVm[]) => SpellOptionVm[],
  ): SpellOptionVm[] {
    const s = this.state();
    if (s.status !== 'ready') {
      return [];
    }
    const seen = new Map<string, SpellOptionVm>();
    for (const section of this.sections()) {
      for (const spell of pick(section, s.catalog.spells)) {
        seen.set(spell.key, spell);
      }
    }
    return [...seen.values()].sort(
      (a, b) => a.level - b.level || a.namePt.localeCompare(b.namePt, 'pt-BR'),
    );
  }

  /** What each section's pickers list now: the cantrips, the spells up to the circle the class level
   * reaches (plus any picked above it, so it can be unchecked), each narrowed by its own search. */
  protected readonly sectionViews = computed(() => {
    const s = this.state();
    if (s.status !== 'ready') {
      return [];
    }
    const filters = this.spellFilters();
    const query = (section: CasterSection, list: string) =>
      filters[`${section.index}:${list}`] ?? '';
    return this.sections().map((section) => {
      const cantrips = cantripsOf(
        s.catalog.spells,
        section,
        this.selectedCantrips(),
        this.master(),
      );
      // An always-prepared spell is never a pick: it is in its locked row, free, and never in these lists.
      const alwaysKeys = new Set(section.alwaysPrepared);
      const known = leveledOf(
        s.catalog.spells,
        section,
        this.selectedSpellsKnown(),
        this.master(),
      ).filter((sp) => !alwaysKeys.has(sp.key));
      const prepared = leveledOf(
        s.catalog.spells,
        section,
        this.selectedSpellsPrepared(),
        this.master(),
      ).filter((sp) => !alwaysKeys.has(sp.key));
      // The subclass's always-prepared spells, shown in this class's section, checked and locked, never in the count.
      const byKey = new Map(s.catalog.spells.map((sp) => [sp.key, sp]));
      const always = section.alwaysPrepared.flatMap((k) => (byKey.has(k) ? [byKey.get(k)!] : []));
      const max = this.preparedMaxByClass()[section.classKey];
      const picked = prepared.filter(
        (sp) => this.selectedSpellsPrepared().has(sp.key) && !alwaysKeys.has(sp.key),
      ).length;
      const preparation = section.preparation;
      const noLeveledYet =
        section.maxCircle === 0 &&
        this.selectedSpellsKnown().size === 0 &&
        this.selectedSpellsPrepared().size === 0;
      return {
        section,
        id: String(section.index),
        cantrips: {
          all: cantrips,
          shown: filterByName(cantrips, query(section, 'cantrips')),
          filter: query(section, 'cantrips'),
          // A cantrip already on the sheet keeps the picker, so it can still be unchecked.
          show:
            cantrips.length > 0 ||
            (this.sections().length === 1 && this.selectedCantrips().size > 0),
          outside: outsideTheLists(s.catalog, this.sections(), query(section, 'cantrips'), true),
        },
        known: {
          all: known,
          shown: filterByName(known, query(section, 'known')),
          filter: query(section, 'known'),
          show: !noLeveledYet && (preparation === 'known' || preparation === 'spellbook'),
          outside: outsideTheLists(s.catalog, this.sections(), query(section, 'known'), false),
        },
        prepared: {
          all: prepared,
          shown: filterByName(prepared, query(section, 'prepared')),
          filter: query(section, 'prepared'),
          show: !noLeveledYet && (preparation === 'prepared' || preparation === 'spellbook'),
          outside: outsideTheLists(s.catalog, this.sections(), query(section, 'prepared'), false),
        },
        always,
        alwaysSourcePt: section.alwaysSourcePt,
        // On an edit the server says how many the class prepares (its own number, from the saved sheet).
        preparedCount:
          section.preparation !== 'known' && max !== undefined
            ? `Preparadas ${picked} de ${max}`
            : '',
        noLeveledYet,
        castingStarts: `O ${sectionName(section)} conjura magias a partir do nível ${section.firstLevel}.`,
      };
    });
  });
  /** How many spells each casting class prepares, from the saved sheet's derived numbers (an edit only). */
  protected readonly preparedMaxByClass = signal<Readonly<Record<string, number>>>({});

  /** What the sheet has that no pick of this form gave it and no subclass of the catalog explains (a race's or a
   * feature's spell): said apart, with where it comes from. */
  protected readonly otherGranted = this.grantedSpells.asReadonly();

  protected setSpellFilter(
    section: number,
    list: 'cantrips' | 'known' | 'prepared',
    value: string,
  ): void {
    this.spellFilters.update((f) => ({ ...f, [`${section}:${list}`]: value }));
  }

  /** The entries chosen in the closed selects, for the "Da mesa" tag beside the name (a select shows only text). */
  protected readonly selectedRace = computed(() => {
    const s = this.state();
    return s.status === 'ready'
      ? s.catalog.races.find((r) => r.key === this.selectedRaceKey())
      : undefined;
  });
  protected readonly selectedSubrace = computed(() =>
    this.availableSubraces().find((r) => r.key === this.selectedSubraceKey()),
  );
  protected readonly selectedSubclass = computed(() =>
    this.selectedClass()?.subclasses.find((c) => c.key === this.selectedSubclassKey()),
  );
  protected readonly selectedBackgroundVm = computed(() => {
    const s = this.state();
    return s.status === 'ready'
      ? s.catalog.backgrounds.find((b) => b.key === this.selectedBackground())
      : undefined;
  });

  /** What the first class asks of the skills and the saving throws, from the server's entry for it. */
  protected readonly skillNote = computed(() => {
    const c = this.selectedClass();
    if (!c || c.skillChoose <= 0) {
      return '';
    }
    const saves = c.savingThrows.map((a) => abilityLabel(a));
    const savesText =
      saves.length > 0
        ? ` e dá proficiência nos testes de resistência de ${BONUS_LIST.format(saves)}`
        : '';
    return `O ${c.namePt} escolhe ${c.skillChoose} ${c.skillChoose === 1 ? 'perícia' : 'perícias'} ao começar${savesText}.`;
  });
  protected readonly spellCircle = spellLevelLabel;
  /** "Ver em Magias", for a spell the lists leave out. */
  protected readonly magiasLink = computed(() => {
    const s = this.state();
    return s.status === 'ready' ? ['/campaigns', s.campaignId, 'spells'] : [];
  });

  /** The race's "+2 e +1 à sua escolha": where the player puts them (the server's numbers, from the catalog). */
  protected readonly choiceBonusesHint = computed(() => {
    const s = this.state();
    const race =
      s.status === 'ready'
        ? s.catalog.races.find((r) => r.key === this.selectedRaceKey())
        : undefined;
    const bonuses = race?.choiceBonuses ?? [];
    return bonuses.length > 0
      ? `Dá ${bonusWords(bonuses)} nas habilidades que você escolher: ponha em “Bônus manuais”, no passo Habilidades.`
      : '';
  });
  /** A table background's equipment, as the master wrote it. */
  protected readonly backgroundEquipment = computed(() => {
    const s = this.state();
    return s.status === 'ready'
      ? (s.catalog.backgrounds.find((b) => b.key === this.selectedBackground())?.equipmentPt ?? '')
      : '';
  });
  protected readonly toolOptions = computed(() => this.toolsAndLanguages('tool'));
  protected readonly languageOptions = computed(() => this.toolsAndLanguages('language'));
  private toolsAndLanguages(kind: 'tool' | 'language') {
    const s = this.state();
    return s.status === 'ready' ? s.catalog.toolsAndLanguages.filter((t) => t.kind === kind) : [];
  }

  /** The hint under "Subclasse" while the level is below the one where the
   * class chooses it; empty once the level reaches it. */
  protected readonly subclassHint = computed(() => {
    const c = this.selectedClass();
    const level = this.selectedLevel();
    return c && c.subclassLevel > 0 && !(level >= c.subclassLevel)
      ? `O ${c.namePt} escolhe a subclasse no nível ${c.subclassLevel}.`
      : '';
  });

  /** The player picked another class: a subclass belongs to one class, so the
   * old one (SRD or custom) must not survive. Wired to the select's
   * `selectionChange`, not `valueChanges`, because loading a saved sheet also
   * sets the class and must keep its subclass. */
  protected onClassChange(): void {
    this.fullForm.patchValue({ subclassName: '', customSubclassName: '' });
  }

  /** A block of the multiclass layout changed: block 0 is the form's own fields, the rest the extra classes. */
  protected changeBlock(index: number, change: Partial<ClassBlock>): void {
    if (index === 0) {
      const f = this.fullForm.controls;
      if (change.classKey !== undefined) f.className.setValue(change.classKey);
      if (change.level !== undefined) f.level.setValue(change.level);
      if (change.subclassKey !== undefined) f.subclassName.setValue(change.subclassKey);
      if (change.customSubclassName !== undefined)
        f.customSubclassName.setValue(change.customSubclassName);
      return;
    }
    this.extraClasses.update((list) =>
      list.map((c, i) => (i === index - 1 ? { ...c, ...change } : c)),
    );
  }

  /** The classes the other blocks already use: a class cannot be taken twice (the SRD's multiclass), so they are not offered here. */
  protected otherClassKeys(index: number): readonly string[] {
    return this.blocks()
      .filter((_, i) => i !== index)
      .map((b) => b.classKey)
      .filter((k) => k !== '');
  }

  /** "Adicionar classe": a new block at level 1 with nothing chosen. The server checks the multiclass
   * prerequisites of every class and shows what the sheet does not meet as issues on the sheet. */
  protected addClass(): void {
    this.extraClasses.update((list) => [
      ...list,
      { classKey: '', level: 1, subclassKey: '', customSubclassName: '' },
    ]);
  }

  protected removeClass(index: number): void {
    this.extraClasses.update((list) => list.filter((_, i) => i !== index - 1));
  }

  /** Set by a submit with an invalid field: from then on, the notice above
   * the buttons lists what is still wrong, and the steps that have it are
   * marked, until everything is fixed. */
  private readonly showErrors = signal(false);
  /** A save was tried: the blocks that lack a class say so. */
  protected readonly submitTried = this.showErrors.asReadonly();
  private readonly fullFormValue = toSignal(this.fullForm.valueChanges);
  private readonly basicFormValue = toSignal(this.basicForm.valueChanges);
  private readonly invalidFullFields = computed(() => {
    this.fullFormValue();
    return this.showErrors() ? this.currentInvalidFullFields() : [];
  });
  private readonly invalidBasicFields = computed(() => {
    this.basicFormValue();
    return this.showErrors() ? invalidBasicFields(this.basicForm) : [];
  });
  /** "Básico: Nome do personagem, Raça. Habilidades: Força." — or `''` when
   * nothing needs fixing (or no submit was tried yet). */
  protected readonly invalidSummary = computed(() => {
    const s = this.state();
    if (s.status !== 'ready') {
      return '';
    }
    return describeInvalidFields(
      isFullSheetKind(s.kind) ? this.invalidFullFields() : this.invalidBasicFields(),
    );
  });
  private readonly stepsWithErrors = computed(
    () => new Set(this.invalidFullFields().map((field) => field.step)),
  );

  /** "Bônus manuais" starts closed; a submit with an invalid bonus opens it. */
  protected readonly bonusesOpen = signal(false);
  private readonly bonusesValue = toSignal(
    this.fullForm.controls.extraAbilityBonuses.valueChanges,
    {
      initialValue: this.fullForm.controls.extraAbilityBonuses.getRawValue(),
    },
  );
  /** "Constituição +1, Inteligência +2", so the closed section still says
   * what it holds. */
  protected readonly bonusesInUse = computed(() => describeBonusesInUse(this.bonusesValue()));

  protected readonly backgroundSkillsCount = computed(() =>
    countLabel(
      this.customBackgroundSkills().size,
      'de 2 escolhida',
      'de 2 escolhidas',
      'Nenhuma escolhida',
    ),
  );

  /** The line under the title: what this form is for, and that the
   * sheet's numbers come from the server. */
  protected readonly lead = computed(() => {
    const s = this.state();
    if (s.status !== 'ready') {
      return '';
    }
    const kind = characterKindLabel(s.kind);
    if (s.mode === 'edit') {
      return isFullSheetKind(s.kind)
        ? 'As mudanças valem quando você salvar. Modificadores, Classe de Armadura e pontos de vida são recalculados na hora.'
        : `Ficha curta de ${kind.toLowerCase()}.`;
    }
    if (s.kind === 'player') {
      return 'Preencha os passos na ordem que quiser. Modificadores, Classe de Armadura e pontos de vida são calculados quando você criar.';
    }
    return isFullSheetKind(s.kind)
      ? `${kind} com ficha completa, como a de um jogador. Só você vê os NPCs da campanha.`
      : `${kind}: ficha curta, só com o que se usa na mesa. Só você vê os NPCs da campanha.`;
  });

  protected readonly pageTitle = computed(() => {
    const s = this.state();
    if (s.status === 'loading') {
      return s.title;
    }
    if (s.status === 'error') {
      return s.blocked?.title ?? 'Não foi possível abrir o formulário';
    }
    return titleFor(s.mode, s.kind);
  });

  /** Cancel goes back where the person came from: the sheet being edited,
   * or the campaign. */
  protected readonly cancelLink = computed(() => {
    const s = this.state();
    if (s.status !== 'ready') {
      return ['/'];
    }
    return s.characterId
      ? ['/campaigns', s.campaignId, 'characters', s.characterId]
      : ['/campaigns', s.campaignId];
  });

  /** What the buttons do, in one line next to them. */
  protected readonly actionsNote = computed(() => {
    const s = this.state();
    if (s.status !== 'ready') {
      return '';
    }
    const full = isFullSheetKind(s.kind);
    if (s.mode === 'create') {
      return full
        ? 'Cria com o que estiver preenchido em todos os passos. Cancelar volta para a campanha sem criar.'
        : 'Cancelar volta para a campanha sem criar.';
    }
    return full
      ? 'Salva todos os passos de uma vez. Cancelar volta para a ficha sem salvar.'
      : 'Cancelar volta para a ficha sem salvar.';
  });

  protected stepHasError(step: EditorStepKey): boolean {
    return this.stepsWithErrors().has(step);
  }

  /** What the page says after the table's content changed under the person ("" when nothing did). */
  protected readonly contentNote = signal('');
  private readonly campaignIdSignal = signal('');

  constructor() {
    // Only a player's editor follows the hint: the master writes the content himself, and an open stream would keep the
    // page from ever being quiet (every NPC editor of a live campaign would hold one).
    this.watcher.whileLive(
      () => (this.master() ? '' : this.campaignIdSignal()),
      () => void this.refreshCatalog(),
    );
    // The subclass of a one-class sheet waits for the level the class chooses it at (as in a block), unless one is chosen.
    effect(() => {
      const c = this.selectedClass();
      const waits =
        !!c &&
        c.subclassLevel > 0 &&
        this.selectedLevel() < c.subclassLevel &&
        this.selectedSubclassKey() === '';
      const control = this.fullForm.controls.subclassName;
      if (waits && control.enabled) {
        control.disable({ emitEvent: false });
      } else if (!waits && control.disabled) {
        control.enable({ emitEvent: false });
      }
    });
    this.destroyRef.onDestroy(() => {
      this.destroyed = true;
      this.loadSeq++;
    });
    this.route.paramMap.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((params) => {
      this.resolveAndLoad(params);
    });
  }

  /** Counts the loads, so a late answer for a route the person left is dropped, and a save that outlives its route does not navigate. */
  private loadSeq = 0;
  /** Counts the reads of the lists after a `content_changed`, so an older answer never replaces a newer one. */
  private catalogSeq = 0;
  private destroyed = false;

  /** A reused component starts every route from nothing: nothing of the previous character stays in the form or the pick sets. */
  private resetEditing(): void {
    this.fullForm.reset();
    this.basicForm.reset();
    this.selectedSkills.set(new Set());
    this.customBackgroundSkills.set(new Set());
    this.customBackgroundProficiencies.set([]);
    this.extraClasses.set([]);
    this.expertiseSkills.set(new Set());
    this.hitPointsRolls.set([]);
    this.selectedCantrips.set(new Set());
    this.selectedSpellsKnown.set(new Set());
    this.selectedSpellsPrepared.set(new Set());
    this.spellFilters.set({});
    this.grantedSpells.set([]);
    this.preparedMaxByClass.set({});
    this.abilityTable.set(null);
    this.abilityMethod.set('typed');
    this.lockedOrigin.set(null);
    this.serverClassProblem.set(null);
    this.saveState.set({ status: 'idle' });
    this.contentNote.set('');
  }

  private resolveAndLoad(params: ParamMap): void {
    const campaignId = params.get('id');
    if (!campaignId) {
      return;
    }
    const characterId = params.get('characterId');
    const npcKind = params.get('kind');
    this.campaignIdSignal.set(campaignId);

    if (characterId) {
      this.loadForEdit(campaignId, characterId);
      return;
    }
    const kind: CharacterKind = npcKind ? (NPC_ROUTE_KINDS[npcKind] ?? 'enemy') : 'player';
    this.loadForCreate(campaignId, kind);
  }

  private loadForCreate(campaignId: string, kind: CharacterKind): void {
    const seq = ++this.loadSeq;
    this.resetEditing();
    this.state.set({ status: 'loading', title: titleFor('create', kind) });
    // A player (or a pending member) makes the scores the table's rules allow; the master's NPCs are free.
    const table =
      kind === 'player' ? this.source.loadAbilityTable(campaignId) : Promise.resolve(null);
    Promise.all([this.source.loadCatalog(campaignId), table]).then(
      ([catalog, abilityTable]) => {
        if (seq !== this.loadSeq) {
          return;
        }
        this.abilityTable.set(abilityTable);
        if (abilityTable?.hitPoints === 'roll') {
          this.fullForm.controls.hitPointsMethod.setValue('rolled');
        } else if (abilityTable?.hitPoints === 'average') {
          this.fullForm.controls.hitPointsMethod.setValue('average');
        }
        this.state.set({
          status: 'ready',
          mode: 'create',
          kind,
          campaignId,
          characterId: null,
          revision: 1,
          catalog,
          xpLocked: false,
        });
        // A new enemy, boss or minion starts at ND 0 and 10 XP, so nobody is left without a
        // number; a story NPC gives none (and never shows the fields).
        if (kind === 'enemy' || kind === 'boss') {
          this.fullForm.patchValue({ challengeRating: '0', xpValue: 10 });
        } else if (kind === 'story') {
          this.basicForm.patchValue({ challengeRating: '', xpValue: 0 });
        }
      },
      (err: unknown) => {
        if (seq !== this.loadSeq) {
          return;
        }
        this.state.set({
          status: 'error',
          message: describeCharacterError(err),
          backLink: ['/campaigns', campaignId],
          backLabel: 'Voltar para a campanha',
        });
      },
    );
  }

  /** The table's rules for the scores changed under the person (the server refused the way of rolling the step offered):
   * they are read again, so the step offers what is allowed now. */
  protected async rereadAbilityTable(): Promise<void> {
    const s = this.state();
    if (s.status !== 'ready' || !this.abilityTable()) {
      return;
    }
    const seq = this.loadSeq;
    try {
      const table = await this.source.loadAbilityTable(s.campaignId);
      if (seq === this.loadSeq && table) {
        this.abilityTable.set(table);
      }
    } catch {
      // The refusal the step already shows stays; the next try reads the table again.
    }
  }

  private loadForEdit(campaignId: string, characterId: string): void {
    const seq = ++this.loadSeq;
    this.resetEditing();
    this.state.set({ status: 'loading', title: titleFor('edit', 'player') });
    Promise.all([
      // With the sheet: what it has comes back even when the master retired it since.
      this.source.loadCatalog(campaignId, characterId),
      this.source.loadCharacterForEdit(campaignId, characterId),
    ])
      .then(async ([catalog, existing]) => {
        // A player's own draft keeps the way its scores were made (RN-24): the step shows that way and its limits.
        // The master (and an NPC, or a sheet made before the rules) keeps the free editor.
        const origin = existing.abilityOrigin ?? null;
        const table =
          origin && existing.kind === 'player'
            ? await this.source.loadAbilityTable(campaignId)
            : null;
        if (seq !== this.loadSeq) {
          return null;
        }
        this.abilityTable.set(table);
        this.lockedOrigin.set(table ? origin : null);
        return [catalog, existing] as const;
      })
      .then((loaded) => {
        if (loaded === null || seq !== this.loadSeq) {
          return;
        }
        const [catalog, existing] = loaded;
        if (existing.blocked) {
          // Say it before the form: a player who opens the edit URL of a
          // locked sheet would otherwise fill it in and only learn at "Salvar".
          this.state.set({
            status: 'error',
            message: characterBlockedMessage(existing.blocked),
            backLink: ['/campaigns', campaignId, 'characters', characterId],
            backLabel: 'Voltar para a ficha',
            blocked: {
              title: existing.blocked === 'character_dead' ? 'Personagem morto' : 'Ficha travada',
            },
          });
          return;
        }
        this.state.set({
          status: 'ready',
          mode: 'edit',
          kind: existing.kind,
          campaignId,
          characterId,
          revision: existing.revision,
          catalog,
          xpLocked: existing.sheetLocked,
        });
        if (existing.full) {
          this.patchFullForm(existing.full);
          this.preparedMaxByClass.set(existing.preparedMax ?? {});
          // What the sheet has from outside the form is told once, as the sheet has it: a spell of the subclass it
          // has now stays out of this list, and stays out when the subclass is changed (the save drops it with the subclass).
          const granted = new Set(existing.grantedSpellKeys ?? []);
          const fromSubclass = new Set(this.sections().flatMap((sec) => sec.alwaysPrepared));
          this.grantedSpells.set(
            catalog.spells.filter((sp) => granted.has(sp.key) && !fromSubclass.has(sp.key)),
          );
        }
        if (existing.basic) {
          patchBasicForm(this.fb, this.basicForm, existing.basic);
        }
      })
      .catch((err: unknown) => {
        if (seq !== this.loadSeq) {
          return;
        }
        this.state.set({
          status: 'error',
          message: describeCharacterError(err),
          backLink: ['/campaigns', campaignId, 'characters', characterId],
          backLabel: 'Voltar para a ficha',
        });
      });
  }

  private patchFullForm(full: CharacterFormValue): void {
    this.fullForm.setValue({
      name: full.name,
      race: full.race,
      subrace: full.subrace,
      className: full.className,
      subclassName: full.subclassName,
      customSubclassName: full.customSubclassName,
      level: full.level,
      background: full.background,
      customBackgroundName: full.customBackgroundName,
      customBackgroundFeatureName: full.customBackgroundFeatureName,
      customBackgroundFeatureText: full.customBackgroundFeatureText,
      customBackgroundEquipment: full.customBackgroundEquipment,
      armor: full.armor,
      shield: full.shield,
      weaponKeys: full.weapons,
      equipmentText: full.equipmentText,
      languagesText: full.languagesText,
      toolProficienciesText: full.toolProficienciesText,
      experiencePoints: full.experiencePoints,
      challengeRating: full.challengeRating,
      xpValue: full.xpValue,
      portraitImageId: full.portraitImageId,
      alignment: full.alignment,
      customFeaturesText: full.customFeaturesText,
      hitPointsMethod: full.hitPointsMethod,
      abilities: full.abilities,
      extraAbilityBonuses: full.extraAbilityBonuses,
    });
    this.selectedSkills.set(new Set(full.skillProficiencies));
    this.customBackgroundSkills.set(new Set(full.customBackgroundSkills ?? []));
    this.customBackgroundProficiencies.set(full.customBackgroundProficiencies);
    this.extraClasses.set(full.extraClasses);
    this.expertiseSkills.set(new Set(full.expertiseSkillKeys));
    this.hitPointsRolls.set(full.hitPointsRolls);
    this.selectedCantrips.set(new Set(full.cantrips));
    this.selectedSpellsKnown.set(new Set(full.spellsKnown));
    this.selectedSpellsPrepared.set(new Set(full.spellsPrepared));
  }

  protected toggleSkill(key: string): void {
    const next = new Set(this.selectedSkills());
    if (next.has(key)) {
      next.delete(key);
      // Expertise requires proficiency first — drop it too so the
      // invariant (expertise ⊆ proficient) never breaks silently.
      if (this.expertiseSkills().has(key)) {
        const nextExpertise = new Set(this.expertiseSkills());
        nextExpertise.delete(key);
        this.expertiseSkills.set(nextExpertise);
      }
    } else {
      next.add(key);
    }
    this.selectedSkills.set(next);
  }

  /** Toggles expertise on a proficient skill (doubles its proficiency
   * bonus — Bard, Rogue). A no-op on a skill that is not proficient yet:
   * the checkbox is disabled for those in the template. */
  protected toggleExpertise(key: string): void {
    if (!this.selectedSkills().has(key)) {
      return;
    }
    const next = new Set(this.expertiseSkills());
    if (next.has(key)) {
      next.delete(key);
    } else {
      next.add(key);
    }
    this.expertiseSkills.set(next);
  }

  private toggleInSet(current: ReadonlySet<string>, key: string): Set<string> {
    const next = new Set(current);
    if (next.has(key)) {
      next.delete(key);
    } else {
      next.add(key);
    }
    return next;
  }

  protected toggleCantrip(key: string): void {
    this.selectedCantrips.set(this.toggleInSet(this.selectedCantrips(), key));
  }

  protected toggleSpellKnown(key: string): void {
    this.selectedSpellsKnown.set(this.toggleInSet(this.selectedSpellsKnown(), key));
  }

  protected toggleSpellPrepared(key: string): void {
    this.selectedSpellsPrepared.set(this.toggleInSet(this.selectedSpellsPrepared(), key));
  }

  /** Only the selected keys that are still in the (class-filtered) catalog
   * list — see `buildFullValue`'s comment. */
  private intersectWithAvailable(
    selected: ReadonlySet<string>,
    available: readonly { readonly key: string }[],
  ): string[] {
    const availableKeys = new Set(available.map((item) => item.key));
    const s = this.state();
    // A pick the catalog does not know at all is kept, never dropped quietly: the server judges it. Only a spell that is
    // known and not on the lists any more (the class changed) goes.
    const known = new Set(s.status === 'ready' ? s.catalog.spells.map((sp) => sp.key) : []);
    return Array.from(selected).filter((key) => availableKeys.has(key) || !known.has(key));
  }

  /** The "?" next to a spell: its description, as a bottom sheet on a phone
   * and a dialog from a tablet up. Focus goes back to the "?" on close. */
  protected describeSpell(spell: SpellOptionVm): void {
    const s = this.state();
    if (s.status !== 'ready') {
      return;
    }
    const data: SpellDetailsData = {
      namePt: spell.namePt,
      load: () => this.loadSpellDetails(s.campaignId, spell.key),
    };
    openSpellDetails(this.dialog, this.bottomSheet, data);
  }

  private loadSpellDetails(campaignId: string, key: string): Promise<SpellDetailsVm> {
    let pending = this.spellDetails.get(key);
    if (!pending) {
      pending = this.source.loadSpellDetails(campaignId, key);
      this.spellDetails.set(key, pending);
      pending.catch(() => this.spellDetails.delete(key));
    }
    return pending;
  }

  /** Toggles one of the custom background's two granted skills. Caps at 2
   * (`CustomBackground.skill_keys`: "at most 2"): a third click while two
   * are already chosen does nothing, so the player unchecks one first. */
  protected toggleCustomBackgroundSkill(key: string): void {
    const next = new Set(this.customBackgroundSkills());
    if (next.has(key)) {
      next.delete(key);
    } else if (next.size < 2) {
      next.add(key);
    }
    this.customBackgroundSkills.set(next);
  }

  /** Picks or drops one of the "Outro" background's two tools or languages; a third does nothing, like the skills. */
  protected setCustomProficiencies(keys: readonly string[]): void {
    this.customBackgroundProficiencies.set(keys.slice(0, 2));
  }

  protected readonly backgroundProficienciesCount = computed(() =>
    countLabel(
      this.customBackgroundProficiencies().length,
      'de 2 escolhida',
      'de 2 escolhidas',
      'Nenhuma escolhida',
    ),
  );

  private buildFullValue(): CharacterFormValue {
    const v = this.fullForm.getRawValue();
    const bgSkills = Array.from(this.customBackgroundSkills());
    return {
      name: v.name,
      race: v.race,
      subrace: v.subrace,
      className: v.className,
      subclassName: v.subclassName,
      customSubclassName: v.customSubclassName,
      level: v.level,
      background: v.background,
      customBackgroundName: v.customBackgroundName,
      // CustomBackground.skill_keys: "fewer than 2 shows an issue" — the
      // server reports that; the editor just sends what was chosen, even 0
      // or 1, rather than guessing or blocking submission over it.
      customBackgroundSkills:
        v.background === 'custom' && bgSkills.length === 2 ? [bgSkills[0], bgSkills[1]] : null,
      // The tools or languages, the feature and the equipment the same way: fewer than asked is sent as is,
      // and the server shows an issue on the sheet, never an error.
      customBackgroundProficiencies:
        v.background === 'custom' ? [...this.customBackgroundProficiencies()] : [],
      customBackgroundFeatureName: v.background === 'custom' ? v.customBackgroundFeatureName : '',
      customBackgroundFeatureText: v.background === 'custom' ? v.customBackgroundFeatureText : '',
      customBackgroundEquipment: v.background === 'custom' ? v.customBackgroundEquipment : '',
      // A blank block never gets here: `classProblems` stops the save first.
      extraClasses: this.extraClasses().filter((c) => c.classKey !== ''),
      skillProficiencies: Array.from(this.selectedSkills()),
      expertiseSkillKeys: Array.from(this.expertiseSkills()),
      abilities: v.abilities,
      extraAbilityBonuses: v.extraAbilityBonuses,
      hitPointsMethod: v.hitPointsMethod,
      // Only as many rolls as the current level needs — a roll left over
      // from a higher level typed earlier is dropped, not sent stale.
      hitPointsRolls: this.hitPointsRolls().slice(0, this.rollsNeeded()),
      isCaster: this.isCaster(),
      // Never send a stale pick: if the class or level changed after a
      // spell was chosen and it dropped off the (class-filtered) catalog
      // list, it never reaches the server — no typed key ever could get
      // here in the first place (integrator fix).
      cantrips: this.intersectWithAvailable(this.selectedCantrips(), this.availableCantrips()),
      spellsKnown: this.intersectWithAvailable(this.selectedSpellsKnown(), this.availableSpells()),
      spellsPrepared: this.intersectWithAvailable(
        this.selectedSpellsPrepared(),
        this.availableSpells(),
      ),
      armor: v.armor,
      shield: v.shield,
      weapons: v.weaponKeys,
      equipmentText: v.equipmentText,
      languagesText: v.languagesText,
      toolProficienciesText: v.toolProficienciesText,
      experiencePoints: v.experiencePoints,
      challengeRating: v.challengeRating,
      xpValue: v.xpValue,
      portraitImageId: v.portraitImageId,
      alignment: v.alignment,
      customFeaturesText: v.customFeaturesText,
    };
  }

  /** The full sheet's invalid fields right now, plus the placing that is
   * not finished (it has no control of its own: see `abilitiesIncomplete`). */
  /** What the class blocks still lack, in words ("Classe 2", "o nível total vai até 20"): not controls of the
   * form, so the page adds them to the invalid fields while they stand. */
  private readonly classProblems = computed<string[]>(() => {
    const problems: string[] = [];
    this.extraClasses().forEach((c, i) => {
      if (c.classKey === '') problems.push(`Classe ${i + 2}`);
      if (!Number.isInteger(c.level) || c.level < 1 || c.level > MAX_TOTAL_LEVEL)
        problems.push(`Nível da classe ${i + 2}`);
    });
    if (this.totalLevelProblem()) problems.push('o nível total vai até 20');
    return problems;
  });

  /** With "Rolado", the levels whose roll is empty or does not fit that level's die: the form keeps no control for
   * them, so the page adds them to the invalid fields while they stand. */
  private readonly rollProblems = computed<string[]>(() => {
    const dice = this.levelDice();
    const rolls = this.hitPointsRolls();
    if (this.selectedHitPointsMethod() !== 'rolled' || this.hitDie() === 0) {
      return [];
    }
    const problems: string[] = [];
    for (let i = 0; i < this.rollsNeeded(); i++) {
      if (validRoll(rolls[i], dice[i] || this.hitDie()) === null) {
        problems.push(`Dado de vida do nível ${i + 2}`);
      }
    }
    return problems;
  });

  private currentInvalidFullFields(): EditorField[] {
    const fields: EditorField[] = [
      ...invalidFields(this.fullForm, FULL_SHEET_FIELDS),
      ...this.classProblems().map((label) => ({ path: 'classes', label, step: 'basico' as const })),
      ...this.rollProblems().map((label) => ({
        path: 'hitPointsRolls',
        label,
        step: 'atributos' as const,
      })),
    ];
    if (!this.abilitiesIncomplete()) {
      return fields;
    }
    const problem = this.abilitiesProblem();
    return [
      ...fields,
      problem ? { ...UNPLACED_RESULTS_FIELD, label: problem } : UNPLACED_RESULTS_FIELD,
    ];
  }

  /** After a submit with an invalid field: opens the step of the first one
   * (and "Bônus manuais", if that is where it is), so the field and its
   * error message are on screen. */
  private openFirstInvalidStep(): void {
    const [first] = this.currentInvalidFullFields();
    if (!first?.step) {
      return;
    }
    if (first.path.startsWith('extraAbilityBonuses.')) {
      this.bonusesOpen.set(true);
    }
    const stepper = this.stepper;
    if (!stepper) {
      return;
    }
    const index = stepper.steps
      .toArray()
      .findIndex((step) => step.label === EDITOR_STEP_LABELS[first.step!]);
    if (index >= 0 && index !== stepper.selectedIndex) {
      stepper.goTo(index);
    }
  }

  protected async submit(): Promise<void> {
    const s = this.state();
    if (s.status !== 'ready') {
      return;
    }
    const isBasic = !isFullSheetKind(s.kind);
    const form = isBasic ? this.basicForm : this.fullForm;
    if (
      form.invalid ||
      (!isBasic &&
        (this.abilitiesIncomplete() ||
          this.classProblems().length > 0 ||
          this.rollProblems().length > 0))
    ) {
      form.markAllAsTouched();
      this.saveState.set({ status: 'idle' });
      this.showErrors.set(true);
      if (!isBasic) {
        this.openFirstInvalidStep();
      }
      return;
    }

    this.saveState.set({ status: 'saving' });
    // The save finishes on the server whatever the person does meanwhile; only the navigation depends on still being here.
    const seq = this.loadSeq;
    const stillHere = (): boolean => !this.destroyed && seq === this.loadSeq;
    try {
      if (s.mode === 'create') {
        const input = {
          campaignId: s.campaignId,
          kind: s.kind,
          full: isBasic ? null : this.buildFullValue(),
          basic: isBasic ? basicFormToValue(this.basicForm) : null,
          ...(this.abilityTable() && !isBasic ? { abilityMethod: this.abilityMethod() } : {}),
        };
        // A retry of the same form (a lost answer, a second tap) sends the same key and makes one character.
        const res = await this.source.createCharacter({
          ...input,
          idempotencyKey: this.createKey.keyFor(input),
        });
        this.createKey.renew();
        if (!stillHere()) {
          return;
        }
        await this.router.navigate(['/campaigns', s.campaignId, 'characters', res.characterId]);
      } else if (s.characterId) {
        await this.source.updateCharacter({
          campaignId: s.campaignId,
          characterId: s.characterId,
          revision: s.revision,
          name: isBasic ? this.basicForm.getRawValue().name : this.fullForm.getRawValue().name,
          full: isBasic ? null : this.buildFullValue(),
          basic: isBasic ? basicFormToValue(this.basicForm) : null,
        });
        if (!stillHere()) {
          return;
        }
        await this.router.navigate(['/campaigns', s.campaignId, 'characters', s.characterId]);
      }
      this.saveState.set({ status: 'idle' });
    } catch (err) {
      if (!stillHere()) {
        return;
      }
      // A refusal that names a key says it by name; one that points at a class block marks that block.
      const field = invalidFieldPath(err);
      const block = field ? /^full\.classes\[(\d+)\]/.exec(field) : null;
      this.serverClassProblem.set(block ? Number(block[1]) : null);
      // An option the master switched off after the lists were read is an error on its field (E10-01 state 5). The name is
      // taken before the lists are read again, because the entry may be gone from them.
      const refused = switchedOffKey(err);
      if (refused !== null && this.markSwitchedOff(refused)) {
        void this.refreshCatalog();
        return;
      }
      this.saveState.set({
        status: 'error',
        message: describeCharacterError(err, (key) => this.nameOfKey(key)),
      });
    }
  }

  /**
   * The server refused a key the master switched off: its field gets the error (`switchedOff`, so it is invalid, red and
   * `aria-invalid`), the step is marked "(com erro)", the editor opens it and the focus goes to the field. Returns false when
   * no field holds the key (a spell, say): the notice above the buttons says it by name instead.
   */
  private markSwitchedOff(key: string): boolean {
    const control = offControlOf(key, this.fullForm.getRawValue());
    if (!control) {
      return false;
    }
    const ref = contentRef(key, (k) => this.nameOfKey(k));
    this.fullForm.controls[control].setErrors({
      switchedOff: characterBlockedMessage('switched_off_content', { ...ref, step: '' }),
    });
    this.fullForm.controls[control].markAsTouched();
    this.showErrors.set(true);
    this.saveState.set({ status: 'idle' });
    this.openFirstInvalidStep();
    afterNextRender(
      () =>
        this.host.nativeElement
          .querySelector<HTMLElement>(`[formcontrolname="${control}"]`)
          ?.focus(),
      { injector: this.injector },
    );
    return true;
  }

  /** The sentence of a switched-off option on its field, for the template. */
  protected switchedOff(
    control: 'className' | 'race' | 'subrace' | 'subclassName' | 'background',
  ): string {
    return (this.fullForm.controls[control].getError('switchedOff') as string | null) ?? '';
  }

  /** `content_changed` (RN-23): the lists are read again with this person's role; what was typed stays. Resolves when done. */
  private async refreshCatalog(): Promise<void> {
    const s = this.state();
    if (s.status !== 'ready') {
      return;
    }
    // The newest read is the one that counts, and an answer for a route the person left does not count at all.
    const seq = ++this.catalogSeq;
    const route = this.loadSeq;
    try {
      const catalog = await this.source.loadCatalog(s.campaignId, s.characterId ?? undefined);
      if (seq !== this.catalogSeq || route !== this.loadSeq) {
        return;
      }
      const now = this.state();
      if (now.status === 'ready' && catalogChanged(now.catalog, catalog)) {
        this.state.set({ ...now, catalog });
        if (offersChanged(now.catalog, catalog)) {
          this.contentNote.set(
            'O mestre mudou as opções da mesa. As listas foram atualizadas: confira o que você escolheu antes de salvar.',
          );
        }
      }
    } catch {
      // Keep the lists on screen: the next change reads again.
    }
  }
}
