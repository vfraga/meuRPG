import { timestampDate } from '@bufbuild/protobuf/wkt';

import {
  type Campaign,
  DiceMode,
  type GetTableRulesResponse,
  HitPointsRule,
  XpMode,
} from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  AbilityMethod as GenAbilityMethod,
  type AbilityOrigin as GenAbilityOrigin,
  type AbilityRolls as GenAbilityRolls,
  FullSheet as GenFullSheet,
} from '../../../gen/meurpg/characters/v1/characters_pb';
import {
  type Content,
  NamedKeyKind,
  SpellPreparation as GenSpellPreparation,
  WeaponCategory,
} from '../../../gen/meurpg/rules/v1/rules_pb';
import { ABILITY_FROM_GEN } from '../../core/characters/ability-gen';
import { isTableKey } from '../../core/content/catalog';
import {
  AbilityMethodKey,
  AbilityRollsVm,
  AbilityTableVm,
  CharacterForEdit,
  FeatCatalogVm,
  RulesCatalogVm,
  SpellPreparation,
  SubclassOptionVm,
  SubraceOptionVm,
} from './character-editor.types';

const ABILITY_METHOD_FROM_GEN: Partial<Record<GenAbilityMethod, AbilityMethodKey>> = {
  [GenAbilityMethod.STANDARD_ARRAY]: 'standard_array',
  [GenAbilityMethod.POINT_BUY]: 'point_buy',
  [GenAbilityMethod.ROLLED_4D6]: 'rolled_4d6',
  [GenAbilityMethod.TYPED]: 'typed',
};

export function abilityOriginFromGen(
  origin: GenAbilityOrigin | undefined,
): CharacterForEdit['abilityOrigin'] {
  const method = origin ? ABILITY_METHOD_FROM_GEN[origin.method] : undefined;
  if (!origin || !method) {
    return null;
  }
  return {
    method,
    rolls:
      method === 'rolled_4d6'
        ? {
            sets: origin.rolls.map((s) => ({ dice: [...s.dice], total: s.total })),
            typed: origin.typed,
            rolledAt: null,
          }
        : null,
  };
}

/** The prepared leveled spells of the derived sheet that the sheet's own lists do not name: granted by a subclass. */
export function grantedSpellKeys(
  derived:
    | { spells: readonly { spell?: { key: string; level: number }; prepared: boolean }[] }
    | undefined,
  full: GenFullSheet | undefined,
): string[] {
  if (!derived || !full) {
    return [];
  }
  const own = new Set([...full.cantripKeys, ...full.knownSpellKeys, ...full.preparedSpellKeys]);
  return derived.spells
    .filter((s) => s.prepared && s.spell && s.spell.level > 0 && !own.has(s.spell.key))
    .map((s) => s.spell!.key);
}

export function rollsFromGen(rolls: GenAbilityRolls): AbilityRollsVm {
  return {
    sets: rolls.sets.map((s) => ({ dice: [...s.dice], total: s.total })),
    typed: rolls.typed,
    rolledAt: rolls.rolledAt ? timestampDate(rolls.rolledAt) : null,
  };
}

const PREPARATION_FROM_GEN: Record<GenSpellPreparation, SpellPreparation | null> = {
  [GenSpellPreparation.UNSPECIFIED]: null,
  [GenSpellPreparation.KNOWN]: 'known',
  [GenSpellPreparation.PREPARED]: 'prepared',
  [GenSpellPreparation.SPELLBOOK]: 'spellbook',
};

/** The feats' issues of a derived sheet (`full.feat_keys[i]`), by the key of the feat each one is about. */
export function featIssuesOf(
  issues: readonly { field: string; message: string }[],
  featKeys: readonly string[],
): Record<string, string> {
  const out: Record<string, string> = {};
  for (const issue of issues) {
    const m = /^full\.feat_keys\[(\d+)\]$/.exec(issue.field);
    const key = m ? featKeys[Number(m[1])] : undefined;
    if (key) {
      out[key] = issue.message;
    }
  }
  return out;
}

type GenContent = Content;

function subracesOf(content: GenContent): Map<string, SubraceOptionVm[]> {
  const byRace = new Map<string, SubraceOptionVm[]>();
  for (const sr of content.subraces) {
    const list = byRace.get(sr.raceKey) ?? [];
    list.push({
      key: sr.key,
      namePt: sr.namePt,
      constitutionBonus: sr.abilityBonuses?.constitution ?? 0,
      skillKeys: sr.skillKeys,
      replacesAbilityBonuses: sr.replacesAbilityBonuses,
      replacedSkillKeys: sr.replacedSkillKeys,
      fromTable: isTableKey(sr.key),
      archived: sr.archived,
      off: sr.off,
    });
    byRace.set(sr.raceKey, list);
  }
  return byRace;
}

function subclassesOf(content: GenContent): Map<string, SubclassOptionVm[]> {
  const byClass = new Map<string, SubclassOptionVm[]>();
  for (const sc of content.subclasses) {
    const list = byClass.get(sc.classKey) ?? [];
    const casting = sc.spellcasting;
    list.push({
      key: sc.key,
      namePt: sc.namePt,
      fromTable: isTableKey(sc.key),
      archived: sc.archived,
      off: sc.off,
      alwaysPrepared: sc.alwaysPrepared.map((a) => ({
        spellKey: a.spellKey,
        classLevel: a.classLevel,
      })),
      casting: casting
        ? {
            preparation: PREPARATION_FROM_GEN[casting.preparation],
            listClassKey: casting.listClassKey,
            firstLevel: casting.firstLevel,
            maxSpellLevelByLevel: casting.maxSpellLevelByLevel,
          }
        : null,
    });
    byClass.set(sc.classKey, list);
  }
  return byClass;
}

function racesOf(content: GenContent): RulesCatalogVm['races'] {
  const subraces = subracesOf(content);
  return content.races.map((r) => ({
    key: r.key,
    namePt: r.namePt,
    constitutionBonus: r.abilityBonuses?.constitution ?? 0,
    subraces: subraces.get(r.key) ?? [],
    choiceBonuses: r.choiceBonuses,
    ...(r.choiceBonusSets.length > 0
      ? { choiceBonusSets: r.choiceBonusSets.map((s) => s.bonuses) }
      : {}),
    ...(r.sizes.length > 0 ? { sizes: r.sizes, size: r.size } : {}),
    skillKeys: r.skillKeys,
    fromTable: isTableKey(r.key),
    archived: r.archived,
    off: r.off,
  }));
}

function classesOf(content: GenContent): RulesCatalogVm['classes'] {
  const subclasses = subclassesOf(content);
  return content.classes.map((c) => ({
    key: c.key,
    namePt: c.namePt,
    hitDie: c.hitDie,
    isCaster: !!c.spellcasting,
    preparation: c.spellcasting ? PREPARATION_FROM_GEN[c.spellcasting.preparation] : null,
    subclasses: subclasses.get(c.key) ?? [],
    subclassLevel: c.subclassLevel,
    spellcastingFirstLevel: c.spellcasting?.firstLevel ?? 0,
    maxSpellLevelByLevel: c.spellcasting?.maxSpellLevelByLevel ?? [],
    fromTable: isTableKey(c.key),
    archived: c.archived,
    off: c.off,
    skillChoose: c.skillChoice?.count ?? 0,
    savingThrows: c.savingThrows.map((a) => ABILITY_FROM_GEN[a]),
    // A table class may reuse another class's list ("Guardião do Vale" casts from the druid's).
    spellListClassKey: c.spellcasting ? c.spellcasting.listClassKey || c.key : '',
  }));
}

/** The "Outro" background's tools and languages, as the catalog names them (a vehicle or a kit counts as a tool). */
function toolsAndLanguagesOf(content: GenContent): RulesCatalogVm['toolsAndLanguages'] {
  return [
    ...content.proficiencies
      .filter((p) => p.kind === NamedKeyKind.TOOL || p.kind === NamedKeyKind.OTHER)
      .map((p) => ({ key: p.key, namePt: p.namePt, kind: 'tool' as const })),
    ...content.languages.map((l) => ({
      key: l.key,
      namePt: l.namePt,
      kind: 'language' as const,
    })),
  ];
}

const XP_MODE_FROM_GEN: Partial<Record<XpMode, NonNullable<RulesCatalogVm['xpMode']>>> = {
  [XpMode.ENEMIES]: 'enemies',
  [XpMode.GOLD]: 'gold',
  [XpMode.MILESTONES]: 'milestones',
};

/** What the campaign read says about the viewer: whether they are the master, and the way the table earns XP. */
export interface CampaignRead {
  readonly viewerIsMaster: boolean;
  readonly xpMode: XpMode | undefined;
}

/** The catalog the editor picks from, out of the content the server lists. */
export function catalogFromGen(
  content: GenContent,
  campaign: CampaignRead,
  feats: FeatCatalogVm | undefined,
): RulesCatalogVm {
  return {
    races: racesOf(content),
    classes: classesOf(content),
    backgrounds: content.backgrounds.map((b) => ({
      key: b.key,
      namePt: b.namePt,
      fromTable: isTableKey(b.key),
      archived: b.archived,
      off: b.off,
      equipmentPt: b.equipmentPt,
      skillKeys: b.skillKeys,
    })),
    skills: content.skills.map((s) => ({
      key: s.key,
      namePt: s.namePt,
      ability: ABILITY_FROM_GEN[s.ability],
    })),
    // Body armor only, as the proto's own comment says — a shield is the
    // sheet's separate yes/no.
    armor: content.armor.map((a) => ({ key: a.key, namePt: a.namePt })),
    weapons: content.weapons.map((w) => ({
      key: w.key,
      namePt: w.namePt,
      fromTable: isTableKey(w.key),
      archived: w.archived,
      off: w.off,
      category: w.category === WeaponCategory.MARTIAL ? ('martial' as const) : ('simple' as const),
      damage: w.damage,
    })),
    spells: content.spells.map((sp) => ({
      key: sp.key,
      namePt: sp.namePt,
      level: sp.level,
      classKeys: sp.classKeys,
      fromTable: isTableKey(sp.key),
      archived: sp.archived,
      off: sp.off,
    })),
    toolsAndLanguages: toolsAndLanguagesOf(content),
    viewerIsMaster: campaign.viewerIsMaster,
    challengeRatings: content.challengeRatings.map((c) => ({ rating: c.rating, xp: c.xp })),
    xpMode: campaign.xpMode === undefined ? undefined : XP_MODE_FROM_GEN[campaign.xpMode],
    levelXp: content.levelXp,
    ...(feats ? { feats } : {}),
  };
}

const HIT_POINTS_RULE_FROM_GEN: Partial<Record<HitPointsRule, AbilityTableVm['hitPoints']>> = {
  [HitPointsRule.ROLL]: 'roll',
  [HitPointsRule.AVERAGE]: 'average',
};

/** The table's ways of making scores, for a player making a new sheet (RN-24). */
export function abilityTableFromGen(
  campaign: Campaign,
  rules: GetTableRulesResponse,
  physical: boolean,
  stored: GenAbilityRolls | undefined,
): AbilityTableVm {
  const methods = rules.rules?.abilityMethods;
  const hitPointsRule = rules.rules?.hitPoints;
  return {
    standardArray: methods?.standardArray ?? true,
    pointBuy: methods?.pointBuy ?? true,
    rolled4d6: methods?.rolled4d6 ?? true,
    typed: methods?.typed ?? true,
    standardValues: rules.standardArray,
    pointBuyCosts: rules.pointBuyCosts,
    pointBuyMinScore: rules.pointBuyMinScore,
    pointBuyBudget: rules.pointBuyBudget,
    typedMin: rules.typedMinScore,
    typedMax: rules.typedMaxScore,
    hitPoints:
      HIT_POINTS_RULE_FROM_GEN[hitPointsRule ?? HitPointsRule.UNSPECIFIED] ?? 'player_chooses',
    physicalDice: physical,
    diceForced: campaign.diceMode !== DiceMode.PLAYERS_CHOOSE,
    rolls: stored ? rollsFromGen(stored) : null,
  };
}
