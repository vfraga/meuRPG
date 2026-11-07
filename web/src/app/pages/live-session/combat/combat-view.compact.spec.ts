import { ChangeDetectionStrategy, Component, TemplateRef, viewChild } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { create } from '@bufbuild/protobuf';

import {
  CombatantKind,
  CombatantSide,
  EncounterMode,
} from '../../../../gen/meurpg/play/v1/combat_pb';
import {
  ActionEconomy,
  ActionOptionSchema,
  AttackOptionSchema,
  SpellOptionSchema,
  TurnOptionsSchema,
} from '../../../../gen/meurpg/rules/v1/rules_pb';
import { combatant, encounter } from '../../../core/combat/combat-testing';
import { PHONE_QUERY } from '../../../shared/map-view/media-query';
import { settle } from '../../../../test-settle';
import { mountCombatView } from './combat-view-testing';

// The player's own turn on a phone and at 1280 (C4-03, C4-04, C6-07): one "Mover" and one "Encerrar turno" on every screen.

const LAPTOP = ['(min-width: 1024px)', '(min-width: 1280px)'];

const toren = combatant({
  id: 't',
  label: 'Toren',
  kind: CombatantKind.PLAYER,
  side: CombatantSide.PARTY,
  characterId: 'toren-c',
  mine: true,
  hitPointsCurrent: 19,
  hitPointsMax: 28,
});
const goblin = combatant({ id: 'g', label: 'Goblin 2' });
const turnOf = (current: string, mine = toren) =>
  encounter({
    mode: EncounterMode.GRID,
    currentCombatantId: current,
    combatants: [mine, goblin],
  });

const options = () => ({
  options: create(TurnOptionsSchema, {
    attacks: [
      create(AttackOptionSchema, {
        attack: { key: 'equipment:longsword', name: 'Espada longa', namePt: 'Espada longa' },
        enabled: true,
      }),
    ],
    spells: [
      create(SpellOptionSchema, {
        spell: { key: 'spell:web', name: 'Teia', namePt: 'Teia', level: 2 },
        economy: ActionEconomy.ACTION,
        enabled: true,
      }),
    ],
    featureActions: [
      create(ActionOptionSchema, {
        action: {
          key: 'feature:second-wind',
          namePt: 'Retomar o Fôlego',
          economy: ActionEconomy.BONUS_ACTION,
        },
        enabled: true,
      }),
    ],
  }),
  maneuverDice: [],
});

async function mount(
  matches: readonly string[],
  current = 't',
  inputs: Record<string, unknown> = {},
) {
  const mounted = mountCombatView({
    enc: turnOf(current),
    matches,
    turnOptions: options,
    inputs: { armorClass: 17, ...inputs },
  });
  mounted.mapState.setMap({
    id: 'map',
    name: 'Cripta do Corvo',
    image: { url: '/m.png', width: 100, height: 100 },
  } as never);
  mounted.fixture.detectChanges();
  await settle(mounted.fixture);
  const named = (name: string) =>
    [...mounted.el.querySelectorAll('button')].filter((b) => {
      const copy = b.cloneNode(true) as HTMLElement;
      copy.querySelectorAll('mat-icon').forEach((i) => i.remove());
      return copy.textContent?.trim() === name;
    });
  return { ...mounted, named };
}

describe('CombatView: the player’s own turn on a phone', () => {
  it('has the map first and the movement row under it, with the one "Mover"', async () => {
    const { el, named, state } = await mount([PHONE_QUERY]);
    const card = el.querySelector('app-combat-map-card')!;
    expect(card.querySelector('app-move-row')).not.toBeNull();
    expect(named('Mover')).toHaveLength(1);
    expect(card.contains(named('Mover')[0])).toBe(true);
    expect(card.querySelector('[aria-label="Ver mapa"]')).not.toBeNull();
    named('Mover')[0].click();
    expect(state.moving()).toBe(true);
  });

  it('has the turn card with the shapes and the numbers, and the action tabs from the options', async () => {
    const { el } = await mount([PHONE_QUERY]);
    const card = el.querySelector('app-turn-panel')!;
    expect(card.querySelector('h2')?.textContent).toBe('Sua vez, Toren');
    expect(card.textContent).toContain('depois de você: Goblin 2');
    expect(card.textContent).toContain('PV 19 de 28');
    expect(card.textContent).toContain('CA 17');
    expect(
      [...card.querySelectorAll('.eco li')].map((li) => li.getAttribute('aria-label')),
    ).toEqual(['Ação disponível', 'Ação bônus disponível', 'Reação disponível']);
    expect(
      [...el.querySelectorAll('app-action-groups [role="tab"]')].map((t) =>
        t.getAttribute('aria-label'),
      ),
    ).toEqual([
      'Ataques 1, 1 ataque',
      'Magias 1, 1 magia',
      'Guerreiro 1, 1 habilidade',
      'Outras 0, 0 ações',
    ]);
  });

  it('has "Ainda tem" and the one "Encerrar turno" in the sticky bar', async () => {
    const { el } = await mount([PHONE_QUERY]);
    const bar = el.querySelector('app-turn-bar')!;
    expect(bar.textContent).toContain('Ainda tem');
    // The card's own button (`.turn__end`) is `display: none` under 1024px, and so is this bar from 1024px: one on every screen.
    expect(bar.querySelectorAll('button.end')).toHaveLength(1);
    expect(el.querySelector('app-turn-panel .banner')).toBeNull();
  });
});

describe('CombatView: the player’s own turn at 1280', () => {
  it('has the turn bar with the shapes, the one "Mover" and the one "Encerrar turno"', async () => {
    const { el, named, state } = await mount(LAPTOP);
    const bar = el.querySelector('app-turn-panel .banner')!;
    expect(named('Mover')).toHaveLength(1);
    expect(bar.contains(named('Mover')[0])).toBe(true);
    expect(bar.querySelectorAll('button.end')).toHaveLength(1);
    expect([...bar.querySelectorAll('li')].map((li) => li.getAttribute('aria-label'))).toHaveLength(
      4,
    );
    named('Mover')[0].click();
    expect(state.moving()).toBe(true);
  });

  it('says in the movement row to drag the token, without a second "Mover"', async () => {
    const { el } = await mount(LAPTOP);
    const row = el.querySelector('app-move-row')!;
    expect(row.querySelector('button')).toBeNull();
    expect(row.textContent).toContain('arraste o seu token no mapa');
  });

  it('puts the actions in a rail named "Ações" and the log in a fold', async () => {
    const { el } = await mount(LAPTOP);
    const rail = el.querySelector('app-session-rail')!;
    expect(rail.getAttribute('aria-label')).toBe('Ações');
    expect(rail.querySelector('app-action-groups')).not.toBeNull();
    const log = el.querySelector('app-combat-log-panel app-fold button')!;
    expect(log.textContent).toContain('Registro do combate');
    expect(log.getAttribute('aria-expanded')).toBe('false');
  });

  it('says in the rail, off turn, what the player still has, and has no "Mover" or "Encerrar turno"', async () => {
    const { el, named } = await mount(LAPTOP, 'g');
    const rail = el.querySelector('app-session-rail')!;
    expect(rail.textContent?.replace(/\s+/g, ' ')).toContain('Reação · Disponível');
    expect(named('Mover')).toHaveLength(0);
    expect(named('Encerrar turno')).toHaveLength(0);
  });
});

@Component({
  template: '<ng-template #t><p class="fake-chat">A conversa</p></ng-template>',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
class ChatHost {
  readonly template = viewChild.required<TemplateRef<unknown>>('t');
}

describe('CombatView: the rail at 1280 and off turn on a phone', () => {
  it('keeps the "Ações" tab in the strip off turn, with the reaction under it', async () => {
    const { el } = await mount(LAPTOP, 'g');
    const tabs = [...el.querySelectorAll('app-session-rail > app-tab-list [role="tab"]')];
    expect(tabs.map((t) => t.textContent?.replace(/\s+/g, ' ').trim())).toEqual([
      expect.stringContaining('Ações'),
    ]);
  });

  it('says on a phone, off turn, what the character still has', async () => {
    const { el } = await mount([PHONE_QUERY], 'g');
    expect(el.querySelector('app-session-rail')?.textContent?.replace(/\s+/g, ' ')).toContain(
      'Reação · Disponível',
    );
  });

  it('has "Ações" and "Conversa" with the unread count in words, and the chat panel mounted and hidden', async () => {
    const opened: boolean[] = [];
    const host = TestBed.createComponent(ChatHost);
    host.detectChanges();
    const mounted = await mount(LAPTOP, 't', {
      chat: host.componentInstance.template(),
      chatUnread: 3,
    });
    mounted.view.chatShown.subscribe((v: boolean) => opened.push(v));
    const rail = mounted.el.querySelector('app-session-rail')!;
    const tabs = [
      ...rail.querySelectorAll<HTMLButtonElement>(':scope > app-tab-list [role="tab"]'),
    ];
    expect(
      tabs.map((t) => t.getAttribute('aria-label') ?? t.textContent?.replace(/\s+/g, ' ').trim()),
    ).toEqual([expect.stringContaining('Ações'), 'Conversa 3, 3 mensagens novas']);
    expect(tabs[1].querySelector('.tl__badge--news')?.textContent).toBe('3');
    const chat = rail.querySelector<HTMLElement>('#rail-panel-chat')!;
    expect(chat.hidden).toBe(true);
    tabs[1].click();
    mounted.fixture.detectChanges();
    expect(chat.hidden).toBe(false);
    await settle(mounted.fixture);
    expect(opened.at(-1)).toBe(true);
  });
});
