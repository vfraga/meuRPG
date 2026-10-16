import { TestBed } from '@angular/core/testing';
import { create } from '@bufbuild/protobuf';

import { PendingDamageStatus } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { TrapDamageSchema } from '../../../../../gen/meurpg/play/v1/traps_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { TrapsClient } from '../../../../core/traps/traps-client';
import type { VitalsVm } from '../../../../core/play/live-session.types';
import { TrapDamages, parseAmount } from './trap-damages';

const damage = (extra = {}) =>
  create(TrapDamageSchema, {
    id: 'd1',
    trapName: 'Fosso escondido',
    characterId: 'ct',
    characterName: 'Toren',
    status: PendingDamageStatus.ROLLED,
    amount: 7,
    damageTypePt: 'concussão',
    roll: { diceCount: 2, diceSides: 6, faces: [3, 4], total: 7 },
    ...extra,
  });
const VITALS = [
  {
    characterId: 'ct',
    name: 'Toren',
    hitPointsCurrent: 40,
    hitPointsMax: 40,
    hitPointsTemporary: 0,
  } as VitalsVm,
];

describe('parseAmount', () => {
  it('takes a whole number from 0 to 1000', () => {
    expect(parseAmount('7')).toBe(7);
    expect(parseAmount(' 0 ')).toBe(0);
    expect(parseAmount('1001')).toBeNull();
    expect(parseAmount('x')).toBeNull();
    expect(parseAmount('')).toBeNull();
  });
});

describe('TrapDamages', () => {
  function setup(d = damage()) {
    const calls: string[] = [];
    const traps = {
      applyDamage: async (_c: string, id: string, amount?: number) => (
        calls.push(`apply ${id} ${amount}`),
        {}
      ),
      discardDamage: async (_c: string, id: string) => (calls.push(`discard ${id}`), {}),
    };
    TestBed.configureTestingModule({
      providers: [
        { provide: TrapsClient, useValue: traps },
        { provide: CombatClient, useValue: {} },
      ],
    });
    const fixture = TestBed.createComponent(TrapDamages);
    fixture.componentRef.setInput('damages', [d]);
    fixture.componentRef.setInput('campaignId', 'c');
    fixture.componentRef.setInput('vitals', VITALS);
    const settled: string[] = [];
    fixture.componentInstance.settled.subscribe((id) => settled.push(id));
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const button = (t: string) =>
      Array.from(el.querySelectorAll('button')).find((b) => b.textContent?.includes(t))!;
    return { fixture, el, calls, settled, button };
  }

  it('says who and how much, with the hit points before and after, and a live region', () => {
    const { el } = setup();
    expect(el.textContent).toContain('Fosso escondido pegou Toren');
    expect(el.textContent).toContain('2d6 (3, 4) = 7 de concussão');
    expect(el.textContent).toContain('Mude o número se houver resistência: o app não calcula.');
    expect(el.querySelector('.td__hp')?.textContent?.replace(/\s+/g, ' ')).toContain('40 de 40');
    expect(el.querySelector('.td__hp')?.textContent).toContain('33');
    expect(el.querySelector('[aria-live=polite]')?.textContent).toContain('esperando você aplicar');
  });

  it('applies the rolled number, or the one the master typed', async () => {
    const { fixture, el, button, calls, settled } = setup();
    expect(button('Aplicar 7 de dano')).toBeTruthy();
    const input = el.querySelector<HTMLInputElement>('.td__input')!;
    input.value = '3';
    input.dispatchEvent(new Event('input'));
    fixture.detectChanges();
    expect(el.querySelector('.td__hp')?.textContent).toContain('37');
    button('Aplicar 3 de dano').click();
    await fixture.whenStable();
    expect(calls).toEqual(['apply d1 3']);
    expect(settled).toEqual(['d1']);
  });

  it("in a combat applies and discards through the combat's own call, and the combat on screen reads the answer", async () => {
    const calls: string[] = [];
    const applied: unknown[] = [];
    const combatApi = {
      applyDamage: async (_c: string, enc: string, id: string, amount?: number) => (
        calls.push(`combat-apply ${enc} ${id} ${amount}`),
        { encounter: { id: 'enc' } }
      ),
      discardDamage: async (_c: string, enc: string, id: string) => (
        calls.push(`combat-discard ${enc} ${id}`),
        { encounter: { id: 'enc' } }
      ),
    };
    TestBed.configureTestingModule({
      providers: [
        { provide: TrapsClient, useValue: {} },
        { provide: CombatClient, useValue: combatApi },
      ],
    });
    const fixture = TestBed.createComponent(TrapDamages);
    fixture.componentRef.setInput('damages', [damage({ encounterId: 'enc' })]);
    fixture.componentRef.setInput('campaignId', 'c');
    fixture.componentRef.setInput('combat', { apply: (e: unknown) => applied.push(e) });
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    Array.from(el.querySelectorAll('button'))
      .find((b) => b.textContent?.includes('Aplicar 7 de dano'))!
      .click();
    await fixture.whenStable();
    expect(calls).toEqual(['combat-apply enc d1 undefined']);
    expect(applied).toHaveLength(1);
  });

  it('asks in place before discarding, with the focus on "Voltar"', async () => {
    const { fixture, el, button, calls } = setup();
    button('Não aplicar').click();
    fixture.detectChanges();
    expect(el.querySelector('[role=alertdialog]')?.textContent).toContain('Descartar o dano de 7?');
    button('Descartar').click();
    await fixture.whenStable();
    expect(calls).toEqual(['discard d1']);
  });
});
