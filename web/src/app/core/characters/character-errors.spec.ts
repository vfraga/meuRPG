import { Code, ConnectError } from '@connectrpc/connect';

import {
  CharacterBlockedReason,
  CharacterBlockedSchema,
  ChoiceRefusalReason,
  ChoiceRefusalSchema,
} from '../../../gen/meurpg/characters/v1/characters_pb';
import { InvalidFieldSchema } from '../../../gen/meurpg/characters/v1/characters_pb';
import {
  characterBlockedMessage,
  contentRef,
  describeCharacterError,
  invalidFieldPath,
  livingRefusal,
  pickRefusalOf,
  switchedOffKey,
} from './character-errors';

function blockedError(reason: CharacterBlockedReason, characterId = 'char-1'): ConnectError {
  return new ConnectError('blocked', Code.FailedPrecondition, undefined, [
    { desc: CharacterBlockedSchema, value: { reason, characterId } },
  ]);
}

describe('characterBlockedMessage', () => {
  it('names every CharacterBlocked reason', () => {
    expect(characterBlockedMessage('sheet_locked')).toContain('travada');
    expect(characterBlockedMessage('character_dead')).toContain('morto');
    expect(characterBlockedMessage('living_character_exists')).toContain('já tem um personagem');
    expect(characterBlockedMessage('story_locked')).toContain('Permitir editar a história');
    expect(characterBlockedMessage('not_pending')).toContain('já foi aprovado');
    expect(characterBlockedMessage('awaiting_approval')).toContain('Aprove ou recuse');
    expect(characterBlockedMessage('no_changes_requested')).toContain('pedido de ajustes aberto');
    expect(characterBlockedMessage('not_dead')).toContain('não está morto');
    expect(characterBlockedMessage('not_reserved')).toContain('já tem dono');
    expect(characterBlockedMessage('claim_link_used')).toContain('já foi usado');
    expect(characterBlockedMessage('not_claimed')).toContain('não veio de um link');
    expect(characterBlockedMessage('character_in_combat')).toContain('Encerre o combate');
    expect(characterBlockedMessage('claim_own_link')).toContain('Este link é para um jogador');
    expect(characterBlockedMessage('reserved')).toContain('reservado');
  });

  it('falls back to a generic message when the reason is unknown', () => {
    expect(characterBlockedMessage(undefined)).toBe('Não foi possível concluir a ação agora.');
  });
});

describe('describeCharacterError', () => {
  it('reads the CharacterBlocked detail off the error itself: SHEET_LOCKED', () => {
    expect(describeCharacterError(blockedError(CharacterBlockedReason.SHEET_LOCKED))).toContain(
      'travada',
    );
  });

  it('reads the CharacterBlocked detail off the error itself: CHARACTER_DEAD', () => {
    expect(describeCharacterError(blockedError(CharacterBlockedReason.CHARACTER_DEAD))).toContain(
      'morto',
    );
  });

  it('reads the CharacterBlocked detail off the error itself: LIVING_CHARACTER_EXISTS', () => {
    expect(
      describeCharacterError(blockedError(CharacterBlockedReason.LIVING_CHARACTER_EXISTS)),
    ).toContain('já tem um personagem');
  });

  it('reads the CharacterBlocked detail off the error itself: STORY_LOCKED', () => {
    expect(describeCharacterError(blockedError(CharacterBlockedReason.STORY_LOCKED))).toContain(
      'Permitir editar a história',
    );
  });

  it('reads the CharacterBlocked detail off the error itself: NOT_PENDING and AWAITING_APPROVAL (MR-024)', () => {
    expect(describeCharacterError(blockedError(CharacterBlockedReason.NOT_PENDING))).toContain(
      'já foi aprovado',
    );
    expect(
      describeCharacterError(blockedError(CharacterBlockedReason.AWAITING_APPROVAL)),
    ).toContain('Aprove ou recuse');
  });

  it('reads NO_CHANGES_REQUESTED, and says a reason that is not 1 to 500 characters (the field "reason")', () => {
    expect(
      describeCharacterError(blockedError(CharacterBlockedReason.NO_CHANGES_REQUESTED)),
    ).toContain('pedido de ajustes aberto');
    const badReason = new ConnectError('bad', Code.InvalidArgument, undefined, [
      { desc: InvalidFieldSchema, value: { field: 'reason' } },
    ]);
    expect(describeCharacterError(badReason)).toBe('O motivo precisa ter de 1 a 500 caracteres.');
  });

  it('falls back to a generic message for failed_precondition with no detail', () => {
    expect(describeCharacterError(new ConnectError('blocked', Code.FailedPrecondition))).toBe(
      'Não foi possível concluir a ação agora.',
    );
  });

  it('maps aborted to "a ficha mudou, recarregue"', () => {
    const err = new ConnectError('stale revision', Code.Aborted);
    expect(describeCharacterError(err)).toContain('A ficha mudou');
  });

  it('maps not_found, permission_denied and invalid_argument', () => {
    expect(describeCharacterError(new ConnectError('x', Code.NotFound))).toBe(
      'Personagem não encontrado.',
    );
    expect(describeCharacterError(new ConnectError('x', Code.PermissionDenied))).toBe(
      'Você não tem permissão para fazer isso.',
    );
    expect(describeCharacterError(new ConnectError('x', Code.InvalidArgument))).toBe(
      'Confira os campos da ficha.',
    );
  });

  it('tells the campaign holds the most characters and NPCs it may', () => {
    expect(describeCharacterError(new ConnectError('full', Code.ResourceExhausted))).toContain(
      '1.000 personagens e NPCs',
    );
  });

  it('falls back to the generic unavailable message for anything else', () => {
    expect(describeCharacterError(new Error('network down'))).toContain(
      'Não foi possível falar com o servidor',
    );
  });
});

describe('the choices a sheet leaves open (CHOICES_MISSING)', () => {
  const missing = (...labels: string[]) =>
    new ConnectError('open', Code.FailedPrecondition, undefined, [
      {
        desc: ChoiceRefusalSchema,
        value: {
          reason: ChoiceRefusalReason.CHOICES_MISSING,
          issues: labels.map((labelPt) => ({ labelPt })),
        },
      },
    ]);

  it('names the open choices by the labels the server sends', () => {
    expect(describeCharacterError(missing('Estilo de Luta'))).toBe(
      'Falta uma escolha: Estilo de Luta. Faça-a no passo "Escolhas".',
    );
    expect(describeCharacterError(missing('Estilo de Luta', 'Invocações'))).toBe(
      'Faltam escolhas: Estilo de Luta; Invocações. Faça-as no passo "Escolhas".',
    );
  });

  it('still says something when the server sends no label', () => {
    expect(describeCharacterError(missing())).toContain('Faltam escolhas');
  });
});

describe('an option the master switched off (RN-23, SWITCHED_OFF_CONTENT)', () => {
  const off = (key: string) =>
    new ConnectError('blocked', Code.FailedPrecondition, undefined, [
      {
        desc: CharacterBlockedSchema,
        value: { reason: CharacterBlockedReason.SWITCHED_OFF_CONTENT, contentKey: key },
      },
    ]);

  it('reads the key of the refused choice off the typed detail', () => {
    expect(switchedOffKey(off('race:tiefling'))).toBe('race:tiefling');
    expect(switchedOffKey(off(''))).toBe('');
  });

  it('is null for any other error, so no other refusal is read as this one', () => {
    expect(switchedOffKey(blockedError(CharacterBlockedReason.SHEET_LOCKED))).toBeNull();
    expect(switchedOffKey(new ConnectError('x', Code.Aborted))).toBeNull();
    expect(switchedOffKey(new Error('offline'))).toBeNull();
  });
});

describe('the content the master retired (RN-23, 10.1d)', () => {
  const retired = (reason: CharacterBlockedReason, contentKey: string) =>
    new ConnectError('blocked', Code.FailedPrecondition, undefined, [
      { desc: CharacterBlockedSchema, value: { reason, contentKey } },
    ]);
  const names = (key: string) =>
    ({
      'class:guardiao@mesa': 'Guardião do Vale',
      'background:cartografo@mesa': 'Cartógrafo do Vale',
      'spell:ink@mesa': 'Lâmina de Nanquim',
    })[key as 'class:guardiao@mesa'];

  it('says an archived class by its name and where to change it, never "Não foi possível concluir a ação"', () => {
    const msg = describeCharacterError(
      retired(CharacterBlockedReason.ARCHIVED_CONTENT, 'class:guardiao@mesa'),
      names,
    );
    expect(msg).toBe(
      'A classe “Guardião do Vale” foi arquivada pelo mestre e não vale mais como escolha nova. Escolha outra opção, no passo Básico.',
    );
  });

  it('writes the masculine of an antecedente, and a spell on its own step', () => {
    expect(
      describeCharacterError(
        retired(CharacterBlockedReason.ARCHIVED_CONTENT, 'background:cartografo@mesa'),
        names,
      ),
    ).toContain('O antecedente “Cartógrafo do Vale” foi arquivado');
    expect(
      describeCharacterError(
        retired(CharacterBlockedReason.ARCHIVED_CONTENT, 'spell:ink@mesa'),
        names,
      ),
    ).toContain('no passo Magias');
  });

  it('says a switched-off option, with its name', () => {
    expect(
      describeCharacterError(
        retired(CharacterBlockedReason.SWITCHED_OFF_CONTENT, 'class:guardiao@mesa'),
        names,
      ),
    ).toBe(
      'A classe “Guardião do Vale” foi desligada pelo mestre para os jogadores. Escolha outra opção, no passo Básico.',
    );
  });

  it('never prints the key when the catalog does not know it', () => {
    const msg = describeCharacterError(
      retired(CharacterBlockedReason.ARCHIVED_CONTENT, 'class:some-key@mesa'),
      () => undefined,
    );
    expect(msg).not.toContain('some-key');
    expect(msg).toContain('A classe escolhida foi arquivada');
    expect(contentRef('thing', () => undefined).noun).toBe('opção');
  });

  it('points a refusal at the class block it is about', () => {
    const err = new ConnectError('x', Code.InvalidArgument, undefined, [
      { desc: InvalidFieldSchema, value: { field: 'full.classes[1].class_key' } },
    ]);
    expect(invalidFieldPath(err)).toBe('full.classes[1].class_key');
    expect(describeCharacterError(err)).toContain('Classe 2: essa classe se repete ou não existe');
    expect(invalidFieldPath(new ConnectError('x', Code.NotFound))).toBeNull();
  });
});

describe('pickRefusalOf', () => {
  const refused = (field: string, sentence: string) =>
    new ConnectError(`${field} ${sentence}`, Code.InvalidArgument, undefined, [
      { desc: InvalidFieldSchema, value: { field } },
    ]);
  const SKILL = 'sheet.full.skill_proficiency_keys[2]';
  const EXPERTISE = 'sheet.full.expertise_skill_keys[0]';
  const sentences: [string, string][] = [
    [
      SKILL,
      'A perícia Furtividade não está em nenhuma das listas de onde o personagem escolhe perícias.',
    ],
    [
      SKILL,
      'Não há vaga para a perícia Furtividade: as listas que a oferecem já estão preenchidas pelas outras perícias marcadas. Desmarque uma delas para marcar Furtividade.',
    ],
    [
      EXPERTISE,
      'A especialização em Furtividade não está em nenhuma das listas de onde o personagem escolhe especializações.',
    ],
    [
      EXPERTISE,
      'Não há vaga para a especialização em Furtividade: as listas que a oferecem já estão preenchidas pelas outras especializações marcadas. Desmarque uma delas para marcar a especialização em Furtividade.',
    ],
  ];

  it.each(sentences)('gives the server sentence of %s without the field', (field, sentence) => {
    const err = refused(field, sentence);
    expect(pickRefusalOf(err)).toBe(sentence);
    expect(describeCharacterError(err)).toBe(sentence);
  });

  it('is null for any other sentence at those fields, and for any other refusal', () => {
    const other = refused(SKILL, 'Qualquer outra frase do servidor.');
    expect(pickRefusalOf(other)).toBeNull();
    expect(describeCharacterError(other)).toBe('Confira os campos da ficha.');
    expect(pickRefusalOf(refused('sheet.full.classes[0]', 'x y'))).toBeNull();
    expect(pickRefusalOf(new ConnectError('x', Code.InvalidArgument))).toBeNull();
    expect(pickRefusalOf(new Error('x'))).toBeNull();
  });
});

describe('livingRefusal', () => {
  it('reads the living character that stops a revival: its id and name', () => {
    const err = new ConnectError('blocked', Code.FailedPrecondition, undefined, [
      {
        desc: CharacterBlockedSchema,
        value: {
          reason: CharacterBlockedReason.LIVING_CHARACTER_EXISTS,
          characterId: 'nuvem-1',
          livingCharacterName: 'Nuvem',
        },
      },
    ]);
    expect(livingRefusal(err)).toEqual({ characterId: 'nuvem-1', name: 'Nuvem' });
  });

  it('is null for any other refusal and for any other error', () => {
    expect(livingRefusal(blockedError(CharacterBlockedReason.NOT_DEAD))).toBeNull();
    expect(livingRefusal(new ConnectError('x', Code.NotFound))).toBeNull();
    expect(livingRefusal(new Error('network'))).toBeNull();
  });

  it('says NOT_DEAD in words through describeCharacterError', () => {
    expect(describeCharacterError(blockedError(CharacterBlockedReason.NOT_DEAD))).toContain(
      'não está morto',
    );
  });
});
