import { type ComponentFixture, TestBed } from '@angular/core/testing';
import { vi } from 'vitest';

import { FavorBlockedReason } from '../../../gen/meurpg/play/v1/favor_pb';
import { FavorClient } from '../../core/play/favor-client';
import { FavorState } from '../../core/play/favor-state';
import {
  FakeFavorClient,
  favorPass,
  favorRefund,
  favorRefused,
} from '../../core/play/favor-testing';
import { FavorRequests } from './favor-requests';

const flat = (n: Element | null | undefined) => n?.textContent?.replace(/\s+/g, ' ').trim() ?? '';

async function setup(
  passes: Parameters<typeof favorPass>[0][] = [],
  refunds: Parameters<typeof favorRefund>[0][] = [],
) {
  const client = new FakeFavorClient();
  client.passes = passes.map((p) => favorPass(p));
  client.refunds = refunds.map((r) => favorRefund(r));
  TestBed.configureTestingModule({
    providers: [{ provide: FavorClient, useValue: client.asClient() }],
  });
  const favor = TestBed.inject(FavorState);
  favor.follow('camp', true);
  await favor.refresh();
  const fixture = TestBed.createComponent(FavorRequests);
  const answered = vi.fn();
  fixture.componentInstance.answered.subscribe(answered);
  await settle(fixture);
  return { fixture, client, favor, answered, el: fixture.nativeElement as HTMLElement };
}

async function settle(fixture: ComponentFixture<unknown>) {
  for (let i = 0; i < 3; i++) {
    await fixture.whenStable();
    await Promise.resolve();
    fixture.detectChanges();
  }
}

const button = (el: HTMLElement, card: string, answer: string) =>
  el.querySelector<HTMLButtonElement>(`[data-testid=${card}] [data-answer=${answer}]`)!;
const lines = (el: HTMLElement) =>
  [...el.querySelectorAll('[data-testid=favor-line]')].map((l) => flat(l.querySelector('p')));

describe('FavorRequests', () => {
  beforeEach(() => TestBed.resetTestingModule());

  it('draws nothing while nothing waits, with its live regions in the page', async () => {
    const { el } = await setup();
    expect(el.querySelector('.fr')).toBeNull();
    expect(el.querySelector('[role=status]')?.getAttribute('aria-live')).toBe('polite');
    expect(el.querySelector('[role=alert]')).not.toBeNull();
  });

  describe('the ask to pass it on', () => {
    it('reads the title, who it goes to and the two answers, under "Para você responder" with its count', async () => {
      const { el } = await setup([{}]);
      expect(flat(el.querySelector('#fr-h'))).toContain('Para você responder');
      expect(flat(el.querySelector('[data-testid=favor-count]'))).toBe('1 pedido');
      expect(flat(el.querySelector('.fr__wait'))).toBe('A mesa não espera');
      const card = el.querySelector('[data-testid=favor-pass-card]')!;
      expect(flat(card.querySelector('h3'))).toBe('Brisa quer passar a Inspiração');
      expect(flat(card.querySelector('.fr__text'))).toBe(
        'Para Tor. Se aceitar, ela sai de Brisa e vai para Tor.',
      );
      expect(flat(card.querySelector('.fr__text b'))).toBe('Para Tor.');
      expect(flat(button(el, 'favor-pass-card', 'refuse'))).toContain('Recusar');
      expect(flat(button(el, 'favor-pass-card', 'accept'))).toContain('Aceitar');
    });

    it('is a named article, and its buttons are described by its title', async () => {
      const { el } = await setup([{ id: 'p1' }]);
      const card = el.querySelector('[data-testid=favor-pass-card]')!;
      expect(card.tagName).toBe('ARTICLE');
      expect(card.getAttribute('aria-labelledby')).toBe('fr-t-p1');
      expect(button(el, 'favor-pass-card', 'accept').getAttribute('aria-describedby')).toBe(
        'fr-t-p1',
      );
      expect(button(el, 'favor-pass-card', 'accept').hasAttribute('aria-label')).toBe(false);
    });

    it('accepts, and the card leaves with nothing said', async () => {
      const { fixture, client, favor, el } = await setup([{}]);
      client.passes = [];
      button(el, 'favor-pass-card', 'accept').click();
      await settle(fixture);
      expect(client.passAnswers.map((a) => [a.passId, a.accept])).toEqual([['pass-1', true]]);
      expect(favor.passes()).toEqual([]);
      expect(el.querySelector('[data-testid=favor-pass-card]')).toBeNull();
      expect(lines(el)).toEqual([]);
    });

    it('refuses with "Recusar"', async () => {
      const { fixture, client, el } = await setup([{}]);
      client.passes = [];
      button(el, 'favor-pass-card', 'refuse').click();
      await settle(fixture);
      expect(client.passAnswers.map((a) => a.accept)).toEqual([false]);
    });

    it('says in "Aceitar" dashed why it cannot be accepted when the receiver has one, and "Recusar" still works', async () => {
      const { fixture, client, el } = await setup([
        { id: 'p1', toName: 'Pensantus', receiverHas: true },
      ]);
      const card = el.querySelector('[data-testid=favor-pass-card]')!;
      expect(flat(card.querySelector('.fr__text'))).toBe(
        'Para Pensantus. Pensantus já tem Inspiração: aceitar não é possível.',
      );
      const accept = button(el, 'favor-pass-card', 'accept');
      expect(accept.getAttribute('aria-disabled')).toBe('true');
      expect(accept.classList.contains('mr-button--off')).toBe(true);
      expect(accept.getAttribute('aria-describedby')).toBe('fr-b-p1');
      accept.click();
      await settle(fixture);
      expect(client.passAnswers).toEqual([]);
      const refuse = button(el, 'favor-pass-card', 'refuse');
      expect(refuse.getAttribute('aria-disabled')).toBeNull();
      client.passes = [];
      refuse.click();
      await settle(fixture);
      expect(client.passAnswers.map((a) => a.accept)).toEqual([false]);
    });

    it('reads "Aceitando…" on the pressed button and dashes its pair, and does not send twice', async () => {
      const { fixture, client, el } = await setup([{}]);
      client.hold = 'answerPass';
      button(el, 'favor-pass-card', 'accept').click();
      await settle(fixture);
      const accept = button(el, 'favor-pass-card', 'accept');
      const refuse = button(el, 'favor-pass-card', 'refuse');
      expect(flat(accept)).toContain('Aceitando…');
      expect(accept.getAttribute('aria-disabled')).toBe('true');
      expect(refuse.getAttribute('aria-disabled')).toBe('true');
      expect(refuse.classList.contains('mr-button--off')).toBe(true);
      expect(flat(refuse)).toContain('Recusar');
      accept.click();
      refuse.click();
      await settle(fixture);
      expect(client.passAnswers).toHaveLength(1);
      client.release.forEach((r) => r());
      await settle(fixture);
    });

    it('reads "Recusando…" on "Recusar" and dashes "Aceitar"', async () => {
      const { fixture, client, el } = await setup([{}]);
      client.hold = 'answerPass';
      button(el, 'favor-pass-card', 'refuse').click();
      await settle(fixture);
      expect(flat(button(el, 'favor-pass-card', 'refuse'))).toContain('Recusando…');
      expect(button(el, 'favor-pass-card', 'accept').getAttribute('aria-disabled')).toBe('true');
      client.release.forEach((r) => r());
      await settle(fixture);
    });

    it('says the receiver cannot hold it, and keeps the card, when the accept is refused with CANNOT_RECEIVE', async () => {
      const { fixture, client, el } = await setup([{}]);
      client.answerPassError = favorRefused(FavorBlockedReason.CANNOT_RECEIVE);
      button(el, 'favor-pass-card', 'accept').click();
      await settle(fixture);
      expect(lines(el)).toEqual(['Essa personagem não pode ficar com a Inspiração agora.']);
      expect(el.querySelector('[data-testid=favor-pass-card]')).not.toBeNull();
      expect(button(el, 'favor-pass-card', 'accept').getAttribute('aria-disabled')).toBeNull();
    });

    it('says the giver gave the ask up, and closes the card, when the answer finds it closed', async () => {
      const { fixture, client, el } = await setup([{}]);
      client.answerPassError = favorRefused(FavorBlockedReason.PASS_CLOSED);
      client.passes = [];
      button(el, 'favor-pass-card', 'accept').click();
      await settle(fixture);
      expect(lines(el)).toEqual(['Brisa cancelou o pedido antes da sua resposta. Nada mudou.']);
      expect(el.querySelector('[data-testid=favor-pass-card]')).toBeNull();
    });

    it('says the same when the ask leaves the list before it was answered', async () => {
      const { fixture, client, favor, el } = await setup([{}]);
      client.passes = [];
      await favor.refresh();
      await settle(fixture);
      expect(lines(el)).toEqual(['Brisa cancelou o pedido antes da sua resposta. Nada mudou.']);
      expect(el.querySelector('.fr')).toBeNull();
    });

    it('dismisses a notice with its close button', async () => {
      const { fixture, client, favor, el } = await setup([{}]);
      client.passes = [];
      await favor.refresh();
      await settle(fixture);
      const close = el.querySelector<HTMLButtonElement>('[data-testid=favor-line] .close')!;
      expect(close.getAttribute('aria-label')).toBe('Dispensar o aviso');
      close.click();
      await settle(fixture);
      expect(lines(el)).toEqual([]);
    });
  });

  describe('the roll that spent it for nothing', () => {
    it('names the roller in a chip, the roll and the hidden source, and asks what to do', async () => {
      const { el } = await setup([], [{ id: 'r1' }]);
      const card = el.querySelector('[data-testid=favor-refund-card]')!;
      expect(flat(card.querySelector('h3'))).toBe('Inspiração sem efeito');
      expect(flat(card.querySelector('[data-testid=favor-who]'))).toContain('Jabuti');
      expect(flat(card.querySelector('.fr__text'))).toBe(
        'Jabuti usou a Inspiração (Ataque). A rolagem já tinha vantagem por algo que Jabuti não vê: Alvo Impedido (só você vê).',
      );
      expect(flat(card.querySelector('.fr__text b'))).toBe('Alvo Impedido');
      expect(flat(card.querySelector('.fr__note'))).toBe(
        'Sem resposta, ela fica gasta. Se devolver, Jabuti lê só que você devolveu.',
      );
      expect(flat(button(el, 'favor-refund-card', 'keep'))).toContain('Manter gasta');
      expect(flat(button(el, 'favor-refund-card', 'give'))).toContain('Devolver a Inspiração');
    });

    it('gives it back, and keeps it spent', async () => {
      const { fixture, client, el } = await setup([], [{ id: 'r1' }, { id: 'r2' }]);
      client.refunds = [favorRefund({ id: 'r2' })];
      el.querySelectorAll<HTMLButtonElement>('[data-answer=give]')[0].click();
      await settle(fixture);
      client.refunds = [];
      el.querySelectorAll<HTMLButtonElement>('[data-answer=keep]')[0].click();
      await settle(fixture);
      expect(client.refundAnswers.map((a) => [a.promptId, a.giveBack])).toEqual([
        ['r1', true],
        ['r2', false],
      ]);
    });

    it('reads "Devolvendo…" and "Mantendo…" on the pressed button, with its pair dashed', async () => {
      const { fixture, client, el } = await setup([], [{ id: 'r1' }]);
      client.hold = 'answerRefund';
      button(el, 'favor-refund-card', 'give').click();
      await settle(fixture);
      expect(flat(button(el, 'favor-refund-card', 'give'))).toContain('Devolvendo…');
      expect(button(el, 'favor-refund-card', 'keep').getAttribute('aria-disabled')).toBe('true');
      expect(button(el, 'favor-refund-card', 'keep').classList.contains('mr-button--off')).toBe(
        true,
      );
      client.release.forEach((r) => r());
      await settle(fixture);
      TestBed.resetTestingModule();

      const again = await setup([], [{ id: 'r1' }]);
      again.client.hold = 'answerRefund';
      button(again.el, 'favor-refund-card', 'keep').click();
      await settle(again.fixture);
      expect(flat(button(again.el, 'favor-refund-card', 'keep'))).toContain('Mantendo…');
      expect(button(again.el, 'favor-refund-card', 'give').getAttribute('aria-disabled')).toBe(
        'true',
      );
      again.client.release.forEach((r) => r());
      await settle(again.fixture);
    });

    it('says the roller has one again, and closes the card, when "Devolver" finds ALREADY_HAS', async () => {
      const { fixture, client, el } = await setup([], [{ id: 'r1' }]);
      client.answerRefundError = favorRefused(FavorBlockedReason.ALREADY_HAS);
      client.refunds = [];
      button(el, 'favor-refund-card', 'give').click();
      await settle(fixture);
      expect(lines(el)).toEqual([
        'Jabuti já tem Inspiração de novo: não há o que devolver. O pedido fechou.',
      ]);
      expect(el.querySelector('[data-testid=favor-refund-card]')).toBeNull();
    });

    it('says it was answered on another screen when the answer finds it closed', async () => {
      const { fixture, client, el } = await setup([], [{ id: 'r1' }]);
      client.answerRefundError = favorRefused(FavorBlockedReason.PASS_CLOSED);
      client.refunds = [];
      button(el, 'favor-refund-card', 'keep').click();
      await settle(fixture);
      expect(lines(el)).toEqual(['Esse pedido já foi respondido em outra tela.']);
    });
  });

  describe('both at once', () => {
    it('counts them together, the ask first', async () => {
      const { el } = await setup([{ id: 'p1' }], [{ id: 'r1' }]);
      expect(flat(el.querySelector('[data-testid=favor-count]'))).toBe('2 pedidos');
      const cards = [...el.querySelectorAll('article')].map((a) => a.getAttribute('data-testid'));
      expect(cards).toEqual(['favor-pass-card', 'favor-refund-card']);
    });

    it('says the title of a card that arrived, in an alert', async () => {
      const { fixture, client, favor, el } = await setup();
      client.passes = [favorPass()];
      await favor.refresh();
      await settle(fixture);
      expect(flat(el.querySelector('[role=alert]'))).toBe('Brisa quer passar a Inspiração');
    });

    it('moves the focus to the next card after an answer, and tells the page when none is left', async () => {
      const { fixture, client, el, answered } = await setup([{ id: 'p1' }], [{ id: 'r1' }]);
      document.body.appendChild(el);
      client.passes = [];
      button(el, 'favor-pass-card', 'refuse').focus();
      button(el, 'favor-pass-card', 'refuse').click();
      await settle(fixture);
      expect(document.activeElement).toBe(el.querySelector('#fr-t-r1'));
      client.refunds = [];
      button(el, 'favor-refund-card', 'keep').focus();
      button(el, 'favor-refund-card', 'keep').click();
      await settle(fixture);
      expect(answered).toHaveBeenCalledTimes(1);
      el.remove();
    });
  });
});
