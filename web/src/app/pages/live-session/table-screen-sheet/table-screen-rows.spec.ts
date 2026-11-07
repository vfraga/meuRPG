import { create } from '@bufbuild/protobuf';

import {
  GetTableScreenResponseSchema,
  TableScreenKind as K,
  TableScreenUnavailable as U,
} from '../../../../gen/meurpg/play/v1/table_screen_pb';
import { screenResponse } from '../../../core/play/table-screen-testing';
import { nowLine, pinRows } from './table-screen-rows';

const NONE = { map: '', image: '', scene: '' };

describe('pinRows', () => {
  it('lists the offered kinds in the board order, with their subtitles', () => {
    const rows = pinRows(
      [
        { kind: K.IDLE, available: true },
        { kind: K.MAP, available: true },
        { kind: K.SCENE, available: true },
      ] as never,
      { map: 'Cripta do Corvo', image: '', scene: 'A taverna do Javali Cego' },
    );
    expect(rows.map((r) => r.key)).toEqual(['map', 'scene', 'idle']);
    expect(rows[0].sub).toBe('Cripta do Corvo, com o que o grupo vê');
    expect(rows[1].sub).toBe('A taverna do Javali Cego');
    expect(rows[2].sub).toBe('Só o nome da campanha e da sessão');
  });

  it('says the reason under a row that cannot be picked', () => {
    const [row] = pinRows(
      [{ kind: K.COMBAT, available: false, unavailable: U.NO_COMBAT }] as never,
      NONE,
    );
    expect(row.available).toBe(false);
    expect(row.sub).toBe('Nenhum combate agora.');
  });

  it('still says something when the server gives no reason', () => {
    const [row] = pinRows([{ kind: K.IMAGE, available: false }] as never, NONE);
    expect(row.sub).toBe('Indisponível agora.');
  });

  it('says the map of the combat in its row, or only the order without one', () => {
    const options = [{ kind: K.COMBAT, available: true }] as never;
    expect(pinRows(options, { ...NONE, map: 'Cripta do Corvo' })[0].sub).toBe(
      'A ordem e o mapa da Cripta do Corvo',
    );
    expect(pinRows(options, NONE)[0].sub).toBe('A ordem do combate');
  });

  it('uses a plain subtitle where the session has no name yet', () => {
    const rows = pinRows(
      [
        { kind: K.MAP, available: true },
        { kind: K.IMAGE, available: true },
        { kind: K.SCENE, available: true },
      ] as never,
      NONE,
    );
    expect(rows.map((r) => r.sub)).toEqual([
      'O mapa atual, com o que o grupo vê',
      'A imagem à mostra',
      'A cena aberta',
    ]);
  });
});

describe('nowLine', () => {
  it('says the map of a pinned combat', () => {
    const data = screenResponse({
      stage: {
        kind: K.COMBAT,
        body: { case: 'combat', value: { round: 1, map: { name: 'Cripta do Corvo' } } },
      },
    });
    expect(nowLine(data, true)).toMatchObject({
      before: 'O combate, fixo: a ordem e o mapa da Cripta do Corvo',
      after: '.',
    });
  });

  it('names the scene on the stage, and marks a pin', () => {
    const data = screenResponse({
      stage: { kind: K.SCENE, body: { case: 'scene', value: { name: 'A taverna' } } },
    });
    expect(nowLine(data, false)).toMatchObject({
      before: 'A cena ',
      name: 'A taverna',
      after: '.',
    });
    expect(nowLine(data, true).after).toBe(', fixa.');
  });

  it('says the round of a combat and the waiting screen', () => {
    const combat = screenResponse({
      stage: { kind: K.COMBAT, body: { case: 'combat', value: { round: 3 } } },
    });
    expect(nowLine(combat, false).before).toBe('O combate: a ordem');
    expect(nowLine(combat, true).before).toBe('O combate, fixo: a ordem');
    expect(nowLine(screenResponse(), true)).toMatchObject({
      icon: 'tv',
      before: 'A tela de espera',
      after: ', fixa.',
    });
    expect(nowLine(create(GetTableScreenResponseSchema, {}), false).before).toBe(
      'A tela de espera',
    );
  });
});
