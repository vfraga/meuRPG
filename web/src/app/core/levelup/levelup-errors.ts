import { Code, ConnectError } from '@connectrpc/connect';

import {
  CharacterBlockedReason,
  CharacterBlockedSchema,
  LevelUpRefusalReason,
  LevelUpRefusalSchema,
  type LevelUpRefusal,
} from '../../../gen/meurpg/characters/v1/characters_pb';
import { XpMode } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { describeConnectError } from '../connect/connect-errors';
import { formatInt, tight } from '../format/text';
import type { StepKey } from './levelup-flow';

/** The step of every refused rule that is not about a retired option. */
const REFUSAL_STEPS: Readonly<Partial<Record<LevelUpRefusalReason, StepKey>>> = {
  [LevelUpRefusalReason.ABILITY_NOT_DUE]: 'abilities',
  [LevelUpRefusalReason.ABILITY_SHAPE]: 'abilities',
  [LevelUpRefusalReason.FEAT]: 'abilities',
  [LevelUpRefusalReason.FEAT_PREREQUISITE]: 'abilities',
  [LevelUpRefusalReason.FEATS_NOT_ALLOWED]: 'abilities',
  [LevelUpRefusalReason.ABILITY_ABOVE_20]: 'abilities',
  [LevelUpRefusalReason.HIT_POINTS]: 'hp',
  [LevelUpRefusalReason.HIT_POINTS_RULE]: 'hp',
  [LevelUpRefusalReason.HIT_POINT_ROLL_MISSING]: 'hp',
  [LevelUpRefusalReason.HIT_POINT_ROLL_OTHER_CLASS]: 'hp',
  [LevelUpRefusalReason.MULTICLASS_PREREQUISITE]: 'class',
  [LevelUpRefusalReason.MULTICLASS_PREREQUISITE_CURRENT]: 'class',
  [LevelUpRefusalReason.CLASS]: 'class',
  [LevelUpRefusalReason.PROFICIENCY_CHOICE]: 'picks',
  [LevelUpRefusalReason.INSTRUMENT_CHOICE]: 'picks',
  [LevelUpRefusalReason.SUBCLASS]: 'picks',
  [LevelUpRefusalReason.FEATURE_CHOICE]: 'picks',
  [LevelUpRefusalReason.LATE_CHOICE_MISSING]: 'picks',
  [LevelUpRefusalReason.SWAP_NOT_ALLOWED]: 'abilities',
  [LevelUpRefusalReason.SKILLS]: 'picks',
  [LevelUpRefusalReason.EXPERTISE]: 'picks',
  [LevelUpRefusalReason.CANTRIPS]: 'spells',
  [LevelUpRefusalReason.SPELLS]: 'spells',
  [LevelUpRefusalReason.PREPARED]: 'spells',
};

/** A retired option sits in the step of the field it is chosen in. */
function retiredChoiceStep(field: string): StepKey | null {
  if (/class|feature_choice/.test(field)) {
    return 'picks';
  }
  return /spell|cantrip/.test(field) ? 'spells' : null;
}

/** The step a refused rule belongs to, so the message can send the player there; `abilityStep` says whether the level has
 * the Habilidades step (where the swap panel is, if so). */
export function refusalStep(
  reason: LevelUpRefusalReason,
  field = '',
  abilityStep = true,
): StepKey | null {
  if (
    reason === LevelUpRefusalReason.ARCHIVED_CHOICE ||
    reason === LevelUpRefusalReason.SWITCHED_OFF_CHOICE
  ) {
    return retiredChoiceStep(field);
  }
  if (
    reason === LevelUpRefusalReason.SWAP_NOT_ALLOWED &&
    field.startsWith('choices.swap_feature')
  ) {
    // The one panel "Trocar uma escolha" is in Habilidades where the level has that step, else in Escolhas.
    return abilityStep ? 'abilities' : 'picks';
  }
  return REFUSAL_STEPS[reason] ?? null;
}

const REFUSAL_MESSAGES: Readonly<Partial<Record<LevelUpRefusalReason, string>>> = {
  [LevelUpRefusalReason.ARCHIVED_CHOICE]:
    'O mestre arquivou uma das opções que você escolheu, e ela não vale mais como escolha nova. Volte e escolha outra.',
  [LevelUpRefusalReason.SWITCHED_OFF_CHOICE]:
    'O mestre desligou uma das opções que você escolheu para os jogadores. Volte e escolha outra.',
  [LevelUpRefusalReason.CLASS]:
    'Esse nível não vale para a classe escolhida. Volte para a ficha e comece de novo.',
  [LevelUpRefusalReason.MAX_LEVEL]: 'Este personagem já está no nível máximo.',
  [LevelUpRefusalReason.MULTICLASS_PREREQUISITE]:
    'Esta classe nova pede uma habilidade que o personagem ainda não tem. Escolha outra classe.',
  [LevelUpRefusalReason.MULTICLASS_PREREQUISITE_CURRENT]:
    'Uma classe que o personagem já tem pede uma habilidade que ele não tem, e por isso nenhuma classe nova entra. Suba uma classe que ele já tem.',
  [LevelUpRefusalReason.PROFICIENCY_CHOICE]:
    'Escolha a perícia que a classe nova dá, da lista dela e que o personagem ainda não tenha.',
  [LevelUpRefusalReason.INSTRUMENT_CHOICE]:
    'Escolha o instrumento musical da classe nova, entre os da lista.',
  [LevelUpRefusalReason.LOCKED_FIELD]:
    'Algo que o nível não muda ficou diferente na ficha. Volte para a ficha e comece de novo.',
  [LevelUpRefusalReason.ABILITY_NOT_DUE]: 'Este nível não dá incremento no valor de habilidade.',
  [LevelUpRefusalReason.ABILITY_SHAPE]: 'O aumento é de +2 em uma habilidade, ou de +1 em duas.',
  [LevelUpRefusalReason.ABILITY_ABOVE_20]: 'Nenhuma habilidade passa de 20. Escolha outra.',
  [LevelUpRefusalReason.FEAT]:
    'O talento escolhido não vale neste nível: ele não existe mais, saiu da lista ou este nível não o permite. Escolha outro.',
  [LevelUpRefusalReason.FEAT_PREREQUISITE]:
    'A ficha não atende ao pré-requisito do talento. Escolha outro talento ou aumente habilidades.',
  [LevelUpRefusalReason.FEATS_NOT_ALLOWED]:
    'O mestre não usa talentos nesta mesa. Aumente habilidades.',
  [LevelUpRefusalReason.HIT_POINTS]: 'O resultado do dado de vida está fora do que o dado permite.',
  [LevelUpRefusalReason.HIT_POINTS_RULE]:
    'A mesa decidiu como se ganham os pontos de vida do nível: use o jeito que ela deixa.',
  [LevelUpRefusalReason.SUBCLASS]: 'Escolha a subclasse do nível.',
  [LevelUpRefusalReason.CANTRIPS]: 'Escolha todos os truques novos do nível, nem mais nem menos.',
  [LevelUpRefusalReason.SPELLS]: 'Escolha todas as magias novas do nível, nem mais nem menos.',
  [LevelUpRefusalReason.PREPARED]: 'As magias preparadas só ganham vagas novas: nenhuma sai.',
  [LevelUpRefusalReason.FEATURE_CHOICE]: 'Escolha todas as opções das novas características.',
  [LevelUpRefusalReason.SWAP_NOT_ALLOWED]:
    'Essa troca de escolha não vale neste nível ou nesta mesa. Tire a troca e confirme o nível.',
  [LevelUpRefusalReason.LATE_CHOICE_MISSING]:
    'Ainda há escolhas que ficaram para trás em níveis anteriores. Faça todas antes de subir de nível.',
  [LevelUpRefusalReason.SKILLS]: 'Escolha todas as perícias novas do nível.',
  [LevelUpRefusalReason.EXPERTISE]: 'Escolha todas as especializações novas do nível.',
  [LevelUpRefusalReason.SHEET_ISSUE]:
    'Alguma das escolhas não vale para a ficha, como uma magia fora da lista da classe ou preparada demais. Confira as magias.',
  [LevelUpRefusalReason.HIT_POINT_ROLL_MISSING]: 'Role o dado de vida antes de confirmar.',
  [LevelUpRefusalReason.HIT_POINT_ROLL_OTHER_CLASS]:
    'O dado deste nível já foi rolado para outra classe.',
  [LevelUpRefusalReason.SHEET_NEEDS_MASTER]:
    'A ficha tem um problema que nenhuma escolha resolve. Peça ao mestre para corrigir a ficha.',
};

const REFUSAL_FALLBACK = 'As regras não aceitaram as escolhas. Confira cada passo.';

/** What a rule the level broke says, by its reason (never by the server's message). */
export function refusalMessage(refusal: Pick<LevelUpRefusal, 'reason'>): string {
  return REFUSAL_MESSAGES[refusal.reason] ?? REFUSAL_FALLBACK;
}

const BLOCKED_MESSAGES: Readonly<Partial<Record<CharacterBlockedReason, string>>> = {
  [CharacterBlockedReason.CANNOT_LEVEL_UP]:
    'Este personagem não pode subir de nível agora: o mestre ainda não marcou um marco, ou o XP não chegou.',
  [CharacterBlockedReason.CHARACTER_DEAD]: 'Esse personagem está morto e não sobe de nível.',
  [CharacterBlockedReason.AWAITING_APPROVAL]: 'Esse personagem ainda espera a aprovação do mestre.',
  [CharacterBlockedReason.DICE_FORCED_PHYSICAL]:
    'Neste jogo todos rolam os próprios dados: digite o número que saiu.',
  [CharacterBlockedReason.DICE_FORCED_IN_APP]: 'Neste jogo todos rolam no app: use "Rolar no app".',
  [CharacterBlockedReason.HIT_POINTS_AVERAGE_ONLY]:
    'A mesa usa a média nos pontos de vida: o dado não é rolado.',
};

/** Why the character cannot go through the flow (or roll its die) now. */
export function blockedMessage(reason: CharacterBlockedReason | undefined): string {
  return (
    (reason !== undefined && BLOCKED_MESSAGES[reason]) || 'Não foi possível concluir a ação agora.'
  );
}

/** What a failed call of the flow means for the screen. */
export type LevelUpFailure =
  /** The sheet changed under the player: read it again. */
  | { readonly kind: 'stale'; readonly message: string }
  /** The rules refused the choices, with the step to go to. */
  | { readonly kind: 'refusal'; readonly message: string; readonly step: StepKey | null }
  /** The character cannot level up (any more): no choice fixes it. */
  | {
      readonly kind: 'blocked';
      readonly message: string;
      readonly reason: CharacterBlockedReason | undefined;
    }
  | { readonly kind: 'other'; readonly message: string };

/**
 * Sorts a failed call of the guided level-up by its code and typed detail:
 * `aborted` is a stale revision, `failed_precondition` carries either the rule
 * that was broken (`LevelUpRefusal`) or why the character cannot level up
 * (`CharacterBlocked`), and the rest are plain codes.
 */
export function describeLevelUpFailure(err: unknown, abilityStep = true): LevelUpFailure {
  const e = ConnectError.from(err, Code.Unavailable);
  if (e.code === Code.Aborted) {
    return {
      kind: 'stale',
      message:
        'A ficha mudou enquanto você escolhia: o mestre mexeu nela ou entrou XP. Leia a ficha de novo e confirme.',
    };
  }
  if (e.code === Code.FailedPrecondition) {
    const [refusal] = e.findDetails(LevelUpRefusalSchema);
    if (refusal) {
      return {
        kind: 'refusal',
        message: refusalMessage(refusal),
        step: refusalStep(refusal.reason, refusal.field, abilityStep),
      };
    }
    const [blocked] = e.findDetails(CharacterBlockedSchema);
    return { kind: 'blocked', message: blockedMessage(blocked?.reason), reason: blocked?.reason };
  }
  return {
    kind: 'other',
    message: describeConnectError(e, {
      [Code.PermissionDenied]:
        'Só o jogador dono do personagem sobe o nível. O mestre edita a ficha.',
      [Code.NotFound]: 'Esse personagem não existe, ou você não pode vê-lo.',
      [Code.InvalidArgument]: 'Alguma escolha não vale. Confira cada passo.',
    }),
  };
}

/** The level the SRD stops at: a character there has nothing left to level up to. */
const MAX_LEVEL = 20;

/**
 * Why a character cannot level up now, in words that say what is missing: a milestone the master has not marked, or
 * the XP still to come, from the sheet's own numbers (the campaign's mode is known). A character at level 20 is done.
 */
export function cannotLevelUpMessage(
  mode: XpMode,
  xp: number,
  nextLevelXp: number,
  level: number,
): string {
  if (level >= MAX_LEVEL) {
    return 'Este personagem já está no nível máximo.';
  }
  if (mode === XpMode.MILESTONES) {
    return 'Falta o mestre marcar um marco para este personagem. Quando ele marcar, o botão aparece na ficha.';
  }
  if (nextLevelXp > xp) {
    return tight(
      `Faltam ${formatInt(nextLevelXp - xp)} XP para o nível ${level + 1} (${formatInt(nextLevelXp)} XP). Quando o XP chegar, o botão aparece na ficha.`,
    );
  }
  return blockedMessage(CharacterBlockedReason.CANNOT_LEVEL_UP);
}
