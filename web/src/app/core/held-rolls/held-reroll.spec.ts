import { create } from '@bufbuild/protobuf';

import { RollAttackResponseSchema } from '../../../gen/meurpg/play/v1/combat_pb';
import { MASTER_WAIT_SECONDS } from './held-roll-controller';
import {
  asksLucky,
  asksReroll,
  rerollFace,
  rerollOf,
  rerollSides,
  weaponHold,
} from './held-roll-offer';
import { answeredText } from './held-roll-result';
import { heldAnswer, heldDie, heldReroll } from './held-roll-testing';
import {
  rerollMasterFooter,
  rerollRollerFooter,
  askingText,
  d20Chip,
  d20Line,
  questionRule,
  questionTitle,
  rerollCost,
  rerollWhat,
} from './held-text';

describe('the reroll rule’s question (rolar de novo?)', () => {
  it('is its own question, with the face it asks about', () => {
    const offer = heldReroll(2);
    expect(asksReroll(offer)).toBe(true);
    expect(asksLucky(offer)).toBe(false);
    expect(asksReroll(heldDie())).toBe(false);
    expect(rerollOf(offer)?.sourceKey).toBe('feature:stubborn-luck');
    expect(rerollFace(offer)).toBe(2);
    expect(rerollFace(heldDie())).toBe(0);
    expect(weaponHold(offer)).toBe(offer);
  });

  it('asks about the die at reroll_index of a pair', () => {
    const offer = heldReroll(2);
    offer.d20!.faces = [14, 3];
    offer.question = {
      case: 'reroll',
      value: { ...rerollOf(offer)!, rerollIndex: 1 },
    };
    expect(rerollFace(offer)).toBe(3);
    expect(questionTitle(offer)).toBe('Tirou 3: rolar de novo?');
    expect(d20Chip(offer)).toBe('3');
    expect(rerollWhat(offer).text).toContain('o dado que deu 3');
  });

  it('draws the die from `sides`, a d20 when the server sent none', () => {
    expect(rerollSides(heldReroll())).toBe(20);
    const offer = heldReroll();
    offer.question = { case: 'reroll', value: { ...rerollOf(offer)!, sides: 0 } };
    expect(rerollSides(offer)).toBe(20);
    offer.question = { case: 'reroll', value: { ...rerollOf(offer)!, sides: 12 } };
    expect(rerollSides(offer)).toBe(12);
    expect(rerollSides(heldDie())).toBe(20);
  });

  it('words the roller’s screen as the board R6 does', () => {
    const offer = heldReroll(2, 5);
    expect(questionTitle(offer)).toBe('Tirou 2: rolar de novo?');
    expect(rerollWhat(offer)).toEqual({
      lead: 'Sorte Teimosa:',
      text: 'você rola este d20 de novo e fica com o novo resultado, mesmo se for pior. Gasta 1 uso de Sorte Teimosa.',
    });
    expect(d20Line(offer)).toBe('d20 · bônus +5 · total 7');
    expect(rerollRollerFooter(offer)).toBe(
      `O app rola o novo d20, também numa mesa de dados físicos. Se você não responder, o mestre pode responder por você depois de ${MASTER_WAIT_SECONDS} s.`,
    );
  });

  it('says "uma característica" for a source with no name, and counts the uses', () => {
    const nameless = heldReroll(2, 5, {}, '');
    expect(rerollWhat(nameless).lead).toBe('uma característica:');
    expect(rerollCost(nameless)).toBe('Gasta 1 uso de uma característica.');
    const two = heldReroll();
    two.question = { case: 'reroll', value: { ...rerollOf(two)!, cost: 2 } };
    expect(rerollCost(two)).toBe('Gasta 2 usos de Sorte Teimosa.');
    const free = heldReroll();
    free.question = { case: 'reroll', value: { ...rerollOf(free)!, cost: 0, resource: '' } };
    expect(rerollCost(free)).toBe('Não gasta nenhum uso.');
  });

  it('words the master’s card: the rule and that nothing is typed', () => {
    expect(questionRule(heldReroll())).toEqual({ lead: '', text: 'Sorte Teimosa gasta 1 uso.' });
    expect(rerollMasterFooter(heldReroll())).toBe(
      'O app rola o novo d20; não há resultado para digitar.',
    );
  });

  it('says what the answer on the way is doing', () => {
    const offer = heldReroll();
    expect(askingText({ kind: 'reroll', reroll: true, roll: null }, offer)).toBe(
      'Rolando o d20 de novo…',
    );
    expect(askingText({ kind: 'reroll', reroll: false, roll: null }, offer)).toBe(
      'Ficando com o resultado do d20…',
    );
  });

  describe('what the master’s answer did', () => {
    const asked = heldReroll(2);

    function attackAnswer(to: number | null) {
      const res = create(RollAttackResponseSchema, {
        roll: {
          d20: {
            diceCount: 1,
            diceSides: 20,
            faces: [to ?? 2],
            modifier: 5,
            total: (to ?? 2) + 5,
            rerolled:
              to === null ? [] : [{ index: 0, from: 2, to, sourceKey: 'feature:stubborn-luck' }],
          },
        },
      });
      return heldAnswer({ result: { case: 'attack', value: res } });
    }

    it('reads the new face and the total from the result', () => {
      expect(
        answeredText(
          'Brisa',
          { kind: 'reroll', reroll: true, roll: null },
          asked,
          attackAnswer(14),
        ),
      ).toBe('Você respondeu por Brisa: rolou de novo e tirou 14 (19 com o bônus).');
    });

    it('says only that it rolled when the result carries no d20', () => {
      expect(
        answeredText('Brisa', { kind: 'reroll', reroll: true, roll: null }, asked, heldAnswer()),
      ).toBe('Você respondeu por Brisa: rolou de novo.');
    });

    it('says that the roll was kept', () => {
      expect(
        answeredText(
          'Brisa',
          { kind: 'reroll', reroll: false, roll: null },
          asked,
          attackAnswer(null),
        ),
      ).toBe('Você respondeu por Brisa: ficou com o 2.');
    });
  });
});
