import { create } from '@bufbuild/protobuf';

import { PreviewSpellAreaResponseSchema } from '../../../gen/meurpg/play/v1/combat_pb';
import { newWall } from './spell-wall';
import { wallSummary } from './wall-summary';

const plain = (t: string) => t.replace(/\u00a0/g, ' ');
const squares = (n: number) => Array.from({ length: n }, (_, i) => ({ col: i, row: 0 }));
const preview = (over: Parameters<typeof create<typeof PreviewSpellAreaResponseSchema>>[1]) =>
  create(PreviewSpellAreaResponseSchema, over);

describe('wallSummary', () => {
  const ring = newWall(true);

  it('says nothing before the server has answered, and nothing for a straight wall', () => {
    expect(wallSummary(ring, null, 'Destreza')).toBeNull();
    expect(wallSummary(newWall(false), preview({ squares: squares(6) }), 'Destreza')).toBeNull();
    expect(wallSummary(null, preview({ squares: squares(6) }), 'Destreza')).toBeNull();
  });

  it('names the ring by its diameter and the squares the server laid', () => {
    const s = wallSummary(ring, preview({ lengthFt: 20, squares: squares(16) }), 'Destreza')!;
    expect(plain(s.title)).toBe('Anel de 6 m · 16 quadrados');
  });

  it('follows the diameter the server laid, and says "1 quadrado" in the singular', () => {
    const s = wallSummary(ring, preview({ lengthFt: 10, squares: squares(1) }), 'Destreza')!;
    expect(plain(s.title)).toBe('Anel de 3 m · 1 quadrado');
  });

  it('lists who is in the wall now, as the server sent them, with the save they make', () => {
    const s = wallSummary(
      ring,
      preview({
        lengthFt: 20,
        squares: squares(16),
        targets: [
          { combatantId: 'k2', label: 'Kobold 2' },
          { combatantId: 'p', label: 'Pensantus', self: true },
        ],
      }),
      'Destreza',
    )!;
    expect(plain(s.who)).toBe(
      'Na muralha agora: Kobold 2 (teste de Destreza) e Pensantus (você, teste de Destreza).',
    );
  });

  it('says nobody is seen in the wall when the server names no one, never that nobody is there', () => {
    const s = wallSummary(ring, preview({ lengthFt: 20, squares: squares(16) }), 'Destreza')!;
    expect(s.who).toBe('Ninguém que você vê está na muralha agora.');
  });

  it('leaves the save out when the spell has none to name', () => {
    const s = wallSummary(
      ring,
      preview({
        lengthFt: 20,
        squares: squares(16),
        targets: [{ combatantId: 'k', label: 'Kobold 1' }],
      }),
      '',
    )!;
    expect(s.who).toBe('Na muralha agora: Kobold 1.');
  });
});
