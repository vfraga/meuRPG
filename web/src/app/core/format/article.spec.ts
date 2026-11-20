import { describe, expect, it } from 'vitest';

import { article, withArticle } from './article';

describe('article', () => {
  it('reads the word the article agrees with', () => {
    expect(article('Cavalo de guerra')).toBe('o');
    expect(article('Aranha-lobo gigante')).toBe('a');
    expect(article('Rede')).toBe('a');
  });
});

describe('withArticle', () => {
  it('puts the name after its article, bare or fused with a preposition', () => {
    expect(withArticle('Cavalo de guerra')).toBe('o Cavalo de guerra');
    expect(withArticle('Aranha')).toBe('a Aranha');
    expect(withArticle('Cavalo de guerra', 'de')).toBe('do Cavalo de guerra');
    expect(withArticle('Aranha', 'de')).toBe('da Aranha');
    expect(withArticle('Cavalo de guerra', 'em')).toBe('no Cavalo de guerra');
    expect(withArticle('Aranha', 'em')).toBe('na Aranha');
    expect(withArticle('Mastim', 'a')).toBe('ao Mastim');
    expect(withArticle('Aranha', 'a')).toBe('à Aranha');
    expect(withArticle('Mastim', 'por')).toBe('pelo Mastim');
    expect(withArticle('Aranha', 'por')).toBe('pela Aranha');
  });
});
