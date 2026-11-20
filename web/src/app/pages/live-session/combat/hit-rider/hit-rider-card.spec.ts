import { ChangeDetectionStrategy, Component } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { create } from '@bufbuild/protobuf';
import { describe, expect, it } from 'vitest';

import {
  CombatantSchema,
  HitRiderChoice,
  HitRiderKind,
  HitRiderOfferSchema,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { HitRiderCard, type RiderPick } from './hit-rider-card';

const goblin = create(CombatantSchema, { id: 'g1', label: 'Goblin' });

@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [HitRiderCard],
  template: `<app-hit-rider-card [offers]="offers" [combatants]="[goblin]" (answer)="picks.push($event)" />`,
})
class Host {
  offers = [
    create(HitRiderOfferSchema, {
      id: 'r1',
      kind: HitRiderKind.OPEN_HAND,
      targetId: 'g1',
      saveDc: 14,
    }),
    create(HitRiderOfferSchema, {
      id: 'r2',
      kind: HitRiderKind.STUNNING_STRIKE,
      targetId: 'g1',
      kiLeft: 0,
      saveDc: 14,
    }),
  ];
  goblin = goblin;
  picks: RiderPick[] = [];
}

describe('HitRiderCard', () => {
  it('offers the three Open Hand choices and answers one', () => {
    const f = TestBed.createComponent(Host);
    f.detectChanges();
    const el = f.nativeElement as HTMLElement;
    const text = el.textContent ?? '';
    expect(text).toContain('Derrubar (teste de Destreza)');
    expect(text).toContain('Empurrar até 4,5 m (teste de Força)');
    expect(text).toContain('Sem reações até o fim do seu próximo turno');
    const prone = [...el.querySelectorAll('button')].find((b) =>
      b.textContent?.includes('Derrubar'),
    );
    prone?.click();
    expect(f.componentInstance.picks).toEqual([{ id: 'r1', choice: HitRiderChoice.PRONE }]);
  });

  it('disables Stunning Strike without ki', () => {
    const f = TestBed.createComponent(Host);
    f.detectChanges();
    const stun = [...(f.nativeElement as HTMLElement).querySelectorAll('button')].find((b) =>
      b.textContent?.includes('Golpe Atordoante (1 ki)'),
    );
    expect(stun?.disabled).toBe(true);
  });

  it('says the save, the DC and what a failure does for each Open Hand choice', () => {
    const f = TestBed.createComponent(Host);
    f.detectChanges();
    const text = ((f.nativeElement as HTMLElement).textContent ?? '').replace(/\s+/g, ' ');
    expect(text).toContain('Teste de Destreza CD 14. Se falhar, fica Derrubado.');
    expect(text).toContain('Teste de Força CD 14. Se falhar, é empurrado até 4,5 m.');
    expect(text).toContain('não pode usar reações até o fim do seu próximo turno');
  });

  it('says what Stunning Strike costs, its save and its effect, and why it is off without ki', () => {
    const f = TestBed.createComponent(Host);
    f.detectChanges();
    const text = ((f.nativeElement as HTMLElement).textContent ?? '').replace(/\s+/g, ' ');
    expect(text).toContain('Sem ki: você não tem pontos de ki para gastar.');
    expect(text).toContain(
      'Teste de Constituição CD 14. Se falhar, fica atordoado até o fim do seu próximo turno.',
    );
  });

  it('with ki left, counts it before and after, and describes the button', () => {
    const f = TestBed.createComponent(Host);
    f.componentInstance.offers = [
      create(HitRiderOfferSchema, {
        id: 'r3',
        kind: HitRiderKind.STUNNING_STRIKE,
        targetId: 'g1',
        kiLeft: 3,
        saveDc: 15,
      }),
    ];
    f.detectChanges();
    const el = f.nativeElement as HTMLElement;
    expect(el.textContent?.replace(/\s+/g, ' ')).toContain(
      'Gasta 1 ponto de ki (restam 3, ficam 2).',
    );
    const stun = [...el.querySelectorAll('button')].find((b) =>
      b.textContent?.includes('Golpe Atordoante'),
    );
    expect(stun?.disabled).toBe(false);
    expect(el.querySelector(`#${stun?.getAttribute('aria-describedby')}`)?.textContent).toContain(
      'CD 15',
    );
  });

  it('leaves the DC out when the server sends none', () => {
    const f = TestBed.createComponent(Host);
    f.componentInstance.offers = [
      create(HitRiderOfferSchema, { id: 'r4', kind: HitRiderKind.OPEN_HAND, targetId: 'g1' }),
    ];
    f.detectChanges();
    const text = (f.nativeElement as HTMLElement).textContent ?? '';
    expect(text).toContain('Teste de Destreza. Se falhar');
    expect(text).not.toContain('CD');
  });
});
