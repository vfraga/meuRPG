import { TestBed } from '@angular/core/testing';

import { TV_STAGE_MAX, TvScene } from './tv-scene';

function render(description: string, stage: { name: string; speaking: boolean }[]) {
  const fixture = TestBed.createComponent(TvScene);
  fixture.componentRef.setInput('description', description);
  fixture.componentRef.setInput('stage', stage);
  fixture.detectChanges();
  return fixture.nativeElement as HTMLElement;
}

describe('TvScene', () => {
  it('shows the description and who is on stage, marking who speaks', () => {
    const el = render('O salão está cheio e quente.', [
      { name: 'Bartolo', speaking: true },
      { name: 'Capitão bandido', speaking: false },
    ]);
    expect(el.querySelector('.tv-scene__description')?.textContent).toBe(
      'O salão está cheio e quente.',
    );
    expect([...el.querySelectorAll('.tv-scene__name')].map((n) => n.textContent)).toEqual([
      'Bartolo',
      'Capitão bandido',
    ]);
    expect(el.querySelectorAll('.tv-scene__speaking')).toHaveLength(1);
    expect(el.querySelector('.tv-scene__speaking')?.textContent).toContain('Fala agora');
    expect(el.querySelector('.tv-scene__npc--speaking .tv-scene__name')?.textContent).toBe(
      'Bartolo',
    );
  });

  it('is one card when nobody is on stage', () => {
    const el = render('Silêncio.', []);
    expect(el.querySelector('.tv-scene--alone')).not.toBeNull();
    expect(el.querySelector('.tv-scene__npcs')).toBeNull();
  });

  it('cuts a crowded stage with "+N"', () => {
    const crowd = Array.from({ length: TV_STAGE_MAX + 2 }, (_, i) => ({
      name: `N${i}`,
      speaking: false,
    }));
    const el = render('', crowd);
    expect(el.querySelectorAll('.tv-scene__npc')).toHaveLength(TV_STAGE_MAX);
    expect(el.querySelector('.tv-scene__more')?.textContent?.trim()).toBe('+2');
  });

  it('draws no description paragraph when there is none', () => {
    expect(render('', []).querySelector('.tv-scene__description')).toBeNull();
  });
});
