import type { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import type { PendingDamage, SpellTargets } from '../../../../../gen/meurpg/play/v1/combat_pb';
import type {
  ActionEconomy,
  MetamagicOption,
  SpellTableOption,
  SlotChoice,
  SpellGrantOption,
} from '../../../../../gen/meurpg/rules/v1/rules_pb';
import type { CombatState } from '../../../../core/combat/combat-state';
import type { MapLayers } from '../../../../core/maps/layers';
import type { Vision } from '../../../../core/maps/vision';
import type { Pool } from '../../../../core/resources/pools';
import type { CombatMapImage } from '../../../../shared/combat-map/combat-map';

/** The battle map an area spell is placed on (a combat with a grid): what the page already draws. */
export interface CastMapData {
  readonly image: CombatMapImage;
  readonly mapName: string;
  readonly columns: number;
  readonly rows: number;
  readonly layers: MapLayers | null;
  /** A player's map with the fog on: what they see. */
  readonly fog: Vision | null;
}

/** What the page hands the cast sheet. */
export interface CastSheetData {
  readonly campaignId: string;
  readonly encounterId: string;
  readonly casterId: string;
  readonly round: number;
  readonly spellKey: string;
  readonly name: string;
  /** 0 for a cantrip. */
  readonly level: number;
  readonly concentration: boolean;
  /** A damage cantrip's dice at the caster's level (`Attack.spellDice`); empty otherwise. */
  readonly cantripDice: string;
  readonly economy: ActionEconomy;
  /** The free slots the spell can be cast with (`SpellOption.slots`). */
  readonly slots: readonly SlotChoice[];
  /** The caster's slots by level and the pact slots, for "1 livre de 4". */
  readonly usage: readonly {
    readonly level: number;
    readonly total: number;
    readonly used: number;
  }[];
  readonly pact: {
    readonly slotLevel: number;
    readonly total: number;
    readonly used: number;
  } | null;
  /** Who it can target, with the distance (`GetTurnOptions.spell_targets`). */
  readonly targets: SpellTargets | undefined;
  /** How many free slots Escudo could still be cast with; `null` without Escudo. */
  readonly shieldFree: number | null;
  /** Escudo's Portuguese name ("Escudo Arcano"), as the spell list says it. */
  readonly shieldName: string;
  /** The grant the spell is cast through (`SpellOption.grant`): no slot unless it allows one, a use of its resource, its own
   * attack bonus and save DC; `null` for a spell of the classes. */
  readonly grant?: SpellGrantOption | null;
  /** The spell reaches only the caster, who rides its bonded steed in this combat (`SpellOption.also_steed`): the check "Estrela também". */
  readonly alsoSteed?: boolean;
  /** The spell attack bonus, for the typed d20's total. */
  readonly attackBonus: number;
  readonly diceMode: DiceMode;
  readonly preference: DicePreference;
  /** Where each answer's combat goes (the page's copy of the combat). */
  readonly state: CombatState;
  /** The master casts it for an NPC: any roll mode, with no need to ask. */
  readonly master?: boolean;
  /** The damage of a cast whose sheet was closed before it was rolled: the
   * sheet opens at the damage with it. */
  readonly resume?: readonly PendingDamage[];
  /** The Metamagic options the sorcerer knows, for this spell (`SpellOption.metamagic_options`); empty for none. */
  readonly metamagic?: readonly MetamagicOption[];
  /** The cast options the character's table features offer for this spell (`SpellOption.table_options`); empty for none. */
  readonly tableOptions?: readonly SpellTableOption[];
  /** The sorcery points left and their maximum, for "Pontos de Feitiçaria: 5 de 5". */
  readonly sorceryPoints?: Pool | null;
  /** The caster has Sculpt Spells and the spell is of evocation (`SpellOption.sculpt_spells`). */
  readonly sculptSpells?: boolean;
  /** The targets the player tapped on the map (C7-04): who is chosen, or how many darts each one has. */
  readonly preset?: {
    readonly chosen: readonly string[];
    readonly dealt: ReadonlyMap<string, number>;
  };
  /** The battle map, in a combat with a grid: an area spell is placed on it. Absent without a map (theatre of the mind),
   * where every area spell keeps the list of who it hits. */
  readonly map?: CastMapData | null;
}
