import {
  CombatantKind,
  CombatantSide,
  CombatantState,
} from '../../../../gen/meurpg/play/v1/combat_pb';
import { DisabledReasonCode } from '../../../../gen/meurpg/rules/v1/rules_pb';
import { combatant, encounter } from '../../../core/combat/combat-testing';
import { mountCombatView } from './combat-view-testing';

describe('CombatView: the rider and the mount it directs (588-04)', () => {
  const enc = (mountKind = 'controlled', state = CombatantState.UNHURT) =>
    encounter({
      currentCombatantId: 'tor',
      turnGroupIds: ['tor', 'h'],
      combatants: [
        combatant({
          id: 'tor',
          label: 'Tor',
          kind: CombatantKind.PLAYER,
          side: CombatantSide.PARTY,
          mine: true,
          initiative: 17,
          mountedOnId: 'h',
          state,
        }),
        combatant({
          id: 'h',
          label: 'Cavalo de guerra',
          initiative: 17,
          riderId: 'tor',
          mountKind,
        }),
        combatant({ id: 'z', label: 'Zumbi 1', initiative: 12 }),
      ],
    });
  const three = (enabled: boolean) => ({
    options: {
      standardActions: ['standard:dash', 'standard:disengage', 'standard:dodge'].map((key) => ({
        action: { key, namePt: '' },
        enabled,
        reason: enabled ? undefined : { code: DisabledReasonCode.COMBATANT_DOWN },
      })),
    },
  });
  const settle = async (fixture: { detectChanges(): void; whenStable(): Promise<unknown> }) => {
    for (let i = 0; i < 3; i++) {
      await fixture.whenStable();
      fixture.detectChanges();
    }
  };

  it("reads the mount's options and draws its three actions, each one TakeAction with the mount's id", async () => {
    const m = mountCombatView({
      enc: enc(),
      turnOptions: (id) => (id === 'h' ? (three(true) as never) : undefined),
    });
    await settle(m.fixture);
    expect(m.callsOf('turnOptions').some((c) => c.args[2] === 'h')).toBe(true);
    const buttons = [...m.el.querySelectorAll<HTMLButtonElement>('app-pair-card .pair__act')];
    expect(buttons.map((b) => b.textContent?.replace(/\s+/g, ' ').trim())).toEqual([
      'keyboard_double_arrow_rightDisparada',
      'keyboard_tabDesengajar',
      'shieldEsquivar',
    ]);
    buttons[0].click();
    await settle(m.fixture);
    expect(m.callsOf('takeAction').map((c) => c.args.slice(2))).toEqual([['h', 'standard:dash']]);
  });

  it('dashes the three with "Caído: o mestre conduz o Cavalo de guerra." when the rider is down', async () => {
    const m = mountCombatView({
      enc: enc(),
      turnOptions: (id) => (id === 'h' ? (three(false) as never) : undefined),
    });
    await settle(m.fixture);
    expect(m.el.querySelectorAll('app-pair-card .mr-button--off')).toHaveLength(3);
    expect(m.el.querySelector('app-pair-card .pair__why')?.textContent).toContain(
      'Caído: o mestre conduz o Cavalo de guerra.',
    );
  });

  it('keeps the card, dashed, beside the death saves of a rider at 0 hit points', async () => {
    const m = mountCombatView({
      enc: enc('controlled', CombatantState.DYING),
      turnOptions: (id) => (id === 'h' ? (three(false) as never) : undefined),
    });
    await settle(m.fixture);
    expect(m.el.querySelector('app-death-saves')).not.toBeNull();
    expect(m.el.querySelectorAll('app-pair-card .mr-button--off')).toHaveLength(3);
    expect(m.el.querySelector('app-pair-card .pair__why')?.textContent).toContain(
      'Caído: o mestre conduz o Cavalo de guerra.',
    );
  });

  it('ends the mount\'s part first, then the rider\'s: one "Encerrar turno" for the two', async () => {
    // The server answers each call with the combat, the mount's part ended after the first.
    const after = enc();
    after.combatants[1].turnPartEnded = true;
    const m = mountCombatView({ enc: enc(), api: { endTurn: async () => after } });
    await settle(m.fixture);
    await m.view.endTurn(false);
    expect(m.callsOf('endTurn').map((c) => c.args[2])).toEqual(['h', 'tor']);
  });

  it('has no pair card and ends only its own part beside an independent mount', async () => {
    const m = mountCombatView({
      enc: enc('independent'),
      api: { endTurn: async () => enc('independent') },
    });
    await settle(m.fixture);
    expect(m.el.querySelector('app-pair-card')).toBeNull();
    await m.view.endTurn(false);
    expect(m.callsOf('endTurn').map((c) => c.args[2])).toEqual(['tor']);
  });
});
