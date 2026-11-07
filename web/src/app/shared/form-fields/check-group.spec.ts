import { ChangeDetectionStrategy, Component, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';

import { CheckGroup, type CheckOption } from './check-group';
import { RadioCards } from './radio-cards';
import { SelectField } from './select-field';

const OPTIONS: CheckOption[] = [
  { value: 'attack', label: 'Jogada de ataque (o d20)' },
  { value: 'check', label: 'Teste de habilidade (o d20)' },
  { value: 'save', label: 'Teste de resistência (o d20)' },
  { value: 'damage', label: 'Dado de dano (de arma ou magia)' },
];

@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [CheckGroup, RadioCards, SelectField],
  template: `
    <app-check-group label="Vale nas jogadas" [options]="options" [values]="values()" path="p.rolls" [issues]="issues()" hint="A nota." (valuesChange)="values.set($event)" />
    <app-select-field label="Como muda" [options]="[{ value: 'set', label: 'Definir' }]" value="set" [disabled]="locked()" (valueChange)="values.set([$event])" />
    <app-radio-cards label="Vale para" [options]="[{ value: 'a', label: 'A' }]" value="a" hint="Só para a margem." />
  `,
})
class Host {
  readonly options = OPTIONS;
  readonly values = signal<string[]>(['save']);
  readonly issues = signal<string[]>([]);
  readonly locked = signal(true);
}

describe('CheckGroup (the rolls of a reroll)', () => {
  function setup() {
    const fixture = TestBed.createComponent(Host);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const boxes = () => Array.from(el.querySelectorAll<HTMLInputElement>('input[type=checkbox]'));
    return { fixture, el, boxes, host: fixture.componentInstance };
  }

  it('is a group named by its label, with a box for each option and the checked ones marked', () => {
    const { el, boxes } = setup();
    const group = el.querySelector('[role=group]')!;
    expect(group.getAttribute('aria-labelledby')).toBe(el.querySelector('.legend')!.id);
    expect(el.querySelector('.legend')!.textContent).toContain('Vale nas jogadas');
    expect(boxes().map((b) => b.checked)).toEqual([false, false, true, false]);
    expect(group.getAttribute('data-field')).toBe('p.rolls');
    expect(group.getAttribute('aria-describedby')).toBe(
      el.querySelector('app-field-note')!.firstElementChild!.id,
    );
    expect(el.textContent).toContain('A nota.');
  });

  it("gives back the checked keys in the list's order, whatever order they were checked in", () => {
    const { fixture, boxes, host } = setup();
    boxes()[3].click();
    fixture.detectChanges();
    expect(host.values()).toEqual(['save', 'damage']);
    boxes()[0].click();
    fixture.detectChanges();
    expect(host.values()).toEqual(['attack', 'save', 'damage']);
    boxes()[2].click();
    fixture.detectChanges();
    expect(host.values()).toEqual(['attack', 'damage']);
  });

  it('shows the refusal in place of the note, with the label in the danger ink and the alert icon', () => {
    const { fixture, el, host } = setup();
    host.issues.set(['Marque ao menos uma jogada.']);
    fixture.detectChanges();
    expect(el.querySelector('.legend--issue')).not.toBeNull();
    expect(el.querySelector('.note--issue mat-icon')).not.toBeNull();
    expect(el.textContent).toContain('Marque ao menos uma jogada.');
    expect(el.textContent).not.toContain('A nota.');
  });

  it('locks a select on its value and puts a hint under the radio cards', () => {
    const { fixture, el, host } = setup();
    expect(el.querySelector<HTMLSelectElement>('select')!.disabled).toBe(true);
    host.locked.set(false);
    fixture.detectChanges();
    expect(el.querySelector<HTMLSelectElement>('select')!.disabled).toBe(false);
    expect(el.querySelector('app-radio-cards')!.textContent).toContain('Só para a margem.');
  });
});
