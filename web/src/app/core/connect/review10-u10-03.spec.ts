import { Code, ConnectError } from '@connectrpc/connect';

import { combatErrorMessage } from '../combat/combat-errors';
import { describeConnectError } from './connect-errors';

const GENERIC = 'Não foi possível falar com o servidor agora. Tente de novo em instantes.';

// Finding U10-3, see review/unit-10-web-core-stream.md
describe('Review10 U10-3: an ended login session is described as a server problem', () => {
  const ended = () => new ConnectError('no session', Code.Unauthenticated);

  it('does not describe an ended session with the Unavailable wording or the generic retry', () => {
    const msg = describeConnectError(ended(), { [Code.Unavailable]: 'Servidor indisponível.' });
    expect(msg).not.toBe('Servidor indisponível.');
    expect(msg).not.toBe(GENERIC);
    expect(msg).toMatch(/entr/i);
  });

  it('does not describe an ended session with the generic retry when no wording is given', () => {
    const msg = describeConnectError(ended(), {});
    expect(msg).not.toBe(GENERIC);
    expect(msg).toMatch(/entr/i);
  });

  it('combat (a real caller) does not tell a signed-out person the server did not answer', () => {
    const msg = combatErrorMessage(ended(), 'atacar');
    expect(msg).not.toBe('Não deu para atacar: o servidor não respondeu. Tente de novo.');
    expect(msg).toMatch(/entr/i);
  });
});
