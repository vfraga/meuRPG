import { TestBed } from '@angular/core/testing';

import { SkillOptionVm } from '../character-editor.types';
import { SkillPicker } from './skill-picker';

const SKILLS: SkillOptionVm[] = [
  { key: 'skill:acrobatics', namePt: 'Acrobacia', ability: 'dex' },
  { key: 'skill:arcana', namePt: 'Arcanismo', ability: 'int' },
  { key: 'skill:history', namePt: 'História', ability: 'int' },
];

describe('SkillPicker', () => {
  function render(
    proficient: string[],
    expertise: string[] = [],
    granted: [string, string][] = [],
  ) {
    TestBed.configureTestingModule({ imports: [SkillPicker] });
    const fixture = TestBed.createComponent(SkillPicker);
    fixture.componentRef.setInput('skills', SKILLS);
    fixture.componentRef.setInput('proficient', new Set(proficient));
    fixture.componentRef.setInput('expertise', new Set(expertise));
    fixture.componentRef.setInput('granted', new Map(granted));
    fixture.detectChanges();
    return { fixture, el: fixture.nativeElement as HTMLElement };
  }

  it('shows each skill with its ability abbreviation, in a group named "Perícias"', () => {
    const { el } = render([]);

    const group = el.querySelector('[role="group"]');
    expect(group?.getAttribute('aria-label')).toBe('Perícias');
    expect(el.textContent).toContain('Acrobacia');
    expect(el.textContent).toContain('Des');
    expect(el.textContent).toContain('Nenhuma perícia marcada');
  });

  it('names each expertise box by its skill and only enables it on a proficient skill', () => {
    const { el } = render(['skill:arcana'], ['skill:arcana']);

    const expertise = Array.from(el.querySelectorAll<HTMLInputElement>('.skill__expertise input'));
    expect(expertise.map((i) => i.getAttribute('aria-label'))).toEqual([
      'Especialização em Acrobacia',
      'Especialização em Arcanismo',
      'Especialização em História',
    ]);
    expect(expertise.map((i) => i.disabled)).toEqual([true, false, true]);
    expect(el.textContent).toContain('1 perícia marcada, 1 com especialização');
  });

  it('reports clicks without changing anything itself', () => {
    const { fixture, el } = render([]);
    const toggled: string[] = [];
    fixture.componentInstance.toggleSkill.subscribe((key) => toggled.push(key));

    el.querySelector<HTMLInputElement>('.skill__name input')?.click();

    expect(toggled).toEqual(['skill:acrobatics']);
  });

  it('shows a skill the race or the background gives as taken and locked, with where it comes from, and leaves it out of the picks', () => {
    const { el } = render(
      ['skill:arcana'],
      [],
      [
        ['skill:history', 'do antecedente'],
        ['skill:acrobatics', 'da raça'],
      ],
    );

    const boxes = Array.from(el.querySelectorAll<HTMLInputElement>('.skill__name input'));
    expect(boxes.map((i) => i.checked)).toEqual([true, true, true]);
    expect(boxes.map((i) => i.disabled)).toEqual([true, false, true]);
    const sources = Array.from(el.querySelectorAll('.skill__source')).map((n) =>
      n.textContent?.trim(),
    );
    expect(sources).toEqual(['da raça', 'do antecedente']);
    expect(el.textContent).toContain('1 perícia marcada, 2 já concedidas');
  });

  it('lets a granted skill take expertise', () => {
    const { el } = render([], [], [['skill:history', 'do antecedente']]);

    const expertise = Array.from(el.querySelectorAll<HTMLInputElement>('.skill__expertise input'));
    expect(expertise.map((i) => i.disabled)).toEqual([true, true, false]);
  });
});
