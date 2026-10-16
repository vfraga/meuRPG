import type { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  AreaPlacement,
  type Combatant,
  type Encounter,
  type GetTurnOptionsResponse,
  type PendingDamage,
  type SpellTargets,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  ActionEconomy,
  type Attack,
  AttackKind,
  type SpellOption,
  type TurnOptions,
} from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { defaultSlot, slotRows } from '../../../../core/combat/cast-flow';
import type { CombatState } from '../../../../core/combat/combat-state';
import type { VitalsVm } from '../../../../core/play/live-session.types';
import { SORCERY_POINTS_RESOURCE, poolOf } from '../../../../core/resources/pools';
import type { MasterAreaCastData } from '../area-cast/master-area-cast';
import type { CastMapData, CastSheetData } from './cast-sheet';

/** The save-DC part of a spell attack bonus: the DC is 8 + proficiency + modifier, so the bonus is the DC minus 8. */
const SAVE_DC_BASE = 8;

/** A turn's options that carry the spell list: `GetTurnOptions` with its `options` present. */
export type SpellTurnOptions = GetTurnOptionsResponse & { readonly options: TurnOptions };

/** Everything the page knows when it opens the cast sheet. */
export interface CastSheetRequest {
  readonly key: string;
  readonly campaignId: string;
  readonly encounter: Encounter;
  readonly caster: Combatant;
  readonly turn: SpellTurnOptions;
  /** The caster's vitals row (slots used, the pact slots, the resources); absent for a combatant with no row. */
  readonly vitals: VitalsVm | undefined;
  readonly master: boolean;
  readonly diceMode: DiceMode;
  readonly preference: DicePreference;
  readonly state: CombatState;
  /** The damage of a cast whose sheet was closed before it was rolled. */
  readonly resume?: readonly PendingDamage[];
  readonly map: CastMapData | null;
}

/** The spell attack bonus, for a typed d20: the one of a spell attack in the options, or a save cantrip's DC minus 8
 * (the same proficiency and modifier). */
export function spellAttackBonus(attacks: readonly Attack[]): number {
  const spellAttack = attacks.find((a) => a.kind === AttackKind.SPELL && a.saveDc === 0);
  if (spellAttack) {
    return spellAttack.attackBonus;
  }
  const save = attacks.find((a) => a.saveDc > 0);
  return save ? save.saveDc - SAVE_DC_BASE : 0;
}

/** The Portuguese name of a spell of the list, of a cantrip that is an attack, or the key when neither is in the options. */
function castName(
  spell: SpellOption | undefined,
  cantrip: Attack | undefined,
  key: string,
): string {
  if (spell?.spell) {
    return spell.spell.namePt || spell.spell.name;
  }
  return cantrip ? cantrip.namePt || cantrip.name : key;
}

/** How many free slots Escudo could still be cast with and its name; `null` free slots without Escudo. */
function shieldOf(spells: readonly SpellOption[]) {
  const shield = spells.find((s) => s.spell?.key === 'spell:shield');
  return {
    shieldFree: shield ? shield.slots.reduce((n, s) => n + s.free, 0) : null,
    shieldName: shield?.spell?.namePt || 'Escudo Arcano',
  };
}

/** What the spell itself says: its level, concentration, economy and slots. */
function spellFacts(spell: SpellOption | undefined) {
  return {
    level: spell?.spell?.level ?? 0,
    concentration: spell?.spell?.concentration ?? false,
    economy: spell?.economy ?? ActionEconomy.ACTION,
    slots: spell?.slots ?? [],
  };
}

/** What the sorcerer's options say: Metamagic, Sculpt Spells, and a damage cantrip's dice at the caster's level. */
function optionFacts(spell: SpellOption | undefined, cantrip: Attack | undefined) {
  return {
    cantripDice: cantrip?.spellDice ?? '',
    metamagic: spell?.metamagicOptions ?? [],
    sculptSpells: spell?.sculptSpells ?? false,
  };
}

/** What the caster's row of vitals says: the slots used, the pact slots and the sorcery points. */
function casterFacts(vitals: VitalsVm | undefined) {
  return {
    usage: vitals?.spellSlots ?? [],
    pact: vitals?.pactSlots ?? null,
    sorceryPoints: poolOf(vitals?.resources, SORCERY_POINTS_RESOURCE),
  };
}

/** The cast sheet's data: a spell of the options, or a cantrip that asks for a saving throw. */
export function castSheetData(req: CastSheetRequest): CastSheetData {
  const { options, spellTargets } = req.turn;
  const e = req.encounter;
  const spell = options.spells.find((s) => s.spell?.key === req.key);
  const cantrip = options.attacks.find((a) => a.attack?.key === req.key)?.attack;
  return {
    campaignId: req.campaignId,
    encounterId: e.id,
    casterId: req.caster.id,
    round: e.round,
    spellKey: req.key,
    name: castName(spell, cantrip, req.key),
    ...spellFacts(spell),
    ...optionFacts(spell, cantrip),
    ...casterFacts(req.vitals),
    targets: spellTargets.find((t) => t.spellKey === req.key),
    ...shieldOf(options.spells),
    attackBonus: spellAttackBonus(options.attacks.flatMap((a) => (a.attack ? [a.attack] : []))),
    master: req.master,
    diceMode: req.diceMode,
    preference: req.preference,
    state: req.state,
    resume: req.resume,
    map: req.map,
  };
}

/** A spell placed on the map opens on the map, its first step (PM-02a); any other on the title. */
export function opensOnMap(data: CastSheetData): boolean {
  return (
    !data.resume &&
    !!data.map &&
    !!data.targets &&
    data.targets.placement !== AreaPlacement.UNSPECIFIED &&
    data.targets.placement !== AreaPlacement.CASTER
  );
}

/** The sheet's accessible name: the damage still to roll, or the cast. */
export function castAriaLabel(name: string, resume: boolean): string {
  return resume ? `Rolar o dano de ${name}` : `Conjurar ${name}`;
}

/** A spell of the options with its targets, its level and its Portuguese name; `null` when the options lack it. */
export function areaSpellOf(turn: GetTurnOptionsResponse | null | undefined, key: string) {
  const spell = turn?.options?.spells.find((s) => s.spell?.key === key);
  const targets = turn?.spellTargets.find((t) => t.spellKey === key);
  if (!spell?.spell || !targets) {
    return null;
  }
  return {
    spell,
    targets,
    level: spell.spell.level,
    name: spell.spell.namePt || spell.spell.name,
  };
}

/** The data of the master's area-spell picker for an NPC; the NPC's slots are not counted, so it casts with the lowest slot
 * the options offer, or the spell's own level. */
export function masterAreaCastData(req: {
  readonly campaignId: string;
  readonly encounter: Encounter;
  readonly caster: Combatant;
  readonly key: string;
  readonly found: NonNullable<ReturnType<typeof areaSpellOf>>;
  readonly map: CastMapData;
  readonly state: CombatState;
}): MasterAreaCastData {
  const { spell, targets, level, name } = req.found;
  const row = defaultSlot(slotRows(level, spell.slots, [], null));
  return {
    campaignId: req.campaignId,
    encounterId: req.encounter.id,
    caster: req.caster,
    spellKey: req.key,
    name,
    level,
    slot: level > 0 ? { level: row?.level ?? level, pact: row?.pact ?? false } : null,
    economy: spell.economy,
    targets,
    map: req.map,
    state: req.state,
  };
}

/** Where an area spell of the NPC's focuses first: the map unless the spell starts on the caster. */
export function masterAreaFocus(targets: SpellTargets): { focus?: string } {
  return targets.placement === AreaPlacement.CASTER ? {} : { focus: '[role="application"]' };
}
