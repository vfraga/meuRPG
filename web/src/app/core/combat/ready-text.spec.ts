import { concentrationNotice, readyLegend, readyMoveText } from './ready-text';

const plain = (s: string) => s.replace(/ /g, ' ');

describe('the words of the Ready sheet (#582)', () => {
  it('says an attack costs the action now and the reaction only when the master fires it', () => {
    const lines = readyLegend('attack', 0);
    expect(lines.map((l) => `${l.lead} ${l.text}`.trim())).toEqual([
      'Ação: gasta agora, ao preparar.',
      'Reação: só se o mestre disparar e você usar.',
      'Se o gatilho não vier até a sua próxima vez, se perde.',
    ]);
  });

  it('says a spell spends the slot too, and is lost with the concentration', () => {
    const lines = readyLegend('spell', 2);
    expect(lines[0].lead).toBe('Ação e espaço de 2º círculo:');
    expect(lines[1].text).toBe('só se o mestre disparar e você soltar a magia.');
    expect(lines[2].text).toContain('a magia e o espaço se perdem');
  });

  it('says a cantrip spends no slot', () => {
    const lines = readyLegend('spell', 0);
    expect(lines[0].lead).toBe('Ação:');
    expect(lines[2].text).not.toContain('espaço');
  });

  it('says how far a readied move goes, and that it provokes', () => {
    expect(plain(readyMoveText(25))).toBe(
      'Você anda até 7,5 m quando o mestre disparar. Provoca ataques de oportunidade, como qualquer movimento.',
    );
    expect(readyMoveText(0)).toContain('até o seu deslocamento');
  });

  it('warns that the concentration in course ends, and names the held spell', () => {
    expect(concentrationNotice('Névoa Obscurecente', 'Mísseis Mágicos')).toEqual({
      lead: 'A concentração em Névoa Obscurecente acaba agora.',
      rest: 'Você se concentra em Mísseis Mágicos até disparar.',
    });
    expect(concentrationNotice('', 'Mísseis Mágicos')).toBeNull();
  });
});
