import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import { DungeonOptionRefusedSchema } from '../../../gen/meurpg/maps/v1/dungeons_pb';
import { MapBlockedReason, MapBlockedSchema } from '../../../gen/meurpg/maps/v1/maps_pb';
import {
  NO_ROOM_TEXT,
  createFailure,
  optionField,
  placeSceneFailure,
  previewFailure,
  redrawFailure,
  refusedOption,
} from './dungeon-errors';

const refused = (option: string) =>
  new ConnectError('refused', Code.InvalidArgument, undefined, [
    { desc: DungeonOptionRefusedSchema, value: create(DungeonOptionRefusedSchema, { option }) },
  ]);
const blocked = (reason: MapBlockedReason) =>
  new ConnectError('blocked', Code.FailedPrecondition, undefined, [
    { desc: MapBlockedSchema, value: create(MapBlockedSchema, { reason }) },
  ]);

describe('the dungeon errors', () => {
  it('reads the refused option by its typed detail, never by the message', () => {
    expect(refusedOption(refused('width'))).toBe('width');
    expect(refusedOption(refused(''))).toBe('');
    expect(
      refusedOption(new ConnectError('width is out of range', Code.InvalidArgument)),
    ).toBeNull();
    expect(refusedOption(new ConnectError('x', Code.NotFound))).toBeNull();
  });

  it('puts the width and the height on the one "Tamanho" field', () => {
    expect(optionField('width')).toBe('size');
    expect(optionField('height')).toBe('size');
    expect(optionField('room_side_min')).toBe('room_side_min');
    expect(optionField('aspect_limit')).toBeNull();
  });

  it('says a refused preview on the field it names, or as a whole when none is at fault', () => {
    expect(previewFailure(refused('width'))).toEqual({
      kind: 'refused',
      field: 'size',
      text: 'O tamanho vai de 21 a 121 quadrados.',
    });
    expect(previewFailure(refused('room_side_max'))).toMatchObject({
      kind: 'refused',
      field: 'room_side_max',
    });
    expect(previewFailure(refused(''))).toEqual({
      kind: 'refused',
      field: null,
      text: NO_ROOM_TEXT,
    });
  });

  it('tells a busy preview from a slow one and from a failure', () => {
    expect(previewFailure(new ConnectError('busy', Code.ResourceExhausted))).toEqual({
      kind: 'busy',
    });
    expect(previewFailure(new ConnectError('slow', Code.DeadlineExceeded))).toMatchObject({
      kind: 'failed',
      text: expect.stringContaining('demorou demais'),
    });
    expect(previewFailure(new ConnectError('down', Code.Unavailable))).toEqual({
      kind: 'failed',
      text: 'Não deu para gerar a prévia.',
    });
  });

  it('says a rate-limited preview in the words of the limit, and does not retry it as a busy one', () => {
    const limited = (code: Code) =>
      new ConnectError('slow down', code, new Headers({ 'Retry-After': '3' }));
    const waiting = 'Muitas ações em pouco tempo. Espere 3 segundos e tente de novo.';
    expect(previewFailure(limited(Code.ResourceExhausted))).toEqual({
      kind: 'failed',
      text: waiting,
    });
    expect(previewFailure(limited(Code.Unavailable))).toEqual({ kind: 'failed', text: waiting });
    expect(previewFailure(new ConnectError('gone', Code.Unauthenticated))).toMatchObject({
      kind: 'failed',
      text: expect.stringContaining('Entre de novo'),
    });
  });

  it('says a failed creation by its field or its code, with the rate limit in words', () => {
    expect(createFailure(refused('stairs'))).toMatchObject({ field: 'stairs' });
    expect(createFailure(new ConnectError('x', Code.ResourceExhausted)).text).toContain(
      'Espere um pouco',
    );
    expect(createFailure(new ConnectError('x', Code.InvalidArgument)).text).toContain('o nome');
    expect(createFailure(new ConnectError('x', Code.DeadlineExceeded)).field).toBeNull();
  });

  it('says a refused "Redesenhar" by its reason', () => {
    expect(redrawFailure(blocked(MapBlockedReason.IMAGE_CHANGED))).toContain('já não é a masmorra');
    expect(redrawFailure(blocked(MapBlockedReason.NO_GRID))).toContain('sem grade');
    expect(redrawFailure(new ConnectError('x', Code.Aborted))).toContain(
      'mudaram enquanto a imagem era desenhada',
    );
    expect(redrawFailure(new ConnectError('x', Code.ResourceExhausted))).toContain('15 segundos');
    expect(redrawFailure(new ConnectError('x', Code.NotFound))).toContain(
      'não é mais uma masmorra',
    );
  });

  it('says a failed scene', () => {
    expect(placeSceneFailure(new ConnectError('x', Code.ResourceExhausted))).toContain(
      '200 pontos',
    );
    expect(placeSceneFailure(new ConnectError('x', Code.NotFound))).toContain('não existe mais');
  });
});
