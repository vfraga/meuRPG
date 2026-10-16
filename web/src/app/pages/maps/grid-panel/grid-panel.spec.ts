import { ComponentFixture, TestBed } from '@angular/core/testing';
import { Code, ConnectError } from '@connectrpc/connect';
import { create } from '@bufbuild/protobuf';

import {
  MapBlockedSchema,
  MapBlockedReason,
  type Map as MapMessage,
} from '../../../../gen/meurpg/maps/v1/maps_pb';
import { FakeMapsClient, mapMessage } from '../../../core/maps/maps-testing';
import { MapsClient } from '../../../core/maps/maps-client';
import { GridPanel } from './grid-panel';
import { nextTurn } from '../../../../test-settle';

describe('GridPanel', () => {
  let fixture: ComponentFixture<GridPanel>;
  let el: HTMLElement;
  let api: FakeMapsClient;
  let changed: MapMessage[];

  async function setup(
    map: MapMessage,
    inputs: { erases?: boolean; combatRunning?: boolean } = {},
  ) {
    api = new FakeMapsClient();
    changed = [];
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({ providers: [{ provide: MapsClient, useValue: api }] });
    fixture = TestBed.createComponent(GridPanel);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput('map', map);
    fixture.componentRef.setInput('erases', inputs.erases ?? false);
    fixture.componentRef.setInput('combatRunning', inputs.combatRunning ?? false);
    fixture.componentInstance.changed.subscribe((m) => changed.push(m));
    fixture.detectChanges();
    el = fixture.nativeElement;
    await fixture.whenStable();
  }

  const button = (text: string) =>
    Array.from(el.querySelectorAll('button')).find((b) => b.textContent?.trim().endsWith(text))!;
  const text = () => (el.textContent ?? '').replace(/\u00a0/g, ' ').replace(/\s+/g, ' ');
  const settle = async () => {
    fixture.detectChanges();
    await nextTurn();
    await fixture.whenStable();
    fixture.detectChanges();
  };
  function type(value: string): void {
    const input = el.querySelector('input')!;
    input.value = value;
    input.dispatchEvent(new Event('input'));
    fixture.detectChanges();
  }

  const withGrid = mapMessage('map-1', 'A caverna do Vale Seco', { gridColumns: 24, gridRows: 16 });

  it('shows the grid in squares and the size of a square', async () => {
    await setup(withGrid);
    expect(text()).toContain('24 × 16 quadrados');
    expect(text()).toContain('1 quadrado = 1,5 m (5 pés)');
  });

  it('asks in place before it erases: the title has the focus, "Voltar" first, and nothing is sent yet', async () => {
    await setup(withGrid, { erases: true });
    button('Mudar a grade').click();
    await settle();
    expect(el.querySelector('h3')?.textContent).toContain('Mudar a grade?');
    expect(document.activeElement).toBe(el.querySelector('h3'));
    expect(text()).toContain(
      'apaga o terreno, as paredes, a cobertura, a luz e as portas pintados, e o que os jogadores já viram',
    );
    expect(text()).toContain('Voltar');
    expect(api.calls).toEqual([]);
    type('30');
    expect(text()).toContain(
      'Linhas: 20, pela proporção da imagem. A grade ficaria com 30 × 20 quadrados.',
    );
    expect(api.calls).toEqual([]);
    button('Apagar e mudar a grade').click();
    await settle();
    expect(api.calls).toEqual(['setGrid map-1 30']);
    expect(changed).toHaveLength(1);
    expect(el.querySelector('h3')).toBeNull();
  });

  it('"Voltar" closes the question, sends nothing and gives the focus back to "Mudar a grade"', async () => {
    await setup(withGrid, { erases: true });
    button('Mudar a grade').click();
    await settle();
    button('Voltar').click();
    await settle();
    expect(api.calls).toEqual([]);
    expect(document.activeElement).toBe(button('Mudar a grade'));
  });

  it('changes straight away, with no warning, when nothing is painted or seen', async () => {
    await setup(withGrid, { erases: false });
    button('Mudar a grade').click();
    await settle();
    expect(text()).not.toContain('apaga o terreno');
    expect(button('Mudar a grade').closest('section')).toBeTruthy();
    type('12');
    Array.from(el.querySelectorAll('button'))
      .filter((b) => b.textContent?.trim() === 'Mudar a grade')
      .at(-1)!
      .click();
    await settle();
    expect(api.calls).toEqual(['setGrid map-1 12']);
  });

  it('refuses a number out of 4 to 200 and the button cannot act', async () => {
    await setup(withGrid);
    button('Mudar a grade').click();
    await settle();
    type('300');
    expect(text()).toContain('Use um número inteiro de 4 a 200.');
    const go = Array.from(el.querySelectorAll('button'))
      .filter((b) => b.textContent?.trim() === 'Mudar a grade')
      .at(-1)!;
    expect(go.classList).toContain('mr-button--off');
    go.click();
    expect(api.calls).toEqual([]);
  });

  it('while a combat runs the change is off, with the reason in a line', async () => {
    await setup(withGrid, { combatRunning: true });
    const change = button('Mudar a grade');
    expect(change.getAttribute('aria-disabled')).toBe('true');
    expect(change.classList).toContain('mr-button--off');
    expect(change.getAttribute('aria-describedby')).toBe('gp-why');
    expect(text()).toContain('Há um combate neste mapa. Termine-o para mudar a grade.');
    change.click();
    await settle();
    expect(el.querySelector('h3')).toBeNull();
  });

  it('without a grid asks for the columns and "Definir a grade"', async () => {
    await setup(mapMessage('map-1', 'Sem grade', { gridColumns: 0, gridRows: 0 }));
    expect(text()).toContain('Este mapa ainda não tem grade.');
    expect(text()).toContain('de 4 a 200');
    type('24');
    expect(text()).toContain('24 × 16 quadrados');
    button('Definir a grade').click();
    await settle();
    expect(api.calls).toEqual(['setGrid map-1 24']);
  });

  it('says why when the server refuses (a combat began meanwhile), by the typed reason', async () => {
    await setup(withGrid);
    api.failWith = new ConnectError('x', Code.FailedPrecondition, undefined, [
      {
        desc: MapBlockedSchema,
        value: create(MapBlockedSchema, { reason: MapBlockedReason.COMBAT_RUNNING }),
      },
    ]);
    button('Mudar a grade').click();
    await settle();
    type('30');
    Array.from(el.querySelectorAll('button'))
      .filter((b) => b.textContent?.trim() === 'Mudar a grade')
      .at(-1)!
      .click();
    await settle();
    expect(text()).toContain('Há um combate neste mapa: a grade e a imagem só mudam depois dele.');
    expect(changed).toEqual([]);
  });

  it('the same size is no change: the button is off and says so, and nothing is erased', async () => {
    await setup(withGrid, { erases: true });
    button('Mudar a grade').click();
    await settle();
    expect(text()).toContain('A grade já tem 24 colunas. Escolha outro número para mudar.');
    const go = button('Apagar e mudar a grade');
    expect(go.classList).toContain('mr-button--off');
    go.click();
    await settle();
    expect(api.calls).toEqual([]);
  });
  describe('the calibration (RN-25)', () => {
    const at3m = mapMessage('map-2', 'A torre em ruínas', {
      gridColumns: 24,
      gridRows: 16,
      drawnColumns: 12,
      drawnRows: 8,
      squareFactor: 2,
    });
    const radio = (label: string) =>
      Array.from(el.querySelectorAll<HTMLInputElement>('input[type="radio"]')).find(
        (i) =>
          i
            .closest('label')
            ?.querySelector('.dice-choice__title')
            ?.textContent?.replace(/\u00a0/g, ' ')
            .trim() === label,
      )!;

    it("says both grids of a calibrated map: the drawing, what a square is worth, and the rules' squares", async () => {
      await setup(at3m);
      expect(text()).toContain('12 × 8 quadrados do desenho');
      expect(text()).toContain('cada um vale 3 m');
      expect(text()).toContain('nas regras: 24 × 16 quadrados de 1,5 m');
      expect(button('Calibrar o quadrado')).toBeTruthy();
    });

    it('"Mudar a grade" on a calibrated map changes the drawing\'s columns and keeps the factor', async () => {
      await setup(at3m);
      button('Mudar a grade').click();
      await settle();
      expect(el.querySelector<HTMLInputElement>('input')!.value).toBe('12');
      type('20');
      // The rules\' grid stays within 200 columns: 100 drawn columns at most at 3 m.
      Array.from(el.querySelectorAll('button'))
        .filter((b) => b.textContent?.trim() === 'Mudar a grade')
        .at(-1)!
        .click();
      await settle();
      expect(api.calls).toEqual(['setGrid map-2 20 x2']);
    });

    it('opens "Cada quadrado deste desenho vale" with the four values and "Outro", on what the map has now', async () => {
      await setup(withGrid);
      button('Calibrar o quadrado').click();
      await settle();
      expect(el.querySelector('h3')?.textContent).toContain('Cada quadrado deste desenho vale');
      expect(document.activeElement).toBe(el.querySelector('h3'));
      expect(
        Array.from(el.querySelectorAll('.dice-choice__title')).map((t) =>
          t.textContent?.replace(/\u00a0/g, ' ').trim(),
        ),
      ).toEqual(['1,5 m', '3 m', '4,5 m', '6 m', 'Outro']);
      expect(radio('1,5 m').checked).toBe(true);
      expect(text()).toContain('O desenho já vale 1,5 m. Escolha outro valor para mudar.');
      expect(button('Salvar a grade').classList).toContain('mr-button--off');
    });

    it('a larger multiple keeps the layers scaled, so it saves at once, without "Mudar a grade?"', async () => {
      await setup(withGrid, { erases: true });
      button('Calibrar o quadrado').click();
      await settle();
      radio('3 m').click();
      await settle();
      expect(text()).toContain('o mapa terá 48 × 32 quadrados.');
      expect(text()).toContain('cada quadrado pintado vira 2 × 2');
      button('Salvar a grade').click();
      await settle();
      expect(el.querySelector('h3')?.textContent ?? '').not.toContain('Mudar a grade?');
      expect(api.calls).toEqual(['setGrid map-1 24 x2']);
      expect(changed).toHaveLength(1);
      expect(changed[0].squareFactor).toBe(2);
    });

    it('"Outro" starts at a value the four cards do not offer, and draws the drawing-to-rules picture', async () => {
      await setup(
        mapMessage('map-2', 'A torre em ruínas', {
          gridColumns: 12,
          gridRows: 8,
          drawnColumns: 12,
          drawnRows: 8,
          squareFactor: 1,
        }),
      );
      button('Calibrar o quadrado').click();
      await settle();
      radio('Outro').click();
      await settle();
      expect(el.querySelector<HTMLInputElement>('input[type="text"]')!.value).toBe('7,5');
      radio('3 m').click();
      await settle();
      expect(text()).toContain('No desenho');
      expect(text()).toContain('Nas regras');
      expect(el.querySelectorAll('.cal__cell')).toHaveLength(4);
      radio('4,5 m').click();
      await settle();
      expect(el.querySelectorAll('.cal__cell')).toHaveLength(9);
    });

    it('"Outro" takes a multiple of 1,5 m and says what the rules\' grid becomes', async () => {
      await setup(
        mapMessage('map-2', 'A torre em ruínas', {
          gridColumns: 12,
          gridRows: 8,
          drawnColumns: 12,
          drawnRows: 8,
          squareFactor: 1,
        }),
      );
      button('Calibrar o quadrado').click();
      await settle();
      radio('Outro').click();
      await settle();
      const field = el.querySelector<HTMLInputElement>('input[type="text"]')!;
      field.value = '4,5';
      field.dispatchEvent(new Event('input'));
      await settle();
      expect(text()).toContain('36 × 24 quadrados');
      expect(text()).toContain('cada quadrado pintado vira 3 × 3');
      field.value = '4';
      field.dispatchEvent(new Event('input'));
      await settle();
      expect(text()).toContain('Use um múltiplo de 1,5 m, de 1,5 m a 30 m, em passos de 1,5.');
      expect(button('Salvar a grade').classList).toContain('mr-button--off');
    });

    it('a change that is not a multiple clears the layers, and asks "Mudar a grade?" first when something is painted', async () => {
      await setup(at3m, { erases: true });
      button('Calibrar o quadrado').click();
      await settle();
      radio('4,5 m').click();
      await settle();
      button('Salvar a grade').click();
      await settle();
      expect(api.calls).toEqual([]);
      expect(el.querySelector('h3')?.textContent).toContain('Mudar a grade?');
      expect(text()).toContain(
        'mudar a grade apaga o terreno, as paredes, a cobertura, a luz e as portas pintados, e o que os jogadores já viram',
      );
      button('Voltar').click();
      await settle();
      expect(el.querySelector('h3')?.textContent).toContain('Cada quadrado deste desenho vale');
      button('Salvar a grade').click();
      await settle();
      button('Apagar e mudar a grade').click();
      await settle();
      expect(api.calls).toEqual(['setGrid map-2 12 x3']);
    });

    it('a change that clears nothing visible saves without asking', async () => {
      await setup(at3m, { erases: false });
      button('Calibrar o quadrado').click();
      await settle();
      radio('1,5 m').click();
      await settle();
      button('Salvar a grade').click();
      await settle();
      expect(api.calls).toEqual(['setGrid map-2 12']);
    });

    it("says why when a combat runs: the button is off with the reason, and the server's refusal reads the same", async () => {
      await setup(withGrid, { combatRunning: true });
      expect(button('Calibrar o quadrado').getAttribute('aria-disabled')).toBe('true');
      expect(text()).toContain('Há um combate neste mapa. Termine-o para mudar a grade.');

      await setup(withGrid);
      api.failWith = new ConnectError('x', Code.FailedPrecondition, undefined, [
        {
          desc: MapBlockedSchema,
          value: create(MapBlockedSchema, { reason: MapBlockedReason.COMBAT_RUNNING }),
        },
      ]);
      button('Calibrar o quadrado').click();
      await settle();
      radio('3 m').click();
      await settle();
      button('Salvar a grade').click();
      await settle();
      expect(text()).toContain(
        'Há um combate neste mapa: a grade e a imagem só mudam depois dele.',
      );
      expect(changed).toEqual([]);
    });

    it('refuses what would pass 200 × 400 rules squares', async () => {
      await setup(
        mapMessage('map-3', 'Grande', {
          gridColumns: 120,
          gridRows: 80,
          drawnColumns: 120,
          drawnRows: 80,
          squareFactor: 1,
        }),
      );
      button('Calibrar o quadrado').click();
      await settle();
      radio('3 m').click();
      await settle();
      expect(text()).toContain('as regras passariam de 200 × 400 quadrados');
      expect(button('Salvar a grade').classList).toContain('mr-button--off');
    });
  });
});
