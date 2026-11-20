import { describe, expect, it } from 'vitest';

import {
  RELEASE_BACK,
  RELEASE_DONE,
  masterFoot,
  moveBand,
  moveNote,
  moveOffText,
  releaseBand,
  releaseGoneView,
  releaseSubtitle,
  releaseTitle,
  turnOfText,
} from './release-text';

/** A value as plain text: the non-breaking spaces the words keep together are written as plain ones. */
const flat = <T>(v: T): T => JSON.parse(JSON.stringify(v).replace(/\u00a0/g, ' ')) as T;

describe('the words of a readied spell or move released on the map (857-02 to 857-07)', () => {
  it('titles the spell by its name, and by the player it is released for when the master does it', () => {
    expect(releaseTitle('Bola de Fogo', false, 'Pensantus')).toBe('Soltar Bola de Fogo');
    expect(releaseTitle('Bola de Fogo', true, 'Pensantus')).toBe(
      'Soltar Bola de Fogo por Pensantus',
    );
    expect(releaseTitle('', false, 'Pensantus')).toBe('Soltar a magia preparada');
  });

  it('subtitles it with the reaction and who readied it, and says the master answers for the player', () => {
    expect(releaseSubtitle('Pensantus', false)).toBe('Reação · a ação preparada de Pensantus');
    expect(releaseSubtitle('Pensantus', true)).toBe(
      'Reação · a ação preparada de Pensantus · você responde por Pensantus',
    );
  });

  it('reads the band of a held spell: the trigger, and the slot it was held with', () => {
    expect(flat(releaseBand('Quando o ogro passar pela porta', 3))).toEqual({
      lead: 'Preparada:',
      quote: '“Quando o ogro passar pela porta”',
      detail: 'Espaço de 3º nível · gasto ao preparar',
    });
    expect(flat(releaseBand('Se ele correr', 0)).detail).toBe('Truque · sem espaço');
  });

  it("reads the Mover page's band for the player and for the master (857-06, 857-07)", () => {
    expect(
      flat(
        moveBand({
          trigger: 'Quando o ogro passar pela porta',
          reactor: 'Tor',
          turnOf: 'vez do Ogro',
          master: false,
        }),
      ),
    ).toEqual({
      lead: 'Ação preparada:',
      quote: '“Quando o ogro passar pela porta”',
      detail: 'Fora da sua vez · vez do Ogro · gasta a ■ reação',
    });
    expect(
      flat(
        moveBand({
          trigger: 'Quando o ogro passar pela porta',
          reactor: 'Tor',
          turnOf: 'vez do Ogro',
          master: true,
        }),
      ),
    ).toEqual({
      lead: 'Ação preparada de Tor:',
      quote: '“Quando o ogro passar pela porta”',
      detail: 'Fora da vez de Tor · vez do Ogro · gasta a ■ reação de Tor · você responde por Tor',
    });
  });

  it("says whose turn it is: an NPC takes its article, a player's character does not", () => {
    expect(turnOfText('Ogro', false)).toBe('vez do Ogro');
    expect(turnOfText('Brisa', true)).toBe('vez de Brisa');
    expect(turnOfText('', false)).toBe('');
  });

  it('says what a readied move costs, once (857-06) and for the master (857-07)', () => {
    expect(flat(moveNote('Tor', false))).toBe(
      'É o movimento preparado: gasta a ■ Reação e não tira nada do deslocamento da sua vez. Vale um movimento só.',
    );
    expect(flat(moveNote('Tor', true))).toBe(
      'É o movimento preparado de Tor: gasta a ■ Reação de Tor e não tira nada do deslocamento da vez de Tor. Vale um movimento só.',
    );
  });

  it('tells the master the player can still answer on their own device, and that the first answer wins', () => {
    expect(masterFoot('Pensantus')).toBe(
      'Pensantus ainda pode responder no próprio aparelho: vale a primeira resposta.',
    );
  });

  it('has the one way back and the one way out of the result', () => {
    expect(RELEASE_BACK).toBe('Voltar');
    expect(RELEASE_DONE).toBe('Voltar à sessão');
  });

  it('says why a move cannot be made now: grappled, or no speed (857-09 card 6)', () => {
    expect(moveOffText(true)).toBe(
      'Condição Agarrado: o seu deslocamento é 0. Dá para deixar passar.',
    );
    expect(moveOffText(false)).toBe('O seu deslocamento é 0. Dá para deixar passar.');
  });
});

describe('a release whose window went away (857-09 card 1)', () => {
  const base = {
    spell: 'Bola de Fogo',
    move: false,
    stillWaiting: false,
    reactionUsed: false,
    down: false,
    timedOut: false,
    afterMap: true,
  };

  it('says the master undid the trigger when the readied row still waits', () => {
    expect(releaseGoneView({ ...base, stillWaiting: true })).toEqual({
      lead: 'O mestre desfez o gatilho:',
      rest: 'a Bola de Fogo continua preparada.',
    });
    expect(releaseGoneView({ ...base, stillWaiting: true, move: true })).toEqual({
      lead: 'O mestre desfez o gatilho:',
      rest: 'o movimento continua preparado.',
    });
  });

  it('says the spell and its slot are lost when the row no longer waits', () => {
    expect(releaseGoneView(base)).toEqual({
      lead: 'A janela fechou.',
      rest: 'A Bola de Fogo não saiu; a magia preparada e o espaço dela se perderam.',
    });
  });

  it('says the move was not made, and the reaction is free', () => {
    expect(releaseGoneView({ ...base, move: true })).toEqual({
      lead: 'A janela fechou.',
      rest: 'Você não se moveu, e a reação continua livre.',
    });
  });

  it('says the reaction was used, the character fell, or that it passed by itself in 2 minutes', () => {
    expect(releaseGoneView({ ...base, reactionUsed: true })).toEqual({
      lead: 'A janela fechou e a sua reação foi usada.',
      rest: '',
    });
    expect(releaseGoneView({ ...base, down: true })).toEqual({
      lead: 'Condição Caído.',
      rest: 'A magia preparada se perde.',
    });
    expect(releaseGoneView({ ...base, timedOut: true })).toEqual({
      lead: 'Passou sozinho:',
      rest: 'você não respondeu em 2 min.',
    });
  });
});
