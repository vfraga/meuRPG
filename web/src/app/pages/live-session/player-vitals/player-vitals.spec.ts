import { TestBed } from '@angular/core/testing';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatDialog } from '@angular/material/dialog';
import { of } from 'rxjs';
import { provideRouter } from '@angular/router';

import { DiceMode, DicePreference } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import type { VitalsVm } from '../../../core/play/live-session.types';
import type { HitDiceSheetData } from '../hit-dice-sheet/hit-dice-sheet';
import { pensantusVitals } from '../../../core/play/live-testing';
import { PlayerVitals } from './player-vitals';

describe('PlayerVitals as a beast (MR-037, E9-11)', () => {
  function render(over: Parameters<typeof pensantusVitals>[0], beastAc: number | null) {
    TestBed.configureTestingModule({ providers: [provideRouter([])] });
    const fixture = TestBed.createComponent(PlayerVitals);
    fixture.componentRef.setInput(
      'vitals',
      pensantusVitals({ name: 'Sálvia', hitPointsCurrent: 38, hitPointsMax: 38, ...over }),
    );
    fixture.componentRef.setInput('sheet', { armorClass: 12, summary: 'Druida 5', senses: [] });
    fixture.componentRef.setInput('campaignId', 'camp');
    fixture.componentRef.setInput('beastAc', beastAc);
    fixture.componentRef.setInput('compact', true);
    fixture.detectChanges();
    return fixture.nativeElement as HTMLElement;
  }
  const flat = (n: Element | null) => n?.textContent?.replace(/\s+/g, ' ').trim();

  it('shows the two reserves where the hit points box is, the beast\'s armor class, and "Sem magias" in place of the slots', () => {
    const el = render(
      {
        wildShape: {
          beastKey: 'monster:wolf',
          beastNamePt: 'Lobo',
          hitPointsCurrent: 11,
          hitPointsMax: 11,
        },
      },
      13,
    );
    expect(Array.from(el.querySelectorAll('app-wild-pools .pool'), flat)).toEqual([
      'PV do Lobo11 de 11',
      'PV da Sálvia38 de 38',
    ]);
    expect(el.querySelector('.hp')).toBeNull();
    // One armor class on the page, the beast's: not the druid's own 12.
    expect(flat(el.querySelector('.shield'))).toContain('13');
    expect(flat(el.querySelector('.shield'))).not.toContain('12');
    expect(flat(el.querySelector('.shield .stats__label'))).toBe('CA do Lobo');
    expect(el.querySelector('.slots')).toBeNull();
    expect(flat(el.querySelector('.nospells'))).toBe('blockSem magias na forma de fera.');
  });

  it('is the ordinary card otherwise', () => {
    const el = render({}, null);
    expect(el.querySelector('app-wild-pools')).toBeNull();
    expect(flat(el.querySelector('.shield'))).toContain('12');
    expect(el.querySelector('.slots')).not.toBeNull();
  });
});

describe('PlayerVitals under Escudo Arcano and Ajuda (PM-03a)', () => {
  const flat = (n: Element | null | undefined) => n?.textContent?.replace(/\s+/g, ' ').trim();

  function mount(over: Parameters<typeof pensantusVitals>[0], bonus: number | null) {
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({ providers: [provideRouter([])] });
    const fixture = TestBed.createComponent(PlayerVitals);
    fixture.componentRef.setInput('vitals', pensantusVitals(over));
    fixture.componentRef.setInput('sheet', { armorClass: 13, summary: 'Mago 5', senses: [] });
    fixture.componentRef.setInput('campaignId', 'camp');
    fixture.componentRef.setInput('compact', true);
    fixture.componentRef.setInput('armorClassBonus', bonus);
    fixture.detectChanges();
    return fixture;
  }

  it('shows the armor class already summed, the small sum under it and the label under the cards', () => {
    const el = mount({}, 5).nativeElement as HTMLElement;
    expect(flat(el.querySelector('.shield__number'))).toBe('18');
    expect(flat(el.querySelector('.shield__sum'))).toBe('13 + 5');
    const label = el.querySelector('.effects[role="status"] .effect--shield');
    expect(flat(label)).toContain('Escudo Arcano +5 até a sua vez');
  });

  it('is the plain shield, with no sum and no label, without the spell', () => {
    const el = mount({}, 0).nativeElement as HTMLElement;
    expect(flat(el.querySelector('.shield__number'))).toBe('13');
    expect(el.querySelector('.shield__sum')).toBeNull();
    expect(el.querySelector('.effect')).toBeNull();
  });

  it('says once, politely, that the shield ended with the base class, and stops after six seconds', () => {
    vi.useFakeTimers();
    try {
      const fixture = mount({}, 5);
      const el = fixture.nativeElement as HTMLElement;
      expect(el.querySelector('.ended')).toBeNull();
      fixture.componentRef.setInput('armorClassBonus', 0);
      fixture.detectChanges();
      expect(flat(el.querySelector('.ended-live[role="status"] .ended'))).toContain(
        'O Escudo Arcano acabou. A sua CA voltou a 13.',
      );
      expect(flat(el.querySelector('.shield__number'))).toBe('13');
      vi.advanceTimersByTime(5999);
      fixture.detectChanges();
      expect(el.querySelector('.ended')).not.toBeNull();
      vi.advanceTimersByTime(1);
      fixture.detectChanges();
      expect(el.querySelector('.ended')).toBeNull();
    } finally {
      vi.useRealTimers();
    }
  });

  it('does not say the shield ended when the combat or the combatant went away', () => {
    const fixture = mount({}, 5);
    fixture.componentRef.setInput('armorClassBonus', null);
    fixture.detectChanges();
    expect((fixture.nativeElement as HTMLElement).querySelector('.ended')).toBeNull();
  });

  it('hangs "+5 de Ajuda" off the maximum, stripes the bar and reads the number for a screen reader', () => {
    const el = mount({ hitPointsCurrent: 43, hitPointsMax: 43, hitPointsMaxBonus: 5 }, 0)
      .nativeElement as HTMLElement;
    expect(flat(el.querySelector('.hp__max'))).toBe('de 43');
    expect(flat(el.querySelector('.hp__top .hp__aid'))).toBe('+5 de Ajuda');
    expect(flat(el.querySelector('.effect--aid'))).toContain(
      'Ajuda: +5 nos PV até o mestre encerrar ou um descanso longo',
    );
    expect(flat(el.querySelector('.hp .mr-visually-hidden'))).toBe(
      'Pontos de vida: 43 de 43, 5 de Ajuda',
    );
    expect(el.querySelector('.hp__bar')?.getAttribute('aria-label')).toBe(
      '43 de 43 pontos de vida',
    );
    const [solid, striped] = Array.from(el.querySelectorAll<HTMLElement>('.hp__fill'));
    expect(parseFloat(solid.style.width)).toBeCloseTo((38 / 43) * 100);
    expect(parseFloat(striped.style.width)).toBeCloseTo((5 / 43) * 100);
  });

  it("leaves the striped part empty when the hit points are below the sheet's maximum", () => {
    const el = mount({ hitPointsCurrent: 31, hitPointsMax: 43, hitPointsMaxBonus: 5 }, 0)
      .nativeElement as HTMLElement;
    const [solid, striped] = Array.from(el.querySelectorAll<HTMLElement>('.hp__fill'));
    expect(parseFloat(solid.style.width)).toBeCloseTo((31 / 43) * 100);
    expect(parseFloat(striped.style.width)).toBe(0);
  });

  it('writes +10 for a third-level Ajuda', () => {
    const el = mount({ hitPointsCurrent: 48, hitPointsMax: 48, hitPointsMaxBonus: 10 }, 0)
      .nativeElement as HTMLElement;
    expect(flat(el.querySelector('.hp__aid'))).toBe('+10 de Ajuda');
    expect(flat(el.querySelector('.effect--aid'))).toContain('Ajuda: +10 nos PV');
  });

  it('writes nothing about Ajuda without it', () => {
    const plain = mount({}, 0).nativeElement as HTMLElement;
    expect(plain.querySelector('.hp__aid')).toBeNull();
    expect(plain.querySelectorAll('.hp__fill')).toHaveLength(1);
  });

  it("writes the sheet's own maximum small under the label while Ajuda is on, and not without it", () => {
    const el = mount({ hitPointsCurrent: 43, hitPointsMax: 43, hitPointsMaxBonus: 5 }, 0)
      .nativeElement as HTMLElement;
    expect(flat(el.querySelector('.hp__sheet'))).toBe('máximo 38 da ficha');
    expect(mount({}, 0).nativeElement.querySelector('.hp__sheet')).toBeNull();
  });
});

describe('PlayerVitals at 0 hit points and the Ajuda that wakes (PM-03a)', () => {
  const flat = (n: Element | null | undefined) => n?.textContent?.replace(/\s+/g, ' ').trim();

  function mount(
    over: Parameters<typeof pensantusVitals>[0],
    saves: { successes: number; failures: number } | null,
  ) {
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({ providers: [provideRouter([])] });
    const fixture = TestBed.createComponent(PlayerVitals);
    fixture.componentRef.setInput(
      'vitals',
      pensantusVitals({ name: 'Toren', hitPointsMax: 40, ...over }),
    );
    fixture.componentRef.setInput('sheet', { armorClass: 18, summary: 'Guerreiro 4', senses: [] });
    fixture.componentRef.setInput('campaignId', 'camp');
    fixture.componentRef.setInput('compact', true);
    fixture.componentRef.setInput('ownDeathSaves', saves);
    fixture.detectChanges();
    return fixture;
  }
  const pills = (f: ReturnType<typeof mount>) =>
    Array.from((f.nativeElement as HTMLElement).querySelectorAll('.effects .effect'), flat);

  it('says "Inconsciente" and the death save counts of the combatant at 0', () => {
    const f = mount({ hitPointsCurrent: 0 }, { successes: 1, failures: 1 });
    expect(pills(f)).toEqual(['Inconsciente', 'Testes contra a morte: 1 sucesso, 1 falha']);
    expect((f.nativeElement as HTMLElement).querySelector('.effects')?.getAttribute('role')).toBe(
      'status',
    );
  });

  it('says only "Inconsciente" when no combat tells the counts, and nothing above 0', () => {
    expect(pills(mount({ hitPointsCurrent: 0 }, null))).toEqual(['Inconsciente']);
    expect(pills(mount({ hitPointsCurrent: 12 }, { successes: 0, failures: 0 }))).toEqual([]);
  });

  it('tells a character at 0 in a combat, off turn, when the death save is rolled, and not on turn', () => {
    const hint = (f: ReturnType<typeof mount>) =>
      flat((f.nativeElement as HTMLElement).querySelector('.hint'));
    const f = mount({ hitPointsCurrent: 0 }, { successes: 0, failures: 0 });
    expect(hint(f)).toBe('Você rola o teste contra a morte no começo da sua vez.');
    f.componentRef.setInput('onTurn', true);
    f.detectChanges();
    expect(hint(f)).toBeUndefined();
    expect(hint(mount({ hitPointsCurrent: 12 }, { successes: 0, failures: 0 }))).toBeUndefined();
    expect(hint(mount({ hitPointsCurrent: 0 }, null))).toBeUndefined();
  });

  it('says the character woke when Ajuda takes the hit points from 0 to above 0, then lets it go', () => {
    vi.useFakeTimers();
    try {
      const f = mount({ hitPointsCurrent: 0, revision: 1 }, { successes: 1, failures: 1 });
      f.componentRef.setInput(
        'vitals',
        pensantusVitals({
          name: 'Toren',
          hitPointsCurrent: 5,
          hitPointsMax: 45,
          hitPointsMaxBonus: 5,
          revision: 2,
        }),
      );
      f.detectChanges();
      expect(pills(f)).toEqual([
        'favorite_borderAjuda: +5 nos PV até o mestre encerrar ou um descanso longo',
        'checkAcordado · testes contra a morte zerados',
      ]);
      expect(flat((f.nativeElement as HTMLElement).querySelector('.effect--ok'))).toContain(
        'Acordado',
      );
      vi.advanceTimersByTime(30000);
      f.detectChanges();
      expect(pills(f)).toHaveLength(1);
    } finally {
      vi.useRealTimers();
    }
  });

  it('does not say it woke for a plain heal, or for hit points that were never 0', () => {
    const heal = mount({ hitPointsCurrent: 0, revision: 1 }, null);
    heal.componentRef.setInput(
      'vitals',
      pensantusVitals({ name: 'Toren', hitPointsCurrent: 5, hitPointsMax: 40, revision: 2 }),
    );
    heal.detectChanges();
    expect(pills(heal)).toEqual([]);
    const aid = mount({ hitPointsCurrent: 10, hitPointsMax: 40, revision: 1 }, null);
    aid.componentRef.setInput(
      'vitals',
      pensantusVitals({
        name: 'Toren',
        hitPointsCurrent: 15,
        hitPointsMax: 45,
        hitPointsMaxBonus: 5,
        revision: 2,
      }),
    );
    aid.detectChanges();
    expect(pills(aid).some((p) => p?.includes('Acordado'))).toBe(false);
  });
});

describe('PlayerVitals hit dice by size (decisions batch 2 B, 6)', () => {
  function render(over: Parameters<typeof pensantusVitals>[0]) {
    TestBed.configureTestingModule({ providers: [provideRouter([])] });
    const fixture = TestBed.createComponent(PlayerVitals);
    fixture.componentRef.setInput('vitals', pensantusVitals(over));
    fixture.componentRef.setInput('campaignId', 'camp');
    fixture.detectChanges();
    return fixture.nativeElement as HTMLElement;
  }
  const flat = (n: Element | null) => n?.textContent?.replace(/\s+/g, ' ').trim();

  it('says the dice by size and what is left of each', () => {
    const el = render({
      hitDice: '5d10 e 1d6',
      hitDiceSizes: [
        { faces: 10, total: 5, used: 2 },
        { faces: 6, total: 1, used: 0 },
      ],
      hitDiceTotal: 6,
      hitDiceUsed: 2,
    });
    const box = Array.from(el.querySelectorAll('.box')).find((b) =>
      flat(b)?.startsWith('Dados de vida'),
    );
    expect(flat(box?.querySelector('.box__value') ?? null)).toBe('5d10 e 1d6');
    expect(flat(box?.querySelector('.box__words') ?? null)).toBe('Restam 3 de 5d10 e 1 de 1d6');
  });

  it('says one size as before: what is left of how many', () => {
    const el = render({});
    expect(flat(el.querySelector('.box__words'))).toBe('Restam 2 de 3d6');
  });
});

describe('PlayerVitals "Gastar dados de vida" (decisions batch 2 B 6)', () => {
  const dialogOpen = vi.fn();
  const sheetOpen = vi.fn();

  beforeEach(() => {
    dialogOpen.mockReset().mockReturnValue({ afterClosed: () => of(true) });
    sheetOpen.mockReset().mockReturnValue({ afterDismissed: () => of(true) });
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        { provide: MatDialog, useValue: { open: dialogOpen } },
        { provide: MatBottomSheet, useValue: { open: sheetOpen } },
      ],
    });
  });

  function render(over: Parameters<typeof pensantusVitals>[0] = {}, compact = false) {
    const fixture = TestBed.createComponent(PlayerVitals);
    fixture.componentRef.setInput('vitals', pensantusVitals(over));
    fixture.componentRef.setInput('campaignId', 'camp');
    fixture.componentRef.setInput('compact', compact);
    fixture.componentRef.setInput('diceMode', DiceMode.PHYSICAL);
    fixture.componentRef.setInput('dicePreference', DicePreference.PHYSICAL);
    const changes: VitalsVm[] = [];
    fixture.componentInstance.vitalsChange.subscribe((v) => changes.push(v));
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const button = () => el.querySelector<HTMLButtonElement>('.js-hit-dice')!;
    return { fixture, el, button, changes };
  }

  /** What the page handed the sheet, from whichever container opened it. */
  const opened = () =>
    (dialogOpen.mock.calls[0] ?? sheetOpen.mock.calls[0])[1].data as HitDiceSheetData;

  it('offers the button on the card, with dice left', () => {
    const { button } = render();
    expect(button().textContent).toContain('Gastar dados de vida');
    expect(button().disabled).toBe(false);
    expect(button().classList.contains('mr-button--off')).toBe(false);
  });

  it("is not on the combat's compact card: a rest does not happen in a combat", () => {
    const { button } = render({}, true);
    expect(button()).toBeNull();
  });

  it('opens the sheet with the campaign, the dice setting and the live vitals', () => {
    const { button, fixture } = render();
    button().click();
    expect(dialogOpen.mock.calls.length + sheetOpen.mock.calls.length).toBe(1);
    const data = opened();
    expect(data.campaignId).toBe('camp');
    expect(data.diceMode).toBe(DiceMode.PHYSICAL);
    expect(data.preference).toBe(DicePreference.PHYSICAL);
    expect(data.vitals().characterId).toBe('pensantus');
    // The same signal as the card's: what the stream brings shows in the sheet.
    fixture.componentRef.setInput('vitals', pensantusVitals({ hitPointsCurrent: 3, revision: 9 }));
    expect(data.vitals().hitPointsCurrent).toBe(3);
  });

  it('hands the vitals of each die to the page', () => {
    const { button, changes } = render();
    button().click();
    const next = pensantusVitals({ revision: 2 });
    opened().apply(next);
    expect(changes).toEqual([next]);
  });

  it('is dashed and says why when every die is used, and opens nothing', () => {
    const { el, button } = render({
      hitDiceSizes: [{ faces: 6, total: 3, used: 3 }],
      hitDiceUsed: 3,
    });
    expect(button().classList.contains('mr-button--off')).toBe(true);
    expect(button().getAttribute('aria-describedby')).toBe('dice-reason');
    expect(el.querySelector('#dice-reason')?.textContent).toContain(
      'Não há dados de vida para gastar.',
    );
    button().click();
    expect(dialogOpen).not.toHaveBeenCalled();
    expect(sheetOpen).not.toHaveBeenCalled();
  });
});
