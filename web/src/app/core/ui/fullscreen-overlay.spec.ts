import { OverlayContainer } from '@angular/cdk/overlay';
import { TestBed } from '@angular/core/testing';

import { FullscreenOverlay } from './fullscreen-overlay';
import { stubFullscreen } from './fullscreen-testing';

describe('FullscreenOverlay', () => {
  let fullscreen: ReturnType<typeof stubFullscreen>;

  beforeEach(() => {
    fullscreen = stubFullscreen(true);
  });
  afterEach(() => fullscreen.restore());

  it('moves the dialogs’ container into the full-screen element and back to the body when it ends', () => {
    TestBed.inject(FullscreenOverlay);
    const container = TestBed.inject(OverlayContainer).getContainerElement();
    expect(container.parentElement).toBe(document.body);

    const map = document.body.appendChild(document.createElement('div'));
    fullscreen.enter(map);
    expect(container.parentElement).toBe(map);

    fullscreen.leave();
    expect(container.parentElement).toBe(document.body);
    map.remove();
  });

  it('puts a container created while the map is already on full screen inside it too', () => {
    TestBed.inject(FullscreenOverlay);
    const map = document.body.appendChild(document.createElement('div'));
    fullscreen.enter(map);
    // The first dialog of the session opens now: the container is created after the change.
    const container = TestBed.inject(OverlayContainer).getContainerElement();
    fullscreen.leave();
    fullscreen.enter(map);
    expect(container.parentElement).toBe(map);
    fullscreen.leave();
    map.remove();
  });

  it('stops following when the app is destroyed', () => {
    TestBed.inject(FullscreenOverlay);
    const container = TestBed.inject(OverlayContainer).getContainerElement();
    TestBed.resetTestingModule();
    const map = document.body.appendChild(document.createElement('div'));
    fullscreen.enter(map);
    expect(container.parentElement).not.toBe(map);
    fullscreen.leave();
    map.remove();
  });
});
