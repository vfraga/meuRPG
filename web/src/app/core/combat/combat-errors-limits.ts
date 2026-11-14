import {
  EncounterBlockedReason,
  type EncounterBlocked,
} from '../../../gen/meurpg/play/v1/combat_pb';

/** The refusals of an action of a stat block that is spent (the master and the controller of the creature read them), of armour changed in combat, of a mount that cannot be marked, mounted or dismounted now and of a feature whose target or choice is no longer valid: what is wrong, and that nothing was spent. The refusal of an undo whose hit points changed is here too: nothing was undone. */
export const LIMITED_ACTION_BLOCKED_TEXT: ReadonlyMap<EncounterBlockedReason, string> = new Map([
  [
    EncounterBlockedReason.CREATURE_ACTION_RECHARGING,
    'Essa ação da criatura ainda não recarregou. Nada foi gasto.',
  ],
  [
    EncounterBlockedReason.CREATURE_ACTION_NO_USES_LEFT,
    'Essa ação da criatura não tem mais usos. Nada foi gasto.',
  ],
  [
    EncounterBlockedReason.UNDO_TARGET_CHANGED,
    'Não dá para desfazer: os pontos de vida de alguém mudaram depois dessa ação, e desfazer apagaria essa mudança. Nada foi desfeito.',
  ],
  [
    EncounterBlockedReason.ARMOR_IN_COMBAT,
    'Vestir ou tirar armadura leva minutos: não dá em pleno combate. Nada foi mudado.',
  ],
  [
    EncounterBlockedReason.ALREADY_MOUNTED,
    'Quem já está montado não monta de novo nem serve de montaria. Desmonte antes. Nada foi mudado.',
  ],
  [
    EncounterBlockedReason.MOUNT_TAKEN,
    'Essa montaria está carregando alguém. O cavaleiro desmonta antes. Nada foi mudado.',
  ],
  [
    EncounterBlockedReason.NOT_A_MOUNT,
    'Essa criatura não aceita ser montada. Peça ao mestre para marcá-la como montaria. Nada foi mudado.',
  ],
  [
    EncounterBlockedReason.MOUNT_TOO_SMALL,
    'A montaria precisa ser maior que quem monta. Nada foi mudado.',
  ],
  [
    EncounterBlockedReason.MOUNT_TOO_FAR,
    'A montaria está longe: monte até 1,5 m (um quadrado, na diagonal também). Nada foi mudado.',
  ],
  [
    EncounterBlockedReason.MOUNT_MOVE_USED,
    'Só dá para montar ou desmontar uma vez por movimento. Nada foi mudado.',
  ],
  [
    EncounterBlockedReason.MOUNTED,
    'Quem está montado anda com a montaria. Desmonte antes de andar sozinho. Nada foi mudado.',
  ],
  [
    EncounterBlockedReason.NO_ROOM_TO_DISMOUNT,
    'Não há quadrado livre ao lado da montaria para desmontar ali. Nada foi mudado.',
  ],
  [
    EncounterBlockedReason.MOUNT_ACTION_LIMITED,
    'Uma montaria que leva alguém só pode usar Disparada, Desengajar ou Esquivar. Nada foi mudado.',
  ],
  [EncounterBlockedReason.FEATURE_TARGET, 'O alvo saiu do alcance. Escolha outro alvo.'],
  [EncounterBlockedReason.FEATURE_CHOICE, 'Essa opção não vale mais. Escolha outra.'],
]);

/** The same refusal naming who changed (`labelOf` gives the name of a combatant on screen). */
export function undoChangedText(
  blocked: EncounterBlocked,
  labelOf: (id: string) => string | undefined,
): string {
  const names = blocked.combatantIds.map(labelOf).filter((n): n is string => !!n);
  return names.length === 0
    ? (LIMITED_ACTION_BLOCKED_TEXT.get(blocked.reason) ?? '')
    : `Não dá para desfazer: os pontos de vida de ${names.join(', ')} mudaram depois dessa ação, e desfazer apagaria essa mudança. Nada foi desfeito.`;
}
