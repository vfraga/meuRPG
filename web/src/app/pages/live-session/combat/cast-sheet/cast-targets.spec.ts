import { ChangeDetectionStrategy, Component, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';

import type { CastTargetRow, Dealt, TargetRule } from '../../../../core/combat/cast-flow';
import { CastTargets } from './cast-targets';

const row = (id: string, label: string, blocked = ''): CastTargetRow => ({
  id,
  label,
  sub: '',
  blocked,
  cover: '',
  coverMark: null,
});

@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [CastTargets],
  template: `<app-cast-targets
    [rows]="rows"
    [rule]="rule"
    [dealt]="dealt()"
    [total]="3"
    (deal)="deals.push($event)"
  />`,
})
class Host {
  readonly rows = [row('a', 'Alfa'), row('b', 'Beta', 'Longe demais')];
  readonly rule: TargetRule = { kind: 'darts', max: 3, min: 1 };
  readonly dealt = signal<Dealt>(new Map());
  readonly deals: { id: string; delta: 1 | -1 }[] = [];
}

describe('CastTargets: the dart steppers', () => {
  function mount(dealt: Dealt = new Map()) {
    const fixture = TestBed.createComponent(Host);
    fixture.componentInstance.dealt.set(dealt);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const button = (label: string) =>
      el.querySelector(`button[aria-label="${label}"]`) as HTMLButtonElement;
    return { fixture, host: fixture.componentInstance, button };
  }

  it('keeps the "+" focusable at the last dart: aria-disabled, not natively disabled, and it places nothing', () => {
    const { fixture, host, button } = mount(new Map([['a', 3]]));
    const plus = button('Pôr um dardo em Alfa');
    expect(plus.disabled).toBe(false);
    expect(plus.getAttribute('aria-disabled')).toBe('true');
    plus.focus();
    plus.click();
    fixture.detectChanges();
    expect(host.deals).toEqual([]);
    expect(document.activeElement).toBe(plus);
  });

  it('keeps the "−" focusable at no dart, and takes nothing', () => {
    const { host, button } = mount();
    const minus = button('Tirar um dardo de Alfa');
    expect(minus.disabled).toBe(false);
    expect(minus.getAttribute('aria-disabled')).toBe('true');
    minus.click();
    expect(host.deals).toEqual([]);
  });

  it('places and takes darts while they can', () => {
    const { host, button, fixture } = mount(new Map([['a', 1]]));
    expect(button('Pôr um dardo em Alfa').getAttribute('aria-disabled')).toBeNull();
    expect(button('Tirar um dardo de Alfa').getAttribute('aria-disabled')).toBeNull();
    button('Pôr um dardo em Alfa').click();
    button('Tirar um dardo de Alfa').click();
    fixture.detectChanges();
    expect(host.deals).toEqual([
      { id: 'a', delta: 1 },
      { id: 'a', delta: -1 },
    ]);
  });

  it('leaves the buttons of a creature that cannot be reached natively disabled: they never change by themselves', () => {
    const { button } = mount();
    expect(button('Pôr um dardo em Beta').disabled).toBe(true);
    expect(button('Tirar um dardo de Beta').disabled).toBe(true);
  });
});
