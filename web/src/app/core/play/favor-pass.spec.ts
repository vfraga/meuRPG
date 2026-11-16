import { FavorBlockedReason } from '../../../gen/meurpg/play/v1/favor_pb';
import {
  FAVOR_ANSWER_FAILED,
  FAVOR_WORK_LABEL,
  askOutcome,
  cancelOutcome,
  favorPassBody,
  favorPassPendingText,
  favorPassRow,
  favorPassSubtitle,
  favorPassTitle,
  favorRefundBody,
  favorRefundNote,
  passAnswerOutcome,
  refundAnswerOutcome,
} from './favor-pass';
import { favorPass, favorRefund, favorRefused } from './favor-testing';

const flat = (pieces: readonly { text: string }[]) => pieces.map((p) => p.text).join('');

describe('the words of the pass', () => {
  it('reads each in-flight label as the board does', () => {
    expect(FAVOR_WORK_LABEL).toEqual({
      ask: 'Pedindo ao mestre…',
      cancel: 'Cancelando…',
      accept: 'Aceitando…',
      refuse: 'Recusando…',
      give: 'Devolvendo…',
      keep: 'Mantendo…',
    });
  });

  it('words the ask, its subtitle and its row', () => {
    expect(favorPassPendingText('Tor')).toBe(
      'Você pediu para passar a Inspiração a Tor. Cancele o pedido para usar.',
    );
    expect(favorPassSubtitle('Brisa')).toBe('Brisa · o mestre aceita ou recusa');
    expect(favorPassRow('Tor')).toEqual({ lead: 'Para Tor', rest: ' · esperando o mestre' });
  });

  it("words the master's card, with the receiver in bold", () => {
    expect(favorPassTitle('Brisa')).toBe('Brisa quer passar a Inspiração');
    const body = favorPassBody(favorPass());
    expect(body[0]).toEqual({ text: 'Para Tor.', bold: true });
    expect(flat(body)).toBe('Para Tor. Se aceitar, ela sai de Brisa e vai para Tor.');
  });

  it('says the receiver has one already, and that accepting is not possible', () => {
    const body = favorPassBody(favorPass({ toName: 'Pensantus', receiverHas: true }));
    expect(flat(body)).toBe('Para Pensantus. Pensantus já tem Inspiração: aceitar não é possível.');
  });

  it('words the refund question from the roll and the sources only the master reads', () => {
    const body = favorRefundBody(favorRefund());
    expect(flat(body)).toBe(
      'Jabuti usou a Inspiração (Ataque). A rolagem já tinha vantagem por algo que Jabuti não vê: Alvo Impedido (só você vê).',
    );
    expect(body.filter((p) => p.bold).map((p) => p.text)).toEqual(['Alvo Impedido']);
  });

  it('lists several sources, each in bold, once', () => {
    const body = favorRefundBody(
      favorRefund({
        hidden: [
          { name: 'Alvo Impedido', advantage: true },
          { name: 'Alvo Impedido', advantage: true },
          { name: 'Atacante Invisível', advantage: true },
          { name: 'Além do alcance normal', advantage: false },
        ],
      }),
    );
    expect(flat(body)).toBe(
      'Jabuti usou a Inspiração (Ataque). A rolagem já tinha vantagem por algo que Jabuti não vê: Alvo Impedido, Atacante Invisível (só você vê).',
    );
  });

  it('says the roll already had advantage when the server names no source', () => {
    expect(flat(favorRefundBody(favorRefund({ hidden: [], rollLabel: '' })))).toBe(
      'Jabuti usou a Inspiração. A rolagem já tinha vantagem.',
    );
  });

  it('tells the master what the roller will read', () => {
    expect(favorRefundNote('Jabuti')).toBe(
      'Sem resposta, ela fica gasta. Se devolver, Jabuti lê só que você devolveu.',
    );
  });
});

describe('what a refusal says', () => {
  const pass = favorPass();

  it('an answer to a pass: closed means the giver gave it up and the card leaves; the others leave it', () => {
    expect(passAnswerOutcome(favorRefused(FavorBlockedReason.PASS_CLOSED), pass)).toEqual({
      closes: true,
      text: 'Brisa cancelou o pedido antes da sua resposta. Nada mudou.',
    });
    expect(passAnswerOutcome(favorRefused(FavorBlockedReason.CANNOT_RECEIVE), pass)).toEqual({
      closes: false,
      text: 'Essa personagem não pode ficar com a Inspiração agora.',
    });
    expect(passAnswerOutcome(favorRefused(FavorBlockedReason.NOT_THE_HOLDER), pass).closes).toBe(
      false,
    );
    expect(passAnswerOutcome(new Error('down'), pass)).toEqual({
      closes: false,
      text: FAVOR_ANSWER_FAILED,
    });
  });

  it('an answer to the refund question: closed, has again (keeps it spent), cannot receive, the rest', () => {
    const prompt = favorRefund();
    expect(refundAnswerOutcome(favorRefused(FavorBlockedReason.PASS_CLOSED), prompt)).toEqual({
      closes: true,
      text: 'Esse pedido já foi respondido em outra tela.',
    });
    expect(refundAnswerOutcome(favorRefused(FavorBlockedReason.ALREADY_HAS), prompt)).toEqual({
      closes: true,
      keep: true,
      text: 'Jabuti já tem Inspiração de novo: não há o que devolver. O pedido fechou.',
    });
    expect(
      refundAnswerOutcome(favorRefused(FavorBlockedReason.CANNOT_RECEIVE), prompt).closes,
    ).toBe(false);
    expect(refundAnswerOutcome(new Error('down'), prompt).text).toBe(FAVOR_ANSWER_FAILED);
  });

  it("an ask never names the receiver's state: CANNOT_RECEIVE is not one a giver reads", () => {
    expect(askOutcome(favorRefused(FavorBlockedReason.CANNOT_RECEIVE)).text).not.toContain(
      'não pode ficar',
    );
  });

  it('a cancel found the ask answered', () => {
    expect(cancelOutcome(favorRefused(FavorBlockedReason.PASS_CLOSED))).toEqual({
      text: 'Esse pedido já foi respondido.',
      stale: true,
    });
    expect(cancelOutcome(new Error('x')).stale).toBe(false);
  });
});
