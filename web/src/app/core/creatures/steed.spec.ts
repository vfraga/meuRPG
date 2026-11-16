import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  CharacterCreatureSchema,
  SummonSpellOptionsSchema,
} from '../../../gen/meurpg/characters/v1/characters_pb';
import {
  EncounterBlockedReason,
  EncounterBlockedSchema,
} from '../../../gen/meurpg/play/v1/combat_pb';
import {
  GameSessionBlockedReason,
  GameSessionBlockedSchema,
} from '../../../gen/meurpg/play/v1/play_pb';
import {
  FIND_STEED,
  STEED_TOO_SMALL,
  steedCastError,
  steedCastVerb,
  steedCostLine,
  steedFixedLine,
  steedHowLines,
  steedSameLine,
  steedTypeWord,
  steedTypes,
} from './steed';

describe('Convocar Montaria, the words of the summon sheet (#588, boards 588-21 and 588-22)', () => {
  const spell = (over: object = {}) =>
    create(SummonSpellOptionsSchema, {
      spellKey: FIND_STEED,
      namePt: 'Convocar Montaria',
      level: 2,
      typeChoices: ['celestial', 'fey', 'fiend'],
      minInt: 6,
      notes: ['Vocês se falam por telepatia a até 1,5 km.', 'Ele entende um idioma que você fala.'],
      ...over,
    } as never);

  it("names the steed's types with the app's words, in the server's order", () => {
    expect(steedTypes(spell())).toEqual([
      { value: 'celestial', label: 'Celestial' },
      { value: 'fey', label: 'Fada' },
      { value: 'fiend', label: 'Ínfero' },
    ]);
    expect(steedTypes(spell({ typeChoices: [] }))).toEqual([]);
  });

  it('writes the type in the lower case of a subtitle, and nothing for a type it does not know', () => {
    expect(steedTypeWord('celestial')).toBe('celestial');
    expect(steedTypeWord('fey')).toBe('fada');
    expect(steedTypeWord('fiend')).toBe('ínfero');
    expect(steedTypeWord('')).toBe('');
    expect(steedTypeWord('dragonish')).toBe('');
  });

  it('folds the spell\'s notes and the least Intelligence under "Como funciona"', () => {
    expect(steedHowLines(spell())).toEqual([
      'Vocês se falam por telepatia a até 1,5 km.',
      'Ele entende um idioma que você fala.',
      'A Inteligência dele é no mínimo 6.',
    ]);
    expect(steedHowLines(spell({ minInt: 0, notes: [] }))).toEqual([]);
  });

  it('says the button for what the cast brings: the form, or the bonded steed by its name', () => {
    expect(steedCastVerb('', '')).toBe('Convocar a montaria');
    expect(steedCastVerb('', 'Cavalo de guerra')).toBe('Convocar Cavalo de guerra');
    expect(steedCastVerb('Estrela', 'Cavalo de guerra')).toBe('Convocar Estrela');
  });

  it('says what the cast spends and when it can be made, and that a bonded steed comes back as it was', () => {
    expect(steedCostLine(2)).toBe(
      'Gasta um espaço de 2º\u00a0nível ou maior. Só durante uma sessão, fora de combate.',
    );
    expect(steedSameLine('Estrela')).toBe(
      'Estrela volta como era, com todos os pontos de vida. Para outra forma ou outro tipo, liberte Estrela antes, no bloco Criaturas.',
    );
    expect(STEED_TOO_SMALL).toBe('Pequeno demais para você montar');
  });

  it('reads the two refusals of a ten-minute casting in its own words, and the rest as any summon', () => {
    const blocked = (reason: EncounterBlockedReason) =>
      new ConnectError('x', Code.FailedPrecondition, undefined, [
        { desc: EncounterBlockedSchema, value: { reason } },
      ]);
    expect(steedCastError(blocked(EncounterBlockedReason.SUMMON_IN_COMBAT))).toBe(
      'Leva 10 minutos: não dá durante um combate.',
    );
    expect(
      steedCastError(
        new ConnectError('x', Code.FailedPrecondition, undefined, [
          {
            desc: GameSessionBlockedSchema,
            value: { reason: GameSessionBlockedReason.NO_OPEN_SESSION },
          },
        ]),
      ),
    ).toBe('Agora não há sessão aberta.');
    expect(steedCastError(blocked(EncounterBlockedReason.NO_SLOT))).toBe(
      'Não há espaço de magia livre.',
    );
  });

  it("writes the fixed row of a bonded steed: the form, the type's word and the numbers", () => {
    const steed = create(CharacterCreatureSchema, {
      name: 'Estrela',
      monsterKey: 'monster:warhorse',
      monsterNamePt: 'Cavalo de guerra',
      creatureType: 'celestial',
      armorClass: 11,
      hitPointsMax: 19,
    });
    expect(steedFixedLine(steed)).toBe('Cavalo de guerra · celestial · CA 11 · 19 PV');
    expect(steedFixedLine(create(CharacterCreatureSchema, { ...steed, creatureType: '' }))).toBe(
      'Cavalo de guerra · CA 11 · 19 PV',
    );
  });
});
