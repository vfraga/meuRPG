import { TestBed } from '@angular/core/testing';
import { By } from '@angular/platform-browser';

import { create } from '@bufbuild/protobuf';

import {
  CombatantKind,
  CombatantSide,
  EncounterMode,
  HiddenRevealQuestionSchema,
  SunlightScope,
  SpellZoneVisibility,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { combatant, encounter, spellZone } from '../../../../core/combat/combat-testing';
import { decodeVision } from '../../../../core/maps/vision';
import { visionResponse } from '../../../../core/maps/vision-testing';
import { CombatMap } from '../../../../shared/combat-map/combat-map';
import { CombatMapCard } from './combat-map-card';
import { stubFullscreen } from '../../../../core/ui/fullscreen-testing';

const plain = (t: string | null | undefined) => (t ?? '').replace(/\s+/g, ' ').trim();

function mount(inputs: Record<string, unknown> = {}) {
  TestBed.resetTestingModule();
  const fixture = TestBed.createComponent(CombatMapCard);
  fixture.componentRef.setInput(
    'encounter',
    encounter({
      currentCombatantId: 'p',
      combatants: [
        combatant({
          id: 'p',
          label: 'Pensantus',
          kind: CombatantKind.PLAYER,
          side: CombatantSide.PARTY,
        }),
      ],
    }),
  );
  fixture.componentRef.setInput('image', { url: '', width: 100, height: 100 });
  fixture.componentRef.setInput('isMaster', true);
  fixture.componentRef.setInput('mapName', 'A caverna do Vale Seco');
  for (const [name, value] of Object.entries(inputs)) {
    fixture.componentRef.setInput(name, value);
  }
  fixture.detectChanges();
  return { fixture, el: fixture.nativeElement as HTMLElement };
}

describe('CombatMapCard: "Mover mesmo assim?" (an NPC dragged past its movement)', () => {
  it('draws nothing unless the page asks', () => {
    const { el } = mount({ forcedSwitch: true });
    expect(el.querySelector('[data-over-ask]')).toBeNull();
  });

  it('says who lacks how much, in a status region that was there first, with the two answers', () => {
    const { fixture, el } = mount({ forcedSwitch: true });
    const live = el.querySelector('.mr-live[role="status"]')!;
    expect(live.textContent?.trim()).toBe('');
    fixture.componentRef.setInput('overAsk', { label: 'Espada voadora 2', missing: '4,5 m' });
    fixture.detectChanges();
    const ask = live.querySelector('[data-over-ask]')!;
    expect(ask.hasAttribute('role')).toBe(false);
    expect(plain(ask.textContent)).toContain('Espada voadora 2 não tem movimento para tanto.');
    expect(plain(ask.textContent)).toContain(
      'Faltam 4,5 m do que resta. Mover mesmo assim, sem gastar o movimento?',
    );
    const answers: string[] = [];
    fixture.componentInstance.overAskConfirm.subscribe(() => answers.push('confirm'));
    fixture.componentInstance.overAskCancel.subscribe(() => answers.push('cancel'));
    const buttons = [...el.querySelectorAll<HTMLButtonElement>('.card__ask-actions button')];
    expect(buttons.map((b) => plain(b.textContent))).toEqual(['Mover mesmo assim', 'Cancelar']);
    buttons.forEach((b) => b.click());
    expect(answers).toEqual(['confirm', 'cancel']);
  });
});

describe('CombatMapCard: "Movimento forçado"', () => {
  it('is not drawn unless the page offers it (the setup map, a player)', () => {
    const { el } = mount();
    expect(plain(el.textContent)).not.toContain('Movimento forçado');
    expect(el.querySelector('[role="status"]')).toBeNull();
  });

  it('is a box of 44 px in the head, with its explanation described and not drawn, off by default', () => {
    const { el } = mount({
      forcedSwitch: true,
      reachSwitch: { name: 'do Pensantus', on: false },
    });
    const boxes = [...el.querySelectorAll<HTMLInputElement>('label.card__check input')];
    expect(boxes.map((b) => plain(b.parentElement?.textContent))).toEqual([
      'Alcance do Pensantus',
      'Movimento forçado',
    ]);
    expect(boxes[1].checked).toBe(false);
    expect(el.querySelector('.card__head .card__switches')).not.toBeNull();
    const why = el.querySelector(`#${boxes[1].getAttribute('aria-describedby')}`)!;
    expect(plain(why.textContent)).toContain('Vale só para o próximo arrasto.');
    expect(why.classList).toContain('mr-visually-hidden');
    expect(plain(el.textContent)).not.toContain('Arraste um token');
    expect(el.querySelector('.card__state')).toBeNull();
  });

  it('on, says so in words; the box tells the page', () => {
    const { fixture, el } = mount({ forcedSwitch: true });
    const changes: boolean[] = [];
    fixture.componentInstance.forcedChange.subscribe((v) => changes.push(v));
    const box = [...el.querySelectorAll<HTMLInputElement>('label.card__check input')].pop()!;
    box.click();
    expect(changes).toEqual([true]);
    fixture.componentRef.setInput('forced', true);
    fixture.detectChanges();
    expect(box.checked).toBe(true);
    expect(plain(el.querySelector('.card__state')?.textContent)).toBe(
      'Ligado para o próximo arrasto',
    );
  });

  it('keeps the live region on the page, empty, and fills it with the result of a forced drag', () => {
    const { fixture, el } = mount({ forcedSwitch: true });
    const region = el.querySelector('.card__result')!;
    expect(plain(region.textContent)).toBe('');
    fixture.componentRef.setInput(
      'forcedNote',
      'Pensantus foi movido à força. Ninguém recebeu oferta de ataque de oportunidade.',
    );
    fixture.detectChanges();
    expect(el.querySelector('.card__result')).toBe(region);
    expect(plain(region.textContent)).toContain('Pensantus foi movido à força.');
  });

  it("draws its checkboxes with the design system's visible outline, not the browser's", () => {
    const { el } = mount({ forcedSwitch: true, reachSwitch: { name: 'do Pensantus', on: false } });
    const boxes = el.querySelectorAll('label.card__check input[type="checkbox"]');
    expect(boxes).toHaveLength(2);
    boxes.forEach((b) => expect(b.classList).toContain('mr-check-input'));
  });

  describe('the dragged token of a forced move', () => {
    type Drag = { set(v: { id: string; col: number; row: number } | null): void };
    function dragged(forced: boolean, to: { col: number; row: number }) {
      const { fixture, el } = mount({ forcedSwitch: true, forced });
      const map = fixture.debugElement.query(By.directive(CombatMap))
        .componentInstance as unknown as {
        drag: Drag;
      };
      map.drag.set({ id: 'p', ...to });
      fixture.detectChanges();
      return el;
    }

    it('marks the square the token left with a dashed ring and a dashed line from it', () => {
      const el = dragged(true, { col: 5, row: 4 });
      expect(el.querySelector('.cm__origin')).not.toBeNull();
      expect(el.querySelectorAll('.cm__path line')).toHaveLength(1);
    });

    it('draws nothing while the box is off, or while the token is still on its square', () => {
      expect(dragged(false, { col: 5, row: 4 }).querySelector('.cm__origin')).toBeNull();
      expect(dragged(true, { col: 0, row: 0 }).querySelector('.cm__origin')).toBeNull();
    });
  });
});

describe('CombatMapCard, a player on a fog map', () => {
  // 20 x 14 squares, the player knows a 4 x 3 corner: the card shows that part, not the black field around it.
  const grid = (known: string) =>
    Array.from({ length: 14 }, (_, r) =>
      r >= 5 && r < 8 ? '.'.repeat(8) + known + '.'.repeat(8) : '.'.repeat(20),
    );

  const standing = encounter({
    currentCombatantId: 'p',
    combatants: [
      combatant({ id: 'p', label: 'Pensantus', kind: CombatantKind.PLAYER, col: 9, row: 6 }),
    ],
  });

  it('crops to what the player knows, wider than the card so the tokens are readable', () => {
    const { el } = mount({
      isMaster: false,
      encounter: standing,
      fog: decodeVision(visionResponse(grid('BBBB'))),
    });
    const crop = el.querySelector<HTMLElement>('.card__crop--on');
    expect(crop).not.toBeNull();
    // 4 known columns and 3 rows, two squares around: 8 x 7 squares, shown as at least a square, of a 20-column grid.
    expect(el.querySelector<HTMLElement>('.card__shift')?.style.width).toBe('250%');
  });

  it('shows the whole map when the player knows most of it, and to the master', () => {
    const everything = Array.from({ length: 14 }, () => 'B'.repeat(20));
    expect(
      mount({ isMaster: false, fog: decodeVision(visionResponse(everything)) }).el.querySelector(
        '.card__crop--on',
      ),
    ).toBeNull();
    const master = mount({
      isMaster: true,
      encounter: standing,
      fog: decodeVision(visionResponse(grid('BBBB'))),
    });
    expect(master.el.querySelector('.card__crop--on')).toBeNull();
    expect(mount({ isMaster: false }).el.querySelector('.card__crop--on')).toBeNull();
  });
});

describe('CombatMapCard: "Área da última magia"', () => {
  const asking = () =>
    encounter({
      currentCombatantId: 'p',
      gridColumns: 10,
      gridRows: 8,
      combatants: [
        combatant({
          id: 'p',
          label: 'Pensantus',
          kind: CombatantKind.PLAYER,
          side: CombatantSide.PARTY,
        }),
      ],
      pendingHiddenReveals: [
        create(HiddenRevealQuestionSchema, {
          id: 'q1',
          casterId: 'p',
          spellKey: 'spell:fireball',
          combatantIds: ['g3'],
          area: {
            origin: { col: 4, row: 3 },
            squares: [
              { col: 4, row: 3 },
              { col: 5, row: 3 },
            ],
          },
        }),
      ],
    });

  it("draws the master's pending area and origin, with their two legend entries", () => {
    const { el } = mount({ encounter: asking() });
    expect(el.querySelector('app-area-overlay')).not.toBeNull();
    expect(el.querySelector('app-area-overlay .ao__fill')?.getAttribute('d')).toBe(
      'M4 3h1v1h-1zM5 3h1v1h-1z',
    );
    expect(el.querySelectorAll('app-area-overlay .ao__diamond')).toHaveLength(1);
    const legend = plain(el.querySelector('.legend')?.textContent);
    expect(legend).toContain('Área da última magia');
    expect(legend).toContain('Ponto de origem');
  });

  it('draws nothing for a player, and nothing without a question', () => {
    const { el } = mount({ encounter: asking(), isMaster: false });
    expect(el.querySelector('app-area-overlay')).toBeNull();
    expect(plain(el.querySelector('.legend')?.textContent)).not.toContain('Área da última magia');
    expect(mount().el.querySelector('app-area-overlay')).toBeNull();
  });
});

describe('CombatMapCard: "Tela cheia"', () => {
  let fs: ReturnType<typeof stubFullscreen> | undefined;
  afterEach(() => fs?.restore());

  const button = (el: HTMLElement) =>
    [...el.querySelectorAll<HTMLButtonElement>('.card__full')].at(0) ?? null;

  it('is "Ver mapa" in the header: the browser\'s full screen where it has one, the map\'s own screen where not', () => {
    fs = stubFullscreen(false);
    expect(plain(button(mount().el)?.textContent)).toContain('Ver mapa');
    expect(button(mount().el)!.getAttribute('aria-pressed')).toBeNull();
    fs.restore();
    fs = stubFullscreen(true);
    expect(plain(button(mount().el)?.textContent)).toContain('Ver mapa');
  });

  it('is offered to a player only where the browser has full screen, as "Tela cheia"', () => {
    fs = stubFullscreen(false);
    expect(button(mount({ isMaster: false }).el)).toBeNull();
    fs.restore();
    fs = stubFullscreen(true);
    expect(plain(button(mount({ isMaster: false }).el)?.textContent)).toContain('Tela cheia');
  });

  it('puts the card on full screen, says how to leave, and leaves on a second tap', () => {
    fs = stubFullscreen(true);
    const { fixture, el } = mount();
    fs.track(el);
    button(el)!.click();
    expect(fs.request).toHaveBeenCalledTimes(1);
    fs.enter(el);
    fixture.detectChanges();
    expect(plain(button(el)!.textContent)).toContain('Sair da tela cheia');
    expect(button(el)!.getAttribute('aria-pressed')).toBe('true');
    button(el)!.click();
    expect(fs.exit).toHaveBeenCalledTimes(1);
  });
});

describe('CombatMapCard: the zones the spells keep', () => {
  const zone = (over: Parameters<typeof spellZone>[0]) =>
    spellZone({
      squares: [
        { col: 1, row: 1 },
        { col: 2, row: 1 },
      ],
      ...over,
    });
  function withZones(zones: ReturnType<typeof spellZone>[], isMaster = true) {
    return mount({
      isMaster,
      encounter: encounter({ gridColumns: 10, gridRows: 10, combatants: [], zones }),
    });
  }

  it('draws no zone layer and no legend entry when none is on the map', () => {
    const { el } = mount();
    expect(el.querySelector('app-zone-layer')).toBeNull();
    expect(el.querySelector('.legend__zone')).toBeNull();
  });

  it('draws the layer inside the map and names each spell present in the legend', () => {
    const { el } = withZones([
      zone({ id: 'a' }),
      zone({ id: 'b', spellKey: 'spell:web', spellNamePt: 'Teia' }),
      zone({ id: 'c' }),
    ]);
    expect(el.querySelector('app-combat-map app-zone-layer')).not.toBeNull();
    const names = [...el.querySelectorAll('.legend__zone .legend__name')].map((n) =>
      plain(n.textContent),
    );
    expect(names).toEqual(['Raio Lunar', 'Teia']);
    const letters = [...el.querySelectorAll('.legend__zone')].map((li) =>
      [...li.querySelectorAll('app-zone-letter [role="img"]')].map((l) =>
        l.getAttribute('aria-label'),
      ),
    );
    expect(letters).toEqual([['Zona A', 'Zona C'], ['Zona B']]);
    // a screen reader hears "Zona A, Zona C: Raio Lunar"
    const first = el.querySelector('.legend__zone')!;
    const hidden = [...first.querySelectorAll('.mr-visually-hidden')].map((n) => n.textContent);
    expect(hidden).toEqual([', ', ': ']);
  });

  it("lists the burning side of a wall in the legend under the wall's letter", () => {
    const { el } = withZones([
      zone({
        id: 'w',
        spellKey: 'spell:wall-of-fire',
        spellNamePt: 'Muralha de Fogo',
        burningSquares: [{ col: 1, row: 0 }],
      }),
    ]);
    const names = [...el.querySelectorAll('.legend__zone .legend__name')].map((n) =>
      plain(n.textContent),
    );
    expect(names).toEqual(['Muralha de Fogo', 'Lado que queima']);
  });

  it('says "só você vê" in the legend for a hidden caster\'s spell', () => {
    const { el } = withZones([
      zone({ id: 'a', visibility: SpellZoneVisibility.MASTER_ONLY, spellNamePt: 'Nuvem Mortal' }),
    ]);
    expect(plain(el.querySelector('.legend__zone .legend__name')?.textContent)).toBe(
      'Nuvem Mortal só você vê',
    );
  });

  it('gives a player the zones the server sent, and no master-only wording', () => {
    const { el } = withZones([zone({ id: 'a' })], false);
    expect(el.querySelector('app-zone-layer')).not.toBeNull();
    expect(plain(el.querySelector('.legend__zone .legend__name')?.textContent)).toBe('Raio Lunar');
  });
});

describe("CombatMapCard: the master's header (C7-02)", () => {
  let fs: ReturnType<typeof stubFullscreen> | undefined;
  afterEach(() => fs?.restore());

  it('has "Ver mapa" with its icon in the header, and no row of "Tela cheia"', () => {
    fs = stubFullscreen(true);
    const { el } = mount({ isMaster: true });
    const button = el.querySelector<HTMLButtonElement>('.card__head .card__full')!;
    expect(plain(button.textContent)).toContain('Ver mapa');
    expect(button.querySelector('mat-icon')!.textContent).toBe('fullscreen');
    expect(plain(el.textContent)).not.toContain('Tela cheia');
  });

  it('opens the map itself where the browser has no full screen', () => {
    fs = stubFullscreen(false);
    const { fixture, el } = mount({ isMaster: true });
    let opened = 0;
    fixture.componentInstance.openMap.subscribe(() => opened++);
    el.querySelector<HTMLButtonElement>('.card__head .card__full')!.click();
    expect(opened).toBe(1);
  });

  it('reads "Personagem · NPC · Escondido (só você vê) · Vez" in the legend, with no line about dragging', () => {
    const { el } = mount({ isMaster: true });
    const words = [...el.querySelectorAll('.legend li')].map((li) => plain(li.textContent));
    expect(words.map((w) => w.replace(/^[A-Z] /, ''))).toEqual(['Personagem', 'NPC', 'Vez']);
    expect(plain(el.textContent)).not.toContain('Arraste um token');
  });
});

describe('CombatMapCard: the player’s compact card (C4-03, C6-07)', () => {
  it('has the map’s name as its title, one "Ver mapa" button, and no "Tela cheia", legend or second "Ver mapa"', () => {
    const { fixture, el } = mount({
      isMaster: false,
      compact: true,
      legend: false,
      title: 'Cripta do Corvo',
    });
    const opened: number[] = [];
    fixture.componentInstance.openMap.subscribe(() => opened.push(1));
    expect(plain(el.querySelector('h2')?.textContent)).toBe('Mapa Cripta do Corvo');
    const buttons = [...el.querySelectorAll('button')];
    expect(buttons.map((b) => b.getAttribute('aria-label') ?? plain(b.textContent))).toEqual([
      'Ver mapa',
    ]);
    expect(el.querySelector('app-map-layers-legend')).toBeNull();
    expect(plain(el.textContent)).not.toContain('Tela cheia');
    buttons[0].click();
    expect(opened).toEqual([1]);
  });

  it('keeps the full head and the "Ver mapa" button under the map for a card that is not compact', () => {
    const { el } = mount({ isMaster: false });
    expect(plain(el.textContent)).toContain('Quadrados de 1,5 m');
    expect(plain(el.querySelector('.card__open')?.textContent)).toContain('Ver mapa');
    expect(el.querySelector('.bar')).toBeNull();
  });
});

describe("CombatMapCard: the master's offer rings on a phone", () => {
  afterEach(() => vi.unstubAllGlobals());

  const phoneMount = (mode: 'offer' | 'target') => {
    vi.stubGlobal('matchMedia', (query: string) => ({
      matches: /max-width: 767/.test(query),
      media: query,
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
    }));
    TestBed.resetTestingModule();
    const fixture = TestBed.createComponent(CombatMapCard);
    const ref = fixture.componentRef;
    ref.setInput(
      'encounter',
      encounter({
        currentCombatantId: 'g',
        gridColumns: 20,
        gridRows: 14,
        combatants: [
          combatant({ id: 'g', label: 'Goblin', placed: true, col: 3, row: 5 }),
          combatant({
            id: 't',
            label: 'Toren',
            placed: true,
            col: 9,
            row: 6,
            kind: CombatantKind.PLAYER,
            side: CombatantSide.PARTY,
          }),
          combatant({
            id: 'b',
            label: 'Brisa',
            placed: true,
            col: 10,
            row: 7,
            kind: CombatantKind.PLAYER,
            side: CombatantSide.PARTY,
          }),
        ],
      }),
    );
    ref.setInput('image', { url: '', width: 2000, height: 1400 });
    ref.setInput('isMaster', true);
    ref.setInput('cropOnPhone', true);
    ref.setInput(
      'pick',
      ['t', 'b'].map((id) => ({ id, label: id, reason: '', note: '', count: 0, mode })),
    );
    fixture.detectChanges();
    return fixture.nativeElement as HTMLElement;
  };
  const shift = (el: HTMLElement) =>
    [...el.querySelectorAll<HTMLElement>('[style*="translate("]')].map((n) => n.style.transform)[0];

  it('keeps the mover, who is on turn and never "own" for the master, inside the window', () => {
    const offer = shift(phoneMount('offer'));
    const target = shift(phoneMount('target'));
    expect(offer).toBeTruthy();
    // The offer's window reaches back to the mover (column 3); the target's holds only the two candidates.
    expect(offer).not.toBe(target);
    const col = (s: string) => Number(/translate\((-?[\d.]+)%/.exec(s)![1]);
    // The window starts one square before the mover (column 3 of 20: 10%), not at the first candidate.
    expect(col(offer)).toBeCloseTo(-10);
    expect(col(target)).toBeLessThan(col(offer));
  });
});

describe('CombatMapCard, the sun (#704)', () => {
  const buttons = (el: HTMLElement) =>
    Array.from(el.querySelectorAll<HTMLButtonElement>('app-sunlight-controls button'));
  // The words of a button without its icon ligature ("check", "brush").
  const words = (b: HTMLElement) =>
    plain(
      Array.from(b.childNodes)
        .filter((n) => n.nodeType === Node.TEXT_NODE)
        .map((n) => n.textContent)
        .join(''),
    );
  const named = (el: HTMLElement, text: string) => buttons(el).find((b) => words(b) === text);
  const lit = (extra: Record<string, unknown> = {}) =>
    encounter({
      currentCombatantId: 'p',
      sunlightOn: true,
      sunlightScope: SunlightScope.SQUARES,
      sunlightSquares: [{ col: 1, row: 1 }],
      combatants: [combatant({ id: 'p', label: 'Pensantus', kind: CombatantKind.PLAYER })],
      ...extra,
    } as never);

  it('has no controls while the sun is off', () => {
    const { el } = mount();
    expect(el.querySelector('app-sunlight-controls')).toBeNull();
  });

  it('has none for a player, and none for a combat without a map', () => {
    expect(
      mount({ encounter: lit(), isMaster: false }).el.querySelector('app-sunlight-controls'),
    ).toBeNull();
    const theatre = mount({ encounter: lit({ mode: EncounterMode.THEATRE }) });
    expect(theatre.el.querySelector('app-sunlight-controls')).toBeNull();
  });

  it('offers the scope and "Pintar" while the sun is on over squares', () => {
    const { el } = mount({ encounter: lit() });
    expect(el.querySelector('[role="radiogroup"]')).not.toBeNull();
    expect(named(el, 'Onde eu pintar')!.getAttribute('aria-checked')).toBe('true');
    expect(named(el, 'Todo o mapa')!.getAttribute('aria-checked')).toBe('false');
    expect(el.querySelector('.sc__start')).not.toBeNull();
  });

  it('shows the scope as a label "Luz do sol" and one two-part segmented radio group of equal parts', () => {
    const { el } = mount({ encounter: lit() });
    expect(plain(el.querySelector('.sc__label')!.textContent)).toContain('Luz do sol');
    expect(el.querySelectorAll('[role="radiogroup"]').length).toBe(1);
    const radios = Array.from(el.querySelectorAll<HTMLElement>('[role="radio"]'));
    expect(radios.map(words)).toEqual(['Todo o mapa', 'Onde eu pintar']);
    expect(radios.map((r) => r.getAttribute('tabindex'))).toEqual(['-1', '0']);
    expect(radios[1].querySelector('mat-icon')?.textContent).toContain('check');
  });

  it('moves the scope with the arrow keys, as a radio group does', () => {
    const { fixture, el } = mount({ encounter: lit() });
    const asked = vi.fn();
    fixture.componentInstance.sunlightChange.subscribe(asked);
    named(el, 'Onde eu pintar')!.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'ArrowLeft', bubbles: true }),
    );
    expect(asked).toHaveBeenCalledWith({ scope: SunlightScope.MAP });
  });

  it('has no "Pintar" for the whole map, and asks for a scope', () => {
    const { fixture, el } = mount({ encounter: lit({ sunlightScope: SunlightScope.MAP }) });
    const asked = vi.fn();
    fixture.componentInstance.sunlightChange.subscribe(asked);
    expect(el.querySelector('.sc__start')).toBeNull();
    named(el, 'Onde eu pintar')!.click();
    expect(asked).toHaveBeenCalledWith({ scope: SunlightScope.SQUARES });
  });

  it('enters paint mode with the tool, the count and "Pronto"', () => {
    const { fixture, el } = mount({ encounter: lit() });
    el.querySelector<HTMLButtonElement>('.sc__start')!.click();
    fixture.detectChanges();
    expect(fixture.componentInstance.painting()).toBe(true);
    expect(plain(el.querySelector('.sc__status')!.textContent)).toBe(
      'Pintando luz do sol · 1 quadrado',
    );
    expect(el.querySelector('.sc__card')).not.toBeNull();
    expect(
      Array.from(el.querySelectorAll('.sc__hints li')).map((l) => plain(l.textContent)),
    ).toEqual(['Um dedo: arrasta e pinta', 'Dois dedos: rola o mapa']);
    expect(el.querySelector('[role="radiogroup"]')).toBeNull();
    const row = el.querySelector('.sc__row--paint')!;
    expect(row.querySelector('.sc__seg--tools')).not.toBeNull();
    expect(row.querySelector('.sc__done')).not.toBeNull();
    expect(el.querySelector('.sl__pad')).not.toBeNull();
    el.querySelector<HTMLButtonElement>('[aria-label^="Apagar"]')!.click();
    fixture.detectChanges();
    expect(el.querySelector('[aria-label^="Apagar"]')!.getAttribute('aria-pressed')).toBe('true');
    el.querySelector<HTMLButtonElement>('.sc__done')!.click();
    fixture.detectChanges();
    expect(fixture.componentInstance.painting()).toBe(false);
    expect(el.querySelector('.sl__pad')).toBeNull();
  });

  it('leaves paint mode when the sun goes off', () => {
    const { fixture } = mount({ encounter: lit() });
    fixture.componentRef.setInput('painting', true);
    fixture.detectChanges();
    fixture.componentRef.setInput('encounter', lit({ sunlightOn: false }));
    fixture.detectChanges();
    expect(fixture.componentInstance.painting()).toBe(false);
  });

  it('names the sun in the legend for squares and for the whole map, only for the master', () => {
    const legend = (el: HTMLElement) => plain(el.querySelector('.legend')?.textContent);
    expect(legend(mount({ encounter: lit() }).el)).toContain('Luz do sol (só você vê)');
    expect(legend(mount({ encounter: lit({ sunlightScope: SunlightScope.MAP }) }).el)).toContain(
      'Luz do sol (só você vê)',
    );
    expect(legend(mount().el)).not.toContain('Luz do sol');
    expect(legend(mount({ encounter: lit(), isMaster: false }).el)).not.toContain('Luz do sol');
  });

  describe('on a phone, over the cropped preview', () => {
    afterEach(() => vi.unstubAllGlobals());

    it('keeps the controls, and "Pintar" swaps the preview for the whole map until "Pronto"', () => {
      vi.stubGlobal('matchMedia', (query: string) => ({
        matches: /max-width: 767/.test(query),
        media: query,
        addEventListener: () => undefined,
        removeEventListener: () => undefined,
      }));
      TestBed.resetTestingModule();
      const fixture = TestBed.createComponent(CombatMapCard);
      fixture.componentRef.setInput('encounter', lit());
      fixture.componentRef.setInput('image', { url: '', width: 2000, height: 1400 });
      fixture.componentRef.setInput('isMaster', true);
      fixture.componentRef.setInput('cropOnPhone', true);
      fixture.detectChanges();
      const el = fixture.nativeElement as HTMLElement;
      expect(el.querySelector('.card__preview')).not.toBeNull();
      expect(el.querySelector('app-sunlight-layer')).toBeNull();
      el.querySelector<HTMLButtonElement>('[aria-label="Pintar luz do sol no mapa"]')!.click();
      fixture.detectChanges();
      expect(fixture.componentInstance.painting()).toBe(true);
      expect(el.querySelector('.card__preview')).toBeNull();
      expect(el.querySelector('app-sunlight-layer')).not.toBeNull();
      el.querySelector<HTMLButtonElement>('.sc__done')!.click();
      fixture.detectChanges();
      expect(el.querySelector('.card__preview')).not.toBeNull();
    });
  });
});
