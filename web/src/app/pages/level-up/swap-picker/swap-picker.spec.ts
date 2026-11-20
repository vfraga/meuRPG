import { TestBed } from '@angular/core/testing';
import { create } from '@bufbuild/protobuf';

import {
  LevelUpNamedKeySchema,
  LevelUpOptionsSchema,
  LevelUpSwapGroupSchema,
} from '../../../../gen/meurpg/characters/v1/characters_pb';
import { LevelUpSwap, plainName } from '../../../core/levelup/levelup-swap';
import { SwapPicker } from './swap-picker';

const named = (key: string, namePt: string) => create(LevelUpNamedKeySchema, { key, namePt });

const FEATURE = 'feature:infusoes@mesa';

/** The table rule's group and one table feature's group, as a level with the ability step offers both. */
function bothOf(): LevelUpSwap {
  const group = (key: string, namePt: string, from: string, to: string[]) =>
    create(LevelUpSwapGroupSchema, {
      group: key,
      namePt,
      picks: [named(`${key}:${from}`, from)],
      pool: to.map((n) => named(`${key}:${n}`, n)),
    });
  return new LevelUpSwap(
    create(LevelUpOptionsSchema, {
      swapGroups: [
        group('fighting_style', 'Estilo de luta', 'Defesa', ['Duelismo']),
        group('cantrip', 'Truque', 'Luz', ['Chama']),
        group(FEATURE, 'Infusões', 'Escudo', ['Radiante', 'Bumerangue']),
        group('feature:outra@mesa', 'Outra', 'X', ['Y']),
      ],
    }),
  );
}

function swapOf(withGroups = true): LevelUpSwap {
  return new LevelUpSwap(
    create(LevelUpOptionsSchema, {
      swapGroups: withGroups
        ? [
            create(LevelUpSwapGroupSchema, {
              group: 'fighting_style',
              namePt: 'Estilo de luta',
              picks: [named('feature:fighter-fighting-style-defense', 'Defesa')],
              pool: [
                named('feature:fighter-fighting-style-dueling', 'Duelismo'),
                named('option:fighting_style:guarda-dupla@mesa', 'Guarda Dupla'),
              ],
            }),
          ]
        : [],
    }),
  );
}

function render(swap: LevelUpSwap) {
  const fixture = TestBed.createComponent(SwapPicker);
  fixture.componentRef.setInput('swap', swap);
  fixture.detectChanges();
  return fixture;
}

const radio = (el: HTMLElement, name: string, label: string) =>
  Array.from(el.querySelectorAll<HTMLInputElement>(`input[name="${name}"]`)).find((i) =>
    i.closest('label')?.textContent?.includes(label),
  )!;

describe('LevelUpSwap and SwapPicker', () => {
  it('has nothing to send and no groups when the table does not use the rule', () => {
    const swap = swapOf(false);
    expect(swap.groups()).toEqual([]);
    expect(swap.pick()).toBeUndefined();
  });

  it('hides the picks until a group is chosen', () => {
    const el = render(swapOf()).nativeElement as HTMLElement;
    expect(el.querySelector('input[name="swap-from"]')).toBeNull();
    expect(radio(el, 'swap-group', 'Não trocar').checked).toBe(true);
  });

  it('puts the swap in the choices and lists it', () => {
    const swap = swapOf();
    const fixture = render(swap);
    const el = fixture.nativeElement as HTMLElement;
    radio(el, 'swap-group', 'Estilo de luta').click();
    fixture.detectChanges();
    radio(el, 'swap-from', 'Defesa').click();
    fixture.detectChanges();
    expect(swap.pick()).toBeUndefined();
    radio(el, 'swap-to', 'Guarda Dupla').click();
    fixture.detectChanges();
    expect(swap.pick()).toEqual({
      group: 'fighting_style',
      fromKey: 'feature:fighter-fighting-style-defense',
      toKey: 'option:fighting_style:guarda-dupla@mesa',
    });
    expect(swap.summary()).toBe('Trocar Defesa por Guarda Dupla');
  });

  it('"Não trocar" clears the swap', () => {
    const swap = swapOf();
    const fixture = render(swap);
    const el = fixture.nativeElement as HTMLElement;
    swap.setGroup('fighting_style');
    swap.setFrom('feature:fighter-fighting-style-defense');
    swap.setTo('feature:fighter-fighting-style-dueling');
    fixture.detectChanges();
    expect(swap.pick()).toBeDefined();
    radio(el, 'swap-group', 'Não trocar').click();
    expect(swap.pick()).toBeUndefined();
    expect(swap.started()).toBe(false);
  });

  it('refuses a pick the group does not offer and drops the picks when the group changes', () => {
    const swap = swapOf();
    swap.setGroup('fighting_style');
    swap.setFrom('feature:other');
    expect(swap.fromKey()).toBe('');
    swap.setFrom('feature:fighter-fighting-style-defense');
    swap.setGroup('cantrip');
    expect(swap.groupKey()).toBe('');
    expect(swap.fromKey()).toBe('');
  });

  it('holds one selection for each group and keeps them when another group is opened', () => {
    const swap = bothOf();
    swap.setGroup(FEATURE);
    swap.setFrom(`${FEATURE}:Escudo`);
    swap.setTo(`${FEATURE}:Radiante`);
    swap.setGroup('feature:outra@mesa');
    swap.setFrom('feature:outra@mesa:X');
    swap.setTo('feature:outra@mesa:Y');
    swap.setGroup(FEATURE);
    expect(swap.fromKey()).toBe(`${FEATURE}:Escudo`);
    expect(swap.featurePicks()).toEqual([
      { group: FEATURE, fromKey: `${FEATURE}:Escudo`, toKey: `${FEATURE}:Radiante` },
      {
        group: 'feature:outra@mesa',
        fromKey: 'feature:outra@mesa:X',
        toKey: 'feature:outra@mesa:Y',
      },
    ]);
    expect(swap.pick()).toBeUndefined();
    expect(swap.rows()).toEqual([
      { key: `swap:${FEATURE}`, label: 'Infusões', before: 'Escudo', after: 'Radiante', sub: '' },
      { key: 'swap:feature:outra@mesa', label: 'Outra', before: 'X', after: 'Y', sub: '' },
    ]);
  });

  it('tells a table feature group from a rule group by the feature: key, and sends them apart', () => {
    const swap = bothOf();
    swap.setGroup('fighting_style');
    swap.setFrom('fighting_style:Defesa');
    swap.setTo('fighting_style:Duelismo');
    swap.setGroup(FEATURE);
    swap.setFrom(`${FEATURE}:Escudo`);
    swap.setTo(`${FEATURE}:Bumerangue`);
    expect(swap.pick()).toEqual({
      group: 'fighting_style',
      fromKey: 'fighting_style:Defesa',
      toKey: 'fighting_style:Duelismo',
    });
    expect(swap.featurePicks()).toEqual([
      { group: FEATURE, fromKey: `${FEATURE}:Escudo`, toKey: `${FEATURE}:Bumerangue` },
    ]);
  });

  it('lets the rule swap one group only: starting another rule group drops the first', () => {
    const swap = bothOf();
    swap.setGroup('fighting_style');
    swap.setFrom('fighting_style:Defesa');
    swap.setTo('fighting_style:Duelismo');
    swap.setGroup('cantrip');
    swap.setFrom('cantrip:Luz');
    swap.setTo('cantrip:Chama');
    expect(swap.pick()?.group).toBe('cantrip');
    swap.setGroup('fighting_style');
    expect(swap.fromKey()).toBe('');
  });

  it('sends nothing for a feature group whose swap is half chosen, and "Não trocar" clears all', () => {
    const swap = bothOf();
    swap.setGroup(FEATURE);
    swap.setFrom(`${FEATURE}:Escudo`);
    expect(swap.featurePicks()).toEqual([]);
    swap.setTo(`${FEATURE}:Radiante`);
    expect(swap.featurePicks()).toHaveLength(1);
    swap.clear();
    expect(swap.featurePicks()).toEqual([]);
    expect(swap.started()).toBe(false);
  });

  it('takes over every selection of another draft that the new options still offer', () => {
    const old = bothOf();
    old.setGroup(FEATURE);
    old.setFrom(`${FEATURE}:Escudo`);
    old.setTo(`${FEATURE}:Radiante`);
    old.setGroup('fighting_style');
    old.setFrom('fighting_style:Defesa');
    const fresh = bothOf();
    fresh.adopt(old);
    expect(fresh.featurePicks()).toHaveLength(1);
    expect(fresh.groupKey()).toBe('fighting_style');
    expect(fresh.fromKey()).toBe('fighting_style:Defesa');
  });

  it('shows a table feature group beside the rule groups in the one panel, and marks it once swapped', () => {
    const swap = bothOf();
    const fixture = render(swap);
    const el = fixture.nativeElement as HTMLElement;
    expect(el.querySelectorAll('h2')).toHaveLength(1);
    expect(radio(el, 'swap-group', 'Infusões')).toBeDefined();
    radio(el, 'swap-group', 'Infusões').click();
    fixture.detectChanges();
    radio(el, 'swap-from', 'Escudo').click();
    radio(el, 'swap-to', 'Bumerangue').click();
    fixture.detectChanges();
    expect(el.querySelector('[role="status"]')?.textContent).toContain(
      'Trocar Escudo por Bumerangue',
    );
    radio(el, 'swap-group', 'Estilo de luta').click();
    fixture.detectChanges();
    expect(
      Array.from(el.querySelectorAll('label')).find((l) => l.textContent?.includes('Infusões'))
        ?.textContent,
    ).toContain('(troca escolhida)');
    expect(swap.featurePicks()).toHaveLength(1);
    // The board's chip is plain: a group whose swap is chosen but is not the open one carries no mark of "on" (no check).
    expect(
      Array.from(el.querySelectorAll('label'))
        .find((l) => l.textContent?.includes('Infusões'))
        ?.classList.contains('opt--open'),
    ).toBe(false);
  });

  it('lists the chosen swaps under the group chips, one line each, and says they count when the level is confirmed', () => {
    const swap = bothOf();
    const fixture = render(swap);
    const el = fixture.nativeElement as HTMLElement;
    expect(el.textContent).not.toContain('Trocas escolhidas');
    swap.setGroup(FEATURE);
    swap.setFrom(`${FEATURE}:Escudo`);
    swap.setTo(`${FEATURE}:Radiante`);
    swap.setGroup('feature:outra@mesa');
    swap.setFrom('feature:outra@mesa:X');
    fixture.detectChanges();
    expect(el.querySelectorAll('.chosen__row')).toHaveLength(1);
    swap.setTo('feature:outra@mesa:Y');
    fixture.detectChanges();
    const lines = Array.from(el.querySelectorAll('.chosen__row')).map((l) =>
      l.textContent?.replace(/\s+/g, ' ').trim(),
    );
    expect(lines).toEqual(['Infusões: Escudo → Radiante', 'Outra: X → Y']);
    expect(el.querySelector('.chosen__note')?.textContent).toBe('Vale ao confirmar o nível.');
    expect(el.querySelector('#swap-chosen')?.textContent).toBe('Trocas escolhidas');
  });

  it('checks a group only when it has a swap, marks an open group by border and fill, and tags what leaves and what comes', () => {
    const swap = bothOf();
    const fixture = render(swap);
    const el = fixture.nativeElement as HTMLElement;
    const label = (text: string) =>
      Array.from(el.querySelectorAll('label')).find((l) => l.textContent?.includes(text))!;
    expect(label('Não trocar').classList.contains('opt--done')).toBe(true);
    radio(el, 'swap-group', 'Infusões').click();
    fixture.detectChanges();
    expect(label('Infusões').classList.contains('opt--open')).toBe(true);
    expect(label('Infusões').classList.contains('opt--done')).toBe(false);
    expect(label('Não trocar').classList.contains('opt--done')).toBe(false);
    radio(el, 'swap-from', 'Escudo').click();
    radio(el, 'swap-to', 'Radiante').click();
    fixture.detectChanges();
    expect(label('Infusões').classList.contains('opt--done')).toBe(true);
    const tag = (name: string, text: string) =>
      label(text).closest(`[aria-labelledby="${name}"]`)?.querySelector('.opt__tag')?.textContent;
    expect(tag('swap-from', 'Escudo')).toBe('sai');
    expect(tag('swap-to', 'Radiante')).toBe('entra');
    expect(label('Bumerangue').querySelector('.opt__tag')).toBeNull();
  });

  it('keeps the status region in the page before its text and gives the hint a bold lead', () => {
    const swap = bothOf();
    const fixture = render(swap);
    const el = fixture.nativeElement as HTMLElement;
    const status = el.querySelector('[role="status"]');
    expect(status).not.toBeNull();
    expect(status?.textContent?.trim()).toBe('');
    swap.setGroup(FEATURE);
    fixture.detectChanges();
    expect(el.querySelector('[role="status"]')).toBe(status);
    expect(status?.querySelector('strong')?.textContent).toBe('Falta escolher.');
    expect(status?.textContent).toContain('abrir mão');
    expect(status?.textContent).toContain('pegar no lugar');
  });

  it('does not let the same pick be given up in two groups', () => {
    const shared = named('shared', 'Comum');
    const group = (key: string, namePt: string) =>
      create(LevelUpSwapGroupSchema, {
        group: key,
        namePt,
        picks: [shared, named(`${key}:p`, `P ${namePt}`)],
        pool: [named(`${key}:q`, `Q ${namePt}`)],
      });
    const swap = new LevelUpSwap(
      create(LevelUpOptionsSchema, {
        swapGroups: [group('feature:a@mesa', 'A'), group('feature:b@mesa', 'B')],
      }),
    );
    swap.setGroup('feature:a@mesa');
    swap.setFrom('shared');
    swap.setGroup('feature:b@mesa');
    expect(swap.givenUpElsewhere('shared')).toBe(true);
    swap.setFrom('shared');
    expect(swap.fromKey()).toBe('');
    swap.setFrom('feature:b@mesa:p');
    expect(swap.fromKey()).toBe('feature:b@mesa:p');
    const fixture = render(swap);
    const el = fixture.nativeElement as HTMLElement;
    expect(radio(el, 'swap-from', 'Comum').disabled).toBe(true);
    swap.setGroup('feature:a@mesa');
    fixture.detectChanges();
    expect(radio(el, 'swap-from', 'Comum').disabled).toBe(false);
  });

  it('drops the group name from the front of a pick name, in the lists and in the rows', () => {
    expect(plainName('Estilo de luta', 'Estilo de Luta: Defesa')).toBe('Defesa');
    expect(plainName('Estilo de luta', 'Estilo de Luta: ')).toBe('Estilo de Luta: ');
    expect(plainName('Posturas', 'Postura do Lobo')).toBe('Postura do Lobo');
    const swap = new LevelUpSwap(
      create(LevelUpOptionsSchema, {
        swapGroups: [
          create(LevelUpSwapGroupSchema, {
            group: 'fighting_style',
            namePt: 'Estilo de luta',
            picks: [named('d', 'Estilo de Luta: Defesa')],
            pool: [named('e', 'Estilo de Luta: Duelismo')],
          }),
        ],
      }),
    );
    swap.setGroup('fighting_style');
    swap.setFrom('d');
    swap.setTo('e');
    expect(swap.rows()[0]).toMatchObject({
      label: 'Estilo de luta',
      before: 'Defesa',
      after: 'Duelismo',
    });
    expect(swap.summary()).toBe('Trocar Defesa por Duelismo');
  });

  it('marks the half-done group, shows the hint for it even when another group is open, and clears it', () => {
    const swap = bothOf();
    const fixture = render(swap);
    const el = fixture.nativeElement as HTMLElement;
    const status = el.querySelector('[role="status"]');
    expect(swap.halfGroups()).toEqual([]);
    swap.setGroup(FEATURE);
    swap.setFrom(`${FEATURE}:Escudo`);
    swap.setGroup('fighting_style');
    fixture.detectChanges();
    expect(swap.halfGroups().map((g) => g.group)).toEqual([FEATURE]);
    expect(el.querySelector('[data-half]')?.textContent).toContain('Infusões');
    expect(el.querySelectorAll('[data-half]')).toHaveLength(1);
    expect(el.querySelector('[role="status"]')).toBe(status);
    expect(status?.textContent).toContain('Falta escolher.');
    expect(status?.textContent).toContain('Infusões');
    expect(el.querySelector('section')?.id).toBe('pick-swap');
    swap.setGroup(FEATURE);
    swap.setTo(`${FEATURE}:Radiante`);
    fixture.detectChanges();
    expect(el.querySelector('[data-half]')).toBeNull();
    expect(swap.halfGroups()).toEqual([]);
  });
});
