import { TestBed } from '@angular/core/testing';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatDialog } from '@angular/material/dialog';
import { of } from 'rxjs';

import { SceneClient } from '../../../../core/play/scene-client';
import { SceneState } from '../../../../core/play/scene-state';
import { FakeSceneClient, masterScene, stageNpc } from '../../../../core/play/scene-testing';
import { type StageCandidate, StageRoster } from '../../../../core/play/stage-roster';
import { StageMaster } from './stage-master';
import { nextTurn } from '../../../../../test-settle';

const nbsp = ' ';
/** The text of an element as read, without the icons' ligature names ("check"). */
const flat = (e: Element | null | undefined) => {
  if (!e) {
    return undefined;
  }
  const copy = e.cloneNode(true) as Element;
  copy.querySelectorAll('mat-icon').forEach((i) => i.remove());
  return copy.textContent
    ?.replace(/\u00a0/g, ' ')
    .replace(/\s+/g, ' ')
    .trim();
};

const roster: StageCandidate[] = [
  {
    characterId: 'mira',
    name: 'Mira',
    kindLabel: 'NPC de história',
    portraitUrl: '/images/i1/thumb',
  },
  {
    characterId: 'capitao',
    name: 'Capitão Goblin',
    kindLabel: 'Inimigo',
    portraitUrl: '/images/i2/thumb',
  },
  { characterId: 'aldo', name: 'Aldo', kindLabel: 'NPC de história', portraitUrl: '' },
  { characterId: 'ivo', name: 'Barão Ivo', kindLabel: 'Boss', portraitUrl: '/images/i4/thumb' },
  { characterId: 'goblin', name: 'Goblin', kindLabel: 'Minion', portraitUrl: '/images/i5/thumb' },
];

describe('StageMaster', () => {
  const dialogOpen = vi.fn();

  async function setup(
    stage = [
      stageNpc('s-mira', 'Mira', { master: true, characterId: 'mira' }),
      stageNpc('s-capitao', 'Capitão Goblin', {
        master: true,
        characterId: 'capitao',
        speaking: true,
      }),
    ],
  ) {
    const api = new FakeSceneClient();
    api.stage = stage;
    api.names = Object.fromEntries(roster.map((c) => [c.characterId, c.name]));
    const state = new SceneState(
      () => api.get(),
      () => true,
    );
    state.apply(masterScene([], stage));
    dialogOpen
      .mockReset()
      .mockReturnValue({ afterClosed: () => of(undefined), afterDismissed: () => of(undefined) });
    TestBed.configureTestingModule({
      providers: [
        { provide: SceneClient, useValue: api },
        { provide: StageRoster, useValue: { list: () => Promise.resolve(roster) } },
        { provide: MatDialog, useValue: { open: dialogOpen } },
        { provide: MatBottomSheet, useValue: { open: dialogOpen } },
      ],
    });
    const fixture = TestBed.createComponent(StageMaster);
    fixture.componentRef.setInput('campaignId', 'c1');
    fixture.componentRef.setInput('state', state);
    fixture.detectChanges();
    await settle(fixture);
    return { fixture, api, state, el: fixture.nativeElement as HTMLElement };
  }

  async function settle(fixture: { detectChanges(): void; whenStable(): Promise<unknown> }) {
    for (let i = 0; i < 3; i++) {
      await fixture.whenStable();
      await nextTurn();
      fixture.detectChanges();
    }
  }

  const cards = (el: HTMLElement) =>
    Array.from(el.querySelectorAll<HTMLElement>('[data-stage-card]'));
  const button = (el: HTMLElement, label: string) =>
    el.querySelector<HTMLButtonElement>(`button[aria-label="${label}"]`)!;
  const putButton = (el: HTMLElement) => el.querySelector<HTMLButtonElement>('.st__put')!;

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('shows one card per NPC on the stage, with the portrait, the name, the kind and how many of 4', async () => {
    const { el } = await setup();
    expect(el.querySelector('h3')?.textContent).toBe('Em cena');
    expect(flat(el.querySelector('.st__count'))).toBe('2 de 4 em cena');
    expect(cards(el).map((c) => flat(c.querySelector('.card__name')))).toEqual([
      'Mira',
      'Capitão Goblin',
    ]);
    expect(cards(el).map((c) => flat(c.querySelector('.card__kind')))).toEqual([
      'NPC de história',
      'Inimigo',
    ]);
    expect(cards(el)[0].querySelector('app-portrait')).not.toBeNull();
    expect(flat(el.querySelector('.st__lead'))).toBe(
      'Os jogadores veem os retratos e o nome de quem está aqui, e quem fala fica na frente.',
    );
  });

  it('marks the speaker on the card: "Fala agora", pressed, with the 2px border', async () => {
    const { el } = await setup();
    const speaker = button(el, 'Capitão Goblin está com a fala. Tirar a fala');
    expect(flat(speaker)).toContain('Fala agora');
    expect(speaker.getAttribute('aria-pressed')).toBe('true');
    expect(cards(el)[1].classList.contains('card--speaking')).toBe(true);
    const other = button(el, 'Dar a fala a Mira');
    expect(flat(other)).toContain('Dar a fala');
    expect(other.getAttribute('aria-pressed')).toBe('false');
    expect(cards(el)[0].classList.contains('card--speaking')).toBe(false);
  });

  it('gives the speech to another NPC, and takes it back from the speaker', async () => {
    const { el, api, fixture, state } = await setup();
    button(el, 'Dar a fala a Mira').click();
    await settle(fixture);
    expect(api.calls).toContain('speaker mira');
    expect(state.stage().map((n) => n.speaking)).toEqual([true, false]);
    expect(button(el, 'Mira está com a fala. Tirar a fala').getAttribute('aria-pressed')).toBe(
      'true',
    );
    button(el, 'Mira está com a fala. Tirar a fala').click();
    await settle(fixture);
    expect(api.calls).toContain('speaker nobody');
    expect(state.stage().every((n) => !n.speaking)).toBe(true);
    expect(flat(el.querySelector('[role="status"]'))).toBe('Ninguém fala.');
  });

  it('takes an NPC off at once, with no question, and moves focus to the next card', async () => {
    const { el, api, fixture, state } = await setup();
    document.body.append(el);
    button(el, 'Tirar Mira de cena').click();
    await settle(fixture);
    expect(api.calls).toEqual(['take mira']);
    expect(state.stage().map((n) => n.name)).toEqual(['Capitão Goblin']);
    expect(el.querySelector('[role="alertdialog"]')).toBeNull();
    expect(flat(el.querySelector('.st__count'))).toBe('1 de 4 em cena');
    expect(document.activeElement).toBe(cards(el)[0].querySelector('button'));
    el.remove();
  });

  it('says "Ninguém em cena" with an empty stage and still offers "Pôr em cena"', async () => {
    const { el } = await setup([]);
    expect(cards(el)).toHaveLength(0);
    expect(flat(el.querySelector('.st__empty'))).toContain('Ninguém em cena');
    expect(flat(putButton(el))).toContain('Pôr em cena');
    expect(flat(el.querySelector('.st__count'))).toBe('0 de 4 em cena');
  });

  describe('"Pôr em cena" on a desktop: the list in place', () => {
    it('opens the campaign\'s NPCs under the cards, focuses its title, and "Em cena" tags the ones already there', async () => {
      const { el, fixture } = await setup();
      document.body.append(el);
      putButton(el).click();
      await settle(fixture);
      const title = el.querySelector<HTMLElement>('.pick__title')!;
      expect(title.textContent).toBe('Pôr em cena');
      expect(document.activeElement).toBe(title);
      const rows = Array.from(el.querySelectorAll('.sl__row'));
      expect(rows.map((r) => flat(r.querySelector('.sl__name')))).toEqual([
        'Mira',
        'Capitão Goblin',
        'Aldo',
        'Barão Ivo',
        'Goblin',
      ]);
      expect(rows.map((r) => flat(r.querySelector('.sl__tag')) ?? null)).toEqual([
        'Em cena',
        'Em cena',
        null,
        null,
        null,
      ]);
      expect(flat(rows[2].querySelector('.sl__kind'))).toBe('NPC de história · sem retrato');
      expect(flat(el.querySelector('.pick__lead'))).toContain('Quem entra vai para o fim da fila');
      el.remove();
    });

    it('puts an NPC on the stage at once, keeps the list open, says it in a green line and in the live region', async () => {
      const { el, api, fixture, state } = await setup();
      putButton(el).click();
      await settle(fixture);
      button(el, 'Pôr Aldo em cena').click();
      await settle(fixture);
      expect(api.calls).toContain('put aldo');
      expect(state.stage().map((n) => n.name)).toEqual(['Mira', 'Capitão Goblin', 'Aldo']);
      expect(el.querySelector('.pick')).not.toBeNull();
      expect(flat(el.querySelector('.st__entered'))).toContain('Aldo entrou na cena.');
      expect(flat(el.querySelector('[role="status"]'))).toBe('Aldo entrou na cena.');
      expect(flat(el.querySelector('.st__count'))).toBe('3 de 4 em cena');
      // The button became the "Em cena" tag, and focus went to the next one that still has a button.
      expect(el.querySelector('button[aria-label="Pôr Aldo em cena"]')).toBeNull();
    });

    it('"Fechar" closes the list and gives focus back to "Pôr em cena"', async () => {
      const { el, fixture } = await setup();
      document.body.append(el);
      putButton(el).click();
      await settle(fixture);
      el.querySelector<HTMLButtonElement>('.pick__close')!.click();
      await settle(fixture);
      expect(el.querySelector('.pick')).toBeNull();
      expect(document.activeElement).toBe(putButton(el));
      el.remove();
    });
  });

  describe('a full stage: 4 de 4', () => {
    const four = ['mira', 'capitao', 'aldo', 'ivo'].map((id) =>
      stageNpc(`s-${id}`, roster.find((c) => c.characterId === id)!.name, {
        master: true,
        characterId: id,
      }),
    );

    it('says "4 de 4" and why, before a dashed, off "Pôr em cena" that stays reachable', async () => {
      const { el, api, fixture } = await setup(four);
      expect(flat(el.querySelector('.st__count'))).toBe('4 de 4 em cena');
      expect(flat(el.querySelector('.st__reason'))).toBe(
        'A cena comporta 4 NPCs. Tire um para pôr outro.',
      );
      const put = putButton(el);
      expect(put.getAttribute('aria-disabled')).toBe('true');
      expect(put.getAttribute('aria-describedby')).toBe('st-reason');
      expect(put.disabled).toBe(false);
      put.click();
      await settle(fixture);
      expect(el.querySelector('.pick')).toBeNull();
      expect(api.calls).toEqual([]);
    });

    it('takes one off and the button comes back', async () => {
      const { el, fixture } = await setup(four);
      button(el, 'Tirar Aldo de cena').click();
      await settle(fixture);
      expect(putButton(el).getAttribute('aria-disabled')).toBe('false');
      expect(el.querySelector('.st__reason')).toBeNull();
    });

    it("turns the list's buttons into the same off, dashed button when the stage fills up", async () => {
      const three = four.slice(0, 3);
      const { el, fixture } = await setup(three);
      putButton(el).click();
      await settle(fixture);
      button(el, 'Pôr Barão Ivo em cena').click();
      await settle(fixture);
      const off = button(el, 'Pôr Goblin em cena');
      expect(off.getAttribute('aria-disabled')).toBe('true');
      expect(off.getAttribute('aria-describedby')).not.toBeNull();
      expect(flat(el.querySelector('.sl__reason'))).toBe(
        'A cena comporta 4 NPCs. Tire um para pôr outro.',
      );
    });
  });

  it('runs one write at a time per NPC: a second press of the same button does nothing', async () => {
    const { el, api, fixture } = await setup();
    putButton(el).click();
    await settle(fixture);
    let release: () => void = () => undefined;
    api.hold = new Promise<void>((r) => (release = r));
    button(el, 'Pôr Aldo em cena').click();
    button(el, 'Pôr Aldo em cena').click();
    fixture.detectChanges();
    expect(button(el, 'Pôr Aldo em cena').getAttribute('aria-disabled')).toBe('true');
    release();
    await settle(fixture);
    expect(api.calls.filter((c) => c === 'put aldo')).toHaveLength(1);
  });

  it('says what went wrong by the typed detail and keeps the stage it had', async () => {
    const { el, api, fixture, state } = await setup();
    putButton(el).click();
    await settle(fixture);
    const { Code, ConnectError } = await import('@connectrpc/connect');
    api.failWith = new ConnectError('boom', Code.Unavailable);
    button(el, 'Pôr Aldo em cena').click();
    await settle(fixture);
    expect(flat(el.querySelector('[role="alert"]'))).toContain('Não deu para mudar o palco');
    expect(state.stage()).toHaveLength(2);
  });

  describe('on a phone: "Pôr em cena" is a sheet', () => {
    it('opens the sheet with the list, not the list in place', async () => {
      vi.stubGlobal('matchMedia', (q: string) => ({
        matches: q.includes('max-width: 767'),
        addEventListener: () => undefined,
        removeEventListener: () => undefined,
      }));
      const { el, fixture } = await setup();
      putButton(el).click();
      await settle(fixture);
      expect(dialogOpen).toHaveBeenCalledTimes(1);
      const config = dialogOpen.mock.calls[0][1] as {
        ariaLabel: string;
        data: { candidates: () => StageCandidate[] };
      };
      expect(config.ariaLabel).toBe('Pôr em cena');
      expect(config.data.candidates()).toHaveLength(5);
      expect(el.querySelector('.pick')).toBeNull();
    });
  });

  it('keeps `nbsp` between a count and its words', async () => {
    const { el } = await setup();
    expect(el.querySelector('.st__count')?.textContent).toBe(`2${nbsp}de${nbsp}4 em cena`);
  });
});
