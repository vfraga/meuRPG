import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';

import { WeaponMarksSheet, type WeaponMarksData } from './weapon-marks-sheet';

const none = { silvered: false, adamantine: false, magical: false };

function setup(over: Partial<WeaponMarksData> = {}) {
  const closed: (boolean | undefined)[] = [];
  const saves: unknown[] = [];
  const data: WeaponMarksData = {
    weaponName: 'Espada curta',
    characterName: 'Tor',
    isMaster: false,
    marks: { ...none, silvered: true },
    save: (m) => {
      saves.push(m);
      return Promise.resolve('');
    },
    ...over,
  };
  TestBed.configureTestingModule({
    providers: [
      { provide: MAT_DIALOG_DATA, useValue: data },
      {
        provide: MatDialogRef,
        useValue: { close: (r?: boolean) => closed.push(r), disableClose: false },
      },
    ],
  });
  const fixture = TestBed.createComponent(WeaponMarksSheet);
  fixture.detectChanges();
  const el = fixture.nativeElement as HTMLElement;
  const box = (word: string) =>
    Array.from(el.querySelectorAll('.marks__row'))
      .find((r) => r.querySelector('.marks__word')!.textContent === word)!
      .querySelector('input')!;
  const text = (sel: string) => el.querySelector(sel)!.textContent!.replace(/\s+/g, ' ').trim();
  const press = (label: string) =>
    Array.from(el.querySelectorAll('button'))
      .find((b) => b.textContent!.includes(label))!
      .click();
  return { fixture, el, box, text, press, saves, closed };
}

describe('WeaponMarksSheet ("Marcas da <arma>")', () => {
  it('is titled with the weapon and says "A arma é:"', () => {
    const { el, text } = setup();
    expect(el.querySelector('h2')!.textContent).toBe('Marcas da Espada curta');
    expect(text('.marks__intro')).toBe('A arma é:');
  });

  it('has one row for each mark: a checkbox, its icon, the word and what it beats', () => {
    const { el } = setup();
    const rows = Array.from(el.querySelectorAll('.marks__row')).map((r) => ({
      word: r.querySelector('.marks__word')!.textContent,
      beats: r.querySelector('.marks__beats')!.textContent,
      icon: r.querySelector('mat-icon')!.textContent,
    }));
    expect(rows).toEqual([
      { word: 'Prateada', beats: 'vence “nem prateado”', icon: 'dark_mode' },
      { word: 'Adamantina', beats: 'vence “nem adamantino”', icon: 'diamond' },
      {
        word: 'Mágica',
        beats: 'vence os três: “não mágico”, “nem prateado”, “nem adamantino”',
        icon: 'auto_awesome',
      },
    ]);
  });

  it('starts from the weapon’s marks and saves what was toggled', async () => {
    const { box, press, saves, closed, fixture } = setup();
    expect(box('Prateada').checked).toBe(true);
    expect(box('Mágica').checked).toBe(false);
    box('Mágica').checked = true;
    box('Mágica').dispatchEvent(new Event('change'));
    press('Salvar marcas');
    await fixture.whenStable();
    expect(saves).toEqual([{ silvered: true, adamantine: false, magical: true }]);
    expect(closed).toEqual([true]);
  });

  it('says the refusal and stays open with the toggles as they were', async () => {
    const { el, box, press, closed, fixture } = setup({
      save: () => Promise.resolve('A ficha mudou enquanto você editava.'),
    });
    box('Adamantina').checked = true;
    box('Adamantina').dispatchEvent(new Event('change'));
    press('Salvar marcas');
    await fixture.whenStable();
    fixture.detectChanges();
    expect(el.querySelector('[role="alert"]')!.textContent).toContain(
      'A ficha mudou enquanto você editava.',
    );
    expect(box('Adamantina').checked).toBe(true);
    expect(closed).toEqual([]);
  });

  it('tells the owner when the sheet locks, and the master whose sheet it is', () => {
    expect(setup().text('.marks__lock')).toContain(
      'O dono só marca enquanto a ficha não trava. Depois, só o mestre.',
    );
    TestBed.resetTestingModule();
    const m = setup({ isMaster: true });
    expect(m.text('.marks__lock')).toContain(
      'Muda a ficha de Tor: depois da trava, só você muda as marcas.',
    );
    expect(m.text('.marks__intro')).toBe('Tor · a arma é:');
  });

  it('has "Salvar marcas" and "Cancelar" of one size in a dialog, and "Cancelar" closes without saving', () => {
    const { el, press, saves, closed } = setup();
    const buttons = Array.from(el.querySelectorAll<HTMLButtonElement>('.pair__btn'));
    expect(buttons.map((b) => b.textContent!.trim())).toEqual(['saveSalvar marcas', 'Cancelar']);
    expect(buttons.every((b) => b.classList.contains('pair__btn'))).toBe(true);
    press('Cancelar');
    expect(saves).toEqual([]);
    expect(closed).toEqual([undefined]);
  });
});
