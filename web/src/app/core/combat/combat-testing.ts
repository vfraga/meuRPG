import { create } from '@bufbuild/protobuf';

import {
  type Creature,
  type CreatureAction,
  CreatureActionClass,
  CreatureActionKind,
  CreatureActionSchema,
  SpellAreaShape,
} from '../../../gen/meurpg/rules/v1/rules_pb';
import { CreaturesClient } from '../creatures/creatures-client';
import {
  type Combatant,
  CombatantKind,
  CombatantSide,
  CombatantState,
  CoverDegree,
  type Encounter,
  EncounterStatus,
  ReactionKind,
  type ReactionWindow,
  ReactionWindowStatus,
  type SpellZone,
  SpellZoneMovement,
  SpellZoneVisibility,
  SunlightScope,
  SunlightTrait,
} from '../../../gen/meurpg/play/v1/combat_pb';

/** A combatant for specs: an NPC unless `kind` says otherwise. */
export function combatant(over: Partial<Combatant> & { id: string; label: string }): Combatant {
  return {
    kind: CombatantKind.NPC,
    mine: false,
    characterId: `char-${over.id}`,
    hidden: false,
    tieUnresolved: false,
    placed: true,
    col: 0,
    row: 0,
    speedFt: 30,
    movementUsedFt: 0,
    movementLeftFt: 30,
    speedDft: 300,
    movementUsedDft: 0,
    movementLeftDft: 300,
    dashed: false,
    actionUsed: false,
    bonusActionUsed: false,
    reactionUsed: false,
    state: CombatantState.UNHURT,
    defeated: false,
    deathSuccesses: 0,
    deathFailures: 0,
    conditions: [],
    conditionNamesPt: [],
    conditionSources: [],
    states: [],
    limitedActions: [],
    concentrationSpell: '',
    armorClassBonus: 0,
    hitPointsMaxBonus: 0,
    deathSaveDue: false,
    turnPartEnded: false,
    side: CombatantSide.ENEMY,
    coverMark: CoverDegree.NONE,
    disengaged: false,
    bestiaryCreatureKey: '',
    challengeRating: '',
    inSunlight: false,
    sunlightTrait: SunlightTrait.UNSPECIFIED,
    ...over,
  } as unknown as Combatant;
}

/** An encounter for specs: ACTIVE, round 2, on a 20 x 14 grid. */
export function encounter(over: Partial<Encounter> = {}): Encounter {
  return {
    id: 'enc',
    name: 'Emboscada na estrada',
    status: EncounterStatus.ACTIVE,
    round: 2,
    mapId: 'map',
    gridColumns: 20,
    gridRows: 14,
    currentCombatantId: '',
    masterTurn: false,
    combatants: [],
    reactionPrompts: [],
    reactionWindows: [],
    turnGroupIds: [],
    npcOnlyGroups: [],
    opportunityOffers: [],
    deaths: [],
    zones: [],
    rollModeRequests: [],
    sunlightOn: false,
    sunlightScope: SunlightScope.UNSPECIFIED,
    sunlightSquares: [],
    revision: 1,
    ...over,
  } as unknown as Encounter;
}

/** A reaction window for specs: open, the first of its group, for the master, with no prompt unless `prompt` says. */
export function reactionWindow(over: Partial<ReactionWindow> & { id: string }): ReactionWindow {
  return {
    kind: ReactionKind.SHIELD,
    status: ReactionWindowStatus.OPEN,
    closedReason: 0,
    groupId: `group-${over.id}`,
    reactorId: '',
    reactorLabel: '',
    reactorIsPlayer: false,
    forYou: true,
    answerNow: true,
    secondStep: false,
    prompt: { case: undefined, value: undefined },
    ...over,
  } as unknown as ReactionWindow;
}

/** A spell zone for specs: Moonbeam as its caster's player (or the master) reads it, a 5 ft cylinder moved by an action; `over` changes any field. */
export function spellZone(
  over: Partial<Omit<SpellZone, 'squares' | 'origin' | 'burningSquares'>> & {
    id: string;
    squares?: { col: number; row: number }[];
    burningSquares?: { col: number; row: number }[];
    origin?: { col: number; row: number };
  },
): SpellZone {
  return {
    spellKey: 'spell:moonbeam',
    spellNamePt: 'Raio Lunar',
    ownerCombatantId: 'bartolo',
    ownerLabel: 'Bartolo',
    shape: SpellAreaShape.CYLINDER,
    sizeFt: 5,
    origin: { col: 8, row: 6 },
    squares: [],
    burningSquares: [],
    wallSide: 0,
    concentration: true,
    roundsLeft: 9,
    movement: SpellZoneMovement.ACTION,
    visibility: SpellZoneVisibility.EVERYONE,
    detailsHidden: false,
    saveAbility: 'con',
    saveAbilityNamePt: '',
    saveDc: 14,
    damageDice: '2d10',
    damageTypeKey: 'radiant',
    damageTypeNamePt: 'radiante',
    halfOnSave: true,
    movesPerTurnFt: 0,
    notes: [],
    occupantIds: [],
    ...over,
  } as unknown as SpellZone;
}

/**
 * The `CreaturesClient` of a spec that renders the NPC card or the rider offers: both read the creature's stat block (the card for
 * its Multiattack and its other actions, the offer for the save's ability). Without this stub the read goes to the network and
 * fails; here it answers `blocks[key]`, and fails for any other key as a stat block that cannot be read does.
 */
export function creaturesProvider(blocks: Readonly<Record<string, Creature>> = {}) {
  const statBlock = async (_campaignId: string, key: string): Promise<Creature> => {
    const block = blocks[key];
    if (!block) {
      throw new Error(`no stat block for ${key}`);
    }
    return block;
  };
  return { provide: CreaturesClient, useValue: { statBlock } };
}

/** An action of a stat block for specs: a save (default) or no roll, with the plan the server would read. */
export function creatureAction(over: Partial<CreatureAction> & { key: string }): CreatureAction {
  return create(CreatureActionSchema, {
    name: over.key.split('#')[1] ?? over.key,
    kind: CreatureActionKind.SAVE,
    planClass: CreatureActionClass.STRUCTURED,
    ...over,
  } as never);
}
