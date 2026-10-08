import { Code, ConnectError } from '@connectrpc/connect';

import {
  GameSessionBlockedReason,
  GameSessionBlockedSchema,
} from '../../../gen/meurpg/play/v1/play_pb';
import {
  SceneBlockedReason,
  type SceneBlocked,
  SceneBlockedSchema,
} from '../../../gen/meurpg/play/v1/scene_pb';
import { describeConnectError } from '../connect/connect-errors';

/** The typed detail of a `failed_precondition` from a scene call, or `null`.
 * Never read from the message. */
export function sceneBlocked(err: unknown): SceneBlocked | null {
  const connectErr = ConnectError.from(err, Code.Unavailable);
  if (connectErr.code !== Code.FailedPrecondition) {
    return null;
  }
  return connectErr.findDetails(SceneBlockedSchema)[0] ?? null;
}

/** What each refusal says, in words the master or the player can act on. */
export function sceneBlockedMessage(reason: SceneBlockedReason): string {
  switch (reason) {
    case SceneBlockedReason.NO_ACTIONS:
      return 'Essa cena não tem ações. Adicione ações no editor do mapa.';
    case SceneBlockedReason.NO_OPEN_SCENE:
      return 'O mestre fechou a cena. Espere ele abrir de novo.';
    case SceneBlockedReason.ALREADY_ROLLED:
      return 'Você não tem mais tentativas nessa ação. O mestre pode dar mais uma.';
    case SceneBlockedReason.WRONG_DICE_MODE:
      return 'A campanha mudou a forma de rolar os dados. Recarregue a página.';
    case SceneBlockedReason.NO_CHARACTER:
      return 'Você não tem um personagem vivo nesta campanha para rolar.';
    case SceneBlockedReason.STAGE_FULL:
      return 'A cena comporta 4 NPCs. Tire um para pôr outro.';
    default:
      return 'A cena não está num estado que aceite isso.';
  }
}

/** The Portuguese message for a failed scene call, by code and typed detail
 * (play.proto lists what each call returns). `what` finishes "Não deu para …". */
export function sceneErrorMessage(err: unknown, what = 'fazer isso'): string {
  const blocked = sceneBlocked(err);
  if (blocked) {
    return sceneBlockedMessage(blocked.reason);
  }
  const connectErr = ConnectError.from(err, Code.Unavailable);
  if (
    connectErr.code === Code.FailedPrecondition &&
    connectErr.findDetails(GameSessionBlockedSchema)[0]?.reason ===
      GameSessionBlockedReason.NO_OPEN_SESSION
  ) {
    return 'A sessão acabou: a cena só muda durante a sessão.';
  }
  return describeConnectError(err, {
    [Code.InvalidArgument]: `Não deu para ${what}: confira o número e tente de novo.`,
    [Code.NotFound]: 'Essa cena não existe mais, ou você não a vê. Recarregue a página.',
    [Code.PermissionDenied]: 'Você não pode fazer isso agora.',
    [Code.Unavailable]: `Não deu para ${what}: o servidor não respondeu. Tente de novo.`,
  });
}

/** What a failed stage call (`PutOnStage`, `TakeOffStage`, `SetSpeaker`) says
 * to the master, by code and typed detail. A scene that closed meanwhile is
 * not "the master closed the scene" here: the master did it. */
export function stageErrorMessage(err: unknown): string {
  const blocked = sceneBlocked(err);
  if (blocked?.reason === SceneBlockedReason.NO_OPEN_SCENE) {
    return 'Não há cena aberta, então não há palco. Abra uma cena para pôr NPCs em cena.';
  }
  const connectErr = ConnectError.from(err, Code.Unavailable);
  if (connectErr.code === Code.NotFound) {
    return 'Esse NPC não está mais na campanha ou não está em cena. A tela foi atualizada.';
  }
  return sceneErrorMessage(err, 'mudar o palco');
}
