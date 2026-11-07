// What a player reads while something that is the master's holds the table or the player's own attack (RN-10):
// the same neutral card, whichever window it is. The server writes the words (`Encounter.reaction_wait`: it names no
// reaction, no creature and no number, and the leak tests of the backend hold it to that); the screen draws them in
// the card and never adds a thing it knows.
import { CombatantKind, ReactionKind } from '../../../../gen/meurpg/play/v1/combat_pb';
import { combatant, encounter, reactionWindow } from '../../../core/combat/combat-testing';
import { settle } from '../../../../test-settle';
import { mountCombatView } from './combat-view-testing';

const plain = (t: string | null | undefined) =>
  (t ?? '').replace(/ /g, ' ').replace(/\s+/g, ' ').trim();

const toren = combatant({
  id: 't',
  label: 'Toren',
  kind: CombatantKind.PLAYER,
  characterId: 'toren-c',
  mine: true,
  placed: false,
});
const captain = combatant({ id: 'c', label: 'Capitão bandido', placed: false });
const dragon = combatant({ id: 'd', label: 'Dragão vermelho adulto', placed: false });

/** What the card must not carry from the table: a name, a reaction, a number. */
const NAMED = /Aparar|Parry|Resistência|Escudo|Capitão|Dragão|Brisa|Toren|lendári|\d/;

/** Three tables that differ in who the NPCs are and which window holds it (the player is given none of them). */
const TABLES = [
  {
    name: 'an NPC with a Parry in ask mode',
    npcs: [
      combatant({ id: 'c', label: 'Capitão bandido', placed: false, parryAsk: true } as never),
    ],
    window: { kind: ReactionKind.SHIELD, reactorId: 'c', reactorLabel: 'Capitão bandido' },
    detail: 'O turno continua quando ele responder.',
  },
  {
    name: 'an NPC without a Parry',
    npcs: [combatant({ id: 'c', label: 'Dragão vermelho adulto', placed: false })],
    window: { kind: ReactionKind.CONCENTRATION_SAVE, reactorId: 'c', reactorLabel: 'Dragão' },
    detail: 'A sua conjuração se resolve quando ele responder.',
  },
  {
    name: "a player's own ask-mode window",
    npcs: [
      combatant({ id: 'c', label: 'Capitão bandido', placed: false }),
      combatant({ id: 'b', label: 'Brisa', kind: CombatantKind.PLAYER, characterId: 'bc' }),
    ],
    window: {
      kind: ReactionKind.MASTER_CHECK,
      reactorId: 'b',
      reactorLabel: 'Brisa',
      reactorIsPlayer: true,
    },
    detail: 'O resultado do seu ataque sai quando ele responder.',
  },
];

describe('CombatView, the player waits for the master', () => {
  function player(current: string, detailPt: string) {
    // `ForYou: false`: the server gives the player the wait line, not the window.
    const enc = encounter({
      combatants: [toren, captain, dragon],
      currentCombatantId: current,
      reactionWait: { titlePt: 'Esperando o mestre', detailPt },
    } as never);
    return mountCombatView({ enc, matches: ['(min-width: 1024px)'] });
  }

  for (const t of TABLES) {
    it(`reads the server's title and detail verbatim, and nothing of the table, while ${t.name} holds it`, async () => {
      const enc = encounter({
        combatants: [toren, ...t.npcs],
        currentCombatantId: 'c',
        reactionWait: { titlePt: 'Esperando o mestre', detailPt: t.detail },
        reactionWindows: [
          reactionWindow({ id: 'w', forYou: false, answerNow: false, ...t.window }),
        ],
      } as never);
      const { fixture, el } = mountCombatView({ enc, matches: ['(min-width: 1024px)'] });
      await settle(fixture);
      const card = el.querySelector('[data-testid="reaction-wait"]')!;
      expect(card.tagName.toLowerCase()).toBe('app-master-wait');
      expect(card.getAttribute('role')).toBe('status');
      const text = plain(card.textContent).replace('hourglass_top', '');
      expect(text).toBe(`Esperando o mestre. ${t.detail}`);
      expect(text).not.toMatch(NAMED);
    });
  }

  it("reads the same card in the player's own turn, with the end of the turn off", async () => {
    const { fixture, el } = player('t', 'O turno continua quando ele responder.');
    await settle(fixture);
    const cards = el.querySelectorAll('app-turn-panel app-master-wait');
    expect(cards.length).toBeGreaterThan(0);
    expect(plain(cards[0].textContent)).toContain('Esperando o mestre. O turno continua');
    const end = [...el.querySelectorAll<HTMLButtonElement>('app-end-turn button')].find((b) =>
      plain(b.textContent).includes('Encerrar turno'),
    );
    expect(end?.getAttribute('aria-disabled')).toBe('true');
  });

  it("says only the server's detail: the attack, the spell or the turn", async () => {
    for (const detail of [
      'O resultado do seu ataque sai quando ele responder.',
      'A sua conjuração se resolve quando ele responder.',
    ]) {
      const { fixture, el } = player('c', detail);
      await settle(fixture);
      expect(plain(el.querySelector('[data-testid="reaction-wait"]')?.textContent)).toContain(
        detail,
      );
    }
  });

  it('shows no master control of the legendary or Parry offers: no card, no line, no switch', async () => {
    const { fixture, el } = player('c', 'O turno continua quando ele responder.');
    await settle(fixture);
    expect(el.querySelector('app-legendary-offers')).toBeNull();
    expect(el.querySelector('app-legendary-card')).toBeNull();
    expect(el.querySelector('app-parry-offer')).toBeNull();
    expect(el.querySelector('app-resistance-offer')).toBeNull();
    expect(plain(el.textContent)).not.toMatch(/Aparar:|ações lendárias|Resistência Lendária/);
  });

  it("keeps a player's own reaction named as the server says, in the plain notice", async () => {
    const enc = encounter({
      combatants: [toren, captain],
      currentCombatantId: 'c',
      reactionWait: {
        titlePt: 'Esperando a reação de Sálvia',
        detailPt: 'O turno continua quando ela responder.',
      },
    } as never);
    const { fixture, el } = mountCombatView({ enc, matches: ['(min-width: 1024px)'] });
    await settle(fixture);
    expect(el.querySelector('app-master-wait')).toBeNull();
    expect(plain(el.querySelector('[data-testid="reaction-wait"]')?.textContent)).toContain(
      'Esperando a reação de Sálvia.',
    );
  });
});
