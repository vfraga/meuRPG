import { TestBed } from '@angular/core/testing';

import { TvMessage } from './tv-message';

function render(inputs: Record<string, unknown>) {
  const fixture = TestBed.createComponent(TvMessage);
  Object.entries(inputs).forEach(([k, v]) => fixture.componentRef.setInput(k, v));
  fixture.detectChanges();
  return fixture.nativeElement as HTMLElement;
}

describe('TvMessage', () => {
  it('is the page heading by default, with the detail and the hint', () => {
    const el = render({
      icon: 'flag',
      title: 'A sessão 3 terminou às 23:40.',
      detail: 'Até a próxima sessão de Mirathel.',
      hint: 'O mestre pode fechar esta aba.',
    });
    expect(el.querySelector('h1')?.textContent).toBe('A sessão 3 terminou às 23:40.');
    expect(el.querySelector('.tv-msg__detail')?.textContent).toBe(
      'Até a próxima sessão de Mirathel.',
    );
    expect(el.querySelector('.tv-msg__hint')?.textContent).toBe('O mestre pode fechar esta aba.');
  });

  it('is a second-level heading under a header', () => {
    const el = render({ icon: 'cloud_off', title: 'Sem conexão desde 21:14', level: 2 });
    expect(el.querySelector('h1')).toBeNull();
    expect(el.querySelector('h2')?.textContent).toBe('Sem conexão desde 21:14');
    expect(el.querySelector('.tv-msg__detail')).toBeNull();
  });

  it('hides its icon from assistive technology', () => {
    expect(
      render({ icon: 'error_outline', title: 'x' })
        .querySelector('mat-icon')
        ?.getAttribute('aria-hidden'),
    ).toBe('true');
  });
});
