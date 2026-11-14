import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  EncounterBlockedReason,
  EncounterBlockedSchema,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { combatErrorMessage } from './combat-errors';

const blocked = (reason: EncounterBlockedReason) =>
  new ConnectError('x', Code.FailedPrecondition, undefined, [
    { desc: EncounterBlockedSchema, value: create(EncounterBlockedSchema, { reason }) },
  ]);

describe('the refusals of the creatures and Wild Shape, by reason (MR-037)', () => {
  it.each([
    [
      EncounterBlockedReason.WILD_SHAPE_NO_SPELLS,
      'Na forma de fera não dá para conjurar. Volte à forma normal e tente de novo.',
    ],
    [
      EncounterBlockedReason.ARMOR_NOT_PROFICIENT,
      'Com armadura ou escudo sem proficiência não dá para conjurar.',
    ],
    [EncounterBlockedReason.ALREADY_IN_WILD_SHAPE, 'Você já está na forma de uma fera.'],
    [
      EncounterBlockedReason.WILD_SHAPE_ENDED,
      'A forma já acabou. Para virar fera de novo, use Forma Selvagem.',
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
    [
      EncounterBlockedReason.TOO_MANY_COMBATANTS,
      'Não cabem mais combatentes neste combate. Nada foi gasto.',
    ],
    [
      EncounterBlockedReason.SUMMON_NEEDS_INITIATIVE,
      'Falta o d20 da iniciativa das criaturas. Se cada uma rola a sua, role no app.',
    ],
    [
      EncounterBlockedReason.CASTING_TIME_TOO_LONG,
      'Essa magia leva mais tempo do que um combate dá. Conjure fora do combate.',
    ],
  ])('%s', (reason, text) => {
    expect(combatErrorMessage(blocked(reason))).toBe(text);
  });
});
