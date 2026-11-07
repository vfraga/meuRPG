import { TestBed } from '@angular/core/testing';
import { create } from '@bufbuild/protobuf';
import { type Mock, vi } from 'vitest';

import { DiceMode } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { ListHeldRollsResponseSchema } from '../../../../gen/meurpg/play/v1/resources_pb';
import { NOTE_LINGER_MS } from '../../../core/held-rolls/held-questions';
import {
  askedAt,
  heldAnswer,
  heldDie,
  heldLucky,
} from '../../../core/held-rolls/held-roll-testing';
import { ResourceClient } from '../../../core/resources/resources-client';
import { HeldRollsPanel } from './held-rolls-panel';

const T0 = 1_700_000_000_000;
const SECOND = 1000;

const brisa = (over = {}) =>
  heldLucky([1], 7, {
    holdId: 'h-brisa',
    rollerLabel: 'Brisa',
    ...askedAt(T0 - 6 * SECOND),
    ...over,
  });
const toren = () =>
  heldDie(11, 1, 6, { holdId: 'h-toren', rollerLabel: 'Toren', ...askedAt(T0 - 30 * SECOND) });

describe('HeldRollsPanel', () => {
  let list: Mock<(campaignId: string) => Promise<unknown>>;
  let api: { listHeldRolls: typeof list; answerHeldRoll: Mock<() => Promise<unknown>> };

  function mount(phone = false) {
    vi.stubGlobal('matchMedia', (query: string) => ({
      matches: phone && query.includes('max-width'),
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
    }));
    TestBed.configureTestingModule({ providers: [{ provide: ResourceClient, useValue: api }] });
    const fixture = TestBed.createComponent(HeldRollsPanel);
    fixture.componentRef.setInput('campaignId', 'c1');
    fixture.componentRef.setInput('diceMode', DiceMode.PLAYERS_CHOOSE);
    fixture.componentRef.setInput('tick', 0);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const flush = async () => {
      for (let i = 0; i < 4; i++) {
        await fixture.whenStable();
        fixture.detectChanges();
      }
    };
    return { fixture, el, flush };
  }

  const reply = (...offers: ReturnType<typeof heldLucky>[]) =>
    Promise.resolve(create(ListHeldRollsResponseSchema, { offers }));

  beforeEach(() => {
    vi.useFakeTimers({ now: T0, toFake: ['Date', 'setInterval', 'clearInterval'] });
    list = vi.fn<(campaignId: string) => Promise<unknown>>(() => reply(brisa(), toren()));
    api = {
      listHeldRolls: list,
      answerHeldRoll: vi.fn<() => Promise<unknown>>(() => Promise.resolve(heldAnswer())),
    };
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it('lists the questions the server lists, above the stage', async () => {
    const { el, flush } = mount();
    await flush();
    expect(list).toHaveBeenCalledWith('c1');
    const section = el.querySelector('section[aria-labelledby="held-panel-t"]');
    expect(section?.querySelector('h2')?.textContent?.trim()).toBe('Perguntas pendentes');
    const headings = Array.from(el.querySelectorAll('h3')).map((h) => h.textContent?.trim());
    expect(headings).toEqual([
      'Brisa · Tirou 1: rolar de novo?',
      'Toren · Usar a Inspiração de Bardo (d6)?',
    ]);
  });

  it('draws nothing but the polite count when nothing is pending', async () => {
    list.mockImplementation(() => reply());
    const { el, flush } = mount();
    await flush();
    expect(el.querySelector('section')).toBeNull();
    expect(el.querySelector('[role="status"]')?.textContent?.trim()).toBe('');
  });

  it('announces the count politely', async () => {
    const { el, flush } = mount();
    await flush();
    const live = el.querySelector('.mr-visually-hidden[role="status"]');
    expect(live?.getAttribute('aria-live')).toBe('polite');
    expect(live?.textContent?.trim()).toBe('2 perguntas pendentes');
  });

  it('reads again when the hint moves, and drops what the server stopped listing', async () => {
    const { fixture, el, flush } = mount();
    await flush();
    list.mockImplementation(() => reply(toren()));
    fixture.componentRef.setInput('tick', 1);
    await flush();
    expect(list).toHaveBeenCalledTimes(2);
    const open = Array.from(el.querySelectorAll('h3')).map((h) => h.textContent?.trim());
    expect(open).toEqual(['Toren · Usar a Inspiração de Bardo (d6)?']);
    // The roller answered: a polite status for a moment, then nothing.
    expect(el.textContent).toContain('Brisa respondeu.');
    vi.advanceTimersByTime(NOTE_LINGER_MS + SECOND);
    fixture.detectChanges();
    expect(el.textContent).not.toContain('Brisa respondeu.');
  });

  it('ignores a read that comes back after a newer one', async () => {
    let late: (v: unknown) => void = () => undefined;
    const { fixture, el, flush } = mount();
    await flush();
    list.mockImplementationOnce(() => new Promise((resolve) => (late = resolve)));
    fixture.componentRef.setInput('tick', 1);
    fixture.detectChanges();
    list.mockImplementation(() => reply(toren()));
    fixture.componentRef.setInput('tick', 2);
    await flush();
    late(create(ListHeldRollsResponseSchema, { offers: [brisa(), toren()] }));
    await flush();
    expect(Array.from(el.querySelectorAll('h3')).map((h) => h.textContent?.trim())).toEqual([
      'Toren · Usar a Inspiração de Bardo (d6)?',
    ]);
  });

  it('keeps what it shows when a read fails', async () => {
    const { fixture, el, flush } = mount();
    await flush();
    list.mockImplementation(() => Promise.reject(new Error('rede')));
    fixture.componentRef.setInput('tick', 1);
    await flush();
    expect(el.querySelectorAll('h3')).toHaveLength(2);
  });

  it('on a phone opens one card (the one the master can answer) and folds the others into rows', async () => {
    const { fixture, el, flush } = mount(true);
    await flush();
    expect(Array.from(el.querySelectorAll('h3')).map((h) => h.textContent?.trim())).toEqual([
      'Toren · Usar a Inspiração de Bardo (d6)?',
    ]);
    const row = el.querySelector<HTMLButtonElement>('button[aria-expanded]');
    expect(row?.getAttribute('aria-expanded')).toBe('false');
    expect(row?.textContent).toContain('Brisa · Tirou 1: rolar de novo?');
    expect(row?.textContent).toContain('você em 4 s');
    row?.click();
    fixture.detectChanges();
    expect(Array.from(el.querySelectorAll('h3')).map((h) => h.textContent?.trim())).toEqual([
      'Brisa · Tirou 1: rolar de novo?',
    ]);
    expect(el.querySelector('button[aria-expanded]')?.textContent).toContain(
      'Toren · Usar a Inspiração de Bardo (d6)?',
    );
  });
});
