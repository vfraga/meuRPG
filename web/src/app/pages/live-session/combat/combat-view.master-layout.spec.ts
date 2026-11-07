import { ChangeDetectionStrategy, Component, TemplateRef, viewChild } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { vi } from 'vitest';

import {
  CombatantKind,
  EncounterMode,
  EncounterStatus,
} from '../../../../gen/meurpg/play/v1/combat_pb';
import { combatant, encounter } from '../../../core/combat/combat-testing';
import { settle } from '../../../../test-settle';
import { mountCombatView } from './combat-view-testing';

@Component({
  template: '<ng-template #t><p class="fake-chat">A conversa</p></ng-template>',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
class ChatHost {
  readonly template = viewChild.required<TemplateRef<unknown>>('t');
}

const rows = () => [
  combatant({
    id: 'g1',
    label: 'Goblin 1',
    hitPointsCurrent: 7,
    hitPointsMax: 7,
    initiative: 14,
    armorClass: 15,
  }),
  combatant({
    id: 'pen',
    label: 'Pensantus',
    kind: CombatantKind.PLAYER,
    characterId: 'pen-c',
    hitPointsCurrent: 23,
    hitPointsMax: 23,
    initiative: 12,
  }),
];
const running = (over = {}) =>
  encounter({ combatants: rows(), currentCombatantId: 'g1', round: 3, ...over });

const WIDE = ['(min-width: 1024px)', '(min-width: 1280px)'];
const LAPTOP = ['(min-width: 1024px)'];

describe("CombatView, the master's three columns (alternative B)", () => {
  async function mount(matches: readonly string[], enc = running(), withChat = true) {
    const mounted = mountCombatView({ enc, master: true, matches });
    if (withChat) {
      const host = TestBed.createComponent(ChatHost);
      host.detectChanges();
      mounted.fixture.componentRef.setInput('chat', host.componentInstance.template());
      mounted.fixture.componentRef.setInput('chatUnread', 3);
    }
    await settle(mounted.fixture);
    return mounted;
  }

  const tabNames = (el: HTMLElement, list: string) =>
    Array.from(el.querySelectorAll(`${list} [role="tab"]`), (t) =>
      (t.getAttribute('aria-label') ?? t.querySelector('span')?.textContent ?? '')
        .replace(/\s+/g, ' ')
        .trim(),
    );

  it('from 1280px has the order on the left, the map in the middle and the rail "Turno e conversa" on the right', async () => {
    const { el } = await mount(WIDE);
    expect(el.querySelector('.tri--wide')).not.toBeNull();
    expect(el.querySelector('.tri__order app-order-list')).not.toBeNull();
    const rail = el.querySelector('app-session-rail')!;
    expect(rail.getAttribute('aria-label')).toBe('Turno e conversa');
    expect(rail.getAttribute('role')).toBe('complementary');
    expect(tabNames(el, 'app-session-rail')).toEqual(['Turno', 'Conversa 3, 3 mensagens novas']);
    // The order is not drawn a second time inside the rail.
    expect(rail.querySelector('app-order-list')).toBeNull();
  });

  it('keeps the conversation mounted and hidden while "Turno" is open, and shows it from its tab', async () => {
    const { fixture, el } = await mount(WIDE);
    const chat = el.querySelector<HTMLElement>('#rail-panel-chat')!;
    expect(chat.hidden).toBe(true);
    expect(chat.querySelector('.fake-chat')).not.toBeNull();
    el.querySelectorAll<HTMLButtonElement>('app-session-rail [role="tab"]')[1].click();
    await settle(fixture);
    expect(chat.hidden).toBe(false);
    expect(el.querySelector<HTMLElement>('#rail-panel-turn')!.hidden).toBe(true);
  });

  it('tells the page when the conversation is what the rail shows', async () => {
    const { fixture, el } = await mount(WIDE, running(), false);
    const shown = vi.fn();
    fixture.componentInstance.chatShown.subscribe(shown);
    const host = TestBed.createComponent(ChatHost);
    host.detectChanges();
    fixture.componentRef.setInput('chat', host.componentInstance.template());
    await settle(fixture);
    expect(shown).toHaveBeenLastCalledWith(false);
    el.querySelectorAll<HTMLButtonElement>('app-session-rail [role="tab"]')[1].click();
    await settle(fixture);
    expect(shown).toHaveBeenLastCalledWith(true);
  });

  const rowButtons = (el: HTMLElement) =>
    Array.from(el.querySelectorAll<HTMLButtonElement>('.more .mr-fold__button'));
  const rowName = (b: HTMLButtonElement) => b.querySelector('.mr-fold__title')!.textContent!.trim();

  it('has no tab set under the map: "Mais do combate" is in the rail, under the content of "Turno"', async () => {
    const { el } = await mount(WIDE);
    expect(el.querySelector('.mtabs')).toBeNull();
    const more = el.querySelector('.tri__rail .more')!;
    expect(more.getAttribute('aria-label')).toBe('Mais do combate');
    expect(more.querySelector('.more__title')!.textContent).toContain('Mais do combate');
    expect(rowButtons(el).map(rowName)).toEqual(['Registro', 'Efeitos']);
    expect(rowButtons(el).every((b) => b.getAttribute('aria-expanded') === 'false')).toBe(true);
  });

  it('opens a row in place, and each row by itself', async () => {
    const { fixture, el } = await mount(WIDE);
    rowButtons(el)[1].click();
    await settle(fixture);
    expect(rowButtons(el)[1].getAttribute('aria-expanded')).toBe('true');
    expect(el.querySelector('.more app-effects-panel')).not.toBeNull();
    expect(rowButtons(el)[0].getAttribute('aria-expanded')).toBe('false');
    rowButtons(el)[0].click();
    await settle(fixture);
    expect(el.querySelector('.more app-combat-log-panel')).not.toBeNull();
    expect(rowButtons(el)[1].getAttribute('aria-expanded')).toBe('true');
  });

  it('folds the order to a strip of tokens, and the map takes the width back', async () => {
    const { fixture, el } = await mount(WIDE);
    el.querySelector<HTMLButtonElement>('[aria-label="Recolher a ordem de iniciativa"]')!.click();
    await settle(fixture);
    expect(el.querySelector('.tri--folded')).not.toBeNull();
    expect(el.querySelectorAll('.strip__token').length).toBe(2);
    el.querySelector<HTMLButtonElement>('[aria-label="Abrir a ordem de iniciativa"]')!.click();
    await settle(fixture);
    expect(el.querySelector('.tri--folded')).toBeNull();
  });

  it('keeps the rail and moves the order into "Turno" between 1024 and 1279px', async () => {
    const { el } = await mount(LAPTOP);
    expect(el.querySelector('.tri--wide')).toBeNull();
    expect(el.querySelector('.tri__order')).toBeNull();
    expect(el.querySelector('#rail-panel-turn app-order-list')).not.toBeNull();
    expect(el.querySelector('[aria-label="Recolher a ordem de iniciativa"]')).toBeNull();
  });

  it('has no "Conversa" tab where the page keeps the conversation elsewhere', async () => {
    const { el } = await mount(WIDE, running(), false);
    expect(el.querySelector('app-session-rail [role="tablist"]')).toBeNull();
    expect(el.querySelector('#rail-panel-turn')).not.toBeNull();
  });

  it('puts the turn card in "Turno", with no "Próximo turno" of its own: the combat bar has it', async () => {
    const { el } = await mount(WIDE);
    const card = el.querySelector('#rail-panel-turn app-npc-card')!;
    expect(card.textContent?.replace(/\u00a0/g, ' ')).toContain('Ações do Goblin 1');
    expect(
      Array.from(card.querySelectorAll('button')).some((b) =>
        b.textContent?.includes('Próximo turno'),
      ),
    ).toBe(false);
    const bar = el.querySelector('app-combat-bar')!;
    expect(
      Array.from(bar.querySelectorAll('button')).filter((b) =>
        b.textContent?.includes('Próximo turno'),
      ).length,
    ).toBe(1);
  });

  it('keeps a combat without a map as it was in the middle: the card and the cover, the rail with "Turno"', async () => {
    const { el } = await mount(
      WIDE,
      running({ mode: EncounterMode.THEATRE, mapId: '', gridColumns: 0, gridRows: 0 }),
    );
    expect(el.querySelector('.tri__main app-npc-card')).not.toBeNull();
    expect(el.querySelector('.tri__main app-cover-panel')).not.toBeNull();
    expect(el.querySelector('#rail-panel-turn app-npc-card')).toBeNull();
    expect(el.querySelector('app-combat-map-card')).toBeNull();
  });
});

describe("CombatView, the master's phone (C6-01)", () => {
  async function mount(enc = running()) {
    const mounted = mountCombatView({ enc, master: true, matches: [] });
    await settle(mounted.fixture);
    return mounted;
  }

  it('has the tabs "Turno · Ordem · Mapa", "Turno" first, the others hidden', async () => {
    const { el } = await mount();
    const names = Array.from(el.querySelectorAll('[role="tab"]'), (t) =>
      (t.getAttribute('aria-label') ?? t.querySelector('span')?.textContent ?? '')
        .replace(/\s+/g, ' ')
        .trim(),
    );
    expect(names).toEqual(['Turno', 'Ordem 2, 2 combatentes', 'Mapa']);
    expect(el.querySelector<HTMLElement>('#ptab-panel-turn')!.hidden).toBe(false);
    expect(el.querySelector<HTMLElement>('#ptab-panel-order')!.hidden).toBe(true);
    expect(el.querySelector<HTMLElement>('#ptab-panel-map')!.hidden).toBe(true);
  });

  it('has the fixed bar with the page\'s one "Próximo turno", and the card has none', async () => {
    const { el } = await mount();
    const buttons = Array.from(el.querySelectorAll('button')).filter((b) =>
      b.textContent?.includes('Próximo turno'),
    );
    expect(buttons.length).toBe(1);
    expect(buttons[0].closest('.phone__bar')).not.toBeNull();
    expect(el.querySelector('app-combat-bar:not([class*="phone"])')).not.toBeNull();
  });

  it('puts the other panels in "Mais", at the end of "Turno", folded', async () => {
    const { fixture, el } = await mount();
    const more = Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find(
      (b) => b.textContent?.includes('Mais') && !b.textContent.includes('Mais ações'),
    )!;
    expect(more.getAttribute('aria-expanded')).toBe('false');
    expect(el.querySelector('.more')).toBeNull();
    more.click();
    await settle(fixture);
    expect(more.getAttribute('aria-expanded')).toBe('true');
    expect(el.querySelector('.more')).not.toBeNull();
    expect(el.querySelector('.more__title')).toBeNull();
  });

  it('opens the order tab when the master answers a question that points at it', async () => {
    const { fixture, el, view } = await mount();
    view.focusOrder();
    await settle(fixture);
    expect(el.querySelector<HTMLElement>('#ptab-panel-order')!.hidden).toBe(false);
  });

  it('has no "Mapa" tab in a combat without a map', async () => {
    const { el } = await mount(
      running({ mode: EncounterMode.THEATRE, mapId: '', gridColumns: 0, gridRows: 0 }),
    );
    expect(el.querySelector('#ptab-panel-map')).toBeNull();
    expect(el.querySelectorAll('[role="tab"]').length).toBe(2);
  });

  it('says the round, the combat and who comes next in one line', async () => {
    const { el } = await mount();
    expect(el.querySelector('.phone__line')?.textContent?.replace(/\s+/g, ' ')).toContain(
      'Rodada 3 · Emboscada na estrada',
    );
  });
});

describe('CombatView, a combat that is not running', () => {
  it('draws the setup with the bar\'s "Começar o combate" and no rail', async () => {
    const { fixture, el } = mountCombatView({
      enc: encounter({ status: EncounterStatus.SETUP, combatants: rows() }),
      master: true,
      matches: WIDE,
    });
    await settle(fixture);
    expect(el.querySelector('app-session-rail')).toBeNull();
    const begins = Array.from(el.querySelectorAll('button')).filter((b) =>
      b.textContent?.includes('Começar o combate'),
    );
    expect(begins.length).toBe(1);
    expect(begins[0].closest('app-combat-bar')).not.toBeNull();
  });
});
