import {
  CombatantKind,
  CombatantState,
  EncounterStatus,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { combatant, encounter } from './combat-testing';
import { turnBanner } from './combat-view';
import { partToEnd } from './joint-turn';
import { rideToEnd, turnEnds } from './pair-turn';
import { CHARACTER_TAB, mineTabs, optionIds, tabsBesideRide } from './mine';

const P = CombatantKind.PLAYER;

/** Tor on the controlled horse; `also` adds Brisa, tied on the same initiative, to the group. */
function fight(
  opts: {
    also?: boolean;
    tor?: object;
    horse?: object;
    mine?: boolean;
  } = {},
) {
  const tor = combatant({
    id: 'tor',
    label: 'Tor',
    kind: P,
    initiative: 17,
    mine: opts.mine ?? true,
    mountedOnId: 'h',
    ...opts.tor,
  });
  const horse = combatant({
    id: 'h',
    label: 'Cavalo de guerra',
    initiative: 17,
    riderId: 'tor',
    mountKind: 'controlled',
    ...opts.horse,
  });
  const brisa = combatant({ id: 'b', label: 'Brisa', kind: P, initiative: 17 });
  const zombie = combatant({ id: 'z', label: 'Zumbi 1', initiative: 12 });
  return encounter({
    status: EncounterStatus.ACTIVE,
    combatants: [tor, horse, ...(opts.also ? [brisa] : []), zombie],
    turnGroupIds: ['tor', 'h', ...(opts.also ? ['b'] : [])],
    currentCombatantId: 'tor',
  });
}
const who = (e: ReturnType<typeof fight>, id: string) => e.combatants.find((c) => c.id === id)!;

describe('the turn of a rider and the mount it directs (#588, 588-04)', () => {
  describe('the banner', () => {
    it("is the rider's own turn while either part is theirs to end, and theirs no more once both ended", () => {
      expect(turnBanner(fight())).toMatchObject({
        title: 'Sua vez, Tor',
        mine: true,
        ownEnded: false,
      });
      // The rider's part ended (the master, or another tab) and the mount's is live: the player still ends it.
      const mountLeft = turnBanner(fight({ tor: { turnPartEnded: true } }));
      expect(mountLeft).toMatchObject({ title: 'Sua vez, Tor', mine: true, ownEnded: false });
      expect(mountLeft.riding?.id).toBe('h');
      const both = turnBanner(
        fight({ tor: { turnPartEnded: true }, horse: { turnPartEnded: true } }),
      );
      expect(both).toMatchObject({
        title: 'Você encerrou a sua parte',
        mine: false,
        ownEnded: true,
      });
    });

    it("leaves the mount to the master once a down rider's own part ended", () => {
      const down = { state: CombatantState.DYING };
      expect(turnBanner(fight({ tor: down }))).toMatchObject({ mine: true });
      expect(turnBanner(fight({ tor: { ...down, turnPartEnded: true } }))).toMatchObject({
        mine: false,
        ownEnded: true,
      });
    });

    it('finds the pair inside a larger tie: the joint banner stays, with the mount the rider directs', () => {
      const banner = turnBanner(fight({ also: true }));
      expect(banner.joint?.members.map((m) => m.id)).toEqual(['tor', 'h', 'b']);
      expect(banner.riding?.id).toBe('h');
      expect(banner).toMatchObject({ title: 'Sua vez, Tor', mine: true });
      const mountLeft = turnBanner(fight({ also: true, tor: { turnPartEnded: true } }));
      expect(mountLeft).toMatchObject({ mine: true, ownEnded: false });
      expect(mountLeft.riding?.id).toBe('h');
    });

    it("is nobody's pair for another player, an independent mount, or a mount that is not in the group", () => {
      expect(turnBanner(fight({ mine: false })).riding).toBeUndefined();
      expect(turnBanner(fight({ also: true, mine: false })).riding).toBeUndefined();
      expect(turnBanner(fight({ horse: { mountKind: 'independent' } })).riding).toBeUndefined();
      const apart = fight({ also: true });
      apart.turnGroupIds = ['tor', 'b'];
      expect(turnBanner(apart).riding).toBeUndefined();
    });
  });

  describe('"Encerrar turno"', () => {
    it("ends the rider's own part first, so its gates hold the mount's part too, then the mount's", () => {
      const e = fight();
      expect(turnEnds(e, who(e, 'tor'), who(e, 'h'))).toEqual(['tor', 'h']);
    });

    it('sends only the part that still acts: a retry after the first call, or a part someone else ended', () => {
      const riderDone = fight({ tor: { turnPartEnded: true } });
      expect(turnEnds(riderDone, who(riderDone, 'tor'), who(riderDone, 'h'))).toEqual(['h']);
      const mountDone = fight({ horse: { turnPartEnded: true } });
      expect(turnEnds(mountDone, who(mountDone, 'tor'), who(mountDone, 'h'))).toEqual(['tor']);
    });

    it("ends only the rider's part when the rider is down: the master leads the mount", () => {
      const e = fight({ tor: { state: CombatantState.DYING } });
      expect(turnEnds(e, who(e, 'tor'), who(e, 'h'))).toEqual(['tor']);
      const ended = fight({ tor: { state: CombatantState.DYING, turnPartEnded: true } });
      expect(turnEnds(ended, who(ended, 'tor'), who(ended, 'h'))).toEqual([]);
    });

    it('is the one part of before for whoever rides nothing', () => {
      const e = fight();
      expect(turnEnds(e, who(e, 'tor'), null)).toEqual(['tor']);
      expect(turnEnds(e, null, null)).toEqual(['tor']);
    });

    it('rideToEnd: the mount that still acts under a rider on its feet, else null', () => {
      const e = fight();
      expect(rideToEnd(e, who(e, 'h'), who(e, 'tor'))?.id).toBe('h');
      expect(rideToEnd(e, null, who(e, 'tor'))).toBeNull();
      expect(rideToEnd(null, who(e, 'h'), who(e, 'tor'))).toBeNull();
      const done = fight({ horse: { turnPartEnded: true } });
      expect(rideToEnd(done, who(done, 'h'), who(done, 'tor'))).toBeNull();
      const down = fight({ tor: { state: CombatantState.DOWN } });
      expect(rideToEnd(down, who(down, 'h'), who(down, 'tor'))).toBeNull();
      // The combat as it is now decides, not the row the screen held.
      const stale = who(fight(), 'h');
      expect(rideToEnd(done, stale, who(done, 'tor'))).toBeNull();
    });

    it("partToEnd: the caller's own part while it acts, else the combatant on turn", () => {
      const e = fight({ also: true });
      expect(partToEnd(e, who(e, 'tor'))).toBe('tor');
      expect(partToEnd(e, who(e, 'b'))).toBe('b');
      expect(partToEnd(e, null)).toBe('tor');
      const ended = fight({ also: true, tor: { turnPartEnded: true } });
      ended.currentCombatantId = 'b';
      expect(partToEnd(ended, who(ended, 'tor'))).toBe('b');
      expect(partToEnd(e, who(e, 'z'))).toBe('tor');
    });
  });

  describe("optionIds, the options the player's page reads besides the character's", () => {
    const wolf = combatant({
      id: 'w',
      label: 'Lobo',
      kind: CombatantKind.CREATURE,
      controlledByMe: true,
    });

    it('adds the mount the character directs to its creatures, once', () => {
      const e = fight();
      e.combatants.push(wolf);
      expect(optionIds(e, false, who(e, 'h'))).toEqual(['w', 'h']);
      expect(optionIds(e, false, null)).toEqual(['w']);
      // A steed that is the player's own creature and the mount: listed once.
      expect(optionIds(e, false, wolf)).toEqual(['w']);
    });

    it('reads nothing for the master, without a combat, or outside a running one', () => {
      const e = fight();
      expect(optionIds(e, true, who(e, 'h'))).toEqual([]);
      expect(optionIds(null, false, who(e, 'h'))).toEqual([]);
      e.status = EncounterStatus.ENDED;
      expect(optionIds(e, false, who(e, 'h'))).toEqual([]);
    });
  });

  describe('the tabs of a player whose own steed is the mount (Find Steed)', () => {
    const steedFight = () => {
      const e = fight({ horse: { kind: CombatantKind.CREATURE, controlledByMe: true } });
      e.combatants.push(
        combatant({ id: 'w', label: 'Lobo', kind: CombatantKind.CREATURE, controlledByMe: true }),
      );
      return e;
    };

    it('leaves the steed out while the pair card stands in for its tab, and keeps the other creatures', () => {
      const e = steedFight();
      expect(mineTabs(e).map((t) => t.id)).toEqual([CHARACTER_TAB, 'h', 'w']);
      expect(tabsBesideRide(mineTabs(e), who(e, 'h')).map((t) => t.id)).toEqual([
        CHARACTER_TAB,
        'w',
      ]);
      expect(tabsBesideRide(mineTabs(e), null).map((t) => t.id)).toEqual([CHARACTER_TAB, 'h', 'w']);
    });

    it("takes only the steed out of a group it shares, with the group's name and who still acts", () => {
      const e = steedFight();
      who(e, 'h').summonGroupId = 'pack';
      who(e, 'w').summonGroupId = 'pack';
      e.turnGroupIds = ['tor', 'h', 'w'];
      const tabs = tabsBesideRide(mineTabs(e), who(e, 'h'));
      expect(tabs.map((t) => t.members.map((m) => m.id))).toEqual([['tor'], ['w']]);
      expect(tabs[1].acting.map((m) => m.id)).toEqual(['w']);
      expect(tabs[1].label).toBe('Lobo');
    });
  });
});
