import {
  type AttackRoll,
  type ConcentrationSaveResult,
  type ReactionResult as GenReactionResult,
  type DeathSave,
  type DiceRoll,
  type Encounter,
  EncounterMode,
  type HeldRollOffer,
  type PendingDamage,
  type SpellCast,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { type RollMode } from '../../../gen/meurpg/play/v1/combat_rolls_pb';
import type { Square } from './combat-grid';

/** Who joins a combat: a character, how many copies (NPCs) and whether the
 * copies start hidden (unset: the server's default, hidden for an NPC). */
export interface JoinSpec {
  readonly characterId: string;
  readonly count?: number;
  readonly hidden?: boolean;
}

/** One creature of the bestiary, with how many come and the base name (empty: the creature's Portuguese name). */
export interface MonsterGroupSpec {
  readonly creatureKey: string;
  readonly count: number;
  readonly name?: string;
}

/** The hit points of monsters: the creature's average, or each one rolls its dice (RN-29). */
export type MonsterHp = 'average' | 'rolled';

/** What a start brings besides the party: the monsters of a saved encounter ("Começar este combate", MR-043) and the
 * battle point it was started from. Hit points and hidden apply to every group. */
export interface StartExtras {
  readonly monsters?: readonly MonsterGroupSpec[];
  readonly monsterHp?: MonsterHp;
  readonly monstersHidden?: boolean;
  /** A BATTLE point of the campaign the combat starts from (never with the combat without a map). */
  readonly mapPointId?: string;
  /** "Com mapa" or "Sem mapa (teatro da mente)" (RN-25). Left out, the server reads the table's "combate com mapa" rule
   * (RN-24); THEATRE sends no map point, and the monsters have no squares. */
  readonly mode?: EncounterMode;
}

/** What `AddMonsters` answers: the combat and the ids of the new combatants, in the order of their names. */
export interface AddMonstersResult {
  readonly encounter: Encounter;
  readonly combatantIds: readonly string[];
}

/** How a combatant's d20 comes (`SubmitInitiative`): the app rolls it, or a
 * face typed from a physical die. */
export type InitiativeRoll = { readonly inApp: true } | { readonly face: number };

/** A table die added to the initiative: its key, and its face when the dice are physical. */
export interface InitiativeManeuver {
  readonly key: string;
  readonly face?: number;
}

/** How a d20 of an attack comes: the app rolls it, or a typed face. */
export type AttackDie = InitiativeRoll;

/** Two physical d20 (advantage or disadvantage), in the order they were rolled. */
export interface PairDie {
  readonly faces: readonly number[];
}

/** The mode a d20 is rolled with when it is not the server's suggestion: the reason, and the answered request that allows a better one. */
export interface ModeChoice {
  readonly mode: RollMode;
  readonly reason: string;
  readonly requestId?: string;
}

/** What the player marked in the damage sheet: the extras with Divine Smite's slot, and the sums typed for physical dice. */
export interface DamageChoice {
  readonly extras: readonly {
    readonly key: string;
    readonly slotLevel: number;
    readonly pact: boolean;
  }[];
  readonly typedParts?: readonly { readonly partKey: string; readonly sum: number }[];
}

/** How the damage comes: the app rolls it, or the sum of the physical dice
 * (without the modifier; the server adds it). */
export type DamageDie = { readonly inApp: true } | { readonly sum: number };
/** How the pool of Sono or Leque Cromático comes: rolled by the server, or the sum of the physical dice. */
export type PoolDie = { readonly inApp: true } | { readonly poolSum: number };

/** What an attack answers: the combat, the roll and the damage it opened. */
export interface AttackResult {
  readonly encounter: Encounter;
  readonly roll: AttackRoll;
  readonly pending: PendingDamage | undefined;
  /** The roll waits for a question (the halfling's Lucky about a 1, which comes first, or the attacker's Bardic
   * Inspiration die): the d20 is rolled and the result is held until the player answers (`AnswerHeldRoll`). `roll`
   * then carries only the d20, with no outcome. */
  readonly held?: HeldRollOffer;
}

/** What a move answers: the combat, and whether it stopped short or provoked. */
export interface MoveResult {
  readonly encounter: Encounter;
  readonly stoppedEarly: boolean;
  /** The move stopped before a locked door (RN-26): the page says "A porta está trancada." */
  readonly lockedDoor: boolean;
  readonly provoked: boolean;
}

/** What "Gastar movimento" answers: the combat, and what the combatant has left (tenths of a foot). */
export interface SpendResult {
  readonly encounter: Encounter;
  readonly movementLeftDft: number;
}

/** What a damage call answers: the combat and the damage as it is now. For an
 * area spell's one roll, `cast` has the other damages of the same cast that
 * the roll settled too. */
export interface DamageResult {
  readonly encounter: Encounter;
  readonly pending: PendingDamage;
  readonly cast: readonly PendingDamage[];
}

/** The slot a spell is cast with (`SpellSlot`); `null` for a cantrip. */
export interface SlotRef {
  readonly level: number;
  readonly pact: boolean;
  /** The resource of an innate spell's uses a day, when the spell is cast with one of them and not with a slot. */
  readonly resource?: string;
}

/** One Metamagic option of a cast (`MetamagicChoice`): the creatures it needs (Twinned: the second target, Careful: who
 * passes on its own, Heightened: who has disadvantage). */
export interface MetamagicSpec {
  readonly key: string;
  readonly targetIds: readonly string[];
  readonly carefulIds: readonly string[];
  readonly heightenedId: string;
}

/** One target of a cast; `darts` is Magic Missile's, 0 for any other spell. */
export interface CastTarget {
  readonly combatantId: string;
  readonly darts: number;
}

/** What a cast answers: the combat and what the spell did. */
export interface CastResult {
  readonly encounter: Encounter;
  readonly cast: SpellCast;
  /** A summoning spell: the combatants that joined the combat, in the order of the creatures chosen. */
  readonly summoned: readonly string[];
  /** An area placed on the map: where it landed and the squares it covers, as the caller knows the map. */
  readonly area: { readonly origin: Square | null; readonly squares: readonly Square[] } | null;
  /** The hidden creatures the area hit: the master's only (empty for a player). */
  readonly hiddenHits: readonly string[];
  /** The question the cast opened for the master ("Perguntar a cada vez"): the master's only. */
  readonly pendingRevealId: string;
}

/** Where an area spell is placed: a point for a sphere or a cylinder, a direction for a cone, a line or a cube. */
export type AreaChoice =
  | { readonly origin: Square }
  | { readonly direction: { readonly dx: number; readonly dy: number } };

/** What else a cast may carry: the placed area, and the master's choice on the hidden creatures it hits. */
export interface CastExtras {
  readonly area?: AreaChoice | null;
  /** The master only: whether the hidden creatures the area hits appear to the players. */
  readonly revealHidden?: boolean;
  /** Aprimorar Habilidade only: the ability it is cast for ("str" to "cha"). */
  readonly abilityKey?: string;
  /** Sculpt Spells: the creatures of the area the caster spares. */
  readonly sculptedIds?: readonly string[];
  /** A teleport spell on a map with a grid (Passo Nebuloso): the square the caster lands on. */
  readonly destination?: { readonly col: number; readonly row: number };
  /** The cast options of the character's table features that were ticked (`SpellOptionChoice`). */
  readonly spellOptions?: readonly { readonly key: string; readonly damageTypeKey: string }[];
  /** A granted spell: the source of the grant it is cast through (`SpellGrantOption.source_key`). */
  readonly grantedBy?: string;
}

/** What a summoning spell brings (`SummonChoice`): the option, a content key per creature and, optionally, a name each. */
export interface SummonRequest {
  readonly option: number;
  readonly creatureKeys: readonly string[];
  readonly names?: readonly string[];
}

/** What an initiative answers: the combat, and the question when the d20 was kept for one (the halfling's Lucky about a 1, a Bardic
 * Inspiration die the character holds); the initiative stays unset until it is answered. */
export interface InitiativeResult {
  readonly encounter: Encounter;
  readonly held: HeldRollOffer | undefined;
}

/** What a death save answers: the combat and the save, or, when the d20 was kept for a question (the halfling's Lucky about a
 * 1, a Bardic Inspiration die), the question and no save yet. */
export interface DeathSaveResult {
  readonly encounter: Encounter;
  readonly save: DeathSave | undefined;
  readonly held: HeldRollOffer | undefined;
}

/** What a standard or feature action answers: the combat, the roll of an
 * action that rolls (Retomar o Fôlego) and the hit points it healed. */
export interface ActionResult {
  readonly encounter: Encounter;
  readonly roll: DiceRoll | undefined;
  readonly healed: number | undefined;
  /** A table feature's action: the points it spent and the temporary hit points the target gained. */
  readonly spent: number | undefined;
  readonly tempHpGained: number | undefined;
}

/** What the master changes in "Condições…": the new set of conditions (unset
 * leaves them) and whether the concentration ends. */
export interface ConditionChange {
  readonly keys?: readonly string[];
  readonly endConcentration?: boolean;
}

/** How a window is answered (`AnswerReaction`): "Usar ..." with what it needs, or "Deixar passar". */
export interface ReactionAnswer {
  readonly use: boolean;
  /** The slot of a spell reaction (`pact` for the pact slot). */
  readonly slot?: SlotRef;
  /** Hellish Rebuke paid with the Infernal Legacy. */
  readonly useRacial?: boolean;
  /** Feather Fall: the falling creatures to save. */
  readonly creatureIds?: readonly string[];
  /** The die the answer needs, when it needs one: the app rolls it, or the face typed from a physical die. */
  readonly die?: { readonly inApp: true } | { readonly typed: number };
  /** A maneuver reduction: the option of the prompt the player uses. */
  readonly maneuverKey?: string;
}

/** The answer to a Legendary Resistance window: the creatures whose failed save becomes a success (none: leave them as rolled). */
export interface LegendaryResistanceAnswer {
  readonly creatureIds: readonly string[];
}

/** What `AnswerReaction` answers: the combat and what the answer did. */
export interface AnswerResult {
  readonly encounter: Encounter;
  readonly result: GenReactionResult | undefined;
  /** The question the d20 of a Counterspell's check waits on (the halfling's Lucky about a 1, a Bardic Inspiration die): the
   * window stays open and nothing is spent until it is answered. */
  readonly held: HeldRollOffer | undefined;
}

/** What an effect changes in an attack roll: the faces of the d4 it adds from physical dice, and the extra action it gives. */
export interface AttackEffects {
  readonly extraDieFaces?: readonly number[];
  readonly useExtraAction?: boolean;
  /** The weapon-attack table die ticked (`RollAttackRequest.maneuver_key`); empty for none. */
  readonly maneuverKey?: string;
  /** The master only: the key of the legendary option of kind "attack" the attack is made with (`LegendaryOption.key`). */
  readonly legendaryOption?: string;
}

/** How a concentration window is settled: the d20 in the app, a typed face, "Deixar o mestre rolar por mim", or the
 * master keeping the concentration. */
export type ConcentrationAnswer =
  | {
      readonly kind: 'app';
      readonly maneuverKey?: string;
      /** The master's choice: if the save fails, Legendary Resistance turns it into a success. */
      readonly useLegendaryResistance?: boolean;
    }
  | {
      readonly kind: 'typed';
      readonly face: number;
      /** The second d20, when the save has advantage or disadvantage. */
      readonly second?: number;
      readonly extra?: readonly number[];
      /** A table die added to the roll: its face is the last of `extra`. */
      readonly maneuverKey?: string;
      readonly useLegendaryResistance?: boolean;
    }
  | { readonly kind: 'hand' }
  | { readonly kind: 'keep' };

/** What `ResolveConcentrationSave` answers: the combat and the save (unset when handed to the master or kept). */
export interface ConcentrationOutcome {
  readonly encounter: Encounter;
  readonly result: ConcentrationSaveResult | undefined;
  /** The d20 was rolled and kept and the save waits for a question (the halfling's Lucky about a 1, which comes first,
   * or the Bardic Inspiration die): the player decides before the result is told (`AnswerHeldRoll`). */
  readonly held?: HeldRollOffer;
}

/** One adjustment of an NPC's hit points ("Dano/Cura"): at most one of the
 * three, and, apart or together, new temporary hit points. */
export interface HpAdjust {
  readonly change?: { readonly kind: 'damage' | 'heal' | 'exact'; readonly value: number };
  readonly temporary?: number;
}

/**
 * Thin wrapper around the generated `CombatService` client (MR-013), in the
 * same shape as `MapsClient`. `providedIn: 'root'`, and only lazy code
 * imports it, so the generated combat code stays out of the initial bundle.
 * Every write sends an idempotency key made for that call: a double tap
 * never runs twice, and a call the app repeats on purpose is a new action.
 * Callers map errors to Portuguese with `combat-errors.ts`.
 */
