import type { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import type {
  Combatant,
  Encounter,
  GetTurnOptionsResponse,
  ReactionWindow,
  ReadiedAction,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { ActionEconomy } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import type { CombatState } from '../../../../core/combat/combat-state';
import { heldTargetsOf } from '../../../../core/combat/readied-release';
import { spellAttackBonus } from './cast-sheet-data';
import type { CastMapData, CastSheetData } from './cast-sheet-types';

/** What the page knows when it releases a held spell on the map. */
export interface CastReleaseRequest {
  readonly campaignId: string;
  readonly encounter: Encounter;
  readonly window: ReactionWindow;
  /** The row of the readier's window (`Combatant.readied`). */
  readonly readied: ReadiedAction;
  readonly caster: Combatant;
  /** The readier's own `GetTurnOptions`: the held spell's entry in `spell_targets` and the attack bonus. */
  readonly turn: GetTurnOptionsResponse;
  /** The master releases it for the player's character. */
  readonly master: boolean;
  readonly diceMode: DiceMode;
  readonly preference: DicePreference;
  readonly state: CombatState;
  readonly map: CastMapData;
}

/**
 * What the cast sheet is handed to release a readied spell (#857, boards 857-02 and 857-03): the held spell at the level it was held
 * with (the slot was spent at Ready, so there is no slot to pick, no Metamagic and no table option), its targets from the readier's own
 * options, and the `release` the sheet reads for its band, its words and its way back. `null` when the options do not list the spell.
 */
export function releaseCastData(req: CastReleaseRequest): CastSheetData | null {
  const targets = heldTargetsOf(req.turn, req.readied);
  if (!targets) {
    return null;
  }
  const attacks = (req.turn.options?.attacks ?? []).flatMap((a) => (a.attack ? [a.attack] : []));
  return {
    campaignId: req.campaignId,
    encounterId: req.encounter.id,
    casterId: req.caster.id,
    round: req.encounter.round,
    spellKey: req.readied.actionKey,
    name: req.readied.actionNamePt || req.readied.actionKey,
    level: req.readied.slotLevel,
    // The concentration the spell holds since Ready is the one it keeps: casting it ends nothing else.
    concentration: false,
    cantripDice: '',
    economy: ActionEconomy.REACTION,
    slots: [],
    usage: [],
    pact: null,
    targets,
    shieldFree: null,
    shieldName: '',
    attackBonus: spellAttackBonus(attacks),
    master: req.master,
    diceMode: req.diceMode,
    preference: req.preference,
    state: req.state,
    map: req.map,
    release: {
      windowId: req.window.id,
      readiedId: req.readied.id,
      reactorLabel: req.caster.label,
      trigger: req.readied.triggerText,
      slotLevel: req.readied.slotLevel,
    },
  };
}
