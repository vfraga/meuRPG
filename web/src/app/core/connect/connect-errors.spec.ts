import { Code, ConnectError } from '@connectrpc/connect';

import { combatErrorMessage } from '../combat/combat-errors';
import {
  OUTCOME_UNKNOWN,
  SESSION_ENDED,
  describeConnectError,
  isRateLimited,
  rateLimitedMessage,
} from './connect-errors';

describe('describeConnectError', () => {
  it('uses the message given for the error code', () => {
    const err = new ConnectError('nope', Code.NotFound);
    expect(describeConnectError(err, { [Code.NotFound]: 'Campanha não encontrada.' })).toBe(
      'Campanha não encontrada.',
    );
  });

  it('does not borrow the unavailable wording for another code with no wording of its own', () => {
    const err = new ConnectError('nope', Code.Internal);
    expect(
      describeConnectError(err, {
        [Code.Unavailable]: 'As imagens estão desligadas.',
      }),
    ).toBe('Não foi possível falar com o servidor agora. Tente de novo em instantes.');
  });

  it('uses the unavailable message for unavailable', () => {
    const err = new ConnectError('down', Code.Unavailable);
    expect(describeConnectError(err, { [Code.Unavailable]: 'Servidor indisponível.' })).toBe(
      'Servidor indisponível.',
    );
  });

  it('tells the person to check an ambiguous outcome (unknown) before trying again', () => {
    const err = new ConnectError('may or may not have been saved', Code.Unknown);
    expect(describeConnectError(err, { [Code.Unavailable]: 'Servidor indisponível.' })).toBe(
      OUTCOME_UNKNOWN,
    );
    expect(describeConnectError(err, { [Code.Unknown]: 'Do jeito da tela.' })).toBe(
      'Do jeito da tela.',
    );
  });

  it('falls back to a generic message when nothing else applies', () => {
    const err = new ConnectError('nope', Code.Internal);
    expect(describeConnectError(err, {})).toBe(
      'Não foi possível falar com o servidor agora. Tente de novo em instantes.',
    );
  });

  it('treats a plain network failure the same as "unavailable" (never as some other code)', () => {
    const err = new TypeError('Failed to fetch');
    expect(
      describeConnectError(err, {
        [Code.Unavailable]: 'Servidor indisponível.',
        [Code.NotFound]: 'Não encontrado.',
      }),
    ).toBe('Servidor indisponível.');
  });

  it('leaves an already-specific ConnectError as is (no re-wrapping)', () => {
    const err = new ConnectError('sessão inválida', Code.PermissionDenied);
    expect(
      describeConnectError(err, { [Code.PermissionDenied]: 'Só o mestre pode fazer isso.' }),
    ).toBe('Só o mestre pode fazer isso.');
  });
});

describe('a rate-limited call', () => {
  const limited = (code: Code, retryAfter = '3') =>
    new ConnectError('slow down', code, new Headers({ 'Retry-After': retryAfter }));

  it('is told apart from the other resource_exhausted answers by Retry-After', () => {
    expect(isRateLimited(limited(Code.ResourceExhausted))).toBe(true);
    expect(isRateLimited(limited(Code.Unavailable))).toBe(true);
    expect(isRateLimited(new ConnectError('gallery full', Code.ResourceExhausted))).toBe(false);
    expect(isRateLimited(new TypeError('Failed to fetch'))).toBe(false);
  });

  it('says how long to wait, and never uses the screen wording of the code', () => {
    expect(rateLimitedMessage(limited(Code.ResourceExhausted, '3'))).toBe(
      'Muitas ações em pouco tempo. Espere 3 segundos e tente de novo.',
    );
    expect(rateLimitedMessage(limited(Code.ResourceExhausted, '1'))).toContain(
      'Espere 1 segundo e',
    );
    expect(rateLimitedMessage(limited(Code.ResourceExhausted, 'soon'))).toContain(
      'Espere alguns segundos',
    );
    expect(
      describeConnectError(limited(Code.ResourceExhausted), {
        [Code.ResourceExhausted]: 'A galeria está cheia.',
      }),
    ).toBe('Muitas ações em pouco tempo. Espere 3 segundos e tente de novo.');
  });

  describe('an ended login session', () => {
    const ended = () => new ConnectError('no session', Code.Unauthenticated);
    const GENERIC = 'Não foi possível falar com o servidor agora. Tente de novo em instantes.';

    it('says to sign in again, not that the server is unavailable', () => {
      const msg = describeConnectError(ended(), { [Code.Unavailable]: 'Servidor indisponível.' });
      expect(msg).toBe(SESSION_ENDED);
      expect(msg).not.toBe(GENERIC);
    });

    it('says so even when the screen gave no wording at all', () => {
      expect(describeConnectError(ended(), {})).toBe(SESSION_ENDED);
    });

    it("keeps a screen's own wording for it", () => {
      expect(describeConnectError(ended(), { [Code.Unauthenticated]: 'Entre para ver.' })).toBe(
        'Entre para ver.',
      );
    });

    it('is the wording a caller without an entry shows, combat included', () => {
      expect(combatErrorMessage(ended(), 'atacar')).toBe(SESSION_ENDED);
    });
  });
});
