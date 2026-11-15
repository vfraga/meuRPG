import { Code, ConnectError } from '@connectrpc/connect';

import {
  CombatantKind,
  CombatantSide,
  CombatantState,
} from '../../../../gen/meurpg/play/v1/combat_pb';
import { DisabledReasonCode } from '../../../../gen/meurpg/rules/v1/rules_pb';
import { combatant, encounter } from '../../../core/combat/combat-testing';
import { PHONE_QUERY } from '../../../shared/map-view/media-query';
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

  it('words the last line for the width: the desktop line, and the short one on a phone (588-05)', async () => {
    const opts = {
      enc: enc(),
      turnOptions: (id: string) => (id === 'h' ? (three(true) as never) : undefined),
    };
    const wide = mountCombatView(opts);
    await settle(wide.fixture);
    expect(wide.el.querySelector('.pair__note')?.textContent?.trim()).toBe(
      'Montaria adestrada: só estas três ações; a reação fica livre. "Encerrar turno" encerra a vez dos dois.',
    );
    const phone = mountCombatView({ ...opts, matches: [PHONE_QUERY] });
    await settle(phone.fixture);
    const note = phone.el.querySelector('.pair__note');
    expect(note?.textContent?.replace(/\s+/g, ' ').trim()).toBe(
      'linkAdestrada: só estas três. "Mover" leva os dois.',
    );
    expect(note?.querySelector('mat-icon')?.getAttribute('aria-hidden')).toBe('true');
  });

  it('on a phone sits inside the one action card: the shapes and "Mover", then the mount, then the tabs (588-05)', async () => {
    const m = mountCombatView({
      enc: enc(),
      matches: [PHONE_QUERY],
      turnOptions: (id) => (id === 'h' ? (three(true) as never) : undefined),
    });
    await settle(m.fixture);
    const kids = [...m.el.querySelector('app-action-groups .groups')!.children];
    const at = (sel: string) => kids.findIndex((k) => k.matches(sel));
    expect(at('.econ')).toBeGreaterThanOrEqual(0);
    expect(at('app-pair-card')).toBeGreaterThan(at('.econ'));
    expect(at('app-tab-list')).toBeGreaterThan(at('app-pair-card'));
    expect(m.el.querySelector('.econ app-economy-shapes')).not.toBeNull();
    expect(m.el.querySelector('.econ .econ__move')).not.toBeNull();
  });

  it('from 768 px comes first, with the rider\'s shapes in it: the row under it keeps only "Mover" (588-04)', async () => {
    const m = mountCombatView({
      enc: enc(),
      turnOptions: (id) => (id === 'h' ? (three(true) as never) : undefined),
    });
    await settle(m.fixture);
    const card = m.el.querySelector<HTMLElement>('app-action-groups app-pair-card')!;
    expect(card.querySelector('.pair__rider app-economy-shapes')).not.toBeNull();
    expect(m.el.querySelector('.econ app-economy-shapes')).toBeNull();
    expect(m.el.querySelector('.econ .econ__move')).not.toBeNull();
    expect(m.el.querySelectorAll('app-pair-card')).toHaveLength(1);
  });

  it('keeps the card, dashed, beside the death saves of a rider at 0 hit points', async () => {
    const m = mountCombatView({
      enc: enc('controlled', CombatantState.DYING),
      turnOptions: (id) => (id === 'h' ? (three(false) as never) : undefined),
    });
    await settle(m.fixture);
    expect(m.el.querySelector('app-death-saves')).not.toBeNull();
    expect(m.el.querySelectorAll('app-pair-card .mr-button--off')).toHaveLength(3);
    // The card and the death saves are two blocks of the column, with its gap between them.
    const column = m.el.querySelector('app-death-saves')!.parentElement!;
    expect(column.classList.contains('player__blocks')).toBe(true);
    expect(column.querySelector(':scope > app-pair-card')).not.toBeNull();
    expect(m.el.querySelector('app-pair-card .pair__why')?.textContent).toContain(
      'Caído: o mestre conduz o Cavalo de guerra.',
    );
  });

  it('ends the rider\'s part first, then the mount\'s: one "Encerrar turno" for the two', async () => {
    // The server answers each call with the combat, the rider's part ended after the first.
    const after = enc();
    after.combatants[0].turnPartEnded = true;
    const m = mountCombatView({ enc: enc(), api: { endTurn: async () => after } });
    await settle(m.fixture);
    await m.view.endTurn(false);
    expect(m.callsOf('endTurn').map((c) => c.args[2])).toEqual(['tor', 'h']);
  });

  it("sends nothing for the mount when the rider's part is refused (a damage still to roll): the mount keeps its move", async () => {
    const m = mountCombatView({
      enc: enc(),
      api: {
        endTurn: async () => {
          throw new ConnectError('a damage is still to roll', Code.FailedPrecondition);
        },
      },
    });
    await settle(m.fixture);
    expect(await m.view.endTurn(false)).toBe(false);
    expect(m.callsOf('endTurn').map((c) => c.args[2])).toEqual(['tor']);
  });

  it("ends only the rider's part when the rider is down: the master leads the mount", async () => {
    const down = enc('controlled', CombatantState.DYING);
    const m = mountCombatView({ enc: down, api: { endTurn: async () => down } });
    await settle(m.fixture);
    await m.view.endTurn(false);
    expect(m.callsOf('endTurn').map((c) => c.args[2])).toEqual(['tor']);
  });

  it("keeps the card and a control to end the mount's part when the rider's part ended elsewhere", async () => {
    const left = enc();
    left.combatants[0].turnPartEnded = true;
    const m = mountCombatView({
      enc: left,
      turnOptions: (id) => (id === 'h' ? (three(true) as never) : undefined),
      api: { endTurn: async () => left },
    });
    await settle(m.fixture);
    expect(m.el.querySelector('app-pair-card')).not.toBeNull();
    const end = [...m.el.querySelectorAll<HTMLButtonElement>('app-end-turn button')].find((b) =>
      b.textContent?.includes('Encerrar turno'),
    );
    expect(end).toBeDefined();
    end!.click();
    await settle(m.fixture);
    expect(m.callsOf('endTurn').map((c) => c.args[2])).toEqual(['h']);
  });

  describe('tied with another character (a larger group)', () => {
    const tied = () => {
      const e = enc();
      e.combatants.push(
        combatant({
          id: 'b',
          label: 'Brisa',
          kind: CombatantKind.PLAYER,
          side: CombatantSide.PARTY,
          initiative: 17,
        }),
      );
      e.turnGroupIds = ['tor', 'h', 'b'];
      return e;
    };

    it('still draws the pair card, inside the joint turn, and waits only for the others', async () => {
      const m = mountCombatView({
        enc: tied(),
        turnOptions: (id) => (id === 'h' ? (three(true) as never) : undefined),
      });
      await settle(m.fixture);
      expect(m.el.querySelectorAll('app-pair-card .pair__act')).toHaveLength(3);
      expect(m.el.querySelector('app-end-part')).not.toBeNull();
      expect(m.view.jointOthers()).toEqual(['Brisa']);
    });

    it('"Encerrar a minha parte" ends the rider\'s part and the mount\'s', async () => {
      const after = tied();
      after.combatants[0].turnPartEnded = true;
      const m = mountCombatView({ enc: tied(), api: { endTurn: async () => after } });
      await settle(m.fixture);
      await m.view.endTurn(false);
      expect(m.callsOf('endTurn').map((c) => c.args[2])).toEqual(['tor', 'h']);
    });
  });

  it('is one turn, not a joint one, for the pair alone: no others to wait for, and the phone bar says "Encerrar turno"', async () => {
    const m = mountCombatView({ enc: enc(), matches: [PHONE_QUERY] });
    await settle(m.fixture);
    expect(m.view.joint()).toBeNull();
    expect(m.view.jointOthers()).toBeNull();
    const bar = m.el.querySelector('app-turn-bar');
    expect(bar?.textContent).toContain('Encerrar turno');
    expect(bar?.textContent).not.toContain('Encerrar a minha parte');
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
