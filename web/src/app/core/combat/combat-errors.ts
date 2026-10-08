import { Code, ConnectError } from '@connectrpc/connect';

import {
  GameSessionBlockedReason,
  GameSessionBlockedSchema,
} from '../../../gen/meurpg/play/v1/play_pb';
import {
  EncounterBlockedReason,
  type EncounterBlocked,
  EncounterBlockedSchema,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { describeConnectError } from '../connect/connect-errors';
import { Recharge } from '../../../gen/meurpg/rules/v1/rules_pb';
import { circleLabel } from './combat-grid';
import { metersFixed, metersText } from '../units';

/** What a refusal says when the first square of a move is a locked door (RN-26): nothing moved and nothing was spent. A move that gets
 * some squares first is no refusal: the "Mover" page says "A porta está trancada." and the map keeps drawing "Porta fechada" (the app
 * does not remember the lock for players). */
export const LOCKED_DOOR_FIRST_STEP_TEXT =
  'A porta está trancada: você não saiu do lugar. Só o mestre a destranca.';

/** The typed detail of a `failed_precondition` from `CombatService`, or
 * `null` (another code, or another detail). Never read from the message. */
export function encounterBlocked(err: unknown): EncounterBlocked | null {
  const connectErr = ConnectError.from(err, Code.Unavailable);
  if (connectErr.code !== Code.FailedPrecondition) {
    return null;
  }
  return connectErr.findDetails(EncounterBlockedSchema)[0] ?? null;
}

/** Whether the campaign has no open session (`GameSessionBlocked`). */
export function sessionClosed(err: unknown): boolean {
  const connectErr = ConnectError.from(err, Code.Unavailable);
  return (
    connectErr.code === Code.FailedPrecondition &&
    connectErr.findDetails(GameSessionBlockedSchema)[0]?.reason ===
      GameSessionBlockedReason.NO_OPEN_SESSION
  );
}

/** When a feature's uses come back, for "Sem usos: volta num descanso curto". */
export function rechargeText(recharge: Recharge): string {
  switch (recharge) {
    case Recharge.SHORT_REST:
      return 'volta num descanso curto';
    case Recharge.LONG_REST:
      return 'volta num descanso longo';
    case Recharge.DAWN:
      return 'volta ao amanhecer';
    default:
      return 'só o mestre devolve';
  }
}

/** Why a move was refused, for the reach line under the map (E6-10). */
export function blockedMessage(blocked: EncounterBlocked): string {
  switch (blocked.reason) {
    case EncounterBlockedReason.ENCOUNTER_ALREADY_OPEN:
      return 'A sessão já tem um combate em andamento.';
    case EncounterBlockedReason.NO_CURRENT_MAP:
      return 'Escolha o mapa do combate antes de iniciar.';
    case EncounterBlockedReason.MAP_HAS_NO_GRID:
      return 'Esse mapa ainda não tem grade. Defina a grade para iniciar o combate.';
    case EncounterBlockedReason.NOT_IN_SETUP:
      return 'O combate já começou.';
    case EncounterBlockedReason.NOT_ACTIVE:
      return 'O combate não está em andamento.';
    case EncounterBlockedReason.ENCOUNTER_ENDED:
      return 'Esse combate já terminou.';
    case EncounterBlockedReason.INITIATIVE_MISSING:
      return 'Ainda falta a iniciativa de alguém.';
    case EncounterBlockedReason.INITIATIVE_ALREADY_SET:
      return 'Sua iniciativa já foi rolada.';
    case EncounterBlockedReason.WRONG_DICE_MODE:
      return 'A campanha mudou a forma de rolar os dados. Recarregue a página.';
    case EncounterBlockedReason.NOT_YOUR_TURN:
      return 'Não é a sua vez.';
    case EncounterBlockedReason.NOT_PLACED:
      return 'Esse combatente ainda não está no mapa.';
    case EncounterBlockedReason.TOO_FAR: {
      // In tenths of a foot when the server sent them; the whole feet are for the old app.
      const missing = metersFixed((blocked.missingDft || blocked.missingFt * 10) / 10);
      // A jump longer than its limit says the limit, and whether it had a running start.
      return blocked.jumpLimitDft > 0
        ? `Longe demais para o seu salto: ele vai até ${metersFixed(blocked.jumpLimitDft / 10)}${blocked.jumpRunningStart ? ' com corrida' : ' parado'}. Faltam ${missing}.`
        : `Esse caminho custa mais do que o movimento que sobra: faltam ${missing}.`;
    }
    case EncounterBlockedReason.MOVE_BLOCKED:
      return 'Não dá para passar por aí: há uma parede ou outra criatura no caminho.';
    case EncounterBlockedReason.ENEMY_IN_THE_WAY:
      return 'Um inimigo está no caminho.';
    case EncounterBlockedReason.TARGET_COVER_TOTAL:
      return 'O alvo está atrás de cobertura total.';
    case EncounterBlockedReason.SQUARE_OCCUPIED:
      return 'Ocupado: escolha outro quadrado.';
    case EncounterBlockedReason.PLAYER_IN_COMBAT:
      return 'Um jogador só sai do combate antes de ele começar.';
    case EncounterBlockedReason.ACTION_USED:
      return 'Sua ação já foi usada neste turno.';
    case EncounterBlockedReason.BONUS_ACTION_USED:
      return 'Sua ação bônus já foi usada neste turno.';
    case EncounterBlockedReason.ATTACKS_USED:
      return 'Os ataques desta ação já foram usados.';
    case EncounterBlockedReason.TARGET_OUT_OF_REACH:
      return `Longe demais: faltam ${metersText(blocked.missingFt)} para chegar ao alvo.`;
    case EncounterBlockedReason.TARGET_DEFEATED:
      return 'Esse alvo já foi derrotado. Escolha outro.';
    case EncounterBlockedReason.PENDING_DAMAGE:
      return 'Ainda falta rolar ou aplicar o dano do ataque antes de passar o turno.';
    case EncounterBlockedReason.DAMAGE_ALREADY_ROLLED:
      return 'O dano desse ataque já foi rolado.';
    case EncounterBlockedReason.DAMAGE_NOT_ROLLED:
      return 'O dano ainda não foi rolado.';
    case EncounterBlockedReason.DAMAGE_RESOLVED:
      return 'Esse dano já foi aplicado ou descartado.';
    case EncounterBlockedReason.NOTHING_TO_UNDO:
      return 'Não há mais nada para desfazer: só a última ação pode ser desfeita.';
    case EncounterBlockedReason.COMBATANT_DOWN:
      return 'Quem está caído não age.';
    case EncounterBlockedReason.REACTION_PENDING:
      return 'Esse acerto espera a reação do alvo (Escudo). Espere o jogador ou responda por ele.';
    case EncounterBlockedReason.NOT_AWAITING_REACTION:
      return 'Esse acerto não espera mais uma reação. A tela foi atualizada.';
    case EncounterBlockedReason.REACTION_USED:
      return 'A reação já foi usada: ela volta no começo da sua vez.';
    case EncounterBlockedReason.NO_SLOT:
      return blocked.minLevel > 0
        ? `Não há espaço de ${circleLabel(blocked.minLevel)} ou maior livre.`
        : 'Não há espaço de magia livre.';
    case EncounterBlockedReason.NO_USES:
      return `Sem usos: ${rechargeText(blocked.recharge)}.`;
    case EncounterBlockedReason.ALREADY_USED_THIS_TURN:
      return 'Esse recurso só pode ser usado uma vez por turno, e já foi usado neste.';
    case EncounterBlockedReason.DEATH_SAVE_DUE:
      return 'Role o teste contra a morte antes de encerrar o turno.';
    case EncounterBlockedReason.DEATH_SAVE_NOT_DUE:
      return 'Não há teste contra a morte para rolar agora. A tela foi atualizada.';
    case EncounterBlockedReason.NOT_DYING:
      return 'Esse personagem não falhou três testes contra a morte. A tela foi atualizada.';
    case EncounterBlockedReason.OPPORTUNITY_PENDING:
      return 'Esperando a reação do mestre: um ataque de oportunidade ainda não foi respondido.';
    case EncounterBlockedReason.NOT_ENDED:
      return 'O combate ainda não terminou. Os destaques aparecem quando ele acabar.';
    // The creatures and Wild Shape (MR-037): what the server refused, in words.
    case EncounterBlockedReason.CASTING_TIME_TOO_LONG:
      return 'Essa magia leva mais tempo do que um combate dá. Conjure fora do combate.';
    case EncounterBlockedReason.SUMMON_CHOICE_INVALID:
      return 'Essa escolha não vale para o espaço que você usou. Mude a quantidade, a criatura ou o espaço e tente de novo.';
    case EncounterBlockedReason.SUMMON_NEEDS_INITIATIVE:
      return 'Falta o d20 da iniciativa das criaturas.';
    case EncounterBlockedReason.CREATURE_CANNOT_ATTACK:
      return 'Essa criatura não ataca assim: um familiar não ataca, e o do Pacto da Corrente só com a reação.';
    case EncounterBlockedReason.TOO_MANY_COMBATANTS:
      return 'Não cabem mais combatentes neste combate. Nada foi gasto.';
    case EncounterBlockedReason.SUMMON_IN_COMBAT:
      return 'Há um combate em andamento: conjure pela sua vez, na tela do combate.';
    case EncounterBlockedReason.WILD_SHAPE_BEAST_NOT_ALLOWED:
      return 'Essa não é uma das feras que o seu nível permite. Feche a lista e abra de novo.';
    case EncounterBlockedReason.ALREADY_IN_WILD_SHAPE:
      return 'Você já está na forma de uma fera.';
    case EncounterBlockedReason.NOT_IN_WILD_SHAPE:
      return 'Você já está na sua forma normal. A tela foi atualizada.';
    case EncounterBlockedReason.WILD_SHAPE_NO_SPELLS:
      return 'Na forma de fera não dá para conjurar. Volte à forma normal e tente de novo.';
    case EncounterBlockedReason.DOOR_LOCKED:
      return LOCKED_DOOR_FIRST_STEP_TEXT;
    // Combat without a map (RN-25): what each side of the line refuses.
    case EncounterBlockedReason.THEATRE_ONLY:
      return 'Este combate tem mapa: o movimento é no mapa, e o ataque de oportunidade o app acha sozinho. A tela foi atualizada.';
    case EncounterBlockedReason.NEEDS_A_MAP:
      return 'Sem mapa, isso não existe: o combate não tem posições. A tela foi atualizada.';
    case EncounterBlockedReason.THEATRE_HAS_NO_MAP:
      return 'Um combate sem mapa não leva um mapa. Escolha "Com mapa" ou tire o mapa.';
    case EncounterBlockedReason.NO_OPPORTUNITY:
      return 'Esse ataque de oportunidade não pode ser oferecido agora: quem ia reagir não pode atacar, está do mesmo lado, ou a oferta já espera.';
    default:
      return 'O combate não está num estado que aceite isso. A tela foi atualizada.';
  }
}

/** The Portuguese message for a failed combat call: what happened and how to
 * fix it, by code and by typed detail (combat.proto lists what each call
 * returns). `what` finishes "Não deu para …". */
export function combatErrorMessage(err: unknown, what = 'fazer isso'): string {
  const blocked = encounterBlocked(err);
  if (blocked) {
    return blockedMessage(blocked);
  }
  if (sessionClosed(err)) {
    return 'A sessão acabou: o combate só muda durante a sessão.';
  }
  return describeConnectError(err, {
    [Code.InvalidArgument]: `Não deu para ${what}: confira os campos e tente de novo.`,
    [Code.NotFound]: 'Esse combate não existe mais, ou você não o vê. Recarregue a página.',
    [Code.PermissionDenied]: 'Você não pode fazer isso agora.',
    [Code.Aborted]: 'O combate mudou enquanto você agia. A tela foi atualizada; tente de novo.',
    [Code.Unavailable]: `Não deu para ${what}: o servidor não respondeu. Tente de novo.`,
  });
}
