import { create } from '@bufbuild/protobuf';
import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  EncounterBlockedReason,
  EncounterBlockedSchema,
  EncounterSchema,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  ConvertSpellSlotResponseSchema,
  CreateSpellSlotResponseSchema,
  ResourceBlockedReason,
  ResourceBlockedSchema,
} from '../../../../../gen/meurpg/play/v1/resources_pb';
import { CombatState } from '../../../../core/combat/combat-state';
import { encounter } from '../../../../core/combat/combat-testing';
import { ResourceClient } from '../../../../core/resources/resources-client';
import { pensantusVitals } from '../../../../core/play/live-testing';
import {
  type Direction,
  FlexibleCastingSheet,
  type FlexibleCastingSheetData,
} from './flexible-casting-sheet';

const flat = (n: Element | null | undefined) =>
  n?.textContent?.replace(/\s+/g, ' ').trim().replace(/ /g, ' ') ?? '';

/** The blocks of a row, each on its own line as the browser draws them. */
const lines = (n: Element | null | undefined) =>
  Array.from(n?.children ?? [], (c) => {
    const copy = c.cloneNode(true) as Element;
    copy.querySelectorAll('mat-icon').forEach((icon) => icon.remove());
    return flat(copy);
  }).join(' ');

/** What a block says, without the name of its icon. */
const said = (n: Element | null | undefined) => {
  const copy = n?.cloneNode(true) as Element | undefined;
  copy?.querySelectorAll('mat-icon').forEach((icon) => icon.remove());
  return flat(copy);
};

describe('FlexibleCastingSheet (PM-07c 10)', () => {
  const createSpellSlot = vi.fn();
  const convertSpellSlot = vi.fn();
  const close = vi.fn();

  /** Nael: Feiticeiro 5, `points` of 5 sorcery points, slots 1º 3 livres de 4, 2º 2 de 3, 3º 2 de 2. */
  function render(over: { points?: number; direction?: Direction } = {}) {
    const state = new CombatState();
    state.apply(encounter());
    const data: FlexibleCastingSheetData = {
      campaignId: 'camp',
      encounterId: 'enc',
      actorId: 'nael',
      direction: over.direction ?? 'create',
      vitals: signal(
        pensantusVitals({
          spellSlots: [
            { level: 1, total: 4, used: 1 },
            { level: 2, total: 3, used: 1 },
            { level: 3, total: 2, used: 0 },
          ],
          resources: [
            {
              key: 'sorcery_points',
              total: 5,
              used: 5 - (over.points ?? 5),
              recharge: 'long_rest',
            },
          ],
        }),
      ),
      state,
    };
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [
        { provide: ResourceClient, useValue: { createSpellSlot, convertSpellSlot } },
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close, disableClose: false } },
      ],
    });
    const fixture = TestBed.createComponent(FlexibleCastingSheet);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const settle = async () => {
      await fixture.whenStable();
      fixture.detectChanges();
    };
    const button = (text: string) =>
      Array.from(el.querySelectorAll('button')).find((b) => flat(b).includes(text))!;
    const radio = (label: string) =>
      Array.from(el.querySelectorAll<HTMLInputElement>('input[type=radio]')).find((r) =>
        flat(r.closest('label')).includes(label),
      )!;
    const choose = (label: string) => {
      const r = radio(label);
      r.checked = true;
      r.dispatchEvent(new Event('change'));
      fixture.detectChanges();
    };
    return { fixture, el, settle, button, radio, choose, state };
  }

  beforeEach(() => {
    createSpellSlot.mockReset();
    convertSpellSlot.mockReset();
    close.mockReset();
  });

  it('opens on the direction of the row, titled "Conjuração Flexível", with the sorcery points in the subtitle', () => {
    const { el, radio } = render();
    expect(flat(el.querySelector('h2'))).toBe('Conjuração Flexível');
    expect(flat(el.querySelector('.frame__sub'))).toBe('Ação bônus · Pontos de Feitiçaria: 5 de 5');
    expect(radio('Pontos → espaço').checked).toBe(true);
  });

  it('lists the five levels with their cost, greys the ones the points cannot pay with the reason written, and picks the lowest', () => {
    const { el } = render();
    const rows = Array.from(el.querySelectorAll('.opt__text'), lines);
    expect(rows).toEqual([
      'Criar um espaço de 1º nível custa 2 pontos',
      'Criar um espaço de 2º nível custa 3 pontos',
      'Criar um espaço de 3º nível custa 5 pontos',
      'Criar um espaço de 4º nível custa 6 pontos (você tem 5)',
      'Criar um espaço de 5º nível custa 7 pontos (você tem 5)',
    ]);
    const radios = Array.from(el.querySelectorAll<HTMLInputElement>('.opt input'));
    expect(radios.map((r) => r.disabled)).toEqual([false, false, false, true, true]);
    expect(radios[0].checked).toBe(true);
  });

  it('says the cost of the chosen level, what is left and that the slot vanishes on a long rest', () => {
    const { el, choose } = render();
    choose('2º nível');
    expect(flat(el.querySelector('p[role=status]'))).toBe(
      'Gasta 3 pontos (restam 2) e cria um espaço de 2º nível. O espaço criado some no descanso longo. Ação bônus.',
    );
    expect(flat(el.querySelector('.frame__foot'))).toContain('Criar o espaço de 2º nível');
  });

  it('creates the slot of the chosen level and says what it cost and what is left', async () => {
    createSpellSlot.mockResolvedValue(
      create(CreateSpellSlotResponseSchema, {
        encounter: create(EncounterSchema, { id: 'enc', revision: 4 }),
        cost: 3,
      }),
    );
    const { el, button, choose, settle, state } = render();
    choose('2º nível');
    button('Criar o espaço de 2º nível').click();
    await settle();
    expect(createSpellSlot).toHaveBeenCalledWith('camp', 'enc', 'nael', 2, expect.any(String));
    expect(state.encounter()?.revision).toBe(4);
    const text = flat(el);
    expect(text).toContain('Espaço criado');
    expect(text).toContain('Gastou 3 pontos: restam 2 de 5. O espaço some no descanso longo.');
    expect(text).toContain('Ação bônus usada.');
  });

  it('lists the free slots of "Espaço → pontos", each giving its level in points, and converts one', async () => {
    convertSpellSlot.mockResolvedValue(create(ConvertSpellSlotResponseSchema, { gain: 2 }));
    const { el, button, choose, settle } = render({ points: 2, direction: 'convert' });
    expect(Array.from(el.querySelectorAll('.opt__text'), lines)).toEqual([
      '1º nível 3 livres de 4 · +1 ponto',
      '2º nível 2 livres de 3 · +2 pontos',
      '3º nível 2 livres de 2 · +3 pontos',
    ]);
    choose('2º nível');
    expect(flat(el.querySelector('p[role=status]'))).toBe(
      'Gasta um espaço de 2º nível e ganha 2 pontos de feitiçaria: ficam em 4 de 5. Ação bônus.',
    );
    button('Converter o espaço de 2º nível').click();
    await settle();
    expect(convertSpellSlot).toHaveBeenCalledWith('camp', 'enc', 'nael', 2, expect.any(String));
    expect(flat(el)).toContain('Ganhou 2 pontos de feitiçaria: ficam em 4 de 5.');
  });

  it('draws the conversion as the irreversible action (danger outline)', () => {
    const { button } = render({ direction: 'convert' });
    expect(button('Converter o espaço').classList.contains('pair__btn--danger')).toBe(true);
    const create = render({ direction: 'create' }).button('Criar o espaço');
    expect(create.classList.contains('pair__btn--danger')).toBe(false);
  });

  it('refuses the conversion with the points at the maximum: the reason written, the button waiting, nothing sent', async () => {
    const { el, button, settle } = render({ points: 5, direction: 'convert' });
    expect(said(el.querySelector('[role=status]'))).toBe(
      'Você já tem o máximo de pontos de feitiçaria. O máximo é o seu nível (5): converter um espaço agora perderia os pontos.',
    );
    const go = button('Converter o espaço de 1º nível');
    expect(go.getAttribute('aria-disabled')).toBe('true');
    go.click();
    await settle();
    expect(convertSpellSlot).not.toHaveBeenCalled();
  });

  it('switches direction and picks the first row of the other list', () => {
    const { el, choose, radio } = render({ points: 2 });
    choose('Espaço → pontos');
    expect(radio('1º nível').checked).toBe(true);
    expect(flat(el.querySelector('.frame__foot'))).toContain('Converter o espaço de 1º nível');
  });

  it('says the bonus action is used when the server refuses with it', async () => {
    createSpellSlot.mockRejectedValue(
      new ConnectError('used', Code.FailedPrecondition, undefined, [
        {
          desc: EncounterBlockedSchema,
          value: create(EncounterBlockedSchema, {
            reason: EncounterBlockedReason.BONUS_ACTION_USED,
          }),
        },
      ]),
    );
    const { el, button, settle } = render();
    button('Criar o espaço de 1º nível').click();
    await settle();
    expect(flat(el.querySelector('[role=alert] p'))).toBe(
      'Sua ação bônus já foi usada neste turno.',
    );
  });

  it('words the refusal of the points the server sent (needed and available)', async () => {
    createSpellSlot.mockRejectedValue(
      new ConnectError('few', Code.FailedPrecondition, undefined, [
        {
          desc: ResourceBlockedSchema,
          value: create(ResourceBlockedSchema, {
            reason: ResourceBlockedReason.NOT_ENOUGH_POINTS,
            needed: 3,
            available: 1,
          }),
        },
      ]),
    );
    const { el, button, settle } = render();
    button('Criar o espaço de 1º nível').click();
    await settle();
    expect(flat(el.querySelector('[role=alert] p'))).toBe(
      'Não há pontos suficientes: precisa de 3, restam 1.',
    );
  });

  it('keeps the key of a request that failed and makes a new one for another level', async () => {
    createSpellSlot.mockRejectedValue(new ConnectError('down', Code.Unavailable));
    const { button, choose, settle } = render();
    button('Criar o espaço de 1º nível').click();
    await settle();
    button('Criar o espaço de 1º nível').click();
    await settle();
    choose('2º nível');
    button('Criar o espaço de 2º nível').click();
    await settle();
    const keys = createSpellSlot.mock.calls.map((c) => c[4] as string);
    expect(keys[0]).toBe(keys[1]);
    expect(keys[2]).not.toBe(keys[0]);
  });
});
