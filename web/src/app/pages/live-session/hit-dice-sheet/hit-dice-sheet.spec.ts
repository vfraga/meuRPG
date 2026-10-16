import { create } from '@bufbuild/protobuf';
import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { Code, ConnectError } from '@connectrpc/connect';

import { DiceMode, DicePreference } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  EncounterBlockedReason,
  EncounterBlockedSchema,
} from '../../../../gen/meurpg/play/v1/combat_pb';
import { CharacterVitalsSchema } from '../../../../gen/meurpg/play/v1/play_pb';
import {
  ResourceBlockedReason,
  ResourceBlockedSchema,
  SpendHitDiceResponseSchema,
  UseArcaneRecoveryResponseSchema,
} from '../../../../gen/meurpg/play/v1/resources_pb';
import { ResourceClient } from '../../../core/resources/resources-client';
import type { VitalsVm } from '../../../core/play/live-session.types';
import { pensantusVitals } from '../../../core/play/live-testing';
import { HitDiceSheet, type HitDiceSheetData } from './hit-dice-sheet';

const flat = (n: Element | null | undefined) => n?.textContent?.replace(/\s+/g, ' ').trim();
/** The blocks of a card, each on its own line as the browser draws them. */
const lines = (n: Element | null | undefined) =>
  Array.from(n?.children ?? [], (c) => flat(c)).join(' ');

/** A multiclass fighter and wizard: 5d10 (2 spent) and 1d6 (none spent). */
const fighterWizard = (over: Partial<VitalsVm> = {}) =>
  pensantusVitals({
    name: 'Mirta',
    hitPointsCurrent: 17,
    hitPointsMax: 50,
    hitDice: '5d10 e 1d6',
    hitDiceSizes: [
      { faces: 10, total: 5, used: 2 },
      { faces: 6, total: 1, used: 0 },
    ],
    hitDiceTotal: 6,
    hitDiceUsed: 2,
    revision: 1,
    ...over,
  });

function answer(over: {
  used10?: number;
  face?: number;
  mod?: number;
  healed?: number;
  revision?: number;
}) {
  return create(SpendHitDiceResponseSchema, {
    vitals: create(CharacterVitalsSchema, {
      characterId: 'pensantus',
      name: 'Mirta',
      hitPointsCurrent: 26,
      hitPointsMax: 50,
      hitDice: [
        { faces: 10, count: 5 },
        { faces: 6, count: 1 },
      ],
      hitDiceUsedByDie: { 10: over.used10 ?? 3 },
      revision: over.revision ?? 2,
    }),
    face: over.face ?? 7,
    constitutionModifier: over.mod ?? 2,
    healed: over.healed ?? 9,
  });
}

function blocked(reason: ResourceBlockedReason): ConnectError {
  return new ConnectError('blocked', Code.FailedPrecondition, undefined, [
    { desc: ResourceBlockedSchema, value: create(ResourceBlockedSchema, { reason }) },
  ]);
}

describe('HitDiceSheet (decisions batch 2 B 6)', () => {
  const spendHitDice = vi.fn();
  const useArcaneRecovery = vi.fn();
  const close = vi.fn();
  const apply = vi.fn();
  const live = signal<VitalsVm>(fighterWizard());

  beforeEach(() => {
    spendHitDice.mockReset();
    useArcaneRecovery.mockReset();
    close.mockReset();
    apply.mockReset();
    live.set(fighterWizard());
  });

  function render(
    over: { diceMode?: DiceMode; preference?: DicePreference } = {},
    vitals: Partial<VitalsVm> | null = null,
  ) {
    if (vitals) {
      live.set(fighterWizard(vitals));
    }
    const data: HitDiceSheetData = {
      campaignId: 'camp',
      vitals: live,
      diceMode: over.diceMode ?? DiceMode.APP,
      preference: over.preference ?? DicePreference.APP,
      apply,
    };
    TestBed.configureTestingModule({
      providers: [
        { provide: ResourceClient, useValue: { spendHitDice, useArcaneRecovery } },
        { provide: MAT_DIALOG_DATA, useValue: data },
        { provide: MatDialogRef, useValue: { close, disableClose: false } },
      ],
    });
    const fixture = TestBed.createComponent(HitDiceSheet);
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    const settle = async () => {
      await fixture.whenStable();
      fixture.detectChanges();
      await fixture.whenStable();
    };
    const button = (name: string) =>
      Array.from(el.querySelectorAll('button')).find((b) => flat(b)?.endsWith(name))!;
    const radios = () => Array.from(el.querySelectorAll<HTMLInputElement>('input[type=radio]'));
    return { fixture, el, settle, button, radios };
  }

  it('is titled "Gastar dados de vida", names the character and shows what is left of each size', () => {
    const { el, radios } = render();
    expect(flat(el.querySelector('h2'))).toBe('Gastar dados de vida');
    expect(flat(el.querySelector('.frame__sub'))).toBe('Mirta · 17 de 50 PV');
    expect(Array.from(el.querySelectorAll('.opt__text'), lines)).toEqual([
      'd10 Restam 3 de 5d10',
      'd6 Resta 1 de 1d6',
    ]);
    // The largest size with dice left is chosen at first.
    expect(radios().map((r) => r.checked)).toEqual([true, false]);
  });

  it('skips a size with none left, which stays in the list as "Nenhum restante"', () => {
    const { el, radios } = render(
      {},
      {
        hitDiceSizes: [
          { faces: 10, total: 2, used: 2 },
          { faces: 6, total: 1, used: 0 },
        ],
      },
    );
    expect(radios().map((r) => [r.checked, r.disabled])).toEqual([
      [false, true],
      [true, false],
    ]);
    expect(lines(el.querySelectorAll('.opt__text')[0])).toBe('d10 Nenhum restante');
  });

  it('rolls the die in the app, says what it did and is ready for the next one', async () => {
    spendHitDice.mockResolvedValue(answer({}));
    const { el, settle, button } = render();
    button('Rolar d10 no app').click();
    await settle();

    expect(spendHitDice).toHaveBeenCalledTimes(1);
    const [campaignId, characterId, faces, roll, key] = spendHitDice.mock.calls[0];
    expect([campaignId, characterId, faces, roll]).toEqual([
      'camp',
      'pensantus',
      10,
      { inApp: true },
    ]);
    expect(key).toMatch(/^[0-9a-f-]{36}$/);
    expect(flat(el.querySelector('.rolls__line'))).toContain('Rolou 7 + 2 = 9 · recuperou 9 PV');
    expect(flat(el.querySelector('[role="status"]'))).toBe('Rolou 7 + 2 = 9 · recuperou 9 PV');
    // The answer's vitals go to the page, and the sheet shows what is left now.
    expect(apply).toHaveBeenCalledTimes(1);
    expect(apply.mock.calls[0][0].hitDiceSizes[0]).toEqual({ faces: 10, total: 5, used: 3 });
    expect(lines(el.querySelectorAll('.opt__text')[0])).toBe('d10 Restam 2 de 5d10');
    expect(flat(el.querySelector('.frame__sub'))).toBe('Mirta · 26 de 50 PV');
    expect(button('Rolar d10 no app')).toBeTruthy();
  });

  it('spends the size that was picked', async () => {
    spendHitDice.mockResolvedValue(answer({}));
    const { settle, button, radios, fixture } = render();
    radios()[1].click();
    fixture.detectChanges();
    expect(button('Rolar d6 no app')).toBeTruthy();
    button('Rolar d6 no app').click();
    await settle();
    expect(spendHitDice.mock.calls[0][2]).toBe(6);
  });

  it('makes a new key for each die, also of the same size, and keeps the key for a retry', async () => {
    spendHitDice
      .mockRejectedValueOnce(new ConnectError('lost', Code.Unavailable))
      .mockResolvedValue(answer({}));
    const { settle, button } = render();
    button('Rolar d10 no app').click();
    await settle();
    button('Rolar d10 no app').click();
    await settle();
    expect(spendHitDice.mock.calls[1][4]).toBe(spendHitDice.mock.calls[0][4]);
    // The retry worked: the next die of the same size is another request.
    button('Rolar d10 no app').click();
    await settle();
    expect(spendHitDice).toHaveBeenCalledTimes(3);
    expect(spendHitDice.mock.calls[2][4]).not.toBe(spendHitDice.mock.calls[1][4]);
  });

  it('never spends a die twice on a double tap', async () => {
    let release!: () => void;
    spendHitDice.mockReturnValue(new Promise((resolve) => (release = () => resolve(answer({})))));
    const { settle, button } = render();
    const go = button('Rolar d10 no app');
    go.click();
    go.click();
    release();
    await settle();
    expect(spendHitDice).toHaveBeenCalledTimes(1);
  });

  describe('real dice (RN-18)', () => {
    it('asks for the face of the real die and spends it as typed', async () => {
      spendHitDice.mockResolvedValue(answer({ face: 5, mod: 0, healed: 5 }));
      const { el, settle, button } = render({ diceMode: DiceMode.PHYSICAL });
      expect(
        Array.from(el.querySelectorAll('button')).some((b) => flat(b)?.includes('no app')),
      ).toBe(false);
      const field = el.querySelector<HTMLInputElement>('input[type=text]')!;
      expect(flat(el.querySelector('.type__hint'))).toBe(
        'Role o seu d10 e digite o número que saiu (1 a 10).',
      );
      field.value = '5';
      field.dispatchEvent(new Event('input'));
      await settle();
      button('Confirmar 5').click();
      await settle();
      expect(spendHitDice.mock.calls[0].slice(2, 4)).toEqual([10, { face: 5 }]);
      expect(flat(el.querySelector('.rolls__line'))).toContain('Rolou 5 · recuperou 5 PV');
    });

    it("asks for the face of a bard's Song of Rest die after the hit die, then spends both", async () => {
      spendHitDice.mockRejectedValueOnce(
        new ConnectError('song', Code.FailedPrecondition, undefined, [
          {
            desc: ResourceBlockedSchema,
            value: create(ResourceBlockedSchema, {
              reason: ResourceBlockedReason.SONG_OF_REST_FACE_NEEDED,
              needed: 6,
            }),
          },
        ]),
      );
      const { el, settle, button } = render({ diceMode: DiceMode.PHYSICAL });
      const type = async (value: string) => {
        const field = el.querySelector<HTMLInputElement>('input[type=text]')!;
        field.value = value;
        field.dispatchEvent(new Event('input'));
        await settle();
      };
      await type('5');
      button('Confirmar 5').click();
      await settle();
      // No error box: the sheet asks for the song's die in the same field, keeping the 5.
      expect(el.querySelector('[role=alert]')).toBeNull();
      expect(flat(el.querySelector('.js-song-ask'))).toContain('recupera também um d6');
      expect(flat(el.querySelector('.js-song-ask'))).toContain('O seu d10 deu 5');
      expect(flat(el.querySelector('.type__hint'))).toBe(
        'Role o seu d6 e digite o número que saiu (1 a 6).',
      );
      spendHitDice.mockResolvedValueOnce(
        create(SpendHitDiceResponseSchema, {
          ...answer({ face: 5, mod: 0, healed: 9 }),
          songOfRestSides: 6,
          songOfRestFace: 4,
        }),
      );
      await type('4');
      button('Confirmar 4').click();
      await settle();
      expect(spendHitDice.mock.calls[1].slice(2, 4)).toEqual([10, { face: 5, songFace: 4 }]);
      expect(flat(el.querySelector('.rolls__line'))).toContain(
        'Rolou 5 + 4 (Canção de Descanso, d6) = 9 · recuperou 9 PV',
      );
      expect(el.querySelector('.js-song-ask')).toBeNull();
    });

    it('lets the player go back from the song question', async () => {
      spendHitDice.mockRejectedValueOnce(
        new ConnectError('song', Code.FailedPrecondition, undefined, [
          {
            desc: ResourceBlockedSchema,
            value: create(ResourceBlockedSchema, {
              reason: ResourceBlockedReason.SONG_OF_REST_FACE_NEEDED,
              needed: 8,
            }),
          },
        ]),
      );
      const { el, settle, button } = render({ diceMode: DiceMode.PHYSICAL });
      const field = el.querySelector<HTMLInputElement>('input[type=text]')!;
      field.value = '3';
      field.dispatchEvent(new Event('input'));
      await settle();
      button('Confirmar 3').click();
      await settle();
      el.querySelector<HTMLButtonElement>('.js-song-cancel')!.click();
      await settle();
      expect(el.querySelector('.js-song-ask')).toBeNull();
      expect(flat(el.querySelector('.type__hint'))).toBe(
        'Role o seu d10 e digite o número que saiu (1 a 10).',
      );
    });

    it('refuses a face outside the die before asking the server', async () => {
      const { el, settle } = render({ diceMode: DiceMode.PHYSICAL });
      const field = el.querySelector<HTMLInputElement>('input[type=text]')!;
      field.value = '11';
      field.dispatchEvent(new Event('input'));
      await settle();
      expect(flat(el.querySelector('.type__err'))).toContain('Digite um número de 1 a 10');
      expect(spendHitDice).not.toHaveBeenCalled();
    });

    it('offers both ways when each player chooses, the preferred one first', () => {
      const { button } = render({
        diceMode: DiceMode.PLAYERS_CHOOSE,
        preference: DicePreference.PHYSICAL,
      });
      expect(button('Rolar d10 no app')).toBeTruthy();
      expect(button('Digitar o resultado')).toBeTruthy();
    });

    it('has no way to type when everyone rolls in the app', () => {
      const { el } = render({ diceMode: DiceMode.APP });
      expect(el.textContent).not.toContain('Digitar o resultado');
    });
  });

  describe('what the server refuses', () => {
    it('says there is no die of that size', async () => {
      spendHitDice.mockRejectedValue(blocked(ResourceBlockedReason.NO_HIT_DICE_LEFT));
      const { el, settle, button } = render();
      button('Rolar d10 no app').click();
      await settle();
      expect(flat(el.querySelector('[role="alert"] p'))).toBe('Não há dado de vida desse tipo.');
      expect(apply).not.toHaveBeenCalled();
    });

    it('says a combat is open', async () => {
      spendHitDice.mockRejectedValue(blocked(ResourceBlockedReason.COMBAT_OPEN));
      const { el, settle, button } = render();
      button('Rolar d10 no app').click();
      await settle();
      expect(flat(el.querySelector('[role="alert"] p'))).toBe('Há um combate em andamento.');
    });

    it('words the wrong way of rolling as the combat does', async () => {
      spendHitDice.mockRejectedValue(
        new ConnectError('wrong', Code.FailedPrecondition, undefined, [
          {
            desc: EncounterBlockedSchema,
            value: create(EncounterBlockedSchema, {
              reason: EncounterBlockedReason.WRONG_DICE_MODE,
            }),
          },
        ]),
      );
      const { el, settle, button } = render();
      button('Rolar d10 no app').click();
      await settle();
      expect(flat(el.querySelector('[role="alert"] p'))).toContain('forma de rolar os dados');
    });
  });

  it('follows the stream: the numbers the page gets while the sheet is open show in it', async () => {
    const { el, fixture } = render();
    live.set(fighterWizard({ hitPointsCurrent: 30, revision: 5 }));
    fixture.detectChanges();
    expect(flat(el.querySelector('.frame__sub'))).toBe('Mirta · 30 de 50 PV');
  });

  it('says there is nothing to spend, and offers no roll, when every die is used', () => {
    const { el, button } = render(
      {},
      {
        hitDiceSizes: [
          { faces: 10, total: 2, used: 2 },
          { faces: 6, total: 1, used: 1 },
        ],
      },
    );
    expect(flat(el.querySelector('.mr-notice'))).toContain('Não há dados de vida para gastar.');
    expect(el.querySelector('app-roll-picker')).toBeNull();
    expect(button('Fechar')).toBeTruthy();
  });

  it('closes with "true" when a die was spent and with "false" when none was', async () => {
    spendHitDice.mockResolvedValue(answer({}));
    const first = render();
    first.button('Fechar').click();
    expect(close).toHaveBeenLastCalledWith(false);

    first.button('Rolar d10 no app').click();
    await first.settle();
    first.button('Fechar').click();
    expect(close).toHaveBeenLastCalledWith(true);
  });

  it('does not close under a die that is being spent', async () => {
    let release!: () => void;
    spendHitDice.mockReturnValue(new Promise((resolve) => (release = () => resolve(answer({})))));
    const { settle, button } = render();
    button('Rolar d10 no app').click();
    button('Fechar').click();
    expect(close).not.toHaveBeenCalled();
    release();
    await settle();
  });
  describe('Recuperação Arcana (SRD 5.1, Wizard)', () => {
    /** A level 3 wizard: allowance 2, four 1st-level and two 2nd-level slots spent, one 6th-level slot spent. */
    const wizard = (over: Partial<VitalsVm> = {}): Partial<VitalsVm> => ({
      arcaneRecoveryAllowance: 2,
      resources: [
        {
          key: 'arcane_recovery',
          namePt: 'Recuperação Arcana',
          total: 1,
          used: 0,
          recharge: 'long_rest',
        },
      ],
      spellSlots: [
        { level: 1, total: 4, used: 3 },
        { level: 2, total: 2, used: 1 },
        { level: 3, total: 2, used: 0 },
        { level: 6, total: 1, used: 1 },
      ],
      ...over,
    });

    function recovered(used: [number, number, number]) {
      return create(UseArcaneRecoveryResponseSchema, {
        recoveredLevels: 2,
        vitals: create(CharacterVitalsSchema, {
          characterId: 'pensantus',
          name: 'Mirta',
          arcaneRecoveryAllowance: 2,
          resources: [{ key: 'arcane_recovery', total: 1, used: 1 }],
          spellSlots: [
            { level: 1, total: 4, used: used[0] },
            { level: 2, total: 2, used: used[1] },
            { level: 6, total: 1, used: used[2] },
          ],
          revision: 5,
        }),
      });
    }

    it('shows the allowance and a picker only for the expended slots of the 1st to the 5th level', () => {
      const { el } = render({}, wizard());
      expect(flat(el.querySelector('.arcane__title'))).toBe('Recuperação Arcana');
      expect(flat(el.querySelector('.arcane .note'))).toContain(
        'Recupere espaços de magia que somem até 2 níveis; nenhum de 6º nível ou mais.',
      );
      expect(Array.from(el.querySelectorAll('.step__text'), lines)).toEqual([
        '1º nível 3 gastos',
        '2º nível 1 gasto',
      ]);
    });

    it('stops at the allowance and at what is expended', () => {
      const { el, fixture } = render({}, wizard());
      const more = () => Array.from(el.querySelectorAll<HTMLButtonElement>('.js-arcane-more'));
      more()[0].click(); // a 1st-level slot
      fixture.detectChanges();
      expect(flat(el.querySelector('.arcane__sum'))).toBe('Total: 1 de 2 níveis');
      more()[1].click(); // a 2nd-level slot would make 3
      fixture.detectChanges();
      expect(more().map((b) => b.disabled)).toEqual([false, true]);
      more()[0].click();
      fixture.detectChanges();
      expect(flat(el.querySelector('.arcane__sum'))).toBe('Total: 2 de 2 níveis');
      expect(more().map((b) => b.disabled)).toEqual([true, true]);
    });

    it('recovers the picked slots with one key and then says it was used today', async () => {
      useArcaneRecovery.mockResolvedValue(recovered([1, 1, 1]));
      const { el, fixture, settle, button } = render({}, wizard());
      const more = () => Array.from(el.querySelectorAll<HTMLButtonElement>('.js-arcane-more'));
      more()[0].click();
      more()[0].click();
      fixture.detectChanges();
      button('Recuperar').click();
      await settle();

      expect(useArcaneRecovery).toHaveBeenCalledTimes(1);
      const [campaignId, characterId, slots, key] = useArcaneRecovery.mock.calls[0];
      expect([campaignId, characterId, slots]).toEqual([
        'camp',
        'pensantus',
        [{ level: 1, count: 2 }],
      ]);
      expect(key).toMatch(/^[0-9a-f-]{36}$/);
      expect(apply.mock.calls[0][0].spellSlots[0]).toEqual({
        level: 1,
        total: 4,
        used: 1,
        created: 0,
      });
      expect(flat(el.querySelector('.arcane'))).toContain(
        'Recuperação Arcana: usada; volta no descanso longo.',
      );
      expect(el.querySelector('.js-arcane-recover')).toBeNull();
    });

    it('does not call the server with nothing picked', async () => {
      const { settle, button } = render({}, wizard());
      button('Recuperar').click();
      await settle();
      expect(useArcaneRecovery).not.toHaveBeenCalled();
    });

    it('says why the server refused', async () => {
      useArcaneRecovery.mockRejectedValue(blocked(ResourceBlockedReason.NO_SHORT_REST));
      const { el, fixture, settle, button } = render({}, wizard());
      el.querySelector<HTMLButtonElement>('.js-arcane-more')!.click();
      fixture.detectChanges();
      button('Recuperar').click();
      await settle();
      expect(flat(el.querySelector('[role="alert"] p'))).toBe(
        'Recuperação Arcana só vale depois de um descanso curto.',
      );
    });

    it('says the use is spent when it is, and offers nothing', () => {
      const spent = wizard({
        resources: [{ key: 'arcane_recovery', total: 1, used: 1, recharge: 'long_rest' }],
      });
      const { el } = render({}, spent);
      expect(flat(el.querySelector('.arcane'))).toContain(
        'Recuperação Arcana: usada; volta no descanso longo.',
      );
      expect(el.querySelector('.step')).toBeNull();
    });

    it('is not there for a character without the feature', () => {
      const { el } = render();
      expect(el.querySelector('.arcane')).toBeNull();
    });
  });
});
