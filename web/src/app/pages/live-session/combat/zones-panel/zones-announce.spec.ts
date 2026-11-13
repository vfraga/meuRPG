import { zoneNotice } from './zones-announce';

const raio = { id: 'a', label: 'Raio Lunar · Bartolo', letter: 'A' };
const teia = { id: 'b', label: 'Teia · Pensantus', letter: 'B' };

describe('what the zones list says when it changes', () => {
  it('says nothing on the first read, nor when nothing changed', () => {
    expect(zoneNotice(null, [raio, teia])).toBe('');
    expect(zoneNotice([raio], [raio])).toBe('');
  });

  it('names the zone that appeared, with its letter', () => {
    expect(zoneNotice([raio], [raio, teia])).toBe('Zona criada: B, Teia · Pensantus.');
  });

  it('names the zone that ended, with its letter', () => {
    expect(zoneNotice([raio, teia], [{ ...teia, letter: 'A' }])).toBe(
      'Zona encerrada: A, Raio Lunar · Bartolo. Letras novas: Teia · Pensantus agora é a zona A.',
    );
  });

  it('says both when one ended as another began, and joins several', () => {
    expect(zoneNotice([raio], [{ ...teia, letter: 'A' }])).toBe(
      'Zona criada: A, Teia · Pensantus. Zona encerrada: A, Raio Lunar · Bartolo.',
    );
    expect(zoneNotice([], [raio, teia])).toBe(
      'Zonas criadas: A, Raio Lunar · Bartolo; B, Teia · Pensantus.',
    );
  });

  it('says which zones took another letter when one before them ended', () => {
    const cloud = { id: 'c', label: 'Névoa · Bartolo', letter: 'C' };
    expect(
      zoneNotice(
        [raio, teia, cloud],
        [
          { ...teia, letter: 'A' },
          { ...cloud, letter: 'B' },
        ],
      ),
    ).toBe(
      'Zona encerrada: A, Raio Lunar · Bartolo. Letras novas: Teia · Pensantus agora é a zona A; Névoa · Bartolo agora é a zona B.',
    );
  });
});
