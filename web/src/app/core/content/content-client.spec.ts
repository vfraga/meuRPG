import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  TableContentBlockedReason,
  TableContentBlockedSchema,
  TableContentRefusalSchema,
  TableContentViolationSchema,
} from '../../../gen/meurpg/rules/v1/table_content_pb';
import { OUTCOME_UNKNOWN, SESSION_ENDED } from '../connect/connect-errors';
import { blockedReason, contentErrorText, isStale, refusalOf } from './content-client';

function blocked(code: Code, reason: TableContentBlockedReason) {
  return new ConnectError('x', code, undefined, [
    {
      desc: TableContentBlockedSchema,
      value: create(TableContentBlockedSchema, { reason, key: 'race:corujeiro@mesa' }),
    },
  ]);
}

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
