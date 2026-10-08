import { Code, ConnectError } from '@connectrpc/connect';

import { saveErrorMessage } from './document-copy';

describe('the copy of a failed document save', () => {
  it('sends a person whose session ended to sign in in another tab, because signing in here reloads the page', () => {
    const text = saveErrorMessage(new ConnectError('x', Code.Unauthenticated));
    expect(text).toContain('em outra aba');
    expect(text).toContain('perde o texto');
  });

  it('keeps the text on a lost connection, where nothing reloads', () => {
    expect(saveErrorMessage(new ConnectError('x', Code.Unavailable))).toContain(
      'O texto continua aqui',
    );
  });
});
