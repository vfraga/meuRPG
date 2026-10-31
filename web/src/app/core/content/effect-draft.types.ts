import type { TableEffect } from '../../../gen/meurpg/rules/v1/table_content_pb';

/** The fields of `TableEffect`, as plain values: strings for text, arrays for lists, numbers for counts and ranges. */
export interface EffectDraft {
  /** A stable id the editor tracks the row by (never sent): focus and "Mais opções" follow a moved item. */
  id: string;
  type: string;
  /** What was typed in a number field when it is not how the number is written back ("+1", "007", text no number): the field keeps it, the
   * saved value is the number. Keyed by the field's proto name; never sent. */
  typed?: Readonly<Record<string, string>>;
  target: string;
  mode: string;
  value: string;
  when: string;
  /** Written as "against:magic, about:items": split into tags when sent. */
  tags: string;
  proficiency: string;
  level: string;
  roll: string;
  targets: string[];
  sense: string;
  /** The range as typed, in metres (the server stores feet): kept as text so the field never rewrites what is being typed. */
  rangeM: string;
  resource: string;
  max: string;
  recharge: string;
  choice: string;
  count: number;
  from: string[];
  economy: string;
  /** superiority_die: where the die goes, and the ability whose modifier a reduction adds. ranged_attack: "spell" or "weapon". */
  applies: string;
  ability: string;
  spells: string[];
  textPt: string;
  /** ranged_attack's switches: past half and three-quarters cover, the spell range times 2 or 3 (0 is none), no disadvantage at long range. */
  ignoreCover: boolean;
  rangeMultiplier: number;
  noLongRangeDisadvantage: boolean;
  /** resistance: the damage type it leaves out, and when it holds ("rage"). */
  except: string;
  while: string;
  /** grant_action and on_event: what a use spends (a formula), and the most the player may choose to spend. */
  cost: string;
  costMax: string;
  /** What each point spent above the cost adds: dice for the heal, a formula for the temporary hit points. */
  valuePerPoint: string;
  /** Hit points regained, as dice ("1d6", "2d4+2"); the ability's modifier is added when `ability` is set. */
  heal: string;
  /** Temporary hit points, a formula. */
  tempHp: string;
  /** on_event: when it runs ("long_rest"...), the resource it gives back, and the heal as a percentage of the maximum. */
  event: string;
  restore: string;
  healPercentMax: number;
  /** proficiency, level "expertise" on a skill: doubles it only where the character is already proficient. */
  onlyProficient: boolean;
  /** granted_spell: the spell (SRD or the table's), the level it starts at (a formula), the resource each cast spends one of
   * (empty: at will), and the switches. The casting ability is `ability`. */
  spell: string;
  fromLevel: string;
  uses: string;
  alsoWithSlot: boolean;
  ritualOnly: boolean;
  countsAsKnown: boolean;
  /** resistance: the damage types it covers (or "all", "nonmagical_bps") and its kind ("resist", "immune", "vulnerable"). */
  damageTypes: string[];
  kind: string;
  /** condition_immunity: the conditions the character takes none of. roll_mode: the condition its saves are against. */
  conditions: string[];
  againstCondition: string;
  /** extra_damage: the dice (a formula) and sides, the type ("" is the weapon's), weapon kind, frequency, no crit doubling. */
  dice: string;
  diceSides: number;
  damageType: string;
  weapon: string;
  frequency: string;
  noCritDoubling: boolean;
  /** spell_modifier: the spells it fits and its change. */
  appliesTo: string;
  spellChange: string;
  /** state: its name in the entry, the formula of its duration in rounds (empty: until it ends), and how else it ends. */
  stateName: string;
  durationRounds: string;
  endsOn: string[];
  /** weapon_rule: which weapons (and the name of "named"), another ability for the attack and for the damage, bonuses (signed),
   * and a melee reach in metres as typed. pseudo_weapon: its kind in `weapon`, `attackName`, `damageDice`, `damageType`, the
   * ability in `ability`, the reach in `reachM` and the range in `rangeM`. */
  weaponName: string;
  attackAbility: string;
  damageAbility: string;
  attackBonus: number;
  damageBonus: number;
  reachM: string;
  attackName: string;
  damageDice: string;
}

/** The fields a request carries for one effect: the type, and only what that type reads. */
export type EffectInit = Partial<Omit<TableEffect, '$typeName' | '$unknown'>>;
