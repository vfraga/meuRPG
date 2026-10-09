import { AbilityKey, CharacterKind, CharacterState } from '../../core/characters/characters.types';
import { DamageTypeKey, SkillProficiency } from '../../core/characters/character-labels';

/**
 * The view-model `CharacterSheetPage` renders. Phase 2 maps `GetCharacter`'s
 * response (a `characters.v1.Character`, carrying `sheet` and
 * `rules.v1.DerivedSheet`, plan §4) onto this shape; nothing below imports
 * from `../../../gen/...`, so that mapping is the only thing phase 2 adds —
 * this file, the component and its template do not change.
 *
 * The browser never computes a rule (ADR-0008): every number here is
 * exactly what the server sent, just formatted for display
 * (`character-labels.ts`'s `formatModifier`, `core/units.ts`).
 */

export interface AbilityScoreVm {
  readonly key: AbilityKey;
  readonly score: number;
  readonly modifier: number;
}

export interface SavingThrowVm {
  readonly key: AbilityKey;
  readonly bonus: number;
  readonly proficient: boolean;
}

export interface SkillVm {
  readonly key: string;
  /** From the server's `name_pt` — never a hand-copied list (plan §5). */
  readonly namePt: string;
  readonly ability: AbilityKey;
  readonly bonus: number;
  readonly proficiency: SkillProficiency;
}

/** `AttackKind`: whether an attack row is a weapon carried, or a damage
 * cantrip (integrator fix, phase 2 — `DerivedSheet.attacks` lists both, as
 * the official sheet does). */
export type AttackKindVm = 'weapon' | 'spell';

export interface AttackVm {
  readonly key: string;
  readonly namePt: string;
  readonly kind: AttackKindVm;
  /** The bonus to add to the d20. 0 for a spell that asks for a saving
   * throw instead (`kind === 'spell'` with `saveDc > 0`) — use `saveDc` /
   * `saveAbility` in that case, not `attackBonus`. */
  readonly attackBonus: number;
  readonly damage: string;
  readonly damageTypePt: string;
  /** The two-handed damage of a versatile weapon ("1d8+1"); empty otherwise. */
  readonly versatileDamage: string;
  /** > 0 only for a spell that asks for a saving throw. */
  readonly saveDc: number;
  readonly saveAbility: AbilityKey | null;
  /** How many attack rolls it makes in one action (Eldritch Blast: 1 to 4
   * beams); 0 for a weapon. */
  readonly beams: number;
}

export interface SpellcastingVm {
  readonly className: string;
  readonly ability: AbilityKey;
  readonly saveDc: number;
  readonly attackBonus: number;
  readonly cantripsKnown: number;
  /** The most spells the character can have prepared; 0 for a class that knows its spells. */
  readonly spellsPreparedMax: number;
  /** The most spells a class that knows them (Bard, Ranger, Sorcerer, Warlock) can know; 0 for a class that prepares. */
  readonly spellsKnownMax: number;
}

export interface FeatureVm {
  /** Portuguese name (`Feature.name_pt`). */
  readonly name: string;
  /** Where it comes from, in Portuguese, e.g. "Mago 1" or "Gnomo"
   * (`Feature.source_pt`) — shown next to the name in the compact row. */
  readonly sourcePt: string;
  /** The SRD's English text (`Feature.description`), collapsed by default
   * behind a native `<details>` — the sheet has no Portuguese text for
   * this yet (plan §5, "open questions"). */
  readonly description: string;
}

/** A real problem with the sheet (`DerivedSheet.issues`): shown in the
 * warning notice under the header, titled from its stable `code`
 * (`sheet-format.ts`'s `issueTitle`). */
export interface IssueVm {
  readonly code: string;
  readonly field: string;
  readonly message: string;
}

/** A table entry the sheet uses that changed after the sheet was last saved, and what no longer fits because of
 * it (`DerivedSheet.changed_content`, RN-23 question 80): the sentences exactly as the server wrote them. Never who
 * changed it nor the history of its edits. */
export interface ChangedContentVm {
  readonly key: string;
  readonly namePt: string;
  /** When the entry last changed. */
  readonly changedAt: Date | null;
  readonly messages: readonly string[];
}

/** A situational reminder the numbers above cannot express, such as
 * advantage on a saving throw against magic (`DerivedSheet.hints`,
 * `rules.proto`'s `Hint`). Shown as a quiet "Lembretes" list in
 * "Características e traços", never with the issues, which are real
 * problems and go in the notice under the header. */
export interface HintVm {
  readonly sourceKey: string;
  readonly text: string;
}

export interface EquipmentItemVm {
  readonly name: string;
  readonly quantity: number;
}

export interface CoinsVm {
  readonly cp: number;
  readonly sp: number;
  readonly ep: number;
  readonly gp: number;
  readonly pp: number;
}

/** A player, enemy or boss sheet (`FullSheet`, plan §4). */
export interface PactSlotsVm {
  readonly level: number;
  readonly count: number;
}

export interface FullSheetVm {
  readonly kind: 'full';
  readonly abilities: readonly AbilityScoreVm[];
  readonly proficiencyBonus: number;
  readonly savingThrows: readonly SavingThrowVm[];
  readonly skills: readonly SkillVm[];
  readonly passivePerception: number;
  readonly passiveInvestigation: number;
  readonly passiveInsight: number;
  readonly initiative: number;
  readonly armorClass: number;
  /** How `armorClass` was computed, in Portuguese, e.g. "Armadura de
   * couro + escudo", "Sem armadura", or the name of a feature such as
   * Unarmored Defense when it gives the better AC
   * (`DerivedSheet.armor_class_description`). */
  readonly armorClassDescription: string;
  /** Whether the stored sheet has body armor (`FullSheet.armor_key` is
   * set). "Equipamento" names the armor only then: without armor, the
   * description above may name a feature, not something carried. */
  readonly wearsArmor: boolean;
  /** Whether the stored sheet carries a shield (`FullSheet.shield`). */
  readonly hasShield: boolean;
  readonly hitPointsMax: number;
  readonly hitDice: string;
  readonly speedWalkFt: number;
  readonly senses: readonly string[];
  readonly attacks: readonly AttackVm[];
  readonly spellcasting: readonly SpellcastingVm[];
  /** Index 0 is level 1. */
  readonly spellSlots: readonly number[];
  /** A Warlock's Pact Magic: the slots are all of one level, and are kept apart from `spellSlots` (null without the feature). */
  readonly pactSlots: PactSlotsVm | null;
  readonly cantripNames: readonly string[];
  readonly spellNames: readonly string[];
  readonly features: readonly FeatureVm[];
  readonly languages: readonly string[];
  readonly proficiencies: readonly string[];
  readonly equipment: readonly EquipmentItemVm[];
  /** The numbers the equipped items change, one line each, for "por causa de" (W7-I). */
  readonly itemModifiers: readonly ItemModifierVm[];
  /** The background's equipment as text: a table background's, or what the
   * player wrote for a custom ("Outro") background; empty otherwise. */
  readonly backgroundEquipment: string;
  readonly coins: CoinsVm;
  /** Locks with the rest of the sheet, unlike `CharacterStoryVm` (A3). */
  readonly customFeaturesText: string;
  readonly issues: readonly IssueVm[];
  /** "A classe mudou": the table's entries this sheet uses that changed and left it with a new issue. */
  readonly changedContent: readonly ChangedContentVm[];
  readonly hints: readonly HintVm[];
  /** A druid with Wild Shape: the "Criaturas" panel exists for it before a first creature (MR-037). Display only. */
  readonly hasWildShape: boolean;
  /** Internal: never shown on the page (docs/design.md, "Nada interno na
   * tela"). */
  readonly contentVersion: string;
}

/** One number an item changes: what, by how much, and the item as the table calls it. */
export interface ItemModifierVm {
  readonly target: string;
  readonly value: number;
  readonly sourceItemId: string;
  readonly label: string;
}

/** A minion or story-NPC sheet (`BasicSheet`, plan §4). */
export interface BasicSheetVm {
  readonly kind: 'basic';
  readonly hitPointsMax: number;
  readonly armorClass: number;
  readonly speedWalkFt: number;
  readonly initiativeBonus: number;
  /** At most three, as the master typed them. */
  readonly attacks: readonly BasicAttackVm[];
  /** The deprecated free-text damage of a sheet saved before Etapa 6 that
   * could not become an attack; empty otherwise. */
  readonly legacyDamage: string;
  readonly description: string;
}

/** One attack of a basic sheet (`BasicAttack`), ready to show:
 * "Cimitarra +4 · 1d6 + 2 cortante". */
export interface BasicAttackVm {
  readonly name: string;
  readonly attackBonus: number;
  readonly damageDiceCount: number;
  readonly damageDiceSides: number;
  readonly damageBonus: number;
  readonly damageType: DamageTypeKey;
}

export interface PersonalityVm {
  readonly traits: string;
  readonly ideals: string;
  readonly bonds: string;
  readonly flaws: string;
}

export interface AppearanceVm {
  readonly age: string;
  readonly height: string;
  readonly weight: string;
  readonly eyes: string;
  readonly skin: string;
  readonly hair: string;
  readonly description: string;
}

/**
 * `CharacterStory` (plan amendment A3): personality, appearance, backstory
 * and allies. Never locks — editable by the owning player or the master in
 * every `CharacterState`, through `UpdateCharacterStory`, independently of
 * `sheet` and RN-01.
 */
export interface CharacterStoryVm {
  readonly personality: PersonalityVm;
  readonly appearance: AppearanceVm;
  readonly backstory: string;
  readonly allies: string;
}

export interface CharacterSheetVm {
  readonly id: string;
  readonly campaignId: string;
  readonly characterKind: CharacterKind;
  readonly name: string;
  /** An NPC's portrait, a gallery image's ID, or empty (MR-031). Only the
   * master reads an NPC's sheet. */
  readonly portraitImageId: string;
  readonly state: CharacterState;
  /** `Character.can_edit`: true for the master always (except a dead NPC's
   * game data, which simply has no lock concept); true for the owning
   * player only while `state` is `'draft'` (RN-01). */
  readonly canEdit: boolean;
  readonly sheetLockedAt: Date | null;
  readonly diedAt: Date | null;
  readonly revision: number;
  readonly sheet: FullSheetVm | BasicSheetVm;
  /**
   * `Character.story` — present for every kind, including a basic-sheet
   * NPC (`BasicSheet.description` is a separate, shorter field, for quick
   * reference at the table). The type stays nullable for a source that has
   * none to report; `CharacterSheetSourceLive` always maps one.
   */
  readonly story: CharacterStoryVm | null;
  /**
   * Whether the current caller may edit `story` right now
   * (`Character.can_edit_story`, integrator amendment to A3, 29/09/2026).
   * The master's value is always `true` — the master can always edit the
   * story. For a player it mirrors the server's own rule ("draft/pending,
   * or the master unlocked it") — the browser never recomputes this, it
   * only reads the flag, so `canEditStory` alone decides whether "Editar
   * história" shows, for either role.
   */
  readonly canEditStory: boolean;
  /** The master's per-character toggle: whether the player may currently
   * edit the story (`Character.story_editing_allowed`). Only meaningful —
   * and only shown — when `canToggleStoryEditing` is true. */
  readonly storyEditingAllowed: boolean;
  /** Whether this caller may see and use the "Permitir editar a
   * história" / "Travar a história" toggle (`Character.can_set_story_editing`)
   * — true for the master, for a player character. */
  readonly canToggleStoryEditing: boolean;
  /** `Character.can_mark_dead`: the master, for a player character that
   * is not dead yet. Gates "Marcar como morto" — never `isMaster` alone,
   * since an NPC can't be marked dead this way (RN-04: its hit points
   * belong to each combat). */
  readonly canMarkDead: boolean;
  /** `Character.can_access_master_notes`: the master. Gates both the
   * "Notas do mestre" panel and the `getMasterNotes` call (RN-11). */
  readonly canAccessMasterNotes: boolean;
  /** `Character.can_approve`: the master, for a character waiting for
   * approval (`state === 'pending'`, RN-15 / MR-024). Gates "Aprovar
   * personagem" and "Recusar personagem". */
  readonly canApprove: boolean;
  readonly isMaster: boolean;
  readonly playerDisplayName: string | null;
  readonly raceLabel: string;
  /** e.g. "Mago 3" (`CharacterSummary.class_summary`, plan §4). */
  readonly classSummary: string;
  readonly backgroundLabel: string;
  /**
   * `FullSheet.alignment`'s Portuguese label (the proto's own comments,
   * word for word — same labels the editor's select uses), or `''` when
   * unset or the sheet is a `BasicSheet` (integrator follow-up: the
   * official sheet's top block, read from the stored sheet — `DerivedSheet`
   * carries no alignment).
   */
  readonly alignmentLabel: string;
  /** `FullSheet.experience_points`, or `null` for a `BasicSheet` (an NPC
   * has no XP of its own). `0` is a real value (a fresh level-1 character)
   * and still shows — only `null` hides it. */
  readonly experiencePoints: number | null;
  /** `DerivedSheet.total_level` (0 when the sheet has no class yet): the XP block
   * says which level the next one is. */
  readonly totalLevel: number;
  /** `DerivedSheet.next_level_xp`: the XP that reaches the next level; 0 at level 20 or without a class. */
  readonly nextLevelXp: number;
  /** `Character.can_level_up` (RN-12): its XP reached the next level's, or the
   * master marked a milestone and its level has not gone up since. Only the
   * master and the owning player ever get it as true. */
  readonly canLevelUp: boolean;
  /** Why it can (`Character.level_up_reason`): XP, or a milestone the master marked; null when it cannot. */
  readonly levelUpReason: 'xp' | 'milestone' | null;
  /** An enemy's, boss's or minion's challenge rating ("ND") and the XP it gives
   * when defeated: the master's, `''` and 0 for everyone else (RN-20). */
  readonly challengeRating: string;
  readonly xpValue: number;
}

/** How the campaign levels (RN-09), as the sheet needs it: whether it counts XP. */
export type CampaignXpMode = 'enemies' | 'gold' | 'milestones';

/**
 * The port `CharacterSheetPage` depends on. Phase 2 provides a concrete
 * implementation wrapping the generated `CharacterService` client
 * (`meurpg.characters.v1`) — see this file's top comment.
 */
export abstract class CharacterSheetSource {
  /** How the campaign levels, to know whether the sheet has an XP block (a
   * milestones campaign has none) or only the "Pode subir de nível" tag. */
  abstract getXpMode(campaignId: string): Promise<CampaignXpMode>;
  abstract getCharacterSheet(campaignId: string, characterId: string): Promise<CharacterSheetVm>;
  /** Called only when `isMaster` — never for a player (RN-11). */
  abstract getMasterNotes(campaignId: string, characterId: string): Promise<string>;
  abstract updateMasterNotes(campaignId: string, characterId: string, notes: string): Promise<void>;
  /** `MarkCharacterDeadRequest` carries no revision — it is idempotent and
   * never changes one (characters.proto). */
  abstract markCharacterDead(campaignId: string, characterId: string): Promise<CharacterSheetVm>;
  abstract updateCharacterStory(
    campaignId: string,
    characterId: string,
    revision: number,
    story: CharacterStoryVm,
  ): Promise<CharacterSheetVm>;
  /** Master only: flips `story_editing_allowed` for this character
   * (`SetStoryEditing`). No revision: the proto's `SetStoryEditingRequest`
   * does not take one, and the call never changes `Character.revision`. */
  abstract setStoryEditingAllowed(
    campaignId: string,
    characterId: string,
    allowed: boolean,
  ): Promise<CharacterSheetVm>;
  /** Master only (MR-024): the pending character becomes a draft, and its
   * player a member of the campaign (`ApproveCharacter`). */
  abstract approveCharacter(campaignId: string, characterId: string): Promise<CharacterSheetVm>;
  /** Master only (MR-024): the pending character is deleted, and its
   * player's pending membership too (`RejectCharacter`). Nothing comes
   * back: the character is gone. */
  abstract rejectCharacter(campaignId: string, characterId: string): Promise<void>;
}
