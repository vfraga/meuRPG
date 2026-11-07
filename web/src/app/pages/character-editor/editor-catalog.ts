import { computed } from '@angular/core';
import { takeUntilDestroyed, toSignal } from '@angular/core/rxjs-interop';

import type { CharacterPreviewVm, ChoicesPreviewVm } from './character-editor.types';
import {
  catalogNameOf,
  chosenSkillsOf,
  choiceBonusesHintOf,
  sizeChoicesOf,
  choicesSubtitleOf,
  finalConstitutionOf,
  grantedSkillsOf,
  sectionHeadingOf,
  skillNoteOf,
  subclassHintOf,
} from './catalog-views';
import {
  type CasterSection,
  type ClassBlock,
  MAX_TOTAL_LEVEL,
  blocksOf,
  casterSections,
  hitDiceAfterFirst,
  offered,
  sectionName,
  totalLevel,
  unlisted,
} from './class-blocks';
import type { EditorCore } from './editor-core';
import {
  actionsNoteOf,
  cancelLinkOf,
  createLabelOf,
  leadPartsOf,
  pageTitleOf,
  phoneNoteOf,
} from './editor-state';
import { countLabel } from './editor-labels';
import { formatXp } from '../../core/format/text';
import {
  availableCantripsOf,
  availableSpellsOf,
  buildSectionView,
  sectionExtrasOf,
} from './spell-views';

/** What `EditorCatalog` reads from the page. */
export interface EditorCatalogHost extends EditorCore {
  /** What the server derives for the draft (hit points and the spell numbers), `null` while it has no answer. */
  readonly derived: () => CharacterPreviewVm | null;
  /** The choices the server reads in the draft, `null` while it has no answer. */
  readonly choices: () => ChoicesPreviewVm | null;
}

/**
 * What the catalog and the form say together, as signals: the entries chosen in the closed selects, what each list
 * offers, the class blocks and the total level, the spell lists a sheet reads from and what each picker shows, the
 * skills the sheet is given. A plain class the editor builds; nothing here is a rule, every table it reads is the
 * server's catalog.
 */
export class EditorCatalog {
  /** The lists of the catalog while the page is open on a sheet, else `null`. */
  readonly catalog = computed(() => {
    const s = this.host.state();
    return s.status === 'ready' ? s.catalog : null;
  });
  /** Any change of the form's fields; the signals the draft is made of are read where it is built. */
  readonly formChanges;
  readonly selectedRaceKey;
  readonly selectedClassKey;
  readonly selectedBackground;
  readonly selectedLevel;
  readonly selectedSubclassKey;
  readonly selectedCustomSubclass;
  readonly selectedHitPointsMethod;
  readonly selectedSubraceKey;
  readonly baseScores;
  readonly bonusesValue;

  /** The caller is the master: the one who is offered what is switched off for the players. */
  readonly master = computed(() => {
    const s = this.host.state();
    return s.status === 'ready' && s.catalog.viewerIsMaster;
  });

  /** Every class of the sheet, in the order of the blocks: the first one is the form's own fields. */
  readonly blocks = computed<ClassBlock[]>(() =>
    blocksOf(
      {
        classKey: this.selectedClassKey(),
        level: this.selectedLevel(),
        subclassKey: this.selectedSubclassKey(),
        customSubclassName: this.selectedCustomSubclass(),
      },
      this.host.picks.extraClasses(),
    ),
  );
  /** More than one class: the Básico step shows one block per class and the total level. */
  readonly multiclass = computed(() => this.host.picks.extraClasses().length > 0);
  readonly totalLevelValue = computed(() => totalLevel(this.blocks()));
  /** The level the whole sheet has, up to 20, or why it does not fit. */
  readonly totalLevelProblem = computed(() =>
    this.totalLevelValue() > MAX_TOTAL_LEVEL ? `O nível total vai até ${MAX_TOTAL_LEVEL}.` : '',
  );
  /** How many rolls "Dados de Vida" needs: one per level after the first, of all the classes together. */
  readonly rollsNeeded = computed(() => Math.max(0, this.totalLevelValue() - 1));
  /** The die of each of those levels, which differs between the classes of a multiclass. */
  readonly levelDice = computed(() => {
    const catalog = this.catalog();
    return catalog ? hitDiceAfterFirst(catalog, this.blocks()) : [];
  });

  readonly availableSubraces = computed(
    () => this.catalog()?.races.find((r) => r.key === this.selectedRaceKey())?.subraces ?? [],
  );
  readonly selectedClass = computed(() =>
    this.catalog()?.classes.find((c) => c.key === this.selectedClassKey()),
  );
  /** The entries chosen in the closed selects, for the "Da mesa" tag beside the name (a select shows only text). */
  readonly selectedRace = computed(() =>
    this.catalog()?.races.find((r) => r.key === this.selectedRaceKey()),
  );
  readonly selectedSubrace = computed(() =>
    this.availableSubraces().find((r) => r.key === this.selectedSubraceKey()),
  );
  readonly selectedSubclass = computed(() =>
    this.selectedClass()?.subclasses.find((c) => c.key === this.selectedSubclassKey()),
  );
  readonly selectedBackgroundVm = computed(() =>
    this.catalog()?.backgrounds.find((b) => b.key === this.selectedBackground()),
  );
  /** Faces of the chosen class's hit die, or 0 before a class is chosen. */
  readonly hitDie = computed(() => this.selectedClass()?.hitDie ?? 0);

  /** What each list offers as a NEW choice: never a retired entry, unless it is the value the form already has. */
  readonly raceOptions = computed(() =>
    offered(this.catalog()?.races ?? [], [this.selectedRaceKey()], this.master()),
  );
  readonly subraceOptions = computed(() =>
    offered(this.availableSubraces(), [this.selectedSubraceKey()], this.master()),
  );
  readonly classOptions = computed(() =>
    offered(this.catalog()?.classes ?? [], [this.selectedClassKey()], this.master()),
  );
  readonly subclassOptions = computed(() =>
    offered(this.selectedClass()?.subclasses ?? [], [this.selectedSubclassKey()], this.master()),
  );
  readonly backgroundOptions = computed(() =>
    offered(this.catalog()?.backgrounds ?? [], [this.selectedBackground()], this.master()),
  );
  /** A value the catalog does not list at all (a key from an old sheet): the select says so instead of staying blank. */
  readonly raceUnlisted = computed(() =>
    unlisted(this.catalog()?.races ?? [], this.selectedRaceKey()),
  );
  readonly classUnlisted = computed(() =>
    unlisted(this.catalog()?.classes ?? [], this.selectedClassKey()),
  );
  readonly backgroundUnlisted = computed(
    () =>
      this.selectedBackground() !== 'custom' &&
      unlisted(this.catalog()?.backgrounds ?? [], this.selectedBackground()),
  );

  /** The Constitution the rolled hit points use (the same sum the server makes; this one only feeds the preview). */
  readonly finalConstitution = computed(() =>
    finalConstitutionOf(
      this.baseScores().con,
      this.selectedRace(),
      this.selectedSubrace(),
      this.bonusesValue().con,
    ),
  );

  /** The spell lists the sheet reads from: one section per class that casts, or per subclass that
   * casts (a third caster), from its level. Empty for a sheet that casts nothing: no "Magias" step. */
  readonly sections = computed<CasterSection[]>(() => {
    const catalog = this.catalog();
    return catalog ? casterSections(catalog, this.blocks()) : [];
  });
  readonly isCaster = computed(() => this.sections().length > 0);
  readonly sectionLabel = sectionName;

  /** Every cantrip and spell of the sheet's lists, for "the chosen line" and for what a save may send. */
  readonly availableCantrips = computed(() => {
    const catalog = this.catalog();
    return catalog
      ? availableCantripsOf(catalog, this.sections(), this.host.spellPicks.cantrips())
      : [];
  });
  readonly availableSpells = computed(() => {
    const catalog = this.catalog();
    return catalog ? availableSpellsOf(catalog, this.sections(), this.sectionExtras()) : [];
  });
  /** For each section, the spells outside its list that `PreviewChoices` lets it pick. */
  private readonly sectionExtras = computed(() => {
    const catalog = this.catalog();
    if (!catalog) {
      return [];
    }
    const { known, prepared } = this.host.spellPicks;
    return sectionExtrasOf(
      catalog,
      this.sections(),
      this.host.choices()?.spellSources ?? [],
      new Set([...known(), ...prepared()]),
      this.master(),
    );
  });
  /** What the server says each casting class of the draft knows and prepares at its level (class table and the
   * SRD's formulas); empty while it has no answer. */
  private readonly spellLimits = computed(
    () => new Map((this.host.derived()?.spellcasting ?? []).map((l) => [l.classKey, l])),
  );

  /** What each section's pickers list now (`buildSectionView`). */
  readonly sectionViews = computed(() => {
    const catalog = this.catalog();
    if (!catalog) {
      return [];
    }
    const sections = this.sections();
    const { cantrips, known, prepared, filters } = this.host.spellPicks;
    return sections.map((section) =>
      buildSectionView({
        catalog,
        sections,
        section,
        cantrips: cantrips(),
        known: known(),
        prepared: prepared(),
        master: this.master(),
        filters: filters(),
        extras: this.sectionExtras(),
        // The draft's own numbers when the server has answered; else the saved sheet's prepared number (an edit).
        serverLimits: this.spellLimits().get(section.classKey),
        savedPreparedMax: this.host.picks.preparedMaxByClass()[section.classKey],
      }),
    );
  });

  /** The skills the race, the subrace and the background give, with where each comes from (`grantedSkillsOf`). */
  readonly grantedSkills = computed<ReadonlyMap<string, string>>(() =>
    grantedSkillsOf(
      this.selectedRace(),
      this.selectedSubrace(),
      this.selectedBackgroundVm(),
      this.selectedBackground() === 'custom',
      Array.from(this.host.picks.customBackgroundSkills()),
    ),
  );
  /** The skills the player picked, without the ones already given. */
  readonly chosenSkills = computed(() =>
    chosenSkillsOf(this.host.picks.selectedSkills(), this.grantedSkills()),
  );
  /** What the first class asks of the skills and the saving throws, from the server's entry for it. */
  readonly skillNote = computed(() => skillNoteOf(this.selectedClass()));
  /** "Ver em Magias", for a spell the lists leave out. */
  readonly magiasLink = computed(() => {
    const s = this.host.state();
    return s.status === 'ready' ? ['/campaigns', s.campaignId, 'spells'] : [];
  });
  /** The race's "+2 e +1 à sua escolha": where the player puts them (the server's numbers, from the catalog). */
  readonly choiceBonusesHint = computed(() =>
    choiceBonusesHintOf(this.selectedRace(), this.selectedSubrace()),
  );
  /** The sizes the race lets the player pick from: empty unless there is a real choice. */
  readonly sizeChoices = computed(() => sizeChoicesOf(this.selectedRace()));
  /** A table background's equipment, as the master wrote it. */
  readonly backgroundEquipment = computed(() => this.selectedBackgroundVm()?.equipmentPt ?? '');
  readonly toolOptions = computed(() => this.toolsAndLanguages('tool'));
  readonly languageOptions = computed(() => this.toolsAndLanguages('language'));
  /** The hint under "Subclasse" while the level is below the one where the class chooses it; empty once the level
   * reaches it. */
  readonly subclassHint = computed(() =>
    subclassHintOf(this.selectedClass(), this.selectedLevel()),
  );
  /** The line next to the counter of the step: "Tharn · Guerreiro 1 · Draconato". */
  readonly choicesSubtitle = computed(() => {
    this.formChanges();
    if (this.host.state().status !== 'ready') {
      return '';
    }
    const v = this.host.fullForm.getRawValue();
    return choicesSubtitleOf(
      v.name,
      this.selectedClass(),
      v.level,
      this.selectedRace(),
      this.selectedSubrace(),
    );
  });

  /** The line under the title in two parts: a phone shows the first only. */
  readonly leadParts = computed(() => leadPartsOf(this.host.state()));
  readonly pageTitle = computed(() => pageTitleOf(this.host.state()));
  /** The label of the create button: "Salvar como reservado" for a character made for a player to claim. */
  readonly createLabel = computed(() => createLabelOf(this.host.state()));
  /** Whether the character being made or edited is reserved: the page tells the master so. */
  readonly reserved = computed(() => {
    const s = this.host.state();
    return s.status === 'ready' && s.reserved;
  });
  /** Cancel goes back where the person came from: the sheet being edited, or the campaign. */
  readonly cancelLink = computed(() => cancelLinkOf(this.host.state()));
  /** What the buttons do, in one line next to them. */
  readonly actionsNote = computed(() => actionsNoteOf(this.host.state()));
  /** The same note cut to one line for a phone. */
  readonly phoneNote = computed(() => phoneNoteOf(this.host.state()));
  readonly backgroundSkillsCount = computed(() =>
    countLabel(
      this.host.picks.customBackgroundSkills().size,
      'de 2 escolhida',
      'de 2 escolhidas',
      'Nenhuma escolhida',
    ),
  );
  readonly backgroundProficienciesCount = computed(() =>
    countLabel(
      this.host.picks.customBackgroundProficiencies().length,
      'de 2 escolhida',
      'de 2 escolhidas',
      'Nenhuma escolhida',
    ),
  );
  /** The locked sheet's XP, to read ("2.716 XP"): only the master's awards change it. */
  readonly experienceLabel;

  constructor(private readonly host: EditorCatalogHost) {
    const controls = host.fullForm.controls;
    this.formChanges = toSignal(host.fullForm.valueChanges);
    this.selectedRaceKey = toSignal(controls.race.valueChanges, { initialValue: '' });
    // A size picked for one race is not the next race's: the picker starts over on the race's own.
    controls.race.valueChanges.pipe(takeUntilDestroyed()).subscribe((race) => {
      const sizes = this.catalog()?.races.find((r) => r.key === race)?.sizes ?? [];
      if (controls.raceSize.value !== '' && !sizes.includes(controls.raceSize.value)) {
        controls.raceSize.setValue('');
      }
    });
    this.selectedClassKey = toSignal(controls.className.valueChanges, { initialValue: '' });
    this.selectedBackground = toSignal(controls.background.valueChanges, { initialValue: '' });
    this.selectedLevel = toSignal(controls.level.valueChanges, { initialValue: 1 });
    this.selectedSubclassKey = toSignal(controls.subclassName.valueChanges, { initialValue: '' });
    this.selectedCustomSubclass = toSignal(controls.customSubclassName.valueChanges, {
      initialValue: '',
    });
    this.selectedHitPointsMethod = toSignal(controls.hitPointsMethod.valueChanges, {
      initialValue: 'average' as const,
    });
    this.selectedSubraceKey = toSignal(controls.subrace.valueChanges, { initialValue: '' });
    this.baseScores = toSignal(controls.abilities.valueChanges, {
      initialValue: controls.abilities.getRawValue(),
    });
    const experienceXp = toSignal(controls.experiencePoints.valueChanges, { initialValue: 0 });
    this.experienceLabel = computed(() => formatXp(experienceXp()));
    this.bonusesValue = toSignal(controls.extraAbilityBonuses.valueChanges, {
      initialValue: controls.extraAbilityBonuses.getRawValue(),
    });
  }

  /** The Portuguese name of any entry of the catalog, for a message that names one (never the key itself). */
  nameOfKey(key: string): string | undefined {
    return catalogNameOf(this.catalog() ?? undefined, key);
  }

  /** The heading of a section when there is more than one: "Clérigo · 1º nível". */
  sectionHeading(section: CasterSection): string {
    return sectionHeadingOf(section);
  }

  /** The classes the other blocks already use: a class cannot be taken twice (the SRD's multiclass), so they are not offered here. */
  otherClassKeys(index: number): readonly string[] {
    return this.blocks()
      .filter((_, i) => i !== index)
      .map((b) => b.classKey)
      .filter((k) => k !== '');
  }

  private toolsAndLanguages(kind: 'tool' | 'language') {
    return this.catalog()?.toolsAndLanguages.filter((t) => t.kind === kind) ?? [];
  }
}
