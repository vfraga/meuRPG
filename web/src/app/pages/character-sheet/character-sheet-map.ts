import { type Timestamp, timestampDate } from '@bufbuild/protobuf/wkt';

import {
  Alignment as GenAlignment,
  type Appearance as GenAppearance,
  BasicSheet as GenBasicSheet,
  Character,
  CharacterReview as GenCharacterReview,
  CharacterState as GenCharacterState,
  CharacterStory as GenCharacterStory,
  FullSheet as GenFullSheet,
  type Personality as GenPersonality,
  LevelUpReason as GenLevelUpReason,
  Review as GenReview,
} from '../../../gen/meurpg/characters/v1/characters_pb';
import {
  Attack as GenAttack,
  AttackKind as GenAttackKind,
  DerivedSheet as GenDerivedSheet,
  ProficiencyLevel as GenProficiencyLevel,
} from '../../../gen/meurpg/rules/v1/rules_pb';
import { toGearVm } from '../../core/characters/gear';
import { speedExtrasOf } from './character-sheet-speeds';
import { KIND_FROM_GEN } from '../../core/characters/character-kind-gen';
import { ABILITY_FROM_GEN } from '../../core/characters/ability-gen';
import { SkillProficiency } from '../../core/characters/character-labels';
import { metersWithFeet } from '../../core/units';
import { AbilityKey, CharacterState, ReviewStatus } from '../../core/characters/characters.types';
import { damageTypeFromGen } from '../../core/characters/damage-type-gen';
import {
  AttackVm,
  BasicSheetVm,
  CharacterSheetVm,
  CharacterStoryVm,
  FullSheetVm,
  ReviewVm,
} from './character-sheet.types';
import { hitDiceWords } from '../../core/resources/hit-dice-text';

const STATE_FROM_GEN: Record<GenCharacterState, CharacterState> = {
  [GenCharacterState.UNSPECIFIED]: 'draft',
  [GenCharacterState.DRAFT]: 'draft',
  [GenCharacterState.LOCKED]: 'locked',
  [GenCharacterState.DEAD]: 'dead',
  // Created through an invite that requires approval (RN-15, MR-024).
  [GenCharacterState.PENDING]: 'pending',
};

const REVIEW_FROM_GEN: Partial<Record<GenReview, ReviewStatus>> = {
  [GenReview.AWAITING]: 'awaiting',
  [GenReview.CHANGES_REQUESTED]: 'changes_requested',
  [GenReview.RESUBMITTED]: 'resubmitted',
};

/** A wire timestamp as a date, `null` when it is unset. */
function dateOf(ts: Timestamp | undefined): Date | null {
  return ts ? timestampDate(ts) : null;
}

function toReviewVm(review: GenCharacterReview | undefined): ReviewVm | null {
  const status = review ? REVIEW_FROM_GEN[review.status] : undefined;
  if (!review || !status) {
    return null;
  }
  return {
    status,
    reason: review.reason,
    requestedAt: dateOf(review.requestedAt),
    resubmittedAt: dateOf(review.resubmittedAt),
  };
}

const LEVEL_UP_REASON: Record<GenLevelUpReason, 'xp' | 'milestone' | null> = {
  [GenLevelUpReason.UNSPECIFIED]: null,
  [GenLevelUpReason.XP]: 'xp',
  [GenLevelUpReason.MILESTONE]: 'milestone',
};

const PROFICIENCY_FROM_GEN: Record<GenProficiencyLevel, SkillProficiency> = {
  [GenProficiencyLevel.UNSPECIFIED]: 'none',
  [GenProficiencyLevel.NONE]: 'none',
  [GenProficiencyLevel.HALF]: 'half',
  [GenProficiencyLevel.PROFICIENT]: 'proficient',
  [GenProficiencyLevel.EXPERTISE]: 'expertise',
};

/** `Alignment`'s Portuguese label, the proto's own comments word for word
 * — same wording as the editor's select (`character-editor.types.ts`'s
 * `ALIGNMENT_LABELS`, kept separate on purpose: this file stays
 * self-contained, per page-folder). `''` for `UNSPECIFIED` — "nothing when
 * unset" (integrator follow-up). */
const ALIGNMENT_LABEL_FROM_GEN: Record<GenAlignment, string> = {
  [GenAlignment.UNSPECIFIED]: '',
  [GenAlignment.LAWFUL_GOOD]: 'Leal e bom',
  [GenAlignment.NEUTRAL_GOOD]: 'Neutro e bom',
  [GenAlignment.CHAOTIC_GOOD]: 'Caótico e bom',
  [GenAlignment.LAWFUL_NEUTRAL]: 'Leal e neutro',
  [GenAlignment.NEUTRAL]: 'Neutro',
  [GenAlignment.CHAOTIC_NEUTRAL]: 'Caótico e neutro',
  [GenAlignment.LAWFUL_EVIL]: 'Leal e mau',
  [GenAlignment.NEUTRAL_EVIL]: 'Neutro e mau',
  [GenAlignment.CHAOTIC_EVIL]: 'Caótico e mau',
};

/** "fogo" → "Fogo": a damage type's name starts a pill. */
function capitalized(text: string): string {
  return text === '' ? text : text[0].toLocaleUpperCase('pt-BR') + text.slice(1);
}

function toAttackVm(attack: GenAttack): AttackVm {
  return {
    key: attack.key,
    namePt: attack.namePt,
    kind: attack.kind === GenAttackKind.SPELL ? 'spell' : 'weapon',
    attackBonus: attack.attackBonus,
    damage: attack.damage,
    damageTypePt: attack.damageTypePt,
    versatileDamage: attack.versatileDamage,
    damageNotePt: attack.damageNotePt,
    saveDc: attack.saveDc,
    saveAbility: attack.saveDc > 0 ? ABILITY_FROM_GEN[attack.saveAbility] : null,
    beams: attack.beams,
  };
}

/** `SpellSlots` only lists levels with at least one slot; this turns that
 * into a dense array, index 0 = level 1 (`FullSheetVm.spellSlots`'s own
 * contract). */
function toSpellSlotsVm(slots: readonly { level: number; count: number }[]): number[] {
  const maxLevel = slots.reduce((max, s) => Math.max(max, s.level), 0);
  const dense = new Array<number>(maxLevel).fill(0);
  for (const s of slots) {
    dense[s.level - 1] = s.count;
  }
  return dense;
}

function spellcastingVm(sc: GenDerivedSheet['spellcasting'][number]) {
  return {
    className: sc.classNamePt,
    ability: ABILITY_FROM_GEN[sc.ability],
    saveDc: sc.saveDc,
    attackBonus: sc.attackBonus,
    cantripsKnown: sc.cantripsKnown,
    // Only one of the two applies: prepared_max for Cleric/Druid/Wizard,
    // spells_known for Bard/Ranger/Sorcerer/Warlock — see rules.proto's
    // SpellPreparation. The sheet says which one in its words.
    spellsPreparedMax: sc.preparedMax,
    spellsKnownMax: sc.spellsKnown,
    spellbook: sc.spellbook,
  };
}

function spellVm(cs: GenDerivedSheet['spells'][number]) {
  return {
    key: cs.spell?.key ?? '',
    namePt: cs.spell?.namePt ?? '',
    level: cs.spell?.level ?? 0,
    prepared: cs.prepared,
    ritual: cs.spell?.ritual ?? false,
    concentration: cs.spell?.concentration ?? false,
    reaction: cs.spell?.reaction ?? false,
  };
}

function featureVm(f: GenDerivedSheet['features'][number]) {
  return {
    name: f.namePt,
    sourcePt: f.sourcePt,
    description: f.description,
    descriptionPt: f.descriptionPt,
    descriptionPtMissing: f.descriptionPtMissing,
    descriptionPtOnly: f.descriptionPtOnly,
    summaryPt: f.summaryPt,
  };
}

function coinsVm(coins: GenFullSheet['coins']) {
  return {
    cp: coins?.copper ?? 0,
    sp: coins?.silver ?? 0,
    ep: coins?.electrum ?? 0,
    gp: coins?.gold ?? 0,
    pp: coins?.platinum ?? 0,
  };
}

/** Armour, weapons and tools, in the order the paper sheet lists them. */
function proficienciesVm(p: GenDerivedSheet['proficiencies']): string[] {
  return [...(p?.armor ?? []), ...(p?.weapons ?? []), ...(p?.tools ?? [])];
}

/** The six abilities, the saving throws and the skills, as the server derived them. */
function scoresVm(derived: GenDerivedSheet) {
  return {
    abilities: derived.abilities.map((a) => ({
      key: ABILITY_FROM_GEN[a.ability],
      score: a.score,
      modifier: a.modifier,
    })),
    savingThrows: derived.savingThrows.map((s) => ({
      key: ABILITY_FROM_GEN[s.ability],
      bonus: s.bonus,
      proficient: s.proficient,
    })),
    skills: derived.skills.map((s) => ({
      key: s.key,
      namePt: s.namePt,
      ability: ABILITY_FROM_GEN[s.ability],
      bonus: s.bonus,
      proficiency: PROFICIENCY_FROM_GEN[s.proficiency],
    })),
  };
}

function grantedSpellVm(g: GenDerivedSheet['grantedSpells'][number], derived: GenDerivedSheet) {
  const resource = derived.resources.find((r) => r.key === g.resource);
  return {
    key: g.spell?.key ?? '',
    namePt: g.spell?.namePt ?? '',
    level: g.spell?.level ?? 0,
    sourcePt: g.sourceNamePt,
    atWill: g.atWill,
    resource: g.resource,
    resourceNamePt: resource?.namePt ?? g.resource,
    alsoWithSlot: g.alsoWithSlot,
    ritualOnly: g.ritualOnly,
    countsAsKnown: g.countsAsKnown,
    ability: g.abilityPending || g.ability === '' ? null : (g.ability as AbilityKey),
    abilityPending: g.abilityPending,
    saveDc: g.saveDc,
    attackBonus: g.attackBonus,
  };
}

/** The spells and the slots to cast them with. */
function castingVm(derived: GenDerivedSheet) {
  return {
    spellcasting: derived.spellcasting.map(spellcastingVm),
    spellSlots: toSpellSlotsVm(derived.spellSlots),
    pactSlots: derived.pactMagic
      ? { level: derived.pactMagic.slotLevel, count: derived.pactMagic.count }
      : null,
    spells: derived.spells.map(spellVm),
    grantedSpells: derived.grantedSpells.map((g) => grantedSpellVm(g, derived)),
  };
}

/** What the rules service says about the sheet besides its numbers: issues, changed content and hints. */
function notesVm(derived: GenDerivedSheet) {
  return {
    issues: derived.issues.map((i) => ({ code: i.code, field: i.field, message: i.message })),
    changedContent: derived.changedContent.map((c) => ({
      key: c.key,
      namePt: c.namePt,
      changedAt: dateOf(c.changedAt),
      messages: [...c.messages],
    })),
    hints: derived.hints.map((h) => ({ sourceKey: h.sourceKey, text: h.text })),
  };
}

function toFullSheetVm(full: GenFullSheet, derived: GenDerivedSheet): FullSheetVm {
  return {
    kind: 'full',
    ...scoresVm(derived),
    proficiencyBonus: derived.proficiencyBonus,
    passivePerception: derived.passivePerception,
    passiveInvestigation: derived.passiveInvestigation,
    passiveInsight: derived.passiveInsight,
    initiative: derived.initiative,
    armorClass: derived.armorClass,
    armorClassDescription: derived.armorClassDescription,
    wearsArmor: full.armorKey !== '',
    hasShield: full.shield,
    shieldIdle: derived.shieldIdle,
    gear: derived.gear.map(toGearVm),
    hitPointsMax: derived.hitPointsMax,
    hitDice: hitDiceWords(derived.hitDice),
    speedWalkFt: derived.speedWalkFt,
    speedExtras: speedExtrasOf(derived),
    senses: derived.senses.map((s) => `${s.namePt}: ${metersWithFeet(s.rangeFt)}`),
    attacks: derived.attacks.map(toAttackVm),
    ...castingVm(derived),
    features: derived.features.map(featureVm),
    breathWeapon: derived.breathWeapon?.textPt ?? '',
    resistances: derived.resistances.map((r) => capitalized(r.damageTypeNamePt)),
    languages: derived.languages,
    proficiencies: proficienciesVm(derived.proficiencies),
    equipment: full.equipment.map((item) => ({ name: item.name, quantity: item.quantity || 1 })),
    backgroundEquipment: derived.backgroundEquipmentPt,
    coins: coinsVm(full.coins),
    customFeaturesText: full.customFeaturesText,
    ...notesVm(derived),
    hasWildShape: derived.features.some((f) => f.key.startsWith('feature:wild-shape')),
    contentVersion: derived.contentVersion,
  };
}

function toBasicSheetVm(basic: GenBasicSheet): BasicSheetVm {
  return {
    kind: 'basic',
    hitPointsMax: basic.hitPointsMax,
    armorClass: basic.armorClass,
    speedWalkFt: basic.speedFt,
    initiativeBonus: basic.initiativeBonus,
    attacks: basic.attacks.map((a) => ({
      name: a.name,
      attackBonus: a.attackBonus,
      damageDiceCount: a.damageDiceCount,
      damageDiceSides: a.damageDiceSides,
      damageBonus: a.damageBonus,
      damageType: damageTypeFromGen(a.damageType),
    })),
    legacyDamage: basic.attacks.length === 0 ? basic.damage : '',
    description: basic.description,
  };
}

function personalityVm(p: GenPersonality | undefined) {
  return {
    traits: p?.traits ?? '',
    ideals: p?.ideals ?? '',
    bonds: p?.bonds ?? '',
    flaws: p?.flaws ?? '',
  };
}

function appearanceVm(a: GenAppearance | undefined) {
  return {
    age: a?.age ?? '',
    height: a?.height ?? '',
    weight: a?.weight ?? '',
    eyes: a?.eyes ?? '',
    skin: a?.skin ?? '',
    hair: a?.hair ?? '',
    description: a?.description ?? '',
  };
}

/**
 * `CharacterStory` → `CharacterStoryVm`, every field. Exported so
 * `character-sheet-source.live.spec.ts` can prove `toCharacterStoryInit`
 * round-trips it unchanged (integrator fix, phase 2b: no field the
 * story form doesn't show may silently disappear on save).
 */
export function toStoryVm(story: GenCharacterStory | undefined): CharacterStoryVm {
  return {
    personality: personalityVm(story?.personality),
    appearance: appearanceVm(story?.appearance),
    backstory: story?.backstory ?? '',
    allies: story?.allies ?? '',
  };
}

/**
 * `CharacterStoryVm` → the wire `CharacterStory` init shape, every field —
 * the exact inverse of `toStoryVm`. Since `CharacterStoryVm` already
 * carries every field `CharacterStory` has (there is nothing left for the
 * story form not to show), this needs no "start from the loaded message"
 * merge the way `FullSheet` does: the round trip is lossless by
 * construction. See `character-sheet-source.live.spec.ts`.
 */
export function toCharacterStoryInit(story: CharacterStoryVm) {
  return {
    personality: { ...story.personality },
    appearance: { ...story.appearance },
    backstory: story.backstory,
    allies: story.allies,
  };
}

/** Defensive only: `CharacterSheet.content` is documented as always set to
 * the kind's matching case; this is just what renders if a future kind
 * this app does not know about yet ever reaches here unset. */
const EMPTY_BASIC_SHEET_VM: BasicSheetVm = {
  kind: 'basic',
  hitPointsMax: 0,
  armorClass: 0,
  speedWalkFt: 0,
  initiativeBonus: 0,
  attacks: [],
  legacyDamage: '',
  description: '',
};

function basicRating(character: Character): string {
  return character.sheet?.content.case === 'basic'
    ? character.sheet.content.value.challengeRating
    : '';
}

function basicXp(character: Character): number {
  return character.sheet?.content.case === 'basic' ? character.sheet.content.value.xpValue : 0;
}

/** The sheet the character carries: the full one with its derived numbers, the basic one, or the empty one. */
function sheetVmOf(character: Character): FullSheetVm | BasicSheetVm {
  const content = character.sheet?.content;
  if (content?.case === 'full' && character.derived) {
    return toFullSheetVm(content.value, character.derived);
  }
  return content?.case === 'basic' ? toBasicSheetVm(content.value) : EMPTY_BASIC_SHEET_VM;
}

/** The identity the official sheet's top block shows from the derived sheet: the race, the classes, the level. */
function derivedLabelsOf(derived: GenDerivedSheet | undefined) {
  return {
    // The subrace's name already says the race ("Gnomo das Rochas"), as the
    // paper sheet's "Raça" box does; the race alone when there is none.
    raceLabel: derived?.subraceNamePt || derived?.raceNamePt || '',
    classSummary: derived ? derived.classes.map((c) => `${c.namePt} ${c.level}`).join(' / ') : '',
    backgroundLabel: derived?.backgroundNamePt ?? '',
    totalLevel: derived?.totalLevel ?? 0,
    nextLevelXp: derived?.nextLevelXp ?? 0,
  };
}

/** Identity fields the official sheet's top block shows but DerivedSheet
 * does not carry — read straight from the stored FullSheet, not
 * computed (integrator follow-up). Empty or `null` for a BasicSheet: an NPC
 * has neither alignment nor experience points. */
function storedIdentityOf(character: Character) {
  const content = character.sheet?.content;
  const full = content?.case === 'full' ? content.value : undefined;
  return {
    alignmentLabel: full ? ALIGNMENT_LABEL_FROM_GEN[full.alignment] : '',
    experiencePoints: full ? full.experiencePoints : null,
    challengeRating: full ? full.challengeRating : basicRating(character),
    xpValue: full ? full.xpValue : basicXp(character),
  };
}

/** Exported so `character-sheet-source.live.spec.ts` can test the identity
 * fields (alignment, XP) directly, without going through the whole
 * `getCharacterSheet` RPC round trip. */
export function toCharacterSheetVm(character: Character): CharacterSheetVm {
  return {
    id: character.id,
    campaignId: character.campaignId,
    characterKind: KIND_FROM_GEN[character.kind],
    name: character.name,
    portraitImageId: character.sheet?.content.value?.portraitImageId ?? '',
    state: STATE_FROM_GEN[character.state],
    canEdit: character.canEdit,
    sheetLockedAt: dateOf(character.sheetLockedAt),
    diedAt: dateOf(character.diedAt),
    revivedAt: dateOf(character.revivedAt),
    revision: character.revision,
    sheet: sheetVmOf(character),
    story: toStoryVm(character.story),
    canEditStory: character.canEditStory,
    storyEditingAllowed: character.storyEditingAllowed,
    canToggleStoryEditing: character.canSetStoryEditing,
    canMarkDead: character.canMarkDead,
    canRevive: character.canRevive,
    canAccessMasterNotes: character.canAccessMasterNotes,
    canApprove: character.canApprove,
    canRequestChanges: character.canRequestChanges,
    canResubmit: character.canResubmit,
    review: toReviewVm(character.review),
    // Character carries no separate "is this caller the master" flag — this
    // one is the unconditional-on-character-state proxy the proto actually
    // offers: true for the master on every character, of any kind or
    // state, unlike can_mark_dead / can_set_story_editing which are also
    // gated on the character being a living player character.
    isMaster: character.canAccessMasterNotes,
    playerDisplayName: character.playerDisplayName || null,
    reserved: character.reserved,
    ...derivedLabelsOf(character.derived),
    ...storedIdentityOf(character),
    canLevelUp: character.canLevelUp,
    levelUpReason: LEVEL_UP_REASON[character.levelUpReason] ?? null,
  };
}
