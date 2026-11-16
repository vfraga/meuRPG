import { TestBed } from '@angular/core/testing';
import { afterEach, vi } from 'vitest';

import { CombatantKind } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { FavorClient } from '../../../../core/play/favor-client';
import { FavorState } from '../../../../core/play/favor-state';
import { FakeFavorClient, favorPass, openFavor } from '../../../../core/play/favor-testing';
import { OrderList } from './order-list';
import { openAllRows } from './order-list-testing';

// The master's "Dar Inspiração" in the order row's "Mais" menu (RN-43, boards 581-01 and 581-07 item 12).

const flat = (n: Element | null | undefined) => n?.textContent?.replace(/\s+/g, ' ').trim() ?? '';

const pensantus = combatant({
  id: 'pen',
  label: 'Pensantus',
  kind: CombatantKind.PLAYER,
  characterId: 'pen-c',
  hitPointsCurrent: 23,
  hitPointsMax: 23,
  initiative: 8,
});
const brisa = combatant({
  id: 'bri',
  label: 'Brisa',
  kind: CombatantKind.PLAYER,
  characterId: 'bri-c',
  hitPointsCurrent: 24,
  hitPointsMax: 24,
  initiative: 9,
});
const wight = combatant({
  id: 'wig',
  label: 'Wight',
  hitPointsCurrent: 30,
  hitPointsMax: 45,
  initiative: 18,
});

function mount(holders: Record<string, string> | null) {
  const client = new FakeFavorClient();
  TestBed.configureTestingModule({
    providers: [{ provide: FavorClient, useValue: client.asClient() }],
  });
  const favor = TestBed.inject(FavorState);
  if (holders) {
    openFavor(favor, holders);
  }
  const fixture = TestBed.createComponent(OrderList);
  fixture.componentRef.setInput(
    'encounter',
    encounter({ combatants: [wight, brisa, pensantus], currentCombatantId: 'wig' }),
  );
  fixture.detectChanges();
  return { fixture, favor, client, el: fixture.nativeElement as HTMLElement };
}

function menuOf(fixture: ReturnType<typeof mount>['fixture'], name: string): HTMLElement[] {
  openAllRows(fixture);
  (fixture.nativeElement as HTMLElement)
    .querySelector<HTMLButtonElement>(`button[aria-label="Mais ações para ${name}"]`)!
    .click();
  fixture.detectChanges();
  return Array.from(document.querySelectorAll<HTMLElement>('.mat-mdc-menu-item'));
}

describe("OrderList: the master's Inspiração", () => {
  // A menu left open in one test must not answer the next one.
  afterEach(() =>
    document.querySelectorAll('.cdk-overlay-container').forEach((c) => c.replaceChildren()),
  );

  it('offers "Dar Inspiração" on a player row, first in the menu, and gives it on a tap', () => {
    const { fixture, favor } = mount({});
    const grant = vi.spyOn(favor, 'grant').mockResolvedValue();
    const items = menuOf(fixture, 'Brisa');
    expect(flat(items[0])).toBe('star_border Dar Inspiração');
    expect(items[0].getAttribute('aria-disabled')).toBe('false');
    items[0].click();
    expect(grant).toHaveBeenCalledWith('bri-c', 'Brisa');
  });

  it('draws the item dashed with "Pensantus já tem Inspiração." for a holder, and sends nothing on a tap', () => {
    const { fixture, favor } = mount({ 'pen-c': 'Pensantus' });
    const grant = vi.spyOn(favor, 'grant').mockResolvedValue();
    const items = menuOf(fixture, 'Pensantus');
    expect(flat(items[0])).toBe('star_border Dar Inspiração Pensantus já tem Inspiração.');
    expect(items[0].getAttribute('aria-disabled')).toBe('true');
    items[0].click();
    expect(grant).not.toHaveBeenCalled();
  });

  it('offers nothing on an NPC row', () => {
    const { fixture } = mount({});
    const items = menuOf(fixture, 'Wight');
    expect(items.some((i) => flat(i).includes('Dar Inspiração'))).toBe(false);
  });

  it('offers nothing outside an open session', () => {
    const { fixture } = mount(null);
    const items = menuOf(fixture, 'Brisa');
    expect(items.some((i) => flat(i).includes('Dar Inspiração'))).toBe(false);
  });

  it('shows "★ Inspiração" in the row of a holder, and in no other', () => {
    const { fixture, el } = mount({ 'pen-c': 'Pensantus' });
    openAllRows(fixture);
    const badges = Array.from(el.querySelectorAll('[data-testid="order-favor"]'));
    expect(badges).toHaveLength(1);
    expect(flat(badges[0])).toBe('starTem Inspiração');
    expect(badges[0].closest('li')?.textContent).toContain('Pensantus');
  });

  it('shows no badge when nobody is known to have it (a read that failed)', () => {
    const { el } = mount({});
    expect(el.querySelector('[data-testid="order-favor"]')).toBeNull();
  });

  it('puts the badge beside the name, never in the line of initiative and class', () => {
    const { fixture, el } = mount({ 'pen-c': 'Pensantus' });
    openAllRows(fixture);
    const badge = el.querySelector('[data-testid="order-favor"]');
    expect(badge?.closest('.row__name')).not.toBeNull();
    expect(badge?.closest('.row__sub')).toBeNull();
  });

  it('keeps the success sentence for a screen reader only and draws a refusal', () => {
    const { fixture, favor, el } = mount({});
    openAllRows(fixture);
    favor.note.set({ characterId: 'bri-c', text: 'Brisa ganhou Inspiração.', refused: false });
    fixture.detectChanges();
    expect(el.querySelector('.js-favor-note')?.classList.contains('mr-visually-hidden')).toBe(true);
    favor.note.set({
      characterId: 'bri-c',
      text: 'Essa personagem já tem Inspiração.',
      refused: true,
    });
    fixture.detectChanges();
    expect(el.querySelector('.js-favor-note')?.classList.contains('mr-visually-hidden')).toBe(
      false,
    );
  });

  it('says in the row what the grant answered, in a live region', () => {
    const { fixture, favor, el } = mount({});
    openAllRows(fixture);
    favor.note.set({ characterId: 'bri-c', text: 'Brisa ganhou Inspiração.', refused: false });
    fixture.detectChanges();
    expect(flat(el.querySelector('.js-favor-note'))).toBe('Brisa ganhou Inspiração.');
    expect(el.querySelector('.js-favor-note')?.parentElement?.getAttribute('role')).toBe('status');
    expect(el.querySelectorAll('.js-favor-note')).toHaveLength(1);
  });

  it('reads "Tem Inspiração · esperando o mestre" in the owner\'s own row while the ask waits, and the plain chip to the master', () => {
    const { fixture, favor, el } = mount({ 'bri-c': 'Brisa' });
    favor.passes.set([favorPass({ fromId: 'bri-c' })]);
    openAllRows(fixture);
    fixture.detectChanges();
    expect(flat(el.querySelector('[data-testid="order-favor"]'))).toBe(
      'starTem Inspiração · esperando o mestre',
    );
    favor.master.set(true);
    fixture.detectChanges();
    expect(flat(el.querySelector('[data-testid="order-favor"]'))).toBe('starTem Inspiração');
  });
});
