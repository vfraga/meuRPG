import { create } from '@bufbuild/protobuf';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  MapBlockedReason,
  MapBlockedSchema,
  type Map as MapMessage,
} from '../../../../gen/meurpg/maps/v1/maps_pb';
import { DungeonsClient } from '../../../core/maps/dungeons-client';
import { FakeDungeonsClient, roomsResponse } from '../../../core/maps/dungeons-testing';
import { mapMessage } from '../../../core/maps/maps-testing';
import { DungeonImage } from './dungeon-image';
import { nextTurn } from '../../../../test-settle';

const blocked = (reason: MapBlockedReason) =>
  new ConnectError('blocked', Code.FailedPrecondition, undefined, [
    { desc: MapBlockedSchema, value: create(MapBlockedSchema, { reason }) },
  ]);

describe('DungeonImage ("Imagem" and "Redesenhar", E10-05 5 and 6)', () => {
  let fixture: ComponentFixture<DungeonImage>;
  let el: HTMLElement;
  let api: FakeDungeonsClient;
  let redrawn: MapMessage[];
  let refused: number;

  async function setup(
    options: { generated?: boolean; prepare?: () => Promise<string | null> } = {},
  ) {
    api = new FakeDungeonsClient();
    redrawn = [];
    refused = 0;
    TestBed.configureTestingModule({ providers: [{ provide: DungeonsClient, useValue: api }] });
    fixture = TestBed.createComponent(DungeonImage);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput('map', mapMessage('dungeon-1', 'Masmorra de Mirathel'));
    fixture.componentRef.setInput(
      'info',
      roomsResponse({ imageIsGenerated: options.generated ?? true }),
    );
    if (options.prepare) {
      fixture.componentRef.setInput('prepare', options.prepare);
    }
    fixture.componentInstance.redrawn.subscribe((m) => redrawn.push(m));
    fixture.componentInstance.refused.subscribe(() => refused++);
    fixture.detectChanges();
    el = fixture.nativeElement;
    await fixture.whenStable();
  }

  const text = () => (el.textContent ?? '').replace(/\u00a0/g, ' ').replace(/\s+/g, ' ');
  const button = (label: string) =>
    Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
      b.textContent?.trim().endsWith(label),
    )!;
  const settle = async () => {
    fixture.detectChanges();
    await nextTurn();
    await fixture.whenStable();
    fixture.detectChanges();
    await nextTurn();
    fixture.detectChanges();
  };

  it('names the map, says it was made by the app, and gives the seed, the size and the doors', async () => {
    await setup();
    expect(el.querySelector('h2')?.textContent).toBe('Imagem');
    expect(text()).toContain('Masmorra de Mirathel');
    expect(text()).toContain('Gerada pelo app');
    expect(text()).toContain(
      'Semente 48213 · 11 × 9 quadrados (16,5 × 13,5 m) · 4 portas e 1 passagem',
    );
    expect(text()).toContain(
      'A imagem leva só chão e paredes. As portas e as escadas são desenhadas por cima e continuam editáveis.',
    );
  });

  it('asks in place: the title has the focus, "Voltar" first, and nothing is drawn before the second click', async () => {
    await setup();
    button('Redesenhar').click();
    await settle();
    expect(el.querySelector('h3')?.textContent).toContain('Redesenhar a imagem?');
    expect(document.activeElement).toBe(el.querySelector('h3'));
    expect(text()).toContain(
      'Faz a imagem de novo a partir das paredes e das portas de agora. O que você pintou continua.',
    );
    expect(text()).toContain('Se o mapa já foi mostrado, os jogadores veem a imagem nova na hora.');
    const buttons = Array.from(el.querySelectorAll('button')).map((b) => b.textContent?.trim());
    expect(buttons).toEqual(['Voltar', 'Redesenhar']);
    expect(api.calls).toEqual([]);
  });

  it('has no dangling aria id while the question is open, and keeps its live region in the page', async () => {
    await setup();
    expect(el.querySelector('section')?.getAttribute('aria-labelledby')).toBe('img-title');
    expect(el.querySelector('.img__done')?.classList).toContain('mr-visually-hidden');
    button('Redesenhar').click();
    await settle();
    const root = el.querySelector('section.mr-panel')!;
    expect(root.getAttribute('aria-labelledby')).toBeNull();
    expect(root.getAttribute('aria-label')).toBe('Redesenhar a imagem');
    expect(el.querySelector('.img__done')).toBeTruthy();
  });

  it('"Voltar" closes the question and gives the focus back to the button that asked', async () => {
    await setup();
    button('Redesenhar').click();
    await settle();
    button('Voltar').click();
    await settle();
    expect(el.querySelector('h3')).toBeNull();
    expect(document.activeElement).toBe(button('Redesenhar'));
    expect(api.calls).toEqual([]);
  });

  it('Esc closes the question like "Voltar"', async () => {
    await setup();
    button('Redesenhar').click();
    await settle();
    el.querySelector('section[role="group"]')!.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }),
    );
    await settle();
    expect(el.querySelector('h3')).toBeNull();
  });

  it('redraws on the second click: the map with its new image goes up, and the panel says the layers stay', async () => {
    await setup();
    button('Redesenhar').click();
    await settle();
    button('Redesenhar').click();
    await settle();
    expect(api.calls).toEqual(['redraw dungeon-1']);
    expect(redrawn).toEqual([api.redrawnMap]);
    expect(el.querySelector('h3')).toBeNull();
    expect(el.querySelector('[role="status"]')?.textContent).toContain(
      'As camadas, os pontos e os tokens continuam como estavam.',
    );
  });

  it('sends what the editor still holds first, and stops with the reason when it cannot', async () => {
    const order: string[] = [];
    await setup({
      prepare: async () => (order.push('prepare'), 'Há traços que o servidor ainda não recebeu.'),
    });
    button('Redesenhar').click();
    await settle();
    button('Redesenhar').click();
    await settle();
    expect(order).toEqual(['prepare']);
    expect(api.calls).toEqual([]);
    expect(el.querySelector('[role="alert"]')?.textContent).toContain(
      'Há traços que o servidor ainda não recebeu.',
    );
    // The question stays open, so the master can try again.
    expect(el.querySelector('h3')?.textContent).toContain('Redesenhar a imagem?');
  });

  describe('each refusal is said by its reason, above the question that stays open', () => {
    async function refuse(err: unknown) {
      await setup();
      api.failWith.set('redraw', err);
      button('Redesenhar').click();
      await settle();
      button('Redesenhar').click();
      await settle();
      return el.querySelector('[role="alert"]')?.textContent ?? '';
    }

    it("the image is no longer the dungeon's (IMAGE_CHANGED: the image, the size, the grid or the layers changed)", async () => {
      expect(await refuse(blocked(MapBlockedReason.IMAGE_CHANGED))).toContain(
        'já não é a masmorra que o app desenhou',
      );
      expect(refused).toBe(1);
      expect(redrawn).toEqual([]);
    });

    it('the grid was removed', async () => {
      expect(await refuse(blocked(MapBlockedReason.NO_GRID))).toContain('sem grade');
    });

    it('the walls or the doors changed while it drew', async () => {
      expect(await refuse(new ConnectError('x', Code.Aborted))).toContain('Tente de novo');
    });

    it('too many dungeons drawn a moment ago', async () => {
      expect(await refuse(new ConnectError('x', Code.ResourceExhausted))).toContain('15 segundos');
    });
  });

  it('offers no "Redesenhar" on a map whose image is no longer the generator\'s, and says so', async () => {
    await setup({ generated: false });
    expect(Array.from(el.querySelectorAll('button'))).toHaveLength(0);
    expect(text()).toContain('Imagem trocada');
    expect(text()).not.toContain('Gerada pelo app');
    expect(text()).toContain('não dá para redesenhar');
  });
});
