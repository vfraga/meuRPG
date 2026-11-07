import { TestBed } from '@angular/core/testing';

import type { GearVm } from '../../core/characters/gear';
import { GearList } from './gear-list';

const sword: GearVm = {
  key: 'equipment:longsword',
  namePt: 'Espada longa',
  kind: 'weapon',
  equipped: true,
  twoHanded: false,
  armorClassIfToggled: 0,
};

describe('GearList', () => {
  function setup(gear: GearVm[]) {
    const fixture = TestBed.createComponent(GearList);
    fixture.componentRef.setInput('gear', gear);
    fixture.detectChanges();
    return fixture;
  }

  it('groups the pieces by their state: "Equipado" and "Carregado"', () => {
    const el = setup([sword, { ...sword, key: 'k2', namePt: 'Maça', equipped: false }])
      .nativeElement as HTMLElement;
    const groups = Array.from(el.querySelectorAll('section')).map((s) => ({
      title: s.querySelector('h3')!.textContent!.trim(),
      names: Array.from(s.querySelectorAll('.gear-row__name')).map((n) => n.textContent!.trim()),
    }));
    expect(groups).toEqual([
      { title: 'Equipado', names: ['Espada longa'] },
      { title: 'Carregado', names: ['Maça'] },
    ]);
  });

  it('shows the armor class a two-handed weapon would give, on its button and in its label', () => {
    const bow: GearVm = {
      ...sword,
      key: 'equipment:longbow',
      namePt: 'Arco longo',
      equipped: false,
      twoHanded: true,
      armorClassIfToggled: 16,
    };
    const fixture = setup([bow]);
    fixture.componentRef.setInput('armorClass', 18);
    fixture.detectChanges();
    const button = (fixture.nativeElement as HTMLElement).querySelector('button')!;
    expect(button.textContent).toContain('CA 16');
    expect(button.getAttribute('aria-label')).toBe('Equipar CA 16 Arco longo');
  });

  it('starts every accessible name with the text the button shows (axe label-content-name-mismatch)', () => {
    const armor: GearVm = {
      ...sword,
      key: 'equipment:chain-mail',
      namePt: 'Cota de malha',
      kind: 'armor',
      armorClassIfToggled: 16,
    };
    for (const combat of [false, true]) {
      const fixture = setup([armor, sword]);
      fixture.componentRef.setInput('armorClass', 18);
      fixture.componentRef.setInput('combat', combat);
      fixture.componentRef.setInput('cost', (g: GearVm) => (g.kind === 'armor' ? '' : 'grátis'));
      fixture.detectChanges();
      for (const b of Array.from(
        (fixture.nativeElement as HTMLElement).querySelectorAll<HTMLButtonElement>('button'),
      )) {
        const copy = b.cloneNode(true) as HTMLElement;
        copy.querySelectorAll('mat-icon').forEach((i) => i.remove());
        const visible = copy.textContent!.replace(/\s+/g, ' ').trim();
        expect(b.getAttribute('aria-label')!.startsWith(visible)).toBe(true);
      }
    }
  });

  it('labels each button with the action and the piece, and waits while a change is saved', () => {
    const fixture = setup([sword]);
    const el = fixture.nativeElement as HTMLElement;
    const button = el.querySelector<HTMLButtonElement>('button')!;
    expect(button.getAttribute('aria-label')).toBe('Guardar Espada longa');
    fixture.componentRef.setInput('busyKey', sword.key);
    fixture.detectChanges();
    expect(button.disabled).toBe(true);
  });

  it('moves the focus to the moved piece’s button when the call ends', async () => {
    const fixture = setup([sword]);
    const el = fixture.nativeElement as HTMLElement;
    el.querySelector<HTMLButtonElement>('button')!.click();
    fixture.componentRef.setInput('busyKey', sword.key);
    fixture.detectChanges();
    // the server's answer: the piece is carried now, and the call is over
    fixture.componentRef.setInput('gear', [{ ...sword, equipped: false }]);
    fixture.componentRef.setInput('busyKey', null);
    document.body.appendChild(el);
    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();
    expect(document.activeElement?.getAttribute('aria-label')).toBe('Equipar Espada longa');
    el.remove();
  });
});
