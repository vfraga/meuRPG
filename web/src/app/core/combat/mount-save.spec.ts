import { create } from '@bufbuild/protobuf';

import {
  CombatantKind,
  MountSaveCause,
  MountSavePromptSchema,
  MountSaveResultSchema,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { RollMode } from '../../../gen/meurpg/play/v1/combat_rolls_pb';
import { combatant, encounter } from './combat-testing';
import { mountSaveResultView, mountSaveView, mountSaveWait, ownMountSave } from './mount-save';

const plain = (t: string) => t.replace(/ /g, ' ');

const prompt = (over: object = {}) =>
  create(MountSavePromptSchema, {
    riderId: 'tor',
    mountId: 'horse',
    dc: 0,
    cause: MountSaveCause.PUSHED,
    mode: RollMode.NORMAL,
    saveBonus: 2,
    bonusKnown: true,
    ...over,
  });

const tor = (over: object = {}) =>
  combatant({
    id: 'tor',
    label: 'Tor',
    kind: CombatantKind.PLAYER,
    mine: true,
    mountedOnId: 'horse',
    ...over,
  });
const horse = combatant({ id: 'horse', label: 'Cavalo de guerra', riderId: 'tor' });
const wight = combatant({ id: 'w', label: 'Wight' });

const fight = (p = prompt(), over: object = {}) =>
  encounter({
    round: 4,
    currentCombatantId: 'w',
    combatants: [tor(), horse, wight],
    mountSave: p,
    ...over,
  } as never);

const result = (over: object = {}) =>
  create(MountSaveResultSchema, {
    save: { diceCount: 1, diceSides: 20, faces: [15], modifier: 2, total: 17 },
    dc: 0,
    held: true,
    ...over,
  } as never);

describe("the rider's save to stay on the mount (#588, boards 588-06, 588-08 and 588-10 card 6)", () => {
  it("is the player's own when the rider is theirs, and nobody's when the server sent none", () => {
    expect(ownMountSave(fight())?.riderId).toBe('tor');
    expect(
      ownMountSave(fight(prompt(), { combatants: [tor({ mine: false }), horse] })),
    ).toBeUndefined();
    expect(ownMountSave(encounter({ combatants: [tor()] } as never))).toBeUndefined();
  });

  it('asks the rider with no DC on a table that hides it: the cause, what a pass keeps, the d20 and the bonus', () => {
    const v = mountSaveView(fight(), prompt(), true);
    expect(v.title).toBe('Você cai da montaria?');
    expect(v.subtitle).toBe('Teste de resistência de Destreza');
    expect(v.cause).toBe('O Cavalo de guerra foi empurrado.');
    expect(v.body).toBe(
      'Passe para continuar montado; se falhar, você cai Derrubado num quadrado ao lado dele.',
    );
    expect(v.tags).toEqual(['1d20 + 2', 'Destreza +2']);
    expect(v.wait).toBe('A vez do Wight espera este teste.');
    expect(v.mode).toBe('normal');
    expect(v.modeLine).toBe('');
    expect(v.autoFail).toBe(false);
    expect(v.bonus).toBe(2);
    expect(v.rollLabel).toBe('Rolar no app');
  });

  it('shows the DC only when the server sent one (show_save_dc, or the master)', () => {
    const v = mountSaveView(fight(), prompt({ dc: 10 }), true);
    expect(plain(v.subtitle)).toBe('Teste de resistência de Destreza · CD 10');
    expect(v.tags.map(plain)).toEqual(['1d20 + 2', 'Destreza +2', 'CD 10']);
    expect(mountSaveView(fight(), prompt({ dc: 0 }), true).tags.join(' ')).not.toContain('CD');
  });

  it('says "Você foi derrubado." for the other cause, and takes the gender of the mount from its name', () => {
    const mare = combatant({ id: 'horse', label: 'Égua', riderId: 'tor' });
    const e = fight(prompt({ cause: MountSaveCause.PRONE }), { combatants: [tor(), mare, wight] });
    const prone = mountSaveView(e, prompt({ cause: MountSaveCause.PRONE }), true);
    expect(prone.cause).toBe('Você foi derrubado.');
    expect(prone.body).toContain('num quadrado ao lado dela.');
    expect(mountSaveView(e, prompt(), true).cause).toBe('A Égua foi empurrada.');
  });

  it('names no mount the player does not see', () => {
    const v = mountSaveView(fight(), prompt({ mountId: '' }), true);
    expect(v.cause).toBe('A sua montaria foi empurrada.');
    expect(v.body).toContain('num quadrado ao lado da montaria.');
  });

  it('writes a negative bonus with its sign, a zero as the d20 alone, and a basic sheet as having none', () => {
    expect(mountSaveView(fight(), prompt({ saveBonus: -1 }), true).tags).toEqual([
      '1d20 − 1',
      'Destreza −1',
    ]);
    expect(mountSaveView(fight(), prompt({ saveBonus: 0 }), true).tags).toEqual([
      '1d20',
      'Destreza +0',
    ]);
    expect(mountSaveView(fight(), prompt({ saveBonus: 0, bonusKnown: false }), false).tags).toEqual(
      ['1d20', 'Sem bônus na ficha'],
    );
  });

  it('says the mode of the d20 in words, with the two dice', () => {
    const v = mountSaveView(fight(), prompt({ mode: RollMode.DISADVANTAGE }), true);
    expect(v.mode).toBe('disadvantage');
    expect(v.modeLine).toBe('Você tem desvantagem neste teste. Role dois d20; vale o menor.');
    expect(v.tags[0]).toBe('2d20 (menor) + 2');
    const adv = mountSaveView(fight(), prompt({ mode: RollMode.ADVANTAGE }), false);
    expect(adv.modeLine).toBe('Tor tem vantagem neste teste. Role dois d20; vale o maior.');
  });

  it('has no d20 for a rider that fails by itself, and says which condition does it when the rider reads one', () => {
    const e = fight(prompt({ autoFail: true }), {
      combatants: [
        tor({ conditions: ['condition:paralyzed'], conditionNamesPt: ['Paralisado'] }),
        horse,
        wight,
      ],
    });
    const v = mountSaveView(e, prompt({ autoFail: true }), true);
    expect(v.autoFail).toBe(true);
    expect(v.tags).toEqual(['Falha automática']);
    expect(v.autoFailLine).toBe('Paralisado: cai sem rolar.');
    expect(v.rollLabel).toBe('Continuar');
    expect(mountSaveView(fight(), prompt({ autoFail: true }), true).autoFailLine).toBe(
      'Você não consegue se segurar: cai sem rolar.',
    );
  });

  it('speaks of the rider by name to the master who rolls for it', () => {
    const v = mountSaveView(
      fight(prompt({ dc: 10 })),
      prompt({ dc: 10, cause: MountSaveCause.PRONE }),
      false,
    );
    expect(v.title).toBe('Tor cai da montaria?');
    expect(
      mountSaveView(fight(), prompt({ riderId: 'w', cause: MountSaveCause.PRONE }), false).cause,
    ).toBe('O Wight foi derrubado.');
    expect(v.cause).toBe('Tor foi derrubado.');
    expect(v.body).toBe(
      'Se passar, continua montado; se falhar, cai Derrubado num quadrado ao lado dele.',
    );
  });

  it('tells the result as the board does: the sum, the DC when it came, and what happened', () => {
    const names = { rider: 'Tor', mount: 'Cavalo de guerra' };
    expect(mountSaveResultView(result(), true, names)).toEqual({
      word: 'Passou',
      good: true,
      line: '15 + 2 = 17: você continua montado.',
    });
    expect(plain(mountSaveResultView(result({ dc: 10 }), true, names).line)).toBe(
      '15 + 2 = 17 contra CD 10: você continua montado.',
    );
    const failed = result({
      held: false,
      save: { diceCount: 1, diceSides: 20, faces: [6], modifier: 2, total: 8 },
    });
    expect(mountSaveResultView(failed, true, names)).toEqual({
      word: 'Falhou',
      good: false,
      line: '6 + 2 = 8: você caiu Derrubado ao lado do Cavalo de guerra.',
    });
    expect(mountSaveResultView(failed, true, { rider: 'Tor', mount: '' }).line).toBe(
      '6 + 2 = 8: você caiu Derrubado ao lado da montaria.',
    );
    expect(mountSaveResultView(failed, false, names).line).toBe(
      '6 + 2 = 8: Tor caiu Derrubado ao lado do Cavalo de guerra.',
    );
    expect(mountSaveResultView(result(), false, names).line).toBe(
      '15 + 2 = 17: Tor continua montado.',
    );
  });

  it('sums the d20 that counted of a pair, and a die added after it as one more term', () => {
    const pair = result({
      save: {
        diceCount: 2,
        diceSides: 20,
        faces: [4, 15],
        countedIndex: 1,
        modifier: 2,
        total: 21,
      },
    });
    expect(mountSaveResultView(pair, true, { rider: 'Tor', mount: '' }).line).toBe(
      '15 + 2 + 4 = 21: você continua montado.',
    );
  });

  it('tells an automatic failure with no numbers, and the fall that found no room only where the server said so', () => {
    const names = { rider: 'Tor', mount: 'Cavalo de guerra' };
    const auto = result({ held: false, autoFail: true, save: undefined });
    expect(mountSaveResultView(auto, true, names)).toEqual({
      word: 'Falha automática',
      good: false,
      line: 'Você caiu Derrubado ao lado do Cavalo de guerra, sem rolar.',
    });
    const far = result({
      held: false,
      noRoomNextToMount: true,
      save: { diceCount: 1, diceSides: 20, faces: [6], modifier: 2, total: 8 },
    });
    expect(mountSaveResultView(far, false, names).line).toBe(
      '6 + 2 = 8: Tor caiu Derrubado no quadrado livre mais perto: não havia lugar ao lado do Cavalo de guerra.',
    );
  });

  it("is the master's wait while it is open: whose test, why, who answers, and the line under Próximo turno", () => {
    const w = mountSaveWait(fight(prompt({ dc: 10 })))!;
    expect(w.title).toBe('Esperando o teste de Tor');
    expect(plain(w.detail)).toBe(
      'Teste de resistência de Destreza CD 10: o Cavalo de guerra foi empurrado. Tor está respondendo no celular.',
    );
    expect(w.rollLabel).toBe('Rolar por Tor');
    expect(w.why).toBe('Espera o teste de Tor');
    expect(mountSaveWait(encounter({ combatants: [tor()] } as never))).toBeNull();
  });

  it("is the master's own roll for a rider that is his, named with its article", () => {
    const rider = combatant({ id: 'w', label: 'Wight', mountedOnId: 'sk' });
    const steed = combatant({ id: 'sk', label: 'Esqueleto de cavalo', riderId: 'w' });
    const p = prompt({ riderId: 'w', mountId: 'sk', dc: 10, cause: MountSaveCause.PRONE });
    const w = mountSaveWait(encounter({ combatants: [rider, steed], mountSave: p } as never))!;
    expect(w.title).toBe('Esperando o teste do Wight');
    expect(plain(w.detail)).toBe(
      'Teste de resistência de Destreza CD 10: o Wight foi derrubado. O teste é seu.',
    );
    expect(w.rollLabel).toBe('Rolar pelo Wight');
    expect(w.why).toBe('Espera o teste do Wight');
  });
});
