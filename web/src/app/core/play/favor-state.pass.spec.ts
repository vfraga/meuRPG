import { TestBed } from '@angular/core/testing';
import { afterEach, beforeEach, vi } from 'vitest';

import { FavorBlockedReason } from '../../../gen/meurpg/play/v1/favor_pb';
import { FavorClient } from './favor-client';
import { FavorState } from './favor-state';
import { FakeFavorClient, favorPass, favorRefund, favorRefused, failedRead } from './favor-testing';

function setup() {
  const client = new FakeFavorClient();
  TestBed.configureTestingModule({
    providers: [FavorState, { provide: FavorClient, useValue: client.asClient() }],
  });
  const favor = TestBed.inject(FavorState);
  return { client, favor };
}

const brisa = { characterId: 'brisa', name: 'Brisa' };

/** Follows as the player or the master and waits for the first read. */
async function follow(favor: FavorState, client: FakeFavorClient, master: boolean) {
  favor.follow('camp', master);
  await vi.waitFor(() => expect(client.reads.length).toBeGreaterThan(0));
  await favor.refresh();
}

const texts = (favor: FavorState) => favor.lines().map((l) => l.text);

describe('FavorState: the pass and the refund question', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    TestBed.resetTestingModule();
  });
  afterEach(() => vi.useRealTimers());

  describe('what it reads', () => {
    it('keeps the asks and the questions the read gives, and the chip waits only for a player', async () => {
      const { client, favor } = setup();
      client.holders = [brisa];
      client.passes = [favorPass()];
      client.refunds = [favorRefund()];
      await follow(favor, client, false);
      expect(favor.passes()).toEqual([favorPass()]);
      expect(favor.refunds()).toEqual([favorRefund()]);
      expect(favor.passOf('brisa')?.toName).toBe('Tor');
      expect(favor.waiting('brisa')).toBe(true);
      expect(favor.waiting('tor')).toBe(false);
      favor.follow('camp', true);
      expect(favor.waiting('brisa')).toBe(false);
    });

    it('keeps the last asks when a read fails, so a card the master is about to answer stays', async () => {
      const { client, favor } = setup();
      client.passes = [favorPass()];
      await follow(favor, client, true);
      client.holders = failedRead;
      await favor.refresh();
      expect(favor.failed()).toBe(true);
      expect(favor.passes()).toHaveLength(1);
    });

    it('forgets the asks and the questions when the session stops', async () => {
      const { client, favor } = setup();
      client.passes = [favorPass()];
      client.refunds = [favorRefund()];
      await follow(favor, client, true);
      favor.stop();
      expect(favor.passes()).toEqual([]);
      expect(favor.refunds()).toEqual([]);
      expect(favor.lines()).toEqual([]);
    });
  });

  describe('the giver asks', () => {
    it("sends the campaign, the two characters and the dialog's key, and shows the ask at once", async () => {
      const { client, favor } = setup();
      client.holders = [brisa];
      await follow(favor, client, false);
      client.passAnswer = favorPass();
      client.passes = [favorPass()];
      expect(await favor.ask('brisa', 'tor', 'key-1')).toBeNull();
      expect(client.asks).toEqual([
        { campaignId: 'camp', fromId: 'brisa', toId: 'tor', key: 'key-1' },
      ]);
      expect(favor.waiting('brisa')).toBe(true);
    });

    it('is "esperando" while it is on its way, and a second tap sends nothing', async () => {
      const { client, favor } = setup();
      await follow(favor, client, false);
      client.hold = 'ask';
      const first = favor.ask('brisa', 'tor', 'k');
      expect(favor.workOf('brisa')).toBe('ask');
      expect(await favor.ask('brisa', 'tor', 'k')).toBeNull();
      expect(client.asks).toHaveLength(1);
      client.release[0]();
      await first;
      expect(favor.workOf('brisa')).toBeNull();
    });

    it.each([
      [FavorBlockedReason.NO_SESSION, 'Só numa sessão aberta.'],
      [
        FavorBlockedReason.PASS_PENDING,
        'Você já pediu para passar a Inspiração. Espere o mestre ou cancele o pedido.',
      ],
      [FavorBlockedReason.NOT_THE_HOLDER, 'Você não está com a Inspiração agora.'],
      [FavorBlockedReason.NOT_A_PLAYER_CHARACTER, 'Essa personagem não está na mesa agora.'],
      [FavorBlockedReason.SELF, 'Você não pode passar a Inspiração a si mesmo.'],
    ])('says what refusal %s means, in words of its own', async (reason, text) => {
      const { client, favor } = setup();
      await follow(favor, client, false);
      client.passError = favorRefused(reason);
      expect(await favor.ask('brisa', 'tor', 'k')).toBe(text);
    });

    it('reads again after a refusal that says the screen is out of date', async () => {
      const { client, favor } = setup();
      await follow(favor, client, false);
      const reads = client.reads.length;
      client.passError = favorRefused(FavorBlockedReason.PASS_PENDING);
      await favor.ask('brisa', 'tor', 'k');
      await vi.waitFor(() => expect(client.reads.length).toBeGreaterThan(reads));
    });

    it('says so when it fails for another reason', async () => {
      const { client, favor } = setup();
      await follow(favor, client, false);
      client.passError = new Error('down');
      expect(await favor.ask('brisa', 'tor', 'k')).toBe(
        'Não foi possível pedir ao mestre. Tente de novo.',
      );
    });
  });

  describe('the giver cancels', () => {
    it("sends the ask's id, takes the ask away and tells nothing: the giver knows", async () => {
      const { client, favor } = setup();
      client.holders = [brisa];
      client.passes = [favorPass()];
      await follow(favor, client, false);
      client.passes = [];
      expect(await favor.cancelAsk(favorPass())).toBeNull();
      expect(client.cancels[0]).toMatchObject({ campaignId: 'camp', passId: 'pass-1' });
      await favor.refresh();
      expect(favor.waiting('brisa')).toBe(false);
      expect(favor.lines()).toEqual([]);
    });

    it('reads "Cancelando…" while it is on its way', async () => {
      const { client, favor } = setup();
      client.passes = [favorPass()];
      await follow(favor, client, false);
      client.hold = 'cancel';
      const call = favor.cancelAsk(favorPass());
      expect(favor.workOf('pass-1')).toBe('cancel');
      client.release[0]();
      await call;
      expect(favor.workOf('pass-1')).toBeNull();
    });

    it('says the ask was answered meanwhile and reads again', async () => {
      const { client, favor } = setup();
      client.passes = [favorPass()];
      await follow(favor, client, false);
      client.cancelError = favorRefused(FavorBlockedReason.PASS_CLOSED);
      expect(await favor.cancelAsk(favorPass())).toBe('Esse pedido já foi respondido.');
    });

    it('keeps the ask and says so when the call fails', async () => {
      const { client, favor } = setup();
      client.passes = [favorPass()];
      await follow(favor, client, false);
      client.cancelError = new Error('down');
      expect(await favor.cancelAsk(favorPass())).toBe(
        'Não foi possível cancelar o pedido. Tente de novo.',
      );
      expect(favor.passes()).toHaveLength(1);
    });
  });

  describe('what the giver reads of the answer (581-07, items 5 and 6)', () => {
    it("reads that the master accepted it, with the receiver's name, when the ask is gone and the Inspiração with it", async () => {
      const { client, favor } = setup();
      client.holders = [brisa];
      client.passes = [favorPass()];
      await follow(favor, client, false);
      client.holders = [];
      client.passes = [];
      await favor.refresh();
      expect(texts(favor)).toEqual(['O mestre aceitou: a sua Inspiração foi para Tor.']);
      expect(favor.lines()[0].tone).toBe('success');
      // It was not a spent one that came back, nor a grant.
      expect(favor.gained()).toBe('');
    });

    it('reads one sentence for every refusal, and never anything of the receiver', async () => {
      const { client, favor } = setup();
      client.holders = [brisa];
      client.passes = [favorPass({ receiverHas: false })];
      await follow(favor, client, false);
      client.passes = [];
      await favor.refresh();
      expect(texts(favor)).toEqual(['O mestre recusou: a Inspiração continua com você.']);
      expect(texts(favor).join(' ')).not.toContain('já tem');
      expect(texts(favor).join(' ')).not.toContain('não pode ficar');
    });

    it('reads nothing the first time it reads: the ask it never saw is no news', async () => {
      const { client, favor } = setup();
      client.holders = [brisa];
      await follow(favor, client, false);
      expect(favor.lines()).toEqual([]);
    });

    it('dismisses a line, and keeps at most four', async () => {
      const { client, favor } = setup();
      client.holders = [brisa];
      await follow(favor, client, false);
      for (let i = 0; i < 6; i++) {
        client.passes = [favorPass({ id: `p${i}` })];
        await favor.refresh();
        client.passes = [];
        await favor.refresh();
      }
      expect(favor.lines()).toHaveLength(4);
      favor.dismissLine(favor.lines()[0].id);
      expect(favor.lines()).toHaveLength(3);
    });
  });

  describe('what the roller reads of a refund (581-07, item 2)', () => {
    it('reads that the master gave it back, when the Inspiração a roll spent comes back', async () => {
      const { client, favor } = setup();
      client.holders = [{ characterId: 'jabuti', name: 'Jabuti' }];
      await follow(favor, client, false);
      favor.spentBy('jabuti');
      client.holders = [];
      await favor.refresh();
      client.holders = [{ characterId: 'jabuti', name: 'Jabuti' }];
      await favor.refresh();
      expect(texts(favor)).toEqual(['O mestre devolveu a sua Inspiração.']);
      expect(favor.gained()).toBe('');
    });

    it('reads a grant after the spend as a grant, when this screen did not spend it (another device, or the master gave another)', async () => {
      const { client, favor } = setup();
      client.holders = [{ characterId: 'jabuti', name: 'Jabuti' }];
      await follow(favor, client, false);
      client.holders = [];
      await favor.refresh();
      client.holders = [{ characterId: 'jabuti', name: 'Jabuti' }];
      await favor.refresh();
      expect(favor.gained()).toBe('jabuti');
      expect(favor.lines()).toEqual([]);
    });

    it("reads any of the reader's characters as the spender when the roll does not name one", async () => {
      const { client, favor } = setup();
      client.holders = [{ characterId: 'jabuti', name: 'Jabuti' }];
      await follow(favor, client, false);
      favor.spentBy('');
      client.holders = [];
      await favor.refresh();
      client.holders = [{ characterId: 'jabuti', name: 'Jabuti' }];
      await favor.refresh();
      expect(texts(favor)).toEqual(['O mestre devolveu a sua Inspiração.']);
    });

    it('reads a grant to a character that never spent one as a grant', async () => {
      const { client, favor } = setup();
      client.holders = [];
      await follow(favor, client, false);
      client.holders = [brisa];
      await favor.refresh();
      expect(favor.gained()).toBe('brisa');
      expect(favor.lines()).toEqual([]);
    });
  });

  describe('the master answers an ask', () => {
    it('sends the answer, takes the card away at once and tells nothing', async () => {
      const { client, favor } = setup();
      client.passes = [favorPass()];
      await follow(favor, client, true);
      client.passes = [];
      await favor.answerPass(favorPass(), true);
      expect(client.passAnswers[0]).toMatchObject({
        campaignId: 'camp',
        passId: 'pass-1',
        accept: true,
      });
      expect(favor.passes()).toEqual([]);
      expect(favor.lines()).toEqual([]);
    });

    it('reads "Aceitando…" and "Recusando…" while it is on its way, and a second tap sends nothing', async () => {
      const { client, favor } = setup();
      client.passes = [favorPass()];
      await follow(favor, client, true);
      client.hold = 'answerPass';
      const first = favor.answerPass(favorPass(), true);
      expect(favor.workOf('pass-1')).toBe('accept');
      await favor.answerPass(favorPass(), false);
      expect(client.passAnswers).toHaveLength(1);
      client.release[0]();
      await first;
      client.hold = 'none';
      client.passes = [favorPass({ id: 'pass-2' })];
      await favor.refresh();
      client.hold = 'answerPass';
      const refuse = favor.answerPass(favorPass({ id: 'pass-2' }), false);
      expect(favor.workOf('pass-2')).toBe('refuse');
      client.release[1]();
      await refuse;
    });

    it('does not send an accept when the receiver has one already: the button is dashed', async () => {
      const { client, favor } = setup();
      client.passes = [favorPass({ receiverHas: true })];
      await follow(favor, client, true);
      await favor.answerPass(favorPass({ receiverHas: true }), true);
      expect(client.passAnswers).toEqual([]);
    });

    it('still sends a refusal for a receiver that has one', async () => {
      const { client, favor } = setup();
      client.passes = [favorPass({ receiverHas: true })];
      await follow(favor, client, true);
      await favor.answerPass(favorPass({ receiverHas: true }), false);
      expect(client.passAnswers).toHaveLength(1);
    });

    it('tells that the giver canceled and closes the card when the answer finds it closed', async () => {
      const { client, favor } = setup();
      client.passes = [favorPass()];
      await follow(favor, client, true);
      client.answerPassError = favorRefused(FavorBlockedReason.PASS_CLOSED);
      client.passes = [];
      await favor.answerPass(favorPass(), true);
      expect(texts(favor)).toEqual(['Brisa cancelou o pedido antes da sua resposta. Nada mudou.']);
      expect(favor.passes()).toEqual([]);
    });

    it('says it once when the ask is gone by the read and by the answer', async () => {
      const { client, favor } = setup();
      client.passes = [favorPass()];
      await follow(favor, client, true);
      client.hold = 'answerPass';
      client.answerPassError = favorRefused(FavorBlockedReason.PASS_CLOSED);
      const call = favor.answerPass(favorPass(), false);
      client.passes = [];
      await favor.refresh();
      client.release[0]();
      await call;
      expect(texts(favor)).toEqual(['Brisa cancelou o pedido antes da sua resposta. Nada mudou.']);
    });

    it('keeps the card and says the receiver cannot take it when an accept finds CANNOT_RECEIVE', async () => {
      const { client, favor } = setup();
      client.passes = [favorPass()];
      await follow(favor, client, true);
      client.answerPassError = favorRefused(FavorBlockedReason.CANNOT_RECEIVE);
      await favor.answerPass(favorPass(), true);
      expect(texts(favor)).toEqual(['Essa personagem não pode ficar com a Inspiração agora.']);
      expect(favor.passes()).toHaveLength(1);
      // The ask still goes away like any other later: its closing is news again.
      client.passes = [];
      await favor.refresh();
      expect(texts(favor)).toHaveLength(2);
    });

    it('keeps the card and says Brisa no longer has it when an accept finds NOT_THE_HOLDER', async () => {
      const { client, favor } = setup();
      client.passes = [favorPass()];
      await follow(favor, client, true);
      client.answerPassError = favorRefused(FavorBlockedReason.NOT_THE_HOLDER);
      await favor.answerPass(favorPass(), true);
      expect(texts(favor)).toEqual(['Brisa não está mais com a Inspiração. Recuse o pedido.']);
      expect(favor.passes()).toHaveLength(1);
    });

    it('keeps the card and says so when the call fails', async () => {
      const { client, favor } = setup();
      client.passes = [favorPass()];
      await follow(favor, client, true);
      client.answerPassError = new Error('down');
      await favor.answerPass(favorPass(), false);
      expect(texts(favor)).toEqual(['Não foi possível responder agora. Tente de novo.']);
      expect(favor.passes()).toHaveLength(1);
    });

    it('tells the master that a card he did not answer here left the list', async () => {
      const { client, favor } = setup();
      client.passes = [favorPass()];
      await follow(favor, client, true);
      client.passes = [];
      await favor.refresh();
      expect(texts(favor)).toEqual(['Brisa cancelou o pedido antes da sua resposta. Nada mudou.']);
    });
  });

  describe('the master answers the refund question', () => {
    it('sends "devolver" and "manter" and takes the card away', async () => {
      const { client, favor } = setup();
      client.refunds = [favorRefund(), favorRefund({ id: 'refund-2' })];
      await follow(favor, client, true);
      client.refunds = [favorRefund({ id: 'refund-2' })];
      await favor.answerRefund(favorRefund(), true);
      expect(client.refundAnswers[0]).toMatchObject({ promptId: 'refund-1', giveBack: true });
      client.refunds = [];
      await favor.answerRefund(favorRefund({ id: 'refund-2' }), false);
      expect(client.refundAnswers[1]).toMatchObject({ promptId: 'refund-2', giveBack: false });
      expect(favor.refunds()).toEqual([]);
      expect(favor.lines()).toEqual([]);
    });

    it('reads "Devolvendo…" and "Mantendo…" while it is on its way', async () => {
      const { client, favor } = setup();
      client.refunds = [favorRefund(), favorRefund({ id: 'refund-2' })];
      await follow(favor, client, true);
      client.hold = 'answerRefund';
      const give = favor.answerRefund(favorRefund(), true);
      const keep = favor.answerRefund(favorRefund({ id: 'refund-2' }), false);
      expect(favor.workOf('refund-1')).toBe('give');
      expect(favor.workOf('refund-2')).toBe('keep');
      client.release.forEach((r) => r());
      await Promise.all([give, keep]);
      expect(favor.working().size).toBe(0);
    });

    it('says the roller has one again, closes the card and keeps it spent on ALREADY_HAS', async () => {
      const { client, favor } = setup();
      client.refunds = [favorRefund()];
      await follow(favor, client, true);
      client.answerRefundError = favorRefused(FavorBlockedReason.ALREADY_HAS);
      // After the screen's own "keep" the server no longer lists the question.
      client.refunds = [];
      await favor.answerRefund(favorRefund(), true);
      expect(texts(favor)).toEqual([
        'Jabuti já tem Inspiração de novo: não há o que devolver. O pedido fechou.',
      ]);
      expect(favor.refunds()).toEqual([]);
      // The server leaves the question open after a refused give back: the screen answers "keep" so that it closes.
      expect(client.refundAnswers.map((a) => a.giveBack)).toEqual([true, false]);
    });

    it('keeps the card and says the roller cannot take it on CANNOT_RECEIVE', async () => {
      const { client, favor } = setup();
      client.refunds = [favorRefund()];
      await follow(favor, client, true);
      client.answerRefundError = favorRefused(FavorBlockedReason.CANNOT_RECEIVE);
      await favor.answerRefund(favorRefund(), true);
      expect(texts(favor)).toEqual(['Essa personagem não pode ficar com a Inspiração agora.']);
      expect(favor.refunds()).toHaveLength(1);
      expect(client.refundAnswers).toHaveLength(1);
    });

    it('closes the card and says it was answered elsewhere on PASS_CLOSED', async () => {
      const { client, favor } = setup();
      client.refunds = [favorRefund()];
      await follow(favor, client, true);
      client.answerRefundError = favorRefused(FavorBlockedReason.PASS_CLOSED);
      client.refunds = [];
      await favor.answerRefund(favorRefund(), false);
      expect(texts(favor)).toEqual(['Esse pedido já foi respondido em outra tela.']);
      expect(favor.refunds()).toEqual([]);
    });

    it('tells the master when a question he did not answer here went away', async () => {
      const { client, favor } = setup();
      client.refunds = [favorRefund()];
      await follow(favor, client, true);
      client.refunds = [];
      await favor.refresh();
      expect(texts(favor)).toEqual(['Esse pedido já foi respondido em outra tela.']);
    });

    it("tells a player nothing about a question: it is the master's", async () => {
      const { client, favor } = setup();
      client.refunds = [favorRefund()];
      await follow(favor, client, false);
      client.refunds = [];
      await favor.refresh();
      expect(favor.lines()).toEqual([]);
    });
  });
});
