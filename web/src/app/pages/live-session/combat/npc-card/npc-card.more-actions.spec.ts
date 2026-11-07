import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';

import { CombatantKind, EncounterStatus } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { fakeCombatClient } from '../../../../core/combat/combat-client-testing';
import { CombatState } from '../../../../core/combat/combat-state';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { settle } from '../../../../../test-settle';
import { NpcCard } from './npc-card';

const plain = (t: string | null | undefined) => (t ?? '').replace(/\s+/g, ' ').trim();

function render(
  opts: {
    theatre?: boolean;
    kind?: CombatantKind;
    why?: string;
    slug?: string;
    status?: EncounterStatus;
  } = {},
) {
  TestBed.resetTestingModule();
  TestBed.configureTestingModule({
    providers: [provideRouter([]), { provide: CombatClient, useValue: fakeCombatClient({}) }],
  });
  const subject = combatant({
    id: 'npc',
    label: 'Capitão bandido',
    kind: opts.kind ?? CombatantKind.NPC,
    bestiaryCreatureKey: opts.slug ? `monster:${opts.slug}` : '',
  });
  const fixture = TestBed.createComponent(NpcCard);
  fixture.componentRef.setInput('campaignId', 'c1');
  fixture.componentRef.setInput(
    'encounter',
    encounter({ status: opts.status ?? EncounterStatus.ACTIVE, combatants: [subject] }),
  );
  fixture.componentRef.setInput('subject', subject);
  const state = new CombatState();
  state.apply(encounter({ status: opts.status ?? EncounterStatus.ACTIVE, combatants: [subject] }));
  fixture.componentRef.setInput('state', state);
  fixture.componentRef.setInput('theatre', opts.theatre ?? false);
  fixture.componentRef.setInput('offerWhy', opts.why ?? '');
  const offered = vi.fn();
  fixture.componentInstance.offer.subscribe(offered);
  fixture.detectChanges();
  return { fixture, el: fixture.nativeElement as HTMLElement, offered };
}

const trigger = (el: HTMLElement) =>
  el.querySelector<HTMLButtonElement>('button[aria-label="Mais ações de Capitão bandido"]');
const items = () => Array.from(document.querySelectorAll<HTMLElement>('[role="menuitem"]'));
const text = (i: HTMLElement) => plain(i.textContent?.replace(/swords|open_in_new/, ' '));
const item = (name: string) => items().find((i) => plain(i.textContent).includes(name))!;

describe('NpcCard, "Mais ações"', () => {
  afterEach(() => {
    document.querySelectorAll('.cdk-overlay-container > *').forEach((n) => n.remove());
    TestBed.resetTestingModule();
  });

  for (const theatre of [true, false]) {
    describe(theatre ? 'in a combat without a map' : 'on a map', () => {
      it('holds the offer and the creature sheet in one menu, with no visible button of its own', async () => {
        const { fixture, el } = render({ theatre, slug: 'bandit-captain' });
        const more = trigger(el)!;
        expect(more).toBeTruthy();
        expect(plain(more.textContent)).toContain('Mais ações');
        expect(more.textContent).toContain('more_vert');
        // Nothing but the menu: no visible "Oferecer…" button, no standalone link.
        expect(plain(el.textContent)).not.toContain('Oferecer ataque de oportunidade');
        expect(el.querySelector('a[href*="bestiary"]')).toBeNull();
        more.click();
        await settle(fixture);
        expect(items().map(text)).toEqual(['Oferecer ataque de oportunidade', 'Ficha da criatura']);
        expect(item('Ficha da criatura').getAttribute('href')).toBe(
          '/campaigns/c1/bestiary/bandit-captain',
        );
      });

      it('choosing the offer closes the menu and asks the page to open the panel', async () => {
        const { fixture, el, offered } = render({ theatre, slug: 'bandit-captain' });
        trigger(el)!.click();
        await settle(fixture);
        item('Oferecer ataque de oportunidade').click();
        await settle(fixture);
        expect(offered).toHaveBeenCalledTimes(1);
        expect(document.querySelector('[role="menu"]')).toBeNull();
      });

      it('writes why nobody can react inside the menu and disables the row in place', async () => {
        const { fixture, el, offered } = render({ theatre, why: 'Ninguém pode reagir agora.' });
        trigger(el)!.click();
        await settle(fixture);
        const row = item('Oferecer ataque de oportunidade');
        expect(row.getAttribute('aria-disabled')).toBe('true');
        expect(plain(row.textContent)).toContain('Ninguém pode reagir agora.');
        row.click();
        await settle(fixture);
        expect(offered).not.toHaveBeenCalled();
        // Still open, so the reason stays readable.
        expect(document.querySelector('[role="menu"]')).not.toBeNull();
      });
    });
  }

  it('has only the creature sheet for a creature of the bestiary in a combat that has not started', async () => {
    const { fixture, el } = render({
      slug: 'goblin',
      status: EncounterStatus.SETUP,
    });
    trigger(el)!.click();
    await settle(fixture);
    expect(items().map(text)).toEqual(['Ficha da criatura']);
  });

  it('is not there for a player on turn, and not for an NPC with no sheet in a combat not under way', () => {
    const player = render({ kind: CombatantKind.PLAYER });
    expect(trigger(player.el)).toBeNull();
    const idle = render({ status: EncounterStatus.SETUP });
    expect(trigger(idle.el)).toBeNull();
  });

  it('closes with Escape and gives the focus back to its button', async () => {
    const { fixture, el } = render();
    const more = trigger(el)!;
    more.focus();
    more.click();
    await settle(fixture);
    const escape = new KeyboardEvent('keydown', { key: 'Escape', bubbles: true });
    Object.defineProperty(escape, 'keyCode', { value: 27 });
    document.querySelector('[role="menu"]')!.dispatchEvent(escape);
    await settle(fixture);
    expect(document.querySelector('[role="menu"]')).toBeNull();
    expect(document.activeElement).toBe(more);
  });

  it('keeps the numbers and the chips for a screen reader only while the offer panel is open', () => {
    const { fixture, el } = render();
    const hidden = () =>
      [el.querySelector('.stats'), el.querySelector('.chips')].map((n) =>
        n?.classList.contains('mr-visually-hidden'),
      );
    expect(hidden()).toEqual([false, false]);
    fixture.componentRef.setInput('offerOpen', true);
    fixture.detectChanges();
    expect(hidden()).toEqual([true, true]);
  });
});
