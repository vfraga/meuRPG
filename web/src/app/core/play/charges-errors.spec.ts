import { Code, ConnectError } from '@connectrpc/connect';
import { create } from '@bufbuild/protobuf';

import { ChargesBlockedReason, ChargesBlockedSchema } from '../../../gen/meurpg/play/v1/charges_pb';
import {
  CharacterBlockedReason,
  CharacterBlockedSchema,
  ItemRechargeKind,
  ItemRechargeSchema,
} from '../../../gen/meurpg/characters/v1/characters_pb';
import {
  EncounterBlockedReason,
  EncounterBlockedSchema,
} from '../../../gen/meurpg/play/v1/combat_pb';
import {
  GameSessionBlockedReason,
  GameSessionBlockedSchema,
} from '../../../gen/meurpg/play/v1/play_pb';
import {
  chargesBlocked,
  chargesBlockedMessage,
  chargesErrorMessage,
  consumableNeedsReload,
  consumableRefusal,
  consumableTargetsRefusal,
  turnBlockMessage,
  turnBlockParts,
  unreadSheetRefusal,
} from './charges-errors';

describe('charges errors', () => {
  it('reads the typed detail of a failed precondition and nothing else', () => {
    const detail = create(ChargesBlockedSchema, {
      reason: ChargesBlockedReason.REGAIN_FACE_NEEDED,
      needed: 6,
    });
    const err = new ConnectError('x', Code.FailedPrecondition, undefined, [
      { desc: ChargesBlockedSchema, value: detail },
    ]);
    expect(chargesBlocked(err)?.needed).toBe(6);
    expect(chargesBlocked(new ConnectError('x', Code.NotFound))).toBeNull();
  });

  it('says what each refusal means', () => {
    const say = (reason: ChargesBlockedReason) =>
      chargesBlockedMessage(create(ChargesBlockedSchema, { reason, needed: 4 }));
    expect(say(ChargesBlockedReason.REGAIN_FACE_NEEDED)).toContain('d4');
    expect(say(ChargesBlockedReason.COMBAT_RUNNING)).toContain('combate');
    expect(say(ChargesBlockedReason.ALREADY_RECOVERED)).toContain('recolhida');
    expect(say(ChargesBlockedReason.UNSPECIFIED)).toContain('agora');
  });
});

describe('chargesErrorMessage', () => {
  const blocked = (reason: CharacterBlockedReason) =>
    new ConnectError('x', Code.FailedPrecondition, undefined, [
      { desc: CharacterBlockedSchema, value: create(CharacterBlockedSchema, { reason }) },
    ]);
  const item = { itemName: 'Varinha de mísseis mágicos', left: 1 };

  it('says the charges of the item are short, with its name and what is left', () => {
    expect(chargesErrorMessage(blocked(CharacterBlockedReason.CHARGES_NOT_ENOUGH), item)).toBe(
      'Cargas a menos. A Varinha de mísseis mágicos só tem 1 carga.',
    );
    expect(
      chargesErrorMessage(blocked(CharacterBlockedReason.CHARGES_NOT_ENOUGH), { ...item, left: 3 }),
    ).toBe('Cargas a menos. A Varinha de mísseis mágicos só tem 3 cargas.');
  });

  it('says an item that comes back at dawn does so, as the board has it', () => {
    expect(
      chargesErrorMessage(blocked(CharacterBlockedReason.ITEM_NO_CHARGES), {
        itemName: 'Varinha de segredos',
        left: 0,
        recharge: create(ItemRechargeSchema, { kind: ItemRechargeKind.DAWN, regainDice: '1d3' }),
      }),
    ).toBe('Sem cargas. A Varinha de segredos volta ao amanhecer.');
  });

  it('says an item with no charges and a destroyed item by name', () => {
    expect(chargesErrorMessage(blocked(CharacterBlockedReason.ITEM_NO_CHARGES), item)).toBe(
      'Sem cargas. A Varinha de mísseis mágicos não tem cargas agora.',
    );
    expect(chargesErrorMessage(blocked(CharacterBlockedReason.ITEM_DESTROYED), item)).toBe(
      'Item destruído. A Varinha de mísseis mágicos não tem mais uso.',
    );
  });

  it('words a refusal of the charges service by its typed detail', () => {
    const err = new ConnectError('x', Code.FailedPrecondition, undefined, [
      {
        desc: ChargesBlockedSchema,
        value: create(ChargesBlockedSchema, { reason: ChargesBlockedReason.COMBAT_RUNNING }),
      },
    ]);
    expect(chargesErrorMessage(err, item)).toContain('combate');
  });

  it('says nothing changed when the call did not get through', () => {
    expect(chargesErrorMessage(new ConnectError('x', Code.Unavailable), item)).toBe(
      'Não foi possível gastar a carga. Nada mudou. Tente de novo.',
    );
  });

  it('words a failure to save a correction or a dawn, not a spend, when the caller says what it was doing', () => {
    const down = new ConnectError('x', Code.Unavailable);
    expect(chargesErrorMessage(down, item, 'salvar')).toBe(
      'Não foi possível salvar. Nada mudou. Tente de novo.',
    );
    expect(chargesErrorMessage(down, item, 'amanhecer')).toBe(
      'Não foi possível amanhecer. Nada mudou. Tente de novo.',
    );
  });

  it('falls back to the character sentences for the other refusals', () => {
    expect(chargesErrorMessage(blocked(CharacterBlockedReason.CHARACTER_DEAD), item)).toContain(
      'morto',
    );
  });
});

describe('consumableRefusal', () => {
  const encounter = (reason: EncounterBlockedReason) =>
    new ConnectError('x', Code.FailedPrecondition, undefined, [
      { desc: EncounterBlockedSchema, value: create(EncounterBlockedSchema, { reason }) },
    ]);
  const dead = (characterId: string) =>
    new ConnectError('x', Code.FailedPrecondition, undefined, [
      {
        desc: CharacterBlockedSchema,
        value: create(CharacterBlockedSchema, {
          reason: CharacterBlockedReason.CHARACTER_DEAD,
          characterId,
        }),
      },
    ]);
  const potion = { kind: 'potion' as const, userName: 'Tor', targetName: 'Brisa', targetId: 'b1' };

  it('leads with "Longe demais." and says the receiver is no longer at the giver\'s side, by name', () => {
    const away = encounter(EncounterBlockedReason.TARGET_NOT_ADJACENT);
    const r = consumableRefusal(away, potion);
    expect(r).toMatchObject({
      lead: 'Longe demais.',
      text: 'Brisa não está mais ao seu lado. Nada foi gasto.',
      cause: 'target-away',
      look: 'danger',
    });
    expect(consumableRefusal(away, { ...potion, actingFor: 'Tor' }).text).toBe(
      'Brisa não está mais ao lado de Tor. Nada foi gasto.',
    );
    expect(consumableRefusal(away, { ...potion, targetName: undefined }).text).toBe(
      'Quem ia receber não está mais ao seu lado. Nada foi gasto.',
    );
  });

  it('leads with "Essa poção já foi usada." for an item that is already used, by the noun of its kind', () => {
    const gone = encounter(EncounterBlockedReason.ITEM_GONE);
    expect(consumableRefusal(gone, potion)).toMatchObject({
      lead: 'Essa poção já foi usada.',
      text: 'A ficha foi atualizada.',
      cause: 'item-gone',
    });
    expect(consumableRefusal(gone, { ...potion, kind: 'scroll' })).toMatchObject({
      lead: 'Esse pergaminho já foi usado.',
      text: 'A ficha foi atualizada.',
    });
    expect(consumableRefusal(gone, { ...potion, kind: 'oil' }).lead).toBe(
      'Esse óleo já foi usado.',
    );
  });

  it('leads with the name of a creature at three failed death saves, still listed as down: "Brisa morreu."', () => {
    const r = consumableRefusal(encounter(EncounterBlockedReason.TARGET_DEAD), potion);
    expect(r).toMatchObject({
      lead: 'Brisa morreu.',
      text: 'Não dá para dar a poção a quem morreu. Nada foi gasto.',
      cause: 'target-dead',
    });
    expect(
      consumableRefusal(encounter(EncounterBlockedReason.TARGET_DEAD), {
        ...potion,
        targetName: undefined,
      }).lead,
    ).toBe('Quem ia receber morreu.');
  });

  it('shows the turn as a ban line, for the player and for the master', () => {
    const turn = encounter(EncounterBlockedReason.NOT_YOUR_TURN);
    const r = consumableRefusal(turn, potion);
    expect(r).toMatchObject({ look: 'ban', block: 'turn' });
    expect(said(r)).toBe('Só na sua vez. Beber ou dar gasta a ação.');
    expect(said(consumableRefusal(turn, { ...potion, actingFor: 'Tor' }))).toBe(
      'Só na vez de Tor. Beber ou dar gasta a ação.',
    );
  });

  it('shows the action of the turn spent as a ban line with its own lead', () => {
    const used = encounter(EncounterBlockedReason.ACTION_USED);
    const r = consumableRefusal(used, potion);
    expect(r).toMatchObject({ look: 'ban', block: 'action', lead: 'Ação já usada.' });
    expect(r.text).toBe('Você já usou a ação desta vez.');
    expect(consumableRefusal(used, { ...potion, actingFor: 'Tor' }).text).toBe(
      'Tor já usou a ação desta vez.',
    );
  });

  it('says who is down does not act but may still receive, by the noun of the item', () => {
    const r = consumableRefusal(encounter(EncounterBlockedReason.COMBATANT_DOWN), potion);
    expect(said(r)).toBe('Quem está caído não age. Mas ainda pode receber a poção.');
  });

  it('leaves the other refusals of a combat to the words the combat has, under a lead', () => {
    const r = consumableRefusal(encounter(EncounterBlockedReason.REACTION_PENDING), potion);
    expect(r.lead).not.toBe('');
    expect(r.text).toBe(
      'Uma reação ainda espera resposta. Espere a resposta ou responda no lugar de quem reage.',
    );
  });

  it('names who is dead: the receiver when the refusal is about them, the user otherwise', () => {
    const forTarget = consumableRefusal(dead('b1'), potion);
    expect(forTarget).toMatchObject({ lead: 'Brisa morreu.', cause: 'target-dead' });
    const forUser = consumableRefusal(dead('t1'), potion);
    expect(forUser).toMatchObject({ lead: 'Tor morreu.', cause: 'other' });
    expect(forUser.text).toBe('Quem morreu não usa nem recebe itens. Nada foi gasto.');
    expect(
      consumableRefusal(dead('t1'), { ...potion, targetName: undefined, targetId: undefined }).lead,
    ).toBe('Tor morreu.');
  });

  it('says there is no open session', () => {
    const closed = new ConnectError('x', Code.FailedPrecondition, undefined, [
      {
        desc: GameSessionBlockedSchema,
        value: create(GameSessionBlockedSchema, {
          reason: GameSessionBlockedReason.NO_OPEN_SESSION,
        }),
      },
    ]);
    expect(said(consumableRefusal(closed, potion))).toBe(
      'Sem sessão aberta. Só dá para usar durante a sessão. Nada foi gasto.',
    );
  });

  it('blames the typed dice only when dice were typed', () => {
    const invalid = new ConnectError('x', Code.InvalidArgument);
    expect(consumableRefusal(invalid, { ...potion, typedDice: true })).toMatchObject({
      lead: 'Dados inválidos.',
      text: 'Cada dado vai de 1 até o número de faces dele. Nada foi gasto.',
    });
    const other = consumableRefusal(invalid, { ...potion, typedDice: false });
    expect(other.text).toBe('Não deu para usar a poção. Nada foi gasto.');
    expect(other.text).not.toContain('dado');
  });

  it('says a receiver who is not there any more, by name, and the user when there is no receiver', () => {
    expect(said(consumableRefusal(new ConnectError('x', Code.NotFound), potion))).toBe(
      'Fora da mesa. Brisa não está mais na mesa. Nada foi gasto.',
    );
    expect(
      consumableRefusal(new ConnectError('x', Code.NotFound), { ...potion, targetName: undefined })
        .text,
    ).toBe('Tor não está mais na mesa. Nada foi gasto.');
  });

  it('says nothing was spent, and offers to try again, when the call never left', () => {
    const down = new ConnectError('x', Code.Unavailable);
    const r = consumableRefusal(down, potion);
    expect(r).toMatchObject({ lead: 'Sem conexão.', retry: true });
    expect(r.text).toBe('Não deu para usar a poção. Nada foi gasto.');
    expect(consumableRefusal(down, { ...potion, kind: 'scroll' }).text).toBe(
      'Não deu para usar o pergaminho. Nada foi gasto.',
    );
  });

  it('does not say nothing was spent when the call may have been carried out', () => {
    for (const code of [Code.Unknown, Code.Internal, Code.DeadlineExceeded]) {
      const r = consumableRefusal(new ConnectError('x', code), potion);
      expect(said(r)).toBe('Não sabemos se a unidade foi usada: confira a ficha.');
      expect(r.retry).toBe(false);
      expect(said(r)).not.toContain('Nada foi gasto');
    }
  });

  it('reads the sheet again after every failure but a call that never left', () => {
    expect(consumableNeedsReload(new ConnectError('x', Code.Unavailable))).toBe(false);
    for (const code of [
      Code.Unknown,
      Code.Internal,
      Code.DeadlineExceeded,
      Code.FailedPrecondition,
      Code.NotFound,
      Code.InvalidArgument,
    ]) {
      expect(consumableNeedsReload(new ConnectError('x', code))).toBe(true);
    }
  });
});

describe('turnBlockMessage', () => {
  it('says the turn is not theirs, for the player and for the master', () => {
    expect(turnBlockMessage('turn', null)).toBe('Só na sua vez. Beber ou dar gasta a ação.');
    expect(turnBlockMessage('turn', 'Tor')).toBe('Só na vez de Tor. Beber ou dar gasta a ação.');
  });

  it('says the action of the turn is spent, for the player and for the master', () => {
    expect(turnBlockMessage('action', null)).toBe('Ação já usada. Você já usou a ação desta vez.');
    expect(turnBlockMessage('action', 'Tor')).toBe('Ação já usada. Tor já usou a ação desta vez.');
  });

  it('has a bold lead and the rest apart', () => {
    expect(turnBlockParts('turn', 'Tor')).toEqual({
      lead: 'Só na vez de Tor.',
      text: 'Beber ou dar gasta a ação.',
    });
  });
});

describe('consumableTargetsRefusal', () => {
  const encounter = (reason: EncounterBlockedReason) =>
    new ConnectError('x', Code.FailedPrecondition, undefined, [
      { desc: EncounterBlockedSchema, value: create(EncounterBlockedSchema, { reason }) },
    ]);
  const potion = { kind: 'potion' as const, userName: 'Tor' };

  it('says the list could not be read when the call did not get through, offers to read it again and never speaks of dice', () => {
    for (const code of [Code.Unavailable, Code.InvalidArgument]) {
      const r = consumableTargetsRefusal(new ConnectError('x', code), potion);
      expect(r).toMatchObject({
        lead: 'Sem a lista.',
        text: 'Não deu para ver quem pode receber a poção.',
        retry: true,
      });
    }
    expect(
      consumableTargetsRefusal(new ConnectError('x', Code.Unavailable), {
        ...potion,
        kind: 'scroll',
      }).text,
    ).toBe('Não deu para ver quem pode receber o pergaminho.');
  });

  it('names the character that is not at the table any more', () => {
    expect(said(consumableTargetsRefusal(new ConnectError('x', Code.NotFound), potion))).toBe(
      'Fora da mesa. Tor não está mais na mesa.',
    );
  });

  it('says the item is already used when the entry is gone', () => {
    expect(
      consumableTargetsRefusal(encounter(EncounterBlockedReason.ITEM_GONE), potion),
    ).toMatchObject({
      lead: 'Essa poção já foi usada.',
      text: 'A ficha foi atualizada.',
      cause: 'item-gone',
    });
  });

  it('names the character that is dead', () => {
    const dead = new ConnectError('x', Code.FailedPrecondition, undefined, [
      {
        desc: CharacterBlockedSchema,
        value: create(CharacterBlockedSchema, { reason: CharacterBlockedReason.CHARACTER_DEAD }),
      },
    ]);
    expect(said(consumableTargetsRefusal(dead, potion))).toBe(
      'Tor morreu. Quem morreu não usa nem recebe itens.',
    );
  });
});

function said(r: { lead: string; text: string }): string {
  return `${r.lead} ${r.text}`;
}

describe('unreadSheetRefusal', () => {
  it('says the unit was used and the sheet was not read, and offers to try again', () => {
    expect(unreadSheetRefusal('potion')).toMatchObject({
      lead: 'Essa poção já foi usada.',
      text: 'Não conseguimos reler a ficha.',
      retry: true,
      cause: 'item-gone',
    });
    expect(unreadSheetRefusal('oil').lead).toBe('Esse óleo já foi usado.');
  });
});
