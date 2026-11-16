import { Code, ConnectError } from '@connectrpc/connect';
import { create } from '@bufbuild/protobuf';

import { FavorBlockedReason, FavorBlockedSchema } from '../../../gen/meurpg/play/v1/favor_pb';
import { favorBlocked, favorBlockedMessage } from './favor-errors';

function blocked(reason: FavorBlockedReason): ConnectError {
  return new ConnectError('no', Code.FailedPrecondition, undefined, [
    { desc: FavorBlockedSchema, value: create(FavorBlockedSchema, { reason }) },
  ]);
}

describe('favor errors', () => {
  const reasons = [
    FavorBlockedReason.ALREADY_HAS,
    FavorBlockedReason.NOT_A_PLAYER_CHARACTER,
    FavorBlockedReason.NOT_THE_HOLDER,
    FavorBlockedReason.HAS_NONE,
    FavorBlockedReason.CANNOT_RECEIVE,
    FavorBlockedReason.SELF,
    FavorBlockedReason.ADDS_NOTHING,
    FavorBlockedReason.AFTER_THE_ROLL,
    FavorBlockedReason.NOTHING_TO_ROLL,
    FavorBlockedReason.PASS_PENDING,
    FavorBlockedReason.PASS_CLOSED,
    FavorBlockedReason.NO_SESSION,
  ];

  it('has its own Portuguese sentence for every refusal', () => {
    const sentences = reasons.map((r) => favorBlockedMessage(r));
    expect(new Set(sentences).size).toBe(reasons.length);
    for (const s of sentences) {
      expect(s).toMatch(/[a-zà-ú]/);
      expect(s).not.toMatch(/PASS_|NO_SESSION|CANNOT/);
    }
  });

  it('says what the pass refusals mean', () => {
    expect(favorBlockedMessage(FavorBlockedReason.PASS_PENDING)).toBe(
      'Você pediu para passar a Inspiração. Cancele o pedido para usar.',
    );
    expect(favorBlockedMessage(FavorBlockedReason.PASS_CLOSED)).toBe(
      'Esse pedido já foi respondido em outra tela.',
    );
    expect(favorBlockedMessage(FavorBlockedReason.NO_SESSION)).toBe('Só numa sessão aberta.');
    expect(favorBlockedMessage(FavorBlockedReason.CANNOT_RECEIVE)).toBe(
      'Essa personagem não pode ficar com a Inspiração agora.',
    );
  });

  it('reads the typed detail, never the message', () => {
    expect(favorBlocked(blocked(FavorBlockedReason.PASS_PENDING))?.reason).toBe(
      FavorBlockedReason.PASS_PENDING,
    );
    expect(favorBlocked(new ConnectError('PASS_PENDING', Code.FailedPrecondition))).toBeNull();
    expect(favorBlocked(new ConnectError('x', Code.NotFound))).toBeNull();
  });
});
