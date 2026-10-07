// Finding U13-31 (review/unit-13-web-live-rest.md): the keep switch re-shows the image the panel still holds, even while "Parar de mostrar" is in flight.
import { TestBed } from '@angular/core/testing';

import { GalleryClient } from '../../../core/images/gallery-client';
import { LiveSessionSource, ShownImageVm } from '../live-session.types';
import { ShownImagePanel } from './shown-image-panel';

describe('Review13 U13-31: keep switch re-shows a withdrawn image', () => {
  const carta: ShownImageVm = {
    id: 'img-1',
    name: 'Capitão Goblin',
    width: 400,
    height: 500,
    url: '/images/img-1',
  };

  function setup() {
    const calls: Array<[string, string | null, boolean | undefined]> = [];
    let releaseStop: () => void = () => undefined;
    const source = {
      setShownImage: vi.fn((c: string, id: string | null, keep?: boolean) => {
        calls.push([c, id, keep]);
        if (id === null) {
          return new Promise<null>((resolve) => {
            releaseStop = () => resolve(null);
          });
        }
        return Promise.resolve(carta);
      }),
      takeBackLeftImage: vi.fn(() => Promise.resolve()),
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
    const el = fixture.nativeElement as HTMLElement;
    const stop = () =>
      [...el.querySelectorAll('button')].find((b) => b.textContent?.includes('Parar de mostrar'))!;
    const sw = () => el.querySelector<HTMLElement>('[role="switch"]')!;
    return { fixture, calls, stop, sw, release: () => releaseStop() };
  }

  it('control: the switch alone asks for keep on the shown image', async () => {
    const { fixture, calls, sw } = setup();
    await fixture.whenStable();
    sw().click();
    await fixture.whenStable();
    expect(calls).toEqual([['c1', 'img-1', true]]);
  });

  it('does not send the withdrawn image again when the switch is flipped during a stop', async () => {
    const { fixture, calls, stop, sw, release } = setup();
    await fixture.whenStable();
    stop().click(); // server is stopping; the page still shows img-1
    await fixture.whenStable();
    expect(calls).toEqual([['c1', null, undefined]]);
    sw().click();
    release();
    await fixture.whenStable();
    // Correct: nothing re-shows img-1 after the stop.
    expect(calls.filter(([, id]) => id === 'img-1')).toEqual([]);
  });
});
