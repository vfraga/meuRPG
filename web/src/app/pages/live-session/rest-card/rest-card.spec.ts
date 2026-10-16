import { create } from '@bufbuild/protobuf';
import { TestBed } from '@angular/core/testing';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  GameSessionBlockedReason,
  GameSessionBlockedSchema,
  CharacterVitalsSchema,
} from '../../../../gen/meurpg/play/v1/play_pb';
import {
  ResourceBlockedReason,
  ResourceBlockedSchema,
  RestKind,
} from '../../../../gen/meurpg/play/v1/resources_pb';
import {
  boardParty,
  restPreview,
  restPreviewResponse,
} from '../../../core/resources/resources-testing';
import { ResourceClient } from '../../../core/resources/resources-client';
import type { VitalsVm } from '../../../core/play/live-session.types';
import { pensantusVitals } from '../../../core/play/live-testing';
import { RestCard } from './rest-card';

const flat = (n: Element | null | undefined) => n?.textContent?.replace(/\s+/g, ' ').trim();

function blocked(reason: ResourceBlockedReason): ConnectError {
  return new ConnectError('blocked', Code.FailedPrecondition, undefined, [
    { desc: ResourceBlockedSchema, value: create(ResourceBlockedSchema, { reason }) },
  ]);
}

describe('RestCard (PM-07b 9)', () => {
  const restPreviewFn = vi.fn();
  const takeRest = vi.fn();
  const restTaken = vi.fn();

  beforeEach(() => {
    restPreviewFn.mockReset();
    takeRest.mockReset();
    restTaken.mockReset();
    takeRest.mockResolvedValue([
      create(CharacterVitalsSchema, { characterId: 'ragna', name: 'Ragna', revision: 4 }),
    ]);
    TestBed.configureTestingModule({
      providers: [{ provide: ResourceClient, useValue: { restPreview: restPreviewFn, takeRest } }],
    });
  });

  afterEach(() => document.body.replaceChildren());

  function render(party: readonly VitalsVm[] = []) {
    const fixture = TestBed.createComponent(RestCard);
    document.body.appendChild(fixture.nativeElement);
    fixture.componentRef.setInput('campaignId', 'camp');
    fixture.componentRef.setInput('party', party);
    fixture.componentInstance.restTaken.subscribe(restTaken);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const settle = async () => {
      await fixture.whenStable();
      fixture.detectChanges();
      await fixture.whenStable();
    };
    const button = (name: string) =>
      Array.from(el.querySelectorAll('button')).find((b) => flat(b)?.endsWith(name))!;
    return { fixture, el, settle, button };
  }

  it('offers the two rests and asks nothing yet', () => {
    const { el, button } = render();
    expect(flat(el.querySelector('h2'))).toBe('Descanso');
    expect(button('Descanso curto')).toBeTruthy();
    expect(button('Descanso longo')).toBeTruthy();
    expect(el.querySelector('[role="alertdialog"]')).toBeNull();
    expect(restPreviewFn).not.toHaveBeenCalled();
  });

  it('asks the server what a long rest gives back, then asks in place with the list from the preview', async () => {
    restPreviewFn.mockResolvedValue(boardParty());
    const { el, settle, button } = render([
      pensantusVitals({
        characterId: 'kai',
        resources: [{ key: 'ki', namePt: 'Chi', total: 5, used: 1, recharge: 'short_rest' }],
      }),
    ]);
    button('Descanso longo').click();
    await settle();

    expect(restPreviewFn).toHaveBeenCalledWith('camp', RestKind.LONG);
    const dialog = el.querySelector('[role="alertdialog"]')!;
    expect(flat(dialog.querySelector('h3'))).toBe('Começar o descanso longo?');
    expect(dialog.getAttribute('aria-labelledby')).toBe('rest-title');
    expect(flat(dialog.querySelector('#rest-intro'))).toBe(
      'Pelo menos 8 horas. Volta, para quem estiver na campanha:',
    );
    const lines = Array.from(dialog.querySelectorAll('.back__line > span'), flat);
    expect(lines).toEqual([
      'Todos os PV voltam ao máximo (e a Ajuda acaba).',
      'Metade dos dados de vida gastos (no mínimo 1) voltam.',
      'Fúria: Ragna 3 de 3 · Cura pelas Mãos: Tavo 25 de 25 · Pontos de Feitiçaria: Nael 5 de 5 (o espaço criado some) · Inspiração de Bardo: Orla 3 de 3.',
      'Chi (Kai 5 de 5): voltam (também voltariam num descanso curto).',
      'espaços de magia: todos.',
    ]);
    expect(flat(button('Descansar'))).toContain('Descansar');
    expect(flat(button('Cancelar'))).toBe('Cancelar');
  });

  it('puts the focus ring on the rest that was tapped, which stays the one in the focus', async () => {
    restPreviewFn.mockResolvedValue(boardParty());
    const { el, settle, button } = render();
    const long = button('Descanso longo');
    long.click();
    await settle();
    expect(document.activeElement).toBe(long);
    expect(long.hasAttribute('data-ring')).toBe(true);
    expect(long.getAttribute('aria-expanded')).toBe('true');
    expect(button('Descanso curto').getAttribute('aria-expanded')).toBe('false');
    expect(el.querySelector('#rest-question')).not.toBeNull();
  });

  it('writes the short rest in its own words, without the long rest warning', async () => {
    restPreviewFn.mockResolvedValue(
      restPreviewResponse(
        [
          restPreview({
            characterId: 'kai',
            name: 'Kai',
            resources: [{ key: 'ki', namePt: 'Chi', spent: 2, total: 5 }],
          }),
        ],
        true,
      ),
    );
    const { el, settle, button } = render();
    button('Descanso curto').click();
    await settle();
    expect(restPreviewFn).toHaveBeenCalledWith('camp', RestKind.SHORT);
    expect(flat(el.querySelector('h3'))).toBe('Começar o descanso curto?');
    expect(flat(el.querySelector('#rest-intro'))).toBe(
      'Pelo menos 1 hora. Volta, para quem estiver na campanha:',
    );
    expect(el.querySelector('.mr-notice--warning')).toBeNull();
  });

  it('warns above the list when a long rest was already taken in the session', async () => {
    restPreviewFn.mockResolvedValue(
      restPreviewResponse([restPreview({ characterId: 'a', name: 'Ana' })], true),
    );
    const { el, settle, button } = render();
    button('Descanso longo').click();
    await settle();
    const warning = el.querySelector('.mr-notice--warning')!;
    expect(flat(warning)).toContain(
      'Já houve um descanso longo nesta sessão. O SRD permite um a cada 24 horas de jogo: você decide.',
    );
    // Above the list.
    expect(
      warning.compareDocumentPosition(el.querySelector('.back')!) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });

  it('tells a character at 0 hit points that the long rest does nothing', async () => {
    restPreviewFn.mockResolvedValue(
      restPreviewResponse([
        restPreview({ characterId: 'a', name: 'Ana', noBenefit: true }),
        restPreview({ characterId: 'b', name: 'Beto' }),
      ]),
    );
    const { el, settle, button } = render();
    button('Descanso longo').click();
    await settle();
    expect(flat(el.querySelector('.back__line--none'))).toContain(
      'Ana está a 0 PV: o descanso longo não faz efeito.',
    );
  });

  it('says so when there is no living player character', async () => {
    restPreviewFn.mockResolvedValue(restPreviewResponse([]));
    const { el, settle, button } = render();
    button('Descanso longo').click();
    await settle();
    expect(flat(el.querySelector('.question__empty'))).toBe(
      'Nenhum personagem de jogador vivo na campanha.',
    );
  });

  it('closes on "Cancelar" and on Esc, takes no rest and gives the focus back to the rest button', async () => {
    restPreviewFn.mockResolvedValue(boardParty());
    const { el, settle, button, fixture } = render();
    const long = button('Descanso longo');
    long.click();
    await settle();
    button('Cancelar').click();
    await settle();
    expect(el.querySelector('[role="alertdialog"]')).toBeNull();
    expect(document.activeElement).toBe(long);

    long.click();
    await settle();
    el.querySelector('[role="alertdialog"]')!.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }),
    );
    await settle();
    fixture.detectChanges();
    expect(el.querySelector('[role="alertdialog"]')).toBeNull();
    expect(takeRest).not.toHaveBeenCalled();
  });

  it('takes the rest under one idempotency key, hands the new vitals to the page and says "Descanso longo feito."', async () => {
    restPreviewFn.mockResolvedValue(boardParty());
    const { el, settle, button } = render();
    button('Descanso longo').click();
    await settle();
    button('Descansar').click();
    await settle();

    expect(takeRest).toHaveBeenCalledTimes(1);
    const [campaignId, kind, key, choices] = takeRest.mock.calls[0];
    expect([campaignId, kind, choices]).toEqual(['camp', RestKind.LONG, []]);
    expect(key).toMatch(/^[0-9a-f-]{36}$/);
    expect(restTaken).toHaveBeenCalledTimes(1);
    expect(restTaken.mock.calls[0][0].map((v: VitalsVm) => [v.characterId, v.revision])).toEqual([
      ['ragna', 4],
    ]);
    expect(el.querySelector('[role="alertdialog"]')).toBeNull();
    expect(flat(el.querySelector('[role="status"]'))).toBe('Descanso longo feito.');
    expect(flat(el.querySelector('.rest__done'))).toContain('Descanso longo feito.');
  });

  it('says "Descanso curto feito." after a short rest', async () => {
    restPreviewFn.mockResolvedValue(
      restPreviewResponse([restPreview({ characterId: 'a', name: 'Ana' })]),
    );
    const { el, settle, button } = render();
    button('Descanso curto').click();
    await settle();
    button('Descansar').click();
    await settle();
    expect(takeRest.mock.calls[0][1]).toBe(RestKind.SHORT);
    expect(flat(el.querySelector('[role="status"]'))).toBe('Descanso curto feito.');
  });

  describe('food and drink (a long rest lowers exhaustion by one level)', () => {
    const toggle = (el: HTMLElement) =>
      el.querySelector('.question__food button[role="switch"]') as HTMLButtonElement | null;

    it('asks "Sem comida ou bebida" in a long rest, off, so food and drink is the default', async () => {
      restPreviewFn.mockResolvedValue(boardParty());
      const { el, settle, button } = render();
      button('Descanso longo').click();
      await settle();
      const sw = toggle(el)!;
      expect(sw.getAttribute('aria-checked')).toBe('false');
      expect(flat(el.querySelector('.question__food'))).toContain('Sem comida ou bebida');
      expect(flat(el.querySelector('.question__food'))).toContain(
        'Com comida e bebida, o descanso longo baixa 1 nível de exaustão de cada personagem.',
      );
      button('Descansar').click();
      await settle();
      expect(takeRest.mock.calls[0][4]).toBe(false);
    });

    it('sends without_food_or_drink when the table had none, and keeps its own key for it', async () => {
      restPreviewFn.mockResolvedValue(boardParty());
      takeRest.mockRejectedValueOnce(new ConnectError('lost', Code.Unavailable));
      const { el, settle, button } = render();
      button('Descanso longo').click();
      await settle();
      toggle(el)!.click();
      await settle();
      expect(toggle(el)!.getAttribute('aria-checked')).toBe('true');
      button('Descansar').click();
      await settle();
      expect(takeRest.mock.calls[0][4]).toBe(true);
      toggle(el)!.click();
      await settle();
      button('Descansar').click();
      await settle();
      expect(takeRest.mock.calls[1][4]).toBe(false);
      expect(takeRest.mock.calls[1][2]).not.toBe(takeRest.mock.calls[0][2]);
    });

    it('does not ask it in a short rest, which never takes a level off', async () => {
      restPreviewFn.mockResolvedValue(
        restPreviewResponse([restPreview({ characterId: 'a', name: 'Ana' })]),
      );
      const { el, settle, button } = render();
      button('Descanso curto').click();
      await settle();
      expect(toggle(el)).toBeNull();
      button('Descansar').click();
      await settle();
      expect(takeRest.mock.calls[0][4]).toBe(false);
    });

    it('starts the next long rest with the switch off again', async () => {
      restPreviewFn.mockResolvedValue(boardParty());
      const { el, settle, button } = render();
      button('Descanso longo').click();
      await settle();
      toggle(el)!.click();
      await settle();
      button('Cancelar').click();
      await settle();
      button('Descanso longo').click();
      await settle();
      expect(toggle(el)!.getAttribute('aria-checked')).toBe('false');
    });
  });

  it('sends the same key on a retry after a failure, and a new one for the next rest', async () => {
    restPreviewFn.mockResolvedValue(boardParty());
    takeRest.mockRejectedValueOnce(new ConnectError('lost', Code.Unavailable));
    const { el, settle, button } = render();
    button('Descanso longo').click();
    await settle();
    button('Descansar').click();
    await settle();
    expect(flat(el.querySelector('[role="alertdialog"] [role="alert"]'))).toContain(
      'Não deu para descansar',
    );
    button('Descansar').click();
    await settle();
    expect(takeRest.mock.calls[1][2]).toBe(takeRest.mock.calls[0][2]);

    button('Descanso longo').click();
    await settle();
    button('Descansar').click();
    await settle();
    expect(takeRest).toHaveBeenCalledTimes(3);
    expect(takeRest.mock.calls[2][2]).not.toBe(takeRest.mock.calls[0][2]);
  });

  it('never takes the rest twice on a double tap', async () => {
    restPreviewFn.mockResolvedValue(boardParty());
    let release!: () => void;
    takeRest.mockReturnValue(new Promise((resolve) => (release = () => resolve([]))));
    const { settle, button } = render();
    button('Descanso longo').click();
    await settle();
    const go = button('Descansar');
    go.click();
    go.click();
    release();
    await settle();
    expect(takeRest).toHaveBeenCalledTimes(1);
  });

  it('says the combat in the way, and keeps the question open for another try', async () => {
    restPreviewFn.mockResolvedValue(boardParty());
    takeRest.mockRejectedValueOnce(blocked(ResourceBlockedReason.COMBAT_OPEN));
    const { el, settle, button } = render();
    button('Descanso longo').click();
    await settle();
    button('Descansar').click();
    await settle();
    expect(flat(el.querySelector('[role="alertdialog"] [role="alert"]'))).toContain(
      'Há um combate em andamento: termine o combate antes de descansar.',
    );
    expect(restTaken).not.toHaveBeenCalled();
    expect(el.querySelector('[role="alertdialog"]')).not.toBeNull();
  });

  it('says the session is over when the preview finds no open session, with no question open', async () => {
    restPreviewFn.mockRejectedValue(
      new ConnectError('none', Code.FailedPrecondition, undefined, [
        {
          desc: GameSessionBlockedSchema,
          value: create(GameSessionBlockedSchema, {
            reason: GameSessionBlockedReason.NO_OPEN_SESSION,
          }),
        },
      ]),
    );
    const { el, settle, button } = render();
    button('Descanso curto').click();
    await settle();
    expect(flat(el.querySelector('[role="alert"]'))).toContain('A sessão acabou');
    expect(el.querySelector('[role="alertdialog"]')).toBeNull();
  });

  it('moves from one rest to the other without taking either', async () => {
    restPreviewFn.mockResolvedValue(boardParty());
    const { el, settle, button } = render();
    button('Descanso longo').click();
    await settle();
    button('Descanso curto').click();
    await settle();
    expect(flat(el.querySelector('h3'))).toBe('Começar o descanso curto?');
    expect(takeRest).not.toHaveBeenCalled();
  });

  describe('which hit dice come back on a long rest', () => {
    const multi = () =>
      restPreviewResponse([
        restPreview({
          characterId: 'fenn',
          name: 'Fenn',
          hitDiceSpent: [
            { faces: 10, count: 3 },
            { faces: 6, count: 2 },
          ],
          hitDiceBack: [{ faces: 10, count: 2 }],
          hitDiceBackLimit: 2,
        }),
        restPreview({
          characterId: 'ana',
          name: 'Ana',
          hitDiceSpent: [{ faces: 8, count: 2 }],
          hitDiceBack: [{ faces: 8, count: 1 }],
          hitDiceBackLimit: 1,
        }),
      ]);
    const stepper = (el: HTMLElement, label: string) =>
      el.querySelector<HTMLButtonElement>(`button[aria-label="${label}"]`)!;
    const valueOf = (el: HTMLElement, label: string) =>
      el.querySelector(`output[aria-label="${label}"]`)?.textContent?.trim();

    it('shows a stepper per size, only under the character that has a choice, starting at the default', async () => {
      restPreviewFn.mockResolvedValue(multi());
      const { el, settle, button } = render();
      button('Descanso longo').click();
      await settle();
      const groups = el.querySelectorAll('.choice');
      expect(groups).toHaveLength(1);
      expect(flat(groups[0].querySelector('.choice__title'))).toBe(
        'Fenn: dados de vida que voltam (no máximo 2 no total)',
      );
      expect(Array.from(groups[0].querySelectorAll('.choice__die'), flat)).toEqual(['d10', 'd6']);
      expect(valueOf(el, '2 d10 de Fenn voltam')).toBe('2');
      expect(valueOf(el, '0 d6 de Fenn voltam')).toBe('0');
    });

    it('lets the total reach the limit and no more', async () => {
      restPreviewFn.mockResolvedValue(multi());
      const { el, settle, button, fixture } = render();
      button('Descanso longo').click();
      await settle();
      // 2 of 2 already: no room for a d6.
      stepper(el, 'Devolver um d6 a mais para Fenn').click();
      await settle();
      expect(valueOf(el, '0 d6 de Fenn voltam')).toBe('0');
      // Giving one d10 up makes room for one d6.
      stepper(el, 'Devolver um d10 a menos de Fenn').click();
      await settle();
      stepper(el, 'Devolver um d6 a mais para Fenn').click();
      fixture.detectChanges();
      await settle();
      expect(valueOf(el, '1 d10 de Fenn voltam')).toBe('1');
      expect(valueOf(el, '1 d6 de Fenn voltam')).toBe('1');
    });

    it('sends the choices of who had one, with the rest', async () => {
      restPreviewFn.mockResolvedValue(multi());
      const { el, settle, button } = render();
      button('Descanso longo').click();
      await settle();
      stepper(el, 'Devolver um d10 a menos de Fenn').click();
      await settle();
      stepper(el, 'Devolver um d6 a mais para Fenn').click();
      await settle();
      button('Descansar').click();
      await settle();
      expect(takeRest.mock.calls[0][3]).toEqual([
        {
          characterId: 'fenn',
          dice: [
            { faces: 10, count: 1 },
            { faces: 6, count: 1 },
          ],
        },
      ]);
    });

    it('sends the default when nobody touched the steppers', async () => {
      restPreviewFn.mockResolvedValue(multi());
      const { settle, button } = render();
      button('Descanso longo').click();
      await settle();
      button('Descansar').click();
      await settle();
      expect(takeRest.mock.calls[0][3]).toEqual([
        {
          characterId: 'fenn',
          dice: [
            { faces: 10, count: 2 },
            { faces: 6, count: 0 },
          ],
        },
      ]);
    });

    it('asks nothing of the sort on a short rest', async () => {
      restPreviewFn.mockResolvedValue(multi());
      const { el, settle, button } = render();
      button('Descanso curto').click();
      await settle();
      expect(el.querySelector('.choice')).toBeNull();
    });
  });
});
