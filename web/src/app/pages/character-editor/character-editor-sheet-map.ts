import {
  Alignment as GenAlignment,
  BasicSheet as GenBasicSheet,
  CuttingWordsAsk,
  CustomBackground,
  FullSheet as GenFullSheet,
  HitPointsMethod as GenHitPointsMethod,
  Item,
  ItemBonus,
  WeaponMark,
} from '../../../gen/meurpg/characters/v1/characters_pb';
import { damageTypeFromGen, damageTypeToGen } from '../../core/characters/damage-type-gen';
import {
  AlignmentKey,
  BasicCharacterFormValue,
  CharacterFormValue,
  CuttingWordsAskKey,
  HitPointsMethod,
} from './character-editor.types';

/** An ability score a sheet leaves out reads as the default 10. */
const DEFAULT_SCORE = 10;

/** The content key of the shield on a sheet (the key of its carried state and of its magic bonus). */
const SHIELD_KEY = 'equipment:shield';

/** "Só em ataques" is the default the server reads from an unset field, so the form sends the field unset for it:
 * a sheet that never had the setting is not given one. */
const CUTTING_WORDS_ASK_TO_GEN: Record<CuttingWordsAskKey, CuttingWordsAsk> = {
  all: CuttingWordsAsk.ALL,
  'only-attacks': CuttingWordsAsk.UNSPECIFIED,
  never: CuttingWordsAsk.NEVER,
};

/** Unspecified is "só em ataques", the server's default. */
function cuttingWordsAskFromGen(ask: CuttingWordsAsk): CuttingWordsAskKey {
  switch (ask) {
    case CuttingWordsAsk.ALL:
      return 'all';
    case CuttingWordsAsk.NEVER:
      return 'never';
    default:
      return 'only-attacks';
  }
}

const ALIGNMENT_TO_GEN: Record<AlignmentKey, GenAlignment> = {
  '': GenAlignment.UNSPECIFIED,
  lawful_good: GenAlignment.LAWFUL_GOOD,
  neutral_good: GenAlignment.NEUTRAL_GOOD,
  chaotic_good: GenAlignment.CHAOTIC_GOOD,
  lawful_neutral: GenAlignment.LAWFUL_NEUTRAL,
  neutral: GenAlignment.NEUTRAL,
  chaotic_neutral: GenAlignment.CHAOTIC_NEUTRAL,
  lawful_evil: GenAlignment.LAWFUL_EVIL,
  neutral_evil: GenAlignment.NEUTRAL_EVIL,
  chaotic_evil: GenAlignment.CHAOTIC_EVIL,
};

const ALIGNMENT_FROM_GEN: Record<GenAlignment, AlignmentKey> = {
  [GenAlignment.UNSPECIFIED]: '',
  [GenAlignment.LAWFUL_GOOD]: 'lawful_good',
  [GenAlignment.NEUTRAL_GOOD]: 'neutral_good',
  [GenAlignment.CHAOTIC_GOOD]: 'chaotic_good',
  [GenAlignment.LAWFUL_NEUTRAL]: 'lawful_neutral',
  [GenAlignment.NEUTRAL]: 'neutral',
  [GenAlignment.CHAOTIC_NEUTRAL]: 'chaotic_neutral',
  [GenAlignment.LAWFUL_EVIL]: 'lawful_evil',
  [GenAlignment.NEUTRAL_EVIL]: 'neutral_evil',
  [GenAlignment.CHAOTIC_EVIL]: 'chaotic_evil',
};

const HIT_POINTS_METHOD_TO_GEN: Record<HitPointsMethod, GenHitPointsMethod> = {
  average: GenHitPointsMethod.AVERAGE,
  rolled: GenHitPointsMethod.ROLLED,
};

const HIT_POINTS_METHOD_FROM_GEN: Record<GenHitPointsMethod, HitPointsMethod> = {
  [GenHitPointsMethod.UNSPECIFIED]: 'average',
  [GenHitPointsMethod.AVERAGE]: 'average',
  [GenHitPointsMethod.ROLLED]: 'rolled',
};

function linesOf(text: string): string[] {
  return text
    .split('\n')
    .map((line) => line.trim())
    .filter((line) => line.length > 0);
}

/** The MVP editor keeps equipment as one line of free text per item — this
 * is the one place that round-trips a line through `Item.quantity` (shown
 * as "Nome (xN)"), so a saved sheet's quantities survive re-opening the
 * editor. */
function itemsFromLines(text: string): Item[] {
  return linesOf(text).map((line) => {
    const match = /^(.*)\s\(x(\d+)\)$/.exec(line);
    return match ? { name: match[1], quantity: Number(match[2]) } : { name: line, quantity: 1 };
  }) as Item[];
}

function lineFromItem(item: { name: string; quantity: number }): string {
  return item.quantity > 1 ? `${item.name} (x${item.quantity})` : item.name;
}

/**
 * Builds the wire `FullSheet` fields this form edits, from the form value
 * alone — used as-is for `CreateCharacter` (nothing to preserve yet) and
 * as the "what changed" half of `mergeFullSheetInit` for `UpdateCharacter`.
 *
 * Exported for `character-editor-source.live.spec.ts`'s round-trip test.
 */
export function toFullSheetInit(v: CharacterFormValue) {
  const subclassOf = (subclassKey: string, customName: string) =>
    subclassKey
      ? { case: 'subclassKey' as const, value: subclassKey }
      : customName
        ? { case: 'customSubclassName' as const, value: customName }
        : { case: undefined, value: undefined };
  // The first class gives the saving throws; the others follow in the order the blocks are on screen.
  const classes = v.className
    ? [
        {
          classKey: v.className,
          level: v.level,
          subclass: subclassOf(v.subclassName, v.customSubclassName),
        },
        ...v.extraClasses
          .filter((c) => c.classKey)
          .map((c) => ({
            classKey: c.classKey,
            level: c.level,
            subclass: subclassOf(c.subclassKey, c.customSubclassName),
          })),
      ]
    : [];

  const background =
    v.background === 'custom'
      ? {
          case: 'customBackground' as const,
          value: {
            name: v.customBackgroundName,
            skillKeys: v.customBackgroundSkills ?? [],
            proficiencyKeys: v.customBackgroundProficiencies,
            featureName: v.customBackgroundFeatureName,
            featureText: v.customBackgroundFeatureText,
            equipment: v.customBackgroundEquipment,
          } satisfies Partial<CustomBackground>,
        }
      : { case: 'backgroundKey' as const, value: v.background };

  return {
    baseScores: {
      strength: v.abilities.str,
      dexterity: v.abilities.dex,
      constitution: v.abilities.con,
      intelligence: v.abilities.int,
      wisdom: v.abilities.wis,
      charisma: v.abilities.cha,
    },
    raceKey: v.race,
    subraceKey: v.subrace,
    size: v.raceSize ?? '',
    classes,
    background,
    skillProficiencyKeys: v.skillProficiencies,
    expertiseSkillKeys: v.expertiseSkillKeys,
    // The master's editor sends the feats; for anyone else they are left out and the save keeps the stored ones.
    ...(v.featKeys ? { featKeys: v.featKeys, featSlots: v.featSlots ?? {} } : {}),
    extraAbilityBonuses: {
      strength: v.extraAbilityBonuses.str,
      dexterity: v.extraAbilityBonuses.dex,
      constitution: v.extraAbilityBonuses.con,
      intelligence: v.extraAbilityBonuses.int,
      wisdom: v.extraAbilityBonuses.wis,
      charisma: v.extraAbilityBonuses.cha,
    },
    hitPoints: {
      method: HIT_POINTS_METHOD_TO_GEN[v.hitPointsMethod],
      rolls: v.hitPointsMethod === 'rolled' ? v.hitPointsRolls : [],
    },
    armorKey: v.armor,
    shield: v.shield,
    weaponKeys: v.weapons,
    // The form has no UI for the marks (silvered, adamantine, magical): a new sheet has none, and a save of a loaded
    // one keeps the marks of the weapons still carried (`mergeFullSheetInit`).
    weaponMarks: [] as WeaponMark[],
    itemBonuses: [] as ItemBonus[],
    cantripKeys: v.cantrips,
    knownSpellKeys: v.spellsKnown,
    preparedSpellKeys: v.spellsPrepared,
    equipment: itemsFromLines(v.equipmentText),
    languages: linesOf(v.languagesText),
    toolProficiencies: linesOf(v.toolProficienciesText),
    experiencePoints: v.experiencePoints,
    challengeRating: v.challengeRating,
    xpValue: v.xpValue,
    portraitImageId: v.portraitImageId,
    alignment: ALIGNMENT_TO_GEN[v.alignment],
    customFeaturesText: v.customFeaturesText,
    featureChoiceKeys: v.featureChoiceKeys,
    featureChoiceText: v.featureChoiceText,
    cuttingWordsAsk: CUTTING_WORDS_ASK_TO_GEN[v.cuttingWordsAsk],
  };
}

/**
 * The no-data-loss fix (integrator review, phase 2b): starts from the
 * `FullSheet` `loadCharacterForEdit` last read for this character and
 * overwrites only the fields `toFullSheetInit` sets from the form — so
 * `coins`, `feature_choice_keys`, and any class beyond the first
 * (multiclassing; this editor only ever edits one) survive a save
 * unchanged instead of silently disappearing. On `CreateCharacter` there
 * is nothing loaded yet, so `original` is `undefined` and this is exactly
 * `toFullSheetInit(v)`.
 *
 * Exported for `character-editor-source.live.spec.ts`'s round-trip test.
 */
export function mergeFullSheetInit(original: GenFullSheet | undefined, v: CharacterFormValue) {
  if (!original) {
    return toFullSheetInit(v);
  }
  // Drop the message's own `$typeName` brand: the result is a plain
  // MessageInitShape for the RPC call, like `toFullSheetInit`'s own
  // return value, not a re-branded `FullSheet` instance.
  const { $typeName: _typeName, ...rest } = original;
  const init = toFullSheetInit(v);
  // An explicit "só em ataques" the sheet already had stays as it was read.
  if (cuttingWordsAskFromGen(original.cuttingWordsAsk) === v.cuttingWordsAsk) {
    init.cuttingWordsAsk = original.cuttingWordsAsk;
  }
  // The form edits every field of a custom background now; this only keeps a field a later
  // version of the message adds, so a save never wipes what the editor does not know.
  if (
    init.background.case === 'customBackground' &&
    original.background.case === 'customBackground'
  ) {
    const { $typeName: _bg, ...kept } = original.background.value;
    init.background = {
      case: 'customBackground',
      value: { ...kept, ...init.background.value },
    };
  }
  // The server refuses a mark whose weapon is not in weapon_keys, so a weapon the player removed takes its mark along.
  const weaponMarks = original.weaponMarks.filter((m) => init.weaponKeys.includes(m.weaponKey));
  // The same for a magic bonus: the form has no UI for it, and a piece the player removed takes its bonus along.
  const onSheet = (key: string) =>
    key === init.armorKey ||
    (key === SHIELD_KEY && init.shield === true) ||
    init.weaponKeys.includes(key);
  const itemBonuses = original.itemBonuses.filter((b) => onSheet(b.key));
  return { ...rest, ...init, weaponMarks, itemBonuses };
}

/** Exported for `character-editor-source.live.spec.ts`. */
export function toBasicSheetInit(v: BasicCharacterFormValue) {
  return {
    hitPointsMax: v.hitPointsMax,
    armorClass: v.armorClass,
    speedFt: v.speedFt,
    initiativeBonus: v.initiativeBonus,
    attacks: v.attacks.map((a) => ({
      name: a.name,
      attackBonus: a.attackBonus,
      damageDiceCount: a.damageDiceCount,
      damageDiceSides: a.damageDiceSides,
      damageBonus: a.damageBonus,
      damageType: damageTypeToGen(a.damageType),
      rangeFt: a.rangeFt,
    })),
    // The deprecated fields go back as read: the server drops them when
    // there are attacks and keeps the old text otherwise.
    attackBonus: v.legacyAttackBonus,
    damage: v.legacyDamage,
    description: v.description,
    // What the minion gives when defeated (E7-11) and its portrait (MR-031):
    // without these the save would wipe what the server holds.
    challengeRating: v.challengeRating,
    xpValue: v.xpValue,
    portraitImageId: v.portraitImageId,
    // The size set elsewhere (the combat reads it) survives the save.
    size: v.size,
  };
}

type GenSubclass = GenFullSheet['classes'][number]['subclass'];

function subclassKeyOf(sub: GenSubclass | undefined): string {
  return sub?.case === 'subclassKey' ? sub.value : '';
}

function customSubclassOf(sub: GenSubclass | undefined): string {
  return sub?.case === 'customSubclassName' ? sub.value : '';
}

/** The first class is the form's own fields, the others are the extra blocks. */
function classFieldsOf(full: GenFullSheet) {
  const first = full.classes[0];
  return {
    className: first?.classKey ?? '',
    level: first?.level ?? 1,
    subclassName: subclassKeyOf(first?.subclass),
    customSubclassName: customSubclassOf(first?.subclass),
    extraClasses: full.classes.slice(1).map((c) => ({
      classKey: c.classKey,
      level: c.level,
      subclassKey: subclassKeyOf(c.subclass),
      customSubclassName: customSubclassOf(c.subclass),
    })),
  };
}

/** The "Outro" background's two granted skills: a pair, or none while the sheet has fewer or more. */
function skillPairOf(skills: readonly string[] | undefined): [string, string] | null {
  return skills?.length === 2 ? [skills[0], skills[1]] : null;
}

function backgroundFieldsOf(background: GenFullSheet['background']) {
  const custom = background.case === 'customBackground' ? background.value : undefined;
  return {
    background: background.case === 'customBackground' ? 'custom' : (background.value ?? ''),
    customBackgroundName: custom?.name ?? '',
    customBackgroundSkills: skillPairOf(custom?.skillKeys),
    customBackgroundProficiencies: custom ? [...custom.proficiencyKeys] : [],
    customBackgroundFeatureName: custom?.featureName ?? '',
    customBackgroundFeatureText: custom?.featureText ?? '',
    customBackgroundEquipment: custom?.equipment ?? '',
  };
}

/** The six scores of a message, `fallback` for one the message leaves out. */
function scoresOf(scores: GenFullSheet['baseScores'], fallback: number) {
  return {
    str: scores?.strength ?? fallback,
    dex: scores?.dexterity ?? fallback,
    con: scores?.constitution ?? fallback,
    int: scores?.intelligence ?? fallback,
    wis: scores?.wisdom ?? fallback,
    cha: scores?.charisma ?? fallback,
  };
}

/** Exported for `character-editor-source.live.spec.ts`'s round-trip test. */
export function toFormFullSheet(name: string, full: GenFullSheet): CharacterFormValue {
  return {
    name,
    race: full.raceKey,
    subrace: full.subraceKey,
    raceSize: full.size,
    ...classFieldsOf(full),
    ...backgroundFieldsOf(full.background),
    skillProficiencies: full.skillProficiencyKeys,
    expertiseSkillKeys: full.expertiseSkillKeys,
    abilities: scoresOf(full.baseScores, DEFAULT_SCORE),
    featKeys: full.featKeys,
    featSlots: { ...full.featSlots },
    extraAbilityBonuses: scoresOf(full.extraAbilityBonuses, 0),
    hitPointsMethod:
      HIT_POINTS_METHOD_FROM_GEN[full.hitPoints?.method ?? GenHitPointsMethod.AVERAGE],
    hitPointsRolls: full.hitPoints?.rolls ?? [],
    // Recomputed by the component from the catalog once loaded — see
    // `CharacterEditor.isCaster`; not read back from the wire.
    isCaster: false,
    cantrips: full.cantripKeys,
    spellsKnown: full.knownSpellKeys,
    spellsPrepared: full.preparedSpellKeys,
    armor: full.armorKey,
    shield: full.shield,
    weapons: full.weaponKeys,
    equipmentText: full.equipment.map(lineFromItem).join('\n'),
    languagesText: full.languages.join('\n'),
    toolProficienciesText: full.toolProficiencies.join('\n'),
    experiencePoints: full.experiencePoints,
    challengeRating: full.challengeRating,
    xpValue: full.xpValue,
    portraitImageId: full.portraitImageId,
    alignment: ALIGNMENT_FROM_GEN[full.alignment],
    customFeaturesText: full.customFeaturesText,
    featureChoiceKeys: [...full.featureChoiceKeys],
    featureChoiceText: { ...full.featureChoiceText },
    cuttingWordsAsk: cuttingWordsAskFromGen(full.cuttingWordsAsk),
  };
}

/** Exported for `character-editor-source.live.spec.ts`. */
export function toFormBasicSheet(name: string, basic: GenBasicSheet): BasicCharacterFormValue {
  return {
    name,
    hitPointsMax: basic.hitPointsMax,
    armorClass: basic.armorClass,
    speedFt: basic.speedFt,
    initiativeBonus: basic.initiativeBonus,
    attacks: basic.attacks.map((a) => ({
      name: a.name,
      attackBonus: a.attackBonus,
      damageDiceCount: a.damageDiceCount,
      damageDiceSides: a.damageDiceSides,
      damageBonus: a.damageBonus,
      damageType: damageTypeFromGen(a.damageType),
      rangeFt: a.rangeFt,
    })),
    legacyDamage: basic.damage,
    legacyAttackBonus: basic.attackBonus,
    description: basic.description,
    challengeRating: basic.challengeRating,
    xpValue: basic.xpValue,
    portraitImageId: basic.portraitImageId,
    size: basic.size,
  };
}
