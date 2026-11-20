import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  EncounterBlockedReason,
  EncounterBlockedSchema,
  EncounterSchema,
  EncounterStatus,
} from '../../../gen/meurpg/play/v1/combat_pb';
import {
  ContestBlockedReason,
  ContestBlockedSchema,
} from '../../../gen/meurpg/play/v1/contest_types_pb';
import { contestBlockedMessage } from '../combat/combat-errors';
import { CreatureSource } from '../../../gen/meurpg/characters/v1/characters_pb';
import { sourcePhrase } from './creature-format';
import {
  STEED_RELEASE_LINE,
  steedActionsHint,
  steedAskError,
  steedAskText,
  steedAwaySubtitle,
  steedCastHint,
  steedWhere,
  steedRefusal,
  steedSubtitle,
} from './steed-card';

describe("the bonded steed's card, in words", () => {
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

  const out = { combat: false, spends: false, mounted: false };
  const riding = { combat: true, spends: true, mounted: true };

  it('says what a dismissal does to the rider in a combat: the bond stays', () => {
    expect(steedAskText('dismiss', 'Estrela', '', riding)).toBe(
      'Estrela some do mapa e da sua ficha, e você desce de pé no quadrado onde Estrela estava. O vínculo fica: Convocar Montaria traz Estrela de volta.',
    );
    expect(steedAskText('dismiss', 'Trovão', 'Lia', riding)).toBe(
      'Trovão some do mapa e da ficha de Lia, e Lia desce de pé no quadrado onde Trovão estava. O vínculo fica: Convocar Montaria traz Trovão de volta.',
    );
  });

  it('with nobody on the steed says nothing of a rider, and out of a combat tells the owner it spends nothing', () => {
    expect(
      steedAskText('dismiss', 'Estrela', '', { combat: true, spends: true, mounted: false }),
    ).toBe(
      'Estrela some do mapa e da sua ficha. O vínculo fica: Convocar Montaria traz Estrela de volta.',
    );
    expect(steedAskText('dismiss', 'Estrela', '', out)).toBe(
      'Estrela some do mapa e da sua ficha. O vínculo fica: Convocar Montaria traz Estrela de volta. Fora de combate, não gasta nada.',
    );
    // The master never spends: the sentence about the cost is the owner's.
    expect(steedAskText('dismiss', 'Estrela', 'Lia', out)).toBe(
      'Estrela some do mapa e da ficha de Lia. O vínculo fica: Convocar Montaria traz Estrela de volta.',
    );
  });

  it('says what a release does: the bond ends, and the next cast picks again', () => {
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
      status: EncounterStatus.ACTIVE,
      combatants: [
        { id: 'c-1', characterId: 'char-1', mountedOnId: 'c-9' },
        { id: 'c-9', creatureId: 'steed-1', riderId: 'c-1' },
      ],
    });
    expect(steedWhere(enc, 'steed-1', 'char-1')).toEqual(riding);
    expect(steedWhere(enc, 'steed-1', 'char-2')).toEqual({
      combat: true,
      spends: true,
      mounted: false,
    });
    expect(steedWhere(enc, 'steed-2', 'char-1')).toEqual(out);
    expect(steedWhere(null, 'steed-1', 'char-1')).toEqual(out);
    // A combat still being set up charges nothing (`SpendDismissal`): no chip, and it is not "fora de combate" either.
    const setup = create(EncounterSchema, { ...enc, status: EncounterStatus.SETUP });
    expect(steedWhere(setup, 'steed-1', 'char-1')).toEqual({
      combat: true,
      spends: false,
      mounted: true,
    });
  });

  it("names the two refusals of a combat by the owner's turn and action; any other error is not one of them", () => {
    expect(steedRefusal(blocked(EncounterBlockedReason.NOT_YOUR_TURN), 'Lia')).toBe(
      'Só na vez de Lia: dispensar gasta a ação.',
    );
    expect(steedRefusal(blocked(EncounterBlockedReason.ACTION_USED), 'Lia')).toBe(
      'Você já usou a ação desta vez.',
    );
    // The rest of the server's gate, in the combat's own words.
    expect(steedRefusal(blocked(EncounterBlockedReason.COMBATANT_DOWN), 'Lia')).toBe(
      'Com 0 PV, não age.',
    );
    expect(steedRefusal(blocked(EncounterBlockedReason.CANNOT_ACT), 'Lia')).toBe(
      'Não dá para agir agora.',
    );
    const surprised = new ConnectError('no', Code.FailedPrecondition, undefined, [
      {
        desc: ContestBlockedSchema,
        value: create(ContestBlockedSchema, { reason: ContestBlockedReason.SURPRISED }),
      },
    ]);
    expect(steedRefusal(surprised, 'Lia')).toBe(
      contestBlockedMessage(surprised.findDetails(ContestBlockedSchema)[0]),
    );
    expect(steedRefusal(surprised, 'Lia')).not.toBe('');
    expect(steedRefusal(blocked(EncounterBlockedReason.NO_SLOT), 'Lia')).toBe('');
    expect(steedRefusal(new ConnectError('down', Code.Unavailable), 'Lia')).toBe('');
  });

  it('writes the row of a steed that is away, and what the cast button says under it', () => {
    expect(steedAwaySubtitle('Cavalo de guerra', 'celestial')).toBe(
      'Cavalo de guerra\u00a0· celestial',
    );
    expect(steedCastHint('Estrela', true)).toBe(
      'Traz Estrela de volta, com a mesma forma, o mesmo tipo e os PV cheios.',
    );
    expect(steedCastHint('Trovão', true)).toBe(
      'Traz Trovão de volta, com a mesma forma, o mesmo tipo e os PV cheios.',
    );
    expect(steedCastHint('Estrela', false)).toBe('Com Estrela aqui, os PV voltam cheios.');
    expect(steedCastHint('Trovão', false)).toBe('Com Trovão aqui, os PV voltam cheios.');
  });

  it('any other refusal of the combat stays in the question without sending the person to close a sheet: the card is no sheet', () => {
    expect(steedAskError(blocked(EncounterBlockedReason.NO_SLOT))).toBe(
      'Não dá para fazer isso agora. Tente de novo.',
    );
    expect(steedAskError(new ConnectError('down', Code.Unavailable))).not.toContain('Feche');
  });

  it('says under the actions what "Dispensar" keeps, and to the owner that it is free out of a combat', () => {
    expect(steedActionsHint('Estrela', false)).toBe(
      'Dispensar mantém o vínculo: Convocar Montaria traz Estrela de volta. Fora de combate, não gasta nada.',
    );
    expect(steedActionsHint('Estrela', true)).toBe(
      'Dispensar mantém o vínculo: Convocar Montaria traz Estrela de volta.',
    );
  });

  it('RN-10: to another player the steed is "Criatura de Lia", with no word of its bond or its type', () => {
    expect(sourcePhrase(CreatureSource.FIND_STEED, 'Lia')).toBe('Criatura de Lia');
  });
});
