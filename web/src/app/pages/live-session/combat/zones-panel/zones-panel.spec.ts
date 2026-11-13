import { TestBed } from '@angular/core/testing';

import {
  SpellZoneMovement,
  SpellZoneVisibility,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { SpellAreaShape } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { combatant, spellZone } from '../../../../core/combat/combat-testing';
import { zoneViews } from '../../../../core/combat/zones';
import { encounter } from '../../../../core/combat/combat-testing';
import { ZonesPanel } from './zones-panel';

const plain = (t: string | null | undefined) => (t ?? '').replace(/\s+/g, ' ').trim();

function mount(zones: ReturnType<typeof spellZone>[], over: { master?: boolean } = {}) {
  const master = over.master ?? true;
  const views = zoneViews(
    encounter({
      combatants: [
        combatant({ id: 'tor', label: 'Tor' }),
        combatant({ id: 'jabuti', label: 'Jabuti' }),
      ],
      zones,
    }),
    master,
  );
  TestBed.resetTestingModule();
  const fixture = TestBed.createComponent(ZonesPanel);
  fixture.componentRef.setInput('zones', views);
  fixture.componentRef.setInput('master', master);
  fixture.detectChanges();
  const el = fixture.nativeElement as HTMLElement;
  const setZones = (next: ReturnType<typeof spellZone>[]) => {
    fixture.componentRef.setInput(
      'zones',
      zoneViews(encounter({ combatants: [], zones: next }), master),
    );
    fixture.detectChanges();
  };
  return { fixture, el, setZones };
}

const click = (el: Element | null, f: { detectChanges: () => void }) => {
  (el as HTMLElement).click();
  f.detectChanges();
};
const button = (el: HTMLElement, text: string) =>
  [...el.querySelectorAll('button')].find((b) => plain(b.textContent).includes(text)) ?? null;

const more = (el: HTMLElement, i: number) =>
  el.querySelectorAll<HTMLButtonElement>('.zp__card .zp__more')[i];
const words = (i: Element) => {
  const copy = i.cloneNode(true) as HTMLElement;
  copy.querySelectorAll('mat-icon').forEach((m) => m.remove());
  return plain(copy.textContent);
};
const items = () => [...document.querySelectorAll('[role="menuitem"]')].map(words);
const menuText = (i: Element) => plain(i.textContent);
async function open(
  f: { detectChanges: () => void; whenStable: () => Promise<unknown> },
  el: HTMLElement,
  i: number,
) {
  more(el, i).click();
  f.detectChanges();
  await f.whenStable();
  f.detectChanges();
}
async function choose(
  f: { detectChanges: () => void; whenStable: () => Promise<unknown> },
  text: string,
) {
  [...document.querySelectorAll<HTMLElement>('[role="menuitem"]')]
    .find((i) => menuText(i).includes(text))!
    .click();
  f.detectChanges();
  await f.whenStable();
  f.detectChanges();
  await f.whenStable();
}

describe('ZonesPanel', () => {
  it('gives each card its letter, in the diamond and for a screen reader', () => {
    const { el } = mount([spellZone({ id: 'a' }), spellZone({ id: 'b', spellNamePt: 'Teia' })]);
    const cards = el.querySelectorAll('.zp__card');
    const names = [...cards].map((c) =>
      c.querySelector('.zp__letter [role="img"]')?.getAttribute('aria-label'),
    );
    expect(names).toEqual(['Zona A', 'Zona B']);
    const named = cards[1].querySelector('.zp__letter [role="img"]');
    expect(named?.getAttribute('aria-label')).toBe('Zona B');
  });

  it('draws nothing for an empty list', () => {
    const { el } = mount([]);
    expect(el.querySelector('section')).toBeNull();
    expect(plain(el.textContent)).toBe('');
  });

  it('is a section named by its heading, with the zones as a list', () => {
    const { el } = mount([spellZone({ id: 'a' }), spellZone({ id: 'b', spellNamePt: 'Teia' })]);
    const section = el.querySelector('section')!;
    const h = el.querySelector('#' + section.getAttribute('aria-labelledby'))!;
    expect(plain(h.textContent)).toBe('Zonas ativas (2)');
    expect(section.querySelectorAll('ul > li').length).toBe(2);
  });

  it('reads a card: title, area, save and damage, rounds, concentration and who is inside', () => {
    const { el } = mount([spellZone({ id: 'a', occupantIds: ['tor', 'jabuti'] })]);
    const card = plain(el.querySelector('li')!.textContent);
    expect(card).toContain('Raio Lunar · Bartolo');
    expect(card).toContain(
      'Cilindro de 1,5 m de raio · Constituição CD 14 · 2d10 radiante, metade se passar',
    );
    expect(card).toContain('restam 9 rodadas');
    expect(card).toContain('Concentração de Bartolo');
    expect(card).toContain('Aqui: Tor, Jabuti');
  });

  it('says "sem teste", the move per turn and the notes', () => {
    const { el } = mount([
      spellZone({
        id: 'a',
        spellNamePt: 'Nuvem Mortal',
        shape: SpellAreaShape.SPHERE,
        sizeFt: 20,
        saveAbility: '',
        movesPerTurnFt: 10,
        notes: ['restrains'],
      }),
    ]);
    const card = plain(el.querySelector('li')!.textContent);
    expect(card).toContain('anda 3 m por turno');
    expect(card).toContain('sem teste');
    expect(card).toContain('contido');
  });

  it('never prints "restam 0": a zone with no count has no chip', () => {
    const { el } = mount([spellZone({ id: 'a', roundsLeft: 0 })]);
    expect(plain(el.textContent)).not.toContain('restam');
  });

  it('shows a zone with hidden details as name, area and caster only', () => {
    const { el } = mount(
      [
        spellZone({
          id: 'a',
          detailsHidden: true,
          saveAbility: '',
          damageDice: '',
          roundsLeft: 0,
          occupantIds: ['tor'],
        }),
      ],
      { master: false },
    );
    const card = el.querySelector('li')!;
    expect(card.querySelector('[role="img"]')?.getAttribute('aria-label')).toBe('Zona A');
    expect(plain(card.querySelector('.zp__name')?.textContent)).toBe('Raio Lunar · Bartolo');
    expect(plain(card.querySelector('.zp__detail')?.textContent)).toBe('Cilindro de 1,5 m de raio');
    expect(el.querySelector('button')).toBeNull();
  });

  it('marks the zone of a hidden caster "Só você vê"', () => {
    const { el } = mount([spellZone({ id: 'a', visibility: SpellZoneVisibility.MASTER_ONLY })]);
    expect(plain(el.querySelector('li')!.textContent)).toContain('Só você vê');
  });

  it('has one "Mais ações" button per card, named for the zone, and no other action button in the card', () => {
    const { el } = mount([spellZone({ id: 'a' }), spellZone({ id: 'b', spellNamePt: 'Teia' })]);
    const buttons = [...el.querySelectorAll('.zp__card button')];
    expect(buttons).toHaveLength(2);
    const label = el.querySelector('.zp__name')!.firstChild!.textContent!.trim();
    expect(buttons[0].getAttribute('aria-label')).toBe(`Mais ações de ${label}`);
    expect(plain(buttons[0].textContent)).toContain('Mais ações');
    expect(buttons[0].getAttribute('aria-haspopup')).toBe('menu');
  });

  it('offers "Mover" in the menu for a zone that moves by an action, and says which zone to the page', async () => {
    const { fixture, el } = mount([spellZone({ id: 'a' })]);
    const seen: string[] = [];
    fixture.componentInstance.move.subscribe((id: string) => seen.push(id));
    await open(fixture, el, 0);
    expect(items()).toEqual(['Mover', 'Encerrar zona']);
    await choose(fixture, 'Mover');
    expect(seen).toEqual(['a']);
  });

  it('offers only "Encerrar zona" for a zone that does not move', async () => {
    const { fixture, el } = mount([
      spellZone({ id: 'b', spellNamePt: 'Teia', movement: SpellZoneMovement.FIXED }),
    ]);
    await open(fixture, el, 0);
    expect(items()).toEqual(['Encerrar zona']);
  });

  it("shows no button at all in a player's panel", () => {
    const { el } = mount([spellZone({ id: 'a' })], { master: false });
    expect(el.querySelector('.zp__card button')).toBeNull();
  });

  it('asks before ending a zone, and "Não" closes the question without a call and gives the focus back to "Mais ações"', async () => {
    const { fixture, el } = mount([spellZone({ id: 'a' })]);
    document.body.appendChild(el);
    const ended: string[] = [];
    fixture.componentInstance.end.subscribe((id: string) => ended.push(id));
    await open(fixture, el, 0);
    await choose(fixture, 'Encerrar zona');
    expect(plain(el.textContent)).toContain('Encerrar a zona?');
    expect(document.activeElement).toBe(button(el, 'Não'));
    click(button(el, 'Não'), fixture);
    await fixture.whenStable();
    expect(plain(el.textContent)).not.toContain('Encerrar a zona?');
    expect(ended).toEqual([]);
    expect(document.activeElement).toBe(more(el, 0));
    el.remove();
  });

  it('ends the zone only on "Sim, encerrar", and says a refusal in place', async () => {
    const { fixture, el } = mount([
      spellZone({ id: 'a' }),
      spellZone({ id: 'b', spellNamePt: 'Teia' }),
    ]);
    const ended: string[] = [];
    fixture.componentInstance.end.subscribe((id: string) => ended.push(id));
    await open(fixture, el, 1);
    await choose(fixture, 'Encerrar zona');
    expect(el.querySelectorAll('.zp__confirm').length).toBe(1);
    fixture.componentRef.setInput('error', 'Só o mestre encerra uma zona.');
    fixture.detectChanges();
    expect(el.querySelector('[role="alert"]')?.textContent).toContain('Só o mestre encerra');
    click(button(el, 'Sim, encerrar'), fixture);
    expect(ended).toEqual(['b']);
  });

  it('holds the button while a call is in the air', () => {
    const { fixture, el } = mount([spellZone({ id: 'a' })]);
    fixture.componentRef.setInput('busy', true);
    fixture.detectChanges();
    expect((more(el, 0) as HTMLButtonElement).disabled).toBe(true);
  });

  it('keeps every button at least 44 px tall', () => {
    const { el } = mount([spellZone({ id: 'a' })]);
    for (const b of el.querySelectorAll('button')) {
      expect(b.classList.contains('zp__btn')).toBe(true);
    }
  });

  it('announces a zone created or ended in a polite region, not on the first read', async () => {
    const { fixture, el, setZones } = mount([spellZone({ id: 'a' })]);
    const live = el.querySelector('[aria-live="polite"]')!;
    await fixture.whenStable();
    expect(plain(live.textContent)).toBe('');
    setZones([spellZone({ id: 'a' }), spellZone({ id: 'b', spellNamePt: 'Teia' })]);
    await fixture.whenStable();
    fixture.detectChanges();
    expect(plain(live.textContent)).toBe('Zona criada: B, Teia · Bartolo.');
    setZones([]);
    await fixture.whenStable();
    fixture.detectChanges();
    expect(plain(live.textContent)).toContain('Zonas encerradas:');
  });

  it('keeps the live region when the list is empty, so the first zone is announced', async () => {
    const { fixture, el, setZones } = mount([]);
    const live = el.querySelector('[aria-live="polite"]')!;
    setZones([spellZone({ id: 'a' })]);
    await fixture.whenStable();
    fixture.detectChanges();
    expect(plain(live.textContent)).toBe('Zona criada: A, Raio Lunar · Bartolo.');
  });
});
