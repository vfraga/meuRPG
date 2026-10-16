import { create } from '@bufbuild/protobuf';
import { ComponentFixture, TestBed } from '@angular/core/testing';

import {
  type MapPoint,
  MapPointKind,
  MapPointSchema,
} from '../../../../gen/meurpg/maps/v1/maps_pb';
import { MapsClient } from '../../../core/maps/maps-client';
import { FakeMapsClient } from '../../../core/maps/maps-testing';
import type { PickRow } from '../../../shared/person-pick/person-pick';
import { TreasurePointPanel } from './treasure-point-panel';
import { nextTurn } from '../../../../test-settle';

const people: PickRow[] = [
  { id: 'c1', name: 'Brisa', sub: 'Clériga 5 · Ana' },
  { id: 'c2', name: 'Toren', sub: 'Guerreiro 5 · Caio' },
];
const base = {
  id: 't1',
  mapId: 'map-1',
  kind: MapPointKind.TREASURE,
  name: 'Baú de moedas',
  description: '250 PO e uma adaga de prata.',
  treasureValuePo: 250,
};
const hidden = create(MapPointSchema, base);
const foundInit = {
  ...base,
  treasureFoundAt: { seconds: 1_790_000_000n },
  treasureFoundBy: [{ characterId: 'c1', characterName: 'Brisa' }],
  revealed: true,
};
const found = create(MapPointSchema, foundInit);
const converted = create(MapPointSchema, { ...foundInit, treasureConverted: true });

describe('TreasurePointPanel', () => {
  let fixture: ComponentFixture<TreasurePointPanel>;
  let el: HTMLElement;
  let api: FakeMapsClient;
  let changed: MapPoint[];

  async function setup(point: MapPoint, session: number | null = null) {
    api = new FakeMapsClient();
    changed = [];
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({ providers: [{ provide: MapsClient, useValue: api }] });
    fixture = TestBed.createComponent(TreasurePointPanel);
    fixture.componentRef.setInput('point', point);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput('people', people);
    fixture.componentRef.setInput('sessionNumber', session);
    fixture.componentInstance.pointChange.subscribe((p) => changed.push(p));
    fixture.detectChanges();
    el = fixture.nativeElement;
    await settle();
  }
  const settle = async () => {
    fixture.detectChanges();
    await nextTurn();
    await fixture.whenStable();
    fixture.detectChanges();
  };
  const text = () => (el.textContent ?? '').replace(/\u00a0/g, ' ').replace(/\s+/g, ' ');
  const button = (t: string) =>
    Array.from(el.querySelectorAll<HTMLElement>('button')).find((b) =>
      b.textContent?.trim().endsWith(t),
    )!;
  const panel = () => fixture.componentInstance;

  it('not found: hidden, with who can mark it and the line about the summary outside a session', async () => {
    await setup(hidden);
    expect(text()).toContain('Tesouro · escondido');
    expect(text()).toContain('Não encontrado');
    expect(text()).toContain('Os jogadores não o veem, nem na lista.');
    expect(text()).toContain('Marcado fora de uma sessão, o tesouro não entra em resumo nenhum.');
    expect(button('Marcar como encontrado')).toBeTruthy();
  });

  it('during a session the line says it enters that summary', async () => {
    await setup(hidden, 6);
    expect(text()).toContain('Marcado durante a Sessão 6, o tesouro entra no resumo dela.');
  });

  it('marks it found by who is chosen: nobody is checked at first, and the call waits for at least one', async () => {
    await setup(hidden);
    button('Marcar como encontrado').click();
    await settle();
    expect(text()).toContain('Quem encontrou Baú de moedas?');
    expect(el.querySelectorAll('input[type=checkbox]:checked')).toHaveLength(0);
    expect(document.activeElement).toBe(el.querySelector('input[type=checkbox]'));
    const go = Array.from(el.querySelectorAll('button'))
      .filter((b) => b.textContent?.trim().endsWith('Marcar como encontrado'))
      .at(-1)!;
    expect(go.getAttribute('aria-disabled')).toBe('true');
    go.click();
    await settle();
    expect(api.calls).toEqual([]);
    const brisa = el.querySelectorAll<HTMLInputElement>('input[type=checkbox]')[0];
    brisa.click();
    await settle();
    expect(text()).toContain('No resumo da sessão: Brisa · 250 PO');
    Array.from(el.querySelectorAll('button'))
      .filter((b) => b.textContent?.trim().endsWith('Marcar como encontrado'))
      .at(-1)!
      .click();
    await settle();
    expect(api.calls).toEqual(['markTreasureFound map-1 t1 c1']);
    expect(changed).toHaveLength(1);
  });

  it('found: by whom and when, and "Desmarcar" asks in place with the focus on the question', async () => {
    await setup(found);
    expect(text()).toContain('Encontrado por Brisa');
    expect(text()).toContain('Todos veem o tesouro, o que há dentro e o valor.');
    button('Desmarcar').click();
    await settle();
    expect(el.querySelector('app-map-ask')?.textContent).toContain('Desmarcar Baú de moedas?');
    expect(document.activeElement?.textContent?.trim()).toBe('Desmarcar Baú de moedas?');
    expect(button('Voltar')).toBeTruthy();
    expect(api.calls).toEqual([]);
    Array.from(el.querySelectorAll('button'))
      .filter((b) => b.textContent?.trim() === 'Desmarcar')
      .at(-1)!
      .click();
    await settle();
    expect(api.calls).toEqual(['unmarkTreasureFound map-1 t1']);
  });

  it('found: "Apagar ponto" cannot act until it is unmarked, and says why', async () => {
    await setup(found);
    const del = button('Apagar ponto');
    expect(del.classList).toContain('mr-button--off');
    expect(text()).toContain('Este tesouro foi encontrado. Desmarque antes de apagar.');
  });

  it('converted into XP: locked fields, "Desmarcar" and "Apagar ponto" cannot act, with the reason', async () => {
    await setup(converted);
    expect(text()).toContain('Tesouro · convertido');
    expect(text()).toContain('Convertido em XP');
    expect(text()).toContain('Para desmarcar, desfaça esse XP na página da campanha.');
    expect(text()).toContain('Convertido em XP: este tesouro não pode ser apagado.');
    const textarea = el.querySelector('textarea')!;
    expect(textarea.readOnly).toBe(true);
    const value = Array.from(el.querySelectorAll('input')).find((i) =>
      i.closest('mat-form-field')?.textContent?.includes('Valor em ouro'),
    )!;
    expect(value.readOnly).toBe(true);
    expect(button('Desmarcar').classList).toContain('mr-button--off');
    expect(button('Apagar ponto').classList).toContain('mr-button--off');
  });

  it('saves the value as a number, and refuses what is not 0 to 1.000.000', async () => {
    await setup(hidden);
    const value = Array.from(el.querySelectorAll('input')).find((i) =>
      i.closest('mat-form-field')?.textContent?.includes('Valor em ouro'),
    )!;
    value.value = '300';
    value.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    expect(panel().changes()).toEqual({ treasureValuePo: 300 });
    value.value = '2000000';
    value.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    expect(panel().changes()).toBeNull();
    await settle();
    expect(text()).toContain('Use um número inteiro de 0 a 1.000.000.');
  });
});
