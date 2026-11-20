import { TestBed } from '@angular/core/testing';

import { creature, flat, raven, summary } from '../../core/creatures/creatures-testing';
import { steedSpell } from '../../core/creatures/steed-testing';
import { SteedChoice } from './steed-choice';

describe('SteedChoice: the steed part of the summon sheet', () => {
  function setup(inputs: Record<string, unknown> = {}) {
    const fixture = TestBed.createComponent(SteedChoice);
    fixture.componentRef.setInput('spell', steedSpell());
    for (const [k, v] of Object.entries(inputs)) {
      fixture.componentRef.setInput(k, v);
    }
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const picks: string[] = [];
    fixture.componentInstance.pickType.subscribe((t) => picks.push(t));
    const types = () => el.querySelector('[role=radiogroup][aria-label="Tipo do corcel"]');
    const fold = () =>
      Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
        b.textContent?.includes('Como funciona'),
      );
    return { fixture, el, picks, types, fold };
  }

  it('offers "Tipo do corcel" as three segments with the app\'s words, none chosen at first', () => {
    const { el, types } = setup();
    expect(el.textContent).toContain('Tipo do corcel');
    const labels = Array.from(types()?.querySelectorAll('label') ?? []).map((l) => flat(l));
    expect(labels).toEqual(['Celestial', 'Fada', 'Ínfero']);
    expect(types()?.querySelector('input:checked')).toBeNull();
  });

  it("tells the sheet the type picked, by the server's value, and shows the one chosen", () => {
    const { fixture, picks, types } = setup();
    const fey = types()!.querySelectorAll<HTMLInputElement>('input[type=radio]')[1];
    fey.checked = true;
    fey.dispatchEvent(new Event('change'));
    expect(picks).toEqual(['fey']);
    fixture.componentRef.setInput('type', 'fey');
    fixture.detectChanges();
    expect(types()?.querySelector<HTMLInputElement>('input:checked')?.value).toBe('fey');
  });

  it('folds "Como funciona": closed on a phone, open on a desktop, with the notes and the least Intelligence', () => {
    const phone = setup({ phone: true });
    expect(phone.fold()?.getAttribute('aria-expanded')).toBe('false');
    expect(phone.el.textContent).not.toContain('telepatia');
    phone.fold()?.click();
    phone.fixture.detectChanges();
    expect(phone.fold()?.getAttribute('aria-expanded')).toBe('true');
    expect(phone.el.textContent).toContain('Vocês se falam por telepatia a até 1,5 km.');
    expect(phone.el.textContent).toContain('A Inteligência mínima do corcel é 6.');

    TestBed.resetTestingModule();
    const desktop = setup({ phone: false });
    expect(desktop.fold()?.getAttribute('aria-expanded')).toBe('true');
    expect(desktop.el.textContent).toContain('A Inteligência mínima do corcel é 6.');
  });

  it('leaves "Como funciona" out when the spell says nothing', () => {
    const { fold } = setup({ spell: steedSpell({ notes: [], minInt: 0 }) });
    expect(fold()).toBeUndefined();
  });

  const estrela = () =>
    creature('steed-1', 'Estrela', {
      monsterKey: 'monster:warhorse',
      monsterNamePt: 'Cavalo de guerra',
      creatureType: 'celestial',
      armorClass: 11,
      hitPointsMax: 19,
      bonded: true,
    });
  const horse = () =>
    raven({
      summary: summary('monster:warhorse', 'Cavalo de guerra', { sizePt: 'Grande' }),
      armorClass: 11,
      hitPoints: 19,
      speedWalkFt: 60,
      speedFlyFt: 0,
    });

  it('part "same": the fixed row with the silhouette, the name, the tag "Mesmo corcel" with the link icon, the numbers, and the line under it', () => {
    const { el, types, fold } = setup({
      spell: steedSpell({ bondedSteed: estrela() }),
      part: 'same',
      block: horse(),
    });
    expect(types()).toBeNull();
    expect(fold()).toBeUndefined();
    const row = el.querySelector('.same')!;
    expect(row.querySelectorAll('app-creature-art')).toHaveLength(1);
    expect(row.querySelector('.same__name')?.textContent?.trim()).toBe('Estrela');
    expect(flat(row.querySelector('.mr-tag'))).toBe('Mesmo corcel');
    expect(row.querySelector('.mr-tag mat-icon')?.textContent?.trim()).toBe('link');
    expect(flat(row.querySelector('.same__sub'))).toBe(
      'Cavalo de guerra · celestial · Grande · 18 m · CA 11 · PV 19',
    );
    expect(el.querySelector('.same__line')?.textContent?.trim()).toBe(
      'Estrela volta como era: a mesma forma, o mesmo tipo e os PV cheios. Para outra forma ou outro tipo, liberte Estrela antes, no bloco Criaturas.',
    );
  });

  it('part "rest" of a bonded steed has no type to pick and no fixed row: only "Como funciona"', () => {
    const { el, types, fold } = setup({ spell: steedSpell({ bondedSteed: estrela() }) });
    expect(types()).toBeNull();
    expect(el.querySelector('.same')).toBeNull();
    expect(flat(fold())).toContain('Como funciona');
    expect(el.querySelectorAll('input[type=radio]').length).toBe(0);
  });
});
