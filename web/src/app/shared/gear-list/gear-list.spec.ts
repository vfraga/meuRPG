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
  count: 1,
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

  it('shows two weapons of one key as one row with their count, and says it in the label', () => {
    const fixture = setup([{ ...sword, key: 'equipment:dagger', namePt: 'Adaga', count: 2 }]);
    const el = fixture.nativeElement as HTMLElement;
    expect(el.querySelectorAll('.gear-row')).toHaveLength(1);
    expect(el.querySelector('.gear-row__count')!.textContent).toContain('×2');
    fixture.componentRef.setInput('canChange', true);
    fixture.detectChanges();
    expect(el.querySelector('button')!.getAttribute('aria-label')).toContain('2 iguais');
  });

  it('draws the button of a row ("Guardar", "Usar") as an ink button, never the crimson of the primary action', () => {
    const fixture = setup([sword]);
    fixture.componentRef.setInput('canChange', true);
    fixture.detectChanges();
    const button = (fixture.nativeElement as HTMLElement).querySelector('.gear-row__button')!;
    expect(button.classList).toContain('mr-button--ink');
  });

  it('gives every row a "⋮" when asked, named like the sheet it opens', () => {
    const armor: GearVm = {
      ...sword,
      key: 'equipment:mail',
      namePt: 'Cota de malha',
      kind: 'armor',
    };
    const shield: GearVm = { ...sword, key: 'equipment:shield', namePt: 'Escudo', kind: 'shield' };
    const fixture = setup([sword, armor, shield]);
    fixture.componentRef.setInput('canMark', true);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const buttons = Array.from(el.querySelectorAll('.gear-row__more'));
    expect(buttons.map((b) => b.getAttribute('aria-label'))).toEqual([
      'Marcas · Espada longa',
      'Bônus · Cota de malha',
      'Bônus · Escudo',
    ]);
    buttons.forEach((b) => expect(b.getAttribute('aria-haspopup')).toBe('dialog'));
    const asked: GearVm[] = [];
    fixture.componentInstance.marksOpened.subscribe((g) => asked.push(g));
    (buttons[1] as HTMLButtonElement).click();
    expect(asked).toEqual([armor]);
  });

  it('draws each row as one line: marked for the layout checks, in the order of the board', () => {
    const fixture = setup([
      {
        ...sword,
        twoHanded: true,
        marks: { silvered: true, adamantine: false, magical: true },
        bonus: 1,
        bonusCounts: true,
      },
    ]);
    fixture.componentRef.setInput('canMark', true);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const row = el.querySelector('li.gear-row')!;
    expect(row.hasAttribute('data-one-line')).toBe(true);
    expect(row.hasAttribute('data-chip-row')).toBe(true);
    expect(el.querySelector('ul.gear-rows')!.getAttribute('data-equal-width')).toBe(
      '.gear-row__button',
    );
    expect(row.querySelector('.gear-row__name')!.hasAttribute('data-chip-ref')).toBe(true);
    const order = Array.from(
      row.querySelectorAll(
        '.gear-row__icon, .gear-row__name, .gear-row__kind, app-weapon-tags, app-bonus-chip, .gear-row__more, .gear-row__button',
      ),
    ).map((n) =>
      n.tagName.startsWith('APP-')
        ? n.tagName.toLowerCase()
        : n.classList[0] === 'mat-icon'
          ? 'gear-row__icon'
          : n.classList.contains('gear-row__button')
            ? 'gear-row__button'
            : n.classList[0],
    );
    expect(order).toEqual([
      'gear-row__icon',
      'gear-row__name',
      'gear-row__kind',
      'app-weapon-tags',
      'app-bonus-chip',
      'gear-row__more',
      'gear-row__button',
    ]);
  });

  it('puts the marks and the chips in one group after the name, so they wrap together', () => {
    const fixture = setup([
      {
        ...sword,
        twoHanded: true,
        marks: { silvered: true, adamantine: false, magical: false },
        bonus: 1,
        bonusCounts: true,
      },
    ]);
    fixture.componentRef.setInput('canMark', true);
    fixture.detectChanges();
    const row = (fixture.nativeElement as HTMLElement).querySelector('li.gear-row')!;
    const body = row.querySelector('.gear-row__body')!;
    const group = body.querySelector(':scope > .gear-row__marks')!;
    expect(group.hasAttribute('data-wrap-group')).toBe(true);
    expect(group.previousElementSibling!.classList).toContain('gear-row__kind');
    expect(group.nextElementSibling).toBeNull();
    expect(Array.from(group.children).map((c) => c.tagName.toLowerCase())).toEqual([
      'app-weapon-tags',
      'app-bonus-chip',
    ]);
    // The "⋮" follows the body in the markup, out of the flow of the lines.
    const more = row.querySelector('.gear-row__main > .gear-row__more')!;
    expect(more.previousElementSibling).toBe(body);
  });

  it('keeps the name as the first item of the body, before everything that can wrap', () => {
    const fixture = setup([{ ...sword, bonus: 2, bonusCounts: true }]);
    const body = (fixture.nativeElement as HTMLElement).querySelector('.gear-row__body')!;
    expect(body.firstElementChild!.classList).toContain('gear-row__name');
  });

  it('says the marks with icons only, and keeps each word for a screen reader', () => {
    const fixture = setup([
      { ...sword, marks: { silvered: true, adamantine: false, magical: false } },
    ]);
    const chip = (fixture.nativeElement as HTMLElement).querySelector('.weapon-tags__chip')!;
    expect(chip.hasAttribute('data-chip')).toBe(true);
    expect(chip.querySelector('.mr-visually-hidden')!.textContent!.trim()).toBe('Prateada');
    const visible = Array.from(chip.childNodes)
      .filter((n) => n.nodeType === Node.TEXT_NODE)
      .map((n) => n.textContent!.trim())
      .join('');
    expect(visible).toBe('');
  });

  it('lets a counting bonus take the place of the "Mágica" chip, and keeps a stored mark without one', () => {
    const marks = { silvered: true, adamantine: false, magical: true };
    const fixture = setup([
      { ...sword, marks, bonus: 1, bonusCounts: true },
      { ...sword, key: 'k2', namePt: 'Maça', marks, bonus: 1, bonusCounts: false },
      { ...sword, key: 'k3', namePt: 'Adaga', marks },
    ]);
    const words = Array.from(
      (fixture.nativeElement as HTMLElement).querySelectorAll('.gear-row'),
    ).map((row) =>
      Array.from(row.querySelectorAll('.weapon-tags__chip .mr-visually-hidden')).map((n) =>
        n.textContent!.trim(),
      ),
    );
    expect(words).toEqual([['Prateada'], ['Prateada', 'Mágica'], ['Prateada', 'Mágica']]);
  });

  it('shows the magic bonus chip only when the piece has one, dashed when it does not count', () => {
    const fixture = setup([
      { ...sword, bonus: 2, bonusCounts: true },
      { ...sword, key: 'k2', namePt: 'Maça', bonus: 1, bonusCounts: false },
      { ...sword, key: 'k3', namePt: 'Adaga' },
    ]);
    const el = fixture.nativeElement as HTMLElement;
    const rows = Array.from(el.querySelectorAll('.gear-row'));
    expect(rows[0].querySelector('.bonus-chip')).not.toBeNull();
    expect(rows[0].querySelector('.bonus-chip--idle')).toBeNull();
    expect(rows[1].querySelector('.bonus-chip--idle')).not.toBeNull();
    expect(rows[2].querySelector('app-bonus-chip')).toBeNull();
  });

  it('marks the shield "Não conta na CA" with a dashed chip when a two-handed weapon leaves it idle', () => {
    const shield: GearVm = {
      ...sword,
      key: 'equipment:shield',
      namePt: 'Escudo',
      kind: 'shield',
      bonus: 1,
      bonusCounts: true,
    };
    const fixture = setup([shield]);
    fixture.componentRef.setInput('shieldIdle', true);
    fixture.detectChanges();
    const row = (fixture.nativeElement as HTMLElement).querySelector('.gear-row')!;
    expect(row.querySelector('.gear-row__idle')!.textContent!.trim()).toBe('Não conta na CA');
    expect(row.querySelector('.bonus-chip--idle')).not.toBeNull();
  });

  it('says in words what drawing a two-handed weapon does to the shield and the armor class', () => {
    const bow: GearVm = {
      ...sword,
      key: 'equipment:longbow',
      namePt: 'Arco longo',
      equipped: false,
      twoHanded: true,
      armorClassIfToggled: 18,
    };
    const fixture = setup([bow]);
    fixture.componentRef.setInput('armorClass', 21);
    fixture.componentRef.setInput('warned', [bow.key]);
    fixture.detectChanges();
    const row = (fixture.nativeElement as HTMLElement).querySelector('.gear-row')!;
    const warn = row.querySelector('.gear-row__warn')!;
    expect(warn.getAttribute('aria-hidden')).toBe('true');
    const said = Array.from(row.querySelectorAll('span')).find(
      (n) => n.children.length === 0 && n.textContent!.includes('Sacar desativa o escudo'),
    )!;
    expect(said.classList).toContain('mr-visually-hidden');
    expect(said.textContent!.trim()).toBe(', Sacar desativa o escudo: CA 21 → 18');
    // the warning is read once: the button's name does not repeat it
    expect(row.querySelector('button')!.getAttribute('aria-label')).toBe(
      'Equipar CA 18 Arco longo',
    );
  });

  it('puts "duas mãos" after the name, aria-hidden, where it can be clipped before the name', () => {
    const fixture = setup([{ ...sword, twoHanded: true }]);
    const el = fixture.nativeElement as HTMLElement;
    const name = el.querySelector('.gear-row__name')!;
    const kind = el.querySelector('.gear-row__kind')!;
    expect(kind.textContent!.trim()).toBe('duas mãos');
    expect(kind.getAttribute('aria-hidden')).toBe('true');
    expect(name.compareDocumentPosition(kind) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    // a screen reader hears the kind label whether or not the words are drawn
    expect(el.querySelector('.gear-row__main')!.textContent).toContain('Arma de duas mãos');
  });

  it('leaves the hidden warning off the row in the combat fold, whose own line says it', () => {
    const bow: GearVm = {
      ...sword,
      key: 'equipment:longbow',
      namePt: 'Arco longo',
      equipped: false,
      twoHanded: true,
      armorClassIfToggled: 14,
    };
    const fixture = setup([bow]);
    fixture.componentRef.setInput('armorClass', 16);
    fixture.componentRef.setInput('warned', [bow.key]);
    fixture.detectChanges();
    const said = () => (fixture.nativeElement as HTMLElement).textContent!;
    expect(said()).toContain('Sacar desativa o escudo: CA 16 → 14');
    fixture.componentRef.setInput('combat', true);
    fixture.detectChanges();
    expect(said()).not.toContain('Sacar desativa o escudo');
    expect((fixture.nativeElement as HTMLElement).querySelector('.gear-row__warn')).not.toBeNull();
  });

  it('has no "duas mãos" in the combat fold', () => {
    const fixture = setup([{ ...sword, twoHanded: true }]);
    fixture.componentRef.setInput('combat', true);
    fixture.detectChanges();
    expect((fixture.nativeElement as HTMLElement).querySelector('.gear-row__kind')).toBeNull();
  });

  it('shows "CA N" on the "Equipado" head when asked, and the two headings start alike', () => {
    const fixture = setup([sword, { ...sword, key: 'k2', namePt: 'Maça', equipped: false }]);
    fixture.componentRef.setInput('armorClass', 21);
    const el = fixture.nativeElement as HTMLElement;
    expect(el.querySelector('.gear-group__ac')).toBeNull();
    fixture.componentRef.setInput('showArmorClass', true);
    fixture.detectChanges();
    const ac = el.querySelector('.gear-group__ac')!;
    expect(ac.textContent!.trim()).toBe('CA 21');
    expect(ac.getAttribute('aria-hidden')).toBe('true');
    const head = ac.parentElement!;
    const said = Array.from(head.querySelectorAll('span')).find(
      (n) => n.textContent!.trim() === 'Classe de armadura 21',
    )!;
    expect(said.classList).toContain('mr-visually-hidden');
    // the number is in the head of "Equipado", after its heading
    expect(head.firstElementChild!.tagName).toBe('H3');
    expect(head.firstElementChild!.textContent!.trim()).toBe('Equipado');
    expect(head.firstElementChild!.compareDocumentPosition(ac)).toBe(
      Node.DOCUMENT_POSITION_FOLLOWING,
    );
    expect(head.closest('section')!.querySelector('ul')!.contains(ac)).toBe(false);
    const titles = Array.from(el.querySelectorAll('h3'));
    expect(titles.map((h) => h.textContent!.trim())).toEqual(['Equipado', 'Carregado']);
    titles.forEach((h) => expect(h.querySelector('mat-icon')).toBeNull());
    // each heading is the first thing in its section (or in its head): nothing in front of it
    expect(head.firstElementChild).toBe(titles[0]);
    expect(titles[1].closest('section')!.firstElementChild).toBe(titles[1]);
  });

  it('has no "⋮" unless asked, and keeps it when the gear has no equip button (a locked sheet)', () => {
    const fixture = setup([sword]);
    expect(fixture.nativeElement.querySelector('.gear-row__more')).toBeNull();
    fixture.componentRef.setInput('canMark', true);
    fixture.componentRef.setInput('canChange', false);
    fixture.detectChanges();
    expect(fixture.nativeElement.querySelector('.gear-row__more')).not.toBeNull();
    expect(fixture.nativeElement.querySelector('.gear-row__button')).toBeNull();
  });

  it('keeps the armor class a two-handed weapon would give out of the button, in its label only', () => {
    const bow: GearVm = {
      ...sword,
      key: 'equipment:longbow',
      namePt: 'Arco longo',
      equipped: false,
      twoHanded: true,
      armorClassIfToggled: 16,
      count: 1,
    };
    const fixture = setup([bow]);
    fixture.componentRef.setInput('armorClass', 18);
    fixture.detectChanges();
    const button = (fixture.nativeElement as HTMLElement).querySelector('button')!;
    expect(button.textContent).not.toContain('CA');
    expect(button.textContent!.trim()).toBe('Equipar');
    expect(button.getAttribute('aria-label')).toBe('Equipar CA 16 Arco longo');
  });

  it('starts every accessible name with the text the button shows (axe label-content-name-mismatch)', () => {
    const armor: GearVm = {
      ...sword,
      key: 'equipment:chain-mail',
      namePt: 'Cota de malha',
      kind: 'armor',
      armorClassIfToggled: 16,
      count: 1,
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
