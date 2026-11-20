import { create } from '@bufbuild/protobuf';
import { describe, expect, it } from 'vitest';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  AreaPlacement,
  GetTurnOptionsResponseSchema,
  ReadiedActionSchema,
  ReadiedKind,
  SpellTargetsSchema,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { CombatState } from '../../../../core/combat/combat-state';
import { combatant, encounter, reactionWindow } from '../../../../core/combat/combat-testing';
import { releaseCastData } from './cast-release';

const readied = create(ReadiedActionSchema, {
  id: 'r1',
  kind: ReadiedKind.SPELL,
  actionKey: 'spell:fireball',
  actionNamePt: 'Bola de Fogo',
  triggerText: 'Quando o ogro passar pela porta',
  slotLevel: 3,
});

function request(over: { targets?: boolean } = {}) {
  const pen = combatant({ id: 'pen', label: 'Pensantus', readied });
  return {
    campaignId: 'camp',
    encounter: encounter({ id: 'enc', round: 2, combatants: [pen] } as never),
    window: reactionWindow({ id: 'w1', reactorId: 'pen' } as never),
    readied,
    caster: pen,
    turn: create(GetTurnOptionsResponseSchema, {
      spellTargets:
        over.targets === false
          ? []
          : [
              create(SpellTargetsSchema, {
                spellKey: 'spell:fireball',
                placement: AreaPlacement.POINT,
                rangeFt: 150,
              }),
            ],
    }),
    master: false,
    diceMode: DiceMode.PHYSICAL,
    preference: DicePreference.UNSPECIFIED,
    state: new CombatState(),
    map: { image: {}, mapName: 'Cripta', columns: 20, rows: 15, layers: null, fog: null } as never,
  };
}

describe("the cast sheet's data for a held spell released on the map (#857, 857-02)", () => {
  it('holds the spell at the level it was held with, with no slot to pick and no concentration of its own', () => {
    const data = releaseCastData(request())!;
    expect(data.spellKey).toBe('spell:fireball');
    expect(data.name).toBe('Bola de Fogo');
    expect(data.level).toBe(3);
    expect(data.slots).toEqual([]);
    expect(data.concentration).toBe(false);
    expect(data.metamagic ?? []).toEqual([]);
    expect(data.tableOptions ?? []).toEqual([]);
  });

  it("reads the held spell from the readier's own targets and keeps the map", () => {
    const data = releaseCastData(request())!;
    expect(data.targets?.placement).toBe(AreaPlacement.POINT);
    expect(data.targets?.rangeFt).toBe(150);
    expect(data.map?.mapName).toBe('Cripta');
  });

  it('says which window it releases, who holds it and the trigger, for the band and the way back', () => {
    expect(releaseCastData(request())!.release).toEqual({
      windowId: 'w1',
      readiedId: 'r1',
      reactorLabel: 'Pensantus',
      trigger: 'Quando o ogro passar pela porta',
      slotLevel: 3,
    });
  });

  it("is the master's when the master releases it for the player", () => {
    expect(releaseCastData({ ...request(), master: true })!.master).toBe(true);
  });

  it('gives nothing when the options do not list the held spell', () => {
    expect(releaseCastData(request({ targets: false }))).toBeNull();
  });
});
