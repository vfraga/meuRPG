import { TestBed } from '@angular/core/testing';
import { MAT_BOTTOM_SHEET_DATA, MatBottomSheetRef } from '@angular/material/bottom-sheet';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';

import type { ConsumableVm } from '../../core/characters/consumable-vm';
import type { ConsumableTargetVm } from '../../core/characters/consumables';
import type { ConsumableRefusal } from '../../core/play/charges-errors';
import {
  type TargetsOutcome,
  type UseAnswer,
  type UseConsumableData,
  UseConsumableSheet,
  type UseConsumableRequest,
} from './use-consumable-sheet';

const POTION: ConsumableVm = {
  kind: 'potion',
  costsAction: true,
  heal: { count: 2, sides: 4, modifier: 2, text: '2d4 + 2' },
};
const RESISTANCE: ConsumableVm = {
  kind: 'potion',
  costsAction: true,
  heal: null,
  lasting: { title: 'Resistência a fogo', length: 'por 1 hora' },
};
const OIL: ConsumableVm = { kind: 'oil', costsAction: false, heal: null };
const SCROLL: ConsumableVm = { kind: 'scroll', costsAction: false, heal: null };

const USED: UseAnswer = { refusal: null };
const refused = (lead: string, text: string, more: Partial<ConsumableRefusal> = {}): UseAnswer => ({
  refusal: { lead, text, look: 'danger', retry: false, cause: 'other', ...more },
});
const read = (
  targets: readonly ConsumableTargetVm[],
  inCombat: boolean | null = true,
  more: Partial<TargetsOutcome> = {},
): TargetsOutcome => ({ targets, inCombat, error: null, ...more });

/** The text of a node as read, without the ligature names of its icons. */
const flat = (n: Element | null | undefined) => {
  if (!n) {
    return undefined;
  }
  const copy = n.cloneNode(true) as Element;
  copy.querySelectorAll('mat-icon').forEach((i) => i.remove());
  return copy.textContent?.replace(/\s+/g, ' ').trim();
};

const BRISA: ConsumableTargetVm = {
  id: 'c2',
  name: 'Brisa',
  down: true,
  line: 'A 1,5 m · Caída',
};
const JABUTI: ConsumableTargetVm = {
  id: 'c3',
  name: 'Jabuti',
  down: false,
  line: 'A 1,5 m · De pé',
};

describe('UseConsumableSheet', () => {
  const use = vi.fn();
  const loadTargets = vi.fn();
  const close = vi.fn();
  const reread = vi.fn();
  /** What the server's targets read says of the combat, unless a spec reads its own list. */
  let serverCombat = true;

  beforeEach(() => {
    serverCombat = true;
    use.mockReset().mockResolvedValue(USED);
    loadTargets
      .mockReset()
      .mockImplementation(() => Promise.resolve(read([BRISA, JABUTI], serverCombat)));
    close.mockReset();
    reread.mockReset().mockResolvedValue({});
  });

  function render(over: Partial<UseConsumableData> = {}, phone = false) {
    const consumable = over.consumable ?? POTION;
    if (over.inCombat !== undefined && over.inCombat !== null) {
      serverCombat = over.inCombat;
    }
    const data: UseConsumableData = {
      itemName: 'Poção de cura',
      kind: consumable.kind,
      effectPt: 'Cura 2d4 + 2',
      left: 2,
      actingFor: null,
      inCombat: true,
      blockedBy: null,
      canApp: true,
      canType: true,
      preferApp: true,
      loadTargets,
      reread,
      use,
      ...over,
      consumable,
    };
    TestBed.configureTestingModule({
      providers: phone
        ? [
            { provide: MAT_BOTTOM_SHEET_DATA, useValue: data },
            { provide: MatBottomSheetRef, useValue: { dismiss: close, disableClose: false } },
          ]
        : [
            { provide: MAT_DIALOG_DATA, useValue: data },
            { provide: MatDialogRef, useValue: { close, disableClose: false } },
          ],
    });
    const fixture = TestBed.createComponent(UseConsumableSheet);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const settle = async () => {
      await fixture.whenStable();
      fixture.detectChanges();
    };
    return { el, settle, fixture };
  }

  /** The text of a button without the ligature of its icon. */
  const label = (b: HTMLButtonElement | undefined) =>
    Array.from(b?.childNodes ?? [])
      .filter(
        (n) =>
          n.nodeType === Node.TEXT_NODE ||
          (n.nodeType === Node.ELEMENT_NODE && (n as Element).tagName !== 'MAT-ICON'),
      )
      .map((n) => n.textContent)
      .join('')
      .replace(/\s+/g, ' ')
      .trim();

  it('names the potion, how many are left and what a combat costs, then its effect', async () => {
    const { el, settle } = render();
    await settle();
    expect(flat(el.querySelector('h2'))).toBe('Usar · Poção de cura');
    expect(flat(el.querySelector('.frame__sub'))).toBe('Restam 2 · em combate, gasta a ação');
    expect(flat(el.querySelector('.effect'))).toContain('Cura 2d4 + 2');
  });

  it('is "Usar por Tor" for the master, with the item in the line under it', async () => {
    const { el, settle } = render({
      actingFor: 'Tor',
      itemName: 'Poção de resistência',
      consumable: RESISTANCE,
      effectPt: 'Resistência a fogo por 1 hora',
      left: 1,
      inCombat: false,
    });
    await settle();
    expect(flat(el.querySelector('h2'))).toBe('Usar por Tor');
    expect(flat(el.querySelector('.frame__sub'))).toBe('Poção de resistência · resta 1');
  });

  it('opens with "Dar a…" outlined and "Beber" filled', async () => {
    const { el, settle } = render();
    await settle();
    const give = Array.from(el.querySelectorAll('button')).find((b) => label(b) === 'Dar a…')!;
    const drink = Array.from(el.querySelectorAll('button')).find((b) => label(b) === 'Beber')!;
    expect(give.classList).toContain('mat-mdc-outlined-button');
    expect(drink.classList).toContain('mat-mdc-unelevated-button');
    expect(el.querySelector('input[type=radio]')).toBeNull();
  });

  it('drinks at once with "Beber": no receiver, the app rolls, one key, and closes with true', async () => {
    const { el, settle } = render();
    await settle();
    Array.from(el.querySelectorAll('button'))
      .find((b) => label(b) === 'Beber')!
      .click();
    await settle();
    expect(use).toHaveBeenCalledWith(
      { target: null, roll: { inApp: true } } satisfies UseConsumableRequest,
      expect.any(String),
    );
    expect(close).toHaveBeenCalledWith(true);
  });

  it('shows no dice for a potion that heals nothing', async () => {
    const { el, settle } = render({
      consumable: RESISTANCE,
      effectPt: 'Resistência a fogo por 1 hora',
    });
    await settle();
    expect(flat(el)).not.toContain('O app rola');
    expect(flat(el)).not.toContain('Digitar os dados');
  });

  it('lists who may receive it as radio rows, the first one chosen, and says what the dice are', async () => {
    const { el, settle } = render();
    await settle();
    Array.from(el.querySelectorAll('button'))
      .find((b) => label(b) === 'Dar a…')!
      .click();
    await settle();
    expect(flat(el.querySelector('legend'))).toBe('Dar a quem está ao seu lado');
    const rows = Array.from(el.querySelectorAll('.who__row'));
    expect(rows.map(flat)).toEqual(['B Brisa A 1,5 m · Caída', 'J Jabuti A 1,5 m · De pé']);
    const radios = Array.from(el.querySelectorAll<HTMLInputElement>('input[type=radio]'));
    expect(radios.map((r) => r.checked)).toEqual([true, false]);
    expect(flat(el.querySelector('.dice'))).toContain('O app rola 2d4 + 2 para Brisa.');
    expect(flat(el.querySelector('.cost'))).toBe('A poção age em Brisa.');
    const buttons = Array.from(el.querySelectorAll('button')).map(label);
    expect(buttons).toContain('Dar a Brisa');
    expect(buttons).toContain('Beber eu mesmo');
  });

  it('follows the choice of another receiver in the button, the dice line and the cost line', async () => {
    const { el, settle, fixture } = render();
    await settle();
    Array.from(el.querySelectorAll('button'))
      .find((b) => label(b) === 'Dar a…')!
      .click();
    await settle();
    el.querySelectorAll<HTMLInputElement>('input[type=radio]')[1].click();
    fixture.detectChanges();
    expect(flat(el.querySelector('.dice'))).toContain('O app rola 2d4 + 2 para Jabuti.');
    expect(flat(el.querySelector('.cost'))).toContain('A poção age em Jabuti.');
    expect(Array.from(el.querySelectorAll('button')).map(label)).toContain('Dar a Jabuti');
  });

  it('gives to the chosen character and closes with true', async () => {
    const { el, settle } = render();
    await settle();
    Array.from(el.querySelectorAll('button'))
      .find((b) => label(b) === 'Dar a…')!
      .click();
    await settle();
    Array.from(el.querySelectorAll('button'))
      .find((b) => label(b) === 'Dar a Brisa')!
      .click();
    await settle();
    expect(use).toHaveBeenCalledWith(
      { target: BRISA, roll: { inApp: true } } satisfies UseConsumableRequest,
      expect.any(String),
    );
    expect(close).toHaveBeenCalledWith(true);
  });

  it('drinks it themselves from the list with "Beber eu mesmo"', async () => {
    const { el, settle } = render();
    await settle();
    Array.from(el.querySelectorAll('button'))
      .find((b) => label(b) === 'Dar a…')!
      .click();
    await settle();
    Array.from(el.querySelectorAll('button'))
      .find((b) => label(b) === 'Beber eu mesmo')!
      .click();
    await settle();
    expect(use).toHaveBeenCalledWith(
      { target: null, roll: { inApp: true } } satisfies UseConsumableRequest,
      expect.any(String),
    );
  });

  it('turns "Dar a…" into a dashed line and says nobody is by, with no list', async () => {
    loadTargets.mockResolvedValue(read([]));
    const { el, settle } = render();
    await settle();
    const give = Array.from(el.querySelectorAll('button')).find((b) => label(b) === 'Dar a…')!;
    expect(give.getAttribute('aria-disabled')).toBe('true');
    expect(give.classList).toContain('give--off');
    give.click();
    await settle();
    expect(el.querySelector('input[type=radio]')).toBeNull();
    expect(flat(el.querySelector('.none'))).toBe('Ninguém a 1,5 m de você para receber a poção.');
  });

  it('says out of a combat that there is nobody in the party to give it to', async () => {
    loadTargets.mockResolvedValue(read([], false));
    const { el, settle } = render({ inCombat: false });
    await settle();
    expect(flat(el.querySelector('.none'))).toBe('Ninguém mais no grupo para receber a poção.');
  });

  it('says the action once, in the subtitle, for a potion the app applies nothing of', async () => {
    const { el, settle } = render({
      effectPt: '',
      itemName: 'Poção de escalada',
      consumable: RESISTANCE,
      left: 1,
    });
    await settle();
    expect(flat(el.querySelector('.frame__sub'))).toBe('Resta 1 · em combate, gasta a ação');
    Array.from(el.querySelectorAll('button'))
      .find((b) => label(b) === 'Dar a…')!
      .click();
    await settle();
    expect(flat(el.querySelector('.cost'))).toBe('A poção age em Brisa.');
    expect(flat(el.querySelector('.frame__sub'))).toContain('gasta a ação');
  });

  it('says an oil costs no action: the master judges the time', async () => {
    const { el, settle } = render({
      effectPt: '',
      itemName: 'Óleo de afiar',
      consumable: OIL,
      left: 1,
    });
    await settle();
    expect(flat(el.querySelector('.frame__sub'))).toBe('Resta 1');
    expect(flat(el.querySelector('.cost'))).toBe(
      'Não gasta a ação: o mestre decide o tempo que isso leva.',
    );
    Array.from(el.querySelectorAll('button'))
      .find((b) => label(b) === 'Dar a…')!
      .click();
    await settle();
    expect(flat(el.querySelector('.cost'))).toBe(
      'Não gasta a ação: o mestre decide o tempo que isso leva. O óleo age em Brisa.',
    );
  });

  it('uses a scroll with one "Usar" button and no "Dar a…"', async () => {
    const { el, settle } = render({
      consumable: SCROLL,
      effectPt: '',
      itemName: 'Pergaminho de magia',
    });
    await settle();
    const labels = Array.from(el.querySelectorAll('button')).map(label);
    expect(labels).toContain('Usar');
    expect(labels).not.toContain('Dar a…');
  });

  it('does not read who may receive a scroll, and never says "receber a poção" for it', async () => {
    loadTargets.mockResolvedValue(read([]));
    const { el, settle } = render({
      consumable: SCROLL,
      effectPt: '',
      itemName: 'Pergaminho de magia',
      inCombat: false,
    });
    await settle();
    expect(loadTargets).not.toHaveBeenCalled();
    expect(el.querySelector('.none')).toBeNull();
    expect(flat(el)).not.toContain('receber a poção');
    expect(el.querySelector('.refusal')).toBeNull();
  });

  it('says out of a combat that it costs no turn, and asks for the party', async () => {
    const { el, settle } = render({ inCombat: false, actingFor: 'Tor' });
    await settle();
    expect(flat(el.querySelector('.frame__sub'))).toBe('Poção de cura · restam 2');
    expect(flat(el.querySelector('.cost'))).toBe(
      'Fora de combate: não gasta a vez. “Dar a…” lista o grupo todo.',
    );
    Array.from(el.querySelectorAll('button'))
      .find((b) => label(b) === 'Dar a…')!
      .click();
    await settle();
    expect(flat(el.querySelector('legend'))).toBe('Dar a alguém do grupo');
    expect(flat(el.querySelector('.cost'))).toBe(
      'Fora de combate, não gasta a vez. A poção age em Brisa.',
    );
  });

  it('keeps "Beber" working and says why when the list could not be read', async () => {
    loadTargets.mockResolvedValue(
      read([], null, {
        error: {
          lead: 'Sem a lista.',
          text: 'Não deu para ver quem pode receber a poção.',
          look: 'danger',
          retry: true,
          cause: 'other',
        },
      }),
    );
    const { el, settle } = render();
    await settle();
    expect(flat(el.querySelector('#use-consumable-targets-error .mr-notice__said'))).toBe(
      'Sem a lista. Não deu para ver quem pode receber a poção.',
    );
    const drink = Array.from(el.querySelectorAll('button')).find((b) => label(b) === 'Beber')!;
    expect(drink.getAttribute('aria-disabled')).not.toBe('true');
    expect(drink.disabled).toBe(false);
  });

  it('describes the dashed "Dar a…" by the error when the list could not be read', async () => {
    loadTargets.mockResolvedValue(
      read([], null, {
        error: {
          lead: 'Sem a lista.',
          text: 'Não deu para ver quem pode receber a poção.',
          look: 'danger',
          retry: true,
          cause: 'other',
        },
      }),
    );
    const { el, settle } = render();
    await settle();
    const give = Array.from(el.querySelectorAll('button')).find((b) => label(b) === 'Dar a…')!;
    expect(give.getAttribute('aria-disabled')).toBe('true');
    expect(give.classList).toContain('mr-button--off');
    const alert = el.querySelector('#use-consumable-targets-error')!;
    expect(alert.id).not.toBe('');
    expect(give.getAttribute('aria-describedby')).toBe(alert.id);
    expect(el.querySelector('.none')).toBeNull();
  });

  it('puts the line that comes after the read in a polite live region that was already there', async () => {
    let finish: (out: TargetsOutcome) => void = () => undefined;
    loadTargets.mockReturnValue(
      new Promise((resolve) => {
        finish = resolve;
      }),
    );
    const { el, settle } = render();
    await settle();
    const region = el.querySelector('[aria-live=polite]')!;
    expect(region).not.toBeNull();
    expect(region.querySelector('.none')).toBeNull();
    finish(read([]));
    await settle();
    expect(el.querySelector('[aria-live=polite]')).toBe(region);
    expect(flat(region)).toBe('Ninguém a 1,5 m de você para receber a poção.');
  });

  it('gives the dashed "Dar a…" a block icon, and the live one the open hand', async () => {
    const live = render();
    await live.settle();
    const giveOf = (el: HTMLElement) =>
      Array.from(el.querySelectorAll('button')).find((b) => label(b) === 'Dar a…')!;
    expect(giveOf(live.el).querySelector('mat-icon')?.textContent?.trim()).toBe(
      'volunteer_activism',
    );
    TestBed.resetTestingModule();
    loadTargets.mockResolvedValue(read([]));
    const empty = render();
    await empty.settle();
    expect(giveOf(empty.el).querySelector('mat-icon')?.textContent?.trim()).toBe('block');
    expect(giveOf(empty.el).classList).toContain('mr-button--off');
  });

  it('describes the dashed "Dar a…" by the line that says nobody is by', async () => {
    loadTargets.mockResolvedValue(read([]));
    const { el, settle } = render();
    await settle();
    const give = Array.from(el.querySelectorAll('button')).find((b) => label(b) === 'Dar a…')!;
    const why = el.querySelector('.none')!;
    expect(why.id).not.toBe('');
    expect(give.getAttribute('aria-describedby')).toBe(why.id);
  });

  describe('the phase of the list', () => {
    const buttons = (el: HTMLElement) => Array.from(el.querySelectorAll('button')).map(label);
    const press = async (el: HTMLElement, settle: () => Promise<void>, name: string) => {
      Array.from(el.querySelectorAll('button'))
        .find((b) => label(b) === name)!
        .click();
      await settle();
    };

    it('offers "Dar a Brisa" and "Beber eu mesmo" in a combat, and no way back', async () => {
      const { el, settle } = render();
      await settle();
      await press(el, settle, 'Dar a…');
      const foot = Array.from(el.querySelectorAll('.foot button')).map((b) =>
        label(b as HTMLButtonElement),
      );
      expect(foot).toEqual(['Beber eu mesmo', 'Dar a Brisa']);
      expect(buttons(el)).not.toContain('Voltar');
    });

    it('offers "Voltar" and "Dar a Brisa" out of a combat, and "Voltar" shows the first choice again', async () => {
      const { el, settle } = render({ inCombat: false });
      await settle();
      await press(el, settle, 'Dar a…');
      const foot = Array.from(el.querySelectorAll('.foot button')).map((b) =>
        label(b as HTMLButtonElement),
      );
      expect(foot).toEqual(['Voltar', 'Dar a Brisa']);
      await press(el, settle, 'Voltar');
      expect(el.querySelector('input[type=radio]')).toBeNull();
      expect(
        Array.from(el.querySelectorAll('.foot button')).map((b) => label(b as HTMLButtonElement)),
      ).toEqual(['Dar a…', 'Beber']);
    });

    it('offers "Voltar" when the page does not know whether there is a combat and the server says there is none', async () => {
      serverCombat = false;
      const { el, settle } = render({ inCombat: null });
      await settle();
      await press(el, settle, 'Dar a…');
      expect(
        Array.from(el.querySelectorAll('.foot button')).map((b) => label(b as HTMLButtonElement)),
      ).toEqual(['Voltar', 'Dar a Brisa']);
    });

    it('puts the focus on "Dar a…" again when "Voltar" is pressed', async () => {
      const { el, settle } = render({ inCombat: false });
      await settle();
      await press(el, settle, 'Dar a…');
      await press(el, settle, 'Voltar');
      await settle();
      const give = Array.from(el.querySelectorAll('button')).find((b) => label(b) === 'Dar a…')!;
      expect(document.activeElement).toBe(give);
    });
  });

  describe('the cost line', () => {
    it('leaves the cost line out of the first choice in a combat: the subtitle says it already', async () => {
      const { el, settle } = render();
      await settle();
      expect(flat(el.querySelector('.frame__sub'))).toBe('Restam 2 · em combate, gasta a ação');
      expect(el.querySelector('.cost')).toBeNull();
      expect(el.querySelector('app-economy-mark')).toBeNull();
    });

    it('says the action once, in the subtitle: the give phase of a combat has no mark and no bold lead', async () => {
      const { el, settle } = render();
      await settle();
      Array.from(el.querySelectorAll('button'))
        .find((b) => label(b) === 'Dar a…')!
        .click();
      await settle();
      const cost = el.querySelector('.cost')!;
      expect(cost.querySelector('app-economy-mark')).toBeNull();
      expect(cost.querySelector('strong')).toBeNull();
      expect(cost.querySelector('mat-icon')?.textContent?.trim()).toBe('info');
      expect(flat(cost)).toBe('A poção age em Brisa.');
      expect(flat(el.querySelector('.frame__sub'))).toBe('Restam 2 · em combate, gasta a ação');
    });

    it('has an info icon and no bold lead for what costs nothing or says "Fora de combate"', async () => {
      const out = render({ inCombat: false });
      await out.settle();
      const cost = out.el.querySelector('.cost')!;
      expect(cost.querySelector('mat-icon')?.textContent?.trim()).toBe('info');
      expect(cost.querySelector('strong')).toBeNull();
      expect(cost.querySelector('app-economy-mark')).toBeNull();
    });

    it('has the info icon for an oil, which costs no action', async () => {
      const { el, settle } = render({
        effectPt: '',
        itemName: 'Óleo de afiar',
        consumable: OIL,
        left: 1,
      });
      await settle();
      const cost = el.querySelector('.cost')!;
      expect(cost.querySelector('mat-icon')?.textContent?.trim()).toBe('info');
      expect(cost.querySelector('app-economy-mark')).toBeNull();
    });
  });

  describe('the list of an item with no gate', () => {
    const oil = {
      effectPt: '',
      itemName: 'Óleo de afiar',
      consumable: OIL,
      left: 1,
    };

    it('asks for anyone in the party, not for who is at the side, in a running combat', async () => {
      const { el, settle } = render(oil);
      await settle();
      Array.from(el.querySelectorAll('button'))
        .find((b) => label(b) === 'Dar a…')!
        .click();
      await settle();
      expect(flat(el.querySelector('legend'))).toBe('Dar a alguém do grupo');
    });

    it('says nobody is left in the party, not that nobody is at 1,5 m, when the list is empty', async () => {
      loadTargets.mockResolvedValue(read([]));
      const { el, settle } = render(oil);
      await settle();
      expect(flat(el.querySelector('.none'))).toBe('Ninguém mais no grupo para receber o óleo.');
    });
  });

  describe('the turn the page knows does not allow it', () => {
    const dashed = (el: HTMLElement) =>
      Array.from(el.querySelectorAll<HTMLButtonElement>('.foot button'));

    it('dashes both buttons and says the action is spent, with no call to the server', async () => {
      const { el, settle } = render({ blockedBy: 'action' });
      await settle();
      expect(dashed(el).map(label)).toEqual(['Dar a…', 'Beber']);
      for (const b of dashed(el)) {
        expect(b.getAttribute('aria-disabled')).toBe('true');
        expect(b.classList).toContain('mr-button--off');
      }
      expect(flat(el.querySelector('.none'))).toBe('Ação já usada. Você já usou a ação desta vez.');
      expect(flat(el.querySelector('.none strong'))).toBe('Ação já usada.');
      expect(el.querySelector('.none mat-icon')?.textContent?.trim()).toBe('block');
      expect(dashed(el).map((b) => b.querySelector('mat-icon')?.textContent?.trim())).toEqual([
        'volunteer_activism',
        'science',
      ]);
      expect(loadTargets).not.toHaveBeenCalled();
      dashed(el).forEach((b) => b.click());
      await settle();
      expect(use).not.toHaveBeenCalled();
      expect(el.querySelector('input[type=radio]')).toBeNull();
    });

    it('says it is not their turn', async () => {
      const { el, settle } = render({ blockedBy: 'turn' });
      await settle();
      expect(flat(el.querySelector('.none'))).toBe('Só na sua vez. Beber ou dar gasta a ação.');
      expect(dashed(el).every((b) => b.getAttribute('aria-disabled') === 'true')).toBe(true);
    });

    it('names the character the master acts for', async () => {
      const { el, settle } = render({ blockedBy: 'action', actingFor: 'Tor' });
      await settle();
      expect(flat(el.querySelector('.none'))).toBe('Ação já usada. Tor já usou a ação desta vez.');
    });

    it('describes both buttons by that line and leaves the cost line out', async () => {
      const { el, settle } = render({ blockedBy: 'turn' });
      await settle();
      const why = el.querySelector('.none')!;
      expect(why.id).not.toBe('');
      expect(dashed(el).map((b) => b.getAttribute('aria-describedby'))).toEqual([why.id, why.id]);
      expect(el.querySelector('.cost')).toBeNull();
    });

    it('still shows the effect of the item and can be closed', async () => {
      const { el, settle } = render({ blockedBy: 'action' });
      await settle();
      expect(flat(el.querySelector('.effect'))).toContain('Cura 2d4 + 2');
      el.querySelector<HTMLButtonElement>('button[aria-label="Fechar"]')!.click();
      expect(close).toHaveBeenCalled();
    });
  });

  describe('the dice (RN-18)', () => {
    it('writes one field for each die when the player types them, and sends the faces', async () => {
      const { el, settle, fixture } = render();
      await settle();
      Array.from(el.querySelectorAll('button'))
        .find((b) => label(b) === 'Dar a…')!
        .click();
      await settle();
      Array.from(el.querySelectorAll('button'))
        .find((b) => label(b) === 'Digitar os dados')!
        .click();
      fixture.detectChanges();
      const fields = Array.from(el.querySelectorAll<HTMLInputElement>('.dice input'));
      expect(fields).toHaveLength(2);
      expect(Array.from(el.querySelectorAll('.dice label')).map(flat)).toEqual([
        'Dado 1 (d4)',
        'Dado 2 (d4)',
      ]);
      expect(flat(el.querySelector('.dice'))).toContain('O app soma + 2.');
      // The sign and the number never part at the end of a line.
      expect(el.querySelector('.dice__text')?.textContent).toContain('soma +\u00a02.');
      for (const [i, v] of ['3', '2'].entries()) {
        fields[i].value = v;
        fields[i].dispatchEvent(new Event('input'));
      }
      fixture.detectChanges();
      Array.from(el.querySelectorAll('button'))
        .find((b) => label(b) === 'Dar a Brisa')!
        .click();
      await settle();
      expect(use).toHaveBeenCalledWith(
        { target: BRISA, roll: { faces: [3, 2] } } satisfies UseConsumableRequest,
        expect.any(String),
      );
    });

    it('keeps the button off until every face is of its die, and says what is wrong', async () => {
      const { el, settle, fixture } = render();
      await settle();
      Array.from(el.querySelectorAll('button'))
        .find((b) => label(b) === 'Digitar os dados')!
        .click();
      fixture.detectChanges();
      const fields = Array.from(el.querySelectorAll<HTMLInputElement>('.dice input'));
      fields[0].value = '5';
      fields[0].dispatchEvent(new Event('input'));
      fields[1].value = '2';
      fields[1].dispatchEvent(new Event('input'));
      fixture.detectChanges();
      const drink = Array.from(el.querySelectorAll('button')).find((b) => label(b) === 'Beber')!;
      expect(drink.getAttribute('aria-disabled')).toBe('true');
      expect(flat(el.querySelector('.dice'))).toContain('Digite um número de 1 a 4');
      drink.click();
      expect(use).not.toHaveBeenCalled();
    });

    it('goes back to the app with "Rolar no app"', async () => {
      const { el, settle, fixture } = render();
      await settle();
      Array.from(el.querySelectorAll('button'))
        .find((b) => label(b) === 'Digitar os dados')!
        .click();
      fixture.detectChanges();
      Array.from(el.querySelectorAll('button'))
        .find((b) => label(b) === 'Rolar no app')!
        .click();
      fixture.detectChanges();
      expect(flat(el.querySelector('.dice'))).toContain('O app rola 2d4 + 2.');
    });

    it('says a wrong face once, under the whole row, and ties every wrong field to that message', async () => {
      const { el, settle, fixture } = render();
      await settle();
      Array.from(el.querySelectorAll('button'))
        .find((b) => label(b) === 'Digitar os dados')!
        .click();
      fixture.detectChanges();
      const fields = Array.from(el.querySelectorAll<HTMLInputElement>('.dice input'));
      for (const f of fields) {
        f.value = '9';
        f.dispatchEvent(new Event('input'));
      }
      fixture.detectChanges();
      const messages = Array.from(el.querySelectorAll('.dice__err'));
      expect(messages.map(flat)).toEqual(['Digite um número de 1 a 4']);
      expect(messages[0].parentElement?.nextElementSibling).toBeNull();
      expect(fields.map((f) => f.getAttribute('aria-describedby'))).toEqual([
        messages[0].id,
        messages[0].id,
      ]);
      expect(fields.every((f) => f.getAttribute('aria-invalid') === 'true')).toBe(true);
    });

    it('says nothing while every face is right or empty, and the message waits in a live region', async () => {
      const { el, settle, fixture } = render();
      await settle();
      Array.from(el.querySelectorAll('button'))
        .find((b) => label(b) === 'Digitar os dados')!
        .click();
      fixture.detectChanges();
      const fields = Array.from(el.querySelectorAll<HTMLInputElement>('.dice input'));
      fields[0].value = '3';
      fields[0].dispatchEvent(new Event('input'));
      fixture.detectChanges();
      expect(el.querySelector('.dice__err')).toBeNull();
      expect(fields.some((f) => f.hasAttribute('aria-invalid'))).toBe(false);
      expect(el.querySelector('.dice .mr-live')?.getAttribute('role')).toBe('alert');
    });

    it('keeps the toggle as one control in the first line, whichever way the dice come', async () => {
      const { el, settle, fixture } = render();
      await settle();
      const toggle = () => el.querySelector<HTMLButtonElement>('.dice__line .dice__switch')!;
      const before = toggle();
      expect(label(before)).toBe('Digitar os dados');
      before.click();
      fixture.detectChanges();
      expect(toggle()).toBe(before);
      expect(label(toggle())).toBe('Rolar no app');
      expect(el.querySelectorAll('.dice__switch')).toHaveLength(1);
      toggle().click();
      fixture.detectChanges();
      expect(toggle()).toBe(before);
      expect(label(toggle())).toBe('Digitar os dados');
    });

    it('opens on the fields when the table plays with real dice only, with no way back to the app', async () => {
      const { el, settle } = render({ canApp: false, preferApp: false });
      await settle();
      expect(el.querySelectorAll('.dice input')).toHaveLength(2);
      expect(
        Array.from(el.querySelectorAll('button')).find((b) => label(b) === 'Rolar no app'),
      ).toBeUndefined();
    });

    it('opens on the fields for a player who prefers their own dice, with "Rolar no app" beside', async () => {
      const { el, settle } = render({ preferApp: false });
      await settle();
      expect(el.querySelectorAll('.dice input')).toHaveLength(2);
      expect(
        Array.from(el.querySelectorAll('button')).find((b) => label(b) === 'Rolar no app')
          ?.disabled,
      ).toBe(false);
    });

    it('offers no typing when the table forces the app', async () => {
      const { el, settle } = render({ canType: false });
      await settle();
      expect(flat(el.querySelector('.dice'))).toContain('O app rola 2d4 + 2.');
      expect(
        Array.from(el.querySelectorAll('button')).find((b) => label(b) === 'Digitar os dados'),
      ).toBeUndefined();
    });
  });

  it('keeps the dialog open and says why when the server refuses, and retries under the same key', async () => {
    use.mockResolvedValueOnce(
      refused('Longe demais.', 'Brisa não está mais ao seu lado. Nada foi gasto.'),
    );
    const { el, settle } = render();
    await settle();
    Array.from(el.querySelectorAll('button'))
      .find((b) => label(b) === 'Dar a…')!
      .click();
    await settle();
    Array.from(el.querySelectorAll('button'))
      .find((b) => label(b) === 'Dar a Brisa')!
      .click();
    await settle();
    expect(flat(el.querySelector('[role=alert]'))).toBe(
      'Longe demais. Brisa não está mais ao seu lado. Nada foi gasto.',
    );
    expect(close).not.toHaveBeenCalled();
    Array.from(el.querySelectorAll('button'))
      .find((b) => label(b) === 'Dar a Brisa')!
      .click();
    await settle();
    expect(use.mock.calls[1][1]).toBe(use.mock.calls[0][1]);
    expect(close).toHaveBeenCalledWith(true);
  });

  it('uses another key for another receiver', async () => {
    use.mockResolvedValueOnce(refused('Sem conexão.', 'Nada foi gasto.'));
    const { el, settle, fixture } = render();
    await settle();
    Array.from(el.querySelectorAll('button'))
      .find((b) => label(b) === 'Dar a…')!
      .click();
    await settle();
    Array.from(el.querySelectorAll('button'))
      .find((b) => label(b) === 'Dar a Brisa')!
      .click();
    await settle();
    el.querySelectorAll<HTMLInputElement>('input[type=radio]')[1].click();
    fixture.detectChanges();
    Array.from(el.querySelectorAll('button'))
      .find((b) => label(b) === 'Dar a Jabuti')!
      .click();
    await settle();
    expect(use.mock.calls[1][1]).not.toBe(use.mock.calls[0][1]);
  });

  it('says "Usando…" with a ring in place of the icon while the call is on its way and does not send twice', async () => {
    let done: (v: UseAnswer) => void = () => undefined;
    use.mockReturnValue(new Promise<UseAnswer>((r) => (done = r)));
    const { el, settle } = render();
    await settle();
    Array.from(el.querySelectorAll('button'))
      .find((b) => label(b) === 'Beber')!
      .click();
    await settle();
    const busy = Array.from(el.querySelectorAll('button')).find((b) => label(b) === 'Usando…')!;
    expect(busy.getAttribute('aria-disabled')).toBe('true');
    expect(busy.querySelector('.go__spin')).not.toBeNull();
    expect(busy.querySelector('mat-icon')).toBeNull();
    expect(busy.classList).toContain('go--busy');
    expect(busy.classList).not.toContain('mr-button--off');
    busy.click();
    expect(use).toHaveBeenCalledTimes(1);
    done(USED);
    await settle();
    expect(close).toHaveBeenCalledWith(true);
  });

  it('closes without using anything with the close button', async () => {
    const { el, settle } = render();
    await settle();
    el.querySelector<HTMLButtonElement>('button[aria-label="Fechar"]')!.click();
    expect(use).not.toHaveBeenCalled();
    expect(close).toHaveBeenCalled();
  });

  describe('an oil', () => {
    const oil = { effectPt: '', itemName: 'Óleo de afiar', consumable: OIL, left: 1 };
    const names = (el: HTMLElement) =>
      Array.from(el.querySelectorAll('.foot button')).map((b) => label(b as HTMLButtonElement));
    const press = async (el: HTMLElement, settle: () => Promise<void>, name: string) => {
      Array.from(el.querySelectorAll('button'))
        .find((b) => label(b) === name)!
        .click();
      await settle();
    };

    it('is used and not drunk: "Usar" in the first choice, with the drop of an oil', async () => {
      const { el, settle } = render(oil);
      await settle();
      expect(names(el)).toEqual(['Dar a…', 'Usar']);
      expect(names(el)).not.toContain('Beber');
      const use = Array.from(el.querySelectorAll('.foot button')).find(
        (b) => label(b as HTMLButtonElement) === 'Usar',
      )!;
      expect(use.querySelector('mat-icon')?.textContent?.trim()).toBe('water_drop');
    });

    it('says "Usar eu mesmo" and "O óleo age em Brisa." once the list is open', async () => {
      const { el, settle } = render(oil);
      await settle();
      await press(el, settle, 'Dar a…');
      expect(names(el)).toEqual(['Usar eu mesmo', 'Dar a Brisa']);
      expect(flat(el.querySelector('.cost'))).toBe(
        'Não gasta a ação: o mestre decide o tempo que isso leva. O óleo age em Brisa.',
      );
      expect(flat(el)).not.toContain('Beber');
      expect(flat(el)).not.toContain('A poção age');
    });

    it('still drinks a potion: "Beber" and "Beber eu mesmo"', async () => {
      const { el, settle } = render();
      await settle();
      expect(names(el)).toEqual(['Dar a…', 'Beber']);
      await press(el, settle, 'Dar a…');
      expect(names(el)).toEqual(['Beber eu mesmo', 'Dar a Brisa']);
    });

    it('uses it on its user with "Usar eu mesmo"', async () => {
      const { el, settle } = render(oil);
      await settle();
      await press(el, settle, 'Dar a…');
      await press(el, settle, 'Usar eu mesmo');
      expect(use).toHaveBeenCalledWith(
        { target: null, roll: { inApp: true } } satisfies UseConsumableRequest,
        expect.any(String),
      );
    });

    it('asks nothing of a bead or a dust that is no oil: no "Dar a…" and "Usar"', async () => {
      const other: ConsumableVm = { kind: 'other', costsAction: false, heal: null };
      const { el, settle } = render({ consumable: other, effectPt: '', itemName: 'Pó de ilusão' });
      await settle();
      expect(names(el)).toEqual(['Usar']);
      expect(loadTargets).not.toHaveBeenCalled();
    });
  });

  describe('one line for the case: in a combat or out of one', () => {
    it('says the action in a combat and nothing of the turn', async () => {
      const { el, settle } = render({ inCombat: null });
      await settle();
      expect(flat(el.querySelector('.frame__sub'))).toBe('Restam 2 · em combate, gasta a ação');
      expect(el.querySelector('.cost')).toBeNull();
      expect(flat(el)).not.toContain('Fora de combate');
    });

    it('says the turn is not spent out of a combat and nothing of the action', async () => {
      serverCombat = false;
      const { el, settle } = render({ inCombat: null });
      await settle();
      expect(flat(el.querySelector('.frame__sub'))).toBe('Restam 2');
      expect(flat(el.querySelector('.cost'))).toBe(
        'Fora de combate: não gasta a vez. “Dar a…” lista o grupo todo.',
      );
      expect(flat(el)).not.toMatch(/gasta a (sua )?ação/);
    });

    it('says nothing of the cost until the targets read answers, and not at all when it cannot', async () => {
      loadTargets.mockResolvedValue(read([], null));
      const { el, settle } = render({ inCombat: null });
      await settle();
      expect(flat(el.querySelector('.frame__sub'))).toBe('Restam 2');
      expect(el.querySelector('.cost')).toBeNull();
    });

    it('follows the server when the page believed there was a combat and there is none', async () => {
      loadTargets.mockResolvedValue(read([BRISA, JABUTI], false));
      const { el, settle } = render({ inCombat: true });
      await settle();
      expect(flat(el.querySelector('.frame__sub'))).toBe('Restam 2');
      expect(flat(el.querySelector('.cost'))).toContain('Fora de combate');
      expect(flat(el)).not.toContain('em combate, gasta');
    });

    it('lists the party out of a combat and who is at the side in one, by the same read', async () => {
      const out = render({ inCombat: null, actingFor: 'Tor' });
      serverCombat = true;
      await out.settle();
      Array.from(out.el.querySelectorAll('button'))
        .find((b) => label(b) === 'Dar a…')!
        .click();
      await out.settle();
      expect(flat(out.el.querySelector('legend'))).toBe('Dar a quem está ao seu lado');
    });
  });

  describe('the list is empty', () => {
    it('names the master\'s character, never "você", in a combat', async () => {
      loadTargets.mockResolvedValue(read([]));
      const { el, settle } = render({ actingFor: 'Pensantus' });
      await settle();
      expect(flat(el.querySelector('.none'))).toBe(
        'Ninguém a 1,5 m de Pensantus para receber a poção.',
      );
      expect(flat(el)).not.toContain('de você');
    });

    it('says "Ninguém mais no grupo para receber…" out of a combat, for the player and for the master', async () => {
      loadTargets.mockResolvedValue(read([], false));
      const { el, settle } = render({ inCombat: null });
      await settle();
      expect(flat(el.querySelector('.none'))).toBe('Ninguém mais no grupo para receber a poção.');
      expect(flat(el)).not.toContain('por perto');
    });
  });

  describe('a refusal', () => {
    const give = async (el: HTMLElement, settle: () => Promise<void>) => {
      Array.from(el.querySelectorAll('button'))
        .find((b) => label(b) === 'Dar a…')!
        .click();
      await settle();
      Array.from(el.querySelectorAll('button'))
        .find((b) => label(b) === 'Dar a Brisa')!
        .click();
      await settle();
    };

    it('keeps its live region on the page before the words come, and puts the lead in bold', async () => {
      use.mockResolvedValueOnce(
        refused('Brisa morreu.', 'Não dá para dar a poção a quem morreu. Nada foi gasto.'),
      );
      const { el, settle } = render();
      await settle();
      const region = el.querySelector('[role=alert]')!;
      expect(region).not.toBeNull();
      expect(region.querySelector('.mr-notice')).toBeNull();
      await give(el, settle);
      expect(el.querySelector('[role=alert]')).toBe(region);
      expect(flat(region.querySelector('strong'))).toBe('Brisa morreu.');
      expect(flat(region)).toBe(
        'Brisa morreu. Não dá para dar a poção a quem morreu. Nada foi gasto.',
      );
    });

    it('keeps a live region for the failed list on the page before it fails, too', async () => {
      const { el, settle } = render();
      await settle();
      expect(el.querySelectorAll('[role=alert]')).toHaveLength(2);
    });

    it('reads the list again and drops the choice when the receiver died, who is no longer offered', async () => {
      use.mockResolvedValueOnce(
        refused('Brisa morreu.', 'Nada foi gasto.', { cause: 'target-dead' }),
      );
      const { el, settle } = render();
      await settle();
      await give(el, settle);
      expect(loadTargets).toHaveBeenCalledTimes(2);
      const rows = Array.from(el.querySelectorAll('.who__row')).map(flat);
      expect(rows).toEqual(['J Jabuti A 1,5 m · De pé']);
      const names = Array.from(el.querySelectorAll('.foot button')).map((b) =>
        label(b as HTMLButtonElement),
      );
      expect(names).toEqual(['Beber eu mesmo', 'Dar a Jabuti']);
      expect(names).not.toContain('Dar a Brisa');
    });

    it('goes back to the first choice, with "Dar a…" dashed, when nobody is left to receive it', async () => {
      use.mockResolvedValueOnce(
        refused('Longe demais.', 'Nada foi gasto.', { cause: 'target-away' }),
      );
      const { el, settle } = render();
      await settle();
      loadTargets.mockResolvedValueOnce(read([BRISA, JABUTI]));
      await give(el, settle);
      expect(el.querySelector('input[type=radio]')).not.toBeNull();
      use.mockResolvedValueOnce(
        refused('Longe demais.', 'Brisa não está mais ao seu lado. Nada foi gasto.', {
          cause: 'target-away',
        }),
      );
      loadTargets.mockResolvedValueOnce(read([]));
      Array.from(el.querySelectorAll('button'))
        .find((b) => label(b) === 'Dar a Brisa')!
        .click();
      await settle();
      expect(el.querySelector('input[type=radio]')).toBeNull();
      const giveButton = Array.from(el.querySelectorAll('button')).find(
        (b) => label(b) === 'Dar a…',
      )!;
      expect(giveButton.getAttribute('aria-disabled')).toBe('true');
      expect(flat(el.querySelector('.none'))).toBe('Ninguém a 1,5 m de você para receber a poção.');
    });

    it('follows the count of the answer in the subtitle', async () => {
      use.mockResolvedValueOnce({
        ...refused('Longe demais.', 'x', { cause: 'target-away' }),
        left: 1,
      });
      const { el, settle } = render();
      await settle();
      await give(el, settle);
      expect(flat(el.querySelector('.frame__sub'))).toBe('Resta 1 · em combate, gasta a ação');
    });

    it('closes the sheet when the item is gone and none is left', async () => {
      use.mockResolvedValueOnce({
        ...refused('Essa poção já foi usada.', 'A ficha foi atualizada.', {
          cause: 'item-gone',
        }),
        left: 0,
        close: true,
      });
      const { el, settle } = render();
      await settle();
      Array.from(el.querySelectorAll('button'))
        .find((b) => label(b) === 'Beber')!
        .click();
      await settle();
      expect(close).toHaveBeenCalledWith(true);
    });

    it('sends the next call under a new key when the unit was replaced by another', async () => {
      use.mockResolvedValueOnce({
        ...refused('Essa poção já foi usada.', 'x', { cause: 'item-gone' }),
        left: 1,
        rebound: true,
      });
      const { el, settle } = render();
      await settle();
      const drink = () =>
        Array.from(el.querySelectorAll('button'))
          .find((b) => label(b) === 'Beber')!
          .click();
      drink();
      await settle();
      expect(flat(el.querySelector('.frame__sub'))).toBe('Resta 1 · em combate, gasta a ação');
      drink();
      await settle();
      expect(use.mock.calls[1][1]).not.toBe(use.mock.calls[0][1]);
    });

    it('keeps the same key for the same call when nothing changed', async () => {
      use.mockResolvedValueOnce(refused('Não sabemos se a unidade foi usada:', 'confira a ficha.'));
      const { el, settle } = render();
      await settle();
      const drink = () =>
        Array.from(el.querySelectorAll('button'))
          .find((b) => label(b) === 'Beber')!
          .click();
      drink();
      await settle();
      drink();
      await settle();
      expect(use.mock.calls[1][1]).toBe(use.mock.calls[0][1]);
    });

    it('says an unknown outcome and does not claim nothing was spent', async () => {
      use.mockResolvedValueOnce(refused('Não sabemos se a unidade foi usada:', 'confira a ficha.'));
      const { el, settle } = render();
      await settle();
      Array.from(el.querySelectorAll('button'))
        .find((b) => label(b) === 'Beber')!
        .click();
      await settle();
      expect(flat(el.querySelector('[role=alert]'))).toBe(
        'Não sabemos se a unidade foi usada: confira a ficha.',
      );
      expect(close).not.toHaveBeenCalled();
    });

    it('offers "Tentar de novo" for a call that never left, and sends the same call again under the same key', async () => {
      use.mockResolvedValueOnce(
        refused('Sem conexão.', 'Não deu para usar a poção. Nada foi gasto.', { retry: true }),
      );
      const { el, settle } = render();
      await settle();
      Array.from(el.querySelectorAll('button'))
        .find((b) => label(b) === 'Beber')!
        .click();
      await settle();
      const again = Array.from(el.querySelectorAll('[role=alert] button')).find(
        (b) => label(b as HTMLButtonElement) === 'Tentar de novo',
      ) as HTMLButtonElement;
      expect(again).toBeDefined();
      again.click();
      await settle();
      expect(use).toHaveBeenCalledTimes(2);
      expect(use.mock.calls[1][1]).toBe(use.mock.calls[0][1]);
      expect(use.mock.calls[1][0]).toEqual(use.mock.calls[0][0]);
      expect(close).toHaveBeenCalledWith(true);
    });

    it('has no "Tentar de novo" for a refusal that is not a lost call', async () => {
      use.mockResolvedValueOnce(refused('Longe demais.', 'Nada foi gasto.'));
      const { el, settle } = render();
      await settle();
      Array.from(el.querySelectorAll('button'))
        .find((b) => label(b) === 'Beber')!
        .click();
      await settle();
      expect(el.querySelector('[role=alert] button')).toBeNull();
    });

    it('reads the list again with "Tentar de novo" when it could not be read', async () => {
      loadTargets.mockResolvedValueOnce(
        read([], null, {
          error: {
            lead: 'Sem a lista.',
            text: 'Não deu para ver quem pode receber a poção.',
            look: 'danger',
            retry: true,
            cause: 'other',
          },
        }),
      );
      const { el, settle } = render();
      await settle();
      const again = Array.from(el.querySelectorAll('#use-consumable-targets-error button')).find(
        (b) => label(b as HTMLButtonElement) === 'Tentar de novo',
      ) as HTMLButtonElement;
      again.click();
      await settle();
      expect(loadTargets).toHaveBeenCalledTimes(2);
      expect(el.querySelector('#use-consumable-targets-error')).toBeNull();
      const give = Array.from(el.querySelectorAll('button')).find((b) => label(b) === 'Dar a…')!;
      expect(give.getAttribute('aria-disabled')).not.toBe('true');
    });

    it('blocks the sheet and shows the turn as a ban line, not as a red notice, when the server says it is not the turn', async () => {
      use.mockResolvedValueOnce(
        refused('Só na sua vez.', 'Beber ou dar gasta a ação.', { look: 'ban', block: 'turn' }),
      );
      const { el, settle } = render();
      await settle();
      Array.from(el.querySelectorAll('button'))
        .find((b) => label(b) === 'Beber')!
        .click();
      await settle();
      expect(el.querySelector('.refusal')).toBeNull();
      expect(flat(el.querySelector('.none'))).toBe('Só na sua vez. Beber ou dar gasta a ação.');
      expect(flat(el.querySelector('.none strong'))).toBe('Só na sua vez.');
      const buttons = Array.from(el.querySelectorAll<HTMLButtonElement>('.foot button'));
      expect(buttons.every((b) => b.getAttribute('aria-disabled') === 'true')).toBe(true);
      buttons.forEach((b) => b.click());
      await settle();
      expect(use).toHaveBeenCalledTimes(1);
    });
  });

  describe('the dice toggle keeps the focus', () => {
    it('puts it on the first die when "Digitar os dados" opens the fields', async () => {
      const { el, settle } = render();
      await settle();
      document.body.appendChild(el);
      Array.from(el.querySelectorAll('button'))
        .find((b) => label(b) === 'Digitar os dados')!
        .click();
      await settle();
      const first = el.querySelector<HTMLInputElement>('.dice input')!;
      expect(document.activeElement).toBe(first);
      el.remove();
    });

    it('puts it back on "Digitar os dados" when "Rolar no app" closes the fields', async () => {
      const { el, settle } = render({ preferApp: false });
      await settle();
      document.body.appendChild(el);
      Array.from(el.querySelectorAll('button'))
        .find((b) => label(b) === 'Rolar no app')!
        .click();
      await settle();
      const back = Array.from(el.querySelectorAll('button')).find(
        (b) => label(b) === 'Digitar os dados',
      )!;
      expect(document.activeElement).toBe(back);
      el.remove();
    });
  });

  describe('the footer, one for each breakpoint', () => {
    const giveIn = async (over: Partial<UseConsumableData>, phone: boolean) => {
      const r = render(over, phone);
      await r.settle();
      return r;
    };
    const foot = (el: HTMLElement) => el.querySelector<HTMLElement>('.foot')!;
    const halves = (el: HTMLElement) =>
      Array.from(foot(el).querySelectorAll('button')).map((b) => b.classList.contains('go--half'));

    it('gives the phone sheet the widths: two halves, whichever buttons the first choice has', async () => {
      const { el } = await giveIn({}, true);
      expect(foot(el).classList).toContain('foot--phone');
      expect(halves(el)).toEqual([true, true]);
    });

    it('gives the one button of an item that is not given the whole width of the phone sheet', async () => {
      const { el } = await giveIn(
        { itemName: 'Pergaminho', kind: 'scroll', consumable: SCROLL, inCombat: false },
        true,
      );
      const buttons = Array.from(foot(el).querySelectorAll('button'));
      expect(buttons.map((b) => b.classList.contains('go--wide'))).toEqual([true]);
    });

    it("right-aligns the dialog's buttons at their own width, with or without a receiver chosen", async () => {
      const { el, settle } = await giveIn({}, false);
      expect(foot(el).classList).not.toContain('foot--phone');
      Array.from(el.querySelectorAll('button'))
        .find((b) => label(b) === 'Dar a…')!
        .click();
      await settle();
      // The dialog right-aligns the buttons at their own width; the footer is no phone grid.
      expect(foot(el).classList).not.toContain('foot--phone');
    });

    it('has the footer buttons of the give phase in a combat in the order they are shown: self-use first, the receiver last', async () => {
      const { el, settle } = await giveIn({}, true);
      Array.from(el.querySelectorAll('button'))
        .find((b) => label(b) === 'Dar a…')!
        .click();
      await settle();
      const order = Array.from(foot(el).querySelectorAll('button')).map((b) => label(b));
      expect(order).toEqual(['Beber eu mesmo', 'Dar a Brisa']);
    });

    it('halves the two buttons of a receiver chosen in a combat on the phone, as it does out of one', async () => {
      const inCombat = await giveIn({}, true);
      Array.from(inCombat.el.querySelectorAll('button'))
        .find((b) => label(b) === 'Dar a…')!
        .click();
      await inCombat.settle();
      expect(foot(inCombat.el).classList).toContain('foot--phone');
      expect(halves(inCombat.el)).toEqual([true, true]);
      expect(
        Array.from(foot(inCombat.el).querySelectorAll('button')).some((b) =>
          b.classList.contains('go--wide'),
        ),
      ).toBe(false);
      TestBed.resetTestingModule();
      const out = await giveIn({ inCombat: false }, true);
      Array.from(out.el.querySelectorAll('button'))
        .find((b) => label(b) === 'Dar a…')!
        .click();
      await out.settle();
      expect(halves(out.el)).toEqual([true, true]);
    });
  });

  describe('the supporting buttons', () => {
    it('are the ink: "Dar a…", "Voltar" and "Beber eu mesmo" carry the ink class, the primary action does not', async () => {
      const { el, settle } = render();
      await settle();
      const ink = () =>
        Array.from(el.querySelectorAll<HTMLButtonElement>('.foot button')).map((b) => [
          label(b),
          b.classList.contains('mr-button--ink'),
        ]);
      expect(ink()).toEqual([
        ['Dar a…', true],
        ['Beber', false],
      ]);
      Array.from(el.querySelectorAll('button'))
        .find((b) => label(b) === 'Dar a…')!
        .click();
      await settle();
      expect(ink()).toEqual([
        ['Beber eu mesmo', true],
        ['Dar a Brisa', false],
      ]);
    });

    it('has "Voltar" in the ink out of a combat', async () => {
      const { el, settle } = render({ inCombat: false });
      await settle();
      Array.from(el.querySelectorAll('button'))
        .find((b) => label(b) === 'Dar a…')!
        .click();
      await settle();
      const back = Array.from(el.querySelectorAll('button')).find((b) => label(b) === 'Voltar')!;
      expect(back.classList).toContain('mr-button--ink');
    });
  });

  describe('a give with nobody left to receive', () => {
    it('sends nothing more once the list is read again with nobody in it', async () => {
      loadTargets
        .mockReset()
        .mockResolvedValueOnce(read([BRISA], true))
        .mockResolvedValue(read([], true));
      use.mockResolvedValueOnce(
        refused('Brisa morreu.', 'Nada foi gasto.', { cause: 'target-dead' }),
      );
      const { el, settle } = render();
      await settle();
      const press = async (name: string) => {
        Array.from(el.querySelectorAll('button'))
          .find((b) => label(b).startsWith(name))!
          .click();
        await settle();
      };
      await press('Dar a…');
      await press('Dar a Brisa');
      expect(use).toHaveBeenCalledTimes(1);
      // Nobody is left: the sheet is back to the first choice, and "Dar a…" is off.
      const dar = Array.from(el.querySelectorAll('button')).find((b) => label(b) === 'Dar a…')!;
      expect(dar.getAttribute('aria-disabled')).toBe('true');
      dar.click();
      await settle();
      expect(use).toHaveBeenCalledTimes(1);
      expect(close).not.toHaveBeenCalled();
    });
  });

  describe('what the lasting effect says', () => {
    it("says the effect's title and length as the server sends them", async () => {
      const { el, settle } = render({
        effectPt: 'Resistência a fogo por 1 hora',
        itemName: 'Poção de resistência',
        consumable: RESISTANCE,
        left: 1,
      });
      await settle();
      expect(flat(el.querySelector('.explain'))).toBe(
        'Quem bebe ganha o efeito “Resistência a fogo” por 1 hora: aparece em “Seus efeitos” e acaba sozinho.',
      );
    });

    it('writes no length of its own when the server sends none', async () => {
      const { el, settle } = render({
        effectPt: 'x',
        itemName: 'Poção de resistência',
        consumable: { ...RESISTANCE, lasting: { title: 'Resistência a fogo', length: '' } },
        left: 1,
      });
      await settle();
      expect(flat(el.querySelector('.explain'))).toBe(
        'Quem bebe ganha o efeito “Resistência a fogo”: aparece em “Seus efeitos” e acaba sozinho.',
      );
    });

    it('says nothing of a lasting effect for an item that starts none', async () => {
      const { el, settle } = render({
        effectPt: 'Conjura bola de fogo',
        itemName: 'Pergaminho',
        kind: 'scroll',
        consumable: SCROLL,
        left: 1,
        inCombat: false,
      });
      await settle();
      expect(el.querySelector('.explain')).toBeNull();
    });
  });

  describe('a unit that is gone while the sheet could not be read again', () => {
    const gone = {
      ...refused('Essa poção já foi usada.', 'A ficha foi atualizada.', { cause: 'item-gone' }),
      unread: true,
    };
    const press = async (
      el: HTMLElement,
      settle: () => Promise<void>,
      name: string,
    ): Promise<void> => {
      Array.from(el.querySelectorAll('button'))
        .find((b) => label(b) === name)!
        .click();
      await settle();
    };

    it('says what is true, offers to read again, and keeps both buttons off', async () => {
      use.mockResolvedValueOnce(gone);
      const { el, settle } = render();
      await settle();
      await press(el, settle, 'Beber');
      expect(flat(el.querySelector('.refusal .mr-notice__said'))).toBe(
        'Essa poção já foi usada. Não conseguimos reler a ficha.',
      );
      expect(flat(el.querySelector('.refusal__retry'))).toBe('Tentar de novo');
      const foot = Array.from(el.querySelectorAll<HTMLButtonElement>('.foot button'));
      expect(foot.map((b) => b.getAttribute('aria-disabled'))).toEqual(['true', 'true']);
      expect(close).not.toHaveBeenCalled();
      await press(el, settle, 'Beber');
      expect(use).toHaveBeenCalledTimes(1);
    });

    it('reads again with "Tentar de novo", and offers the unit again once a read lands', async () => {
      use.mockResolvedValueOnce(gone);
      reread.mockResolvedValueOnce({ left: 1, rebound: true });
      const { el, settle } = render();
      await settle();
      await press(el, settle, 'Beber');
      await press(el, settle, 'Tentar de novo');
      expect(reread).toHaveBeenCalledTimes(1);
      expect(use).toHaveBeenCalledTimes(1);
      // The refusal now says what is true: the unit was used and the sheet is up to date.
      expect(flat(el.querySelector('.refusal'))).toBe(
        'Essa poção já foi usada. A ficha foi atualizada.',
      );
      expect(el.querySelector('.refusal__retry')).toBeNull();
      expect(flat(el.querySelector('.frame__sub'))).toBe('Resta 1 · em combate, gasta a ação');
      const foot = Array.from(el.querySelectorAll<HTMLButtonElement>('.foot button'));
      expect(foot.map((b) => b.getAttribute('aria-disabled'))).toEqual([null, null]);
      await press(el, settle, 'Beber');
      expect(use).toHaveBeenCalledTimes(2);
    });

    it('stays off and says so again while the read fails', async () => {
      use.mockResolvedValueOnce(gone);
      reread.mockResolvedValueOnce({ unread: true });
      const { el, settle } = render();
      await settle();
      await press(el, settle, 'Beber');
      await press(el, settle, 'Tentar de novo');
      expect(reread).toHaveBeenCalledTimes(1);
      expect(flat(el.querySelector('.refusal'))).toContain('Não conseguimos reler a ficha.');
      expect(
        Array.from(el.querySelectorAll<HTMLButtonElement>('.foot button')).map((b) =>
          b.getAttribute('aria-disabled'),
        ),
      ).toEqual(['true', 'true']);
    });

    it('closes the sheet when the read lands and no unit is left', async () => {
      use.mockResolvedValueOnce(gone);
      reread.mockResolvedValueOnce({ left: 0, close: true });
      const { el, settle } = render();
      await settle();
      await press(el, settle, 'Beber');
      await press(el, settle, 'Tentar de novo');
      expect(close).toHaveBeenCalledWith(true);
    });
  });
  describe('a unit that is gone at open, with a sheet that cannot be read again', () => {
    const unreadTargets = { targets: [], inCombat: null, error: null, unread: true };
    const press = async (
      el: HTMLElement,
      settle: () => Promise<void>,
      name: string,
    ): Promise<void> => {
      Array.from(el.querySelectorAll('button'))
        .find((b) => label(b) === name)!
        .click();
      await settle();
    };

    it('says only that the sheet was not read: no reason about nobody to give to, and "Dar a…" points to the refusal', async () => {
      loadTargets.mockReset().mockResolvedValue(unreadTargets);
      const { el, settle } = render();
      await settle();
      expect(flat(el.querySelector('.refusal'))).toContain('Não conseguimos reler a ficha.');
      expect(el.querySelector('.why .none')).toBeNull();
      const give = Array.from(el.querySelectorAll<HTMLButtonElement>('.foot button')).find(
        (b) => label(b) === 'Dar a…',
      )!;
      expect(give.getAttribute('aria-disabled')).toBe('true');
      expect(give.getAttribute('aria-describedby')).toBe(el.querySelector('.refusal')?.id);
      expect(give.querySelector('mat-icon')?.textContent?.trim()).toBe('volunteer_activism');
    });

    it('says the unit was used when a read lands, as the send path does', async () => {
      loadTargets
        .mockReset()
        .mockResolvedValueOnce(unreadTargets)
        .mockResolvedValue(read([BRISA]));
      reread.mockResolvedValueOnce({ left: 1, rebound: true });
      const { el, settle } = render();
      await settle();
      await press(el, settle, 'Tentar de novo');
      expect(flat(el.querySelector('.refusal'))).toBe(
        'Essa poção já foi usada. A ficha foi atualizada.',
      );
    });
  });
});
