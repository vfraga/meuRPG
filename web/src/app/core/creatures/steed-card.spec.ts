import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  EncounterBlockedReason,
  EncounterBlockedSchema,
  EncounterSchema,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { CreatureSource } from '../../../gen/meurpg/characters/v1/characters_pb';
import { sourcePhrase } from './creature-format';
import {
  STEED_RELEASE_LINE,
  steedAskText,
  steedAwaySubtitle,
  steedCastHint,
  steedWhere,
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

  const out = { combat: false, mounted: false };
  const riding = { combat: true, mounted: true };

  it("says what a dismissal does to the rider in a combat, in board 588-24's words: the bond stays", () => {
    expect(steedAskText('dismiss', 'Estrela', '', riding)).toBe(
      'Estrela some do mapa e da sua ficha, e você desce de pé no quadrado dela. O vínculo fica: Convocar Montaria a traz de volta.',
    );
    expect(steedAskText('dismiss', 'Trovão', 'Lia', riding)).toBe(
      'Trovão some do mapa e da ficha de Lia, e Lia desce de pé no quadrado dele. O vínculo fica: Convocar Montaria o traz de volta.',
    );
  });

  it('with nobody on the steed says nothing of a rider, and out of a combat tells the owner it spends nothing', () => {
    expect(steedAskText('dismiss', 'Estrela', '', { combat: true, mounted: false })).toBe(
      'Estrela some do mapa e da sua ficha. O vínculo fica: Convocar Montaria a traz de volta.',
    );
    expect(steedAskText('dismiss', 'Estrela', '', out)).toBe(
      'Estrela some do mapa e da sua ficha. O vínculo fica: Convocar Montaria a traz de volta. Fora de combate, não gasta nada.',
    );
    // The master never spends: the sentence about the cost is the owner's.
    expect(steedAskText('dismiss', 'Estrela', 'Lia', out)).toBe(
      'Estrela some do mapa e da ficha de Lia. O vínculo fica: Convocar Montaria a traz de volta.',
    );
  });

  it("says what a release does, in board 588-25's words: the bond ends, and the next cast picks again", () => {
    expect(steedAskText('release', 'Estrela', '', out)).toBe(
      'O vínculo acaba: Estrela some da ficha e do mapa, e a próxima Convocar Montaria escolhe de novo a forma e o tipo. Fora de combate, não gasta nada.',
    );
    expect(steedAskText('release', 'Estrela', '', riding)).toBe(
      'O vínculo acaba: Estrela some da ficha e do mapa, e a próxima Convocar Montaria escolhe de novo a forma e o tipo.',
    );
    expect(steedAskText('release', 'Estrela', 'Lia', out)).toBe(
      'O vínculo acaba: Estrela some da ficha de Lia e do mapa, e a próxima Convocar Montaria escolhe de novo a forma e o tipo.',
    );
    expect(STEED_RELEASE_LINE).toBe('Acaba o vínculo: a próxima é outra montaria.');
  });

  it('reads from the combat whether it holds the steed and whether its owner rides it', () => {
    const enc = create(EncounterSchema, {
      combatants: [
        { id: 'c-1', characterId: 'char-1', mountedOnId: 'c-9' },
        { id: 'c-9', creatureId: 'steed-1', riderId: 'c-1' },
      ],
    });
    expect(steedWhere(enc, 'steed-1', 'char-1')).toEqual({ combat: true, mounted: true });
    expect(steedWhere(enc, 'steed-1', 'char-2')).toEqual({ combat: true, mounted: false });
    expect(steedWhere(enc, 'steed-2', 'char-1')).toEqual({ combat: false, mounted: false });
    expect(steedWhere(null, 'steed-1', 'char-1')).toEqual({ combat: false, mounted: false });
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

  it('writes the row of a steed that is away, and what the cast button says under it (board 588-26 card 1, 588-23)', () => {
    expect(steedAwaySubtitle('Cavalo de guerra', 'celestial')).toBe(
      'Cavalo de guerra\u00a0· celestial\u00a0· fora da mesa',
    );
    expect(steedCastHint('Estrela', true)).toBe(
      'Traz Estrela de volta, com a forma e o tipo dela e os PV cheios.',
    );
    expect(steedCastHint('Trovão', true)).toBe(
      'Traz Trovão de volta, com a forma e o tipo dele e os PV cheios.',
    );
    expect(steedCastHint('Estrela', false)).toBe('Com Estrela aqui, ela volta aos PV cheios.');
    expect(steedCastHint('Trovão', false)).toBe('Com Trovão aqui, ele volta aos PV cheios.');
  });

  it('RN-10: to another player the steed is "Criatura de Lia", with no word of its bond or its type', () => {
    expect(sourcePhrase(CreatureSource.FIND_STEED, 'Lia')).toBe('Criatura de Lia');
  });
});
