// The legendary offers on the master's screen: the card in the turn column, the line under "Próximo turno", the
// attack sheet of the creature, the log in Portuguese and the Parry switch. A player never gets any of it.
import { create } from '@bufbuild/protobuf';

import {
  CombatLogKind,
  CombatantKind,
  ReactionKind,
} from '../../../../gen/meurpg/play/v1/combat_pb';
import { AttackOptionSchema, AttackSchema } from '../../../../gen/meurpg/rules/v1/rules_pb';
import { combatant, encounter, reactionWindow } from '../../../core/combat/combat-testing';
import { settle } from '../../../../test-settle';
import { AttackSheet } from './attack-sheet/attack-sheet';
import { mountCombatView } from './combat-view-testing';

const plain = (node: Node | null | undefined) => {
  if (!node) {
    return '';
  }
  const copy = node.cloneNode(true) as HTMLElement;
  copy.querySelectorAll('mat-icon').forEach((i) => i.remove());
  return (copy.textContent ?? '').replace(/ /g, ' ').replace(/\s+/g, ' ').trim();
};

const options = [
  {
    key: 'detect',
    name: 'Detect',
    namePt: 'Detectar',
    cost: 1,
    kind: 'text',
    text: 'x',
    affordable: true,
    attackKey: '',
  },
  {
    key: 'tail',
    name: 'Tail Attack',
    namePt: 'Ataque de cauda',
    cost: 1,
    kind: 'attack',
    text: 'x',
    affordable: true,
    attackKey: 'm#tail',
  },
  {
    key: 'wing',
    name: 'Wing Attack',
    namePt: 'Ataque de asas',
    cost: 2,
    kind: 'move',
    text: 'x',
    affordable: true,
    attackKey: '',
  },
];
const dragon = (left = 2) =>
  combatant({
    id: 'd',
    label: 'Dragão vermelho adulto',
    initiative: 19,
    hitPointsCurrent: 198,
    hitPointsMax: 256,
    legendary: {
      actionsPerRound: 3,
      actionsLeft: left,
      resistancePerDay: 3,
      resistanceLeft: 2,
      usableNow: true,
      options,
    },
  } as never);
const captain = combatant({
  id: 'c',
  label: 'Capitão bandido',
  initiative: 14,
  parryAsk: true,
} as never);
const tor = combatant({
  id: 't',
  label: 'Tor',
  kind: CombatantKind.PLAYER,
  characterId: 'tc',
  initiative: 11,
});
const brisa = combatant({
  id: 'b',
  label: 'Brisa',
  kind: CombatantKind.PLAYER,
  characterId: 'bc',
  initiative: 16,
});
const running = (over: object = {}) =>
  encounter({
    id: 'enc',
    combatants: [dragon(), captain, brisa, tor],
    currentCombatantId: 't',
    round: 4,
    ...over,
  } as never);

const LAPTOP = ['(min-width: 1024px)'];

describe("CombatView, the master's legendary offers", () => {
  it('draws the card of the legendary creature off turn in the turn column, under the queue', async () => {
    const { fixture, el } = mountCombatView({ enc: running(), master: true, matches: LAPTOP });
    await settle(fixture);
    const queue = el.querySelector('app-reaction-queue[part="rest"]')!;
    expect(
      queue.querySelector('app-legendary-offers [data-testid="legendary-card"]'),
    ).not.toBeNull();
    expect(plain(queue.querySelector('[data-testid="legendary-card"] h3'))).toBe(
      'Dragão vermelho adulto',
    );
    expect(plain(queue.querySelector('[data-testid="legendary-badge"]'))).toBe('Opcional');
    // Nothing holds the table: the card of the turn stays first, the optional card follows it.
    const turn = el.querySelector('app-act-as-card, app-npc-card')!;
    expect(turn.compareDocumentPosition(queue) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it('draws it on a phone too, once, with no "Esperando você" title when nothing holds the table', async () => {
    const { fixture, el } = mountCombatView({ enc: running(), master: true });
    await settle(fixture);
    expect(el.querySelectorAll('[data-testid="legendary-card"]')).toHaveLength(1);
    expect(el.querySelector('[data-testid="queue-header"]')).toBeNull();
  });

  it('puts it after the windows the master answers, as "Depois"', async () => {
    const parry = reactionWindow({
      id: 'p',
      kind: ReactionKind.PARRY,
      reactorId: 'c',
      reactorLabel: 'Capitão bandido',
      answerNow: true,
      trigger: { actorId: 't', actorLabel: 'Tor', attackTotal: 16, targetArmorClass: 15 } as never,
      prompt: {
        case: 'parry',
        value: {
          attackerLabel: 'Tor',
          attackNamePt: 'Espada longa',
          acBonus: 2,
          armorClassWithParry: 17,
        },
      },
    } as never);
    const { fixture, el } = mountCombatView({
      enc: running({ reactionWindows: [parry] }),
      master: true,
      matches: LAPTOP,
    });
    await settle(fixture);
    const queue = el.querySelector('app-reaction-queue[part="rest"]')!;
    const kids = Array.from(queue.children).map((c) => c.tagName.toLowerCase());
    expect(kids.indexOf('app-legendary-offers')).toBeGreaterThan(kids.indexOf('app-parry-offer'));
    expect(plain(queue.querySelector('[data-testid="legendary-badge"]'))).toBe('Depois');
    // A window that holds the table stands above the card of the turn: its answer is on the first screen.
    const turn = el.querySelector('app-act-as-card, app-npc-card')!;
    expect(turn.compareDocumentPosition(queue) & Node.DOCUMENT_POSITION_PRECEDING).toBeTruthy();
  });

  it("on the creature's own turn the card is gone and the NPC's card says what came back", async () => {
    const { fixture, el } = mountCombatView({
      enc: running({
        currentCombatantId: 'd',
        combatants: [dragon(3), captain, brisa, tor],
      }),
      master: true,
      matches: LAPTOP,
    });
    await settle(fixture);
    expect(el.querySelector('[data-testid="legendary-card"]')).toBeNull();
    const note = el.querySelector('app-npc-card [data-testid="legendary-recovered"]');
    expect(plain(note)).toContain('Ações lendárias: 3 de 3 , renovadas agora');
    expect(plain(note)).toContain(
      'Ações lendárias se usam no turno de outra criatura, nunca no dele.',
    );
  });

  it('says what is left under "Próximo turno", on a phone and on a laptop', async () => {
    for (const matches of [[], LAPTOP]) {
      const { fixture, el } = mountCombatView({ enc: running(), master: true, matches });
      await settle(fixture);
      const line = el.querySelector('[data-testid="legendary-left"]');
      expect(plain(line)).toBe('Restam 2 ações lendárias: Dragão vermelho adulto.');
      expect(line?.previousElementSibling?.textContent).toContain('Próximo turno');
    }
  });

  it('says nothing under the button when the legendary actions are all spent', async () => {
    const { fixture, el } = mountCombatView({
      enc: running({ combatants: [dragon(0), captain, brisa, tor] }),
      master: true,
    });
    await settle(fixture);
    expect(el.querySelector('[data-testid="legendary-left"]')).toBeNull();
  });

  it('gives a player none of it: no card, no line', async () => {
    const { fixture, el } = mountCombatView({
      enc: running({ combatants: [dragon(), captain, brisa, { ...tor, mine: true }] }),
      matches: LAPTOP,
      inputs: { vitals: [] },
    });
    await settle(fixture);
    expect(el.querySelector('app-legendary-offers')).toBeNull();
    expect(el.querySelector('[data-testid="legendary-left"]')).toBeNull();
    expect(el.textContent).not.toContain('Ações lendárias');
  });

  describe('"Atacar"', () => {
    const tailAttack = create(AttackOptionSchema, {
      attack: create(AttackSchema, {
        key: 'm#tail',
        name: 'Tail',
        namePt: 'Cauda',
        attackBonus: 14,
        rangeFt: 15,
        damage: '2d8 + 8',
      }),
    });

    it("opens the creature's attack sheet with the option, its title and the cost, and spends nothing itself", async () => {
      const { fixture, el, opened, callsOf } = mountCombatView({
        enc: running(),
        master: true,
        matches: LAPTOP,
        turnOptions: (id) =>
          id === 'd' ? ({ options: { attacks: [tailAttack] } } as never) : undefined,
      });
      await settle(fixture);
      Array.from(el.querySelectorAll<HTMLButtonElement>('[data-testid="legendary-row"] button'))
        .find((b) => plain(b) === 'Atacar')!
        .click();
      await settle(fixture);
      const sheet = opened.find((o) => o.component === AttackSheet)!;
      expect(sheet).toBeDefined();
      const data = sheet.config.data as {
        attackerId: string;
        attack: { key: string };
        legendary: { option: string; title: string; subtitle: string };
      };
      expect(data.attackerId).toBe('d');
      expect(data.attack.key).toBe('m#tail');
      expect(data.legendary).toEqual({
        option: 'tail',
        title: 'Ataque de cauda · ação lendária',
        subtitle: 'Dragão vermelho adulto · custa 1 de 2 restantes',
        cost: 1,
        label: 'Dragão vermelho adulto',
      });
      expect(callsOf('spendLegendaryAction')).toHaveLength(0);
    });
  });

  describe('"Usar"', () => {
    it('spends the option of the card with the encounter and the creature', async () => {
      const enc = running();
      const { fixture, el, callsOf } = mountCombatView({
        enc,
        master: true,
        matches: LAPTOP,
        api: { spendLegendaryAction: () => enc },
      });
      await settle(fixture);
      Array.from(el.querySelectorAll<HTMLButtonElement>('[data-testid="legendary-row"] button'))
        .find((b) => plain(b) === 'Usar')!
        .click();
      await settle(fixture);
      expect(callsOf('spendLegendaryAction')[0].args.slice(0, 4)).toEqual([
        'c',
        'enc',
        'd',
        'detect',
      ]);
    });

    it("arms the move of its option: the next drag of the creature's token walks it with the option", async () => {
      const { fixture, el, view, callsOf } = mountCombatView({
        enc: running({ combatants: [dragon(3), captain, brisa, tor] }),
        master: true,
        matches: LAPTOP,
      });
      await settle(fixture);
      el.querySelectorAll<HTMLButtonElement>('[data-testid="legendary-row"] button')[2].click();
      await settle(fixture);
      expect(view.moves.legendaryMove()).toEqual({ combatantId: 'd', optionKey: 'wing' });
      expect(plain(el.querySelector('[data-testid="legendary-armed"]'))).toContain(
        'Arraste a ficha',
      );
      await view.moves.drop({ id: 'd', col: 3, row: 4 });
      await settle(fixture);
      const move = callsOf('moveCombatant').length ? callsOf('moveCombatant') : callsOf('move');
      expect(move[0].args[8]).toBe('wing');
      expect(view.moves.legendaryMove()).toBeNull();
    });
  });

  describe('the log', () => {
    it("writes a spend with the option's Portuguese name, for the master", async () => {
      const spent = {
        id: 'ev',
        kind: CombatLogKind.LEGENDARY_SPENT,
        actorId: 'd',
        actorLabel: 'Dragão vermelho adulto',
        hidden: true,
        undoable: true,
        legendarySpent: { optionName: 'Wing Attack', cost: 2 },
      };
      const { fixture, el } = mountCombatView({
        enc: running(),
        master: true,
        matches: ['(min-width: 1024px)', '(min-width: 1280px)'],
        api: { log: () => ({ rounds: [{ round: 4, entries: [spent] }], undoableEventId: 'ev' }) },
      });
      await settle(fixture);
      el.querySelector<HTMLButtonElement>('.more .mr-fold__button')!.click();
      await settle(fixture);
      const log = plain(el.querySelector('app-combat-log-panel'));
      expect(log).toContain('gastou 2 ações lendárias: Ataque de asas');
      expect(log).not.toContain('Wing Attack');
      // The same spend is the line of the card; the undo is the log's, so the card has no button for it.
      const card = el.querySelector('[data-testid="legendary-spent"]');
      expect(plain(card)).toContain('Gastou 2: Ataque de asas.');
      expect(card?.querySelector('button')).toBeNull();
    });
  });

  describe('the Parry switch', () => {
    it('calls the client for the creature with the mode it is switched to', async () => {
      const enc = running();
      const { fixture, el, callsOf } = mountCombatView({
        enc,
        master: true,
        matches: LAPTOP,
        api: { setMonsterReactionMode: () => enc },
      });
      await settle(fixture);
      el.querySelectorAll<HTMLButtonElement>('.row__head').forEach((h) => h.click());
      await settle(fixture);
      el.querySelector<HTMLButtonElement>(
        'button[aria-label="Mais ações para Capitão bandido"]',
      )!.click();
      await settle(fixture);
      Array.from(document.querySelectorAll<HTMLElement>('.mat-mdc-menu-item'))
        .find((i) => i.textContent?.includes('Aparar: perguntar a cada golpe'))!
        .click();
      await settle(fixture);
      expect(callsOf('setMonsterReactionMode')[0].args.slice(0, 4)).toEqual([
        'c',
        'enc',
        'c',
        false,
      ]);
    });
  });
});
