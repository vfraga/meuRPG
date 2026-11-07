import { toTvScreen } from '../../core/play/table-screen-view';
import { screenResponse } from '../../core/play/table-screen-testing';
import { TableScreenKind } from '../../../gen/meurpg/play/v1/table_screen_pb';
import { tvAnnouncement, tvHeading } from './tv-heading';

const screen = (kind: TableScreenKind, body: object) =>
  toTvScreen(screenResponse({ stage: { kind, body } }));

describe('tvHeading', () => {
  it('names the session on the idle screen', () => {
    expect(tvHeading(toTvScreen(screenResponse()))).toEqual({
      icon: 'tv',
      kicker: 'Mirathel',
      title: 'Sessão 3',
    });
  });

  it('names a map, an image and a scene', () => {
    expect(
      tvHeading(screen(TableScreenKind.MAP, { case: 'map', value: { name: 'Cripta do Corvo' } })),
    ).toMatchObject({
      icon: 'map',
      kicker: 'Mapa · Mirathel · Sessão 3',
      title: 'Cripta do Corvo',
    });
    expect(
      tvHeading(screen(TableScreenKind.IMAGE, { case: 'image', value: { name: 'A carta' } })).title,
    ).toBe('A carta');
    expect(
      tvHeading(screen(TableScreenKind.SCENE, { case: 'scene', value: { name: 'A taverna' } }))
        .kicker,
    ).toBe('Cena · Mirathel · Sessão 3');
  });

  it('falls back to a plain word when the server sends no name', () => {
    expect(
      tvHeading(screen(TableScreenKind.MAP, { case: 'map', value: { hiddenByFog: true } })).title,
    ).toBe('Mapa');
    expect(tvHeading(screen(TableScreenKind.IMAGE, { case: 'image', value: {} })).title).toBe(
      'Imagem',
    );
    expect(tvHeading(screen(TableScreenKind.SCENE, { case: 'scene', value: {} })).title).toBe(
      'Cena',
    );
  });

  it('says a combat has no map, or names it', () => {
    const combat = (map?: object) =>
      screen(TableScreenKind.COMBAT, { case: 'combat', value: { round: 1, map } });
    expect(tvHeading(combat()).title).toBe('Combate sem mapa');
    expect(tvHeading(combat({ name: 'Cripta do Corvo' })).title).toBe('Combate · Cripta do Corvo');
    expect(tvHeading(combat({ hiddenByFog: true })).title).toBe('Combate');
  });

  it('announces the title', () => {
    expect(tvAnnouncement(toTvScreen(screenResponse()))).toBe('Na tela: Sessão 3');
  });
});
