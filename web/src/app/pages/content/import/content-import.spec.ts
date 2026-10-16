import { TestBed } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap, provideRouter } from '@angular/router';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';
import { of } from 'rxjs';

import { Role } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  ImportTableContentResponseSchema,
  TableContentKind,
  TableContentRefusalSchema,
  TableContentViolationSchema,
  TableImportMode,
  TableImportStatus,
} from '../../../../gen/meurpg/rules/v1/table_content_pb';
import { CampaignsService } from '../../../core/campaigns/campaigns.service';
import { TableContentClient } from '../../../core/content/content-client';
import { ContentImport } from './content-import';
import { settle } from '../../../../test-settle';

const PACK = JSON.stringify({
  format: 'meurpg.table-content',
  version: 1,
  name: 'Pacote do Vale',
  entries: [],
});

function answer(
  over: Parameters<typeof create<typeof ImportTableContentResponseSchema>>[1],
  mode = TableImportMode.PREVIEW,
) {
  return create(ImportTableContentResponseSchema, { mode, packName: 'Pacote do Vale', ...over });
}

const entries = [
  {
    key: 'class:a@mesa',
    kind: TableContentKind.CLASS,
    namePt: 'Guardião',
    status: TableImportStatus.NEW,
  },
  {
    key: 'spell:b@mesa',
    kind: TableContentKind.SPELL,
    namePt: 'Lâmina',
    status: TableImportStatus.UPDATED,
    changedFields: ['name_pt', 'table_spell.desc_pt'],
  },
  {
    key: 'feat:c@mesa',
    kind: TableContentKind.FEAT,
    namePt: 'Lutador',
    status: TableImportStatus.UNCHANGED,
  },
];

describe('ContentImport (MR-025)', () => {
  const importPack = vi.fn();
  const getCampaign = vi.fn();

  async function setup(role = Role.MASTER) {
    importPack.mockReset();
    getCampaign.mockReset().mockResolvedValue({
      campaign: { id: 'camp-1', name: 'Mirathel', myRole: role, awaitingApproval: false },
    });
    TestBed.resetTestingModule();
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        {
          provide: ActivatedRoute,
          useValue: { paramMap: of(convertToParamMap({ id: 'camp-1' })) },
        },
        { provide: CampaignsService, useValue: { getCampaign } },
        { provide: TableContentClient, useValue: { importPack } },
      ],
    });
    const fixture = TestBed.createComponent(ContentImport);
    await settle(fixture, 4);
    return { fixture, el: fixture.nativeElement as HTMLElement };
  }

  const text = (e: Element) => (e.textContent ?? '').replace(/ /g, ' ').replace(/\s+/g, ' ');
  const button = (el: HTMLElement, label: string) =>
    Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
      text(b).includes(label),
    )!;

  /** Chooses a file the way the browser hands it over. */
  async function choose(
    fixture: { detectChanges(): void; whenStable(): Promise<unknown> },
    el: HTMLElement,
    content: string,
    size = content.length,
  ) {
    const input = el.querySelector<HTMLInputElement>('input[type="file"]')!;
    const file = new File([content], 'pacote.json', { type: 'application/json' });
    Object.defineProperty(file, 'size', { value: size });
    Object.defineProperty(input, 'files', { value: [file], configurable: true });
    input.dispatchEvent(new Event('change'));
    await settle(fixture, 4);
  }

  it('tells a player that only the master imports', async () => {
    const { el } = await setup(Role.PLAYER);
    expect(text(el)).toContain('Só o mestre importa conteúdo.');
    expect(el.querySelector('input[type="file"]')).toBeNull();
  });

  it('asks for the file first and explains nothing is imported before the preview', async () => {
    const { el } = await setup();
    expect(text(el.querySelector('h1')!)).toBe('Importar pacote');
    expect(text(el)).toContain('Nada é importado antes de você ver o que muda');
    expect(button(el, 'Escolher arquivo')).toBeDefined();
  });

  it('refuses a file that is not a pack before calling the server', async () => {
    const { fixture, el } = await setup();
    await choose(fixture, el, '{ isto não é json');
    expect(importPack).not.toHaveBeenCalled();
    expect(text(el.querySelector('[role="alert"]')!)).toContain('não é um JSON válido');
    await choose(fixture, el, PACK, 3 * 1024 * 1024);
    expect(text(el.querySelector('[role="alert"]')!)).toContain('2 MiB');
    await choose(fixture, el, JSON.stringify({ format: 'outro' }));
    expect(text(el.querySelector('[role="alert"]')!)).toContain('meurpg.table-content');
    expect(importPack).not.toHaveBeenCalled();
  });

  it('previews the entries by kind and by outcome, saying what an update changes, and applies with a key', async () => {
    const { fixture, el } = await setup();
    importPack.mockResolvedValueOnce(
      answer({ entries, totals: { new: 1, updated: 1, unchanged: 1, refused: 0 } }),
    );
    await choose(fixture, el, PACK);
    expect(importPack.mock.calls[0].slice(2, 3)).toEqual(['preview']);
    // The server gets the file's own bytes, not a parsed pack.
    expect(ArrayBuffer.isView(importPack.mock.calls[0][1])).toBe(true);
    expect(new TextDecoder().decode(importPack.mock.calls[0][1])).toBe(PACK);
    expect(importPack.mock.calls[1]).toBeUndefined();
    const page = text(el);
    expect(page).toContain('Pacote do Vale');
    expect(page).toContain('Classes 1');
    expect(page).toContain('Magias 1');
    expect(page).toContain('Talentos 1');
    expect(page).toContain('Novas · 1');
    expect(page).toContain('Atualizadas · 1');
    expect(page).toContain('Sem mudança · 1');
    expect(page).toContain('Muda o nome e o texto.');
    expect(button(el, 'Importar 2 entradas').disabled).toBe(false);

    importPack.mockResolvedValueOnce(
      answer(
        { entries, totals: { new: 1, updated: 1, unchanged: 1, refused: 0 } },
        TableImportMode.APPLY,
      ),
    );
    button(el, 'Importar 2 entradas').click();
    await settle(fixture, 4);
    const [campaign, , mode, key] = importPack.mock.calls[1];
    expect([campaign, mode]).toEqual(['camp-1', 'apply']);
    expect(key).toEqual(expect.any(String));
    expect(key).not.toBe('');
    expect(text(el.querySelector('h1')!)).toBe('Pacote importado');
    expect(text(el)).toContain('Pronto. A mesa já usa o conteúdo novo.');
  });

  it('blocks the import while an entry is refused, with every reason and how to fix the file', async () => {
    const { fixture, el } = await setup();
    importPack.mockResolvedValueOnce(
      answer({
        entries: [
          ...entries,
          {
            key: 'feat:d@mesa',
            kind: TableContentKind.FEAT,
            namePt: 'Quebrado',
            status: TableImportStatus.REFUSED,
            violations: [
              { field: 'table_feat.prerequisite.level', reason: 'bad_value' },
              { field: 'table_feat.name_pt', reason: 'duplicate_name' },
            ],
          },
        ],
        totals: { new: 1, updated: 1, unchanged: 1, refused: 1 },
      }),
    );
    await choose(fixture, el, PACK);
    expect(button(el, 'Importar 2 entradas').disabled).toBe(true);
    const page = text(el);
    expect(page).toContain('Recusadas · 1');
    expect(page).toContain('O nível vai de 1 a 20');
    expect(page).toContain('Já existe um talento da mesa com este nome.');
    expect(page).toContain('Uma entrada foi recusada, então nada pode ser importado.');
    expect(page).toContain('Edite o arquivo');
    expect(importPack).toHaveBeenCalledTimes(1);
  });

  it('shows a refused pack (the file itself) as a list of reasons, and goes back to choosing', async () => {
    const { fixture, el } = await setup();
    importPack.mockRejectedValueOnce(
      new ConnectError('bad pack', Code.InvalidArgument, undefined, [
        {
          desc: TableContentRefusalSchema,
          value: create(TableContentRefusalSchema, {
            violations: [
              create(TableContentViolationSchema, {
                field: 'pack.entries[2].key',
                reason: 'duplicate_key',
              }),
            ],
          }),
        },
      ]),
    );
    await choose(fixture, el, PACK);
    expect(text(el.querySelector('[role="alert"]')!)).toContain('mesma chave');
    expect(button(el, 'Escolher arquivo')).toBeDefined();
  });

  it('lists the characters left with issues after the import, like an edit does', async () => {
    const { fixture, el } = await setup();
    const totals = { new: 0, updated: 1, unchanged: 0, refused: 0 };
    importPack.mockResolvedValueOnce(answer({ entries: [entries[1]], totals }));
    await choose(fixture, el, PACK);
    importPack.mockResolvedValueOnce(
      answer(
        {
          entries: [entries[1]],
          totals,
          affectedCharacters: [
            { characterId: 'ch-1', name: 'Pensantus', playerDisplayName: 'Ana', issues: 2 },
          ],
        },
        TableImportMode.APPLY,
      ),
    );
    button(el, 'Importar 1 entrada').click();
    await settle(fixture, 4);
    const warn = text(el.querySelector('.mr-notice--warning')!);
    expect(warn).toContain('1 ficha ficou com aviso.');
    expect(warn).toContain('Pensantus');
    expect(warn).toContain('Ana');
    expect(warn).toContain('2 avisos');
    expect(el.querySelector('a[href$="/characters/ch-1"]')).not.toBeNull();
  });

  it('says so when the pack changes nothing', async () => {
    const { fixture, el } = await setup();
    importPack.mockResolvedValueOnce(
      answer({ entries: [entries[2]], totals: { new: 0, updated: 0, unchanged: 1, refused: 0 } }),
    );
    await choose(fixture, el, PACK);
    expect(text(el)).toContain('O pacote não traz nada de novo');
    expect(button(el, 'Importar 0 entradas').disabled).toBe(true);
  });
});
