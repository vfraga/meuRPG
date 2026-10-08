import { Injectable, inject } from '@angular/core';
import { createClient } from '@connectrpc/connect';
import { timestampDate } from '@bufbuild/protobuf/wkt';

import { CampaignService, XpMode } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  Alignment as GenAlignment,
  BasicSheet as GenBasicSheet,
  Character,
  CharacterKind as GenCharacterKind,
  CharacterService,
  CharacterState as GenCharacterState,
  CharacterStory as GenCharacterStory,
  FullSheet as GenFullSheet,
  LevelUpReason as GenLevelUpReason,
} from '../../../gen/meurpg/characters/v1/characters_pb';
import {
  Ability as GenAbility,
  Attack as GenAttack,
  AttackKind as GenAttackKind,
  DerivedSheet as GenDerivedSheet,
  ProficiencyLevel as GenProficiencyLevel,
} from '../../../gen/meurpg/rules/v1/rules_pb';
import { SkillProficiency } from '../../core/characters/character-labels';
import { metersWithFeet } from '../../core/units';
import { AbilityKey, CharacterKind, CharacterState } from '../../core/characters/characters.types';
import { damageTypeFromGen } from '../../core/characters/damage-type-gen';
import { CONNECT_TRANSPORT } from '../../core/connect/transport';
import {
  AttackVm,
  BasicSheetVm,
  CampaignXpMode,
  CharacterSheetSource,
  CharacterSheetVm,
  CharacterStoryVm,
  FullSheetVm,
} from './character-sheet.types';

const KIND_FROM_GEN: Record<GenCharacterKind, CharacterKind> = {
  [GenCharacterKind.UNSPECIFIED]: 'player',
  [GenCharacterKind.PLAYER]: 'player',
  [GenCharacterKind.ENEMY]: 'enemy',
  [GenCharacterKind.BOSS]: 'boss',
  [GenCharacterKind.MINION]: 'minion',
  [GenCharacterKind.STORY]: 'story',
};

const STATE_FROM_GEN: Record<GenCharacterState, CharacterState> = {
  [GenCharacterState.UNSPECIFIED]: 'draft',
  [GenCharacterState.DRAFT]: 'draft',
  [GenCharacterState.LOCKED]: 'locked',
  [GenCharacterState.DEAD]: 'dead',
  // Created through an invite that requires approval (RN-15, MR-024).
  [GenCharacterState.PENDING]: 'pending',
};

const LEVEL_UP_REASON: Record<GenLevelUpReason, 'xp' | 'milestone' | null> = {
  [GenLevelUpReason.UNSPECIFIED]: null,
  [GenLevelUpReason.XP]: 'xp',
  [GenLevelUpReason.MILESTONE]: 'milestone',
};

const ABILITY_FROM_GEN: Record<GenAbility, AbilityKey> = {
  [GenAbility.UNSPECIFIED]: 'str',
  [GenAbility.STRENGTH]: 'str',
  [GenAbility.DEXTERITY]: 'dex',
  [GenAbility.CONSTITUTION]: 'con',
  [GenAbility.INTELLIGENCE]: 'int',
  [GenAbility.WISDOM]: 'wis',
  [GenAbility.CHARISMA]: 'cha',
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

function toAttackVm(attack: GenAttack): AttackVm {
  return {
    key: attack.key,
    namePt: attack.namePt,
    kind: attack.kind === GenAttackKind.SPELL ? 'spell' : 'weapon',
    attackBonus: attack.attackBonus,
    damage: attack.damage,
    damageTypePt: attack.damageTypePt,
    saveDc: attack.saveDc,
    saveAbility: attack.saveDc > 0 ? ABILITY_FROM_GEN[attack.saveAbility] : null,
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

function toFullSheetVm(full: GenFullSheet, derived: GenDerivedSheet): FullSheetVm {
  return {
    kind: 'full',
    abilities: derived.abilities.map((a) => ({
      key: ABILITY_FROM_GEN[a.ability],
      score: a.score,
      modifier: a.modifier,
    })),
    proficiencyBonus: derived.proficiencyBonus,
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
    passivePerception: derived.passivePerception,
    passiveInvestigation: derived.passiveInvestigation,
    passiveInsight: derived.passiveInsight,
    initiative: derived.initiative,
    armorClass: derived.armorClass,
    armorClassDescription: derived.armorClassDescription,
    wearsArmor: full.armorKey !== '',
    hasShield: full.shield,
    hitPointsMax: derived.hitPointsMax,
    hitDice: derived.hitDice.map((hd) => `${hd.count}d${hd.faces}`).join(' + ') || '—',
    speedWalkFt: derived.speedWalkFt,
    senses: derived.senses.map((s) => `${s.namePt}: ${metersWithFeet(s.rangeFt)}`),
    attacks: derived.attacks.map(toAttackVm),
    spellcasting: derived.spellcasting.map((sc) => ({
      className: sc.classNamePt,
      ability: ABILITY_FROM_GEN[sc.ability],
      saveDc: sc.saveDc,
      attackBonus: sc.attackBonus,
      cantripsKnown: sc.cantripsKnown,
      // The one number worth a single line on the sheet: how many the
      // character can have ready, whichever the class's style calls it
      // (prepared_max for Cleric/Druid/Wizard, spells_known for
      // Bard/Ranger/Sorcerer/Warlock — see rules.proto's SpellPreparation).
      spellsPreparedMax: sc.preparedMax > 0 ? sc.preparedMax : sc.spellsKnown,
    })),
    spellSlots: toSpellSlotsVm(derived.spellSlots),
    pactSlots: derived.pactMagic
      ? { level: derived.pactMagic.slotLevel, count: derived.pactMagic.count }
      : null,
    cantripNames: derived.spells
      .filter((cs) => (cs.spell?.level ?? 0) === 0)
      .map((cs) => cs.spell?.namePt ?? ''),
    spellNames: derived.spells
      .filter((cs) => (cs.spell?.level ?? 0) > 0)
      .map((cs) => cs.spell?.namePt ?? ''),
    features: derived.features.map((f) => ({
      name: f.namePt,
      sourcePt: f.sourcePt,
      description: f.description,
    })),
    languages: derived.languages,
    proficiencies: [
      ...(derived.proficiencies?.armor ?? []),
      ...(derived.proficiencies?.weapons ?? []),
      ...(derived.proficiencies?.tools ?? []),
    ],
    equipment: full.equipment.map((item) => ({ name: item.name, quantity: item.quantity || 1 })),
    coins: {
      cp: full.coins?.copper ?? 0,
      sp: full.coins?.silver ?? 0,
      ep: full.coins?.electrum ?? 0,
      gp: full.coins?.gold ?? 0,
      pp: full.coins?.platinum ?? 0,
    },
    customFeaturesText: full.customFeaturesText,
    issues: derived.issues.map((i) => ({ code: i.code, field: i.field, message: i.message })),
    changedContent: derived.changedContent.map((c) => ({
      key: c.key,
      namePt: c.namePt,
      changedAt: c.changedAt ? timestampDate(c.changedAt) : null,
      messages: [...c.messages],
    })),
    hints: derived.hints.map((h) => ({ sourceKey: h.sourceKey, text: h.text })),
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

/**
 * `CharacterStory` → `CharacterStoryVm`, every field. Exported so
 * `character-sheet-source.live.spec.ts` can prove `toCharacterStoryInit`
 * round-trips it unchanged (integrator fix, phase 2b: no field the
 * story form doesn't show may silently disappear on save).
 */
export function toStoryVm(story: GenCharacterStory | undefined): CharacterStoryVm {
  return {
    personality: {
      traits: story?.personality?.traits ?? '',
      ideals: story?.personality?.ideals ?? '',
      bonds: story?.personality?.bonds ?? '',
      flaws: story?.personality?.flaws ?? '',
    },
    appearance: {
      age: story?.appearance?.age ?? '',
      height: story?.appearance?.height ?? '',
      weight: story?.appearance?.weight ?? '',
      eyes: story?.appearance?.eyes ?? '',
      skin: story?.appearance?.skin ?? '',
      hair: story?.appearance?.hair ?? '',
      description: story?.appearance?.description ?? '',
    },
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

/** Exported so `character-sheet-source.live.spec.ts` can test the identity
 * fields (alignment, XP) directly, without going through the whole
 * `getCharacterSheet` RPC round trip. */
export function toCharacterSheetVm(character: Character): CharacterSheetVm {
  const sheetCase = character.sheet?.content.case;
  const sheet: FullSheetVm | BasicSheetVm =
    sheetCase === 'full' && character.derived
      ? toFullSheetVm(character.sheet!.content.value as GenFullSheet, character.derived)
      : sheetCase === 'basic'
        ? toBasicSheetVm(character.sheet!.content.value as GenBasicSheet)
        : EMPTY_BASIC_SHEET_VM;
  // Identity fields the official sheet's top block shows but DerivedSheet
  // does not carry — read straight from the stored FullSheet, not
  // computed (integrator follow-up). `undefined` for a BasicSheet: an NPC
  // has neither.
  const full = sheetCase === 'full' ? (character.sheet!.content.value as GenFullSheet) : undefined;

  return {
    id: character.id,
    campaignId: character.campaignId,
    characterKind: KIND_FROM_GEN[character.kind],
    name: character.name,
    portraitImageId: character.sheet?.content.value?.portraitImageId ?? '',
    state: STATE_FROM_GEN[character.state],
    canEdit: character.canEdit,
    sheetLockedAt: character.sheetLockedAt ? timestampDate(character.sheetLockedAt) : null,
    diedAt: character.diedAt ? timestampDate(character.diedAt) : null,
    revision: character.revision,
    sheet,
    story: toStoryVm(character.story),
    canEditStory: character.canEditStory,
    storyEditingAllowed: character.storyEditingAllowed,
    canToggleStoryEditing: character.canSetStoryEditing,
    canMarkDead: character.canMarkDead,
    canAccessMasterNotes: character.canAccessMasterNotes,
    canApprove: character.canApprove,
    // Character carries no separate "is this caller the master" flag — this
    // one is the unconditional-on-character-state proxy the proto actually
    // offers: true for the master on every character, of any kind or
    // state, unlike can_mark_dead / can_set_story_editing which are also
    // gated on the character being a living player character.
    isMaster: character.canAccessMasterNotes,
    playerDisplayName: character.playerDisplayName || null,
    // The subrace's name already says the race ("Gnomo das Rochas"), as the
    // paper sheet's "Raça" box does; the race alone when there is none.
    raceLabel: character.derived?.subraceNamePt || character.derived?.raceNamePt || '',
    classSummary: character.derived
      ? character.derived.classes.map((c) => `${c.namePt} ${c.level}`).join(' / ')
      : '',
    backgroundLabel: character.derived?.backgroundNamePt ?? '',
    alignmentLabel: full ? ALIGNMENT_LABEL_FROM_GEN[full.alignment] : '',
    experiencePoints: full ? full.experiencePoints : null,
    totalLevel: character.derived?.totalLevel ?? 0,
    nextLevelXp: character.derived?.nextLevelXp ?? 0,
    canLevelUp: character.canLevelUp,
    levelUpReason: LEVEL_UP_REASON[character.levelUpReason] ?? null,
    challengeRating: full ? full.challengeRating : basicRating(character),
    xpValue: full ? full.xpValue : basicXp(character),
  };
}

/**
 * `CharacterSheetSource` over the generated `CharacterService` client
 * (`meurpg.characters.v1`, phase 2). Provided at the route level for
 * `/campaigns/:id/characters/:characterId` — see
 * `character-sheet.routes.ts` — so this client, and the two generated
 * `_pb.ts` files it pulls in, stay out of the app's eager bundle.
 */
@Injectable()
export class CharacterSheetSourceLive implements CharacterSheetSource {
  private readonly client = createClient(CharacterService, inject(CONNECT_TRANSPORT));
  private readonly campaigns = createClient(CampaignService, inject(CONNECT_TRANSPORT));

  async getXpMode(campaignId: string): Promise<CampaignXpMode> {
    const res = await this.campaigns.getCampaign({ campaignId });
    switch (res.campaign?.xpMode) {
      case XpMode.MILESTONES:
        return 'milestones';
      case XpMode.GOLD:
        return 'gold';
      default:
        return 'enemies';
    }
  }

  async getCharacterSheet(campaignId: string, characterId: string): Promise<CharacterSheetVm> {
    const res = await this.client.getCharacter({ campaignId, characterId });
    return toCharacterSheetVm(res.character!);
  }

  async getMasterNotes(campaignId: string, characterId: string): Promise<string> {
    const res = await this.client.getMasterNotes({ campaignId, characterId });
    return res.notes;
  }

  async updateMasterNotes(campaignId: string, characterId: string, notes: string): Promise<void> {
    await this.client.updateMasterNotes({ campaignId, characterId, notes });
  }

  async markCharacterDead(campaignId: string, characterId: string): Promise<CharacterSheetVm> {
    const res = await this.client.markCharacterDead({ campaignId, characterId });
    return toCharacterSheetVm(res.character!);
  }

  async updateCharacterStory(
    campaignId: string,
    characterId: string,
    revision: number,
    story: CharacterStoryVm,
  ): Promise<CharacterSheetVm> {
    const res = await this.client.updateCharacterStory({
      campaignId,
      characterId,
      revision,
      story: toCharacterStoryInit(story),
    });
    return toCharacterSheetVm(res.character!);
  }

  async setStoryEditingAllowed(
    campaignId: string,
    characterId: string,
    allowed: boolean,
  ): Promise<CharacterSheetVm> {
    const res = await this.client.setStoryEditing({ campaignId, characterId, allowed });
    return toCharacterSheetVm(res.character!);
  }

  async approveCharacter(campaignId: string, characterId: string): Promise<CharacterSheetVm> {
    const res = await this.client.approveCharacter({ campaignId, characterId });
    return toCharacterSheetVm(res.character!);
  }

  async rejectCharacter(campaignId: string, characterId: string): Promise<void> {
    await this.client.rejectCharacter({ campaignId, characterId });
  }
}
