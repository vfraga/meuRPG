import { Component } from '@angular/core';
import { TestBed } from '@angular/core/testing';

import { ChosenRow } from './chosen-row';

@Component({
  imports: [ChosenRow],
  template: `
    <app-chosen-row icon="swords" caption="Ataque preparado">
      <b>Espada longa</b> no Ogro
      <button action type="button">Trocar</button>
    </app-chosen-row>
  `,
})
class Host {}

describe('ChosenRow', () => {
  it('draws the icon, the caption over what was chosen and the projected action at the end', () => {
    const fixture = TestBed.createComponent(Host);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    expect(el.querySelector('mat-icon')?.textContent).toBe('swords');
    expect(el.querySelector('.chosen__cap')?.textContent).toBe('Ataque preparado');
    expect(el.querySelector('.chosen__text')?.textContent?.replace(/\s+/g, ' ')).toContain(
      'Espada longa no Ogro',
    );
    expect(el.querySelector('.chosen > button')?.textContent).toBe('Trocar');
  });
});
