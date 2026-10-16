import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  CampaignPackageBlockedReason,
  PackageProblemKind,
  PackageProblemReason,
} from '../../../../gen/meurpg/campaignpackage/v1/campaignpackage_pb';
import { CampaignCreationRefusedReason } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { CampaignPackageClient } from '../../../core/campaign-package/campaign-package-client';
import {
  FakePackageClient,
  FakePartUploader,
  counts,
  creationRefused,
  importUpload,
  packageBlocked,
  previewOf,
} from '../../../core/campaign-package/campaign-package-testing';
import { ImportPartUploader } from '../../../core/campaign-package/import-part-uploader';
import { fingerprintOf } from '../../../core/campaign-package/package-copy';
import { PART_RETRIES } from '../../../core/campaign-package/import-upload';
import { OUTCOME_UNKNOWN } from '../../../core/connect/connect-errors';
import { plain } from '../../../core/images/gallery-testing';
import { UploadFailed } from '../../../core/images/upload-errors';
import { CampaignImportPage } from './campaign-import';
import { nextTurn } from '../../../../test-settle';

const MIB = 1024 * 1024;

/** A 35-byte package; the fake server cuts it into parts of 10 (four parts). */
function packageFile(name = 'Mirathel.meurpg.zip', lastModified = 1_700_000_000_000): File {
  return new File([new Uint8Array(35)], name, { lastModified });
}

function problem(kind: PackageProblemKind, reason: PackageProblemReason, name = '') {
  return {
    $typeName: 'meurpg.campaignpackage.v1.PackageProblem' as const,
    kind,
    name,
    reason,
    limit: 0n,
  };
}

describe('CampaignImportPage', () => {
  let pkg: FakePackageClient;
  let uploader: FakePartUploader;

  beforeEach(() => {
    TestBed.configureTestingModule({
      imports: [CampaignImportPage],
      providers: [
        provideRouter([]),
        { provide: CampaignPackageClient, useClass: FakePackageClient },
        { provide: ImportPartUploader, useClass: FakePartUploader },
      ],
    });
    pkg = TestBed.inject(CampaignPackageClient) as unknown as FakePackageClient;
    uploader = TestBed.inject(ImportPartUploader) as unknown as FakePartUploader;
    pkg.beginResult = () => Promise.resolve({ upload: importUpload() } as never);
  });

  afterEach(() => vi.useRealTimers());

  async function settle(fixture: ComponentFixture<CampaignImportPage>): Promise<void> {
    if (vi.isFakeTimers()) {
      await vi.advanceTimersByTimeAsync(0);
    } else {
      await nextTurn();
      await fixture.whenStable();
    }
    fixture.detectChanges();
  }

  async function render(): Promise<{
    el: HTMLElement;
    fixture: ComponentFixture<CampaignImportPage>;
  }> {
    const fixture = TestBed.createComponent(CampaignImportPage);
    fixture.detectChanges();
    await settle(fixture);
    return { el: fixture.nativeElement as HTMLElement, fixture };
  }

  /** The words of an element, without its icons' ligatures, no-break spaces read as plain ones. */
  function text(el: Element): string {
    const clone = el.cloneNode(true) as Element;
    clone.querySelectorAll('mat-icon').forEach((i) => i.remove());
    return plain(clone.textContent).replace(/\s+/g, ' ').trim();
  }

  const label = text;

  function control(el: HTMLElement, name: string): HTMLElement | undefined {
    return Array.from(el.querySelectorAll<HTMLElement>('button, a')).find((c) => label(c) === name);
  }

  function press(el: HTMLElement, name: string): void {
    const found = control(el, name);
    if (!found) {
      throw new Error(`no control "${name}"`);
    }
    found.click();
  }

  function pick(el: HTMLElement, file: File): void {
    const input = el.querySelector('input[type="file"]') as HTMLInputElement;
    Object.defineProperty(input, 'files', { value: [file], configurable: true });
    input.dispatchEvent(new Event('change'));
  }

  async function chooseAndSettle(
    fixture: ComponentFixture<CampaignImportPage>,
    file = packageFile(),
  ): Promise<void> {
    pick(fixture.nativeElement as HTMLElement, file);
    await settle(fixture);
  }

  describe('choosing the file', () => {
    it('opens on "Escolher o arquivo", with the drop area and the limits', async () => {
      const { el } = await render();
      expect(el.querySelector('h1')?.textContent).toBe('Minhas campanhas');
      expect(text(el)).toContain('Voltar para minhas campanhas');
      expect(el.querySelector('.mr-panel__title')?.textContent).toBe('Importar campanha');
      expect(text(el)).toContain('Arraste o arquivo .meurpg.zip aqui');
      expect(text(el)).toContain('ou');
      expect(text(el)).toContain(
        'Pacote de até 200 MB, com até 2.000 itens e nenhum acima de 10 MB.',
      );
      expect(document.activeElement).toBe(control(el, 'Escolher o arquivo'));
      expect(el.querySelector('input[type="file"]')?.getAttribute('accept')).toContain('.zip');
      expect(pkg.calls).toEqual([['getImport']]);
    });

    it('offers "Continuar o envio" when the server still holds an upload, and says how to resume', async () => {
      pkg.getImportResult = () =>
        Promise.resolve({
          upload: importUpload({ receivedParts: [1, 2], partCount: 4 }),
          fingerprint: 'x',
        } as never);
      const { el } = await render();
      expect(text(el)).toContain('Você tem um envio guardado.');
      expect(text(el)).toContain('2 de 4 partes');
      expect(text(el)).toContain(
        'Escolha o mesmo arquivo de novo e o envio continua de onde parou.',
      );
      expect(control(el, 'Continuar o envio')).toBeTruthy();
      // The focus still opens on the main button.
      expect(document.activeElement).toBe(control(el, 'Escolher o arquivo'));
    });

    it('says a failure to read the kept upload and still lets the person choose', async () => {
      pkg.getImportResult = () => Promise.reject(new ConnectError('x', Code.Unavailable));
      const { el } = await render();
      expect(el.querySelector('[role="alert"]')?.textContent).toContain('Não foi possível falar');
      expect(control(el, 'Escolher o arquivo')).toBeTruthy();
    });

    it('refuses a file that is not a zip, with a friendly message and no call', async () => {
      const { el, fixture } = await render();
      await chooseAndSettle(fixture, packageFile('Mirathel.pdf'));
      expect(text(el.querySelector('[role="alert"]') as HTMLElement)).toContain(
        'Esse arquivo não parece um pacote de campanha. Ele precisa terminar em .meurpg.zip (ou .zip).',
      );
      expect(pkg.callsOf('beginImport')).toEqual([]);
    });

    it('keeps the refusal of a file chosen before the kept upload was read', async () => {
      let answer: (value: never) => void = () => undefined;
      pkg.getImportResult = () =>
        new Promise((resolve) => {
          answer = resolve;
        });
      const fixture = TestBed.createComponent(CampaignImportPage);
      fixture.detectChanges();
      await chooseAndSettle(fixture, packageFile('notas.txt'));
      answer({
        upload: importUpload({ receivedParts: [1], partCount: 2 }),
        fingerprint: 'x',
      } as never);
      await settle(fixture);
      const el = fixture.nativeElement as HTMLElement;
      expect(el.querySelector('[role="alert"]')?.textContent).toContain('não parece um pacote');
    });

    it('refuses a file over 200 MiB before any byte is sent', async () => {
      const { el, fixture } = await render();
      const big = packageFile();
      Object.defineProperty(big, 'size', { value: 200 * MIB + 1 });
      await chooseAndSettle(fixture, big);
      expect(text(el.querySelector('[role="alert"]') as HTMLElement)).toBe(
        'Esse arquivo passa de 200 MB, o limite de um pacote.',
      );
      expect(pkg.callsOf('beginImport')).toEqual([]);
      expect(uploader.sent).toEqual([]);
    });

    it('takes a file dropped on the area', async () => {
      const { el, fixture } = await render();
      const drop = new Event('drop', { cancelable: true });
      Object.defineProperty(drop, 'dataTransfer', { value: { files: [packageFile()] } });
      el.querySelector('.drop')!.dispatchEvent(drop);
      await settle(fixture);
      expect(drop.defaultPrevented).toBe(true);
      expect(pkg.callsOf('beginImport')).toHaveLength(1);
    });
  });

  describe('uploading', () => {
    it('begins with the name, size and fingerprint, then sends the parts one by one and reads the package', async () => {
      const { el, fixture } = await render();
      const file = packageFile();
      await chooseAndSettle(fixture, file);

      expect(pkg.callsOf('beginImport')).toEqual([
        ['Mirathel.meurpg.zip', 35, fingerprintOf(file)],
      ]);
      expect(uploader.partNumbers).toEqual([1, 2, 3, 4]);
      expect(uploader.sent.every((s) => s.importId === 'imp-1')).toBe(true);
      expect(pkg.callsOf('previewImport')).toEqual([['imp-1']]);
      expect(el.querySelector('#preview-heading')?.textContent?.trim()).toBe('Prévia do pacote');
    });

    it('says the part and the percent in a polite live region, and has a cancel button', async () => {
      let release: () => void = () => undefined;
      const gate = new Promise<void>((resolve) => (release = resolve));
      uploader.behave = (call) => (call.part === 3 ? gate : Promise.resolve());
      const { el, fixture } = await render();
      await chooseAndSettle(fixture);

      const live = el.querySelector('.progress-text') as HTMLElement;
      expect(live.getAttribute('aria-live')).toBe('polite');
      expect(text(live)).toBe(
        'Enviando em partes: parte 3 de 4 (75%). Se a conexão cair, continua de onde parou. As partes ficam guardadas por 1 hora.',
      );
      expect(text(el)).toContain('Mirathel.meurpg.zip · 1 KB');
      expect(el.querySelector('[role="progressbar"]')).not.toBeNull();
      expect(control(el, 'Cancelar o envio')).toBeTruthy();
      expect(document.activeElement).toBe(el.querySelector('#import-heading'));

      release();
      await settle(fixture);
      expect(el.querySelector('#preview-heading')).not.toBeNull();
    });

    it('skips the parts the server already holds: choosing the same file resumes', async () => {
      pkg.beginResult = () =>
        Promise.resolve({ upload: importUpload({ receivedParts: [1, 2, 4] }) } as never);
      const { fixture } = await render();
      await chooseAndSettle(fixture);
      expect(uploader.partNumbers).toEqual([3]);
      expect(pkg.callsOf('previewImport')).toHaveLength(1);
    });

    it('sends a failed part again by itself, then goes on', async () => {
      vi.useFakeTimers();
      uploader.behave = (call, attempt) =>
        call.part === 2 && attempt < PART_RETRIES
          ? Promise.reject(new UploadFailed('NETWORK'))
          : Promise.resolve();
      const { el, fixture } = await render();
      await chooseAndSettle(fixture);
      expect(uploader.partNumbers).toEqual([1, 2]);
      await vi.advanceTimersByTimeAsync(800);
      await vi.advanceTimersByTimeAsync(1600);
      fixture.detectChanges();
      expect(uploader.partNumbers).toEqual([1, 2, 2, 2, 3, 4]);
      expect(el.querySelector('#preview-heading')).not.toBeNull();
      expect(el.querySelector('#import-heading')).toBeNull();
    });

    it('shows the failure card when a part keeps failing, and "Tentar de novo" sends only what is missing', async () => {
      vi.useFakeTimers();
      uploader.behave = (call) =>
        call.part === 3 ? Promise.reject(new UploadFailed('NETWORK')) : Promise.resolve();
      const { el, fixture } = await render();
      await chooseAndSettle(fixture);
      await vi.advanceTimersByTimeAsync(800 + 1600);
      fixture.detectChanges();

      expect(text(el.querySelector('[role="alert"]') as HTMLElement)).toBe(
        'O envio não terminou. A conexão caiu durante o envio.',
      );
      expect(text(el)).toContain('“Tentar de novo” usa as partes que já foram enviadas.');
      expect(control(el, 'Tentar de novo')).toBeTruthy();
      expect(control(el, 'Voltar às campanhas')?.getAttribute('href')).toBe('/campaigns');
      expect(pkg.callsOf('previewImport')).toEqual([]);

      // The server now holds parts 1 and 2; the connection is back.
      uploader.behave = () => Promise.resolve();
      pkg.beginResult = () =>
        Promise.resolve({ upload: importUpload({ receivedParts: [1, 2] }) } as never);
      uploader.sent.length = 0;
      press(el, 'Tentar de novo');
      await settle(fixture);
      expect(pkg.callsOf('beginImport')).toHaveLength(2);
      // The same file, so the same fingerprint: the server finds its own upload.
      const [first, second] = pkg.callsOf('beginImport');
      expect(second).toEqual(first);
      expect(uploader.partNumbers).toEqual([3, 4]);
      expect(el.querySelector('#preview-heading')).not.toBeNull();
    });

    it('cancels: stops the part, asks the server to drop the upload, and goes back to choosing', async () => {
      let abortSignal: AbortSignal | undefined;
      uploader.upload = (importId, part, bytes, options) => {
        uploader.sent.push({ importId, part, size: bytes.size });
        abortSignal = options?.signal;
        return new Promise<void>((_resolve, reject) => {
          options?.signal?.addEventListener('abort', () => reject(new UploadFailed('CANCELED')));
        });
      };
      const { el, fixture } = await render();
      await chooseAndSettle(fixture);
      expect(uploader.partNumbers).toEqual([1]);

      press(el, 'Cancelar o envio');
      await settle(fixture);
      expect(abortSignal?.aborted).toBe(true);
      expect(pkg.callsOf('cancelImport')).toEqual([['imp-1']]);
      expect(uploader.partNumbers).toEqual([1]);
      expect(control(el, 'Escolher o arquivo')).toBeTruthy();
      expect(el.querySelector('[role="alert"]')).toBeNull();
      expect(pkg.callsOf('previewImport')).toEqual([]);
      expect(el.querySelector('p.mr-visually-hidden[role="status"]')?.textContent).toBe(
        'Envio cancelado.',
      );
    });

    it('stops the upload when the page goes, without cancelling it on the server (so it resumes)', async () => {
      let abortSignal: AbortSignal | undefined;
      uploader.upload = (_i, _p, _b, options) => {
        abortSignal = options?.signal;
        return new Promise<void>(() => undefined);
      };
      const { fixture } = await render();
      await chooseAndSettle(fixture);
      fixture.destroy();
      expect(abortSignal?.aborted).toBe(true);
      expect(pkg.callsOf('cancelImport')).toEqual([]);
    });

    it('goes back to choosing, with the reason, when the server refuses the file itself', async () => {
      pkg.beginResult = () => Promise.reject(new ConnectError('too big', Code.InvalidArgument));
      const { el, fixture } = await render();
      await chooseAndSettle(fixture);
      expect(text(el.querySelector('[role="alert"]') as HTMLElement)).toContain(
        'Esse arquivo não pôde ser enviado',
      );
      expect(control(el, 'Escolher o arquivo')).toBeTruthy();
      expect(uploader.sent).toEqual([]);
    });

    it('shows the failure card, not a server message, when the server is down', async () => {
      pkg.beginResult = () =>
        Promise.reject(new ConnectError('connection refused', Code.Unavailable));
      const { el, fixture } = await render();
      await chooseAndSettle(fixture);
      expect(text(el)).not.toContain('connection refused');
      expect(control(el, 'Tentar de novo')).toBeTruthy();
    });
  });

  describe('the preview', () => {
    it('shows the counts, the headline and "Tudo pode ser criado.", with "Criar campanha" first', async () => {
      const { el, fixture } = await render();
      await chooseAndSettle(fixture);

      expect(text(el.querySelector('.headline') as HTMLElement)).toBe(
        'Mirathel · exportado em 08/10 · formato do pacote 1',
      );
      const rows = Array.from(el.querySelectorAll('.counts__row')).map(
        (r) => `${text(r.querySelector('dt')!)} ${text(r.querySelector('dd')!)}`,
      );
      expect(rows).toEqual([
        'Mapas 6',
        'NPCs e criaturas 24',
        'Cenas 9',
        'Quebra-cabeças 3',
        'Pontos de batalha e encontros 7',
        'Pontos de tesouro 5',
        'Imagens 58 (82 MB)',
        'Conteúdo da mesa 6 entradas',
        'Personagens (reservados) 4',
      ]);
      expect(text(el)).toContain('Tudo pode ser criado. Nenhum item foi recusado.');
      const buttons = Array.from(el.querySelectorAll('.actions > *')).map(label);
      expect(buttons).toEqual(['Criar campanha', 'Escolher outro arquivo']);
      expect(document.activeElement).toBe(el.querySelector('#preview-heading'));
    });

    it('lists every problem in plain words when the package is refused, and offers only "Escolher outro arquivo"', async () => {
      pkg.previewResult = () =>
        Promise.resolve(
          previewOf({
            problems: [
              problem(
                PackageProblemKind.IMAGE,
                PackageProblemReason.ENTRY_TOO_BIG,
                'Mapa antigo.png',
              ),
              problem(
                PackageProblemKind.SCENE,
                PackageProblemReason.UNKNOWN_CONTENT,
                'A ponte quebrada',
              ),
              problem(PackageProblemKind.CHARACTER, PackageProblemReason.NOT_IN_PACKAGE, 'Orla'),
            ],
          }),
        );
      const { el, fixture } = await render();
      await chooseAndSettle(fixture);

      expect(text(el)).toContain(
        'Este pacote não pode ser criado. Nada é criado pela metade: corrija o pacote e envie de novo.',
      );
      expect(Array.from(el.querySelectorAll('.problems li')).map(label)).toEqual([
        'A imagem “Mapa antigo.png” tem mais de 10 MB.',
        'A cena “A ponte quebrada” usa uma ação de teste que este servidor ainda não conhece.',
        'O personagem “Orla” usa uma classe da mesa que não está no conteúdo do pacote.',
      ]);
      expect(control(el, 'Criar campanha')).toBeUndefined();
      expect(Array.from(el.querySelectorAll('button, a')).map(label)).toContain(
        'Escolher outro arquivo',
      );
      expect(el.querySelector('.counts')).toBeNull();
    });

    it('lists the first 20 problems and says how many more there are', async () => {
      const many = Array.from({ length: 23 }, (_, i) =>
        problem(PackageProblemKind.MAP, PackageProblemReason.INVALID, `Mapa ${i}`),
      );
      pkg.previewResult = () => Promise.resolve(previewOf({ problems: many }));
      const { el, fixture } = await render();
      await chooseAndSettle(fixture);
      expect(el.querySelectorAll('.problems li')).toHaveLength(20);
      expect(text(el)).toContain('E mais 3 problemas.');
    });

    it('shows the newer-version card alone, even beside other problems', async () => {
      pkg.previewResult = () =>
        Promise.resolve(
          previewOf({
            campaignName: '',
            problems: [
              problem(PackageProblemKind.PACKAGE, PackageProblemReason.NEWER_VERSION),
              problem(PackageProblemKind.MAP, PackageProblemReason.UNKNOWN_FIELD, 'X'),
            ],
          }),
        );
      const { el, fixture } = await render();
      await chooseAndSettle(fixture);
      expect(text(el)).toContain(
        'Este pacote é de uma versão mais nova do MeuRPG, e este servidor ainda não sabe abri-lo. Peça a quem cuida do servidor para atualizá-lo, ou exporte de novo pelo app antigo.',
      );
      expect(el.querySelector('.problems')).toBeNull();
      expect(control(el, 'Criar campanha')).toBeUndefined();
      expect(control(el, 'Escolher outro arquivo')).toBeTruthy();
    });

    it('goes back to the first card with "Escolher outro arquivo"', async () => {
      const { el, fixture } = await render();
      await chooseAndSettle(fixture);
      press(el, 'Escolher outro arquivo');
      await settle(fixture);
      expect(control(el, 'Escolher o arquivo')).toBeTruthy();
      expect(document.activeElement).toBe(control(el, 'Escolher o arquivo'));
    });
  });

  describe('creating the campaign', () => {
    async function toPreview() {
      const ctx = await render();
      await chooseAndSettle(ctx.fixture);
      return ctx;
    }

    it('creates with the import and a key, and shows "Campanha criada" with the counts of the answer', async () => {
      const { el, fixture } = await toPreview();
      press(el, 'Criar campanha');
      await settle(fixture);

      const [id, key] = pkg.callsOf('createFromImport')[0] as [string, string];
      expect(id).toBe('imp-1');
      expect(key.length).toBeGreaterThan(0);
      expect(el.querySelector('#done-heading')?.textContent).toBe('Campanha criada');
      expect(text(el.querySelector('.mr-notice--success') as HTMLElement)).toBe(
        'Mirathel foi criada com 6 mapas, 24 NPCs e criaturas e 4 personagens reservados.',
      );
      expect(control(el, 'Abrir a campanha')?.getAttribute('href')).toBe('/campaigns/camp-new');
      expect(text(el)).toContain('Próximo passo: mande a cada jogador o link do personagem dele.');
    });

    it('pluralises the done card and leaves a zero out', async () => {
      pkg.createResult = () =>
        Promise.resolve({
          campaign: { id: 'c2', name: 'Ilha' },
          counts: counts({ maps: 1, npcs: 0, characters: 1 }),
        } as never);
      const { el, fixture } = await toPreview();
      press(el, 'Criar campanha');
      await settle(fixture);
      expect(text(el.querySelector('.mr-notice--success') as HTMLElement)).toBe(
        'Ilha foi criada com 1 mapa e 1 personagem reservado.',
      );
    });

    it('does not suggest the next step when no character came', async () => {
      pkg.createResult = () =>
        Promise.resolve({
          campaign: { id: 'c3', name: 'Vazia' },
          counts: counts({ maps: 2, npcs: 1, characters: 0 }),
        } as never);
      const { el, fixture } = await toPreview();
      press(el, 'Criar campanha');
      await settle(fixture);
      expect(text(el.querySelector('.mr-notice--success') as HTMLElement)).toBe(
        'Vazia foi criada com 2 mapas e 1 NPC ou criatura.',
      );
      expect(text(el)).not.toContain('Próximo passo');
    });

    it('shows the failure card that says nothing was left behind, and retries with the SAME key', async () => {
      const { el, fixture } = await toPreview();
      pkg.createResult = () =>
        Promise.reject(new ConnectError('conn reset by peer', Code.Unavailable));
      press(el, 'Criar campanha');
      await settle(fixture);

      const alert = text(el.querySelector('[role="alert"]') as HTMLElement);
      expect(alert).toContain('A importação falhou no meio e nada foi criado.');
      expect(alert).toContain('Nenhuma campanha, imagem ou personagem ficou para trás.');
      expect(alert).not.toContain('conn reset');
      expect(text(el)).toContain('“Tentar de novo” usa as partes que já foram enviadas.');

      pkg.createResult = () =>
        Promise.resolve({
          campaign: { id: 'camp-new', name: 'Mirathel' },
          counts: counts(),
        } as never);
      pkg.beginResult = () =>
        Promise.resolve({
          upload: importUpload({ receivedParts: [1, 2, 3, 4] }),
        } as never);
      uploader.sent.length = 0;
      press(el, 'Tentar de novo');
      await settle(fixture);

      const creates = pkg.callsOf('createFromImport');
      expect(creates).toHaveLength(2);
      expect(creates[1]).toEqual(creates[0]);
      // Every part was still on the server: nothing is sent again, and no preview is asked a second time.
      expect(uploader.sent).toEqual([]);
      expect(pkg.callsOf('previewImport')).toHaveLength(1);
      expect(el.querySelector('#done-heading')).not.toBeNull();
    });

    it('begins again with the same file when the upload was lost, and sends all the parts', async () => {
      const { el, fixture } = await toPreview();
      pkg.createResult = () => Promise.reject(new ConnectError('gone', Code.NotFound));
      press(el, 'Criar campanha');
      await settle(fixture);
      expect(text(el)).toContain('O envio não existe mais.');
      expect(text(el)).toContain('Nenhuma campanha, imagem ou personagem ficou para trás.');

      pkg.createResult = () =>
        Promise.resolve({
          campaign: { id: 'camp-new', name: 'Mirathel' },
          counts: counts(),
        } as never);
      pkg.beginResult = () =>
        Promise.resolve({ upload: importUpload({ id: 'imp-2', receivedParts: [] }) } as never);
      uploader.sent.length = 0;
      press(el, 'Tentar de novo');
      await settle(fixture);
      expect(uploader.partNumbers).toEqual([1, 2, 3, 4]);
      const creates = pkg.callsOf('createFromImport');
      expect(creates[creates.length - 1][0]).toBe('imp-2');
      expect(el.querySelector('#done-heading')).not.toBeNull();
    });

    it('does not claim nothing was created when the server could not say', async () => {
      const { el, fixture } = await toPreview();
      pkg.createResult = () => Promise.reject(new ConnectError('tx', Code.Unknown));
      press(el, 'Criar campanha');
      await settle(fixture);
      const alert = text(el.querySelector('[role="alert"]') as HTMLElement);
      expect(alert).toContain('Não deu para confirmar se a campanha foi criada.');
      expect(alert).toContain(OUTCOME_UNKNOWN);
      expect(alert).not.toContain('Nenhuma campanha, imagem ou personagem ficou para trás.');
      expect(text(el)).toContain('“Tentar de novo” é seguro');
    });

    it('shows the refused package when the server finds problems at creation (HAS_PROBLEMS)', async () => {
      const { el, fixture } = await toPreview();
      pkg.createResult = () =>
        Promise.reject(
          packageBlocked(
            CampaignPackageBlockedReason.HAS_PROBLEMS,
            previewOf({
              problems: [
                problem(PackageProblemKind.MAP, PackageProblemReason.NOT_IN_PACKAGE, 'Cripta'),
              ],
            }),
          ),
        );
      press(el, 'Criar campanha');
      await settle(fixture);
      expect(text(el)).toContain('Este pacote não pode ser criado.');
      expect(text(el)).toContain(
        'O mapa “Cripta” usa um mapa ou uma imagem que não está no pacote.',
      );
      expect(control(el, 'Criar campanha')).toBeUndefined();
    });

    it('does not create twice from a double tap', async () => {
      const { el, fixture } = await toPreview();
      const create = control(el, 'Criar campanha') as HTMLButtonElement;
      create.click();
      create.click();
      await settle(fixture);
      expect(pkg.callsOf('createFromImport')).toHaveLength(1);
    });
  });

  describe('the campaign cap', () => {
    it('says "Você já tem 10 campanhas." with the number of the detail, before any byte is sent', async () => {
      pkg.beginResult = () =>
        Promise.reject(creationRefused(CampaignCreationRefusedReason.LIMIT_REACHED, 7));
      const { el, fixture } = await render();
      await chooseAndSettle(fixture);
      expect(text(el)).toContain(
        'Você já tem 7 campanhas. Este é o limite por conta. Apague uma campanha que você não usa mais e envie o pacote de novo.',
      );
      expect(control(el, 'Voltar às campanhas')?.getAttribute('href')).toBe('/campaigns');
      expect(control(el, 'Tentar de novo')).toBeUndefined();
      expect(uploader.sent).toEqual([]);
    });

    it('shows the same card when it is the creation that is refused', async () => {
      const { el, fixture } = await render();
      await chooseAndSettle(fixture);
      pkg.createResult = () =>
        Promise.reject(creationRefused(CampaignCreationRefusedReason.LIMIT_REACHED, 10));
      press(el, 'Criar campanha');
      await settle(fixture);
      expect(text(el)).toContain('Você já tem 10 campanhas. Este é o limite por conta.');
    });

    it('uses the campaigns page wording for an account that may not create campaigns', async () => {
      pkg.beginResult = () =>
        Promise.reject(creationRefused(CampaignCreationRefusedReason.NOT_ALLOWED));
      const { el, fixture } = await render();
      await chooseAndSettle(fixture);
      expect(text(el)).toContain(
        'Este servidor só deixa algumas pessoas criarem campanhas. Peça ao mestre da sua mesa um convite para jogar.',
      );
      expect(control(el, 'Voltar às campanhas')).toBeTruthy();
    });
  });

  it('keeps nothing in the browser: the fingerprint is made from the file, never stored', async () => {
    const setItem = vi.spyOn(Storage.prototype, 'setItem');
    const { fixture } = await render();
    await chooseAndSettle(fixture);
    expect(setItem).not.toHaveBeenCalled();
    expect(plain(fingerprintOf(packageFile()))).toContain('Mirathel.meurpg.zip');
  });
});
