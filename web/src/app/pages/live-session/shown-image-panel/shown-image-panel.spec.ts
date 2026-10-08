import { TestBed } from '@angular/core/testing';
import { Code, ConnectError } from '@connectrpc/connect';

import { GalleryClient } from '../../../core/images/gallery-client';
import { mapMessage } from '../../../core/maps/maps-testing';
import { LiveSessionSource, ShownImageVm } from '../live-session.types';
import {
  ShownImagePanel,
  hiddenMapImages,
  showErrorMessage,
  shownImageNote,
  takeBackErrorMessage,
} from './shown-image-panel';

describe('shownImageNote', () => {
  it('says what the players see the first time, and what replaces what on a swap', () => {
    expect(shownImageNote({ name: 'Carta' }, null)).toBe(
      'Os jogadores veem a imagem na hora, com o nome dela como legenda. O mapa atual continua na tela deles.',
    );
    expect(shownImageNote({ name: 'Taverna do Javali' }, { name: 'Capitão Goblin' })).toBe(
      'Os jogadores passam a ver Taverna do Javali no lugar de Capitão Goblin.',
    );
  });
});

describe('hiddenMapImages', () => {
  it('lists the images that are the background of a hidden map, with the map', () => {
    const maps = [
      mapMessage('a', 'Mirathel', { revealed: true }),
      mapMessage('b', 'Covil dos goblins', { revealed: false }),
    ];
    expect([...hiddenMapImages(maps)]).toEqual([['img-b', 'Covil dos goblins']]);
  });
});

describe('showErrorMessage', () => {
  it('maps the codes of SetShownImage', () => {
    expect(showErrorMessage(new ConnectError('x', Code.FailedPrecondition))).toContain(
      'A sessão acabou',
    );
    expect(showErrorMessage(new ConnectError('x', Code.NotFound))).toContain(
      'não está mais na galeria',
    );
    expect(showErrorMessage(new ConnectError('x', Code.PermissionDenied))).toContain('Só o mestre');
    expect(showErrorMessage(new TypeError('Failed to fetch'))).toContain('Tente de novo');
  });

  it('tells the master a full gallery is why a fog map image cannot be shown', () => {
    const message = showErrorMessage(new ConnectError('full', Code.ResourceExhausted));
    expect(message).toContain('galeria da campanha está cheia');
    expect(message).not.toContain('falar com o servidor');
  });
});

describe('ShownImagePanel, "Deixar com os jogadores" (E6-25)', () => {
  const carta = {
    id: 'img-1',
    name: 'Capitão Goblin',
    width: 400,
    height: 500,
    url: '/images/img-1',
  };
  const planta = {
    id: 'img-2',
    name: 'Planta da torre',
    width: 800,
    height: 600,
    url: '/images/img-2',
  };
  const source = {
    setShownImage: vi.fn(() => Promise.resolve(null)),
    takeBackLeftImage: vi.fn(() => Promise.resolve()),
  };

  async function render(inputs: { keep?: boolean; left?: ShownImageVm[] }) {
    source.setShownImage.mockClear();
    source.takeBackLeftImage.mockClear();
    TestBed.configureTestingModule({
      providers: [
        { provide: LiveSessionSource, useValue: source },
        { provide: GalleryClient, useValue: { list: () => Promise.resolve({ images: [] }) } },
      ],
    });
    const fixture = TestBed.createComponent(ShownImagePanel);
    fixture.componentRef.setInput('campaignId', 'c1');
    fixture.componentRef.setInput('shown', carta);
    fixture.componentRef.setInput('keep', inputs.keep ?? false);
    fixture.componentRef.setInput('left', inputs.left ?? []);
    await fixture.whenStable();
    return { fixture, el: fixture.nativeElement as HTMLElement };
  }

  const click = async (fixture: { whenStable(): Promise<unknown> }, el: Element | null) => {
    (el as HTMLElement).click();
    await fixture.whenStable();
  };

  it('draws the switch off by default, as a switch with a word beside it', async () => {
    const { el } = await render({});
    const sw = el.querySelector('[role="switch"]');
    expect(sw?.getAttribute('aria-checked')).toBe('false');
    expect(el.querySelector('#keep-label')?.textContent).toContain('Deixar com os jogadores');
    expect(el.querySelector('.state')?.textContent?.trim()).toBe('Desligado');
    expect(el.querySelector('app-left-images-list')?.textContent?.trim() ?? '').toBe('');
  });

  it('asks the server for the new state with the image already shown', async () => {
    const { fixture, el } = await render({});
    const changes: boolean[] = [];
    fixture.componentInstance.keepChange.subscribe((v) => changes.push(v));
    await click(fixture, el.querySelector('[role="switch"]'));
    expect(source.setShownImage).toHaveBeenCalledWith('c1', 'img-1', true);
    expect(changes).toEqual([true]);
  });

  it('says the image stays when stopping with the switch on, and asks for the list again', async () => {
    const { fixture, el } = await render({ keep: true });
    let left = 0;
    fixture.componentInstance.leftChanged.subscribe(() => left++);
    const stop = [...el.querySelectorAll('button')].find((b) =>
      b.textContent?.includes('Parar de mostrar'),
    );
    await click(fixture, stop ?? null);
    expect(source.setShownImage).toHaveBeenCalledWith('c1', null);
    expect(left).toBe(1);
    expect(el.querySelector('[role="status"]')?.textContent).toContain(
      'Capitão Goblin continua com os jogadores.',
    );
  });

  it('lists the left images and takes one back at once, with a status', async () => {
    const { fixture, el } = await render({ left: [planta] });
    expect(el.querySelector('#left-heading')?.textContent).toContain('Deixadas com os jogadores');
    const taken: ShownImageVm[] = [];
    fixture.componentInstance.taken.subscribe((i) => taken.push(i));
    await click(
      fixture,
      el.querySelector('button[aria-label="Tirar Planta da torre dos jogadores"]'),
    );
    expect(source.takeBackLeftImage).toHaveBeenCalledWith('c1', 'img-2');
    expect(taken).toEqual([planta]);
    expect(el.querySelector('[role="status"]')?.textContent).toContain(
      'Planta da torre foi tirada.',
    );
  });

  it('does not show the withdrawn image again when the switch is flipped during a stop', async () => {
    const { fixture, el } = await render({});
    let release: () => void = () => undefined;
    source.setShownImage.mockImplementationOnce(
      () => new Promise<null>((resolve) => (release = () => resolve(null))),
    );
    const stop = [...el.querySelectorAll('button')].find((b) =>
      b.textContent?.includes('Parar de mostrar'),
    );
    await click(fixture, stop ?? null);
    expect(source.setShownImage).toHaveBeenCalledTimes(1);
    await click(fixture, el.querySelector('[role="switch"]'));
    release();
    await fixture.whenStable();
    expect(source.setShownImage.mock.calls).toEqual([['c1', null]]);
  });

  it('takes an image back once when "Tirar" is clicked twice before the answer', async () => {
    const { fixture, el } = await render({ left: [planta] });
    // The first call succeeds; a second one would find the image already back.
    source.takeBackLeftImage.mockResolvedValueOnce(undefined);
    source.takeBackLeftImage.mockRejectedValueOnce(new ConnectError('gone', Code.NotFound));
    let left = 0;
    fixture.componentInstance.leftChanged.subscribe(() => left++);
    const button = el.querySelector<HTMLElement>(
      'button[aria-label="Tirar Planta da torre dos jogadores"]',
    )!;
    button.click();
    button.click();
    await fixture.whenStable();
    expect(source.takeBackLeftImage).toHaveBeenCalledTimes(1);
    expect(el.textContent).not.toContain('já não estava com os jogadores');
    expect(left).toBe(0);
  });
});

describe('takeBackErrorMessage', () => {
  it('maps the codes of TakeBackLeftImage', () => {
    expect(takeBackErrorMessage(new ConnectError('x', Code.NotFound))).toContain('já não estava');
    expect(takeBackErrorMessage(new ConnectError('x', Code.PermissionDenied))).toContain(
      'Só o mestre',
    );
  });
});
