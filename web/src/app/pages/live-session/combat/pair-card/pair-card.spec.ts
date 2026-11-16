import { TestBed } from '@angular/core/testing';

import { CombatantKind } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { DisabledReasonCode } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { combatant } from '../../../../core/combat/combat-testing';
import { PHONE_QUERY } from '../../../../shared/map-view/media-query';
import { PairCard } from './pair-card';

const flat = (n: Element | null | undefined) => n?.textContent?.replace(/\s+/g, ' ').trim() ?? '';

const tor = combatant({
  id: 'tor',
  label: 'Tor',
  kind: CombatantKind.PLAYER,
  mine: true,
  mountedOnId: 'h',
  bonusActionUsed: true,
});
const horse = (over: object = {}) =>
  combatant({
    id: 'h',
    label: 'Cavalo de guerra',
    riderId: 'tor',
    mountKind: 'controlled',
    movementLeftFt: 60,
    ...over,
  });

const three = (enabled: boolean, over: object = {}) =>
  ({
    options: {
      standardActions: ['standard:dash', 'standard:disengage', 'standard:dodge'].map((key) => ({
        action: { key, namePt: '' },
        enabled,
        reason: enabled ? undefined : { code: DisabledReasonCode.COMBATANT_DOWN },
      })),
      ...over,
    },
  }) as never;

describe('PairCard (#588, boards 588-04 and 588-05)', () => {
  function setup(
    opts: {
      options?: unknown;
      phone?: boolean;
      busy?: boolean;
      mount?: ReturnType<typeof horse>;
    } = {},
  ) {
    onTestFinished(() => void vi.unstubAllGlobals());
    vi.stubGlobal('matchMedia', (query: string) => ({
      matches: !!opts.phone && query === PHONE_QUERY,
      media: query,
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
    }));
    TestBed.resetTestingModule();
    const fixture = TestBed.createComponent(PairCard);
    fixture.componentRef.setInput('rider', tor);
    fixture.componentRef.setInput('mount', opts.mount ?? horse());
    fixture.componentRef.setInput('options', 'options' in opts ? opts.options : three(true));
    fixture.componentRef.setInput('busy', !!opts.busy);
    const acted: string[] = [];
    fixture.componentInstance.act.subscribe((k) => acted.push(k));
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const buttons = () => [...el.querySelectorAll<HTMLButtonElement>('.pair__act')];
    return { fixture, el, buttons, acted };
  }

  it("draws the head, the rider's line with its three shapes, the mount's line and the three actions (588-04)", () => {
    const { el, buttons } = setup();
    expect(flat(el.querySelector('.pair__head'))).toBe(
      'linkTor e Cavalo de guerra · turno conjunto',
    );
    const rider = el.querySelector('.pair__rider')!;
    expect(flat(rider.querySelector('.pair__name'))).toBe('Tor');
    const riderShapes = rider.querySelector('app-economy-shapes')!;
    expect(riderShapes.getAttribute('aria-label') ?? flat(riderShapes)).toBeTruthy();
    expect(flat(riderShapes)).toContain('Ação');
    expect(flat(riderShapes)).toContain('Bônus');
    expect(flat(riderShapes)).toContain('Reação');
    expect(flat(el.querySelector('.pair__mount .pair__name'))).toBe('Cavalo de guerra');
    expect(buttons().map((b) => flat(b.querySelector('.mdc-button__label')))).toEqual([
      'Disparada',
      'Desengajar',
      'Esquivar',
    ]);
  });

  it('on a phone is the mount alone: no head and no rider line, which the row above it already says (588-05)', () => {
    const { el } = setup({ phone: true });
    expect(el.querySelector('.pair__head')).toBeNull();
    expect(el.querySelector('.pair__rider')).toBeNull();
    expect(flat(el.querySelector('.pair__mount .pair__name'))).toBe('Cavalo de guerra');
    // With no visible head, the section is named by its own label.
    expect(el.querySelector('section')?.getAttribute('aria-label')).toBe(
      'Tor e Cavalo de guerra, turno conjunto',
    );
  });

  it("names each action as the mount's, unlike the rider's own Dash", () => {
    const { buttons } = setup();
    expect(buttons().map((b) => b.getAttribute('aria-label'))).toEqual([
      'Disparada do Cavalo de guerra',
      'Desengajar do Cavalo de guerra',
      'Esquivar do Cavalo de guerra',
    ]);
    const mare = setup({ mount: horse({ label: 'Égua' }) });
    expect(mare.buttons()[0].getAttribute('aria-label')).toBe('Disparada da Égua');
  });

  it("reads the mount's economy from its options, and from its own row when the options carry none", () => {
    const fromOptions = setup({
      options: three(true, { economy: { action: { used: true }, movement: { leftFt: 40 } } }),
    });
    const shapes = fromOptions.el.querySelector('.pair__mount app-economy-shapes')!;
    expect(flat(shapes)).toContain('12,0 m');
    expect(shapes.innerHTML).toContain('Ação usada');
    const fromRow = setup({ mount: horse({ actionUsed: false, movementLeftFt: 30 }) });
    const row = fromRow.el.querySelector('.pair__mount app-economy-shapes')!;
    expect(flat(row)).toContain('9,0 m');
    expect(row.innerHTML).toContain('Ação disponível');
    const spent = setup({ mount: horse({ actionUsed: true, movementLeftFt: 30 }) });
    expect(spent.el.querySelector('.pair__mount app-economy-shapes')!.innerHTML).toContain(
      'Ação usada',
    );
  });

  it('labels the shapes with the article of the mount', () => {
    const { el } = setup();
    expect(el.querySelector('.pair__mount app-economy-shapes')?.innerHTML).toContain(
      'O que o Cavalo de guerra tem',
    );
    const mare = setup({ mount: horse({ label: 'Égua' }) });
    expect(mare.el.querySelector('.pair__mount app-economy-shapes')?.innerHTML).toContain(
      'O que a Égua tem',
    );
  });

  it('draws the token of a creature for a steed of the player, and of an NPC for an NPC', () => {
    const npc = setup().el.querySelector('.pair__mount app-combatant-token')!;
    expect(npc.classList.contains('tk--npc')).toBe(true);
    expect(npc.classList.contains('tk--creature')).toBe(false);
    const steed = setup({ mount: horse({ kind: CombatantKind.CREATURE, controlledByMe: true }) });
    const token = steed.el.querySelector('.pair__mount app-combatant-token')!;
    expect(token.classList.contains('tk--npc')).toBe(false);
    expect(token.classList.contains('tk--creature')).toBe(true);
  });

  it('before the options are read says so, with no made-up economy; the three wait dashed, with no reason', () => {
    const { el, buttons } = setup({ options: null });
    expect(el.querySelector('.pair__mount app-economy-shapes')).toBeNull();
    expect(flat(el.querySelector('.pair__loading'))).toBe('Carregando…');
    expect(el.querySelector('.pair__loading')?.getAttribute('role')).toBe('status');
    // The region is in the card whether or not it has words, so the words are announced when they come.
    const read = setup();
    expect(read.el.querySelector('.pair__loading')?.getAttribute('role')).toBe('status');
    expect(flat(read.el.querySelector('.pair__loading'))).toBe('');
    expect(buttons().every((b) => b.classList.contains('mr-button--off'))).toBe(true);
    expect(buttons().every((b) => !b.hasAttribute('aria-describedby'))).toBe(true);
    expect(el.querySelector('#pair-why')).toBeNull();
  });

  it('points a refused action at the reason, which is said in a polite live region', () => {
    const { el, buttons } = setup({ options: three(false) });
    const why = el.querySelector('#pair-why')!;
    expect(flat(why)).toBe('blockCaído: o mestre conduz o Cavalo de guerra.');
    expect(why.closest('[role=status]')).not.toBeNull();
    expect(buttons().map((b) => b.getAttribute('aria-describedby'))).toEqual([
      'pair-why',
      'pair-why',
      'pair-why',
    ]);
    expect(
      setup()
        .buttons()
        .every((b) => !b.hasAttribute('aria-describedby')),
    ).toBe(true);
    // The live region is there before the reason arrives, so the reason is announced.
    expect(setup().el.querySelector('.pair [role=status]')).not.toBeNull();
  });

  it('emits the key of the action pressed, and nothing while busy or for a refused one', () => {
    const ok = setup();
    ok.buttons()[1].click();
    expect(ok.acted).toEqual(['standard:disengage']);
    const busy = setup({ busy: true });
    busy.buttons()[0].click();
    // The guard of the handler itself, not only the disabled attribute.
    (busy.fixture.componentInstance as unknown as { press(a: object): void }).press({
      key: 'standard:dash',
      off: false,
    });
    expect(busy.acted).toEqual([]);
    const off = setup({ options: three(false) });
    off.buttons()[0].click();
    expect(off.acted).toEqual([]);
  });

  it('fits three in a row at 390 px and in the 368 px rail: each as wide as its words, sharing what is left, on one line', () => {
    const { el, buttons } = setup();
    const row = getComputedStyle(el.querySelector('.pair__actions')!);
    expect(row.display).toBe('flex');
    expect(row.gap).toBe('6px');
    // Under about 360 px (320 is supported) the three do not fit: the row wraps, and no label is cut.
    expect(row.flexWrap).toBe('wrap');
    for (const b of buttons()) {
      const s = getComputedStyle(b);
      expect(s.flexGrow).toBe('1');
      expect(s.flexBasis).toBe('auto');
      expect(s.minWidth).toBe('0px');
      expect(s.whiteSpace).toBe('nowrap');
      expect(s.paddingLeft).toBe('6px');
      expect(s.paddingRight).toBe('6px');
      expect(s.getPropertyValue('--mat-button-outlined-label-text-size').trim()).toBe(
        'var(--mr-font-13)',
      );
    }
  });
});
