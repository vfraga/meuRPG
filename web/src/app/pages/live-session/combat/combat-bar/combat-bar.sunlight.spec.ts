import { TestBed } from '@angular/core/testing';

import { EncounterStatus, SunlightScope } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { encounter } from '../../../../core/combat/combat-testing';
import { CombatBar } from './combat-bar';

function onPhone() {
  vi.stubGlobal('matchMedia', (query: string) => ({
    matches: query === '(max-width: 767.98px)',
    media: query,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
  }));
}

describe('CombatBar, the sun switch', () => {
  afterEach(() => vi.unstubAllGlobals());

  function setup(over: Parameters<typeof encounter>[0] = {}) {
    const fixture = TestBed.createComponent(CombatBar);
    fixture.componentRef.setInput('encounter', encounter(over));
    const asked = vi.fn();
    fixture.componentInstance.sunlight.subscribe(asked);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const sun = () => el.querySelector<HTMLButtonElement>('button[role="switch"]')!;
    return { fixture, sun, asked };
  }

  it('is a switch named "Luz do sol: desligada" while the combat has no sun', () => {
    const { sun } = setup();
    expect(sun().getAttribute('aria-checked')).toBe('false');
    expect(sun().getAttribute('aria-label')).toBe('Luz do sol: desligada');
  });

  it('says the whole map while the sun is on over it', () => {
    const { sun } = setup({ sunlightOn: true, sunlightScope: SunlightScope.MAP });
    expect(sun().getAttribute('aria-checked')).toBe('true');
    expect(sun().getAttribute('aria-label')).toBe('Luz do sol: todo o mapa');
  });

  it('says where the master paints while the sun is on over squares', () => {
    const { sun } = setup({ sunlightOn: true, sunlightScope: SunlightScope.SQUARES });
    expect(sun().getAttribute('aria-label')).toBe('Luz do sol: onde eu pintar');
  });

  it('asks for the sun when it is off and for none when it is on', () => {
    const off = setup();
    off.sun().click();
    expect(off.asked).toHaveBeenCalledWith(true);
    const on = setup({ sunlightOn: true, sunlightScope: SunlightScope.MAP });
    on.sun().click();
    expect(on.asked).toHaveBeenCalledWith(false);
  });

  it('is on the end-only bar of the tabbed combat too, beside "Encerrar combate"', () => {
    const fixture = TestBed.createComponent(CombatBar);
    fixture.componentRef.setInput(
      'encounter',
      encounter({ sunlightOn: true, sunlightScope: SunlightScope.MAP }),
    );
    fixture.componentRef.setInput('endOnly', true);
    const asked = vi.fn();
    fixture.componentInstance.sunlight.subscribe(asked);
    fixture.detectChanges();
    const sun = (fixture.nativeElement as HTMLElement).querySelector<HTMLButtonElement>(
      '.end-only__row button[role="switch"]',
    )!;
    expect(sun.getAttribute('aria-label')).toBe('Luz do sol: todo o mapa');
    sun.click();
    expect(asked).toHaveBeenCalledWith(false);
  });

  it('has a 44px target with the sun icon and a visible track and thumb', () => {
    const { sun } = setup();
    expect(sun().classList).toContain('sun');
    expect(sun().querySelector('mat-icon')).not.toBeNull();
    expect(sun().querySelector('.sun__track .sun__thumb')).not.toBeNull();
  });

  it('shows the on state by the filled sun and the class of the track, not only by colour', () => {
    const off = setup();
    expect(off.sun().querySelector('mat-icon')!.textContent).toContain('wb_twilight');
    expect(off.sun().classList).not.toContain('sun--on');
    const on = setup({ sunlightOn: true, sunlightScope: SunlightScope.MAP });
    expect(on.sun().querySelector('mat-icon')!.textContent).toContain('wb_sunny');
    expect(on.sun().classList).toContain('sun--on');
  });

  it('is not on the end-only bar on a phone: the combat header has the one switch', () => {
    onPhone();
    const fixture = TestBed.createComponent(CombatBar);
    fixture.componentRef.setInput('encounter', encounter({ sunlightOn: true }));
    fixture.componentRef.setInput('endOnly', true);
    fixture.detectChanges();
    expect((fixture.nativeElement as HTMLElement).querySelector('[role="switch"]')).toBeNull();
  });

  it('sits in the head of the setup bar on a phone, once', () => {
    onPhone();
    const { el } = (() => {
      const fixture = TestBed.createComponent(CombatBar);
      fixture.componentRef.setInput(
        'encounter',
        encounter({ status: EncounterStatus.SETUP, sunlightOn: true }),
      );
      fixture.detectChanges();
      return { el: fixture.nativeElement as HTMLElement };
    })();
    expect(el.querySelectorAll('[role="switch"]').length).toBe(1);
    expect(el.querySelector('.bar__head [role="switch"]')).not.toBeNull();
    expect(el.querySelector('.bar__actions [role="switch"]')).toBeNull();
  });
});
