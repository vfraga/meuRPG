import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  EncounterBlockedReason,
  EncounterBlockedSchema,
} from '../../../gen/meurpg/play/v1/combat_pb';
import {
  STEED_RELEASE_LINE,
  steedAskText,
  steedAwayLine,
  steedRefusal,
  steedSubtitle,
} from './steed-card';

describe("the bonded steed's card, in words (#588, boards 588-23 to 588-26)", () => {
  const blocked = (reason: EncounterBlockedReason) =>
    new ConnectError('no', Code.FailedPrecondition, undefined, [
      { desc: EncounterBlockedSchema, value: create(EncounterBlockedSchema, { reason }) },
    ]);

  it("writes the subtitle with the type's word and no origin: the tag says whose it is", () => {
    expect(steedSubtitle('Cavalo de guerra', 'Grande', 'celestial')).toBe(
      'Cavalo de guerra\u00a0· Grande\u00a0· celestial',
    );
    expect(steedSubtitle('Cavalo de guerra', '', 'fey')).toBe('Cavalo de guerra\u00a0· fada');
    expect(steedSubtitle('Cavalo de guerra', 'Grande', '')).toBe('Cavalo de guerra\u00a0· Grande');
  });

  it('says what a dismissal does, to the owner and to the master: the bond stays', () => {
    expect(steedAskText('dismiss', 'Estrela', '')).toBe(
      'Estrela some do mapa e da sua ficha, e você desce de pé se estiver montado. O vínculo fica: Convocar Montaria traz Estrela de volta.',
    );
    expect(steedAskText('dismiss', 'Estrela', 'Lia')).toBe(
      'Estrela some do mapa e da ficha de Lia, e Lia desce de pé se estiver montado. O vínculo fica: Convocar Montaria traz Estrela de volta.',
    );
  });

  it('says what a release does: the bond ends, and the next cast picks again', () => {
    expect(steedAskText('release', 'Estrela', '')).toBe(
      'O vínculo acaba: Estrela some do mapa e da sua ficha, e a próxima Convocar Montaria escolhe de novo a forma e o tipo.',
    );
    expect(steedAskText('release', 'Estrela', 'Lia')).toBe(
      'O vínculo acaba: Estrela some do mapa e da ficha de Lia, e a próxima Convocar Montaria escolhe de novo a forma e o tipo.',
    );
    expect(STEED_RELEASE_LINE).toBe('Acaba o vínculo: a próxima é outra montaria.');
  });

  it("names the two refusals of a combat by the owner's turn and action; any other error is not one of them", () => {
    expect(steedRefusal(blocked(EncounterBlockedReason.NOT_YOUR_TURN), 'Lia')).toBe(
      'Só na vez de Lia: dispensar gasta a ação.',
    );
    expect(steedRefusal(blocked(EncounterBlockedReason.ACTION_USED), 'Lia')).toBe(
      'Você já usou a ação desta vez.',
    );
    expect(steedRefusal(blocked(EncounterBlockedReason.NO_SLOT), 'Lia')).toBe('');
    expect(steedRefusal(new ConnectError('down', Code.Unavailable), 'Lia')).toBe('');
  });

  it('says how a steed that is away comes back', () => {
    expect(steedAwayLine('Estrela')).toBe('Convocar Montaria traz Estrela de volta.');
  });
});
