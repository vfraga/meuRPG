import { create } from '@bufbuild/protobuf';

import { GetTurnOptionsResponseSchema } from '../../../gen/meurpg/play/v1/combat_pb';
import { DisabledReasonCode } from '../../../gen/meurpg/rules/v1/rules_pb';
import { combatant } from './combat-testing';
import { pairActions } from './pair-actions';

const horse = combatant({ id: 'h', label: 'Cavalo de guerra' });

function options(enabled: boolean, code = DisabledReasonCode.UNSPECIFIED, textPt = '') {
  return create(GetTurnOptionsResponseSchema, {
    options: {
      standardActions: [
        'standard:dash',
        'standard:disengage',
        'standard:dodge',
        'standard:help',
      ].map((key) => ({
        action: { key, namePt: '' },
        enabled,
        reason: enabled ? undefined : { code, textPt },
      })),
    },
  });
}

describe('pairActions', () => {
  it('gives the three actions of a mount that carries its rider, in order, and nothing else', () => {
    const { actions, why } = pairActions(options(true), horse);
    expect(actions.map((a) => [a.name, a.off])).toEqual([
      ['Disparada', false],
      ['Desengajar', false],
      ['Esquivar', false],
    ]);
    expect(why).toBe('');
  });

  it('says "Caído: o mestre conduz o Cavalo de guerra." when the rider is down', () => {
    const { actions, why } = pairActions(options(false, DisabledReasonCode.COMBATANT_DOWN), horse);
    expect(actions.every((a) => a.off)).toBe(true);
    expect(why).toBe('Caído: o mestre conduz o Cavalo de guerra.');
    // The state label keeps the form of the condition names, whatever the rider's name.
    expect(pairActions(options(false, DisabledReasonCode.COMBATANT_DOWN), horse).why).toMatch(
      /^Caído:/,
    );
  });

  it("reads the server's own sentence for any other refusal", () => {
    expect(
      pairActions(options(false, DisabledReasonCode.UNSPECIFIED, 'Você já usou a ação.'), horse)
        .why,
    ).toBe('Você já usou a ação.');
  });

  it('keeps the three off, with no reason, until the options are read', () => {
    const { actions, why } = pairActions(null, horse);
    expect(actions.every((a) => a.off)).toBe(true);
    expect(why).toBe('');
  });
});
