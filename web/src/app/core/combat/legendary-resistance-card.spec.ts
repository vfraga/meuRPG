import { ReactionKind } from '../../../gen/meurpg/play/v1/combat_pb';
import { reactionWindow } from './combat-testing';
import {
  canConfirmResistance,
  legendaryResistanceCard,
  resistanceBox,
  resistanceIds,
} from './legendary-resistance-card';

const offer = (id: string, label: string, d20: number, total: number, left = 3, over = {}) => ({
  combatantId: id,
  label,
  d20,
  total,
  dc: 15,
  usesLeft: left,
  usesPerDay: 3,
  ...over,
});

function window(offers: object[]) {
  return reactionWindow({
    id: 'lr',
    kind: ReactionKind.LEGENDARY_RESISTANCE,
    trigger: {
      actorId: 'pen',
      actorLabel: 'Pensantus',
      actionNamePt: 'Relâmpago',
      spellKey: 'lightning-bolt',
    } as never,
    prompt: { case: 'legendaryResistance', value: { offers } },
  } as never);
}

describe('the Legendary Resistance window', () => {
  const two = legendaryResistanceCard(
    window([offer('d', 'Dragão vermelho adulto', 4, 10), offer('v', 'Vampiro', 7, 11, 1)]),
    true,
  );

  it('asks for the failures, names the source and every failed roll', () => {
    expect(two?.title).toBe('Transformar falhas em sucesso?');
    expect(two?.source).toBe(
      'Relâmpago de Pensantus · teste de resistência, CD 15. A magia espera a sua resposta; os dados já foram rolados.',
    );
    expect(two?.rows.map((r) => r.failed)).toEqual([
      'Dragão vermelho adulto falhou: d20 4 + 6 = 10 contra CD 15',
      'Vampiro falhou: d20 7 + 4 = 11 contra CD 15',
    ]);
    expect(two?.rows[0].line).toBe('vira sucesso · gasta 1 · restam 3 de 3 hoje');
    expect(two?.rows[1].line).toBe('vira sucesso · gasta 1 · restam 1 de 3 hoje');
    expect(two?.rows[0].checkboxLabel).toBe('Usar Resistência Lendária');
  });

  it('starts with nothing ticked, declines first and confirms only the ticked', () => {
    expect(two?.rows.every((r) => !r.checked)).toBe(true);
    expect(two?.declineLabel).toBe('Deixar como rolou');
    expect(two?.confirmLabel).toBe('Usar nas marcadas');
    expect(two?.footer).toBe(
      'Os jogadores veem “Esperando o mestre” e, depois, o resultado do teste.',
    );
    if (!two) {
      throw new Error('no card');
    }
    expect(canConfirmResistance(two, new Set())).toBe(false);
    expect(canConfirmResistance(two, new Set(['v', 'ghost']))).toBe(true);
    expect(resistanceIds(two, new Set(['v', 'd', 'ghost']))).toEqual(['d', 'v']);
  });

  it('speaks in the singular for one creature and reads a raised d20', () => {
    const one = legendaryResistanceCard(
      window([offer('d', 'Dragão vermelho adulto', 1, 10, 3, { treatedAs: 8 })]),
    );
    expect(one?.title).toBe('Transformar a falha em sucesso?');
    expect(one?.rows[0].failed).toBe(
      'Dragão vermelho adulto falhou: d20 1, contou como 8 + 2 = 10 contra CD 15',
    );
  });

  it('says an automatic failure (d20 and total 0: stunned, paralyzed) without a roll, and keeps a real roll of 0 or less', () => {
    const auto = legendaryResistanceCard(window([offer('d', 'Dragão vermelho adulto', 0, 0)]));
    expect(auto?.rows[0].failed).toBe('Dragão vermelho adulto falhou sem rolar contra CD 15');
    // A penalty can take a real total to 0 or below, but the d20 is still a face that came up.
    const penalty = legendaryResistanceCard(window([offer('d', 'Dragão vermelho adulto', 2, -1)]));
    expect(penalty?.rows[0].failed).toContain('d20 2');
  });

  it('has no checkbox for one creature: a plain line and a primary that uses it', () => {
    const one = legendaryResistanceCard(window([offer('d', 'Dragão vermelho adulto', 4, 10)]));
    if (!one) {
      throw new Error('no card');
    }
    expect(one.mode).toBe('one');
    expect(one.rows[0].line).toBe('Resistência Lendária: restam 3 de 3 hoje · usar gasta 1');
    expect(one.confirmLabel).toBe('Usar Resistência Lendária');
    expect(one.declineLabel).toBe('Deixar como rolou');
    expect(canConfirmResistance(one, new Set())).toBe(true);
    expect(resistanceIds(one, new Set())).toEqual(['d']);
    expect(two?.mode).toBe('many');
  });

  it('does not invent a DC the offers do not share, and has no card without its prompt', () => {
    const c = legendaryResistanceCard(
      window([offer('d', 'A', 4, 10), offer('v', 'B', 7, 11, 1, { dc: 17 })]),
    );
    expect(c?.source).toContain('teste de resistência. A magia espera');
    expect(legendaryResistanceCard(reactionWindow({ id: 'z' }))).toBeNull();
  });
});

describe('"Se falhar, usar Resistência Lendária" on the master\'s own save', () => {
  it('shows only with the trait and a use left, unchecked', () => {
    expect(resistanceBox({ resistanceLeft: 2, resistancePerDay: 3 })).toEqual({
      label: 'Se falhar, usar Resistência Lendária',
      line: 'gasta 1 · restam 2 de 3 hoje',
      checked: false,
    });
    expect(resistanceBox({ resistanceLeft: 0, resistancePerDay: 3 })).toBeNull();
    expect(resistanceBox({ resistanceLeft: 0, resistancePerDay: 0 })).toBeNull();
    expect(resistanceBox(undefined)).toBeNull();
  });

  it('uses a neutral source when the trigger has no action name', () => {
    const w = reactionWindow({
      id: 'n',
      kind: ReactionKind.LEGENDARY_RESISTANCE,
      trigger: { actorId: 'x', actorLabel: 'Tor', actionNamePt: '' } as never,
      prompt: { case: 'legendaryResistance', value: { offers: [offer('d', 'A', 4, 10)] } },
    } as never);
    expect(legendaryResistanceCard(w)?.source).toBe(
      'Teste de resistência, CD 15. A ação espera a sua resposta; os dados já foram rolados.',
    );
  });

  it('names a spell of an NPC caster with its article', () => {
    const w = reactionWindow({
      id: 'n2',
      kind: ReactionKind.LEGENDARY_RESISTANCE,
      trigger: {
        actorId: 'm',
        actorLabel: 'Mago 1',
        actionNamePt: 'Relâmpago',
        spellKey: 'lightning-bolt',
      } as never,
      prompt: { case: 'legendaryResistance', value: { offers: [offer('d', 'A', 4, 10)] } },
    } as never);
    expect(legendaryResistanceCard(w, false)?.source.replace(/\u00a0/g, ' ')).toContain(
      'Relâmpago do Mago 1 ·',
    );
  });

  it('says the action, not the magic, for a rider save (no spell key), and the magic for a cast', () => {
    const mk = (trigger: object) =>
      legendaryResistanceCard(
        reactionWindow({
          id: 'r',
          kind: ReactionKind.LEGENDARY_RESISTANCE,
          trigger: trigger as never,
          prompt: { case: 'legendaryResistance', value: { offers: [offer('d', 'A', 4, 10)] } },
        } as never),
      )?.source;
    expect(mk({ actorId: 'm', actorLabel: 'Tor', actionNamePt: 'Golpe Atordoante' })).toContain(
      'A ação espera a sua resposta',
    );
    expect(
      mk({
        actorId: 'm',
        actorLabel: 'Tor',
        actionNamePt: 'Relâmpago',
        spellKey: 'lightning-bolt',
      }),
    ).toContain('A magia espera a sua resposta');
  });
});
