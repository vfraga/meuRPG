import { create } from '@bufbuild/protobuf';
import { of } from 'rxjs';

import { DiceMode, DicePreference } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  GetTurnOptionsResponseSchema,
  ManeuverDieOptionSchema,
  OpportunityOfferSchema,
} from '../../../../gen/meurpg/play/v1/combat_pb';
import {
  AttackKind,
  AttackSchema,
  RollDieSchema,
  TurnOptionsSchema,
} from '../../../../gen/meurpg/rules/v1/rules_pb';
import { CombatState } from '../../../core/combat/combat-state';
import { combatant, encounter } from '../../../core/combat/combat-testing';
import type { AttackSheetData } from './attack-sheet/attack-sheet';
import type { CombatSheetsContext } from './combat-sheets-context';
import { ReactionSheets } from './reaction-sheets';

const sword = create(AttackSchema, {
  key: 'attack:sword',
  name: 'Sword',
  namePt: 'Espada longa',
  kind: AttackKind.WEAPON,
});
const strike = create(ManeuverDieOptionSchema, {
  die: create(RollDieSchema, {
    key: 'golpe',
    namePt: 'Golpe Certeiro',
    applies: 'attack',
    sides: 8,
  }),
  resourceNamePt: 'Reserva de Golpes',
  usesLeft: 2,
});
const options = create(GetTurnOptionsResponseSchema, {
  options: create(TurnOptionsSchema, { attacks: [{ attack: sword }] }),
  maneuverDice: [strike],
});
const offer = create(OpportunityOfferSchema, {
  id: 'o1',
  reactorId: 't',
  moverId: 'g',
  moverLabel: 'Goblin',
});

function opened(byMaster: boolean): AttackSheetData {
  const configs: { data: AttackSheetData }[] = [];
  const dialog = {
    open: (_c: unknown, config: { data: AttackSheetData }) => {
      configs.push(config);
      return { afterClosed: () => of(undefined) };
    },
  };
  const state = new CombatState();
  const enc = encounter({ combatants: [combatant({ id: 't', label: 'Toren' })] });
  const ctx = {
    dialog,
    bottomSheet: {},
    campaignId: () => 'c',
    encounter: () => enc,
    isMaster: () => byMaster,
    state: () => state,
    diceMode: () => DiceMode.PLAYERS_CHOOSE,
    preference: () => DicePreference.APP,
  } as unknown as CombatSheetsContext;
  new ReactionSheets(ctx, () => undefined).opportunityAttack(offer, sword.key, options, byMaster);
  return configs[0].data;
}

describe('ReactionSheets.opportunityAttack: the maneuver dice', () => {
  it("hands the character's dice to the attack sheet, which sends the reaction with its key", () => {
    const data = opened(false);
    expect(data.maneuverDice).toEqual([strike]);
    expect(data.asReaction).toBe(true);
  });

  it('hands none when the master rolls it for an NPC', () => {
    expect(opened(true).maneuverDice).toEqual([]);
  });
});
