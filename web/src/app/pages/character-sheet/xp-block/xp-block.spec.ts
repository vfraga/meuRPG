import { TestBed } from '@angular/core/testing';

import { XpBlock } from './xp-block';

describe('XpBlock', () => {
  function render(over: Partial<Record<'xp' | 'nextLevelXp' | 'level' | 'canLevelUp', unknown>>) {
    const fixture = TestBed.createComponent(XpBlock);
    const inputs = { xp: 2366, nextLevelXp: 2700, level: 3, canLevelUp: false, ...over };
    for (const [k, v] of Object.entries(inputs)) {
      fixture.componentRef.setInput(k, v);
    }
    fixture.detectChanges();
    return (fixture.nativeElement as HTMLElement)
      .querySelector('.xp__line')
      ?.textContent?.replace(/ /g, ' ');
  }

  it('says how far the character is from the next level, from the numbers the server sent', () => {
    expect(render({})).toBe('2.366 de 2.700 XP para o nível 4. Faltam 334 XP.');
  });

  it('says it can level up when the server says so', () => {
    expect(render({ xp: 2716, canLevelUp: true })).toContain('Chegou aos 2.700 XP do nível 4.');
  });

  it('never says XP is missing when the XP already reached the next level but the server does not offer the level (a dead character)', () => {
    const line = render({ xp: 2716, canLevelUp: false });
    expect(line).toBe('2.716 de 2.700 XP para o nível 4.');
    expect(line).not.toContain('Faltam');
  });

  it('says the top level has none to reach', () => {
    expect(render({ xp: 400000, nextLevelXp: 0, level: 20 })).toBe('Nível máximo.');
  });
});
