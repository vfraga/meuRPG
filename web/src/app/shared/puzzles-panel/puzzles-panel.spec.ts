import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { Code, ConnectError } from '@connectrpc/connect';

import { textOf } from '../../core/format/text-testing';
import { PuzzlesClient } from '../../core/puzzles/puzzles-client';
import {
  FakePuzzlesClient,
  lightsPuzzle,
  lockPuzzle,
  pillarsPuzzle,
} from '../../core/puzzles/puzzles-testing';
import { PuzzlesPanel } from './puzzles-panel';
import { nextTurn } from '../../../test-settle';

describe('PuzzlesPanel (MR-038, E10-06 state 1)', () => {
  let api: FakePuzzlesClient;

  async function render(prep: (a: FakePuzzlesClient) => void = () => undefined) {
    api = new FakePuzzlesClient();
    prep(api);
    TestBed.configureTestingModule({
      providers: [provideRouter([]), { provide: PuzzlesClient, useValue: api }],
    });
    const fixture = TestBed.createComponent(PuzzlesPanel);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    document.body.append(fixture.nativeElement);
    const settle = async () => {
      for (let i = 0; i < 3; i++) {
        fixture.detectChanges();
        await nextTurn();
        await fixture.whenStable();
      }
      fixture.detectChanges();
    };
    await settle();
    return { el: fixture.nativeElement as HTMLElement, settle, fixture };
  }

  const four = () => [
    lightsPuzzle('a', 'O selo da Capela'),
    lockPuzzle('b', 'O cofre do Refeitório'),
    pillarsPuzzle('c', 'Os pilares da Galeria'),
    lightsPuzzle('d', 'Os candelabros da cripta', {
      config: { kind: { case: 'lights', value: { size: 7 } } },
    }),
  ];
  const named = (el: HTMLElement, name: string) =>
    el.querySelector<HTMLElement>(`[aria-label="${name}"]`)!;

  afterEach(() => {
    document.body.replaceChildren();
  });

  it('lists the four puzzles with their kind, their line and their state, and "Novo quebra-cabeça"', async () => {
    const { el } = await render((a) => (a.listResult = four()));
    expect(el.querySelector('h2')?.textContent).toBe('Quebra-cabeças');
    const rows = Array.from(el.querySelectorAll('.mr-list li'));
    expect(rows).toHaveLength(4);
    expect(textOf(rows[0])).toContain(
      'O selo da Capela Apagar as luzes · 5 × 5 Não mostrado Editar Arquivar',
    );
    expect(textOf(rows[1])).toContain('Fechadura de combinação · 4 rodas de runas');
    expect(textOf(rows[2])).toContain('Símbolos giratórios · 4 pilares, girando juntos');
    expect(textOf(rows[3])).toContain('Apagar as luzes · 7 × 7');
    const add = Array.from(el.querySelectorAll('a')).find((a) =>
      a.textContent?.includes('Novo quebra-cabeça'),
    )!;
    expect(add.getAttribute('href')).toBe('/campaigns/camp-1/puzzles/new');
    expect(el.textContent).toContain('Só você vê estes quebra-cabeças.');
    expect(named(el, 'Editar O selo da Capela').getAttribute('href')).toBe(
      '/campaigns/camp-1/puzzles/a/edit',
    );
  });

  it('invites the first puzzle when there is none', async () => {
    const { el } = await render();
    expect(el.textContent).toContain(
      'Nenhum quebra-cabeça ainda. Crie o primeiro para mostrar aos jogadores numa sessão.',
    );
    expect(el.querySelector('.mr-list')).toBeNull();
    expect(el.querySelector('a')?.getAttribute('href')).toBe('/campaigns/camp-1/puzzles/new');
  });

  it('has no "Editar" for a puzzle already shown: the server would refuse', async () => {
    const { el } = await render(
      (a) => (a.listResult = [lightsPuzzle('a', 'O selo', { shown: true })]),
    );
    expect(textOf(el.querySelector('.mr-list li'))).toContain('Já mostrado');
    expect(el.querySelector('[aria-label="Editar O selo"]')).toBeNull();
    expect(named(el, 'Arquivar O selo')).toBeTruthy();
  });

  it('asks in place before archiving: the question takes the row, nothing is archived before the second tap', async () => {
    const { el, settle } = await render((a) => (a.listResult = four()));
    named(el, 'Arquivar O selo da Capela').click();
    await settle();
    const ask = el.querySelector('app-map-ask')!;
    expect(ask.querySelector('h3')?.textContent).toBe('Arquivar “O selo da Capela”?');
    expect(document.activeElement).toBe(ask.querySelector('h3'));
    expect(ask.textContent).toContain('dá para desarquivar');
    expect(api.calls.some((c) => c[0] === 'archive')).toBe(false);
    expect(el.querySelector('[aria-label="Editar O selo da Capela"]')).toBeNull();
    // "Voltar" brings the row back and the focus to the button that asked.
    (
      Array.from(ask.querySelectorAll('button')).find(
        (b) => b.textContent?.trim() === 'Voltar',
      ) as HTMLElement
    ).click();
    await settle();
    expect(el.querySelector('app-map-ask')).toBeNull();
    expect(document.activeElement).toBe(named(el, 'Arquivar O selo da Capela'));
  });

  it('archives on the second tap, says so and reads the list again', async () => {
    const { el, settle } = await render((a) => (a.listResult = four()));
    named(el, 'Arquivar O selo da Capela').click();
    await settle();
    api.listResult = four().slice(1);
    (
      Array.from(el.querySelectorAll('app-map-ask button')).find(
        (b) => b.textContent?.trim() === 'Arquivar',
      ) as HTMLElement
    ).click();
    await settle();
    expect(api.calls.find((c) => c[0] === 'archive')).toEqual(['archive', 'camp-1', 'a']);
    expect(el.querySelectorAll('.mr-list li')).toHaveLength(3);
    expect(el.textContent).toContain('“O selo da Capela” foi arquivado.');
  });

  it('brings the archived ones back on request, with "Desarquivar", with no new read', async () => {
    const { el, settle } = await render(
      (a) =>
        (a.listResult = [
          ...four().slice(1),
          lightsPuzzle('z', 'O velho selo', { archived: true }),
        ]),
    );
    expect(el.querySelectorAll('.mr-list li')).toHaveLength(3);
    expect(api.calls.filter((c) => c[0] === 'list')).toHaveLength(1);
    const toggle = Array.from(el.querySelectorAll('button')).find((b) =>
      b.textContent?.includes('Mostrar os arquivados'),
    ) as HTMLElement;
    expect(toggle.getAttribute('aria-pressed')).toBe('false');
    toggle.click();
    await settle();
    expect(api.calls.filter((c) => c[0] === 'list')).toHaveLength(1);
    expect(toggle.getAttribute('aria-pressed')).toBe('true');
    expect(toggle.textContent).toContain('Mostrar os arquivados'); // one label, whatever its state
    const row = Array.from(el.querySelectorAll('.mr-list li')).at(-1)!;
    expect(textOf(row)).toContain('Arquivado');
    expect(textOf(row)).not.toContain('Editar');
    api.listResult = four().slice(1);
    (row.querySelector('[aria-label="Desarquivar O velho selo"]') as HTMLElement).click();
    await settle();
    expect(api.calls.find((c) => c[0] === 'unarchive')).toEqual(['unarchive', 'camp-1', 'z']);
    expect(el.textContent).toContain('“O velho selo” voltou para a lista.');
  });

  it('says every puzzle is archived, and how to get one back', async () => {
    const { el } = await render(
      (a) => (a.listResult = [lightsPuzzle('z', 'O velho selo', { archived: true })]),
    );
    expect(el.textContent).toContain('Todos os quebra-cabeças estão arquivados.');
    expect(el.querySelector('.mr-list')).toBeNull();
    expect(
      Array.from(el.querySelectorAll('button')).some((b) =>
        b.textContent?.includes('Mostrar os arquivados'),
      ),
    ).toBe(true);
  });

  it("moves the focus to the row that took the archived one's place", async () => {
    const { el, settle } = await render((a) => (a.listResult = four()));
    named(el, 'Arquivar O selo da Capela').click();
    await settle();
    api.listResult = four().slice(1);
    (
      Array.from(el.querySelectorAll('app-map-ask button')).find(
        (b) => b.textContent?.trim() === 'Arquivar',
      ) as HTMLElement
    ).click();
    await settle();
    expect(document.activeElement).toBe(named(el, 'Arquivar O cofre do Refeitório'));
  });

  it('says what went wrong, by code, and offers to try again', async () => {
    const { el, settle } = await render(
      (a) => (a.failWith = new ConnectError('x', Code.Unavailable)),
    );
    expect(el.querySelector('[role="alert"]')?.textContent).toContain('o servidor não respondeu');
    api.failWith = undefined;
    api.listResult = four();
    (
      Array.from(el.querySelectorAll('button')).find((b) =>
        b.textContent?.includes('Tentar de novo'),
      ) as HTMLElement
    ).click();
    await settle();
    expect(el.querySelectorAll('.mr-list li')).toHaveLength(4);
  });
});
