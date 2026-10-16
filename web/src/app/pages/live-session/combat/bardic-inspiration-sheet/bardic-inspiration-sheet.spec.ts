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
  GiveBardicInspirationResponseSchema,
  ResourceBlockedReason,
  ResourceBlockedSchema,
} from '../../../../../gen/meurpg/play/v1/resources_pb';
import { CombatState } from '../../../../core/combat/combat-state';
import { encounter } from '../../../../core/combat/combat-testing';
import { ResourceClient } from '../../../../core/resources/resources-client';
import { pensantusVitals } from '../../../../core/play/live-testing';
import {
  BardicInspirationSheet,
  type BardicInspirationSheetData,
} from './bardic-inspiration-sheet';

const flat = (n: Element | null | undefined) =>
  n?.textContent?.replace(/\s+/g, ' ').trim().replace(/ /g, ' ') ?? '';

const target = (id: string, label: string, feet: number, why = '') =>
  create(ResourceTargetSchema, {
    target: create(TargetInReachSchema, {
      combatantId: id,
      label,
      state: CombatantState.UNHURT,
      distanceFt: feet,
    }),
    disabledReasonPt: why,
  });

/** The blocks of a row, each on its own line as the browser draws them. */
const lines = (n: Element | null | undefined) =>
  Array.from(n?.children ?? [], (c) => {
    const copy = c.cloneNode(true) as Element;
    copy.querySelectorAll('mat-icon').forEach((icon) => icon.remove());
    return flat(copy);
  }).join(' ');

describe('BardicInspirationSheet (PM-07c 12, give)', () => {
  const giveBardicInspiration = vi.fn();
  const close = vi.fn();

  function render(
    targets = [
      target('toren', 'Toren', 15),
      target('brisa', 'Brisa', 25, 'Já tem um dado'),
      target('salvia', 'Sálvia', 30, 'Não ouve você'),
    ],
  ) {
    const state = new CombatState();
    state.apply(encounter());
    const data: BardicInspirationSheetData = {
      campaignId: 'camp',
      encounterId: 'enc',
      actorId: 'orla',
      targets,
      vitals: signal(
        pensantusVitals({
          resources: [{ key: 'bardic_inspiration', total: 3, used: 0, recharge: 'long_rest' }],
        }),
      ),
      state,
    };
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [
        { provide: ResourceClient, useValue: { giveBardicInspiration } },
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close, disableClose: false } },
      ],
    });
    const fixture = TestBed.createComponent(BardicInspirationSheet);
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
    return { fixture, el, settle, button, radio };
  }

  beforeEach(() => {
    giveBardicInspiration.mockReset();
    close.mockReset();
  });

  it('is titled "Inspiração de Bardo" with the reach and the uses in the subtitle', () => {
    const { el } = render();
    expect(flat(el.querySelector('h2'))).toBe('Inspiração de Bardo');
    expect(flat(el.querySelector('.frame__sub'))).toBe(
      'Ação bônus · alcance 18 m · Inspiração de Bardo: 3 de 3',
    );
  });

  it('lists who the server listed, with the reason written for the ones that cannot get a die, and picks the first that can', () => {
    const { el, radio } = render();
    const rows = Array.from(el.querySelectorAll('.opt__text'), lines);
    expect(rows[0]).toBe('Toren Ileso · a 4,5 m');
    expect(rows[1]).toContain('Já tem um dado');
    expect(rows[2]).toContain('Não ouve você');
    expect(radio('Brisa').disabled).toBe(true);
    expect(radio('Sálvia').disabled).toBe(true);
    expect(radio('Toren').checked).toBe(true);
  });

  it('never names a creature type in the list or the words', () => {
    const { el } = render();
    expect(flat(el)).not.toMatch(/morto-vivo|constructo|undead|construct|surdo/i);
  });

  it('says what the gift costs: the bonus action and one use', () => {
    const { el } = render();
    expect(flat(el.querySelector('.tags'))).toContain('Ação bônus');
    expect(flat(el.querySelector('.tags'))).toContain('1 uso: restam 2 de 3');
  });

  it('gives the die to the chosen creature and says who got it', async () => {
    giveBardicInspiration.mockResolvedValue(
      create(GiveBardicInspirationResponseSchema, {
        encounter: create(EncounterSchema, { id: 'enc', revision: 6 }),
      }),
    );
    const { el, button, settle } = render();
    button('Inspirar Toren').click();
    await settle();
    expect(giveBardicInspiration).toHaveBeenCalledWith(
      'camp',
      'enc',
      'orla',
      'toren',
      expect.any(String),
    );
    expect(flat(el)).toContain('Você inspirou Toren.');
    expect(flat(el)).toContain('Ação bônus usada.');
    button('Fechar').click();
    expect(close).toHaveBeenLastCalledWith(true);
  });

  it('says the target cannot get the die, with no type, when the server refuses it', async () => {
    giveBardicInspiration.mockRejectedValue(
      new ConnectError('no', Code.FailedPrecondition, undefined, [
        {
          desc: ResourceBlockedSchema,
          value: create(ResourceBlockedSchema, { reason: ResourceBlockedReason.TARGET_REFUSED }),
        },
      ]),
    );
    const { el, button, settle } = render();
    button('Inspirar Toren').click();
    await settle();
    expect(flat(el.querySelector('[role=alert] p'))).toBe('Essa criatura não pode receber o dado.');
  });

  it('says there are no uses left when the server says so', async () => {
    giveBardicInspiration.mockRejectedValue(
      new ConnectError('no', Code.FailedPrecondition, undefined, [
        {
          desc: ResourceBlockedSchema,
          value: create(ResourceBlockedSchema, { reason: ResourceBlockedReason.NO_USES_LEFT }),
        },
      ]),
    );
    const { el, button, settle } = render();
    button('Inspirar Toren').click();
    await settle();
    expect(flat(el.querySelector('[role=alert] p'))).toContain('Sem usos da Inspiração de Bardo');
  });

  it('sends the same key when a failed gift is tried again', async () => {
    giveBardicInspiration.mockRejectedValue(new ConnectError('down', Code.Unavailable));
    const { button, settle } = render();
    button('Inspirar Toren').click();
    await settle();
    button('Inspirar Toren').click();
    await settle();
    const keys = giveBardicInspiration.mock.calls.map((c) => c[4] as string);
    expect(keys[0]).toBe(keys[1]);
  });

  it('has nobody to pick when nobody can be inspired, and asks nothing', async () => {
    const { el, button, settle } = render([target('brisa', 'Brisa', 25, 'Já tem um dado')]);
    button('Inspirar').click();
    await settle();
    expect(giveBardicInspiration).not.toHaveBeenCalled();
    expect(button('Inspirar').getAttribute('aria-disabled')).toBe('true');
    expect(flat(el)).toContain('Já tem um dado');
  });
});
