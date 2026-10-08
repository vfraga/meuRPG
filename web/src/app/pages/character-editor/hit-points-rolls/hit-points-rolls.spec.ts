import { ComponentFixture, TestBed } from '@angular/core/testing';

import { ROLL_DIE, RollDie } from '../../../core/dice/dice';
import { HitPointsRolls } from './hit-points-rolls';

function sequence(faces: number[]): RollDie {
  let i = 0;
  return () => faces[i++ % faces.length];
}

describe('HitPointsRolls (E6-21)', () => {
  function setup(rolls: number[] = [], faces: number[] = [8, 3, 5]) {
    TestBed.configureTestingModule({
      providers: [{ provide: ROLL_DIE, useValue: sequence(faces) }],
    });
    const fixture = TestBed.createComponent(HitPointsRolls);
    fixture.componentRef.setInput('hitDie', 12);
    fixture.componentRef.setInput('level', 3);
    fixture.componentRef.setInput('constitution', 16);
    fixture.componentRef.setInput('rolls', rolls);
    fixture.detectChanges();
    return { fixture, el: fixture.nativeElement as HTMLElement };
  }

  const button = (el: HTMLElement, text: string) =>
    Array.from(el.querySelectorAll('button')).find((b) =>
      b.textContent?.replace(/\s+/g, ' ').includes(text),
    )!;
  const click = (fixture: ComponentFixture<HitPointsRolls>, b: HTMLElement) => {
    b.click();
    fixture.detectChanges();
  };

  it('has a row per level from the 2nd, named with the die', () => {
    const { el } = setup();
    const labels = Array.from(el.querySelectorAll('mat-label')).map((l) => l.textContent?.trim());
    expect(labels).toEqual(['Nível 2 (1d12)', 'Nível 3 (1d12)']);
  });

  it('"Rolar" fills only its row, and the formula uses the real Constitution modifier', () => {
    const { fixture, el } = setup();
    click(fixture, button(el, 'Rolar o nível 2'));
    expect(fixture.componentInstance.rolls()).toEqual([8, 0]);
    expect(el.querySelector('.hp__formula')?.textContent?.replace(/\s+/g, ' ').trim()).toBe(
      '1d12 (8) + 3 (Constituição) = 11 PV',
    );
    expect(el.querySelector('.hp__sum')?.textContent?.replace(/\s+/g, ' ').trim()).toBe(
      '26 PV máximos até agora',
    );
  });

  it('shows what is missing and the range the final total can reach', () => {
    const { fixture, el } = setup();
    click(fixture, button(el, 'Rolar o nível 2'));
    const note = el.querySelector('.hp__note')?.textContent?.replace(/\s+/g, ' ');
    expect(note).toContain('Falta rolar o nível 3.');
    expect(note).toContain('entre 30 e 41 PV');
    expect(note).toContain('Constituição 16 (+3 por nível)');
  });

  it('lists the levels still to roll the Portuguese way', () => {
    const { fixture, el } = setup([0, 0, 0]);
    fixture.componentRef.setInput('level', 4);
    fixture.detectChanges();
    const note = el.querySelector('.hp__note')?.textContent?.replace(/\s+/g, ' ');
    expect(note).toContain('Falta rolar os níveis 2, 3 e 4.');
  });

  it('"Rolar os níveis que faltam" rolls only the empty rows', () => {
    const { fixture, el } = setup([7, 0]);
    click(fixture, button(el, 'Rolar os níveis que faltam'));
    expect(fixture.componentInstance.rolls()).toEqual([7, 8]);
    expect(button(el, 'Rolar os níveis que faltam').disabled).toBe(true);
  });

  it('accepts a roll typed in, and ignores one that does not fit the die', () => {
    const { fixture, el } = setup();
    const input = el.querySelector<HTMLInputElement>('input')!;
    input.value = '9';
    input.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    expect(fixture.componentInstance.rolls()).toEqual([9, 0]);
    input.value = '20';
    input.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    expect(el.querySelector('.hp__sum')?.textContent?.replace(/\s+/g, ' ').trim()).toBe(
      '15 PV máximos até agora',
    );
  });

  it('labels itself a preview', () => {
    const { el } = setup();
    expect(el.querySelector('.hp__note')?.textContent).toContain('É uma prévia');
  });

  it('says the bonuses of the race, the class and the features are left out of the preview, and added when saved', () => {
    const { el } = setup();
    const note = el.querySelector('.hp__note')?.textContent?.replace(/\s+/g, ' ');
    expect(note).toContain(
      'não conta os pontos de vida que a raça, a classe ou uma característica somam',
    );
    expect(note).toContain('entram na ficha ao salvar');
  });
});
