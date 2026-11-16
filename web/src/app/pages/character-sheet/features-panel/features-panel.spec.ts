import { TestBed } from '@angular/core/testing';

import { GearVm } from '../../../core/characters/gear';
import { FullSheetVm, ItemModifierVm } from '../character-sheet.types';
import { FeaturesPanel } from './features-panel';

const SHIELD_HINT = {
  sourceKey: 'equipment:shield',
  text: 'O escudo não conta na CA enquanto uma arma de duas mãos está equipada: guarde a arma de duas mãos.',
};
const OTHER_HINT = { sourceKey: 'feature:unarmored-defense', text: 'Sem armadura.' };

const shield = (bonus?: number): GearVm =>
  ({ key: 'shield', namePt: 'Escudo', kind: 'shield', bonus }) as GearVm;

const shieldMod = (reasonPt: string): ItemModifierVm => ({
  target: 'ac',
  value: 1,
  sourceKey: 'equipment:shield',
  sourceNamePt: 'Escudo',
  counts: false,
  reasonPt,
});

function reminders(over: Partial<FullSheetVm>): string[] {
  const fixture = TestBed.createComponent(FeaturesPanel);
  fixture.componentRef.setInput('sheet', {
    kind: 'full',
    features: [],
    hints: [SHIELD_HINT, OTHER_HINT],
    gear: [],
    itemModifiers: [],
    shieldIdle: false,
    resistances: [],
    languages: [],
    proficiencies: [],
    ...over,
  } as unknown as FullSheetVm);
  fixture.detectChanges();
  return Array.from((fixture.nativeElement as HTMLElement).querySelectorAll('.hints li')).map(
    (li) => li.textContent!.trim(),
  );
}

describe('FeaturesPanel "Lembretes"', () => {
  it('lists every hint while the shield is not idle', () => {
    expect(reminders({})).toEqual([SHIELD_HINT.text, OTHER_HINT.text]);
  });

  it("leaves the idle shield's hint out when the combat column's note says it", () => {
    expect(reminders({ shieldIdle: true, gear: [shield()] })).toEqual([OTHER_HINT.text]);
  });

  it("leaves it out when the shield's own line says why", () => {
    expect(
      reminders({
        shieldIdle: true,
        gear: [shield(1)],
        itemModifiers: [shieldMod('Uma arma de duas mãos equipada deixa o escudo parado.')],
      }),
    ).toEqual([OTHER_HINT.text]);
  });

  it('keeps it when the combat column says no reason for the shield', () => {
    expect(
      reminders({ shieldIdle: true, gear: [shield(1)], itemModifiers: [shieldMod('')] }),
    ).toEqual([SHIELD_HINT.text, OTHER_HINT.text]);
  });
});

function panelRows(features: object[]): HTMLElement[] {
  const fixture = TestBed.createComponent(FeaturesPanel);
  fixture.componentRef.setInput('sheet', {
    kind: 'full',
    features,
    hints: [],
    gear: [],
    itemModifiers: [],
    shieldIdle: false,
    resistances: [],
    languages: [],
    proficiencies: [],
  } as unknown as FullSheetVm);
  fixture.detectChanges();
  return Array.from(
    (fixture.nativeElement as HTMLElement).querySelectorAll<HTMLElement>('.feature'),
  );
}

const featureRow = (name: string, over: object = {}) => ({
  name,
  sourcePt: 'Talento',
  description: '',
  descriptionPt: '',
  descriptionPtMissing: false,
  descriptionPtOnly: true,
  summaryPt: '',
  optionOf: '',
  lost: false,
  ...over,
});

describe('FeaturesPanel, a feat or option that does not apply now (board 580-05)', () => {
  it('marks the lost row with the pending "Não vale agora" tag, and only that row', () => {
    const rows = panelRows([
      featureRow('Golpe do Corvo', { optionOf: 'Truques de lâmina' }),
      featureRow('Asa Partida', { optionOf: 'Truques de lâmina', lost: true }),
    ]);
    expect(rows[0].querySelector('.feature__lost')).toBeNull();
    const tag = rows[1].querySelector('.feature__lost');
    expect(tag?.textContent?.trim()).toBe('Não vale agora');
    expect(tag?.classList).toContain('mr-tag--pending');
    expect(tag?.classList).toContain('mr-tag--compact');
  });

  it("draws the lost row's name muted, a row that applies as it was", () => {
    const rows = panelRows([featureRow('Postura Firme', { lost: true }), featureRow('Fúria')]);
    expect(rows[0].querySelector('.feature__name')?.classList).toContain('feature__name--lost');
    expect(rows[1].querySelector('.feature__name')?.classList).not.toContain('feature__name--lost');
  });
});

describe('FeaturesPanel, an option row (board 580-05)', () => {
  it('says "Opção de <feature>" under the name and has no source on the right, for every option', () => {
    const rows = panelRows([
      featureRow('Golpe do Corvo', { optionOf: 'Truques de lâmina' }),
      featureRow('Asa Partida', { optionOf: 'Truques de lâmina', lost: true }),
    ]);
    for (const row of rows) {
      expect(row.querySelector('.feature__source')).toBeNull();
      expect(row.querySelector('.feature__sub')?.textContent?.trim()).toBe(
        'Opção de Truques de lâmina',
      );
      expect(row.classList).toContain('feature--option');
    }
  });

  it('keeps the source on the right and no sub-line for a row that is not an option', () => {
    const [row] = panelRows([featureRow('Recuperação Arcana', { sourcePt: 'Mago 1' })]);
    expect(row.querySelector('.feature__source')?.textContent?.trim()).toBe('Mago 1');
    expect(row.querySelector('.feature__sub')).toBeNull();
    expect(row.classList).not.toContain('feature--option');
  });
});
