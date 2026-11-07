import { ReactionKind } from '../../../gen/meurpg/play/v1/combat_pb';
import { reactionWindow } from './combat-testing';
import { parryCard } from './parry-card';

function parry(total: number, ac = 15, bonus = 2) {
  return reactionWindow({
    id: 'p',
    kind: ReactionKind.PARRY,
    reactorId: 'c',
    reactorLabel: 'Capitão bandido',
    trigger: { actorId: 't', actorLabel: 'Tor', attackTotal: total, targetArmorClass: ac } as never,
    prompt: {
      case: 'parry',
      value: {
        attackerLabel: 'Tor',
        attackNamePt: 'Espada longa',
        acBonus: bonus,
        armorClassWithParry: ac + bonus,
      },
    },
  } as never);
}

describe("the master's Parry window", () => {
  it('turns the hit into a miss when the armor class with Parry beats it', () => {
    const c = parryCard(parry(16), true);
    expect(c?.title).toBe('Aparar o golpe de Tor?');
    expect(c?.text).toBe(
      'Tor acertou com a Espada longa: 16 contra CA 15. Aparar soma 2: contra CA 17, vira erro.',
    );
    expect(c?.stops).toBe(true);
    expect(c?.passLabel).toBe('Deixar passar');
    expect(c?.useLabel).toBe('Aparar (+2 CA)');
    expect(c?.reactor).toBe('Capitão bandido');
  });

  it('says the hit lands anyway when +AC would not stop it', () => {
    const c = parryCard(parry(22));
    expect(c?.text).toBe(
      'Tor acertou com a Espada longa: 22 contra CA 15. Aparar soma 2: contra CA 17, ainda acerta.',
    );
    expect(c?.stops).toBe(false);
  });

  it('takes the article of an NPC attacker and has no card without its prompt', () => {
    const w = parry(16);
    if (w.prompt.case === 'parry') {
      w.prompt.value.attackerLabel = 'Goblin 1';
    }
    expect(parryCard(w, false)?.title.replace(/\u00a0/g, ' ')).toBe('Aparar o golpe do Goblin 1?');
    expect(parryCard(reactionWindow({ id: 'x', kind: ReactionKind.PARRY }))).toBeNull();
  });

  it('names a player attacker without an article, in the title and the text', () => {
    const c = parryCard(parry(16), true);
    expect(c?.title).toBe('Aparar o golpe de Tor?');
    expect(c?.text.startsWith('Tor acertou com a Espada longa')).toBe(true);
  });
});
