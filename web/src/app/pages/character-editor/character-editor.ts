import {
  Component,
  DestroyRef,
  ElementRef,
  Injector,
  ViewChild,
  computed,
  effect,
  inject,
  signal,
  untracked,
} from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { FormBuilder, ReactiveFormsModule } from '@angular/forms';
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
import { ActivatedRoute, Router, RouterLink } from '@angular/router';

import { ContentWatcher } from '../../core/content/content-watcher';
import { LiveSessionSourceLive } from '../../core/play/live-session-source.live';
import { isFullSheetKind } from '../../core/characters/characters.types';
import { ChoiceGroups } from '../../shared/choice-groups/choice-groups';
import { FictionNotice } from '../../shared/fiction-notice/fiction-notice';
import { OpenDown } from '../../shared/form-fields/open-down';
import { TableMark } from '../../shared/table-mark/table-mark';
import { AbilityFields } from './ability-fields/ability-fields';
import { AbilityScores } from './ability-scores/ability-scores';
import { TableAbilityScores } from './table-ability-scores/table-ability-scores';
import {
  ALIGNMENT_LABELS,
  AbilityMethodKey,
  AbilityTableVm,
  AlignmentKey,
  CharacterEditorSource,
  CharacterForEdit,
  SpellOptionVm,
} from './character-editor.types';
import { ClassBlockFields } from './class-block/class-block';
import { GrantedSpells } from './granted-spells/granted-spells';
import type { ClassBlock } from './class-blocks';
import { subclassWaits, weaponDetail } from './catalog-views';
import { CUTTING_WORDS_OPTIONS } from './editor-options';
import { EditorLoader } from './editor-loader';
import { type PageState, startingXp } from './editor-state';
import { EditorStepper } from './editor-stepper/editor-stepper';
import { HitPointsRolls } from './hit-points-rolls/hit-points-rolls';
import { EDITOR_STEP_LABELS } from './editor-labels';
import { createFullForm, patchFirstClass } from './full-form';
import { EditorCatalog } from './editor-catalog';
import { EditorDraft } from './editor-draft';
import { EditorPicks } from './editor-picks';
import { sentencesOf } from './pick-refusals';
import { basicFormToValue, createBasicForm } from './npc-short-form/basic-form';
import { SaveFlow } from './save-flow';
import { filterByName } from './spell-views';
import { SpellPicks } from './spell-picks';
import { DefeatXp } from './defeat-xp/defeat-xp';
import { FeatsField } from './feats-field/feats-field';
import { PortraitField } from './portrait-field/portrait-field';
import { NpcShortForm } from './npc-short-form/npc-short-form';
import { SkillPicker } from './skill-picker/skill-picker';
import { SpellPicker } from './spell-picker/spell-picker';

import { EditorServerDraft } from './editor-server-draft';
import { EditorSubmit } from './editor-submit';
import type { EditorCore } from './editor-core';
import { EditorSpellDetails } from './editor-spell-details';
/**
 * The character editor (MR-003, MR-005, MR-006): a stepper for a full
 * sheet (player, enemy, boss) — Básico, Habilidades, Perícias, Escolhas, Magias
 * (only for a caster class), Equipamento, `EditorStepper` over the CDK stepper —
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
 * The page owns the two forms, the page state and the signals the plain
 * classes share, and builds the plain classes that hold the rest:
 * `EditorPicks` and `SpellPicks` (what is picked beside the form's controls),
 * `EditorCatalog` (the lists and the views over catalog and form),
 * `EditorDraft` (the draft and what is wrong with it), `EditorServerDraft`
 * (what the server derives from the draft), `SaveFlow` and `EditorSubmit`
 * (the save), `EditorLoader` (opening a route) and `EditorSpellDetails`.
 * The template binds those members directly (`views.blocks()`,
 * `picks.toggleSkill($event)`, `server.pendingSteps()`), so a member is
 * documented on the class that owns it; the page keeps only what needs the
 * component itself: the route, the step and choice-group view children, the
 * subclass and XP effects and the block and class-change handlers.
 *
 * Race, subrace, class, subclass, background, skills, armor, weapons,
 * cantrips and spells are picked from `RulesCatalogVm`
 * (`ContentService.ListContent`): `namePt` shown, `key` sent, never typed as
 * free text, since a typed name almost never matches the real key.
 */
@Component({
  selector: 'app-character-editor',
  imports: [
    ChoiceGroups,
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
    OpenDown,
    DefeatXp,
    FeatsField,
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
  private readonly route = inject(ActivatedRoute);
  private readonly router = inject(Router);
  private readonly destroyRef = inject(DestroyRef);
  private readonly fb = inject(FormBuilder);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly injector = inject(Injector);
  // The session's stream says when the table's content changes (10.1d, RN-23): one stream, one read per hint, with the debounce.
  private readonly watcher = inject(ContentWatcher);

  @ViewChild(EditorStepper) private stepper?: EditorStepper;
  @ViewChild(ChoiceGroups) private choiceGroups?: ChoiceGroups;

  protected readonly state = signal<PageState>({ status: 'loading', title: 'Ficha' });
  protected readonly fullForm = createFullForm(this.fb);
  protected readonly basicForm = createBasicForm(this.fb);

  /** The table's ways of making scores, when a player makes a new sheet (RN-24); `null` is the free editor of the master's NPCs and of an edit. */
  protected readonly abilityTable = signal<AbilityTableVm | null>(null);
  /** The way the player chose; the server checks the scores against it. */
  protected readonly abilityMethod = signal<AbilityMethodKey>('typed');
  /** A player editing their own draft: the way the server recorded the scores were made, which the step keeps (RN-24). */
  protected readonly lockedOrigin = signal<NonNullable<CharacterForEdit['abilityOrigin']> | null>(
    null,
  );
  /** "Bônus manuais" starts closed; a submit with an invalid bonus opens it. */
  protected readonly bonusesOpen = signal(false);
  /** What the page says after the table's content changed under the person ("" when nothing did). */
  protected readonly contentNote = signal('');
  private readonly campaignIdSignal = signal('');

  /** What the person picked beside the form's controls (`EditorPicks`) and the spells of the "Magias" step. */
  protected readonly picks: EditorPicks = new EditorPicks(() => this.views.grantedSkills());
  protected readonly spellPicks: SpellPicks = new SpellPicks({
    sections: () => this.views.sections(),
  });
  /** What every plain class below reads: the state, the two forms and the picks. */
  private readonly core: EditorCore = {
    state: this.state,
    fullForm: this.fullForm,
    basicForm: this.basicForm,
    picks: this.picks,
    spellPicks: this.spellPicks,
  };
  /** What the catalog and the form say together: the lists, the blocks, the spell pickers (`EditorCatalog`). */
  protected readonly views: EditorCatalog = new EditorCatalog({
    ...this.core,
    derived: () => this.server.hitPoints.answer(),
    choices: () => this.server.choices.answer(),
  });
  /** The draft and what is wrong with it (`EditorDraft`). */
  protected readonly draft = new EditorDraft({ ...this.core, catalog: this.views });
  /** What the server says about the draft: its hit points and spell numbers, and its choices (`EditorServerDraft`). */
  protected readonly server = new EditorServerDraft({
    ...this.core,
    source: this.source,
    views: this.views,
    abilityTable: this.abilityTable,
    build: () => this.draft.build(),
  });
  /** The save: the request, the navigation after it and what a refusal does (`SaveFlow`). */
  protected readonly saving: SaveFlow = new SaveFlow({
    source: this.source,
    navigate: (commands) => this.router.navigate(commands),
    full: () => this.draft.build(),
    basic: () => basicFormToValue(this.basicForm),
    name: (isBasic) =>
      isBasic ? this.basicForm.getRawValue().name : this.fullForm.getRawValue().name,
    abilityMethod: () => (this.abilityTable() ? this.abilityMethod() : null),
    stillHere: () => this.loader.stillHere(),
    nameOfKey: (key) => this.views.nameOfKey(key),
    markSwitchedOff: (key) => this.submitting.markSwitchedOff(key),
    openStep: (step) => this.submitting.openStep(step),
    refreshCatalog: () => this.refreshCatalog(),
  });
  /** The class block a refusal points at (`full.classes[i]`), so the block says so. */
  /** The save button and what it does before the request (`EditorSubmit`). */
  protected readonly submitting: EditorSubmit = new EditorSubmit({
    ...this.core,
    draft: this.draft,
    views: this.views,
    saving: this.saving,
    choicesBlocked: this.server.choicesBlocked,
    bonusesOpen: this.bonusesOpen,
    stepper: () => this.stepper,
    choiceGroups: () => this.choiceGroups,
    element: this.host.nativeElement,
    injector: this.injector,
  });
  /** The "?" next to a spell (`EditorSpellDetails`). */
  protected readonly spellDetails = new EditorSpellDetails({
    source: this.source,
    state: this.state,
    dialog: inject(MatDialog),
    bottomSheet: inject(MatBottomSheet),
  });

  /**
   * The pick the server cannot place, with its sentence: the save's refusal, else what the server says of the draft on
   * screen. The pickers show it on the field of the pick, for as long as the draft holds the pick.
   */
  private readonly pickRefusal = computed(
    () => this.saving.pickRefusal() ?? this.server.pickFit.answer(),
  );
  protected readonly skillSentences = computed(() =>
    sentencesOf(this.pickRefusal(), 'skill', (key) => this.picks.selectedSkills().has(key)),
  );
  protected readonly expertiseSentences = computed(() =>
    sentencesOf(this.pickRefusal(), 'expertise', (key) => this.picks.expertiseSkills().has(key)),
  );
  protected readonly toolSentences = computed(() =>
    sentencesOf(this.pickRefusal(), 'tool', (key) => this.picks.featureChoiceKeys().includes(key)),
  );

  protected readonly cuttingWordsOptions = CUTTING_WORDS_OPTIONS;
  protected readonly isFullSheetKind = isFullSheetKind;
  protected readonly weaponDetail = weaponDetail;
  protected readonly stepLabels = EDITOR_STEP_LABELS;
  protected readonly alignmentKeys = Object.keys(ALIGNMENT_LABELS) as AlignmentKey[];
  protected readonly alignmentLabel = (key: AlignmentKey) => ALIGNMENT_LABELS[key];

  /** What the table leaves of the hit points of a new sheet: both ways, only "Rolado" or only "Média". */
  protected readonly hpRule = computed<'player_chooses' | 'roll' | 'average'>(() => {
    const s = this.state();
    return s.status === 'ready' && s.mode === 'create'
      ? (this.abilityTable()?.hitPoints ?? 'player_chooses')
      : 'player_chooses';
  });

  /** What the page reads from the server to open, what it does with the answer, and the routes it drops the answers of (`EditorLoader`). */
  private readonly loader = new EditorLoader({
    ...this.core,
    source: this.source,
    fb: this.fb,
    views: this.views,
    abilityTable: this.abilityTable,
    abilityMethod: this.abilityMethod,
    lockedOrigin: this.lockedOrigin,
    contentNote: this.contentNote,
    campaignId: this.campaignIdSignal,
    reservedRoute: () => this.isReservedRoute(),
    forget: () => {
      this.server.choices.stop();
      this.saving.reset();
    },
  });

  constructor() {
    // Only a player's editor follows the hint: the master writes the content himself, and an open stream would keep the
    // page from ever being quiet (every NPC editor of a live campaign would hold one).
    this.watcher.whileLive(
      () => (this.views.master() ? '' : this.campaignIdSignal()),
      () => void this.refreshCatalog(),
    );
    // The subclass of a one-class sheet waits for the level the class chooses it at (as in a block), unless one is chosen.
    effect(() => {
      const waits = subclassWaits(
        this.views.selectedClass(),
        this.views.selectedLevel(),
        this.views.selectedSubclassKey(),
      );
      const control = this.fullForm.controls.subclassName;
      if (waits && control.enabled) {
        control.disable({ emitEvent: false });
      } else if (!waits && control.disabled) {
        control.enable({ emitEvent: false });
      }
    });
    // A new player's sheet made above level 1 starts at its level's XP in a campaign that levels by XP, until the
    // person types in the field; a milestone campaign keeps the 0.
    effect(() => {
      const xp = startingXp(this.state(), this.views.totalLevelValue());
      const control = this.fullForm.controls.experiencePoints;
      if (xp !== null && !control.dirty) {
        untracked(() => control.setValue(xp));
      }
    });
    // The save's refusal of a pick goes when the skills, the expertise or the classes change.
    effect(() => {
      this.picks.selectedSkills();
      this.picks.expertiseSkills();
      this.picks.featureChoiceKeys();
      this.picks.extraClasses();
      this.views.selectedClass();
      untracked(() => this.saving.clearPickProblem());
    });
    this.destroyRef.onDestroy(() => {
      this.loader.destroy();
      this.server.stop();
    });
    this.route.paramMap.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((params) => {
      this.loader.resolve(params);
    });
  }

  /**
   * Whether the route is `/campaigns/:id/reserved/new`: its `data` says so. The router hands a child route the data of its
   * parent (an empty path under a lazy parent), so the snapshot is where it is read; a route a test builds by hand may have none.
   */
  private isReservedRoute(): boolean {
    const snapshot = (this.route as Partial<ActivatedRoute>).snapshot;
    return snapshot?.data['reserved'] === true;
  }

  /** The table's rules for the scores changed under the person (the server refused the way of rolling the step offered):
   * they are read again, so the step offers what is allowed now. */
  protected rereadAbilityTable(): Promise<void> {
    return this.loader.rereadAbilityTable();
  }

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
      patchFirstClass(this.fullForm, change);
      return;
    }
    this.picks.changeExtraClass(index - 1, change);
  }

  /** The search of an extra list ("0:patron"), and what it narrows. */
  protected readonly filterSpells = (spells: readonly SpellOptionVm[], id: string) =>
    filterByName(spells, this.spellPicks.filterOf(id));
  protected readonly setSpellFilter = (
    section: number,
    list: 'cantrips' | 'known' | 'prepared',
    value: string,
  ) => this.spellPicks.setFilter(`${section}:${list}`, value);

  /** `content_changed` (RN-23): the lists are read again with this person's role; what was typed stays. Resolves when done. */
  private refreshCatalog(): Promise<void> {
    return this.loader.refreshCatalog();
  }
}
