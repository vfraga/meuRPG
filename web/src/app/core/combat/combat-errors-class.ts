import { EncounterBlockedReason } from '../../../gen/meurpg/play/v1/combat_pb';

/** The refusals of what a class or a table maneuver asks of the turn, and of Ready (Preparar): what happened and what to do instead. */
export const CLASS_BLOCKED_TEXT: ReadonlyMap<EncounterBlockedReason, string> = new Map([
  [
    EncounterBlockedReason.RECKLESS_TOO_LATE,
    'O ataque descuidado se declara no primeiro ataque do turno, e você já atacou neste turno.',
  ],
  [
    EncounterBlockedReason.FRENZY_UNAVAILABLE,
    'Só o bárbaro do Caminho do Berserker tem frenesi. Entre em fúria sem ele.',
  ],
  [
    EncounterBlockedReason.NO_MELEE_HIT,
    'Essa manobra pede um acerto corpo a corpo com arma neste turno, e você ainda não acertou.',
  ],
  [
    EncounterBlockedReason.WILD_SHAPE_ENDED,
    'A forma já acabou. Para virar fera de novo, use Forma Selvagem.',
  ],
  [
    EncounterBlockedReason.READY_TRIGGER,
    'Escreva o gatilho da ação preparada, com até 200 caracteres. Nada foi gasto.',
  ],
  [
    EncounterBlockedReason.READY_ACTION,
    'Essa ação não pode ser preparada agora. Escolha outra. Nada foi gasto.',
  ],
  [
    EncounterBlockedReason.READY_SPELL_TIME,
    'Só uma magia de 1 ação pode ser preparada. Escolha outra. Nada foi gasto.',
  ],
  [
    EncounterBlockedReason.READY_SPELL_NOT_RELEASABLE,
    'Esta magia não pode ser preparada: ela pede uma escolha que só se faz ao conjurar na sua vez. Nada foi gasto.',
  ],
  [
    EncounterBlockedReason.READY_OWN_TURN,
    'A ação preparada só dispara fora do turno de quem a preparou. Nada foi gasto.',
  ],
]);
