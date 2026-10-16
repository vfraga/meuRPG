import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { type MessageInitShape, create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  CharacterBlockedReason,
  CharacterBlockedSchema,
} from '../../../../gen/meurpg/characters/v1/characters_pb';
import {
  type RevivifyRequest,
  RevivifyRequestSchema,
  RevivifyRequestStatus,
} from '../../../../gen/meurpg/play/v1/revivify_pb';
import { RevivifyClient } from '../../../core/revivify/revivify-client';
import { RevivifyAsk } from './revivify-ask';
import { nextTurn } from '../../../../test-settle';

const asked = (id: string, over: MessageInitShape<typeof RevivifyRequestSchema> = {}) =>
  create(RevivifyRequestSchema, {
    id,
    status: RevivifyRequestStatus.PENDING,
    casterName: 'Ilaria',
    targetName: 'Toren',
    createdAt: timestampFromDate(new Date(2026, 9, 9, 10, 0, Number(id.replace(/\D/g, '')))),
    ...over,
  });

describe('RevivifyAsk', () => {
  let requests: RevivifyRequest[];
  let confirmFn: (pendingId: string, within: boolean) => Promise<unknown>;
  const confirms: Array<{ pendingId: string; within: boolean; key: string }> = [];
  const listed = vi.fn();

  beforeEach(() => {
    requests = [];
    confirms.length = 0;
    listed.mockClear();
    confirmFn = () => Promise.resolve(undefined);
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        {
          provide: RevivifyClient,
          useValue: {
            list: () => {
              listed();
              return Promise.resolve(requests);
            },
            confirmTime: (_c: string, pendingId: string, within: boolean, key: string) => {
              confirms.push({ pendingId, within, key });
              return confirmFn(pendingId, within);
            },
          },
        },
      ],
    });
  });

  async function open() {
    const fixture = TestBed.createComponent(RevivifyAsk);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.detectChanges();
    await nextTurn();
    await fixture.whenStable();
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const button = (text: string) =>
      Array.from(el.querySelectorAll('button')).find((b) => b.textContent?.trim() === text)!;
    return { fixture, el, button };
  }

  it('shows nothing while no cast waits', async () => {
    const { el } = await open();
    expect(el.querySelector('[role="alertdialog"]')).toBeNull();
  });

  it('asks in an alertdialog, in the words of the design, with the focus on the first button', async () => {
    requests = [asked('r1')];
    const { el, button } = await open();

    const dialog = el.querySelector('[role="alertdialog"]')!;
    expect(dialog.querySelector('h2')?.textContent?.trim()).toBe(
      'Ilaria quer conjurar Revivificar em Toren',
    );
    const text = dialog.textContent!.replace(/\s+/g, ' ');
    expect(text).toContain(
      'Toren morreu fora de combate. A magia só vale se ele morreu no último minuto. Nada foi gasto: Ilaria espera a sua resposta.',
    );
    expect(text).toContain(
      'Se você responde “Já passou”, nenhum espaço, ação ou diamante é gasto, e Ilaria lê só “O mestre disse que não dá”.',
    );
    expect(button('Faz menos de 1 minuto')).toBeTruthy();
    expect(button('Já passou')).toBeTruthy();
    expect(document.activeElement).toBe(button('Faz menos de 1 minuto'));
  });

  it('cannot be dismissed with Escape: the cast waits for an answer', async () => {
    requests = [asked('r1')];
    const { fixture, el } = await open();
    el.querySelector('[role="alertdialog"]')!.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }),
    );
    fixture.detectChanges();
    expect(el.querySelector('[role="alertdialog"]')).not.toBeNull();
    expect(confirms).toEqual([]);
  });

  it('answers "Faz menos de 1 minuto" with within_minute true, then reads the list again', async () => {
    requests = [asked('r1')];
    const { fixture, el, button } = await open();
    requests = [];
    button('Faz menos de 1 minuto').click();
    await nextTurn();
    fixture.detectChanges();

    expect(confirms).toHaveLength(1);
    expect(confirms[0]).toMatchObject({ pendingId: 'r1', within: true });
    expect(confirms[0].key).not.toBe('');
    expect(el.querySelector('[role="alertdialog"]')).toBeNull();
    expect(Array.from(el.querySelectorAll('[role="status"]')).map((n) => n.textContent)).toContain(
      'Toren voltou à vida',
    );
  });

  it('answers "Já passou" with within_minute false', async () => {
    requests = [asked('r1')];
    const { fixture, el, button } = await open();
    requests = [];
    button('Já passou').click();
    await nextTurn();
    fixture.detectChanges();

    expect(confirms[0]).toMatchObject({ pendingId: 'r1', within: false });
    expect(el.querySelector('[role="alertdialog"]')).toBeNull();
  });

  it('asks the oldest first, and the next one when it is answered', async () => {
    requests = [asked('r2', { casterName: 'Brisa', targetName: 'Nuvem' }), asked('r1')];
    const { fixture, el, button } = await open();
    expect(el.querySelector('h2')?.textContent).toContain('Ilaria quer conjurar');
    expect(el.textContent).toContain('Mais um pedido espera depois deste.');

    requests = [asked('r2', { casterName: 'Brisa', targetName: 'Nuvem' })];
    button('Já passou').click();
    await nextTurn();
    fixture.detectChanges();

    expect(el.querySelector('h2')?.textContent).toContain(
      'Brisa quer conjurar Revivificar em Nuvem',
    );
  });

  it('ignores a cast that was answered already', async () => {
    requests = [asked('r1', { status: RevivifyRequestStatus.CONFIRMED })];
    const { el } = await open();
    expect(el.querySelector('[role="alertdialog"]')).toBeNull();
  });

  it('reads the list again when the page says the casts changed', async () => {
    const { fixture, el } = await open();
    expect(el.querySelector('[role="alertdialog"]')).toBeNull();

    requests = [asked('r1')];
    fixture.componentRef.setInput('reload', 1);
    fixture.detectChanges();
    await nextTurn();
    fixture.detectChanges();

    expect(listed).toHaveBeenCalledTimes(2);
    expect(el.querySelector('[role="alertdialog"]')).not.toBeNull();
  });

  it("shows the card of the player's other living character when the server refuses, and the question comes back", async () => {
    requests = [asked('r1')];
    const { fixture, el, button } = await open();
    confirmFn = () =>
      Promise.reject(
        new ConnectError('blocked', Code.FailedPrecondition, undefined, [
          {
            desc: CharacterBlockedSchema,
            value: {
              reason: CharacterBlockedReason.LIVING_CHARACTER_EXISTS,
              characterId: 'nuvem-1',
              livingCharacterName: 'Nuvem',
            },
          },
        ]),
      );
    button('Faz menos de 1 minuto').click();
    await nextTurn();
    fixture.detectChanges();

    const card = el.querySelector('[role="alert"]')!;
    expect(card.textContent!.replace(/\s+/g, ' ')).toContain(
      'criou Nuvem depois da morte de Toren',
    );
    expect(card.querySelector('a')?.getAttribute('href')).toBe(
      '/campaigns/camp-1/characters/nuvem-1',
    );

    button('Cancelar').click();
    fixture.detectChanges();
    expect(el.querySelector('[role="alertdialog"]')).not.toBeNull();
  });

  it('reads the list again, without an error, when the cast was answered already', async () => {
    requests = [asked('r1')];
    const { fixture, el, button } = await open();
    confirmFn = () => Promise.reject(new ConnectError('answered', Code.FailedPrecondition));
    requests = [];
    button('Faz menos de 1 minuto').click();
    await nextTurn();
    fixture.detectChanges();

    expect(el.querySelector('[role="alertdialog"]')).toBeNull();
    expect(el.querySelector('.ask__error')).toBeNull();
  });

  it('says a failed call in words and keeps the question', async () => {
    requests = [asked('r1')];
    const { fixture, el, button } = await open();
    confirmFn = () => Promise.reject(new ConnectError('gone', Code.NotFound));
    button('Faz menos de 1 minuto').click();
    await nextTurn();
    fixture.detectChanges();

    expect(el.querySelector('.ask__error')?.textContent).toContain('não existe mais');
    expect(el.querySelector('[role="alertdialog"]')).not.toBeNull();
  });
});
