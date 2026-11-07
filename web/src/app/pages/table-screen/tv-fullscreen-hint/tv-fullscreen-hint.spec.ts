import { TestBed } from '@angular/core/testing';

import { TvFullscreenHint } from './tv-fullscreen-hint';

describe('TvFullscreenHint', () => {
  it('is a button that asks for full screen when clicked', () => {
    const fixture = TestBed.createComponent(TvFullscreenHint);
    const asked = vi.fn();
    fixture.componentInstance.enter.subscribe(asked);
    fixture.detectChanges();
    const button = fixture.nativeElement.querySelector('button') as HTMLButtonElement;
    expect(button.textContent).toContain('Pressione F11 ou clique na tela');
    expect(button.textContent).toContain('para pôr em tela cheia');
    button.click();
    expect(asked).toHaveBeenCalledTimes(1);
  });
});
