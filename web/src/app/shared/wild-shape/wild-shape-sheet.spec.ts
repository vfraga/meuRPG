import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { Code, ConnectError } from '@connectrpc/connect';

import { CreaturesClient } from '../../core/creatures/creatures-client';
import { FakeCreaturesClient, flat, raven, summary } from '../../core/creatures/creatures-testing';
import { WildShapeSheet, type WildShapeSheetData } from './wild-shape-sheet';
import { nextTurn } from '../../../test-settle';

describe('WildShapeSheet (E9-11 state 2)', () => {
  async function setup(data: Partial<WildShapeSheetData> = {}, fail: unknown = null) {
    const api = new FakeCreaturesClient();
    api.forms = [
      summary('monster:boar', 'Javali', { name: 'Boar', sizePt: 'Médio', challengeRating: '1/4' }),
      summary('monster:wolf', 'Lobo', { name: 'Wolf', sizePt: 'Médio', challengeRating: '1/4' }),
    ];
    api.blocks.set(
      'monster:boar',
      raven({ armorClass: 11, hitPoints: 11, speedWalkFt: 40, speedFlyFt: 0, actions: [] }),
    );
    api.blocks.set(
      'monster:wolf',
      raven({ armorClass: 13, hitPoints: 11, speedWalkFt: 40, speedFlyFt: 0 }),
    );
    api.failWith = fail;
    const close = vi.fn();
    TestBed.configureTestingModule({
      providers: [
        { provide: CreaturesClient, useValue: api },
        { provide: MatDialogRef, useValue: { close } },
        {
          provide: MAT_DIALOG_DATA,
          useValue: {
            campaignId: 'c',
            characterId: 'ch',
            inCombat: true,
            uses: { left: 2, total: 2 },
            ...data,
          },
        },
      ],
    });
    const fixture = TestBed.createComponent(WildShapeSheet);
    const settle = async () => {
      for (let i = 0; i < 4; i++) {
        await fixture.whenStable();
        await nextTurn();
        fixture.detectChanges();
      }
    };
    fixture.detectChanges();
    await settle();
    return { el: fixture.nativeElement as HTMLElement, api, close, settle };
  }

  it('says the rule, counts the beasts the server lists and asks to choose one', async () => {
    const { el } = await setup();
    expect(flat(el.querySelector('.frame__sub'))).toBe('Feras de ND até 1/2, sem voo.');
    expect(flat(el.querySelector('.cap'))).toBe('2 feras que o seu nível permite');
    expect(flat(el.querySelector('.line'))).toBe('Escolha uma fera.');
    expect(el.querySelector('.go')?.getAttribute('aria-disabled')).toBe('true');
    expect(flat(el.querySelector('.row__title'))).toBe('Javali (Boar)');
  });

  it('the rule line says what the level forbids: flying, swimming, both or neither', async () => {
    for (const [wild, text] of [
      [{ maxCr: '1/2', noFly: true, noSwim: true }, 'Feras de ND até 1/2, sem voo nem natação.'],
      [{ maxCr: '1', noFly: true, noSwim: false }, 'Feras de ND até 1, sem voo.'],
      [{ maxCr: '1/4', noFly: false, noSwim: true }, 'Feras de ND até 1/4, sem natação.'],
      [{ maxCr: '1', noFly: false, noSwim: false }, 'Feras de ND até 1.'],
    ] as const) {
      TestBed.resetTestingModule();
      const api = new FakeCreaturesClient();
      api.wild = wild;
      api.forms = [summary('monster:wolf', 'Lobo', { name: 'Wolf' })];
      api.blocks.set('monster:wolf', raven({ speedWalkFt: 40 }));
      TestBed.configureTestingModule({
        providers: [
          { provide: CreaturesClient, useValue: api },
          { provide: MatDialogRef, useValue: { close: vi.fn() } },
          {
            provide: MAT_DIALOG_DATA,
            useValue: {
              campaignId: 'c',
              characterId: 'ch',
              inCombat: true,
              uses: { left: 2, total: 2 },
            },
          },
        ],
      });
      const fixture = TestBed.createComponent(WildShapeSheet);
      for (let i = 0; i < 4; i++) {
        fixture.detectChanges();
        await fixture.whenStable();
        await nextTurn();
      }
      fixture.detectChanges();
      expect(flat(fixture.nativeElement.querySelector('.frame__sub'))).toBe(text);
    }
  });

  it('shows the numbers of the book and the cost before it is done, then turns the druid', async () => {
    const { el, api, close, settle } = await setup();
    el.querySelectorAll<HTMLInputElement>('input[type=radio]')[1].click();
    await settle();
    expect(flat(el.querySelectorAll('.row__sub')[1])).toContain('CA 13');
    expect(flat(el.querySelector('.line'))).toContain(
      'Gasta a ação e 1 uso de Forma Selvagem (restará 1).',
    );
    expect(flat(el.querySelector('.go'))).toBe('Virar Lobo');
    el.querySelector<HTMLButtonElement>('.go')!.click();
    await settle();
    expect(api.assumed).toEqual(['monster:wolf']);
    expect(close).toHaveBeenCalled();
  });

  it('out of a combat it costs only the use', async () => {
    const { el, settle } = await setup({ inCombat: false });
    el.querySelectorAll<HTMLInputElement>('input[type=radio]')[0].click();
    await settle();
    expect(flat(el.querySelector('.line'))).toContain('Gasta 1 uso de Forma Selvagem (restará 1).');
  });

  it('a refusal stays in the sheet, in words', async () => {
    const { el, api, close, settle } = await setup();
    el.querySelectorAll<HTMLInputElement>('input[type=radio]')[0].click();
    await settle();
    api.failWith = new ConnectError('x', Code.PermissionDenied);
    el.querySelector<HTMLButtonElement>('.go')!.click();
    await settle();
    expect(el.querySelector('[role=alert]')).not.toBeNull();
    expect(close).not.toHaveBeenCalled();
  });
});
