import { TestBed } from '@angular/core/testing';

import type { VitalsVm } from '../../../core/play/live-session.types';
import { pensantusVitals } from '../../../core/play/live-testing';
import { ResourceCounters } from './resource-counters';

describe('ResourceCounters (PM-07b 7)', () => {
  /** The text of each block of a box or a row, apart: the browser draws them as separate lines. */
  const flat = (n: Element | null | undefined) =>
    n
      ? Array.from(n.children)
          .map((c) => c.textContent?.replace(/\s+/g, ' ').trim())
          .filter(Boolean)
          .join(' ')
      : undefined;

  function render(vitals: VitalsVm) {
    const fixture = TestBed.createComponent(ResourceCounters);
    fixture.componentRef.setInput('vitals', vitals);
    fixture.detectChanges();
    return { fixture, el: fixture.nativeElement as HTMLElement };
  }

  const barbarian = () =>
    pensantusVitals({
      name: 'Ragna',
      spellSlots: [],
      resources: [{ key: 'rage', namePt: 'Fúria', total: 3, used: 1, recharge: 'long_rest' }],
    });

  it('draws a box for the resource: name, "2 de 3", the pips and when it comes back', () => {
    const { el } = render(barbarian());
    expect(el.querySelector('h2')?.textContent?.trim()).toBe('Recursos');
    const box = el.querySelector('.box');
    expect(flat(box)).toBe('Fúria 2 de 3 Volta num descanso longo');
    const pips = Array.from(box!.querySelectorAll('.pip'));
    expect(pips.map((p) => p.classList.contains('pip--left'))).toEqual([true, true, false]);
  });

  it('has no section for the slots when the character has none', () => {
    const { el } = render(barbarian());
    expect(el.textContent).not.toContain('Espaços de magia');
  });

  it('writes no recharge line for a resource that comes back another way', () => {
    const { el } = render(
      pensantusVitals({
        spellSlots: [],
        resources: [{ key: 'odd', namePt: 'Outro', total: 2, used: 0, recharge: 'dawn' }],
      }),
    );
    expect(el.querySelector('.box__again')).toBeNull();
  });

  it('shows only the number from 7 uses, and "ilimitado" for 99', () => {
    const { el } = render(
      pensantusVitals({
        spellSlots: [],
        resources: [
          { key: 'loh', namePt: 'Cura pelas Mãos', total: 25, used: 0, recharge: 'long_rest' },
          { key: 'rage', namePt: 'Fúria', total: 99, used: 0, recharge: 'long_rest' },
        ],
      }),
    );
    const boxes = Array.from(el.querySelectorAll('.box'));
    expect(flat(boxes[0])).toBe('Cura pelas Mãos 25 de 25 Volta num descanso longo');
    expect(flat(boxes[1])).toBe('Fúria ilimitado Volta num descanso longo');
    expect(el.querySelectorAll('.pip')).toHaveLength(0);
  });

  it('draws the slot rows with the footnote that applies', () => {
    const { el } = render(pensantusVitals());
    expect(el.querySelectorAll('h2')[0].textContent?.trim()).toBe('Espaços de magia');
    expect(Array.from(el.querySelectorAll('.row'), flat)).toEqual([
      '1º nível 2 de 4',
      '2º nível 2 de 2',
    ]);
    expect(el.querySelector('.block__note')?.textContent?.trim()).toBe(
      'Voltam num descanso longo.',
    );
    expect(el.querySelector('h2#resources-heading')).toBeNull();
  });

  it('says "criado" next to the count, and the warlock sentence for a character with pact slots', () => {
    const { el } = render(
      pensantusVitals({
        spellSlots: [{ level: 1, total: 3, used: 1, created: 1 }],
        pactSlots: { slotLevel: 2, total: 2, used: 0 },
      }),
    );
    expect(Array.from(el.querySelectorAll('.row'), flat)).toEqual([
      '1º nível 2 de 3 · criado',
      'Pacto · 2º nível 2 de 2',
    ]);
    expect(el.querySelector('.block__note')?.textContent).toContain(
      'O Bruxo recupera os espaços de pacto',
    );
  });

  it('is read-only: nothing in it takes focus', () => {
    const { el } = render(
      pensantusVitals({
        resources: [{ key: 'rage', namePt: 'Fúria', total: 3, used: 0, recharge: 'long_rest' }],
      }),
    );
    expect(el.querySelectorAll('button, a, input, select, textarea, [tabindex]')).toHaveLength(0);
  });

  it('follows the live numbers: a spent use changes the box', () => {
    const { fixture, el } = render(barbarian());
    fixture.componentRef.setInput(
      'vitals',
      pensantusVitals({
        spellSlots: [],
        resources: [{ key: 'rage', namePt: 'Fúria', total: 3, used: 3, recharge: 'long_rest' }],
      }),
    );
    fixture.detectChanges();
    expect(el.querySelector('.box__count')?.textContent?.trim()).toBe('0 de 3');
  });

  it('draws nothing for a character with no resource and no slots', () => {
    const { el } = render(pensantusVitals({ spellSlots: [], resources: [] }));
    expect(el.querySelector('section')).toBeNull();
  });
});
