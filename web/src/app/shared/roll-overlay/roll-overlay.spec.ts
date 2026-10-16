import { TestBed } from '@angular/core/testing';
import { vi } from 'vitest';

import { RollAnimator, type RollShow, shapesOf } from './roll-animator';
import { RollOverlay } from './roll-overlay';

// With --coverage test-setup.ts only hooks the first file of a worker, so the stubbed globals this file leaves on would
// reach the next file (a real setTimeout never fires on a fake clock).
afterEach(() => {
  vi.unstubAllGlobals();
});

const attack: RollShow = {
  label: 'Ataque com Machado grande',
  dice: [{ sides: 20, face: 17 }],
  line: '17 + 5 = 22',
  outcome: { word: 'Acertou', good: true },
};

function setup(reduce = false) {
  vi.stubGlobal('matchMedia', (q: string) => ({ matches: reduce && q.includes('reduce') }));
  const fixture = TestBed.createComponent(RollOverlay);
  fixture.detectChanges();
  const animator = TestBed.inject(RollAnimator);
  const el = fixture.nativeElement as HTMLElement;
  const step = (ms: number) => {
    vi.advanceTimersByTime(ms);
    fixture.detectChanges();
  };
  return { fixture, animator, el, step };
}

describe('RollOverlay', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it('tumbles, lands on the real face and shows the outcome', () => {
    const { animator, el, step } = setup();
    animator.play(attack);
    step(0);
    expect(el.querySelector('.scrim')).toBeTruthy();
    expect(el.querySelector('.result')).toBeNull();
    expect(el.querySelector('[role=status]')?.textContent?.trim()).toBe('');
    step(700);
    expect(el.querySelector('.num')?.textContent?.trim()).toBe('17');
    expect(el.querySelector('.result')?.textContent).toContain('17 + 5 = 22');
    expect(el.querySelector('.result')?.textContent).toContain('Acertou');
    expect(el.querySelector('[role=status]')?.textContent).toContain(
      'Ataque com Machado grande: 17. 17 + 5 = 22. Acertou.',
    );
  });

  it('stays open for the table to read and closes by itself at 10 s', () => {
    const { animator, el, step } = setup();
    animator.play(attack);
    step(9_900);
    expect(el.querySelector('.scrim')).toBeTruthy();
    step(100);
    expect(el.querySelector('.scrim')).toBeNull();
  });

  it('closes at once on a click and on Esc', () => {
    const { animator, el, step } = setup();
    animator.play(attack);
    step(100);
    el.querySelector<HTMLElement>('.scrim')!.click();
    TestBed.inject(RollAnimator);
    expect(animator.current()).toBeNull();
    animator.play(attack);
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    expect(animator.current()).toBeNull();
  });

  it('replaces a roll that is playing', () => {
    const { animator, el, step } = setup();
    animator.play(attack);
    step(300);
    animator.play({ label: 'Teste de Sabedoria', dice: [{ sides: 20, face: 4 }] });
    step(700);
    expect(el.querySelector('.result')?.textContent).toContain('Teste de Sabedoria');
    expect(el.querySelectorAll('.die').length).toBe(1);
  });

  it('shows two d20 for advantage, the counting one highlighted', () => {
    const { animator, el, step } = setup();
    animator.play({
      label: 'Ataque',
      dice: [
        { sides: 20, face: 3, counts: false },
        { sides: 20, face: 15, counts: true },
      ],
    });
    step(700 + 80);
    expect(el.querySelectorAll('.die').length).toBe(2);
    expect(el.querySelectorAll('.die--dim').length).toBe(1);
  });

  it('draws a d6 damage roll as two d6 shapes that land one after another, with the sum line', () => {
    const { animator, el, step } = setup();
    animator.play({
      label: 'Dano',
      dice: [
        { sides: 6, face: 2 },
        { sides: 6, face: 5 },
      ],
      line: '2 + 5 + 3 = 10',
      note: 'de fogo',
    });
    step(700);
    expect(el.querySelectorAll('.die').length).toBe(2);
    expect(el.querySelectorAll('svg[data-die="d6"]').length).toBe(2);
    expect(el.querySelector('.result')).toBeNull();
    step(80);
    expect(el.querySelector('.result')?.textContent).toContain('2 + 5 + 3 = 10');
    expect(el.querySelector('.result')?.textContent).toContain('de fogo');
  });

  it('draws a d100 as two d10 with the tens and the units', () => {
    expect(shapesOf([{ sides: 100, face: 47 }]).map((s) => [s.shape, s.text])).toEqual([
      [10, '40'],
      [10, '7'],
    ]);
    expect(shapesOf([{ sides: 100, face: 100 }]).map((s) => s.text)).toEqual(['00', '0']);
    const { animator, el, step } = setup();
    animator.play({ label: 'Dado', dice: [{ sides: 100, face: 47 }] });
    step(700 + 80);
    expect(el.querySelectorAll('.die').length).toBe(2);
    expect(Array.from(el.querySelectorAll('.num')).map((n) => n.textContent?.trim())).toEqual([
      '40',
      '7',
    ]);
    expect(Array.from(el.querySelectorAll('.part')).map((n) => n.textContent?.trim())).toEqual([
      'dezenas',
      'unidades',
    ]);
  });

  it('shows at most eight dice and a +N chip for the rest', () => {
    const { animator, el, step } = setup();
    animator.play({
      label: 'Bola de Fogo',
      dice: Array.from({ length: 10 }, () => ({ sides: 6 as const, face: 3 })),
    });
    step(0);
    expect(el.querySelectorAll('.die').length).toBe(8);
    expect(el.querySelector('.more')?.textContent).toContain('+2');
  });

  it('does nothing when the browser asks for reduced motion', () => {
    const { animator, el, step } = setup(true);
    animator.play(attack);
    step(0);
    expect(animator.current()).toBeNull();
    expect(el.querySelector('.scrim')).toBeNull();
  });

  it('does nothing without matchMedia or without a mounted overlay', () => {
    vi.stubGlobal('matchMedia', undefined);
    const fixture = TestBed.createComponent(RollOverlay);
    fixture.detectChanges();
    const animator = TestBed.inject(RollAnimator);
    animator.play(attack);
    expect(animator.current()).toBeNull();
    fixture.destroy();
    vi.stubGlobal('matchMedia', () => ({ matches: false }));
    animator.play(attack);
    expect(animator.current()).toBeNull();
  });

  it('refuses a face the die does not have', () => {
    const { animator } = setup();
    animator.play({ label: 'x', dice: [{ sides: 6, face: 9 }] });
    expect(animator.current()).toBeNull();
  });
});
