import { describe, expect, it } from 'vitest';

import { article, theName } from './article';

describe('article', () => {
  it('reads the word the article agrees with', () => {
    expect(article('Cavalo de guerra')).toBe('o');
    expect(article('Aranha-lobo gigante')).toBe('a');
    expect(article('Rede')).toBe('a');
  });
});

describe('theName', () => {
  it('puts the name after its article, bare or fused with a preposition', () => {
    expect(theName('Cavalo de guerra')).toBe('o Cavalo de guerra');
    expect(theName('Aranha')).toBe('a Aranha');
    expect(theName('Cavalo de guerra', 'de')).toBe('do Cavalo de guerra');
    expect(theName('Aranha', 'de')).toBe('da Aranha');
    expect(theName('Cavalo de guerra', 'em')).toBe('no Cavalo de guerra');
    expect(theName('Aranha', 'em')).toBe('na Aranha');
    expect(theName('Mastim', 'a')).toBe('ao Mastim');
    expect(theName('Aranha', 'a')).toBe('à Aranha');
    expect(theName('Mastim', 'por')).toBe('pelo Mastim');
    expect(theName('Aranha', 'por')).toBe('pela Aranha');
  });
});
