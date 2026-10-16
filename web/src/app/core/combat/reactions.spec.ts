import {
  ReactionKind,
  ReactionRollKind,
  AttackOutcome,
  ReactionWindowStatus,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { encounter, reactionWindow } from './combat-testing';
import {
  concentrationResultView,
  contestIdOf,
  contestWindows,
  dieSidesOf,
  openWindows,
  promptView,
  rebukeRows,
  reactionWait,
  resultView,
  sheetWindow,
  slotChoices,
  throwText,
  windowForPlayer,
} from './reactions';

const plain = (text: string) => text.replace(/ /g, ' ');

function win(case_: string, value: object, over: object = {}) {
  return reactionWindow({
    id: `w-${case_}`,
    prompt: { case: case_, value } as never,
    ...over,
  } as never);
}

describe('a contest waits in a window of kind CONTEST (W7-X)', () => {
  const contest = reactionWindow({
    id: 'wc',
    kind: ReactionKind.CONTEST,
    prompt: { case: 'contest', value: { contestId: 'ct1' } } as never,
  });

  it('is never a reaction card, a queue or a sheet of the reactions: the contest sheets answer it', () => {
    const e = encounter({ reactionWindows: [contest] } as never);
    expect(openWindows(e)).toEqual([]);
    expect(windowForPlayer(e)).toBeUndefined();
    expect(sheetWindow(e)).toBeUndefined();
  });

  it('keeps the other windows in their order beside it', () => {
    const shield = win('shield', { slots: [] }, { id: 'shield' });
    const e = encounter({ reactionWindows: [contest, shield] } as never);
    expect(openWindows(e).map((w) => w.id)).toEqual(['shield']);
    expect(sheetWindow(e)?.id).toBe('shield');
  });

  it('is found by the contest sheets, with the contest it waits for', () => {
    const closed = reactionWindow({
      id: 'old',
      kind: ReactionKind.CONTEST,
      status: ReactionWindowStatus.ANSWERED,
    });
    const e = encounter({ reactionWindows: [contest, closed] } as never);
    expect(contestWindows(e).map((w) => w.id)).toEqual(['wc']);
    expect(contestIdOf(contest)).toBe('ct1');
    expect(contestIdOf(reactionWindow({ id: 'x' }))).toBe('');
  });

  it('is told to everybody else with the combat’s own wait line', () => {
    const e = encounter({
      reactionWindows: [],
      reactionWait: {
        titlePt: 'Esperando Brisa',
        detailPt: 'O turno continua quando ele responder.',
      },
    } as never);
    expect(reactionWait(e)).toEqual({
      title: 'Esperando Brisa',
      detail: 'O turno continua quando ele responder.',
    });
  });
});

describe('the reaction windows a player answers', () => {
  it('lists the open windows and gives a player the first one that is theirs', () => {
    const answered = reactionWindow({ id: 'a', status: ReactionWindowStatus.ANSWERED });
    const notMine = reactionWindow({ id: 'b', forYou: false });
    const mine = reactionWindow({ id: 'c' });
    const e = encounter({ reactionWindows: [answered, notMine, mine] } as never);
    expect(openWindows(e).map((w) => w.id)).toEqual(['b', 'c']);
    expect(windowForPlayer(e)?.id).toBe('c');
  });

  it('opens a sheet only for a window that has a prompt of its own for the player', () => {
    const check = win('masterCheck', {}, { id: 'check', kind: ReactionKind.MASTER_CHECK });
    const save = win('hellishRebukeSave', {}, { id: 'save' });
    const none = reactionWindow({ id: 'none', kind: ReactionKind.OPPORTUNITY });
    const shield = win('shield', { slots: [] }, { id: 'shield' });
    expect(
      sheetWindow(encounter({ reactionWindows: [check, save, none] } as never)),
    ).toBeUndefined();
    expect(sheetWindow(encounter({ reactionWindows: [check, shield] } as never))?.id).toBe(
      'shield',
    );
  });

  it('reads the wait the server wrote, and none when nothing waits', () => {
    expect(reactionWait(encounter())).toBeNull();
    expect(
      reactionWait(
        encounter({
          reactionWait: {
            titlePt: 'Esperando o mestre',
            detailPt: 'O turno continua quando ele responder.',
          },
        } as never),
      ),
    ).toEqual({ title: 'Esperando o mestre', detail: 'O turno continua quando ele responder.' });
  });
});

describe('the prompt of each reaction', () => {
  it('keeps the Escudo copy: "Você foi atingido", the attacker, and the cost in the question', () => {
    const v = promptView(
      win('shield', {
        attackerLabel: 'Capitão Goblin',
        attackNamePt: 'Cimitarra',
        slots: [],
        spellNamePt: 'Escudo Arcano',
      }),
      2,
    )!;
    expect(v.title).toBe('Você foi atingido');
    expect(plain(v.subtitle)).toBe('Capitão Goblin · Cimitarra · Rodada 2');
    expect(v.question).toBe(
      'Usar Escudo Arcano? A sua CA sobe 5 até o começo do seu próximo turno, e o ataque pode virar erro. Gasta a sua reação e um espaço de magia.',
    );
    expect(v.useLabel).toBe('Conjurar Escudo Arcano');
  });

  it('says only the round when the attacker is hidden, and takes the damage away for Mísseis Mágicos', () => {
    const hidden = promptView(win('shield', { attackerLabel: '', slots: [] }), 3)!;
    expect(hidden.subtitle).toBe('Rodada 3');
    const missile = promptView(win('shield', { magicMissile: true, slots: [] }), 3)!;
    expect(missile.question).toContain('Mísseis Mágicos');
  });

  it('Esquiva Sobrenatural: the damage and the half, the reaction as the only cost', () => {
    const v = promptView(
      win('uncannyDodge', {
        attackerLabel: 'Hobgoblin',
        attackNamePt: 'Espada longa',
        damage: 9,
        halved: 4,
      }),
      2,
    )!;
    expect(v.question).toBe(
      'Usar Esquiva Sobrenatural? O ataque causaria 9 de dano; ele cai pela metade, para 4. Gasta a sua reação.',
    );
    expect(v.costs).toEqual(['Reação', 'Sem espaço de magia']);
  });

  it('says on every prompt that the reaction is spent until the next turn', () => {
    const spent = 'só volta no começo do seu próximo turno';
    const prompts = [
      win('hellishRebuke', { aggressorLabel: 'Hobgoblin', saveDc: 13, options: [] }),
      win('counterspell', { casterLabel: 'Mago 1', slotOptions: [] }),
      win('cuttingWords', { rollerLabel: 'Hobgoblin', dieSides: 8, usesLeft: 1 }),
      win('deflectMissiles', { attackerLabel: 'Arqueiro', dieSides: 10, dexMod: 3, monkLevel: 5 }),
      win('maneuverReduce', { attackerLabel: 'Ogro', options: [] }),
      win('featherFall', { falling: [], fallFt: 30 }),
    ];
    for (const w of prompts) {
      expect(promptView(w, 2)!.question).toContain(spent);
    }
  });

  it('Contramágica: the caster and the reach, never the spell or its level', () => {
    const v = promptView(
      win('counterspell', { casterLabel: 'Mago 1', distanceFt: 30, slotOptions: [] }),
      2,
    )!;
    expect(v.title).toBe('Mago 1 está conjurando');
    expect(plain(v.subtitle)).toMatch(/^Mago 1 · a [\d,]+ m de você · Rodada 2$/);
    expect(v.question).not.toMatch(/Bola de Fogo|nível da magia é/);
    expect(v.note).toContain('não sabe qual magia é nem o nível dela');
    expect(plain(v.costs[1])).toContain('Alcance 18 m: está a');
  });

  it('Palavras de Interrupção: who rolls what, the die and the uses', () => {
    const v = promptView(
      win('cuttingWords', {
        rollKind: ReactionRollKind.ATTACK,
        rollerLabel: 'Hobgoblin',
        targetLabel: 'Toren',
        dieSides: 8,
        usesLeft: 3,
      }),
      2,
    )!;
    expect(v.title).toBe('O Hobgoblin vai atacar');
    expect(v.subtitle).toContain('Hobgoblin ataca o Toren');
    expect(v.question).toContain('dado de Inspiração de Bardo (d8) da rolagem de ataque');
    expect(v.costs).toContain('Inspiração de Bardo: 3 usos');
    const test = promptView(
      win('cuttingWords', {
        rollKind: ReactionRollKind.TEST,
        rollerLabel: 'Goblin 2',
        dieSides: 6,
        usesLeft: 1,
      }),
      2,
    )!;
    expect(test.title).toBe('O Goblin 2 vai fazer um teste');
    expect(test.costs).toContain('Inspiração de Bardo: 1 uso');
  });

  it('Defletir Projéteis: 1d10 plus the Destreza and the monk level', () => {
    const v = promptView(
      win('deflectMissiles', {
        attackerLabel: 'Goblin',
        damage: 9,
        dieSides: 10,
        dexMod: 3,
        monkLevel: 5,
        flatBonus: 8,
      }),
      2,
    )!;
    expect(v.title).toBe('Você foi atingido à distância');
    expect(v.question).toContain('1d10 + 3 (Destreza) + 5 (nível de monge)');
    expect(v.note).toContain('causaria 9 de dano');
  });

  it('Queda Suave: names who falls, or counts them', () => {
    const one = promptView(
      win('featherFall', {
        falling: [{ combatantId: 't', label: 'Toren', ally: true, distanceFt: 15 }],
        maxTargets: 5,
        fallFt: 20,
        slotOptions: [],
      }),
      2,
    )!;
    expect(one.title).toBe('Toren está caindo');
    const two = promptView(
      win('featherFall', {
        falling: [
          { combatantId: 't', label: 'Toren', ally: true },
          { combatantId: 'g', label: 'Goblin 2', ally: false },
        ],
        maxTargets: 5,
        fallFt: 20,
        slotOptions: [],
      }),
      2,
    )!;
    expect(two.title).toBe('2 criaturas estão caindo');
  });

  it('the concentration save: the DC the server sent and the minimum of 10', () => {
    const v = promptView(
      win('concentrationSave', {
        spellNamePt: 'Constrição',
        damageTaken: 28,
        dc: 14,
        sourceLabel: 'Bola de Fogo do Mago 1',
        saveBonus: 2,
        bonusKnown: true,
      }),
      2,
    )!;
    expect(v.title).toBe('Concentração em risco');
    expect(plain(v.subtitle)).toBe('Bola de Fogo do Mago 1 · você sofreu 28 de dano · Rodada 2');
    expect(v.question).toBe(
      'Você mantém a concentração em Constrição. Faça um teste de resistência de Constituição contra CD 14 (metade dos 28 de dano; o mínimo é 10).',
    );
  });

  it("has no player sheet for the master's check", () => {
    expect(promptView(win('masterCheck', {}), 2)).toBeNull();
  });

  it('lists the slots a window offers and the die it rolls', () => {
    const s = win('counterspell', {
      casterLabel: 'M',
      slotOptions: [{ level: 3, pact: false, free: 2 }],
    });
    expect(slotChoices(s)).toHaveLength(1);
    expect(dieSidesOf(win('cuttingWords', { dieSides: 8 }))).toBe(8);
    expect(dieSidesOf(s)).toBe(0);
  });

  it('lists Repreensão Infernal as the Infernal Legacy first, then the pact slot', () => {
    const rows = rebukeRows(
      win('hellishRebuke', {
        aggressorLabel: 'Hobgoblin',
        saveDc: 13,
        options: [
          { racial: true, level: 2, diceCount: 3, usesLeft: 1 },
          { racial: false, level: 2, diceCount: 3, slot: { level: 2, pact: true, free: 2 } },
        ],
      }),
    );
    expect(rows.map((r) => r.title)).toEqual(['Legado Infernal', 'Espaço de pacto']);
    expect(plain(rows[0].detail)).toBe('2º nível, sem gastar espaço · 1 uso por descanso longo');
    expect(plain(rows[1].detail)).toBe('2º nível · 2 livres');
    expect(rows.every((r) => r.enabled)).toBe(true);
  });
});

describe('the result of an answered reaction', () => {
  const ctx = { round: 2, after: { level: 3, free: 1, total: 2 }, armorClass: 15 };

  it('Escudo: the hit stopped, and the new armor class', () => {
    const asked = win('shield', {
      attackerLabel: 'Capitão Goblin',
      spellNamePt: 'Escudo Arcano',
      slots: [],
    });
    const v = resultView(
      { used: true, result: { case: 'shield', value: { stopped: true } } } as never,
      asked,
      ctx,
    );
    expect(v.title).toBe('Escudo Arcano conjurado');
    expect(v.pill).toEqual({ word: 'Errou', good: true });
    expect(plain(v.text[0])).toBe('O Escudo Arcano segurou o ataque do Capitão Goblin.');
    expect(v.text[1]).toBe('Sua CA é 20 até o começo do seu próximo turno.');
    expect(plain(v.chips.join('|'))).toContain('Reação usada|Espaços de 3º nível: 1 livre de 2');
    const still = resultView(
      { used: true, result: { case: 'shield', value: { stopped: false } } } as never,
      asked,
      ctx,
    );
    expect(still.pill?.word).toBe('Acertou');
  });

  it('Repreensão Infernal: a halfling’s save the app rolled again says "Sortudo: 1 → 15"', () => {
    const asked = win('hellishRebuke', { aggressorLabel: 'Pipo', saveDc: 13, options: [] });
    const save = (rerolled: object[]) => ({
      diceCount: 1,
      diceSides: 20,
      faces: [15],
      modifier: 4,
      total: 19,
      rerolled,
    });
    const answer = (rerolled: object[]) =>
      resultView(
        {
          used: true,
          result: {
            case: 'hellishRebuke',
            value: { save: save(rerolled), saveDc: 13, saved: true, damage: 6, level: 1 },
          },
        } as never,
        asked,
        ctx,
      );
    const lucky = answer([{ index: 0, from: 1, to: 15, sourceKey: 'trait:lucky' }]);
    expect(plain(lucky.text[0])).toContain('1d20 (15) + 4 = 19 contra CD 13');
    expect(lucky.text[1]).toBe('Sortudo: 1 → 15');
    expect(answer([]).text).toHaveLength(1);
  });

  it('Contramágica: reveals the spell and the level now that it happened', () => {
    const asked = win('counterspell', { casterLabel: 'Mago 1', slotOptions: [] });
    const v = resultView(
      {
        used: true,
        result: {
          case: 'counterspell',
          value: { countered: true, spellNamePt: 'Bola de Fogo', spellLevel: 3, checkDc: 0 },
        },
      } as never,
      asked,
      ctx,
    );
    expect(v.title).toBe('Contramágica usada');
    expect(v.pill).toEqual({ word: 'Anulada', good: true });
    expect(plain(v.text[0])).toBe('A magia do Mago 1 foi anulada: Bola de Fogo, de 3º nível.');
    expect(v.text[1]).toContain('não houve teste');
  });

  it('Palavras de Interrupção: only "acertou" or "errou", and "Sem efeito" for a creature that cannot hear', () => {
    const asked = win('cuttingWords', {
      rollerLabel: 'Hobgoblin',
      targetLabel: 'Toren',
      dieSides: 8,
      usesLeft: 3,
    });
    const v = resultView(
      {
        used: true,
        result: {
          case: 'cuttingWords',
          value: {
            effective: true,
            die: { diceSides: 8, total: 4 },
            outcome: AttackOutcome.MISS,
            rollKind: ReactionRollKind.ATTACK,
          },
        },
      } as never,
      asked,
      ctx,
    );
    expect(plain(v.text[0])).toContain('O d8 saiu 4 e foi subtraído da rolagem.');
    expect(plain(v.text[0])).toContain('errou o Toren');
    const none = resultView(
      {
        used: true,
        result: {
          case: 'cuttingWords',
          value: { effective: false, rollKind: ReactionRollKind.ATTACK },
        },
      } as never,
      asked,
      ctx,
    );
    expect(none.text).toEqual(['Sem efeito.']);
  });

  it('Defletir Projéteis: caught, and the throw back is offered with the chi in the second step', () => {
    const asked = win('deflectMissiles', { attackerLabel: 'Goblin', dieSides: 10, damage: 9 });
    const v = resultView(
      {
        used: true,
        nextWindowId: 'w2',
        result: {
          case: 'deflectMissiles',
          value: {
            reduction: {
              diceCount: 1,
              diceSides: 10,
              faces: [7],
              modifier: 8,
              total: 15,
              physical: false,
            },
            damageBefore: 9,
            damageAfter: 0,
            caught: true,
            throwBackAvailable: true,
          },
        },
      } as never,
      asked,
      ctx,
    );
    expect(v.title).toBe('Você apanhou a flecha');
    expect(v.throwBack).toBe(true);
    expect(v.text[0]).toContain('o dano, de 9, caiu a 0');
    expect(plain(throwText(4, 20, 60))).toMatch(
      /^Devolver o ataque\? Gasta 1 de chi \(4 restantes\) e faz parte da mesma reação: um ataque à distância com o projétil, com proficiência, alcance [\d,]+ m e [\d,]+ m\.$/,
    );
  });

  it('Queda Suave: names the creatures saved, in the singular and the plural', () => {
    const asked = win('featherFall', {
      falling: [
        { combatantId: 't', label: 'Toren', ally: true },
        { combatantId: 'g', label: 'Goblin 2', ally: false },
      ],
      maxTargets: 5,
      slotOptions: [],
    });
    const one = resultView(
      { used: true, result: { case: 'featherFall', value: { savedIds: ['t'] } } } as never,
      asked,
      ctx,
    );
    expect(one.text[0]).toContain('Toren desce devagar e não sofreu dano de queda');
    const both = resultView(
      { used: true, result: { case: 'featherFall', value: { savedIds: ['t', 'g'] } } } as never,
      asked,
      ctx,
    );
    expect(both.text[0]).toContain('descem devagar e não sofreram dano de queda');
  });

  it('the concentration result says kept or lost, with the roll against the DC', () => {
    const lost = concentrationResultView(
      {
        save: { diceCount: 1, diceSides: 20, faces: [7], modifier: 2, total: 9, physical: false },
        dc: 14,
        kept: false,
        spellNamePt: 'Constrição',
      } as never,
      2,
    );
    expect(lost.title).toBe('Você perdeu a concentração');
    expect(lost.pill).toEqual({ word: 'Concentração perdida', good: false });
    expect(lost.text[0]).toContain('contra CD 14');
    expect(lost.text[0]).toContain('A concentração em Constrição acabou.');
  });
});
