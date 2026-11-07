import {
  TableScreenKind as K,
  TableScreenNotice as N,
  TableScreenUnavailable as U,
} from '../../../gen/meurpg/play/v1/table_screen_pb';
import {
  type TableScreenLabelState,
  barButtonLabel,
  clockText,
  kindName,
  menuLine,
  noticeText,
  unavailableText,
} from './table-screen-labels';

const LOST = new Date(2026, 0, 5, 21, 14);
const base: TableScreenLabelState = {
  open: false,
  lostAt: null,
  mirror: true,
  pin: K.UNSPECIFIED,
  shown: K.IDLE,
};

describe('the session bar button and the menu line', () => {
  it('says "Tela da mesa" with nothing open', () => {
    expect(barButtonLabel(base)).toEqual({ text: 'Tela da mesa', pinned: false });
    expect(menuLine(base)).toBe('Nenhuma tela aberta');
  });

  it('names the kind the server resolved while mirroring', () => {
    const open = { ...base, open: true, shown: K.SCENE };
    expect(barButtonLabel(open)).toEqual({ text: 'Na TV: Cena', pinned: false });
    expect(menuLine({ ...open, shown: K.MAP })).toBe('Na TV: Mapa · espelhando');
  });

  it('names the pin, with the marker, while pinned', () => {
    const pinned = { ...base, open: true, mirror: false, pin: K.COMBAT, shown: K.IDLE };
    expect(barButtonLabel(pinned)).toEqual({ text: 'Na TV: Combate', pinned: true });
    expect(menuLine(pinned)).toBe('Na TV: Combate · fixo');
  });

  it('says the connection is lost, with the hour', () => {
    const lost = { ...base, lostAt: LOST };
    expect(barButtonLabel(lost)).toEqual({ text: 'TV sem conexão', pinned: false });
    expect(menuLine(lost)).toBe('Sem conexão desde 21:14');
  });

  it('is open again once a tab is back, whatever lostAt says', () => {
    expect(barButtonLabel({ ...base, open: true, lostAt: LOST, shown: K.IMAGE }).text).toBe(
      'Na TV: Imagem',
    );
  });

  it('names every kind, the waiting screen for an unknown one', () => {
    expect([K.IDLE, K.MAP, K.IMAGE, K.SCENE, K.COMBAT, K.UNSPECIFIED].map(kindName)).toEqual([
      'Espera',
      'Mapa',
      'Imagem',
      'Cena',
      'Combate',
      'Espera',
    ]);
  });

  it('writes the hour with two digits', () => {
    expect(clockText(new Date(2026, 0, 5, 9, 5))).toBe('09:05');
  });
});

describe('the texts of the panel', () => {
  it('gives each unavailable reason its line', () => {
    expect(unavailableText(U.NO_COMBAT)).toBe('Nenhum combate agora.');
    expect(unavailableText(U.NEEDS_GROUP_VISION)).toBe(
      'Precisa da Visão do grupo (editor do mapa).',
    );
    expect(unavailableText(U.UNSPECIFIED)).toBe('');
  });

  it('says why the stage is not the choice', () => {
    expect(noticeText(N.PIN_ENDED, K.COMBAT)).toBe(
      'O combate acabou. A TV foi para a tela de espera.',
    );
    expect(noticeText(N.PIN_ENDED, K.IDLE)).toContain('O que estava fixo acabou.');
    expect(noticeText(N.SCENE_NOT_FOR_ALL, K.UNSPECIFIED)).toContain('só para alguns');
    expect(noticeText(N.MAP_NEEDS_GROUP_VISION, K.UNSPECIFIED)).toContain('Visão do grupo');
    expect(noticeText(N.UNSPECIFIED, K.UNSPECIFIED)).toBe('');
  });
});
