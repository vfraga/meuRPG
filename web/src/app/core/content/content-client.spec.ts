import { TestBed } from '@angular/core/testing';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError, createRouterTransport } from '@connectrpc/connect';

import {
  ExportTableContentResponseSchema,
  ImportTableContentResponseSchema,
  TableContentService,
  TableImportMode,
  TableContentBlockedReason,
  TableContentBlockedSchema,
  TableContentRefusalSchema,
  TableContentViolationSchema,
} from '../../../gen/meurpg/rules/v1/table_content_pb';
import {
  RequestRefusedReason,
  RequestRefusedSchema,
} from '../../../gen/meurpg/system/v1/request_limits_pb';
import { OUTCOME_UNKNOWN, SESSION_ENDED } from '../connect/connect-errors';
import { CONNECT_TRANSPORT } from '../connect/transport';
import {
  TableContentClient,
  blockedReason,
  contentErrorText,
  isStale,
  refusalOf,
} from './content-client';

function blocked(code: Code, reason: TableContentBlockedReason) {
  return new ConnectError('x', code, undefined, [
    {
      desc: TableContentBlockedSchema,
      value: create(TableContentBlockedSchema, { reason, key: 'race:corujeiro@mesa' }),
    },
  ]);
}

describe('a table content call refused for the size of a field', () => {
  const refused = (reason: RequestRefusedReason) =>
    new ConnectError('request over its size limit', Code.InvalidArgument, undefined, [
      {
        desc: RequestRefusedSchema,
        value: create(RequestRefusedSchema, { reason, field: 'pack' }),
      },
    ]);

  it('says so instead of "confira os dados"', () => {
    expect(contentErrorText(refused(RequestRefusedReason.LIST_TOO_LONG), 'importar')).toBe(
      'O pedido tem itens demais. Reduza a lista e tente de novo.',
    );
    expect(contentErrorText(refused(RequestRefusedReason.TEXT_TOO_LONG), 'importar')).toBe(
      'O texto é grande demais. Encurte e tente de novo.',
    );
  });

  it('is not a refusal of the table rules', () => {
    expect(refusalOf(refused(RequestRefusedReason.LIST_TOO_LONG))).toBeNull();
  });
});

describe('the errors of the table content calls', () => {
  it('reads a refusal by its typed detail', () => {
    const err = new ConnectError('refused', Code.InvalidArgument, undefined, [
      {
        desc: TableContentRefusalSchema,
        value: create(TableContentRefusalSchema, {
          violations: [
            create(TableContentViolationSchema, {
              field: 'table_spell.name_pt',
              reason: 'duplicate_name',
            }),
          ],
        }),
      },
    ]);
    expect(refusalOf(err)?.map((v) => v.field)).toEqual(['table_spell.name_pt']);
    expect(refusalOf(new ConnectError('x', Code.NotFound))).toBeNull();
    expect(refusalOf(new Error('offline'))).toBeNull();
  });

  it('says "Esta entrada mudou enquanto você editava" for a stale write, by the detail and not by the message', () => {
    const err = blocked(Code.Aborted, TableContentBlockedReason.STALE);
    expect(isStale(err)).toBe(true);
    expect(blockedReason(err)).toBe(TableContentBlockedReason.STALE);
    expect(contentErrorText(err, 'salvar a magia')).toContain(
      'Esta entrada mudou enquanto você editava',
    );
    expect(isStale(new ConnectError('stale entry', Code.Aborted))).toBe(false);
  });

  it('says what archiving twice and unarchiving what is not archived mean', () => {
    expect(
      contentErrorText(
        blocked(Code.FailedPrecondition, TableContentBlockedReason.ARCHIVED),
        'arquivar',
      ),
    ).toBe('Esta entrada já está arquivada.');
    expect(
      contentErrorText(
        blocked(Code.FailedPrecondition, TableContentBlockedReason.NOT_ARCHIVED),
        'desarquivar',
      ),
    ).toBe('Esta entrada não está arquivada.');
  });

  it('says the rest in words: no permission, gone, offline', () => {
    expect(contentErrorText(new ConnectError('x', Code.PermissionDenied), 'salvar')).toContain(
      'Só o mestre',
    );
    expect(contentErrorText(new ConnectError('x', Code.NotFound), 'salvar')).toContain(
      'não existe mais',
    );
    expect(contentErrorText(new Error('network'), 'salvar a raça')).toBe(
      'Não foi possível salvar a raça. Confira a conexão e tente de novo.',
    );
  });

  it('says the reasons of a refused archive, and a plain refusal when the server names none', () => {
    const refused = new ConnectError('refused', Code.InvalidArgument, undefined, [
      {
        desc: TableContentRefusalSchema,
        value: create(TableContentRefusalSchema, {
          violations: [
            create(TableContentViolationSchema, {
              field: 'table_spell.name_pt',
              reason: 'duplicate_name',
            }),
          ],
        }),
      },
    ]);
    expect(contentErrorText(refused, 'desarquivar')).toBe(
      'Não foi possível desarquivar. Já existe uma entrada da mesa com este nome. Escolha outro.',
    );
    expect(contentErrorText(new ConnectError('x', Code.InvalidArgument), 'salvar')).toBe(
      'Não foi possível salvar: confira os dados e tente de novo.',
    );
  });

  it('says to check an ambiguous outcome and to wait out a rate limit, never "confira a conexão"', () => {
    expect(contentErrorText(new ConnectError('x', Code.Unknown), 'salvar')).toBe(OUTCOME_UNKNOWN);
    const limited = new ConnectError(
      'slow down',
      Code.ResourceExhausted,
      new Headers({ 'Retry-After': '3' }),
    );
    expect(contentErrorText(limited, 'salvar')).toBe(
      'Muitas ações em pouco tempo. Espere 3 segundos e tente de novo.',
    );
    expect(contentErrorText(new ConnectError('x', Code.Unauthenticated), 'salvar')).toBe(
      SESSION_ENDED,
    );
  });
});

describe('the content pack calls (MR-025)', () => {
  it("sends the file's own bytes, the idempotency key with an APPLY only, and exports the campaign's pack", async () => {
    const seen: { mode: TableImportMode; key: string; json: string }[] = [];
    const transport = createRouterTransport(({ service }) => {
      service(TableContentService, {
        importTableContent: (req) => {
          seen.push({
            mode: req.mode,
            key: req.idempotencyKey,
            json: new TextDecoder().decode(req.packJson),
          });
          return create(ImportTableContentResponseSchema);
        },
        exportTableContent: (req) =>
          create(ExportTableContentResponseSchema, {
            pack: { format: 'meurpg.table-content', version: 1, name: req.campaignId },
          }),
      });
    });
    TestBed.configureTestingModule({
      providers: [{ provide: CONNECT_TRANSPORT, useValue: transport }],
    });
    const client = TestBed.inject(TableContentClient);
    const pack = await client.exportPack('camp-1');
    const bytes = new TextEncoder().encode('{}');
    expect(pack.name).toBe('camp-1');
    await client.importPack('camp-1', bytes, 'preview', 'key-1');
    await client.importPack('camp-1', bytes, 'apply', 'key-1');
    expect(seen).toEqual([
      { mode: TableImportMode.PREVIEW, key: '', json: '{}' },
      { mode: TableImportMode.APPLY, key: 'key-1', json: '{}' },
    ]);
  });
});
