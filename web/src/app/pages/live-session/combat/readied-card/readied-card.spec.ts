import { create } from '@bufbuild/protobuf';
import { TestBed } from '@angular/core/testing';
import { OverlayContainer } from '@angular/cdk/overlay';

import {
  ReadiedActionSchema,
  ReadiedKind,
  ReadiedStatus,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { ReadiedCard } from './readied-card';

const flat = (n: Element | null | undefined) =>
  n?.textContent?.replace(/\s+/g, ' ').replace(/ /g, ' ').trim() ?? '';

describe('ReadiedCard (#582)', () => {
  function render(ownTurn: boolean, kind = ReadiedKind.ATTACK) {
    const fixture = TestBed.createComponent(ReadiedCard);
    fixture.componentRef.setInput(
      'readied',
      create(ReadiedActionSchema, {
        id: 'r1',
        triggerText: 'Quando o zumbi passar pela porta',
        kind,
        actionNamePt: kind === ReadiedKind.SPELL ? 'Mísseis Mágicos' : 'Espada longa',
        targetCombatantId: 'z1',
        status: ReadiedStatus.WAITING,
      }),
    );
    fixture.componentRef.setInput('combatants', [{ id: 'z1', label: 'Zumbi 1' }]);
    fixture.componentRef.setInput('ownTurn', ownTurn);
    const given: number[] = [];
    fixture.componentInstance.giveUp.subscribe(() => given.push(1));
    fixture.detectChanges();
    return { fixture, el: fixture.nativeElement as HTMLElement, given };
  }

  it('reads as a card named by the held action, with the trigger quoted', () => {
    const { el } = render(true);
    expect(el.querySelector('section')!.getAttribute('aria-label')).toBe(
      'Ação preparada: Espada longa no Zumbi 1',
    );
    expect(flat(el.querySelector('h3'))).toBe('Espada longa no Zumbi 1');
    expect(flat(el.querySelector('.card__quote'))).toBe('“Quando o zumbi passar pela porta”');
  });

  it('has a "⋮" with "Desistir da ação preparada" on its own turn, and calls give up from it', () => {
    const { el, given, fixture } = render(true);
    const more = el.querySelector<HTMLButtonElement>('.card__more')!;
    expect(more.getAttribute('aria-label')).toBe('Mais ações da ação preparada');
    more.click();
    fixture.detectChanges();
    const overlay = TestBed.inject(OverlayContainer).getContainerElement();
    const item = overlay.querySelector<HTMLButtonElement>('button[mat-menu-item]')!;
    expect(flat(item)).toContain('Desistir da ação preparada');
    expect(flat(item)).toContain('A ação gasta não volta.');
    item.click();
    fixture.detectChanges();
    expect(given).toEqual([1]);
  });

  it('only reads off the turn: no button at all, because the master decides the trigger', () => {
    const { el } = render(false);
    expect(el.querySelector('button')).toBeNull();
    expect(flat(el.querySelector('.card__state'))).toContain('Esperando o gatilho · reação livre');
  });

  it('marks a held spell as the concentration', () => {
    const { el } = render(false, ReadiedKind.SPELL);
    expect(flat(el.querySelector('.card__chip'))).toContain('Concentração');
    expect(render(false).el.querySelector('.card__chip')).toBeNull();
  });
});
