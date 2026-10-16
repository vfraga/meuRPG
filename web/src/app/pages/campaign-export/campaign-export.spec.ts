import { ComponentFixture, TestBed } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap, provideRouter } from '@angular/router';
import { Code, ConnectError } from '@connectrpc/connect';
import { of } from 'rxjs';

import {
  CampaignExportFailure,
  CampaignExportState,
} from '../../../gen/meurpg/campaignpackage/v1/campaignpackage_pb';
import { Role } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { CampaignPackageClient } from '../../core/campaign-package/campaign-package-client';
import {
  FakePackageClient,
  doneExport,
  exportOf,
} from '../../core/campaign-package/campaign-package-testing';
import { CampaignsService } from '../../core/campaigns/campaigns.service';
import { plain } from '../../core/images/gallery-testing';
import { OUTCOME_UNKNOWN } from '../../core/connect/connect-errors';
import { CampaignExportPage, EXPORT_POLL_MS } from './campaign-export';
import { nextTurn } from '../../../test-settle';

const MIB = 1024 * 1024;

describe('CampaignExportPage', () => {
  let pkg: FakePackageClient;
  let role: Role;
  let campaignResult: () => Promise<unknown>;

  beforeEach(() => {
    role = Role.MASTER;
    campaignResult = () =>
      Promise.resolve({ campaign: { id: 'camp-1', name: 'Mirathel', myRole: role } });
    TestBed.configureTestingModule({
      imports: [CampaignExportPage],
      providers: [
        provideRouter([]),
        {
          provide: ActivatedRoute,
          useValue: { paramMap: of(convertToParamMap({ id: 'camp-1' })) },
        },
        { provide: CampaignsService, useValue: { getCampaign: () => campaignResult() } },
        { provide: CampaignPackageClient, useClass: FakePackageClient },
      ],
    });
    pkg = TestBed.inject(CampaignPackageClient) as unknown as FakePackageClient;
    pkg.getExportResult = () =>
      Promise.resolve({
        export: undefined,
        estimatedBytes: BigInt(84 * MIB),
        limitBytes: BigInt(200 * MIB),
      } as never);
  });

  afterEach(() => vi.useRealTimers());

  async function render(): Promise<{
    el: HTMLElement;
    fixture: ComponentFixture<CampaignExportPage>;
  }> {
    const fixture = TestBed.createComponent(CampaignExportPage);
    fixture.detectChanges();
    await settle(fixture);
    return { el: fixture.nativeElement as HTMLElement, fixture };
  }

  async function settle(fixture: ComponentFixture<CampaignExportPage>): Promise<void> {
    await nextTurn();
    await fixture.whenStable();
    fixture.detectChanges();
  }

  /** The words of a control, without its icon's ligature. */
  function label(control: Element): string {
    const clone = control.cloneNode(true) as Element;
    clone.querySelectorAll('mat-icon').forEach((i) => i.remove());
    return clone.textContent?.trim() ?? '';
  }

  function button(el: HTMLElement, name: string): HTMLButtonElement {
    const found = Array.from(el.querySelectorAll('button')).find((b) => label(b) === name);
    if (!found) {
      throw new Error(`no button "${name}"`);
    }
    return found;
  }

  const text = (el: Element) => plain(el.textContent).replace(/\s+/g, ' ').trim();

  describe('the master, with no export yet', () => {
    it('shows the campaign, what goes and what never goes, and the file with its size', async () => {
      const { el } = await render();
      expect(el.querySelector('h1')?.textContent).toBe('Mirathel');
      expect(text(el)).toContain('Configurações da campanha');
      expect(text(el)).toContain('Voltar para a campanha');
      expect(text(el)).toContain('Gera um arquivo com tudo o que é da mesa');
      expect(text(el)).toContain('É uma foto de agora: se há uma sessão aberta, ela fica de fora.');

      const lists = Array.from(el.querySelectorAll('ul.marks'));
      const yes = Array.from(lists[0].querySelectorAll('li')).map((li) =>
        label(li).replace(/\s+/g, ' '),
      );
      expect(yes).toHaveLength(9);
      expect(yes[0]).toBe('O documento, as anotações do mestre e a galeria (imagens e miniaturas)');
      expect(yes[8]).toBe('Todos os personagens, como reservados (sem dono)');
      const no = Array.from(lists[1].querySelectorAll('li')).map((li) =>
        label(li).replace(/\s+/g, ' '),
      );
      expect(no).toEqual([
        'As anotações privadas dos jogadores',
        'O histórico das sessões e os registros',
        'As contas: ids, e-mails e nomes',
        'Quem é dono e quem é membro',
        'Convites e links de personagem',
        'A memória de névoa de cada jogador',
      ]);

      expect(text(el.querySelector('.file__name') as HTMLElement)).toBe('Mirathel.meurpg.zip');
      expect(text(el.querySelector('.file__size') as HTMLElement)).toBe('cerca de 84 MB');
      expect(text(el)).toContain(
        'O tamanho vem das imagens e dos dados já guardados. O limite do pacote é 200 MB.',
      );
      expect(pkg.calls).toEqual([['getExport', 'camp-1']]);
    });

    it('opens with the focus on "Exportar campanha"', async () => {
      const { el } = await render();
      expect(document.activeElement).toBe(button(el, 'Exportar campanha'));
    });

    it('shows no card on the side', async () => {
      const { el } = await render();
      expect(el.querySelector('.side')?.children.length).toBe(0);
    });
  });

  describe('who may open it', () => {
    it('tells a player that only the master exports, and asks nothing of the export', async () => {
      role = Role.PLAYER;
      const { el } = await render();
      expect(text(el)).toContain('Só o mestre exporta a campanha.');
      expect(el.querySelector('button')).toBeNull();
      expect(pkg.calls).toEqual([]);
    });

    it('says "campanha não encontrada" for a campaign that is not there', async () => {
      campaignResult = () => Promise.reject(new ConnectError('nope', Code.NotFound));
      const { el } = await render();
      expect(el.querySelector('h1')?.textContent).toContain('Campanha não encontrada');
    });

    it('treats a refusal of the export call itself as the same calm notice', async () => {
      pkg.getExportResult = () => Promise.reject(new ConnectError('no', Code.PermissionDenied));
      const { el } = await render();
      expect(text(el)).toContain('Só o mestre exporta a campanha.');
    });

    it('says an error in Portuguese, with "Tentar de novo" that reads again', async () => {
      pkg.getExportResult = () => Promise.reject(new ConnectError('db exploded', Code.Internal));
      const { el, fixture } = await render();
      expect(el.querySelector('[role="alert"]')?.textContent).not.toContain('db exploded');
      expect(text(el)).toContain('Não foi possível falar com o servidor agora.');

      pkg.getExportResult = () =>
        Promise.resolve({ export: undefined, estimatedBytes: 0n, limitBytes: 0n } as never);
      button(el, 'Tentar de novo').click();
      await settle(fixture);
      expect(el.querySelector('h1')?.textContent).toBe('Mirathel');
    });
  });

  describe('exporting', () => {
    it('starts with one key, keeps it on a retry after a failure and renews it after success', async () => {
      const { el, fixture } = await render();
      pkg.startResult = () => Promise.reject(new ConnectError('lost', Code.Unavailable));
      button(el, 'Exportar campanha').click();
      await settle(fixture);
      expect(el.querySelector('.action-error')?.textContent).toContain('Não foi possível falar');

      button(el, 'Exportar campanha').click();
      await settle(fixture);
      const [first, second] = pkg.callsOf('startExport');
      expect(first).toEqual(['camp-1', second[1]]);
      expect(first[1]).toBe(second[1]);

      pkg.startResult = () =>
        Promise.resolve({
          export: exportOf({ state: CampaignExportState.RUNNING, percent: 5 }),
        } as never);
      button(el, 'Exportar campanha').click();
      await settle(fixture);
      expect(pkg.callsOf('startExport')[2][1]).toBe(first[1]);
      expect(el.querySelector('.action-error')).toBeNull();

      // That export ended (here: cancelled) and the master exports again: a new action, a new key.
      button(el, 'Cancelar').click();
      await settle(fixture);
      el.querySelector<HTMLButtonElement>('.primary')!.click();
      await settle(fixture);
      expect(pkg.callsOf('startExport')).toHaveLength(4);
      expect(pkg.callsOf('startExport')[3][1]).not.toBe(first[1]);
    });

    it('says the ambiguous outcome instead of a Go string', async () => {
      const { el, fixture } = await render();
      pkg.startResult = () => Promise.reject(new ConnectError('sql: tx', Code.Unknown));
      button(el, 'Exportar campanha').click();
      await settle(fixture);
      expect(el.querySelector('.action-error')?.textContent).toContain(OUTCOME_UNKNOWN);
    });

    it('shows "Exportando…" with the progress text, the bar and "Cancelar", and the button off', async () => {
      pkg.getExportResult = () =>
        Promise.resolve({
          export: exportOf({ state: CampaignExportState.RUNNING, percent: 62 }),
          estimatedBytes: BigInt(84 * MIB),
          limitBytes: BigInt(200 * MIB),
        } as never);
      const { el } = await render();
      expect(el.querySelector('#running-heading')?.textContent).toBe('Exportando…');
      expect(text(el.querySelector('.progress-text') as HTMLElement)).toBe(
        'Juntando os mapas e as imagens (62%). Pode fechar esta página: o arquivo fica pronto por 24 horas.',
      );
      expect(el.querySelector('.progress-text')?.getAttribute('aria-live')).toBe('polite');
      const bar = el.querySelector('[role="progressbar"]');
      expect(bar?.getAttribute('aria-valuenow')).toBe('62');
      expect(button(el, 'Cancelar')).toBeTruthy();
      expect((el.querySelector('.primary') as HTMLButtonElement).disabled).toBe(true);
    });

    it('cancels the running export with its id and shows the canceled card', async () => {
      pkg.getExportResult = () =>
        Promise.resolve({
          export: exportOf({ id: 'exp-9', state: CampaignExportState.RUNNING, percent: 10 }),
          estimatedBytes: 0n,
          limitBytes: 0n,
        } as never);
      const { el, fixture } = await render();
      button(el, 'Cancelar').click();
      await settle(fixture);
      expect(pkg.callsOf('cancelExport')).toEqual([['camp-1', 'exp-9']]);
      expect(el.querySelector('#canceled-heading')?.textContent).toBe('Exportação cancelada');
      expect(text(el)).toContain('Nenhum arquivo foi guardado.');
      expect(el.querySelector('[role="progressbar"]')).toBeNull();
      expect((el.querySelector('.primary') as HTMLButtonElement).disabled).toBe(false);
    });

    it('says a failed cancel and keeps the export running', async () => {
      pkg.getExportResult = () =>
        Promise.resolve({
          export: exportOf({ state: CampaignExportState.RUNNING }),
          estimatedBytes: 0n,
          limitBytes: 0n,
        } as never);
      const { el, fixture } = await render();
      pkg.cancelExportResult = () => Promise.reject(new ConnectError('x', Code.Unavailable));
      button(el, 'Cancelar').click();
      await settle(fixture);
      expect(el.querySelector('.action-error')?.textContent).toContain('Não foi possível falar');
      expect(el.querySelector('#running-heading')).not.toBeNull();
    });
  });

  describe('polling a running export', () => {
    function running(percent: number) {
      return Promise.resolve({
        export: exportOf({ state: CampaignExportState.RUNNING, percent }),
        estimatedBytes: BigInt(84 * MIB),
        limitBytes: BigInt(200 * MIB),
      } as never);
    }

    async function renderWithClock(): Promise<{
      el: HTMLElement;
      fixture: ComponentFixture<CampaignExportPage>;
    }> {
      vi.useFakeTimers();
      const fixture = TestBed.createComponent(CampaignExportPage);
      fixture.detectChanges();
      await vi.advanceTimersByTimeAsync(0);
      fixture.detectChanges();
      return { el: fixture.nativeElement as HTMLElement, fixture };
    }

    async function tick(
      fixture: ComponentFixture<CampaignExportPage>,
      ms = EXPORT_POLL_MS,
    ): Promise<void> {
      await vi.advanceTimersByTimeAsync(ms);
      fixture.detectChanges();
    }

    it('asks every 2 seconds, shows the new percent, and stops when it is done', async () => {
      pkg.getExportResult = () => running(20);
      const { el, fixture } = await renderWithClock();
      expect(pkg.callsOf('getExport')).toHaveLength(1);

      await tick(fixture, EXPORT_POLL_MS - 1);
      expect(pkg.callsOf('getExport')).toHaveLength(1);
      pkg.getExportResult = () => running(62);
      await tick(fixture, 1);
      expect(pkg.callsOf('getExport')).toHaveLength(2);
      expect(text(el.querySelector('.progress-text') as HTMLElement)).toContain('(62%)');

      pkg.getExportResult = () =>
        Promise.resolve({
          export: doneExport(new Date(), new Date(Date.now() + 24 * 60 * 60 * 1000)),
          estimatedBytes: 0n,
          limitBytes: 0n,
        } as never);
      await tick(fixture);
      expect(pkg.callsOf('getExport')).toHaveLength(3);
      expect(el.querySelector('#last-heading')?.textContent).toBe('Último pacote');

      await tick(fixture, EXPORT_POLL_MS * 5);
      expect(pkg.callsOf('getExport')).toHaveLength(3);
    });

    it('announces the change in the polite region when the file is ready', async () => {
      pkg.getExportResult = () => running(90);
      const { el, fixture } = await renderWithClock();
      pkg.getExportResult = () =>
        Promise.resolve({
          export: doneExport(new Date(), new Date()),
          estimatedBytes: 0n,
          limitBytes: 0n,
        } as never);
      await tick(fixture);
      expect(el.querySelector('p.mr-visually-hidden[role="status"]')?.textContent).toBe(
        'O pacote está pronto para baixar.',
      );
    });

    it('keeps asking after a blip, saying so, and clears the note when it answers', async () => {
      pkg.getExportResult = () => running(20);
      const { el, fixture } = await renderWithClock();
      pkg.getExportResult = () => Promise.reject(new ConnectError('x', Code.Unavailable));
      await tick(fixture);
      expect(el.querySelector('.hint[role="status"]')?.textContent).toContain('Tentando de novo');
      pkg.getExportResult = () => running(40);
      await tick(fixture);
      expect(el.querySelector('.hint[role="status"]')).toBeNull();
      expect(pkg.callsOf('getExport')).toHaveLength(3);
    });

    it('stops asking when the session is gone, and says so', async () => {
      pkg.getExportResult = () => running(20);
      const { el, fixture } = await renderWithClock();
      pkg.getExportResult = () => Promise.reject(new ConnectError('', Code.Unauthenticated));
      await tick(fixture);
      expect(text(el)).toContain('Sua sessão acabou.');
      await tick(fixture, EXPORT_POLL_MS * 3);
      expect(pkg.callsOf('getExport')).toHaveLength(2);
    });

    it('clears its timer when the page goes: no poll after the destroy', async () => {
      pkg.getExportResult = () => running(20);
      const { fixture } = await renderWithClock();
      fixture.destroy();
      await vi.advanceTimersByTimeAsync(EXPORT_POLL_MS * 5);
      expect(pkg.callsOf('getExport')).toHaveLength(1);
      expect(vi.getTimerCount()).toBe(0);
    });

    it('starts polling after "Exportar campanha"', async () => {
      pkg.getExportResult = () =>
        Promise.resolve({ export: undefined, estimatedBytes: 0n, limitBytes: 0n } as never);
      const { el, fixture } = await renderWithClock();
      pkg.startResult = () =>
        Promise.resolve({ export: exportOf({ state: CampaignExportState.RUNNING }) } as never);
      el.querySelector<HTMLButtonElement>('.primary')!.click();
      await vi.advanceTimersByTimeAsync(0);
      pkg.getExportResult = () => running(30);
      await tick(fixture);
      expect(pkg.callsOf('getExport')).toHaveLength(2);
    });
  });

  describe('the last package', () => {
    it('offers "Baixar de novo" as a plain download link, and says how long it is kept', async () => {
      const finished = new Date();
      pkg.getExportResult = () =>
        Promise.resolve({
          export: doneExport(finished, new Date(finished.getTime() + 24 * 60 * 60 * 1000)),
          estimatedBytes: 0n,
          limitBytes: 0n,
        } as never);
      const { el } = await render();
      expect(el.querySelector('#last-heading')?.textContent).toBe('Último pacote');
      expect(text(el.querySelector('.mr-notice--success') as HTMLElement)).toContain(
        'Mirathel.meurpg.zip · 84,2 MB · 312 itens · gerado hoje às',
      );
      const link = Array.from(el.querySelectorAll('a')).find((a) =>
        a.textContent?.includes('Baixar de novo'),
      ) as HTMLAnchorElement;
      expect(link.getAttribute('href')).toBe('/downloads/campaign-exports/abc123');
      expect(link.hasAttribute('download')).toBe(true);
      expect(text(el)).toContain('O arquivo fica guardado por 24 horas (até amanhã às');
      expect(text(el)).toContain(') e depois é apagado. Só o mestre baixa.');
    });
  });

  describe('a failed export', () => {
    it('says the package passes the limit, with no retry button (it would fail again)', async () => {
      pkg.getExportResult = () =>
        Promise.resolve({
          export: exportOf({
            state: CampaignExportState.FAILED,
            failure: CampaignExportFailure.TOO_BIG,
          }),
          estimatedBytes: 0n,
          limitBytes: BigInt(200 * MIB),
        } as never);
      const { el } = await render();
      expect(el.querySelector('#failed-heading')).not.toBeNull();
      expect(text(el)).toContain('O pacote passa do limite de 200 MB.');
      expect(el.querySelector('.side button')).toBeNull();
    });

    it('says an interrupted export stopped and offers "Exportar campanha" again', async () => {
      pkg.getExportResult = () =>
        Promise.resolve({
          export: exportOf({
            state: CampaignExportState.FAILED,
            failure: CampaignExportFailure.INTERRUPTED,
          }),
          estimatedBytes: 0n,
          limitBytes: 0n,
        } as never);
      const { el, fixture } = await render();
      expect(text(el.querySelector('.side') as HTMLElement)).toContain(
        'A exportação parou no meio.',
      );
      const again = el.querySelector('.side button') as HTMLButtonElement;
      expect(again.textContent?.trim()).toBe('Exportar campanha');
      again.click();
      await settle(fixture);
      expect(pkg.callsOf('startExport')).toHaveLength(1);
    });
  });
});
