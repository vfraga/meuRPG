import { Code, ConnectError } from '@connectrpc/connect';

import { XpMode } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  type XPBlocked,
  XPBlockedReason,
  XPBlockedSchema,
} from '../../../gen/meurpg/progression/v1/progression_pb';
import { describeConnectError } from '../connect/connect-errors';

/** The typed detail of a `failed_precondition` from `ProgressionService`, or
 * `null` (another code, or another detail). Never read from the message. */
export function xpBlocked(err: unknown): XPBlocked | null {
  const connectErr = ConnectError.from(err, Code.Unavailable);
  if (connectErr.code !== Code.FailedPrecondition) {
    return null;
  }
  return connectErr.findDetails(XPBlockedSchema)[0] ?? null;
}

/** Whether the call changed nothing because the screen was stale: another
 * award is the last now (`expected_award_id`), or the data moved under it. */
export function xpAborted(err: unknown): boolean {
  return ConnectError.from(err, Code.Unavailable).code === Code.Aborted;
}

/** Portuguese for every `XPBlocked` reason: what happened and what to do. */
export function xpBlockedMessage(blocked: XPBlocked): string {
  switch (blocked.reason) {
    case XPBlockedReason.XP_BLOCKED_REASON_MODE_NOT_ALLOWED:
      switch (blocked.xpMode) {
        case XpMode.MILESTONES:
          return 'Esta campanha conta marcos, não XP. Use "Registrar marco".';
        case XpMode.GOLD:
          return 'Esta campanha conta XP por ouro: dê o ouro encontrado ou um valor avulso.';
        case XpMode.ENEMIES:
          return 'Esta campanha conta XP por inimigos derrotados: dê o XP de um combate ou um valor avulso.';
        default:
          return 'Esta campanha não aceita essa forma de dar XP.';
      }
    case XPBlockedReason.XP_BLOCKED_REASON_ENCOUNTER_NOT_ENDED:
      return 'O combate ainda não terminou. Encerre o combate para dar o XP dele.';
    case XPBlockedReason.XP_BLOCKED_REASON_ALREADY_AWARDED:
      return 'O XP desse combate já foi dado. Para dar de novo, desfaça esse prêmio em "Experiência", na página da campanha.';
    case XPBlockedReason.XP_BLOCKED_REASON_NOTHING_TO_GIVE:
      return 'Não há XP para dar: o total é pequeno demais para cada um receber pelo menos 1 XP.';
    case XPBlockedReason.XP_BLOCKED_REASON_NOTHING_TO_UNDO:
      return 'Não há nenhum prêmio para desfazer. A tela foi atualizada.';
    case XPBlockedReason.XP_BLOCKED_REASON_CHARACTER_NOT_ELIGIBLE:
      return 'Um dos personagens marcados não pode receber: ele morreu ou saiu da campanha. A lista foi atualizada.';
    case XPBlockedReason.XP_BLOCKED_REASON_MILESTONE_ALREADY_REACHED:
      return 'Esse marco já foi alcançado. A lista foi atualizada.';
    case XPBlockedReason.XP_BLOCKED_REASON_MILESTONE_NOT_REACHED:
      return 'Esse marco não está mais alcançado: ele foi desfeito. A lista foi atualizada.';
    case XPBlockedReason.XP_BLOCKED_REASON_CHARACTER_ALREADY_MARKED:
      return 'Um dos personagens marcados já tem esse marco. A lista foi atualizada.';
    case XPBlockedReason.XP_BLOCKED_REASON_TREASURE_NOT_FOUND_YET:
      return 'Um dos tesouros marcados não está mais como encontrado: o mestre desmarcou o achado. A lista foi atualizada; confira e tente de novo.';
    case XPBlockedReason.XP_BLOCKED_REASON_TREASURE_ALREADY_CONVERTED:
      return 'Um dos tesouros marcados já virou XP em outro prêmio. A lista foi atualizada; confira e tente de novo.';
    case XPBlockedReason.XP_BLOCKED_REASON_TREASURES_OVER_LIMIT:
      return 'Os tesouros marcados valem mais de 1.000.000 PO juntos, o máximo de um prêmio. Desmarque alguns e converta o resto depois.';
    default:
      return 'Isso não pode ser feito agora. A tela foi atualizada.';
  }
}

/** Whether an undo found no award to undo (`NOTHING_TO_UNDO`): the list on screen is stale. */
export function xpNothingToUndo(err: unknown): boolean {
  return xpBlocked(err)?.reason === XPBlockedReason.XP_BLOCKED_REASON_NOTHING_TO_UNDO;
}

/** Whether the server did not find something the call named (`not_found`). */
export function xpNotFound(err: unknown): boolean {
  return ConnectError.from(err, Code.Unavailable).code === Code.NotFound;
}

/** The same for "Voltar à cidade": a mode that does not take it says so in
 * the treasures' words, a treasure that is gone says the list is stale, the
 * rest is as for any award. */
export function townErrorMessage(err: unknown): string {
  if (xpNotFound(err)) {
    return 'Um dos tesouros marcados não está mais no mapa. A lista foi atualizada; confira e tente de novo.';
  }
  const blocked = xpBlocked(err);
  if (blocked?.reason === XPBlockedReason.XP_BLOCKED_REASON_MODE_NOT_ALLOWED) {
    return blocked.xpMode === XpMode.MILESTONES
      ? 'Esta campanha conta marcos, não XP: o tesouro não vira XP.'
      : 'Esta campanha dá XP por inimigos, então o tesouro não vira XP. Ele continua aparecendo no resumo de cada sessão.';
  }
  return xpErrorMessage(err, 'voltar à cidade');
}

/** The Portuguese message for a failed XP call, by code and by typed detail
 * (progression.proto lists what each call returns). `what` finishes "Não
 * deu para …". */
export function xpErrorMessage(err: unknown, what = 'fazer isso'): string {
  const blocked = xpBlocked(err);
  if (blocked) {
    return xpBlockedMessage(blocked);
  }
  return describeConnectError(err, {
    [Code.InvalidArgument]: `Não deu para ${what}: confira o motivo e o valor e tente de novo.`,
    [Code.NotFound]:
      'Essa campanha não existe mais, ou você não faz parte dela. Recarregue a página.',
    [Code.PermissionDenied]: 'Só o mestre da campanha pode fazer isso.',
    [Code.ResourceExhausted]:
      'A campanha já tem 100 marcos. Remova um marco planejado para escrever outro.',
    [Code.Aborted]:
      'O XP mudou enquanto você agia. A tela foi atualizada; confira e tente de novo.',
    [Code.Unavailable]: `Não deu para ${what}: o servidor não respondeu. Tente de novo.`,
  });
}
