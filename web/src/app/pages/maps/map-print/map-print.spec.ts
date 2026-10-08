import { TestBed, type ComponentFixture } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap, provideRouter } from '@angular/router';
import { BehaviorSubject, of } from 'rxjs';

import { Role } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { CampaignsService } from '../../../core/campaigns/campaigns.service';
import { mapMessage, mapResponse } from '../../../core/maps/maps-testing';
import { MapsClient } from '../../../core/maps/maps-client';
import { MapPrint } from './map-print';

/** The artboard's map: 30 x 20 squares on a 3.000 x 2.000 px image. */
const gridded = () =>
  mapMessage('map-1', 'Estrada do Vale', {
    gridColumns: 30,
    gridRows: 20,
    image: {
      id: 'i',
      url: '/images/i',
      thumbnailUrl: '',
      width: 3000,
      height: 2000,
      name: 'x',
    } as never,
  });

/** Detects changes and turns the no-break spaces the page writes ("1 cm",
 * "76,2 × 50,8") into plain ones, so the specs read the words. */
function settle(fixture: ComponentFixture<unknown>): void {
  fixture.detectChanges();
  const walker = document.createTreeWalker(fixture.nativeElement, NodeFilter.SHOW_TEXT);
  for (let node = walker.nextNode(); node; node = walker.nextNode()) {
    node.nodeValue = node.nodeValue!.replace(/\u00a0/g, ' ');
  }
}

async function render(
  options: { role?: Role; map?: ReturnType<typeof gridded> | null } = {},
): Promise<{ fixture: ComponentFixture<MapPrint>; el: HTMLElement }> {
  const map = options.map === undefined ? gridded() : options.map;
  TestBed.configureTestingModule({
    providers: [
      provideRouter([]),
      {
        provide: ActivatedRoute,
        useValue: { paramMap: of(convertToParamMap({ id: 'camp-1', mapId: 'map-1' })) },
      },
      {
        provide: CampaignsService,
        useValue: {
          getCampaign: () =>
            Promise.resolve({
              campaign: {
                name: 'Mirathel',
                myRole: options.role ?? Role.MASTER,
                awaitingApproval: false,
              },
            }),
        },
      },
      {
        provide: MapsClient,
        useValue: {
          get: () => (map ? Promise.resolve(mapResponse(map)) : Promise.reject(new Error('gone'))),
        },
      },
    ],
  });
  const fixture = TestBed.createComponent(MapPrint);
  settle(fixture);
  await new Promise((resolve) => setTimeout(resolve, 0));
  settle(fixture);
  return { fixture, el: fixture.nativeElement as HTMLElement };
}

function field(el: HTMLElement): HTMLInputElement {
  return el.querySelector<HTMLInputElement>('#square')!;
}

function type(fixture: ComponentFixture<MapPrint>, text: string): void {
  const input = field(fixture.nativeElement);
  input.value = text;
  input.dispatchEvent(new Event('input'));
  settle(fixture);
}

function choosePaper(fixture: ComponentFixture<MapPrint>, name: string): void {
  const radios = Array.from(
    (fixture.nativeElement as HTMLElement).querySelectorAll<HTMLInputElement>('input[type=radio]'),
  );
  const radio = radios.find((r) =>
    r.closest('mat-radio-button')?.textContent?.trim().startsWith(name),
  )!;
  radio.click();
  settle(fixture);
}

function summaryText(el: HTMLElement): string {
  return el.querySelector('.summary')!.textContent!.replace(/\s+/g, ' ').trim();
}

function printButton(el: HTMLElement): HTMLButtonElement {
  return Array.from(el.querySelectorAll('button')).find((b) =>
    b.textContent?.includes('Imprimir'),
  )!;
}

describe('MapPrint: o campo do tamanho do quadrado (E8-12)', () => {
  it('abre com 2,54 cm, A4 e as contas do mapa de 30 × 20 quadrados', async () => {
    const { el } = await render();
    expect(field(el).value).toBe('2,54');
    expect(summaryText(el)).toContain('76,2 × 50,8 cm');
    expect(summaryText(el)).toContain('em 9 folhas A4');
    expect(el.textContent).toContain('Paisagem usa 9 folhas e retrato usa 10');
    expect(el.textContent).toContain('Folhas A1 a C3 · 9 no total · paisagem');
    expect(
      Array.from(el.querySelectorAll('button')).some((b) =>
        b.textContent?.includes('Voltar a 2,54'),
      ),
    ).toBe(false);
  });

  it('aceita vírgula e ponto, e mostra o que foi digitado, sem completar com ",00"', async () => {
    const { fixture, el } = await render();
    type(fixture, '5');
    expect(field(el).value).toBe('5');
    expect(summaryText(el)).toContain('150,0 × 100,0 cm');
    type(fixture, '2.5');
    expect(field(el).value).toBe('2.5');
    expect(summaryText(el)).toContain('75,0 × 50,0 cm');
    type(fixture, '2,5');
    expect(summaryText(el)).toContain('75,0 × 50,0 cm');
  });

  it('2 cm em A3 dá 3 folhas em retrato, e a tabela marca "A menor"', async () => {
    const { fixture, el } = await render();
    type(fixture, '2');
    choosePaper(fixture, 'A3');
    expect(summaryText(el)).toContain('60,0 × 40,0 cm');
    expect(summaryText(el)).toContain('em 3 folhas A3');
    expect(el.querySelector('.block__main')!.textContent).toContain('Retrato');
    const rows = Array.from(el.querySelectorAll('app-print-table tbody tr'));
    const a3 = rows.find((r) => r.querySelector('th')!.textContent!.trim().startsWith('A3'))!;
    expect(a3.textContent).toContain('papel escolhido');
    expect(a3.textContent!.replace(/\s+/g, ' ')).toContain('3 × 1 = 3 folhas');
    expect(a3.querySelectorAll('.smallest').length).toBe(1);
    expect(a3.querySelectorAll('td')[1].querySelector('.smallest')).not.toBeNull();
  });

  for (const bad of ['0,5', '11', '', 'abc', '2,5,4']) {
    it(`"${bad}" mostra o erro, deixa o preview vazio e o Imprimir tracejado`, async () => {
      const { fixture, el } = await render();
      type(fixture, bad);
      expect(el.querySelector('.hint-error')!.textContent).toContain(
        'Use um tamanho de 1 a 10 cm.',
      );
      expect(field(el).getAttribute('aria-invalid')).toBe('true');
      expect(field(el).getAttribute('aria-describedby')).toBe('square-hint');
      expect(el.querySelector('app-print-preview svg')).toBeNull();
      expect(el.querySelector('app-print-table')).toBeNull();
      expect(summaryText(el)).toContain('— × — cm');
      expect(printButton(el).getAttribute('aria-disabled')).toBe('true');
      expect(printButton(el).querySelector('.mat-icon')!.textContent).toContain('block');
    });
  }

  it('"Voltar a 2,54 cm" aparece só quando o valor muda e o devolve', async () => {
    const { fixture, el } = await render();
    type(fixture, '3');
    const back = Array.from(el.querySelectorAll('button')).find((b) =>
      b.textContent?.includes('Voltar a 2,54 cm'),
    )!;
    expect(back).toBeTruthy();
    back.click();
    settle(fixture);
    expect(field(el).value).toBe('2,54');
    expect(
      Array.from(el.querySelectorAll('button')).some((b) =>
        b.textContent?.includes('Voltar a 2,54'),
      ),
    ).toBe(false);
    // "2,540" is still the default's value, but the field shows what was typed:
    // only the exact default text hides the action.
    type(fixture, '2.54');
    expect(
      Array.from(el.querySelectorAll('button')).some((b) =>
        b.textContent?.includes('Voltar a 2,54'),
      ),
    ).toBe(true);
  });

  it('o Imprimir chama window.print só com um tamanho válido', async () => {
    const { fixture, el } = await render();
    const print = vi.spyOn(window, 'print').mockImplementation(() => undefined);
    printButton(el).click();
    expect(print).toHaveBeenCalledTimes(1);
    type(fixture, '0');
    printButton(el).click();
    expect(print).toHaveBeenCalledTimes(1);
    print.mockRestore();
  });
});

describe('MapPrint: números e unidades não se partem', () => {
  it('"1 cm", "3 linhas" e o "×" levam espaço sem quebra, e o ponto nunca começa uma linha', async () => {
    const { el } = await render();
    const { tightCm } = await import('./print-math');
    expect(tightCm('76,2 × 50,8 cm')).toBe('76,2\u00a0×\u00a050,8\u00a0cm');
    expect(tightCm('em 9 folhas')).toBe('em 9\u00a0folhas');
    expect(tightCm('De 1 a 10 cm')).toBe('De 1\u00a0a\u00a010\u00a0cm');
    expect(el.querySelector('.summary .hint')!.textContent).toContain(
      'margem de 1 cm · 1 cm de sobreposição',
    );
  });
});

describe('MapPrint: os avisos de muitas folhas', () => {
  it('até 16 folhas só há o aviso da escala', async () => {
    const { fixture, el } = await render();
    type(fixture, '5');
    choosePaper(fixture, 'A3'); // 16 sheets
    expect(summaryText(el)).toContain('em 16 folhas A3');
    expect(el.querySelectorAll('.mr-notice--warning').length).toBe(1);
  });

  it('passando de 16 o aviso nomeia o papel que gasta menos, e a prévia encolhe os rótulos', async () => {
    const { fixture, el } = await render();
    type(fixture, '5'); // A4: 36 sheets
    const notices = Array.from(el.querySelectorAll('.mr-notice--warning')).map((n) =>
      n.textContent!.replace(/\s+/g, ' '),
    );
    expect(notices.length).toBe(2);
    expect(notices[0]).toContain('São 36 folhas.');
    expect(notices[0]).toContain('No A1 o mesmo mapa gasta 4 folhas (paisagem, 2 × 2)');
    expect(notices[1]).toContain('escala em 100%');
    const chips = el.querySelectorAll('app-print-preview .chip');
    expect(chips.length).toBe(36);
    expect(el.querySelector('app-print-preview .chip--small')).not.toBeNull();
  });

  it('com 17 a 36 folhas os rótulos ficam e encolhem; com 16 ou menos, não encolhem', async () => {
    const { fixture, el } = await render();
    expect(el.querySelectorAll('app-print-preview .chip').length).toBe(9);
    expect(el.querySelector('app-print-preview .chip--small')).toBeNull();
    type(fixture, '5');
    choosePaper(fixture, 'A3');
    expect(el.querySelector('app-print-preview .chip--small')).toBeNull();
  });

  it('passando de 36 folhas a prévia esconde os rótulos, e nada trava', async () => {
    const { fixture, el } = await render();
    type(fixture, '10'); // A4: 136 sheets
    expect(summaryText(el)).toContain('em 136 folhas A4');
    expect(el.querySelectorAll('app-print-preview .chip').length).toBe(0);
    expect(el.querySelector('app-print-preview svg')).not.toBeNull();
    expect(el.querySelector('.mr-notice--warning')!.textContent).toContain('São 136 folhas.');
    expect(printButton(el).getAttribute('aria-disabled')).not.toBe('true');
  });

  it('quando o papel escolhido já é o que gasta menos, o aviso diz só que um quadrado menor ajuda', async () => {
    // A wide map (120 squares of 10 cm): even A1, the biggest paper, needs more than 16.
    const wide = { ...gridded(), gridColumns: 120, gridRows: 80 };
    const { fixture, el } = await render({ map: wide });
    type(fixture, '10');
    choosePaper(fixture, 'A1');
    const text = el.querySelector('.mr-notice--warning')!.textContent!.replace(/\s+/g, ' ');
    expect(text).not.toContain('No A1 o mesmo mapa');
    expect(text).toContain('Um quadrado menor gasta menos folhas.');
  });
});

describe('MapPrint: o que o mestre vê e o que os outros veem', () => {
  it('um mapa sem grade mostra o motivo e não mostra o campo', async () => {
    const { el } = await render({ map: mapMessage('map-1', 'Sem grade') });
    expect(el.textContent).toContain('Este mapa ainda não tem grade.');
    expect(el.querySelector('#square')).toBeNull();
    expect(el.querySelector('a[href$="/grid"]')!.textContent).toContain('Definir a grade');
  });

  it('um jogador recebe só a resposta de que a impressão é do mestre', async () => {
    const { el } = await render({ role: Role.PLAYER });
    expect(el.textContent).toContain('Só o mestre imprime o mapa.');
    expect(el.querySelector('#square')).toBeNull();
    expect(el.querySelector('app-print-sheets')).toBeNull();
  });

  it('um mapa que não existe diz isso', async () => {
    const { el } = await render({ map: null });
    expect(el.textContent).toContain('Não foi possível abrir o mapa');
  });

  it('as folhas que saem na impressora são uma por página, com o rótulo de colar', async () => {
    const { fixture, el } = await render();
    // Not in the page until the browser is about to print.
    expect(el.querySelector('app-print-sheets')).toBeNull();
    window.dispatchEvent(new Event('beforeprint'));
    settle(fixture);
    const sheets = el.querySelectorAll('app-print-sheets .sheet');
    expect(sheets.length).toBe(9);
    expect(sheets[4].querySelector('.label')!.textContent).toBe(
      'Página B2 · cole à direita da B1 e abaixo da A2',
    );
    expect(sheets[0].querySelector('.edge--above')).toBeNull();
    expect(sheets[4].querySelectorAll('.edge').length).toBe(4);
    expect(sheets[4].querySelector('.ruler')!.textContent).toContain('confira a escala');
    const sources = Array.from(el.querySelectorAll('app-print-sheets img')).map((i) =>
      i.getAttribute('src'),
    );
    expect(new Set(sources)).toEqual(new Set(['/images/i']));
    window.dispatchEvent(new Event('afterprint'));
    settle(fixture);
    expect(el.querySelector('app-print-sheets')).toBeNull();
  });

  it('escreve o @page do papel escolhido e o tira ao sair', async () => {
    const { fixture, el } = await render();
    const css = () =>
      Array.from(document.head.querySelectorAll('style'))
        .map((s) => s.textContent)
        .join('');
    expect(css()).toContain('@page { size: 29.7cm 21cm; margin: 1cm; }');
    type(fixture, '2');
    choosePaper(fixture, 'A3'); // portrait
    TestBed.tick();
    expect(css()).toContain('@page { size: 29.7cm 42cm; margin: 1cm; }');
    expect(el).toBeTruthy();
    fixture.destroy();
    expect(css()).not.toContain('@page');
  });
});

describe('MapPrint: a resposta atrasada de outra rota', () => {
  it('não troca o mapa nem o nome da campanha que a página já mostra', async () => {
    const params = new BehaviorSubject(convertToParamMap({ id: 'camp-1', mapId: 'map-1' }));
    const answers = new Map<string, (value: unknown) => void>();
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        { provide: ActivatedRoute, useValue: { paramMap: params } },
        {
          provide: CampaignsService,
          useValue: {
            getCampaign: (id: string) =>
              new Promise((resolve) => {
                answers.set(id, resolve);
              }),
          },
        },
        {
          provide: MapsClient,
          useValue: {
            get: (_campaign: string, mapId: string) =>
              Promise.resolve(mapResponse(mapMessage(mapId, `Mapa ${mapId}`))),
          },
        },
      ],
    });
    const fixture = TestBed.createComponent(MapPrint);
    settle(fixture);
    params.next(convertToParamMap({ id: 'camp-2', mapId: 'map-2' }));
    settle(fixture);
    const campaign = (name: string) => ({
      campaign: { name, myRole: Role.MASTER, awaitingApproval: false },
    });
    answers.get('camp-2')!(campaign('Segunda'));
    await new Promise((resolve) => setTimeout(resolve, 0));
    settle(fixture);
    // The first campaign answers last: it is no longer the one on screen.
    answers.get('camp-1')!(campaign('Primeira'));
    await new Promise((resolve) => setTimeout(resolve, 0));
    settle(fixture);
    const text = (fixture.nativeElement as HTMLElement).textContent ?? '';
    expect(text).toContain('Segunda');
    expect(text).not.toContain('Primeira');
  });
});
