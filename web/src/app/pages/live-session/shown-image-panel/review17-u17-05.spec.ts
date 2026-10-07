import { Code, ConnectError } from '@connectrpc/connect';

import { showErrorMessage } from './shown-image-panel';

// Finding U17-5 (review/unit-17-contract.md)
describe('Review17 U17-5: a full gallery on "show image" is reported as the server being unreachable', () => {
  it('does not tell the master the server could not be reached when the gallery is full', () => {
    const err = new ConnectError(
      "the campaign's gallery is full: a fog map's image needs a copy of its own",
      Code.ResourceExhausted,
    );
    const message = showErrorMessage(err);
    expect(message).not.toContain('falar com o servidor');
  });
});
