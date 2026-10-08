import { Code, ConnectError } from '@connectrpc/connect';

import {
  AbilityScoresRefusalReason,
  AbilityScoresRefusalSchema,
  CharacterBlockedReason as GenCharacterBlockedReason,
  CharacterBlockedSchema,
  InvalidFieldSchema,
  LevelUpRefusalReason,
  LevelUpRefusalSchema,
} from '../../../gen/meurpg/characters/v1/characters_pb';
import { describeConnectError } from '../connect/connect-errors';
import { CharacterBlockedReason } from './characters.types';

/**
 * The content key a player's save was refused for because the master switched it off in "Opções para os jogadores" (RN-23):
 * `failed_precondition` with `CharacterBlocked` `SWITCHED_OFF_CONTENT` and the key ("race:tiefling", "class:wizard"). `null` for
 * any other error. The editor shows it on the field that holds the key, by the typed reason and never by the message.
 */
export function switchedOffKey(err: unknown): string | null {
  const e = ConnectError.from(err, Code.Unavailable);
  if (e.code !== Code.FailedPrecondition) {
    return null;
  }
  const [detail] = e.findDetails(CharacterBlockedSchema);
  return detail?.reason === GenCharacterBlockedReason.SWITCHED_OFF_CONTENT
    ? detail.contentKey || ''
    : null;
}

/**
 * Turns a `CharacterBlocked.reason` into the message the sheet and the
 * editor show as-is. Kept separate from `describeCharacterError` so both can
 * be unit-tested without a `ConnectError` in hand — this one takes the
 * already-decoded local reason, not a wire enum.
 */
export function characterBlockedMessage(
  reason: CharacterBlockedReason | undefined,
  content?: ContentRef,
): string {
  switch (reason) {
    case 'archived_content':
      return `${contentWords(content)} foi arquivad${content?.masculine ? 'o' : 'a'} pelo mestre e não vale mais como escolha nova. Escolha outra opção${content?.step ? `, no passo ${content.step}` : ''}.`;
    case 'switched_off_content':
      return `${contentWords(content)} foi desligad${content?.masculine ? 'o' : 'a'} pelo mestre para os jogadores. Escolha outra opção${content?.step ? `, no passo ${content.step}` : ''}.`;
    case 'sheet_locked':
      return 'A ficha está travada porque a campanha já começou a jogar. Só o mestre pode editá-la agora.';
    case 'character_dead':
      return 'Esse personagem está morto e a ficha não pode mais ser editada.';
    case 'living_character_exists':
      return 'Você já tem um personagem vivo nesta campanha.';
    case 'story_locked':
      return 'O mestre ainda não liberou a edição da história. Peça para ele liberar em "Permitir editar a história".';
    case 'not_pending':
      return 'Esse personagem já foi aprovado e faz parte da campanha: não dá mais para recusá-lo.';
    case 'awaiting_approval':
      return 'Esse personagem ainda espera a sua aprovação. Aprove ou recuse antes.';
    default:
      return 'Não foi possível concluir a ação agora.';
  }
}

/** What a content key is, in words, for an error that names one: "A classe “Guardião do Vale”". */
export interface ContentRef {
  /** The Portuguese name from the catalog; empty when the catalog does not know the key. */
  readonly name: string;
  /** The kind of entry, from the key: "a classe", "o antecedente"... */
  readonly noun: string;
  readonly masculine: boolean;
  /** The step of the editor that holds the field. */
  readonly step: string;
}

const KEY_KINDS: Record<string, { noun: string; masculine: boolean; step: string }> = {
  class: { noun: 'classe', masculine: false, step: 'Básico' },
  subclass: { noun: 'subclasse', masculine: false, step: 'Básico' },
  race: { noun: 'raça', masculine: false, step: 'Básico' },
  subrace: { noun: 'sub-raça', masculine: false, step: 'Básico' },
  background: { noun: 'antecedente', masculine: true, step: 'Básico' },
  spell: { noun: 'magia', masculine: false, step: 'Magias' },
};

/** The words for a content key: its kind from the key, its name from `nameOf` (never the key itself). */
export function contentRef(key: string, nameOf: (key: string) => string | undefined): ContentRef {
  const kind = KEY_KINDS[key.split(':')[0]] ?? { noun: 'opção', masculine: false, step: '' };
  return { name: nameOf(key) ?? '', ...kind };
}

function contentWords(content: ContentRef | undefined): string {
  if (!content) {
    return 'Uma das opções';
  }
  const article = content.masculine ? 'O' : 'A';
  return content.name
    ? `${article} ${content.noun} “${content.name}”`
    : `${article} ${content.noun} escolhid${content.masculine ? 'o' : 'a'}`;
}

/** The sheet field an `invalid_argument` points at ("full.classes[1].class_key"), from its typed detail; `null` when it has none. */
export function invalidFieldPath(err: unknown): string | null {
  const connectErr = ConnectError.from(err, Code.Unavailable);
  if (connectErr.code !== Code.InvalidArgument) {
    return null;
  }
  return connectErr.findDetails(InvalidFieldSchema)[0]?.field ?? null;
}

/** Maps the wire `CharacterBlockedReason` enum (characters.proto) onto the
 * local, UI-facing union `characterBlockedMessage` reads. `UNSPECIFIED` and
 * any future value this app does not know about yet fall through to
 * `undefined`, which `characterBlockedMessage` already turns into a safe
 * generic message instead of throwing. */
function mapBlockedReason(
  reason: GenCharacterBlockedReason | undefined,
): CharacterBlockedReason | undefined {
  switch (reason) {
    case GenCharacterBlockedReason.SHEET_LOCKED:
      return 'sheet_locked';
    case GenCharacterBlockedReason.CHARACTER_DEAD:
      return 'character_dead';
    case GenCharacterBlockedReason.LIVING_CHARACTER_EXISTS:
      return 'living_character_exists';
    case GenCharacterBlockedReason.STORY_LOCKED:
      return 'story_locked';
    case GenCharacterBlockedReason.NOT_PENDING:
      return 'not_pending';
    case GenCharacterBlockedReason.AWAITING_APPROVAL:
      return 'awaiting_approval';
    case GenCharacterBlockedReason.ARCHIVED_CONTENT:
      return 'archived_content';
    case GenCharacterBlockedReason.SWITCHED_OFF_CONTENT:
      return 'switched_off_content';
    default:
      return undefined;
  }
}

/** The typed reason of an `AbilityScoresRefusal` an error carries, or `null` for any other error. */
export function abilityRefusalReason(err: unknown): AbilityScoresRefusalReason | null {
  const connectErr = ConnectError.from(err, Code.Unavailable);
  if (connectErr.code !== Code.FailedPrecondition) {
    return null;
  }
  return connectErr.findDetails(AbilityScoresRefusalSchema)[0]?.reason ?? null;
}

/** What each `AbilityScoresRefusal` says (RN-24): by the typed reason, never by the server's message. */
export function abilityRefusalMessage(reason: AbilityScoresRefusalReason): string {
  switch (reason) {
    case AbilityScoresRefusalReason.METHOD_NOT_ALLOWED:
      return 'O mestre não liberou esse jeito de fazer as habilidades nesta mesa. Escolha outro.';
    case AbilityScoresRefusalReason.NOT_STANDARD_ARRAY:
      return 'Os valores não são o conjunto padrão: cada valor do conjunto vai em uma habilidade, uma vez só.';
    case AbilityScoresRefusalReason.BAD_POINT_BUY:
      return 'A compra por pontos passou do total de pontos, ou tem um valor fora do limite. Confira os valores.';
    case AbilityScoresRefusalReason.NO_ROLLS_STORED:
      return 'Role as habilidades antes de criar o personagem.';
    case AbilityScoresRefusalReason.NOT_THE_ROLLS:
      return 'Os valores não são os seis resultados rolados. Use cada resultado uma vez.';
    case AbilityScoresRefusalReason.TYPED_OUT_OF_RANGE:
      return 'Um valor digitado está fora do limite da mesa. Confira os seis valores.';
    case AbilityScoresRefusalReason.DICE_FORCED_IN_APP:
      return 'Nesta campanha todos rolam no app: peça a rolagem ao servidor, sem digitar dados.';
    case AbilityScoresRefusalReason.DICE_FORCED_PHYSICAL:
      return 'Nesta campanha todos usam os próprios dados: digite os dados que você tirou.';
    case AbilityScoresRefusalReason.ROLLS_ALREADY_STORED:
      return 'Os dados já foram guardados e não mudam. Use os resultados que aparecem.';
    case AbilityScoresRefusalReason.EXTRA_BONUSES:
      return 'Os bônus manuais passam do que a raça e os incrementos no valor de habilidade deixam pôr.';
    default:
      return 'As habilidades não seguem as regras da mesa. Confira o passo "Habilidades".';
  }
}

/**
 * Maps any `CharacterService` error to a message a form can show as-is.
 *
 * For `failed_precondition`, this reads the `CharacterBlocked` detail off
 * the error itself (`findDetails(CharacterBlockedSchema)`) — the caller
 * never needs to guess or pass a reason in.
 */
export function describeCharacterError(
  err: unknown,
  nameOf?: (key: string) => string | undefined,
): string {
  const connectErr = ConnectError.from(err, Code.Unavailable);

  if (connectErr.code === Code.FailedPrecondition) {
    const [refusal] = connectErr.findDetails(AbilityScoresRefusalSchema);
    if (refusal) {
      return abilityRefusalMessage(refusal.reason);
    }
    // A new sheet whose hit points above level 1 are not what the table's rule allows (RN-24).
    const [hp] = connectErr.findDetails(LevelUpRefusalSchema);
    if (hp?.reason === LevelUpRefusalReason.HIT_POINTS_RULE) {
      return 'A mesa decidiu como se ganham os pontos de vida dos níveis acima do 1º. Use o jeito que ela deixa, no passo "Habilidades".';
    }
    const [detail] = connectErr.findDetails(CharacterBlockedSchema);
    const content = detail?.contentKey
      ? contentRef(detail.contentKey, nameOf ?? (() => undefined))
      : undefined;
    return characterBlockedMessage(mapBlockedReason(detail?.reason), content);
  }
  if (connectErr.code === Code.Aborted) {
    // AIP-154-style stale revision: someone else (the player, the master,
    // or the server locking the sheet) saved first.
    return 'A ficha mudou enquanto você editava. Recarregue a página e tente de novo.';
  }

  return describeConnectError(connectErr, {
    [Code.PermissionDenied]: 'Você não tem permissão para fazer isso.',
    [Code.NotFound]: 'Personagem não encontrado.',
    [Code.InvalidArgument]: invalidArgumentMessage(connectErr),
  });
}

/** "Classe 2: ..." when the server points at a class block; the generic line otherwise. */
function invalidArgumentMessage(err: ConnectError): string {
  const field = err.findDetails(InvalidFieldSchema)[0]?.field ?? '';
  const block = /^full\.classes\[(\d+)\]/.exec(field);
  if (block) {
    return `Classe ${Number(block[1]) + 1}: essa classe se repete ou não existe. Cada classe entra uma vez só; escolha outra no passo Básico.`;
  }
  return 'Confira os campos da ficha.';
}
