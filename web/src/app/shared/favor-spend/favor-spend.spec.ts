import { TestBed } from '@angular/core/testing';

import { RollMode } from '../../../gen/meurpg/play/v1/combat_rolls_pb';
import { FavorClient } from '../../core/play/favor-client';
import {
  FAVOR_ONLY_PLAYER,
  FAVOR_READ_FAILED,
  type FavorSpendHost,
  FavorSpend,
} from '../../core/play/favor-spend';
import { FavorState } from '../../core/play/favor-state';
import { FakeFavorClient, favorPass, openFavor } from '../../core/play/favor-testing';
import { FavorSpendBox } from './favor-spend';

const plain = (t: string | null | undefined) => (t ?? '').replace(/\s+/g, ' ').trim();

function setup(
  host: Partial<FavorSpendHost> = {},
  holders: Record<string, string> = { c1: 'Brisa' },
) {
  TestBed.resetTestingModule();
  TestBed.configureTestingModule({
    providers: [FavorState, { provide: FavorClient, useValue: new FakeFavorClient().asClient() }],
  });
  const favor = TestBed.inject(FavorState);
  openFavor(favor, holders);
  const fav = TestBed.runInInjectionContext(
    () =>
      new FavorSpend({
        favor,
        who: () => ({ characterId: 'c1', name: 'Brisa' }),
        circumstances: () => ({ mode: RollMode.NORMAL, advantages: [], disadvantages: [] }),
        ...host,
      }),
  );
  const fixture = TestBed.createComponent(FavorSpendBox);
  fixture.componentRef.setInput('control', fav);
  fixture.detectChanges();
  const el = fixture.nativeElement as HTMLElement;
  const box = () => el.querySelector<HTMLInputElement>('input[type=checkbox]');
  return { fixture, el, fav, favor, box };
}

describe('FavorSpendBox', () => {
  it('draws nothing when the character has none', () => {
    const { el } = setup({}, {});
    expect(el.querySelector('.favor')).toBeNull();
    expect(plain(el.textContent)).toBe('');
  });

  it('draws nothing for a roll that has no Inspiração to spend (a creature)', () => {
    expect(setup({ who: () => null }).el.querySelector('.favor')).toBeNull();
  });

  it('is open: the title, the label and the hint, unticked', () => {
    const { el, box } = setup();
    expect(plain(el.querySelector('h3')?.textContent)).toBe('Inspiração do mestre');
    expect(plain(el.querySelector('.favor__label')?.textContent)).toBe('Usar a Inspiração');
    expect(plain(el.querySelector('.favor__hint')?.textContent)).toBe(
      'Vantagem nesta jogada · ela sai da sua ficha',
    );
    expect(box()!.checked).toBe(false);
    expect(box()!.getAttribute('aria-disabled')).toBeNull();
    expect(el.querySelector('.favor__why')).toBeNull();
  });

  it('ticks and unticks, and the control says it spends', () => {
    const { fixture, fav, box } = setup();
    box()!.click();
    fixture.detectChanges();
    expect(fav.spend()).toBe(true);
    expect(box()!.checked).toBe(true);
    expect(fixture.nativeElement.querySelector('.favor__box--on')).not.toBeNull();
    box()!.click();
    fixture.detectChanges();
    expect(fav.spend()).toBe(false);
  });

  it('is dashed and still reachable when the sources already give advantage, with the reason linked', () => {
    const { fixture, el, fav, box } = setup({
      circumstances: () => ({
        mode: RollMode.ADVANTAGE,
        advantages: ['Ajuda de Tor'],
        disadvantages: [],
      }),
    });
    const why = el.querySelector('.favor__why')!;
    expect(plain(why.textContent)).toContain(
      'A Inspiração não mudaria esta rolagem: ela já tem vantagem (Ajuda de Tor).',
    );
    expect(box()!.getAttribute('aria-disabled')).toBe('true');
    expect(box()!.getAttribute('aria-describedby')).toContain(why.id);
    expect(box()!.getAttribute('aria-describedby')).toContain('-hint');
    expect(box()!.disabled).toBe(false);
    box()!.click();
    fixture.detectChanges();
    expect(fav.ticked()).toBe(false);
    expect(box()!.checked).toBe(false);
  });

  it('is dashed when the master acts for the player', () => {
    const { el } = setup({ byMaster: () => true });
    expect(plain(el.querySelector('.favor__hint')?.textContent)).toBe('O mestre agindo por Brisa');
    expect(plain(el.querySelector('.favor__why')?.textContent)).toContain(FAVOR_ONLY_PLAYER);
  });

  it('is dashed with the ask that waits for the master, the reason linked, and a tap changes nothing (581-07, item 4)', () => {
    const { fixture, el, fav, favor, box } = setup();
    favor.passes.set([favorPass({ fromId: 'c1' })]);
    fixture.detectChanges();
    const why = el.querySelector('.favor__why')!;
    expect(plain(why.textContent)).toContain(
      'Você pediu para passar a Inspiração a Tor. Cancele o pedido para usar.',
    );
    expect(box()!.getAttribute('aria-disabled')).toBe('true');
    expect(box()!.getAttribute('aria-describedby')).toContain(why.id);
    box()!.click();
    fixture.detectChanges();
    expect(fav.spend()).toBe(false);
    // The ask is gone: the box opens again.
    favor.passes.set([]);
    fixture.detectChanges();
    expect(box()!.getAttribute('aria-disabled')).toBeNull();
  });

  it('is dashed when the read failed', () => {
    const { fixture, el, favor } = setup();
    favor.failed.set(true);
    fixture.detectChanges();
    expect(plain(el.querySelector('.favor__why')?.textContent)).toContain(FAVOR_READ_FAILED);
  });

  it('is dashed while a request is in the air, and never changes the tick', () => {
    const { fixture, fav, box } = setup();
    fav.set(true);
    fixture.componentRef.setInput('busy', true);
    fixture.detectChanges();
    box()!.click();
    fixture.detectChanges();
    expect(fav.spend()).toBe(true);
    expect(box()!.getAttribute('aria-disabled')).toBe('true');
  });
});
