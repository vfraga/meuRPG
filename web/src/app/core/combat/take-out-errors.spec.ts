import { Code, ConnectError } from '@connectrpc/connect';
import { create } from '@bufbuild/protobuf';

import {
  EncounterBlockedReason,
  EncounterBlockedSchema,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { takeOutErrorMessage } from './take-out-errors';

function blocked(reason: EncounterBlockedReason, combatantIds: string[] = []): ConnectError {
  return new ConnectError('x', Code.FailedPrecondition, undefined, [
    {
      desc: EncounterBlockedSchema,
      value: create(EncounterBlockedSchema, { reason, combatantIds }),
    },
  ]);
}

describe('takeOutErrorMessage', () => {
  it('words the refusals of the undo behind a take-out, and says nothing changed', () => {
    expect(takeOutErrorMessage(blocked(EncounterBlockedReason.NOTHING_TO_UNDO))).toBe(
      'Já aconteceu outra coisa depois deste dano; use Desfazer ou corrija os PV. Nada mudou.',
    );
    expect(
      takeOutErrorMessage(blocked(EncounterBlockedReason.UNDO_TARGET_CHANGED, ['z']), (id) =>
        id === 'z' ? 'Zumbi 1' : undefined,
      ),
    ).toBe(
      'Não dá para desfazer: os pontos de vida de Zumbi 1 mudaram depois dessa ação, e desfazer apagaria essa mudança. Nada foi desfeito. Corrija os PV à mão.',
    );
    expect(takeOutErrorMessage(blocked(EncounterBlockedReason.DAMAGE_RESOLVED))).toContain(
      'Nada mudou.',
    );
  });

  it('says the card is as it was when the server did not answer', () => {
    expect(takeOutErrorMessage(new ConnectError('off', Code.Unavailable))).toBe(
      'Não deu para tirar a parte agora. Nada mudou. Tente de novo.',
    );
  });
});
