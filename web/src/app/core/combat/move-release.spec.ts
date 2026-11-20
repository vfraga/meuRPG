import { describe, expect, it } from 'vitest';

import { type MoveRelease, moveReleaseView } from './move-release';

const tor: MoveRelease = {
  windowId: 'w1',
  readiedId: 'r1',
  reactorId: 'tor',
  reactor: 'Tor',
  trigger: 'Quando o ogro passar pela porta',
  master: false,
};

const flat = (t: string) => t.replace(/\u00a0/g, ' ');

describe("the Mover page's words for a readied move (#856, boards 857-06 and 857-07)", () => {
  it("says the player's band, the note and no footer", () => {
    const v = moveReleaseView(tor, 'vez do Ogro', null);
    expect(v.band.lead).toBe('Ação preparada:');
    expect(flat(v.band.detail)).toBe('Fora da sua vez · vez do Ogro · gasta a ■ reação');
    expect(v.note).toContain(
      'gasta a ■ Reação e não tira nada do deslocamento da sua vez. Vale um movimento só.',
    );
    expect(v.foot).toBe('');
    expect(v.master).toBe(false);
    expect(v.gone).toBeNull();
  });

  it("says the master's band with who he answers for, and the footer that the player may still answer", () => {
    const v = moveReleaseView({ ...tor, master: true }, 'vez do Ogro', null);
    expect(v.band.lead).toBe('Ação preparada de Tor:');
    expect(flat(v.band.detail)).toContain('você responde por Tor');
    expect(v.master).toBe(true);
    expect(v.foot).toBe('Tor ainda pode responder no próprio aparelho: vale a primeira resposta.');
  });

  it('carries what became of the window', () => {
    const gone = {
      lead: 'A janela fechou.',
      rest: 'Você não se moveu, e a reação continua livre.',
    };
    expect(moveReleaseView(tor, '', gone).gone).toBe(gone);
  });
});
