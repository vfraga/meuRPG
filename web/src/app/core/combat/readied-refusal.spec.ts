import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import { ReadiedActionSchema, ReadiedKind } from '../../../gen/meurpg/play/v1/combat_pb';
import { readiedRefusalText } from './readied-refusal';

const held = (kind: ReadiedKind) => create(ReadiedActionSchema, { id: 'r', kind });

describe('the refusal of the answer of a readied action (#582)', () => {
  it('says what a refused spell most likely needs: the server takes only creatures and the d20 from the answer', () => {
    const refused = new ConnectError(
      'every target of Magic Missile needs at least one dart',
      Code.InvalidArgument,
    );
    expect(readiedRefusalText(refused, held(ReadiedKind.SPELL), 'responder à reação')).toBe(
      'Não deu para soltar a magia por aqui: ela pede dardos ou um ponto no mapa, que este aviso ainda não envia. Dá para deixar passar.',
    );
  });

  it('leaves any other refusal, and any other action, to the usual words', () => {
    const usual = (err: unknown, kind: ReadiedKind) =>
      readiedRefusalText(err, held(kind), 'responder à reação');
    expect(usual(new ConnectError('x', Code.InvalidArgument), ReadiedKind.ATTACK)).not.toContain(
      'dardos',
    );
    expect(usual(new ConnectError('x', Code.FailedPrecondition), ReadiedKind.SPELL)).not.toContain(
      'dardos',
    );
  });
});
