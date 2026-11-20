import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  CharacterBlockedReason,
  CharacterBlockedSchema,
  LevelUpRefusalReason,
  LevelUpRefusalSchema,
} from '../../../gen/meurpg/characters/v1/characters_pb';
import { XpMode } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  blockedMessage,
  cannotLevelUpMessage,
  describeLevelUpFailure,
  refusalMessage,
  refusalStep,
} from './levelup-errors';

function refused(reason: LevelUpRefusalReason) {
  return new ConnectError('x', Code.FailedPrecondition, undefined, [
    {
      desc: LevelUpRefusalSchema,
      value: create(LevelUpRefusalSchema, { reason, field: 'full.cantrip_keys' }),
    },
  ]);
}

describe('the failures of the guided level-up', () => {
  it('reads a stale revision (aborted) as "read the sheet again"', () => {
    const f = describeLevelUpFailure(new ConnectError('x', Code.Aborted));
    expect(f.kind).toBe('stale');
    expect(f.message).toContain('A ficha mudou');
  });

  it('reads a refusal by its reason, never its message, and points at the step that owns it', () => {
    const f = describeLevelUpFailure(refused(LevelUpRefusalReason.CANTRIPS));
    expect(f).toMatchObject({ kind: 'refusal', step: 'spells' });
    expect(f.message).toBe('Escolha todos os truques novos do nível, nem mais nem menos.');
    expect(describeLevelUpFailure(refused(LevelUpRefusalReason.ABILITY_ABOVE_20))).toMatchObject({
      step: 'abilities',
      message: 'Nenhuma habilidade passa de 20. Escolha outra.',
    });
    expect(
      describeLevelUpFailure(refused(LevelUpRefusalReason.HIT_POINT_ROLL_MISSING)),
    ).toMatchObject({ step: 'hp' });
    expect(describeLevelUpFailure(refused(LevelUpRefusalReason.SUBCLASS))).toMatchObject({
      step: 'picks',
    });
  });

  it('says a choice the master switched off is not available any more, and sends it to the step that holds it (RN-23)', () => {
    const off = (field: string) =>
      new ConnectError('x', Code.FailedPrecondition, undefined, [
        {
          desc: LevelUpRefusalSchema,
          value: create(LevelUpRefusalSchema, {
            reason: LevelUpRefusalReason.SWITCHED_OFF_CHOICE,
            field,
          }),
        },
      ]);
    const subclass = describeLevelUpFailure(off('full.subclass_key'));
    expect(subclass).toMatchObject({ kind: 'refusal', step: 'picks' });
    expect(subclass.message).toBe(
      'O mestre desligou uma das opções que você escolheu para os jogadores. Volte e escolha outra.',
    );
    expect(describeLevelUpFailure(off('full.cantrip_keys'))).toMatchObject({ step: 'spells' });
    expect(describeLevelUpFailure(off('full.known_spell_keys'))).toMatchObject({ step: 'spells' });
  });

  it('gives every reason words, and the ones no step owns no step', () => {
    for (const reason of Object.values(LevelUpRefusalReason).filter(
      (v): v is LevelUpRefusalReason => typeof v === 'number' && v > 0,
    )) {
      expect(refusalMessage({ reason }).length).toBeGreaterThan(10);
    }
    expect(refusalStep(LevelUpRefusalReason.SHEET_NEEDS_MASTER)).toBeNull();
    expect(refusalMessage({ reason: LevelUpRefusalReason.SHEET_NEEDS_MASTER })).toContain(
      'Peça ao mestre',
    );
  });

  it('reads why the character cannot level up from the CharacterBlocked detail', () => {
    const blocked = new ConnectError('x', Code.FailedPrecondition, undefined, [
      {
        desc: CharacterBlockedSchema,
        value: create(CharacterBlockedSchema, { reason: CharacterBlockedReason.CANNOT_LEVEL_UP }),
      },
    ]);
    const f = describeLevelUpFailure(blocked);
    expect(f).toMatchObject({ kind: 'blocked', reason: CharacterBlockedReason.CANNOT_LEVEL_UP });
    expect(f.message).toContain('não pode subir de nível');
  });

  it('maps the plain codes', () => {
    expect(describeLevelUpFailure(new ConnectError('x', Code.PermissionDenied)).message).toContain(
      'dono do personagem',
    );
    expect(describeLevelUpFailure(new ConnectError('x', Code.NotFound)).message).toContain(
      'não existe',
    );
    expect(describeLevelUpFailure(new Error('network')).kind).toBe('other');
  });
});

describe('an option the master retired (RN-23, 10.1d)', () => {
  const refusedAt = (reason: LevelUpRefusalReason, field: string) =>
    new ConnectError('x', Code.FailedPrecondition, undefined, [
      { desc: LevelUpRefusalSchema, value: create(LevelUpRefusalSchema, { reason, field }) },
    ]);

  it('says an archived choice and sends the player to the step of its field', () => {
    const f = describeLevelUpFailure(
      refusedAt(LevelUpRefusalReason.ARCHIVED_CHOICE, 'subclass_key'),
    );
    expect(f).toMatchObject({ kind: 'refusal', step: 'picks' });
    expect(f.message).toContain('O mestre arquivou uma das opções que você escolheu');
    expect(
      describeLevelUpFailure(
        refusedAt(LevelUpRefusalReason.ARCHIVED_CHOICE, 'full.known_spell_keys'),
      ),
    ).toMatchObject({ step: 'spells' });
  });

  it('says a switched-off choice too, never the generic line', () => {
    const f = describeLevelUpFailure(
      refusedAt(LevelUpRefusalReason.SWITCHED_OFF_CHOICE, 'full.cantrip_keys'),
    );
    expect(f.message).toContain('desligou uma das opções');
    expect(f.message).not.toContain('As regras não aceitaram');
    expect(refusalStep(LevelUpRefusalReason.SWITCHED_OFF_CHOICE)).toBeNull();
  });

  it('sends a level refused for a late choice to the Escolhas step, with its own words', () => {
    const f = describeLevelUpFailure(
      refusedAt(LevelUpRefusalReason.LATE_CHOICE_MISSING, 'full.feature_choice_keys'),
    );
    expect(f).toMatchObject({ kind: 'refusal', step: 'picks' });
    expect(f.message).toContain('escolhas que ficaram para trás');
  });

  it('words a refused swap of a pick and sends the player to the Habilidades step', () => {
    const f = describeLevelUpFailure(
      refusedAt(LevelUpRefusalReason.SWAP_NOT_ALLOWED, 'choices.swap_pick'),
    );
    expect(f).toMatchObject({ kind: 'refusal', step: 'abilities' });
    expect(f.message).toContain('troca de escolha');
  });

  it('sends a refused swap of a table feature option to the step where the panel is', () => {
    const err = refusedAt(LevelUpRefusalReason.SWAP_NOT_ALLOWED, 'choices.swap_feature_options');
    // No ability step at the level: the panel is in Escolhas. With one: in Habilidades.
    expect(describeLevelUpFailure(err, false)).toMatchObject({ kind: 'refusal', step: 'picks' });
    expect(describeLevelUpFailure(err, true)).toMatchObject({ kind: 'refusal', step: 'abilities' });
    expect(
      refusalStep(LevelUpRefusalReason.SWAP_NOT_ALLOWED, 'choices.swap_feature_options', false),
    ).toBe('picks');
    expect(
      refusalStep(LevelUpRefusalReason.SWAP_NOT_ALLOWED, 'choices.swap_feature_options', true),
    ).toBe('abilities');
    expect(refusalStep(LevelUpRefusalReason.SWAP_NOT_ALLOWED, 'choices.swap_pick', false)).toBe(
      'abilities',
    );
  });

  it('words the refusals of a feat and sends the player to the Habilidades step', () => {
    for (const reason of [
      LevelUpRefusalReason.FEAT,
      LevelUpRefusalReason.FEAT_PREREQUISITE,
      LevelUpRefusalReason.FEATS_NOT_ALLOWED,
    ]) {
      expect(describeLevelUpFailure(refused(reason))).toMatchObject({ step: 'abilities' });
    }
    expect(refusalMessage({ reason: LevelUpRefusalReason.FEAT_PREREQUISITE })).toContain(
      'pré-requisito do talento',
    );
    expect(refusalMessage({ reason: LevelUpRefusalReason.FEATS_NOT_ALLOWED })).toBe(
      'O mestre não usa talentos nesta mesa. Aumente habilidades.',
    );
    expect(refusalMessage({ reason: LevelUpRefusalReason.FEAT })).toContain('Escolha outro');
  });
});

describe('the multiclass refusals', () => {
  it('send the player to the class step or to the picks, with words of their own', () => {
    expect(refusalStep(LevelUpRefusalReason.MULTICLASS_PREREQUISITE)).toBe('class');
    expect(refusalStep(LevelUpRefusalReason.MULTICLASS_PREREQUISITE_CURRENT)).toBe('class');
    expect(refusalStep(LevelUpRefusalReason.PROFICIENCY_CHOICE)).toBe('picks');
    expect(refusalStep(LevelUpRefusalReason.INSTRUMENT_CHOICE)).toBe('picks');
    expect(refusalMessage({ reason: LevelUpRefusalReason.MULTICLASS_PREREQUISITE })).toContain(
      'Escolha outra classe',
    );
    expect(
      refusalMessage({ reason: LevelUpRefusalReason.MULTICLASS_PREREQUISITE_CURRENT }),
    ).toContain('nenhuma classe nova entra');
    expect(refusalMessage({ reason: LevelUpRefusalReason.PROFICIENCY_CHOICE })).toContain(
      'perícia',
    );
    expect(refusalMessage({ reason: LevelUpRefusalReason.INSTRUMENT_CHOICE })).toContain(
      'instrumento',
    );
  });
});

describe('the level-up refusals, one by one', () => {
  const MESSAGES: readonly (readonly [LevelUpRefusalReason, string])[] = [
    [
      LevelUpRefusalReason.ARCHIVED_CHOICE,
      'O mestre arquivou uma das opções que você escolheu, e ela não vale mais como escolha nova. Volte e escolha outra.',
    ],
    [
      LevelUpRefusalReason.SWITCHED_OFF_CHOICE,
      'O mestre desligou uma das opções que você escolheu para os jogadores. Volte e escolha outra.',
    ],
    [
      LevelUpRefusalReason.CLASS,
      'Esse nível não vale para a classe escolhida. Volte para a ficha e comece de novo.',
    ],
    [LevelUpRefusalReason.MAX_LEVEL, 'Este personagem já está no nível máximo.'],
    [
      LevelUpRefusalReason.MULTICLASS_PREREQUISITE,
      'Esta classe nova pede uma habilidade que o personagem ainda não tem. Escolha outra classe.',
    ],
    [
      LevelUpRefusalReason.MULTICLASS_PREREQUISITE_CURRENT,
      'Uma classe que o personagem já tem pede uma habilidade que ele não tem, e por isso nenhuma classe nova entra. Suba uma classe que ele já tem.',
    ],
    [
      LevelUpRefusalReason.PROFICIENCY_CHOICE,
      'Escolha a perícia que a classe nova dá, da lista dela e que o personagem ainda não tenha.',
    ],
    [
      LevelUpRefusalReason.INSTRUMENT_CHOICE,
      'Escolha o instrumento musical da classe nova, entre os da lista.',
    ],
    [
      LevelUpRefusalReason.LOCKED_FIELD,
      'Algo que o nível não muda ficou diferente na ficha. Volte para a ficha e comece de novo.',
    ],
    [LevelUpRefusalReason.ABILITY_NOT_DUE, 'Este nível não dá incremento no valor de habilidade.'],
    [LevelUpRefusalReason.ABILITY_SHAPE, 'O aumento é de +2 em uma habilidade, ou de +1 em duas.'],
    [LevelUpRefusalReason.ABILITY_ABOVE_20, 'Nenhuma habilidade passa de 20. Escolha outra.'],
    [
      LevelUpRefusalReason.FEAT,
      'O talento escolhido não vale neste nível: ele não existe mais, saiu da lista ou este nível não o permite. Escolha outro.',
    ],
    [
      LevelUpRefusalReason.FEAT_PREREQUISITE,
      'A ficha não atende ao pré-requisito do talento. Escolha outro talento ou aumente habilidades.',
    ],
    [
      LevelUpRefusalReason.FEATS_NOT_ALLOWED,
      'O mestre não usa talentos nesta mesa. Aumente habilidades.',
    ],
    [
      LevelUpRefusalReason.HIT_POINTS,
      'O resultado do dado de vida está fora do que o dado permite.',
    ],
    [
      LevelUpRefusalReason.HIT_POINTS_RULE,
      'A mesa decidiu como se ganham os pontos de vida do nível: use o jeito que ela deixa.',
    ],
    [LevelUpRefusalReason.SUBCLASS, 'Escolha a subclasse do nível.'],
    [LevelUpRefusalReason.CANTRIPS, 'Escolha todos os truques novos do nível, nem mais nem menos.'],
    [LevelUpRefusalReason.SPELLS, 'Escolha todas as magias novas do nível, nem mais nem menos.'],
    [LevelUpRefusalReason.PREPARED, 'As magias preparadas só ganham vagas novas: nenhuma sai.'],
    [LevelUpRefusalReason.FEATURE_CHOICE, 'Escolha todas as opções das novas características.'],
    [
      LevelUpRefusalReason.LATE_CHOICE_MISSING,
      'Ainda há escolhas que ficaram para trás em níveis anteriores. Faça todas antes de subir de nível.',
    ],
    [
      LevelUpRefusalReason.SWAP_NOT_ALLOWED,
      'Essa troca de escolha não vale neste nível ou nesta mesa. Tire a troca e confirme o nível.',
    ],
    [LevelUpRefusalReason.SKILLS, 'Escolha todas as perícias novas do nível.'],
    [LevelUpRefusalReason.EXPERTISE, 'Escolha todas as especializações novas do nível.'],
    [
      LevelUpRefusalReason.SHEET_ISSUE,
      'Alguma das escolhas não vale para a ficha, como uma magia fora da lista da classe ou preparada demais. Confira as magias.',
    ],
    [LevelUpRefusalReason.HIT_POINT_ROLL_MISSING, 'Role o dado de vida antes de confirmar.'],
    [
      LevelUpRefusalReason.HIT_POINT_ROLL_OTHER_CLASS,
      'O dado deste nível já foi rolado para outra classe.',
    ],
    [
      LevelUpRefusalReason.SHEET_NEEDS_MASTER,
      'A ficha tem um problema que nenhuma escolha resolve. Peça ao mestre para corrigir a ficha.',
    ],
  ];
  const STEPS: readonly (readonly [LevelUpRefusalReason, string | null])[] = [
    [LevelUpRefusalReason.ABILITY_NOT_DUE, 'abilities'],
    [LevelUpRefusalReason.ABILITY_SHAPE, 'abilities'],
    [LevelUpRefusalReason.FEAT, 'abilities'],
    [LevelUpRefusalReason.FEAT_PREREQUISITE, 'abilities'],
    [LevelUpRefusalReason.FEATS_NOT_ALLOWED, 'abilities'],
    [LevelUpRefusalReason.ABILITY_ABOVE_20, 'abilities'],
    [LevelUpRefusalReason.HIT_POINTS, 'hp'],
    [LevelUpRefusalReason.HIT_POINTS_RULE, 'hp'],
    [LevelUpRefusalReason.HIT_POINT_ROLL_MISSING, 'hp'],
    [LevelUpRefusalReason.HIT_POINT_ROLL_OTHER_CLASS, 'hp'],
    [LevelUpRefusalReason.MULTICLASS_PREREQUISITE, 'class'],
    [LevelUpRefusalReason.MULTICLASS_PREREQUISITE_CURRENT, 'class'],
    [LevelUpRefusalReason.CLASS, 'class'],
    [LevelUpRefusalReason.PROFICIENCY_CHOICE, 'picks'],
    [LevelUpRefusalReason.INSTRUMENT_CHOICE, 'picks'],
    [LevelUpRefusalReason.SUBCLASS, 'picks'],
    [LevelUpRefusalReason.FEATURE_CHOICE, 'picks'],
    [LevelUpRefusalReason.LATE_CHOICE_MISSING, 'picks'],
    [LevelUpRefusalReason.SWAP_NOT_ALLOWED, 'abilities'],
    [LevelUpRefusalReason.SKILLS, 'picks'],
    [LevelUpRefusalReason.EXPERTISE, 'picks'],
    [LevelUpRefusalReason.CANTRIPS, 'spells'],
    [LevelUpRefusalReason.SPELLS, 'spells'],
    [LevelUpRefusalReason.PREPARED, 'spells'],
    [LevelUpRefusalReason.MAX_LEVEL, null],
    [LevelUpRefusalReason.LOCKED_FIELD, null],
    [LevelUpRefusalReason.SHEET_ISSUE, null],
    [LevelUpRefusalReason.SHEET_NEEDS_MASTER, null],
  ];
  const BLOCKED: readonly (readonly [CharacterBlockedReason, string])[] = [
    [
      CharacterBlockedReason.CANNOT_LEVEL_UP,
      'Este personagem não pode subir de nível agora: o mestre ainda não marcou um marco, ou o XP não chegou.',
    ],
    [CharacterBlockedReason.CHARACTER_DEAD, 'Esse personagem está morto e não sobe de nível.'],
    [
      CharacterBlockedReason.AWAITING_APPROVAL,
      'Esse personagem ainda espera a aprovação do mestre.',
    ],
    [
      CharacterBlockedReason.DICE_FORCED_PHYSICAL,
      'Neste jogo todos rolam os próprios dados: digite o número que saiu.',
    ],
    [
      CharacterBlockedReason.DICE_FORCED_IN_APP,
      'Neste jogo todos rolam no app: use "Rolar no app".',
    ],
    [
      CharacterBlockedReason.HIT_POINTS_AVERAGE_ONLY,
      'A mesa usa a média nos pontos de vida: o dado não é rolado.',
    ],
  ];

  it.each(MESSAGES)('says reason %i in words', (reason, message) => {
    expect(refusalMessage({ reason })).toBe(message);
  });

  it('has the words of every reason the server can name', () => {
    const named = MESSAGES.map(([reason]) => reason);
    const all = Object.values(LevelUpRefusalReason).filter(
      (v): v is LevelUpRefusalReason =>
        typeof v === 'number' && v !== LevelUpRefusalReason.UNSPECIFIED,
    );
    expect(all.filter((v) => !named.includes(v))).toEqual([]);
  });

  it('falls back to a general sentence for a reason it does not know', () => {
    const fallback = 'As regras não aceitaram as escolhas. Confira cada passo.';
    expect(refusalMessage({ reason: LevelUpRefusalReason.UNSPECIFIED })).toBe(fallback);
    expect(refusalMessage({ reason: 9999 as LevelUpRefusalReason })).toBe(fallback);
  });

  it.each(STEPS)('sends reason %i to its step', (reason, step) => {
    expect(refusalStep(reason, 'full.other')).toBe(step);
  });

  it('sends a retired choice to the step of the field it is chosen in', () => {
    for (const reason of [
      LevelUpRefusalReason.ARCHIVED_CHOICE,
      LevelUpRefusalReason.SWITCHED_OFF_CHOICE,
    ]) {
      expect(refusalStep(reason, 'full.class_key')).toBe('picks');
      expect(refusalStep(reason, 'full.feature_choice.x')).toBe('picks');
      expect(refusalStep(reason, 'full.spell_keys')).toBe('spells');
      expect(refusalStep(reason, 'full.cantrip_keys')).toBe('spells');
      expect(refusalStep(reason, 'full.hp')).toBeNull();
      expect(refusalStep(reason)).toBeNull();
    }
    expect(refusalStep(9999 as LevelUpRefusalReason)).toBeNull();
  });

  it.each(BLOCKED)('says why the character is blocked (%i)', (reason, message) => {
    expect(blockedMessage(reason)).toBe(message);
  });

  it('says a general sentence for a block it does not know', () => {
    expect(blockedMessage(undefined)).toBe('Não foi possível concluir a ação agora.');
    expect(blockedMessage(CharacterBlockedReason.UNSPECIFIED)).toBe(
      'Não foi possível concluir a ação agora.',
    );
  });

  it('sorts a blocked character with the reason, a refusal without a detail, and the plain codes', () => {
    const blocked = new ConnectError('x', Code.FailedPrecondition, undefined, [
      {
        desc: CharacterBlockedSchema,
        value: create(CharacterBlockedSchema, { reason: CharacterBlockedReason.CHARACTER_DEAD }),
      },
    ]);
    expect(describeLevelUpFailure(blocked)).toEqual({
      kind: 'blocked',
      message: 'Esse personagem está morto e não sobe de nível.',
      reason: CharacterBlockedReason.CHARACTER_DEAD,
    });
    expect(describeLevelUpFailure(new ConnectError('x', Code.FailedPrecondition))).toMatchObject({
      kind: 'blocked',
      reason: undefined,
    });
    expect(describeLevelUpFailure(new ConnectError('x', Code.PermissionDenied))).toEqual({
      kind: 'other',
      message: 'Só o jogador dono do personagem sobe o nível. O mestre edita a ficha.',
    });
    expect(describeLevelUpFailure(new ConnectError('x', Code.NotFound)).message).toBe(
      'Esse personagem não existe, ou você não pode vê-lo.',
    );
    expect(describeLevelUpFailure(new ConnectError('x', Code.InvalidArgument)).message).toBe(
      'Alguma escolha não vale. Confira cada passo.',
    );
  });
});

describe('why a character cannot level up', () => {
  it('says the maximum level first, then milestones, then the XP still missing', () => {
    expect(cannotLevelUpMessage(XpMode.GOLD, 0, 100, 20)).toBe(
      'Este personagem já está no nível máximo.',
    );
    expect(cannotLevelUpMessage(XpMode.MILESTONES, 0, 100, 3)).toContain('marcar um marco');
    expect(cannotLevelUpMessage(XpMode.GOLD, 250, 1000, 3).replace(/\u00a0/g, ' ')).toBe(
      'Faltam 750 XP para o nível 4 (1.000 XP). Quando o XP chegar, o botão aparece na ficha.',
    );
    expect(cannotLevelUpMessage(XpMode.GOLD, 1000, 1000, 3)).toBe(
      blockedMessage(CharacterBlockedReason.CANNOT_LEVEL_UP),
    );
  });
});
