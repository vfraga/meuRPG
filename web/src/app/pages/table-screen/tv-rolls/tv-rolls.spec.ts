import { TestBed } from '@angular/core/testing';

import { rollsOf } from '../table-screen-testing';
import { TvRolls, facesText } from './tv-rolls';

function render(rolls: readonly object[]) {
  const fixture = TestBed.createComponent(TvRolls);
  fixture.componentRef.setInput('rolls', rolls);
  fixture.detectChanges();
  return fixture.nativeElement as HTMLElement;
}

describe('facesText', () => {
  it('reads the faces as a sentence', () => {
    expect(facesText([])).toBe('');
    expect(facesText([12])).toBe('12');
    expect(facesText([4, 4])).toBe('4 e 4');
    expect(facesText([2, 5, 6])).toBe('2, 5 e 6');
  });
});

describe('TvRolls', () => {
  it('shows nothing without rolls', () => {
    expect(render([]).querySelector('section')).toBeNull();
  });

  it('makes the newest roll the big one, with its dice, and lists the older ones', () => {
    const el = render(rollsOf);
    expect(el.querySelector('h2')?.textContent).toContain('Rolagens da mesa');
    const last = el.querySelector('[data-testid="tv-roll-last"]')!;
    expect(last.textContent).toContain('Brisa rolou');
    expect(last.querySelector('.tv-rolls__total')?.textContent).toBe('11');
    expect(last.textContent).toContain('2d6+3');
    expect(last.textContent).toContain('· 2 e 2');
    const rows = el.querySelectorAll('.tv-rolls__row');
    expect(rows).toHaveLength(1);
    expect(rows[0].textContent).toContain('Tor');
    expect(rows[0].querySelector('.tv-rolls__row-total')?.textContent).toBe('14');
  });

  it('has no list of older rolls with just one', () => {
    expect(render([rollsOf[0]]).querySelector('.tv-rolls__older')).toBeNull();
  });
});
