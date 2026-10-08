import { ComponentFixture, TestBed } from '@angular/core/testing';
import { FormControl, FormGroup } from '@angular/forms';
import { provideRouter } from '@angular/router';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  AbilityScoresRefusalReason,
  AbilityScoresRefusalSchema,
} from '../../../../gen/meurpg/characters/v1/characters_pb';
import { AbilityKey } from '../../../core/characters/characters.types';
import type { AbilityFormGroup } from '../ability-fields/ability-fields';
import {
  type AbilityMethodKey,
  type AbilityRollsVm,
  type AbilityTableVm,
  CharacterEditorSource,
} from '../character-editor.types';
import { TableAbilityScores } from './table-ability-scores';

const STORED: AbilityRollsVm = {
  sets: [
    { dice: [6, 5, 5, 2], total: 16 },
    { dice: [5, 5, 4, 1], total: 14 },
    { dice: [5, 4, 4, 3], total: 13 },
    { dice: [4, 4, 4, 2], total: 12 },
    { dice: [4, 3, 3, 2], total: 10 },
    { dice: [3, 3, 2, 1], total: 8 },
  ],
  typed: false,
  rolledAt: new Date('2026-10-05T20:14:00'),
};

function table(over: Partial<AbilityTableVm> = {}): AbilityTableVm {
  return {
    standardArray: true,
    pointBuy: true,
    rolled4d6: true,
    typed: true,
    standardValues: [15, 14, 13, 12, 10, 8],
    pointBuyCosts: [0, 1, 2, 3, 4, 5, 7, 9],
    pointBuyMinScore: 8,
    pointBuyBudget: 27,
    typedMin: 3,
    typedMax: 18,
    hitPoints: 'player_chooses',
    physicalDice: false,
    diceForced: false,
    rolls: null,
    ...over,
  };
}

function group(): AbilityFormGroup {
  const c = () => new FormControl(10, { nonNullable: true });
  return new FormGroup({ str: c(), dex: c(), con: c(), int: c(), wis: c(), cha: c() });
}

describe('TableAbilityScores', () => {
  let fixture: ComponentFixture<TableAbilityScores>;
  let el: HTMLElement;
  let g: AbilityFormGroup;
  let roll: ReturnType<typeof vi.fn>;
  let method: AbilityMethodKey;
  let incomplete: boolean;
  let problem: string;

  async function setup(t: AbilityTableVm = table()) {
    g = group();
    roll = vi.fn().mockResolvedValue(STORED);
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        { provide: CharacterEditorSource, useValue: { rollAbilityScores: roll } },
      ],
    });
    fixture = TestBed.createComponent(TableAbilityScores);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput('group', g);
    fixture.componentRef.setInput('table', t);
    fixture.componentInstance.method.subscribe((m) => (method = m));
    fixture.componentInstance.incomplete.subscribe((v) => (incomplete = v));
    fixture.componentInstance.problem.subscribe((v) => (problem = v));
    method = 'typed';
    incomplete = false;
    problem = '';
    el = fixture.nativeElement;
    await settle();
  }

  async function settle() {
    for (let i = 0; i < 3; i++) {
      fixture.detectChanges();
      await new Promise((r) => setTimeout(r));
      await fixture.whenStable();
    }
    fixture.detectChanges();
  }

  const text = () => (el.textContent ?? '').replace(/\u00a0/g, ' ').replace(/\s+/g, ' ');
  const tab = (label: string) =>
    Array.from(el.querySelectorAll<HTMLInputElement>('input[name="ability-method"]')).find(
      (i) => i.closest('label')?.textContent?.replace('check', '').trim() === label,
    )!;
  const button = (label: string) =>
    Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
      b.textContent?.replace(/\s+/g, ' ').trim().includes(label),
    )!;
  const scores = (): number[] =>
    (['str', 'dex', 'con', 'int', 'wis', 'cha'] as AbilityKey[]).map((k) => g.controls[k].value);

  it('offers only the ways the master allows, in order, with the first one chosen', async () => {
    await setup(table({ standardArray: false, typed: false }));
    const tabs = Array.from(el.querySelectorAll('input[name="ability-method"]')).map((i) =>
      i.closest('label')?.textContent?.replace('check', '').trim(),
    );
    expect(tabs).toEqual(['Pontos', '4d6']);
    expect(method).toBe('point_buy');
    expect(el.querySelector('input[name="ability-method"]')).toHaveProperty('checked', true);
  });

  it('labels the three SRD 5.2.1 ways with the 2024 label and "Créditos", and "Digitar" without it', async () => {
    await setup();
    expect(text()).toContain('Conjunto padrão');
    expect(text()).toContain('SRD 5.2.1 (regras de 2024) · Créditos');
    expect(el.querySelector('a[href="/credits"]')).not.toBeNull();
    tab('4d6').click();
    await settle();
    expect(text()).toContain('4d6, descartando o menor');
    expect(text()).toContain('SRD 5.2.1 (regras de 2024)');
    tab('Digitar').click();
    await settle();
    expect(text()).toContain('Digitar os valores');
    expect(text()).not.toContain('2024');
  });

  describe('the standard array', () => {
    it("lists the server's six values, and is incomplete until each one is on an ability", async () => {
      await setup();
      expect(method).toBe('standard_array');
      expect(incomplete).toBe(true);
      expect(problem).toBe('coloque cada valor do conjunto numa habilidade');
      expect(text()).toContain('Faltam 6 habilidades');
      const select = el.querySelectorAll('select')[0] as HTMLSelectElement;
      expect(Array.from(select.options).map((o) => o.textContent?.trim())).toEqual([
        'Escolher',
        '15',
        '14',
        '13',
        '12',
        '10',
        '8',
      ]);
    });

    it('writes each placed value into the form and completes after the sixth', async () => {
      await setup();
      const selects = Array.from(el.querySelectorAll('select')) as HTMLSelectElement[];
      for (const [i, s] of selects.entries()) {
        s.value = String(i);
        s.dispatchEvent(new Event('change'));
        await settle();
      }
      expect(scores()).toEqual([15, 14, 13, 12, 10, 8]);
      expect(incomplete).toBe(false);
      expect(problem).toBe('');
    });
  });

  describe('point buy', () => {
    async function pointBuy(t = table()) {
      await setup(t);
      tab('Pontos').click();
      await settle();
    }
    const plus = (name: string) =>
      el.querySelector<HTMLButtonElement>(`button[aria-label="Aumentar ${name}"]`)!;
    const minus = (name: string) =>
      el.querySelector<HTMLButtonElement>(`button[aria-label="Diminuir ${name}"]`)!;

    it('starts every score at 8 with all 27 points left, and "−" dashed at 8', async () => {
      await pointBuy();
      expect(method).toBe('point_buy');
      expect(scores()).toEqual([8, 8, 8, 8, 8, 8]);
      expect(text()).toContain('Restam 27 pontos');
      expect(text()).toContain('0 de 27 gastos');
      expect(minus('Força').classList).toContain('buy__btn--off');
      expect(minus('Força').getAttribute('aria-disabled')).toBe('true');
      expect(plus('Força').getAttribute('aria-disabled')).toBe('false');
      expect(incomplete).toBe(false);
    });

    it("starts every score at the lowest of the server's table, not at a fixed 8", async () => {
      await pointBuy(
        table({ pointBuyMinScore: 6, pointBuyCosts: [0, 1, 2, 3, 4, 5, 7, 9], pointBuyBudget: 20 }),
      );
      expect(scores()).toEqual([6, 6, 6, 6, 6, 6]);
      expect(text()).toContain('Restam 20 pontos');
      expect(text()).toContain('Cada valor vai de 6 a 13');
    });

    it("adds the server's costs: 15, 14, 13, 10, 10 and 8 leaves 2 points", async () => {
      await pointBuy();
      const set = (name: string, to: number) => {
        for (let v = 8; v < to; v++) plus(name).click();
      };
      for (const [name, to] of [
        ['Sabedoria', 15],
        ['Destreza', 14],
        ['Constituição', 13],
        ['Força', 10],
        ['Carisma', 10],
      ] as const) {
        set(name, to);
        fixture.detectChanges();
      }
      await settle();
      expect(scores()).toEqual([10, 14, 13, 8, 15, 10]);
      expect(text()).toContain('Restam 2 pontos');
      expect(text()).toContain('25 de 27 gastos');
      expect(text()).toContain('custo 9 pontos');
      expect(text()).toContain('8 = 0');
      expect(text()).toContain('15 = 9');
    });

    it('turns "+" off when the next point costs more than what is left, and at 15', async () => {
      await pointBuy();
      for (let i = 0; i < 7; i++) plus('Força').click();
      await settle();
      expect(scores()[0]).toBe(15);
      expect(plus('Força').classList).toContain('buy__btn--off');
      for (let i = 0; i < 7; i++) plus('Destreza').click();
      await settle();
      for (let i = 0; i < 7; i++) plus('Constituição').click();
      await settle();
      // 27 − 9 − 9 − 9 = 0 left: nothing else goes up.
      expect(text()).toContain('Restam 0 pontos');
      expect(plus('Inteligência').classList).toContain('buy__btn--off');
      const before = scores();
      plus('Inteligência').click();
      await settle();
      expect(scores()).toEqual(before);
    });
  });

  describe('4d6', () => {
    async function dice(t: AbilityTableVm) {
      await setup(t);
      tab('4d6').click();
      await settle();
    }

    it('asks the server for the roll, never the browser, and shows the dice with the kept sum and when', async () => {
      await dice(table());
      expect(text()).toContain('Rolar as habilidades');
      expect(incomplete).toBe(true);
      expect(problem).toBe('role as habilidades');
      const spy = vi.spyOn(crypto, 'getRandomValues');
      button('Rolar as habilidades').click();
      await settle();
      expect(roll).toHaveBeenCalledTimes(1);
      expect(roll).toHaveBeenCalledWith('camp-1', undefined);
      expect(spy).not.toHaveBeenCalled();
      expect(text()).toContain('Rolados em 05/10 20:14. Rolar de novo mostra os mesmos.');
      expect(el.querySelectorAll('app-dice-result')).toHaveLength(6);
      expect(el.querySelector('app-dice-result')?.textContent).toContain('16');
      expect(button('Rolar de novo')).toBeUndefined();
      expect(problem).toBe('coloque cada resultado numa habilidade');
    });

    it('shows the roll the server already kept, the same on a reload, with no roll button', async () => {
      await dice(table({ rolls: STORED }));
      expect(button('Rolar as habilidades')).toBeUndefined();
      expect(el.querySelectorAll('app-dice-result')).toHaveLength(6);
      expect(roll).not.toHaveBeenCalled();
    });

    it('places the six results and completes', async () => {
      await dice(table({ rolls: STORED }));
      const selects = Array.from(el.querySelectorAll('select')) as HTMLSelectElement[];
      for (const [i, s] of selects.entries()) {
        s.value = String(i);
        s.dispatchEvent(new Event('change'));
        await settle();
      }
      expect(scores()).toEqual([16, 14, 13, 12, 10, 8]);
      expect(incomplete).toBe(false);
    });

    it('with physical dice types the four dice of each roll once, and the server stores them', async () => {
      await dice(table({ physicalDice: true }));
      expect(text()).toContain('Digite os quatro dados de cada rolagem.');
      expect(text()).toContain('Falta o primeiro dado da rolagem 1.');
      expect(button('Guardar os dados').classList).toContain('mr-button--off');
      const typed = [
        [6, 5, 5, 2],
        [5, 5, 4, 1],
        [5, 4, 4, 3],
        [4, 4, 4, 2],
        [4, 3, 3, 2],
        [3, 3, 2, null],
      ];
      const inputs = Array.from(el.querySelectorAll<HTMLInputElement>('.roll__die'));
      expect(inputs).toHaveLength(24);
      typed.flat().forEach((v, i) => {
        if (v !== null) {
          inputs[i].value = String(v);
          inputs[i].dispatchEvent(new Event('input'));
        }
      });
      await settle();
      expect(text()).toContain('Falta o quarto dado da rolagem 6.');
      button('Guardar os dados').click();
      await settle();
      expect(roll).not.toHaveBeenCalled();
      inputs[23].value = '1';
      inputs[23].dispatchEvent(new Event('input'));
      await settle();
      expect(text()).not.toContain('Falta o');
      roll.mockResolvedValueOnce({ ...STORED, typed: true });
      button('Guardar os dados').click();
      await settle();
      // Nothing is stored yet: the question lists the 24 dice and says it is for good.
      expect(roll).not.toHaveBeenCalled();
      expect(el.querySelector('.ask__title')?.textContent).toContain('Guardar estas rolagens?');
      expect(text()).toContain('Depois não dá para mudar');
      expect(text()).toContain('Rolagem 1: 6, 5, 5, 2');
      expect(text()).toContain('Rolagem 6: 3, 3, 2, 1');
      // "Voltar" stores nothing and brings the dice back as typed.
      button('Voltar').click();
      await settle();
      expect(roll).not.toHaveBeenCalled();
      expect(el.querySelectorAll<HTMLInputElement>('.roll__die')[0].value).toBe('6');
      button('Guardar os dados').click();
      await settle();
      button('Guardar as rolagens').click();
      await settle();
      expect(roll).toHaveBeenCalledWith('camp-1', [
        [6, 5, 5, 2],
        [5, 5, 4, 1],
        [5, 4, 4, 3],
        [4, 4, 4, 2],
        [4, 3, 3, 2],
        [3, 3, 2, 1],
      ]);
      expect(text()).toContain('Dados digitados em 05/10 20:14. Ficam guardados');
      expect(el.querySelectorAll('app-dice-result')).toHaveLength(6);
    });

    it('says why when the server refuses the roll, by the typed reason', async () => {
      await dice(table());
      roll.mockRejectedValueOnce(
        new ConnectError('x', Code.FailedPrecondition, undefined, [
          {
            desc: AbilityScoresRefusalSchema,
            value: create(AbilityScoresRefusalSchema, {
              reason: AbilityScoresRefusalReason.DICE_FORCED_PHYSICAL,
            }),
          },
        ]),
      );
      button('Rolar as habilidades').click();
      await settle();
      expect(text()).toContain(
        'Nesta campanha todos usam os próprios dados: digite os dados que você tirou.',
      );
    });
  });

  describe('when the rolls changed under the person', () => {
    const refusal = (reason: AbilityScoresRefusalReason) =>
      new ConnectError('x', Code.FailedPrecondition, undefined, [
        { desc: AbilityScoresRefusalSchema, value: create(AbilityScoresRefusalSchema, { reason }) },
      ]);

    async function physical() {
      await setup(table({ physicalDice: true }));
      tab('4d6').click();
      await settle();
      const inputs = Array.from(el.querySelectorAll<HTMLInputElement>('.roll__die'));
      STORED.sets
        .flatMap((x) => x.dice)
        .forEach((v, i) => {
          inputs[i].value = String(v);
          inputs[i].dispatchEvent(new Event('input'));
        });
      await settle();
      button('Guardar os dados').click();
      await settle();
    }

    it('shows the dice another tab stored when the typed ones are refused for it', async () => {
      await physical();
      roll.mockRejectedValueOnce(refusal(AbilityScoresRefusalReason.ROLLS_ALREADY_STORED));
      roll.mockResolvedValueOnce(STORED);
      button('Guardar as rolagens').click();
      await settle();
      expect(roll).toHaveBeenCalledTimes(2);
      expect(roll).toHaveBeenLastCalledWith('camp-1', undefined);
      expect(el.querySelectorAll('app-dice-result')).toHaveLength(6);
      expect(el.querySelector('.ask__title')).toBeNull();
    });

    it('keeps the refusal and asks again only once when the stored dice cannot be read either', async () => {
      await physical();
      roll.mockRejectedValue(refusal(AbilityScoresRefusalReason.ROLLS_ALREADY_STORED));
      button('Guardar as rolagens').click();
      await settle();
      expect(roll).toHaveBeenCalledTimes(2);
      expect(text()).toContain('Os dados já foram guardados e não mudam.');
    });

    it.each([
      AbilityScoresRefusalReason.DICE_FORCED_IN_APP,
      AbilityScoresRefusalReason.DICE_FORCED_PHYSICAL,
    ])(
      'asks the page to read the table again when the server refuses with reason %s',
      async (reason) => {
        await setup(table());
        const stale = vi.fn();
        fixture.componentInstance.staleTable.subscribe(stale);
        tab('4d6').click();
        await settle();
        roll.mockRejectedValueOnce(refusal(reason));
        button('Rolar as habilidades').click();
        await settle();
        expect(stale).toHaveBeenCalledTimes(1);
      },
    );

    it('does not ask the page to read the table again for any other refusal (positive control)', async () => {
      await setup(table());
      const stale = vi.fn();
      fixture.componentInstance.staleTable.subscribe(stale);
      tab('4d6').click();
      await settle();
      roll.mockRejectedValueOnce(refusal(AbilityScoresRefusalReason.METHOD_NOT_ALLOWED));
      button('Rolar as habilidades').click();
      await settle();
      expect(stale).not.toHaveBeenCalled();
    });
  });

  describe('a draft that already has a recorded method (RN-24)', () => {
    async function locked(
      method: AbilityMethodKey,
      values: number[],
      rolls: AbilityRollsVm | null = null,
    ) {
      g = group();
      (['str', 'dex', 'con', 'int', 'wis', 'cha'] as AbilityKey[]).forEach((k, i) =>
        g.controls[k].setValue(values[i]),
      );
      roll = vi.fn();
      TestBed.resetTestingModule();
      TestBed.configureTestingModule({
        providers: [
          provideRouter([]),
          { provide: CharacterEditorSource, useValue: { rollAbilityScores: roll } },
        ],
      });
      fixture = TestBed.createComponent(TableAbilityScores);
      fixture.componentRef.setInput('campaignId', 'camp-1');
      fixture.componentRef.setInput('group', g);
      fixture.componentRef.setInput('table', table());
      fixture.componentRef.setInput('locked', { method, rolls });
      fixture.componentInstance.problem.subscribe((v) => (problem = v));
      fixture.componentInstance.incomplete.subscribe((v) => (incomplete = v));
      el = fixture.nativeElement;
      await settle();
    }

    it('shows only that method, with no picker', async () => {
      await locked('point_buy', [10, 14, 13, 8, 15, 10]);
      expect(el.querySelector('input[name="ability-method"]')).toBeNull();
      expect(text()).toContain('Compra por pontos');
      expect(text()).toContain('Os valores desta ficha foram feitos por este jeito');
      expect(text()).toContain('Restam 2 pontos');
      expect(scores()).toEqual([10, 14, 13, 8, 15, 10]);
    });

    it('keeps the 4d6 the sheet was made with, placed as the scores are, and never rolls again', async () => {
      await locked('rolled_4d6', [16, 14, 13, 12, 10, 8], STORED);
      expect(button('Rolar as habilidades')).toBeUndefined();
      expect(el.querySelectorAll('app-dice-result')).toHaveLength(6);
      expect(fixture.componentInstance.incomplete()).toBe(false);
      expect(scores()).toEqual([16, 14, 13, 12, 10, 8]);
      expect(roll).not.toHaveBeenCalled();
    });

    it('the standard array comes back placed', async () => {
      await locked('standard_array', [12, 15, 13, 14, 10, 8]);
      expect(fixture.componentInstance.incomplete()).toBe(false);
      expect(text()).toContain('Os seis resultados estão colocados.');
    });
  });

  describe('typed values', () => {
    it('takes six values from 3 to 18 and is incomplete outside them (a 19 is refused)', async () => {
      await setup();
      tab('Digitar').click();
      await settle();
      expect(method).toBe('typed');
      expect(incomplete).toBe(false);
      expect(text()).toContain('de 3 a 18, antes do bônus da raça');
      g.controls.str.setValue(19);
      await settle();
      expect(incomplete).toBe(true);
      expect(problem).toBe('digite valores de 3 a 18');
      g.controls.str.setValue(2);
      await settle();
      expect(incomplete).toBe(true);
      g.controls.str.setValue(18);
      g.controls.dex.setValue(3);
      await settle();
      expect(incomplete).toBe(false);
    });
  });
});
