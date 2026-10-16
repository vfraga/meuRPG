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
import {
  ResourceBlockedReason,
  ResourceBlockedSchema,
} from '../../../gen/meurpg/play/v1/resources_pb';
import {
  type ContestBlocked,
  ContestBlockedReason,
  ContestBlockedSchema,
} from '../../../gen/meurpg/play/v1/contest_types_pb';
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

/** The typed detail of a `failed_precondition` from `ContestService` (the grapple, the shove, Hide, Help, the group check), or
 * `null`. The reasons the combat already had keep coming as `EncounterBlocked`. Never read from the message. */
export function contestBlocked(err: unknown): ContestBlocked | null {
  const connectErr = ConnectError.from(err, Code.Unavailable);
  if (connectErr.code !== Code.FailedPrecondition) {
    return null;
  }
  return connectErr.findDetails(ContestBlockedSchema)[0] ?? null;
}

/** Why a contest or a special action was refused, in words (W7-X). Nothing here names a total, a DC or a creature the player does not see. */
const CONTEST_BLOCKED_TEXT: Partial<Record<ContestBlockedReason, string>> = {
  [ContestBlockedReason.TARGET_TOO_BIG]: 'Grande demais: no máximo um tamanho acima do seu.',
  [ContestBlockedReason.NOT_AWAITING]:
    'Essa disputa não espera mais por isso. A tela foi atualizada.',
  [ContestBlockedReason.CONTEST_OPEN]: 'Uma disputa sua ainda espera a resposta.',
  [ContestBlockedReason.NOT_GRAPPLED]: 'Não há agarrão para soltar. A tela foi atualizada.',
  [ContestBlockedReason.PUSH_BLOCKED]: 'Há algo na casa de trás: o empurrão não sai do lugar.',
  [ContestBlockedReason.NO_ROOM_TO_DRAG]:
    'Não dá para arrastar por aí: não há casa livre atrás de você para quem você segura.',
  [ContestBlockedReason.SURPRISED]:
    'Surpresa: não se move, não age e não reage até o fim do turno.',
  [ContestBlockedReason.NOT_AVAILABLE]: 'Essa ação não está na sua ficha.',
  [ContestBlockedReason.NOT_AN_ALLY]: 'Só dá para ajudar um aliado que ainda está de pé.',
  [ContestBlockedReason.TASK_NOT_AVAILABLE]: 'Não dá para ajudar nessa tarefa.',
  [ContestBlockedReason.HIDE_NOT_PENDING]:
    'Esse esconderijo já foi decidido. A tela foi atualizada.',
  [ContestBlockedReason.GROUP_CHECK_OPEN]: 'Já há um teste aberto. Encerre-o antes de pedir outro.',
  [ContestBlockedReason.GROUP_CHECK_CLOSED]: 'O mestre já encerrou esse teste.',
  [ContestBlockedReason.NOT_IN_GROUP_CHECK]: 'O mestre não pediu esse teste ao seu personagem.',
  [ContestBlockedReason.ALREADY_ANSWERED]: 'Você já rolou esse teste.',
  [ContestBlockedReason.COMBAT_BEGUN]: 'O combate já começou.',
  [ContestBlockedReason.NOT_HIDDEN]: 'Ninguém está escondido aqui. A tela foi atualizada.',
};

export function contestBlockedMessage(blocked: ContestBlocked): string {
  return CONTEST_BLOCKED_TEXT[blocked.reason] ?? 'Isso não vale agora. A tela foi atualizada.';
}

/** What any action says while a roll of the character waits for the answer about a Bardic Inspiration die: the
 * combatant does nothing else until it is answered (`ResourceBlocked` INSPIRATION_PENDING, from any call). */
export const INSPIRATION_PENDING_TEXT = 'Responda primeiro à pergunta da Inspiração de Bardo.';

/** The same while a roll of a halfling waits for the Lucky answer, "Tirou 1: rolar de novo?" (`ResourceBlocked`
 * LUCKY_PENDING): that question comes first. */
export const LUCKY_PENDING_TEXT =
  'Responda primeiro à pergunta do Sortudo (tirou 1: rolar de novo?).';

/** The sentence of a refusal while a roll of the character waits for an answer (INSPIRATION_PENDING or LUCKY_PENDING),
 * `''` for any other reason. */
export function pendingQuestionReasonText(reason: ResourceBlockedReason | undefined): string {
  switch (reason) {
    case ResourceBlockedReason.INSPIRATION_PENDING:
      return INSPIRATION_PENDING_TEXT;
    case ResourceBlockedReason.LUCKY_PENDING:
      return LUCKY_PENDING_TEXT;
    default:
      return '';
  }
}

/** The same for any error: `''` unless it is such a refusal. */
function pendingQuestionText(err: unknown): string {
  const connectErr = ConnectError.from(err, Code.Unavailable);
  return connectErr.code === Code.FailedPrecondition
    ? pendingQuestionReasonText(connectErr.findDetails(ResourceBlockedSchema)[0]?.reason)
    : '';
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

/** What each refusal of the combat says when the words do not depend on a number: the Portuguese sentence, by reason. */
const BLOCKED_TEXT: ReadonlyMap<EncounterBlockedReason, string> = new Map([
  [EncounterBlockedReason.ENCOUNTER_ALREADY_OPEN, 'A sessão já tem um combate em andamento.'],
  [EncounterBlockedReason.NO_CURRENT_MAP, 'Escolha o mapa do combate antes de iniciar.'],
  [
    EncounterBlockedReason.MAP_HAS_NO_GRID,
    'Esse mapa ainda não tem grade. Defina a grade para iniciar o combate.',
  ],
  [EncounterBlockedReason.NOT_IN_SETUP, 'O combate já começou.'],
  [EncounterBlockedReason.NOT_ACTIVE, 'O combate não está em andamento.'],
  [EncounterBlockedReason.ENCOUNTER_ENDED, 'Esse combate já terminou.'],
  [EncounterBlockedReason.INITIATIVE_MISSING, 'Ainda falta a iniciativa de alguém.'],
  [EncounterBlockedReason.INITIATIVE_ALREADY_SET, 'Sua iniciativa já foi rolada.'],
  [
    EncounterBlockedReason.WRONG_DICE_MODE,
    'A campanha mudou a forma de rolar os dados. Recarregue a página.',
  ],
  [EncounterBlockedReason.NOT_YOUR_TURN, 'Não é a sua vez.'],
  [EncounterBlockedReason.NOT_PLACED, 'Esse combatente ainda não está no mapa.'],
  [EncounterBlockedReason.NOT_PRONE, 'Você já está de pé. A tela foi atualizada.'],
  [EncounterBlockedReason.CANNOT_STAND_UP, 'Sem velocidade, não dá para se levantar.'],
  [
    EncounterBlockedReason.MOVE_BLOCKED,
    'Não dá para passar por aí: há uma parede ou outra criatura no caminho.',
  ],
  [EncounterBlockedReason.WALL_ON_SQUARE, 'Esse quadrado é uma parede: ninguém fica dentro dele.'],
  [EncounterBlockedReason.NO_FREE_SQUARE, 'Não há quadrado livre longe dos jogadores neste mapa.'],
  [EncounterBlockedReason.TARGET_DEAD, 'Não dá para curar quem já morreu.'],
  [EncounterBlockedReason.ENEMY_IN_THE_WAY, 'Um inimigo está no caminho.'],
  [EncounterBlockedReason.TARGET_COVER_TOTAL, 'O alvo está atrás de cobertura total.'],
  [EncounterBlockedReason.SQUARE_OCCUPIED, 'Ocupado: escolha outro quadrado.'],
  [EncounterBlockedReason.PLAYER_IN_COMBAT, 'Um jogador só sai do combate antes de ele começar.'],
  [EncounterBlockedReason.ACTION_USED, 'Sua ação já foi usada neste turno.'],
  [EncounterBlockedReason.BONUS_ACTION_USED, 'Sua ação bônus já foi usada neste turno.'],
  [EncounterBlockedReason.ATTACKS_USED, 'Os ataques desta ação já foram usados.'],
  [EncounterBlockedReason.TARGET_DEFEATED, 'Esse alvo já foi derrotado. Escolha outro.'],
  [
    EncounterBlockedReason.PENDING_DAMAGE,
    'Ainda falta rolar ou aplicar o dano do ataque antes de passar o turno.',
  ],
  [EncounterBlockedReason.DAMAGE_ALREADY_ROLLED, 'O dano desse ataque já foi rolado.'],
  [EncounterBlockedReason.DAMAGE_NOT_ROLLED, 'O dano ainda não foi rolado.'],
  [EncounterBlockedReason.DAMAGE_RESOLVED, 'Esse dano já foi aplicado ou descartado.'],
  [
    EncounterBlockedReason.NOTHING_TO_UNDO,
    'Não há mais nada para desfazer: só a última ação pode ser desfeita.',
  ],
  [EncounterBlockedReason.COMBATANT_DOWN, 'Quem está caído não age.'],
  [
    EncounterBlockedReason.REACTION_PENDING,
    'Uma reação ainda espera resposta. Espere a resposta ou responda no lugar de quem reage.',
  ],
  [
    EncounterBlockedReason.CONCENTRATION_SAVE_PENDING,
    'Um teste de concentração ainda espera resposta.',
  ],
  [
    EncounterBlockedReason.NOT_YOUR_TURN_TO_ANSWER,
    'Essa reação não espera a sua resposta agora. A tela foi atualizada.',
  ],
  [
    EncounterBlockedReason.REACTION_NEEDS_ROLL,
    'Falta o dado: role no app ou digite o resultado do dado.',
  ],
  [
    EncounterBlockedReason.NOT_AWAITING_REACTION,
    'Esse acerto não espera mais uma reação. A tela foi atualizada.',
  ],
  [EncounterBlockedReason.REACTION_USED, 'A reação já foi usada: ela volta no começo da sua vez.'],
  [
    EncounterBlockedReason.ALREADY_USED_THIS_TURN,
    'Esse recurso só pode ser usado uma vez por turno, e já foi usado neste.',
  ],
  [
    EncounterBlockedReason.BONUS_ACTION_SPELL_LIMIT,
    'Quem conjura uma magia com ação bônus não conjura outra no mesmo turno, a não ser um truque de 1 ação.',
  ],
  [
    EncounterBlockedReason.ATTACK_ACTION_FIRST,
    'A Rajada de Golpes vem logo depois da ação de Atacar.',
  ],
  [EncounterBlockedReason.DEATH_SAVE_DUE, 'Role o teste contra a morte antes de encerrar o turno.'],
  [
    EncounterBlockedReason.DEATH_SAVE_NOT_DUE,
    'Não há teste contra a morte para rolar agora. A tela foi atualizada.',
  ],
  [
    EncounterBlockedReason.NOT_DYING,
    'Esse personagem não falhou três testes contra a morte. A tela foi atualizada.',
  ],
  [
    EncounterBlockedReason.OPPORTUNITY_PENDING,
    'Esperando a reação do mestre: um ataque de oportunidade ainda não foi respondido.',
  ],
  // The turn waits for the master's answer about hidden creatures an area hit: a player reads the same words as any wait
  // for the master, never why (RN-10). The master's own screen says what it waits for, so this refusal is never news to him.
  [EncounterBlockedReason.HIDDEN_REVEAL_PENDING, 'Esperando o mestre.'],
  [
    EncounterBlockedReason.NOT_ENDED,
    'O combate ainda não terminou. Os destaques aparecem quando ele acabar.',
  ],
  // The creatures and Wild Shape (MR-037): what the server refused, in words.
  [
    EncounterBlockedReason.CASTING_TIME_TOO_LONG,
    'Essa magia leva mais tempo do que um combate dá. Conjure fora do combate.',
  ],
  [
    EncounterBlockedReason.SUMMON_CHOICE_INVALID,
    'Essa escolha não vale para o espaço que você usou. Mude a quantidade, a criatura ou o espaço e tente de novo.',
  ],
  [EncounterBlockedReason.SUMMON_NEEDS_INITIATIVE, 'Falta o d20 da iniciativa das criaturas.'],
  [
    EncounterBlockedReason.CREATURE_CANNOT_ATTACK,
    'Essa criatura não ataca assim: um familiar não ataca, e o do Pacto da Corrente só com a reação.',
  ],
  [
    EncounterBlockedReason.TOO_MANY_COMBATANTS,
    'Não cabem mais combatentes neste combate. Nada foi gasto.',
  ],
  [
    EncounterBlockedReason.SUMMON_IN_COMBAT,
    'Há um combate em andamento: conjure pela sua vez, na tela do combate.',
  ],
  [
    EncounterBlockedReason.WILD_SHAPE_BEAST_NOT_ALLOWED,
    'Essa não é uma das feras que o seu nível permite. Feche a lista e abra de novo.',
  ],
  [EncounterBlockedReason.ALREADY_IN_WILD_SHAPE, 'Você já está na forma de uma fera.'],
  [
    EncounterBlockedReason.NOT_IN_WILD_SHAPE,
    'Você já está na sua forma normal. A tela foi atualizada.',
  ],
  [
    EncounterBlockedReason.WILD_SHAPE_NO_SPELLS,
    'Na forma de fera não dá para conjurar. Volte à forma normal e tente de novo.',
  ],
  [EncounterBlockedReason.DOOR_LOCKED, LOCKED_DOOR_FIRST_STEP_TEXT],
  // Combat without a map (RN-25): what each side of the line refuses.
  [
    EncounterBlockedReason.THEATRE_ONLY,
    'Este combate tem mapa: o movimento é no mapa, e o ataque de oportunidade o app acha sozinho. A tela foi atualizada.',
  ],
  [
    EncounterBlockedReason.NEEDS_A_MAP,
    'Sem mapa, isso não existe: o combate não tem posições. A tela foi atualizada.',
  ],
  [
    EncounterBlockedReason.THEATRE_HAS_NO_MAP,
    'Um combate sem mapa não leva um mapa. Escolha "Com mapa" ou tire o mapa.',
  ],
  [
    EncounterBlockedReason.NO_OPPORTUNITY,
    'Esse ataque de oportunidade não pode ser oferecido agora: quem ia reagir não pode atacar, está do mesmo lado, ou a oferta já espera.',
  ],
  [EncounterBlockedReason.RAGING_CANNOT_CAST, 'Você está em fúria: não pode lançar magias.'],
  [
    EncounterBlockedReason.RAGE_END_PENDING,
    'A pergunta da fúria espera uma resposta antes de passar o turno.',
  ],
  [
    EncounterBlockedReason.RAGE_END_NOT_PENDING,
    'Essa pergunta da fúria já foi respondida. A tela foi atualizada.',
  ],
  [EncounterBlockedReason.NOT_RAGING, 'Esse personagem não está em fúria. A tela foi atualizada.'],
  [
    EncounterBlockedReason.ROLL_MODE_NEEDS_APPROVAL,
    'Esse modo é melhor do que o sugerido: peça ao mestre.',
  ],
  [EncounterBlockedReason.ROLL_MODE_REQUEST_PENDING, 'O mestre ainda não respondeu ao seu pedido.'],
  [
    EncounterBlockedReason.ROLL_MODE_REQUEST_CLOSED,
    'Esse pedido ao mestre já foi respondido, cancelado ou usado. A tela foi atualizada.',
  ],
  [
    EncounterBlockedReason.EXTRA_NOT_AVAILABLE,
    'Um dos extras marcados não vale mais. A tela foi atualizada.',
  ],
  [
    EncounterBlockedReason.DAMAGE_PART_NOT_REMOVABLE,
    'Só um extra pode ser tirado do dano, e uma vez só.',
  ],
  // Effects that last (RN-22): never the cause (a hidden effect, or a condition only the master reads); the words are the same
  // for a player whether it is lethargy, a spell or a condition.
  [EncounterBlockedReason.CANNOT_ACT, 'Não dá para agir agora.'],
  [
    EncounterBlockedReason.EXTRA_ACTION_UNAVAILABLE,
    'A ação extra não está disponível para isso agora: ela já foi usada neste turno ou não vale para essa ação.',
  ],
  [
    EncounterBlockedReason.CHARACTER_RESERVED,
    'Um personagem reservado (ainda sem jogador) não entra no combate. Desmarque-o e tente de novo.',
  ],
]);

/** The refusals whose words carry a number the server sent (a distance, a level, the recharge). */
const BLOCKED_TEXT_WITH_NUMBERS: ReadonlyMap<
  EncounterBlockedReason,
  (b: EncounterBlocked) => string
> = new Map([
  [EncounterBlockedReason.TOO_FAR, tooFarText],
  [
    EncounterBlockedReason.TARGET_OUT_OF_REACH,
    (b) => `Longe demais: faltam ${metersText(b.missingFt)} para chegar ao alvo.`,
  ],
  [
    EncounterBlockedReason.NO_SLOT,
    (b) =>
      b.minLevel > 0
        ? `Não há espaço de ${circleLabel(b.minLevel)} ou maior livre.`
        : 'Não há espaço de magia livre.',
  ],
  [EncounterBlockedReason.NO_USES, (b) => `Sem usos: ${rechargeText(b.recharge)}.`],
]);

/** The move that goes further than the movement left, or the jump past its limit. */
function tooFarText(blocked: EncounterBlocked): string {
  // In tenths of a foot when the server sent them; the whole feet are for the old app.
  const missing = metersFixed((blocked.missingDft || blocked.missingFt * 10) / 10);
  // A jump longer than its limit says the limit, and whether it had a running start.
  return blocked.jumpLimitDft > 0
    ? `Longe demais para o seu salto: ele vai até ${metersFixed(blocked.jumpLimitDft / 10)}${blocked.jumpRunningStart ? ' com corrida' : ' parado'}. Faltam ${missing}.`
    : `Esse caminho custa mais do que o movimento que sobra: faltam ${missing}.`;
}

/** Why a move was refused, for the reach line under the map (E6-10). */
export function blockedMessage(blocked: EncounterBlocked): string {
  return (
    BLOCKED_TEXT.get(blocked.reason) ??
    BLOCKED_TEXT_WITH_NUMBERS.get(blocked.reason)?.(blocked) ??
    'O combate não está num estado que aceite isso. A tela foi atualizada.'
  );
}

/** The Portuguese message for a failed combat call: what happened and how to
 * fix it, by code and by typed detail (combat.proto lists what each call
 * returns). `what` finishes "Não deu para …". */
export function combatErrorMessage(err: unknown, what = 'fazer isso'): string {
  const blocked = encounterBlocked(err);
  if (blocked) {
    return blockedMessage(blocked);
  }
  const contest = contestBlocked(err);
  if (contest) {
    return contestBlockedMessage(contest);
  }
  const pending = pendingQuestionText(err);
  if (pending) {
    return pending;
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
