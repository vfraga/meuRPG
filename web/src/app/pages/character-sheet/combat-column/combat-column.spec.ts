import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';

import { GearVm } from '../../../core/characters/gear';
import { AttackVm, FullSheetVm, ItemModifierVm } from '../character-sheet.types';
import { CombatColumn } from './combat-column';

const attack = (key: string, marks?: AttackVm['marks'], magicBonus = 0): AttackVm => ({
  key,
  namePt: key,
  kind: 'weapon',
  attackBonus: 5,
  damage: '1d8',
  damageTypePt: 'cortante',
  versatileLine: '',
  saveDc: 0,
  saveAbility: null,
  beams: 0,
  magicBonus,
  marks,
});

const mod = (
  target: string,
  sourceKey: string,
  over: Partial<ItemModifierVm> = {},
): ItemModifierVm => ({
  target,
  value: 1,
  sourceKey,
  sourceNamePt: sourceKey,
  counts: true,
  reasonPt: '',
  ...over,
});

const shield = (bonus?: number): GearVm =>
  ({ key: 'shield', namePt: 'Escudo', kind: 'shield', bonus }) as GearVm;

function setup(over: Partial<FullSheetVm>) {
  const sheet = signal({
    kind: 'full',
    attacks: [],
    gear: [],
    itemModifiers: [],
    shieldIdle: false,
    spellcasting: [],
    spells: [],
    spellSlots: [],
    pactSlots: null,
    grantedSpells: [],
    speedExtras: [],
    breathWeapon: '',
    ...over,
  } as unknown as FullSheetVm);
  const fixture = TestBed.createComponent(CombatColumn);
  fixture.componentRef.setInput('sheet', sheet());
  fixture.detectChanges();
  return fixture.nativeElement as HTMLElement;
}

describe('CombatColumn item modifiers', () => {
  it('shows no block without entries', () => {
    expect(setup({}).querySelector('.modifiers')).toBeNull();
  });

  it('groups the entries of a piece into one line and mutes one that does not count, with its reason once', () => {
    const el = setup({
      itemModifiers: [
        mod('ac', 'armor'),
        mod('attack:bow', 'bow', { counts: false, reasonPt: 'Carregado, não equipado.' }),
        mod('damage:bow', 'bow', { counts: false, reasonPt: 'Carregado, não equipado.' }),
      ],
    });
    expect(el.querySelector('.modifiers h3')!.textContent).toContain('Modificadores de itens');
    const items = Array.from(el.querySelectorAll('.modifiers__item'));
    expect(items.length).toBe(2);
    expect(items[0].querySelector('.modifiers__target')!.textContent).toBe('na CA');
    expect(items[1].querySelector('.modifiers__target')!.textContent).toBe('no ataque e no dano');
    expect(items[1].classList).toContain('modifiers__item--idle');
    expect(items[1].querySelectorAll('.modifiers__reason').length).toBe(1);
    expect(items[1].querySelector('.modifiers__reason')!.textContent).toContain(
      'Carregado, não equipado.',
    );
    expect(items[0].querySelector('.modifiers__reason')).toBeNull();
  });

  it('shows the idle shield note with the app words when the shield has no bonus', () => {
    const el = setup({ shieldIdle: true, gear: [shield()] });
    expect(el.querySelector('.shield-note')!.textContent).toContain(
      'O escudo não conta na CA: há uma arma de duas mãos equipada. Guarde a arma de duas mãos.',
    );
  });

  it('leaves the note out when the shield has a bonus, its line says why', () => {
    const el = setup({
      shieldIdle: true,
      gear: [shield(1)],
      itemModifiers: [
        mod('ac', 'shield', {
          counts: false,
          reasonPt: 'Uma arma de duas mãos equipada deixa o escudo parado.',
        }),
      ],
    });
    expect(el.querySelector('.shield-note')).toBeNull();
    expect(el.querySelector('.modifiers__reason')!.textContent).toContain('deixa o escudo parado');
  });
});

describe('CombatColumn "Ataques" +N chip', () => {
  const marks = { silvered: true, adamantine: false, magical: true };

  it('shows the +N of the field after the marks in place of "Mágica"', () => {
    const el = setup({ attacks: [attack('sword', marks, 2)] });
    const row = el.querySelector('.attacks__name')!;
    expect(row.hasAttribute('data-chip-row')).toBe(true);
    expect(row.querySelector('[data-chip-ref]')!.textContent).toContain('sword');
    expect(row.querySelector('.bonus-chip')!.getAttribute('aria-label')).toBe(
      'Bônus mágico +2, já somado',
    );
    expect(row.querySelector('app-bonus-chip')!.hasAttribute('data-chip')).toBe(true);
    expect(row.textContent).toContain('Prateada');
    expect(row.textContent).not.toContain('Mágica');
  });

  it('puts the marks before the chip in the row', () => {
    const el = setup({ attacks: [attack('sword', marks, 1)] });
    const kids = Array.from(el.querySelector('.attacks__name')!.children).map((c) =>
      c.tagName.toLowerCase(),
    );
    expect(kids).toEqual(['span', 'app-weapon-tags', 'app-bonus-chip']);
  });

  it('reads the field and nothing else: a gear or modifier bonus with a 0 field gives no chip', () => {
    const el = setup({
      attacks: [attack('bow', marks, 0)],
      gear: [{ key: 'bow', namePt: 'Arco', kind: 'weapon', bonus: 2, bonusCounts: true } as GearVm],
      itemModifiers: [mod('attack:bow', 'bow', { value: 2 })],
    });
    expect(el.querySelector('.attacks .bonus-chip')).toBeNull();
    expect(el.querySelector('.attacks__name')!.textContent).toContain('Mágica');
  });

  it('gives a second weapon without a bonus no chip and keeps its "Mágica"', () => {
    const el = setup({ attacks: [attack('sword', marks, 2), attack('dagger', marks, 0)] });
    const rows = el.querySelectorAll('.attacks__name');
    expect(rows[0].querySelector('.bonus-chip')).not.toBeNull();
    expect(rows[1].querySelector('.bonus-chip')).toBeNull();
    expect(rows[1].textContent).toContain('Mágica');
  });

  it('shows no chip on a spell row', () => {
    const el = setup({ attacks: [{ ...attack('fire-bolt'), kind: 'spell' }] });
    expect(el.querySelector('.attacks .bonus-chip')).toBeNull();
  });

  it('lists the improvised melee line of a bow as a weapon row of its own, with no ammunition count', () => {
    const el = setup({
      attacks: [
        { ...attack('equipment:longbow'), namePt: 'Arco longo' },
        {
          ...attack('equipment:longbow:melee'),
          namePt: 'Arco longo, corpo a corpo',
          attackBonus: 3,
          damage: '1d4+3',
          damageTypePt: 'concussão',
        },
      ],
    });
    const rows = Array.from(el.querySelectorAll('.attacks tbody tr')).map((r) =>
      (r.textContent ?? '').replace(/\s+/g, ' ').trim(),
    );
    expect(rows).toEqual([
      'Arco longo +5 1d8 cortante',
      'Arco longo, corpo a corpo +3 1d4+3 concussão',
    ]);
    expect(el.textContent).not.toMatch(/flecha|munição/i);
  });
});
