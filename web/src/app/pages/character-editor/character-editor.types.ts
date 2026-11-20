import {
  AbilityKey,
  CharacterBlockedReason,
  CharacterKind,
} from '../../core/characters/characters.types';
import { DamageTypeKey } from '../../core/characters/character-labels';
import type {
  ChoiceGroup,
  ChoiceSpellSource,
  PickAllowed,
} from '../../../gen/meurpg/characters/v1/characters_pb';
import type { SpellDetailsVm } from '../../shared/spell-details/spell-details.types';

/**
 * The view-model and port `CharacterEditor` needs. Phase 2 maps
 * `rules.v1.ContentService.ListContent`'s response onto `RulesCatalogVm`,
 * and `characters.v1.CharacterService`'s `Create/UpdateCharacter` onto
 * `CharacterEditorSource` — nothing below imports from `../../../gen/...`,
 * so that mapping is the only thing phase 2 adds.
 */

/** How the editor was opened. `'create'` covers both a player creating
 * their own character and a master creating an NPC — `kind` (below) tells
 * them apart; `'edit'` loads the existing sheet first. */
export type CharacterEditorMode = 'create' | 'edit';

export interface AbilityScoresInput {
  str: number;
  dex: number;
  con: number;
  int: number;
  wis: number;
  cha: number;
}

/** `Alignment` (characters.proto), as a string key instead of the wire
 * enum so this file stays gen-free. `''` is `ALIGNMENT_UNSPECIFIED` — not
 * chosen. Labels are the proto's own comments, word for word. */
export type AlignmentKey =
  | ''
  | 'lawful_good'
  | 'neutral_good'
  | 'chaotic_good'
  | 'lawful_neutral'
  | 'neutral'
  | 'chaotic_neutral'
  | 'lawful_evil'
  | 'neutral_evil'
  | 'chaotic_evil';

export const ALIGNMENT_LABELS: Record<AlignmentKey, string> = {
  '': 'Não escolhido',
  lawful_good: 'Leal e bom',
  neutral_good: 'Neutro e bom',
  chaotic_good: 'Caótico e bom',
  lawful_neutral: 'Leal e neutro',
  neutral: 'Neutro',
  chaotic_neutral: 'Caótico e neutro',
  lawful_evil: 'Leal e mau',
  neutral_evil: 'Neutro e mau',
  chaotic_evil: 'Caótico e mau',
};

/** How a character's maximum hit points grow after the first level, which
 * always takes the hit die's maximum (`HitPointsMethod`, characters.proto). */
export type HitPointsMethod = 'average' | 'rolled';

/** One class after the first on the sheet (`ClassLevel`). */
export interface ExtraClassValue {
  classKey: string;
  level: number;
  subclassKey: string;
  customSubclassName: string;
}

/**
 * The form value for a player, enemy or boss (`FullSheet`). Every field
 * that is a content key (`race`, `subrace`, `className`, `subclassName`,
 * `background`, skill/expertise keys, `armor`, `weapons`, `cantrips`,
 * `spellsKnown`, `spellsPrepared`) is picked from the loaded
 * `RulesCatalogVm`, never typed — a typed key almost never matches a real
 * one, and `CreateCharacter`/`UpdateCharacter` reject it with
 * `invalid_argument` (integrator fix: the editor must never make a person
 * type a content key). Only genuinely free text stays free text: other
 * equipment items, languages, tool proficiencies, and custom features —
 * `FullSheet.equipment`/`languages`/`tool_proficiencies` are themselves
 * free text or simple repeated strings on the wire, so a line-per-item
 * textarea maps onto them directly. Equipment quantity round-trips through
 * a "(xN)" suffix on that same line (`CharacterEditorSourceLive`'s
 * `itemsFromLines`/`lineFromItem`) rather than a separate structured
 * field — simple enough to be lossless without a dedicated add/remove list
 * UI (integrator review, phase 2b).
 *
 * `featureChoiceKeys` and `featureChoiceText` (a fighting style, a dragon ancestry, an eldritch
 * invocation, the humanoid races of a favored enemy...) are not typed either: the "Escolhas" step
 * draws them from `CharacterEditorSource.previewChoices` (PM-05), which says which choices the draft
 * has and the exact string each option stores; a pick only ever writes what the server listed.
 */
export interface CharacterFormValue {
  name: string;
  race: string;
  subrace: string;
  className: string;
  subclassName: string;
  customSubclassName: string;
  level: number;
  /** `'custom'` selects the "Outro (personalizado)" option — see
   * `customBackgroundName` and `customBackgroundSkills`. */
  background: string;
  customBackgroundName: string;
  customBackgroundSkills: [string, string] | null;
  /** The "Outro" background's two tools or languages (content keys), the feature the player
   * wrote and the equipment (SRD 5.1 "Customizing a Background"). */
  customBackgroundProficiencies: string[];
  customBackgroundFeatureName: string;
  customBackgroundFeatureText: string;
  customBackgroundEquipment: string;
  /** The classes after the first (multiclass at creation): each its own level and subclass. The
   * first class is `className`, `level` and the subclass fields above. */
  extraClasses: ExtraClassValue[];
  skillProficiencies: string[];
  /** A subset of `skillProficiencies`: expertise doubles the proficiency
   * bonus (Bard, Rogue). */
  expertiseSkillKeys: string[];
  abilities: AbilityScoresInput;
  /** Manual bonuses on top of race/subrace (an ability score improvement,
   * a race's chosen +1s, a magic item): each -10 to +10. */
  extraAbilityBonuses: AbilityScoresInput;
  hitPointsMethod: HitPointsMethod;
  /** Only meaningful with `hitPointsMethod === 'rolled'`: one roll per
   * level after the first (`level - 1` entries, since the MVP editor's one
   * class means "every level of the first class" is every level). */
  hitPointsRolls: number[];
  isCaster: boolean;
  /** Content keys, picked from `RulesCatalogVm.spells` filtered to
   * `level === 0` and the chosen class's list (`ClassOptionVm.key` in
   * `Spell.classKeys`). */
  cantrips: string[];
  /** Content keys, `RulesCatalogVm.spells` filtered to `level >= 1` and the
   * chosen class's list. Shown when the class's preparation is "known" or
   * "spellbook". */
  spellsKnown: string[];
  /** Same filter as `spellsKnown`. Shown when the class's preparation is
   * "prepared" or "spellbook". */
  spellsPrepared: string[];
  /** A content key from `RulesCatalogVm.armor`, or `''` for "Sem armadura". */
  armor: string;
  shield: boolean;
  /** Content keys from `RulesCatalogVm.weapons`. */
  weapons: string[];
  equipmentText: string;
  languagesText: string;
  toolProficienciesText: string;
  experiencePoints: number;
  /** An enemy's or boss's challenge rating ("ND"): "0", "1/8", "1/4", "1/2",
   * "1" to "30", or `''` for none. A player's is always `''` and 0. */
  challengeRating: string;
  /** The XP an enemy or boss gives when defeated ("XP ao derrotar"). */
  xpValue: number;
  /** An enemy's or boss's portrait, a gallery image's ID or empty (MR-031). A
   * player's is always empty. */
  portraitImageId: string;
  alignment: AlignmentKey;
  /** Locks with the rest of the sheet, unlike the story fields (A3). */
  customFeaturesText: string;
  /** The picks of the class and race choices, in the order they were made (`FullSheet.feature_choice_keys`). */
  featureChoiceKeys: string[];
  /** The free texts of those picks, by `<choice key>#<n>` (`FullSheet.feature_choice_text`). */
  featureChoiceText: Record<string, string>;
  /** A College of Lore bard's "Perguntar" setting for Palavras de Interrupção: on which rolls of an enemy the combat asks. */
  cuttingWordsAsk: CuttingWordsAskKey;
  /** The feats of the sheet (`FullSheet.feat_keys`, MR-025). Set by the master's editor only: absent, the save keeps the stored feats. */
  featKeys?: string[];
  /** The size picked among the ones the race offers (`FullSheet.size`, not the NPC's `size`); `''` is the race's own. */
  raceSize?: string;
  /** A feat taken in place of an improvement (`FullSheet.feat_slots`), carried with `featKeys`; the server drops the slot of a removed feat. */
  featSlots?: Record<string, string>;
}

/** `CuttingWordsAsk`: every roll of the enemy, attacks only (the default) or never. */
export type CuttingWordsAskKey = 'all' | 'only-attacks' | 'never';

/** One attack of a minion or story NPC (`BasicAttack`). */
export interface BasicAttackFormValue {
  name: string;
  attackBonus: number;
  damageDiceCount: number;
  damageDiceSides: number;
  damageBonus: number;
  damageType: DamageTypeKey;
  /** `BasicAttack.range_ft`, kept as saved: the form has no field for it. */
  rangeFt: number;
}

/** The form value for a minion or story NPC (`BasicSheet`) — the "single
 * short form" the plan asks for. */
export interface BasicCharacterFormValue {
  name: string;
  hitPointsMax: number;
  armorClass: number;
  speedFt: number;
  initiativeBonus: number;
  attacks: BasicAttackFormValue[];
  /** `BasicSheet.damage` and `attack_bonus`, the deprecated fields of a
   * sheet saved before the attacks list existed whose damage could not become an attack. The
   * form shows the text as a note and sends both back unchanged. */
  legacyDamage: string;
  legacyAttackBonus: number;
  description: string;
  /** The challenge rating ("ND") and the XP the minion gives when defeated,
   * as `FullSheet` has them (E7-11). */
  challengeRating: string;
  xpValue: number;
  /** The NPC's portrait, a gallery image's ID or empty (MR-031). Carried
   * through unchanged, so saving the short form never clears it. */
  portraitImageId: string;
  /** The NPC's size (`rules.v1.CreatureSize`, 0 = unset, Medium), carried through
   * unchanged: there is no control for it yet (the combat reads it), and saving
   * must never clear it. */
  size: number;
}

/** One row of the SRD's "Experience Points by Challenge Rating" table
 * (`rules.v1.ChallengeRating`): "1/4" gives 50 XP. */
export interface ChallengeRatingVm {
  readonly rating: string;
  readonly xp: number;
}

/** What marks an entry of the table's own content: "Da mesa" on screen, "Arquivada" for the master. */
export interface TableMark {
  /** The key ends in "@mesa": the master wrote it for this campaign. */
  readonly fromTable: boolean;
  /** Retired by the master: the master receives it marked, a player only when their own sheet uses it. */
  readonly archived: boolean;
  /** Switched off for the players (10.1d): the master may still pick it, a player never gets it unless their sheet uses it. */
  readonly off: boolean;
}

export interface SubraceOptionVm extends TableMark {
  readonly key: string;
  readonly namePt: string;
  /** The subrace's Constitution increase, added to the race's: the HP
   * preview needs the final score. The other abilities are not read here. */
  readonly constitutionBonus: number;
  /** The skills its traits give (`Subrace.skill_keys`), which the player does not choose; absent means none. */
  readonly skillKeys?: readonly string[];
  /** The subrace swaps the race's ability bonuses whole (`Subrace.replaces_ability_bonuses`): the race's Constitution and
   * placed bonuses are gone. */
  readonly replacesAbilityBonuses?: boolean;
  /** The race's skills that go with the traits it replaces (`Subrace.replaced_skill_keys`). */
  readonly replacedSkillKeys?: readonly string[];
}

export interface RaceOptionVm extends TableMark {
  readonly key: string;
  readonly namePt: string;
  /** The race's Constitution increase (see `SubraceOptionVm`). */
  readonly constitutionBonus: number;
  /** "+2 and +1 to your choice" as [2, 1]: the player places them in the manual bonuses; empty for the SRD's races. */
  readonly choiceBonuses: readonly number[];
  /** Alternative sets of such bonuses, of which the player picks one ([[2, 1], [1, 1, 1]]); absent means none (`Race.choice_bonus_sets`). */
  readonly choiceBonusSets?: readonly (readonly number[])[];
  /** The sizes the player picks one from ("Small", "Medium"), and the race's own (`Race.sizes`, `Race.size`); absent means no choice. */
  readonly sizes?: readonly string[];
  readonly size?: string;
  /** The skills its traits give (`Race.skill_keys`), which the player does not choose; absent means none. */
  readonly skillKeys?: readonly string[];
  readonly subraces: readonly SubraceOptionVm[];
}

/** A third caster's subclass (the table's, or an SRD fighter's or rogue's): it casts on its own. */
export interface SubclassCastingVm {
  readonly preparation: SpellPreparation | null;
  /** The class whose spell list it casts from. */
  readonly listClassKey: string;
  /** The class level casting starts at. */
  readonly firstLevel: number;
  /** The highest circle at each class level, index 0 = level 1. */
  readonly maxSpellLevelByLevel: readonly number[];
}

export interface SubclassOptionVm extends TableMark {
  readonly key: string;
  readonly namePt: string;
  /** Set only for a subclass that casts. */
  readonly casting: SubclassCastingVm | null;
  /** The spells it always prepares, from a class level: they never count against the limit. */
  readonly alwaysPrepared: readonly { readonly spellKey: string; readonly classLevel: number }[];
}

/** How a caster class handles its spell list (integrator amendment,
 * 29/09/2026, from WP-A's `rules.v1` contract): `'known'` (e.g. Sorcerer,
 * Bard — a fixed list of spells known, all always available); `'prepared'`
 * (e.g. Cleric, Druid — chooses which spells to prepare each day from the
 * whole class list, no separate "known" list); `'spellbook'` (Wizard —
 * both a spellbook of known spells and a smaller prepared subset). Only
 * meaningful when `isCaster` is true. */
export type SpellPreparation = 'known' | 'prepared' | 'spellbook';

export interface ClassOptionVm extends TableMark {
  readonly key: string;
  readonly namePt: string;
  /** How many skills the class chooses at level 1 (`skill_choice.count`), from the server's entry. */
  readonly skillChoose: number;
  /** The two saving throws the class is proficient in, as the server lists them. */
  readonly savingThrows: readonly AbilityKey[];
  /** The class whose spell list this one casts from: its own key, or the one a table class reuses
   * (`ClassSpellcasting.list_class_key`). Empty for a class that never casts. */
  readonly spellListClassKey: string;
  /** Faces of the hit die: 6, 8, 10 or 12. */
  readonly hitDie: number;
  readonly isCaster: boolean;
  readonly preparation: SpellPreparation | null;
  readonly subclasses: readonly SubclassOptionVm[];
  /** The class level at which the subclass is chosen (`subclass_level`). */
  readonly subclassLevel: number;
  /** The class level at which spellcasting starts (`first_level`), or 0 for
   * a class that never casts. */
  readonly spellcastingFirstLevel: number;
  /** The highest spell circle at each class level, index 0 = level 1 (from
   * the server, `max_spell_level_by_level`). Empty for a non-caster. */
  readonly maxSpellLevelByLevel: readonly number[];
}

export interface BackgroundOptionVm extends TableMark {
  readonly key: string;
  readonly namePt: string;
  /** A table background's equipment, as text; empty for the SRD's. */
  readonly equipmentPt: string;
  /** The skills it gives (`Background.skill_keys`), which the player does not choose; absent means none. */
  readonly skillKeys?: readonly string[];
}

export interface SkillOptionVm {
  readonly key: string;
  readonly namePt: string;
  readonly ability: AbilityKey;
}

export interface ArmorOptionVm {
  readonly key: string;
  readonly namePt: string;
}

export interface WeaponOptionVm {
  readonly key: string;
  readonly namePt: string;
  /** A weapon of the master's table (key `weapon:…@mesa`), not the book's. */
  readonly fromTable: boolean;
  readonly archived: boolean;
  readonly off: boolean;
  readonly category: 'simple' | 'martial';
  /** The damage dice ("1d6"); empty for a weapon that deals none. */
  readonly damage: string;
}

export interface SpellOptionVm {
  readonly key: string;
  readonly namePt: string;
  /** 0 is a cantrip, 1-9 a leveled spell — this decides whether a spell
   * belongs in "Truques" or in "Magias conhecidas"/"preparadas". Which
   * circles a character level reaches comes from the server
   * (`ClassOptionVm.maxSpellLevelByLevel`); the browser only filters by it. */
  readonly level: number;
  /** Content keys of the classes whose spell list has this spell. */
  readonly classKeys: readonly string[];
  /** The master's own spell ("Da mesa"). */
  readonly fromTable: boolean;
  readonly archived: boolean;
  readonly off: boolean;
}

export interface ToolOrLanguageVm {
  readonly key: string;
  readonly namePt: string;
  readonly kind: 'tool' | 'language';
}

/** `rules.v1.Content`, trimmed to what the editor's dropdowns need
 * (plan §4's `ContentService.ListContent`). */
export interface RulesCatalogVm {
  readonly races: readonly RaceOptionVm[];
  readonly classes: readonly ClassOptionVm[];
  /** SRD 5.1 has a single background, Acólito (ADR-0008) — the editor
   * always adds a fixed "Outro (personalizado)" option after these. */
  readonly backgrounds: readonly BackgroundOptionVm[];
  readonly skills: readonly SkillOptionVm[];
  /** Body armor only — a shield is the separate `shield` checkbox. */
  readonly armor: readonly ArmorOptionVm[];
  readonly weapons: readonly WeaponOptionVm[];
  readonly spells: readonly SpellOptionVm[];
  /** The tools and languages an "Outro" background may grant, named by the catalog (`Content.proficiencies` and `languages`). */
  readonly toolsAndLanguages: readonly ToolOrLanguageVm[];
  /** The caller is the campaign's master: only they are offered what is switched off for the players. */
  readonly viewerIsMaster: boolean;
  /** The ND to XP table, in order (0, 1/8, 1/4, 1/2, 1 to 30), for the NPC's
   * "Nível de desafio (ND)" picker. */
  readonly challengeRatings: readonly ChallengeRatingVm[];
  /** The campaign's way of earning XP (`Campaign.xp_mode`). */
  readonly xpMode?: 'enemies' | 'gold' | 'milestones';
  /** The XP that reaches each level, from level 1 (0) to level 20 (`Content.level_xp`). */
  readonly levelXp?: readonly number[];
  /** The campaign's feats, for the master's "Talentos" section; absent for a player, or when the list could not be read. */
  readonly feats?: FeatCatalogVm;
}

/** One feat the master may give a sheet (`OptionSwitchEntry` of the kind feat). */
export interface FeatOptionVm {
  readonly key: string;
  readonly namePt: string;
  readonly fromTable: boolean;
  /** Switched off for the players: the master may still give it. */
  readonly off: boolean;
}

/** The feats of a campaign and whether its table uses them (the rule "Talentos"). */
export interface FeatCatalogVm {
  readonly options: readonly FeatOptionVm[];
  readonly featsAllowed: boolean;
}

/** How a player made the base scores of a new sheet (`AbilityMethod`, RN-24), as a string key so this file stays gen-free. */
export type AbilityMethodKey = 'standard_array' | 'point_buy' | 'rolled_4d6' | 'typed';

/** One roll of "4d6, dropping the lowest": four dice and the server's total. */
export interface AbilityRollSetVm {
  readonly dice: readonly number[];
  readonly total: number;
}

/** The six sets the server stored for the player's next sheet (`GetAbilityRolls`, `RollAbilityScores`). */
export interface AbilityRollsVm {
  readonly sets: readonly AbilityRollSetVm[];
  /** The player typed the dice (physical dice); false when the server rolled them. */
  readonly typed: boolean;
  readonly rolledAt: Date | null;
}

/**
 * What the "Habilidades" step needs from the table's rules (RN-24) when a player makes a new sheet: the ways the master
 * allows, the numbers they use (the server's, so the browser does no rules math), where the dice are rolled (RN-18)
 * and the roll the server already stored.
 */
export interface AbilityTableVm {
  readonly standardArray: boolean;
  readonly pointBuy: boolean;
  readonly rolled4d6: boolean;
  readonly typed: boolean;
  readonly standardValues: readonly number[];
  readonly pointBuyCosts: readonly number[];
  readonly pointBuyMinScore: number;
  readonly pointBuyBudget: number;
  readonly typedMin: number;
  readonly typedMax: number;
  /** How the table makes the hit points of a new sheet above level 1 (RN-24): the player chooses, or only the die, or only the average. */
  readonly hitPoints: 'player_chooses' | 'roll' | 'average';
  /** The player rolls with their own dice (the campaign forces it, or they chose it): they type the dice, once. */
  readonly physicalDice: boolean;
  /** The campaign makes everybody roll the same way, so the player has no say. */
  readonly diceForced: boolean;
  readonly rolls: AbilityRollsVm | null;
}

export interface CreateCharacterInput {
  readonly campaignId: string;
  readonly kind: CharacterKind;
  readonly full: CharacterFormValue | null;
  readonly basic: BasicCharacterFormValue | null;
  /** How a player's base scores were made; the server checks them against it (RN-24). */
  readonly abilityMethod?: AbilityMethodKey;
  /** The master makes a player character for a player to claim: saved as reserved, with no owner (MR-049). */
  readonly forPlayer?: boolean;
  /** One per create, sent again on a retry: the server makes the character once (a UUID; see `ActionKey`). */
  readonly idempotencyKey: string;
}

export interface UpdateCharacterInput {
  readonly campaignId: string;
  readonly characterId: string;
  readonly revision: number;
  readonly name: string;
  readonly full: CharacterFormValue | null;
  readonly basic: BasicCharacterFormValue | null;
}

/** A draft sheet to derive without saving it (`PreviewCharacter`). */
export interface PreviewCharacterInput {
  readonly campaignId: string;
  /** The character being edited; `null` while creating one. */
  readonly characterId: string | null;
  readonly kind: CharacterKind;
  readonly full: CharacterFormValue;
  /** As `CreateCharacterInput.forPlayer`: the master previews a reserved character's sheet. */
  readonly forPlayer?: boolean;
}

/** What the server derives for a draft: the numbers the editor's "Pontos de vida" box shows. */
export interface CharacterPreviewVm {
  /** `DerivedSheet.hit_points_max`: what the saved sheet would have. */
  readonly hitPointsMax: number;
  /** `DerivedSheet.hit_points_from_effects`: what race, class and feature effects add (negative if one takes away). */
  readonly hitPointsFromEffects: number;
  /** `DerivedSheet.spellcasting`: what each casting class of the draft knows and prepares at its level. */
  readonly spellcasting: readonly SpellLimitsVm[];
  /** What the server said about a feat of the draft (its prerequisite not met), by feat key. */
  readonly featIssues?: Readonly<Record<string, string>>;
}

/** How many cantrips and spells a casting class has at its level (`Spellcasting`); 0 where the class has no such number. */
export interface SpellLimitsVm {
  readonly classKey: string;
  readonly cantripsKnown: number;
  /** Spells a "known" caster (bard, ranger, sorcerer, warlock) knows; 0 for a class that prepares. */
  readonly spellsKnown: number;
  /** Spells a preparing class (cleric, druid, paladin, wizard) prepares each day; 0 for the others. */
  readonly preparedMax: number;
}

/** What `PreviewChoices` says about a draft: the choices its race and classes ask, and where the pickers find spells. */
export interface ChoicesPreviewVm {
  /** Empty when the draft asks nothing (a human rogue 1): the editor has no step for it. */
  readonly groups: readonly ChoiceGroup[];
  /** "Escolhas feitas: done de total". */
  readonly done: number;
  readonly total: number;
  /** The picks the draft stores that no choice of it offers (the class or the race changed): the editor drops them. */
  readonly notOffered: readonly string[];
  /** For each casting class, the spells outside its list (the patron's, Magical Secrets). */
  readonly spellSources: readonly ChoiceSpellSource[];
  /** The skills the draft's sources allow (`any` when a source takes any); null when the server names none. */
  readonly skillsAllowed: PickAllowed | null;
  /** The skills expertise may go on; null when it is not cut. */
  readonly expertiseAllowed: PickAllowed | null;
}

export interface CharacterForEdit {
  readonly kind: CharacterKind;
  readonly revision: number;
  readonly full: CharacterFormValue | null;
  readonly basic: BasicCharacterFormValue | null;
  /** Why the caller may not save this sheet now (`Character.can_edit` is
   * false), or null when they may. The editor shows the reason instead of
   * a form whose save the server would refuse (RN-01, RN-03). */
  readonly blocked: CharacterBlockedReason | null;
  /** The player's sheet is locked (a session started, RN-01) or the
   * character is dead. The master may still edit it; the XP is then read-only
   * here, because only awards change it (MR-016). */
  readonly sheetLocked: boolean;
  /** The character is reserved (MR-049): made by the master for a player to claim, with no owner. */
  readonly reserved?: boolean;
  /** How the base scores were made, as the server recorded it at creation (RN-24); `null` for NPCs and sheets made before the rules. */
  readonly abilityOrigin?: {
    readonly method: AbilityMethodKey;
    readonly rolls: AbilityRollsVm | null;
  } | null;
  /** The leveled spells the server's derived sheet has prepared that none of the sheet's own lists holds: the ones a
   * subclass always prepares (a domain's, an oath's), which never count against the limit. Read from the saved sheet,
   * so only an edit has them (a new sheet has no derived sheet yet). */
  readonly grantedSpellKeys?: readonly string[];
  /** How many spells each casting class prepares (the saved sheet's `spellcasting[].prepared_max`), by class key. */
  readonly preparedMax?: Readonly<Record<string, number>>;
  /** The keys of the features the server's derived sheet has (`feature:cutting-words` shows the bard's "Perguntar" setting). An edit only. */
  readonly featureKeys?: readonly string[];
}

/**
 * The port `CharacterEditor` depends on. Phase 2 provides a concrete
 * implementation wrapping `rules.v1.ContentService` and
 * `characters.v1.CharacterService` (see this file's top comment).
 */
export abstract class CharacterEditorSource {
  /** With `characterId` (an edit of the caller's own sheet), the entries the sheet has come back even when retired. */
  abstract loadCatalog(campaignId: string, characterId?: string): Promise<RulesCatalogVm>;
  /** One spell in full, for the "?" next to its name. */
  abstract loadSpellDetails(campaignId: string, spellKey: string): Promise<SpellDetailsVm>;
  abstract loadCharacterForEdit(campaignId: string, characterId: string): Promise<CharacterForEdit>;
  /** The table's ways of making ability scores, for a player (or a pending member) creating a sheet; `null` for the master, whose NPCs are free. */
  abstract loadAbilityTable(campaignId: string): Promise<AbilityTableVm | null>;
  /** `RollAbilityScores`: the server rolls the six sets (or stores the typed dice) and keeps them; asking again returns the same. */
  abstract rollAbilityScores(
    campaignId: string,
    typedDice?: readonly (readonly number[])[],
  ): Promise<AbilityRollsVm>;
  /** `PreviewCharacter`: the server derives the draft and writes nothing. */
  abstract previewCharacter(input: PreviewCharacterInput): Promise<CharacterPreviewVm>;
  /** `PreviewChoices`: the choices the draft asks, with what is picked in it; writes nothing. */
  abstract previewChoices(input: PreviewCharacterInput): Promise<ChoicesPreviewVm>;
  abstract createCharacter(input: CreateCharacterInput): Promise<{ characterId: string }>;
  abstract updateCharacter(input: UpdateCharacterInput): Promise<{ revision: number }>;
}
