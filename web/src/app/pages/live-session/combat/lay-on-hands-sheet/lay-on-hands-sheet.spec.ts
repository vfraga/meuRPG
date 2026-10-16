import { create } from '@bufbuild/protobuf';
import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  CombatantState,
  EncounterSchema,
  ResourceTargetSchema,
  TargetInReachSchema,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  ResourceBlockedReason,
  ResourceBlockedSchema,
  UseLayOnHandsResponseSchema,
} from '../../../../../gen/meurpg/play/v1/resources_pb';
import { CombatState } from '../../../../core/combat/combat-state';
import { encounter } from '../../../../core/combat/combat-testing';
import { ResourceClient } from '../../../../core/resources/resources-client';
import { pensantusVitals } from '../../../../core/play/live-testing';
import { LayOnHandsSheet, type LayOnHandsSheetData } from './lay-on-hands-sheet';

const flat = (n: Element | null | undefined) => n?.textContent?.replace(/\s+/g, ' ').trim() ?? '';

/** The blocks of a row, each on its own line as the browser draws them. */
const lines = (n: Element | null | undefined) =>
  Array.from(n?.children ?? [], (c) => {
    const copy = c.cloneNode(true) as Element;
    copy.querySelectorAll('mat-icon').forEach((icon) => icon.remove());
    return flat(copy);
  }).join(' ');

const target = (id: string, label: string, over: { feet?: number; far?: boolean } = {}) =>
  create(ResourceTargetSchema, {
    target: create(TargetInReachSchema, {
      combatantId: id,
      label,
      state: CombatantState.HURT,
      distanceFt: over.feet,
      tooFar: over.far ?? false,
    }),
  });

const pool = (used: number) => [
  { key: 'lay_on_hands', total: 25, used, recharge: 'long_rest' as const },
];

describe('LayOnHandsSheet (PM-07c 9 and 9b)', () => {
  const useLayOnHands = vi.fn();
  const close = vi.fn();

  function render(over: { used?: number; targets?: ReturnType<typeof target>[] } = {}) {
    const state = new CombatState();
    state.apply(encounter());
    const data: LayOnHandsSheetData = {
      campaignId: 'camp',
      encounterId: 'enc',
      actorId: 'tavo',
      targets: over.targets ?? [
        target('brisa', 'Brisa', { feet: 5 }),
        target('salvia', 'Sálvia', { feet: 15, far: true }),
        target('tavo', 'Tavo', { feet: 0 }),
      ],
      vitals: signal(pensantusVitals({ resources: pool(over.used ?? 8) })),
      state,
    };
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [
        { provide: ResourceClient, useValue: { useLayOnHands } },
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close, disableClose: false } },
      ],
    });
    const fixture = TestBed.createComponent(LayOnHandsSheet);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const settle = async () => {
      await fixture.whenStable();
      fixture.detectChanges();
    };
    const button = (text: string) =>
      Array.from(el.querySelectorAll('button')).find((b) => flat(b).includes(text))!;
    const radios = () => Array.from(el.querySelectorAll<HTMLInputElement>('input[type=radio]'));
    const radio = (label: string) =>
      radios().find((r) => flat(r.closest('label')).includes(label))!;
    const choose = (label: string) => {
      const r = radio(label);
      r.checked = true;
      r.dispatchEvent(new Event('change'));
      fixture.detectChanges();
    };
    const step = (label: string) => {
      (el.querySelector(`button[aria-label="${label}"]`) as HTMLButtonElement).click();
      fixture.detectChanges();
    };
    return { fixture, el, settle, button, choose, radio, step, state };
  }

  beforeEach(() => {
    useLayOnHands.mockReset();
    close.mockReset();
  });

  it('is titled "Cura pelas Mãos" and says the pool: "Ação · toque · Reserva: 17 de 25 pontos"', () => {
    const { el } = render();
    expect(flat(el.querySelector('h2'))).toBe('Cura pelas Mãos');
    expect(flat(el.querySelector('.frame__sub'))).toBe('Ação · toque · Reserva: 17 de 25 pontos');
  });

  it('lists who the server listed (the paladin itself marked), disables the one beyond the touch with the reason, and picks the first that can be touched', () => {
    const { el, radio } = render();
    const rows = Array.from(el.querySelectorAll('.opt__text'), lines);
    expect(rows[0]).toContain('Brisa Ferido · a 1,5 m');
    expect(rows[1]).toContain('Sálvia');
    expect(rows[1]).toContain('Longe demais: alcance de 1,5 m');
    expect(rows[2]).toContain('Tavo (você)');
    expect(radio('Sálvia').disabled).toBe(true);
    expect(radio('Brisa').checked).toBe(true);
  });

  it('shows the sentence of the heal and moves it with the stepper', () => {
    const { el, step } = render();
    expect(flat(el.querySelector('p[role=status]'))).toBe(
      'Brisa recupera até 5 PV (não passa do máximo). Gasta 5: restam 12 de 25.',
    );
    step('Mais um ponto');
    step('Mais um ponto');
    step('Mais um ponto');
    expect(flat(el.querySelector('p[role=status]'))).toContain('Gasta 8: restam 9 de 25.');
    expect(flat(el.querySelector('footer, .frame__foot'))).toContain('Curar 8 PV em Brisa');
  });

  it('limits the stepper to what the pool has left', () => {
    const { el, step } = render({ used: 23 });
    // 2 points are left: the stepper starts there and cannot go higher.
    expect(flat(el.querySelector('.step output'))).toBe('2');
    step('Mais um ponto');
    expect(flat(el.querySelector('.step output'))).toBe('2');
    expect(flat(el.querySelector('.step__unit'))).toContain('de 1 a 2');
  });

  it('touches with the amount, and says what the server answered (the hit points healed and the pool)', async () => {
    useLayOnHands.mockResolvedValue(
      create(UseLayOnHandsResponseSchema, {
        encounter: create(EncounterSchema, { id: 'enc', revision: 5 }),
        spent: 5,
        poolLeft: 12,
        healed: 4,
      }),
    );
    const { el, button, settle, state } = render();
    button('Curar 5 PV em Brisa').click();
    await settle();
    expect(useLayOnHands).toHaveBeenCalledWith(
      'camp',
      'enc',
      'tavo',
      'brisa',
      { amount: 5 },
      expect.any(String),
    );
    expect(state.encounter()?.revision).toBe(5);
    const text = flat(el);
    expect(text).toContain('Curou');
    expect(text).toContain('Brisa recuperou 4 PV.');
    expect(text).toContain('Gastou 5 pontos da reserva: restam 12 de 25.');
    expect(text).toContain('Ação usada.');
  });

  it('cures a poison or a disease for 5 points, with no stepper, and sends the cure', async () => {
    useLayOnHands.mockResolvedValue(
      create(UseLayOnHandsResponseSchema, { spent: 5, poolLeft: 12 }),
    );
    const { el, button, choose, settle } = render();
    choose('Neutralizar um veneno');
    expect(el.querySelector('.step')).toBeNull();
    expect(flat(el.querySelector('p[role=status]'))).toContain(
      'Gasta 5 da reserva (cada doença ou veneno custa 5, separados).',
    );
    button('Neutralizar o veneno de Brisa').click();
    await settle();
    expect(useLayOnHands).toHaveBeenCalledWith(
      'camp',
      'enc',
      'tavo',
      'brisa',
      { cure: 'poison' },
      expect.any(String),
    );
    expect(flat(el)).toContain('Você neutralizou o veneno de Brisa.');
  });

  it('greys the cures with the reason when the pool has less than 5', () => {
    const { el } = render({ used: 23 });
    const cure = Array.from(el.querySelectorAll('.opt')).find((o) =>
      flat(o).includes('Neutralizar um veneno'),
    )!;
    expect(flat(cure)).toContain('custa 5 pontos (restam 2)');
    expect(cure.querySelector<HTMLInputElement>('input')!.disabled).toBe(true);
  });

  it('answers a touch that did nothing with "Sem efeito" and "Nada acontece.", never a reason or a type', async () => {
    useLayOnHands.mockResolvedValue(
      create(UseLayOnHandsResponseSchema, { spent: 5, poolLeft: 12, nothingHappened: true }),
    );
    const { el, button, settle } = render({
      targets: [target('esq', 'Esqueleto', { feet: 5 }), target('tavo', 'Tavo', { feet: 0 })],
    });
    button('Curar 5 PV em Esqueleto').click();
    await settle();
    const text = flat(el);
    expect(text).toContain('Sem efeito');
    expect(text).toContain(
      'Você tocou o Esqueleto e gastou 5 pontos da reserva (restam 12 de 25). Nada acontece.',
    );
    expect(text).not.toMatch(/morto-vivo|constructo|undead|construct|imune|não age/i);
  });

  it('does not word a refusal by the creature type in the list either: the list says only distance and condition', () => {
    const { el } = render({ targets: [target('esq', 'Esqueleto', { feet: 5 })] });
    expect(flat(el)).not.toMatch(/morto-vivo|constructo|undead|construct/i);
  });

  it('shows the refusal of the rule in words, and keeps the sheet open', async () => {
    useLayOnHands.mockRejectedValue(
      new ConnectError('no', Code.FailedPrecondition, undefined, [
        {
          desc: ResourceBlockedSchema,
          value: create(ResourceBlockedSchema, {
            reason: ResourceBlockedReason.NOT_ENOUGH_POINTS,
            needed: 8,
            available: 3,
          }),
        },
      ]),
    );
    const { el, button, settle } = render();
    button('Curar 5 PV em Brisa').click();
    await settle();
    expect(flat(el.querySelector('[role=alert] p'))).toBe(
      'Não há pontos suficientes: precisa de 8, restam 3.',
    );
    expect(close).not.toHaveBeenCalled();
  });

  it('keeps the key of a touch that failed (a retry never spends twice) and makes a new one when the touch changes', async () => {
    useLayOnHands.mockRejectedValue(new ConnectError('down', Code.Unavailable));
    const { button, settle, step } = render();
    button('Curar 5 PV em Brisa').click();
    await settle();
    button('Curar 5 PV em Brisa').click();
    await settle();
    step('Mais um ponto');
    button('Curar 6 PV em Brisa').click();
    await settle();
    const keys = useLayOnHands.mock.calls.map((c) => c[5] as string);
    expect(keys[0]).toBe(keys[1]);
    expect(keys[2]).not.toBe(keys[0]);
  });

  it('says the pool is empty and asks for nothing when no points are left', () => {
    const { el } = render({ used: 25 });
    expect(flat(el)).toContain('A reserva está vazia.');
    expect(el.querySelector('.step')).toBeNull();
  });

  it('closes with whether a touch was made', async () => {
    useLayOnHands.mockResolvedValue(
      create(UseLayOnHandsResponseSchema, { spent: 5, poolLeft: 12 }),
    );
    const { button, settle } = render();
    button('Cancelar').click();
    expect(close).toHaveBeenLastCalledWith(false);
    button('Curar 5 PV em Brisa').click();
    await settle();
    button('Fechar').click();
    expect(close).toHaveBeenLastCalledWith(true);
  });
});
