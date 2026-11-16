import { ChangeDetectionStrategy, Component, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';

import { type Segment, Segmented } from './segmented';

@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [Segmented],
  template: `<app-segmented label="Como se mover" [segments]="segments" [value]="value()" (choose)="picked.push($event)" />`,
})
class Host {
  readonly value = signal<'a' | 'b'>('a');
  readonly picked: string[] = [];
  segments: readonly Segment<'a' | 'b'>[] = [
    { value: 'a', label: 'A' },
    { value: 'b', label: 'B', disabled: true },
  ];
}

describe('Segmented', () => {
  const radios = (host: HTMLElement) =>
    Array.from(host.querySelectorAll<HTMLInputElement>('input[type="radio"]'));

  it('disables an off choice that has no reason to read', () => {
    const fixture = TestBed.createComponent(Host);
    fixture.detectChanges();
    const [, b] = radios(fixture.nativeElement);
    expect(b.disabled).toBe(true);
  });

  it('keeps an off choice with a reason focusable, reads the reason and never takes the choice', () => {
    const fixture = TestBed.createComponent(Host);
    fixture.componentInstance.segments = [
      { value: 'a', label: 'A' },
      { value: 'b', label: 'B', disabled: true, describedBy: 'why' },
    ];
    fixture.detectChanges();
    const [a, b] = radios(fixture.nativeElement);
    expect(b.disabled).toBe(false);
    expect(b.getAttribute('aria-disabled')).toBe('true');
    expect(b.getAttribute('aria-describedby')).toBe('why');
    b.click();
    fixture.detectChanges();
    expect(fixture.componentInstance.picked).toEqual([]);
    expect(b.checked).toBe(false);
    a.click();
    expect(a.getAttribute('aria-disabled')).toBeNull();
  });

  it('draws the saddle of Cavalgar as a decoration, the words name it', () => {
    const fixture = TestBed.createComponent(Host);
    fixture.componentInstance.segments = [
      { value: 'a', label: 'Cavalgar', custom: 'saddle' },
      { value: 'b', label: 'Desmontar' },
    ];
    fixture.componentInstance.value.set('b');
    fixture.detectChanges();
    const label = (fixture.nativeElement as HTMLElement).querySelector('.seg__item')!;
    expect(label.querySelector('app-saddle-icon svg')?.getAttribute('aria-hidden')).toBe('true');
    expect(label.textContent?.trim()).toBe('Cavalgar');
  });

  it('draws the saddle at the size of the check, so a choice never changes the width', () => {
    const fixture = TestBed.createComponent(Host);
    fixture.componentInstance.segments = [
      { value: 'a', label: 'A' },
      { value: 'b', label: 'B', custom: 'saddle' },
    ];
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const saddle = el.querySelector('app-saddle-icon')!;
    const check = el.querySelector('mat-icon')!;
    expect(getComputedStyle(saddle).width).toBe('20px');
    expect(getComputedStyle(check).width).toBe('20px');
  });
});
