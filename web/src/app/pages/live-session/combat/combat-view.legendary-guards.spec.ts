// The legendary offers keep to one answer at a time: the attack row while its sheet opens, the armed move while its
// drop is in the air, the cards while a spend is sent, and the focus when a card folds.
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  CombatantKind,
  EncounterBlockedReason,
  EncounterBlockedSchema,
} from '../../../../gen/meurpg/play/v1/combat_pb';
import { AttackOptionSchema, AttackSchema } from '../../../../gen/meurpg/rules/v1/rules_pb';
import { combatant, encounter } from '../../../core/combat/combat-testing';
import { settle } from '../../../../test-settle';
import { AttackSheet } from './attack-sheet/attack-sheet';
import { mountCombatView } from './combat-view-testing';

const plain = (node: Node | null | undefined) => {
  if (!node) {
    return '';
  }
  const copy = node.cloneNode(true) as HTMLElement;
  copy.querySelectorAll('mat-icon').forEach((i) => i.remove());
  return (copy.textContent ?? '').replace(/\s+/g, ' ').trim();
};

const option = (key: string, kind: string, cost: number, attackKey = '') => ({
  key,
  name: key,
  namePt: key === 'tail' ? 'Ataque de cauda' : key === 'wing' ? 'Ataque de asas' : 'Detectar',
  cost,
  kind,
  text: 'x',
  affordable: true,
  attackKey,
});
const creature = (id: string, label: string) =>
  combatant({
    id,
    label,
    initiative: 19,
    legendary: {
      actionsPerRound: 3,
      actionsLeft: 3,
      resistancePerDay: 0,
      resistanceLeft: 0,
      usableNow: true,
      options: [
        option('detect', 'text', 1),
        option('tail', 'attack', 1, 'm#tail'),
        option('wing', 'move', 2),
      ],
    },
  } as never);
const tor = combatant({ id: 't', label: 'Tor', kind: CombatantKind.PLAYER, characterId: 'tc' });
const enc = encounter({
  id: 'enc',
  combatants: [creature('d', 'Dragão vermelho adulto'), creature('v', 'Vampiro'), tor],
  currentCombatantId: 't',
  round: 4,
} as never);
const LAPTOP = ['(min-width: 1024px)'];

const tailAttack = create(AttackOptionSchema, {
  attack: create(AttackSchema, { key: 'm#tail', namePt: 'Cauda', attackBonus: 14 }),
});

function deferred<T>() {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

/** The button of a card (0: the first creature's) whose words start with `label`. */
const rowButton = (el: HTMLElement, card: number, label: string) =>
  Array.from(
    el.querySelectorAll('[data-testid="legendary-card"]')[card].querySelectorAll('button'),
  ).find((b) => plain(b).startsWith(label)) as HTMLButtonElement;

describe('Legendary offers: one answer at a time', () => {
  describe('"Atacar"', () => {
    const mount = (turnOptions: () => Promise<unknown>) =>
      mountCombatView({ enc, master: true, matches: LAPTOP, api: { turnOptions } });

    it('opens one sheet on a double tap, says "Abrindo…" meanwhile and holds every other row', async () => {
      const gate = deferred<unknown>();
      const { fixture, el, opened, callsOf } = mount(() => gate.promise);
      await settle(fixture);
      rowButton(el, 0, 'Atacar').click();
      rowButton(el, 0, 'Atacar').click();
      await settle(fixture);
      expect(callsOf('turnOptions').filter((c) => c.args[2] === 'd')).toHaveLength(1);
      expect(plain(rowButton(el, 0, 'Abrindo'))).toBe('Abrindo…');
      expect(rowButton(el, 1, 'Usar').getAttribute('aria-disabled')).toBe('true');
      gate.resolve({
        options: { attacks: [tailAttack] },
        attackTargets: [],
        spellTargets: [],
        pendingDamages: [],
        resourceTargets: [],
      });
      await settle(fixture);
      expect(opened.filter((o) => o.component === AttackSheet)).toHaveLength(1);
      expect(plain(rowButton(el, 0, 'Atacar'))).toBe('Atacar');
    });

    it('lets the row be tapped again when the sheet could not be opened', async () => {
      const { fixture, el, callsOf } = mount(() => Promise.reject(new Error('down')));
      await settle(fixture);
      rowButton(el, 0, 'Atacar').click();
      await settle(fixture);
      rowButton(el, 0, 'Atacar').click();
      await settle(fixture);
      expect(callsOf('turnOptions').filter((c) => c.args[2] === 'd')).toHaveLength(2);
    });
  });

  describe('a spend in the air', () => {
    it("holds the rows of every card, not only the sender's, and sends once", async () => {
      const gate = deferred<unknown>();
      const { fixture, el, callsOf } = mountCombatView({
        enc,
        master: true,
        matches: LAPTOP,
        api: { spendLegendaryAction: () => gate.promise },
      });
      await settle(fixture);
      rowButton(el, 0, 'Usar').click();
      await settle(fixture);
      rowButton(el, 1, 'Usar').click();
      await settle(fixture);
      expect(callsOf('spendLegendaryAction')).toHaveLength(1);
      expect(rowButton(el, 1, 'Usar').getAttribute('aria-disabled')).toBe('true');
      expect(rowButton(el, 0, 'Atacar').getAttribute('aria-disabled')).toBe('true');
      gate.resolve(enc);
      await settle(fixture);
      expect(rowButton(el, 1, 'Usar').getAttribute('aria-disabled')).not.toBe('true');
    });

    it('keeps the key of a lost answer for that creature and option, and gives a new one after a refusal', async () => {
      let n = 0;
      const { fixture, el, callsOf } = mountCombatView({
        enc,
        master: true,
        matches: LAPTOP,
        api: {
          spendLegendaryAction: () => {
            n += 1;
            if (n === 1) {
              throw new ConnectError('down', Code.Unavailable);
            }
            if (n === 3) {
              throw new ConnectError('no', Code.InvalidArgument);
            }
            return enc;
          },
        },
      });
      await settle(fixture);
      rowButton(el, 0, 'Usar').click(); // answer lost
      await settle(fixture);
      rowButton(el, 1, 'Usar').click(); // another creature in between
      await settle(fixture);
      rowButton(el, 0, 'Usar').click(); // the first one again: a retry
      await settle(fixture);
      rowButton(el, 1, 'Usar').click(); // the other one again
      await settle(fixture);
      const keys = callsOf('spendLegendaryAction').map((c) => c.args[4]);
      expect(keys).toHaveLength(4);
      expect(keys[2]).toBe(keys[0]);
      expect(keys[1]).not.toBe(keys[0]);
    });
  });

  describe('the armed move', () => {
    type Drop = (d: { id: string; col: number; row: number }) => Promise<void>;
    const armedView = async (move: (...a: unknown[]) => unknown, matches: string[] = LAPTOP) => {
      const m = mountCombatView({ enc, master: true, matches, api: { move } });
      await settle(m.fixture);
      m.view.moves.legendaryMove.set({ combatantId: 'd', optionKey: 'wing' });
      await settle(m.fixture);
      return m;
    };

    it('charges once when a second token is dropped before the first answer', async () => {
      const gate = deferred<unknown>();
      const { fixture, view, callsOf } = await armedView(() =>
        gate.promise.then(() => ({ encounter: enc })),
      );
      const drop = view.moves.drop.bind(view.moves) as Drop;
      const first = drop({ id: 'd', col: 3, row: 4 });
      expect(view.moves.legendaryMove()).toBeNull();
      const second = drop({ id: 'd', col: 4, row: 4 });
      gate.resolve(undefined);
      await Promise.all([first, second]);
      await settle(fixture);
      const sent = callsOf('move').map((c) => c.args[8]);
      expect(sent.filter((o) => o === 'wing')).toHaveLength(1);
    });

    it('gives the option back when the server refuses, to try again', async () => {
      const { view } = await armedView(() => {
        throw new Error('refused');
      });
      await view.moves.drop({ id: 'd', col: 3, row: 4 });
      expect(view.moves.legendaryMove()).toEqual({ combatantId: 'd', optionKey: 'wing' });
    });

    it('never sends it forced, and the "Movimento forçado" box waits for the next drag', async () => {
      const { view, callsOf } = await armedView(() => ({ encounter: enc }));
      view.moves.forcedMove.set(true);
      await view.moves.drop({ id: 'd', col: 3, row: 4 });
      const args = callsOf('move')[0].args;
      expect(args[7]).toBe(false);
      expect(args[8]).toBe('wing');
      expect(view.moves.forcedMove()).toBe(true);
    });

    it('does not offer "Mover mesmo assim" when the walk is too far', async () => {
      const { view, callsOf } = await armedView(() => {
        throw new ConnectError('too far', Code.FailedPrecondition, undefined, [
          {
            desc: EncounterBlockedSchema,
            value: create(EncounterBlockedSchema, {
              reason: EncounterBlockedReason.TOO_FAR,
              missingFt: 15,
              missingDft: 150,
            }),
          },
        ]);
      });
      await view.moves.drop({ id: 'd', col: 3, row: 4 });
      expect(view.moves.overAsk()).toBeNull();
      expect(view.error()).not.toBe('');
      expect(callsOf('move')).toHaveLength(1);
      expect(view.moves.legendaryMove()).not.toBeNull();
    });

    it('says where to drag, and on a phone that the map is its own tab', async () => {
      const laptop = await armedView(() => ({ encounter: enc }));
      expect(plain(laptop.el.querySelector('[data-testid="legendary-armed"]'))).toContain(
        'Arraste a ficha de Dragão vermelho adulto no mapa',
      );
      const phone = await armedView(() => ({ encounter: enc }), []);
      expect(plain(phone.el.querySelector('[data-testid="legendary-armed"]'))).toContain(
        'Abra a aba Mapa',
      );
    });
  });

  describe('the count of a card', () => {
    it('is one element of its own, so a test can read it alone', async () => {
      const { fixture, el } = mountCombatView({ enc, master: true, matches: LAPTOP });
      await settle(fixture);
      const counts = el.querySelectorAll(
        '[data-testid="legendary-card"] [data-testid="legendary-counts"]',
      );
      expect(counts).toHaveLength(2);
      expect(plain(counts[0])).toBe('3 de 3');
    });
  });

  describe('folding a card', () => {
    it('moves the focus to the button that undoes it', async () => {
      const { fixture, el } = mountCombatView({ enc, master: true, matches: LAPTOP });
      await settle(fixture);
      el.querySelector<HTMLButtonElement>('.lcard__fold')!.click();
      await settle(fixture);
      const open = el.querySelector<HTMLElement>('[data-testid="legendary-folded"] button')!;
      expect(document.activeElement).toBe(open);
      open.click();
      await settle(fixture);
      expect(document.activeElement).toBe(el.querySelector('.lcard__fold'));
    });
  });
});
