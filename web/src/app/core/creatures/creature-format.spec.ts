import { isTableCreature, tableCreatureNote } from './creature-format';

describe('the table creature words', () => {
  it('knows a table creature by the suffix of its key', () => {
    expect(isTableCreature('creature:lobo-do-vale@mesa')).toBe(true);
    expect(isTableCreature('monster:wolf')).toBe(false);
    expect(isTableCreature('')).toBe(false);
  });

  it('names the owner whose sheet gives the numbers, or says "do dono"', () => {
    expect(tableCreatureNote('Tor')).toBe('Os números vêm da ficha de Tor quando ele chegou');
    expect(tableCreatureNote(' Tor ')).toBe('Os números vêm da ficha de Tor quando ele chegou');
    expect(tableCreatureNote('')).toBe('Os números vêm da ficha do dono quando ele chegou');
    expect(tableCreatureNote('   ')).toBe('Os números vêm da ficha do dono quando ele chegou');
  });
});
