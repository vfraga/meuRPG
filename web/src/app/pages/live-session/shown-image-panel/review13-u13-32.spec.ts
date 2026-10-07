// Finding U13-32 (review/unit-13-web-live-rest.md): "Tirar" has no in-flight guard, so a double click sends two takeBackLeftImage calls and shows a false NotFound error.
import { TestBed } from '@angular/core/testing';
import { Code, ConnectError } from '@connectrpc/connect';

import { GalleryClient } from '../../../core/images/gallery-client';
import { LiveSessionSource, ShownImageVm } from '../live-session.types';
import { ShownImagePanel } from './shown-image-panel';

describe('Review13 U13-32: double click on "Tirar" takes the image back once', () => {
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

  async function render() {
    let calls = 0;
    const source = {
      setShownImage: vi.fn(() => Promise.resolve(null)),
      // First call succeeds, any later one is NotFound (the image is already back).
      takeBackLeftImage: vi.fn(() => {
        calls++;
        return calls === 1
          ? Promise.resolve()
          : Promise.reject(new ConnectError('not found', Code.NotFound));
      }),
    };
    TestBed.configureTestingModule({
      providers: [
        { provide: LiveSessionSource, useValue: source },
        { provide: GalleryClient, useValue: { list: () => Promise.resolve({ images: [] }) } },
      ],
    });
    const fixture = TestBed.createComponent(ShownImagePanel);
    fixture.componentRef.setInput('campaignId', 'c1');
    fixture.componentRef.setInput('shown', carta);
    fixture.componentRef.setInput('left', [planta] as ShownImageVm[]);
    await fixture.whenStable();
    const el = fixture.nativeElement as HTMLElement;
    const button = el.querySelector<HTMLButtonElement>(
      'button[aria-label="Tirar Planta da torre dos jogadores"]',
    );
    let leftChanged = 0;
    fixture.componentInstance.leftChanged.subscribe(() => leftChanged++);
    return {
      fixture,
      el,
      button: button as HTMLButtonElement,
      source,
      leftChanged: () => leftChanged,
    };
  }

  it('control: a single click takes it back with no error', async () => {
    const { fixture, el, button, source, leftChanged } = await render();
    button.click();
    await fixture.whenStable();
    expect(source.takeBackLeftImage).toHaveBeenCalledTimes(1);
    expect(el.querySelector('[role="alert"]')).toBeNull();
    expect(el.textContent).not.toContain('já não estava com os jogadores');
    expect(leftChanged()).toBe(0);
  });

  it('two clicks before the first answer make one call and show no error', async () => {
    const { fixture, el, button, source, leftChanged } = await render();
    button.click();
    button.click();
    await fixture.whenStable();
    expect(source.takeBackLeftImage).toHaveBeenCalledTimes(1);
    expect(el.textContent).not.toContain('Essa imagem já não estava com os jogadores.');
    expect(leftChanged()).toBe(0);
  });
});
