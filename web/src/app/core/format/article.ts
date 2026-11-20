/** "o" or "a" before a name: by its first word's ending, which is right for
 * the names this table uses (Brisa, Toren, Rapieira, Machado, Raio). */
export function article(name: string): 'o' | 'a' {
  // "Aranha-lobo gigante" is read by "Aranha", the word the article agrees with.
  const first = name.trim().split(/\s+/)[0].split('-')[0].toLowerCase();
  return /a$/.test(first) || /^(foice|rede|clava|mace|nuvem|glaive)$/.test(first) ? 'a' : 'o';
}

const FORMS = {
  '': ['o', 'a'],
  de: ['do', 'da'],
  em: ['no', 'na'],
  a: ['ao', 'à'],
  por: ['pelo', 'pela'],
} as const;

/** The name after its article, for a creature or an item: "o Cavalo de guerra", "da Aranha" (`'de'`), "no" (`'em'`), "ao"/"à"
 * (`'a'`), "pelo"/"pela" (`'por'`). A character is named with no article: never pass one here. */
export function withArticle(name: string, preposition: keyof typeof FORMS = ''): string {
  return `${FORMS[preposition][article(name) === 'a' ? 1 : 0]} ${name}`;
}
