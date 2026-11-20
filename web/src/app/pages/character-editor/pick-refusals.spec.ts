import { pickSentenceOf, sentencesOf } from './pick-refusals';

const SENT = {
  skillProficiencies: ['skill:arcana', 'skill:stealth'],
  expertiseSkillKeys: ['skill:stealth'],
  featureChoiceKeys: ['fighting-style:archery', 'f1#tool=tool:herbalism-kit'],
};

describe('pickSentenceOf', () => {
  it('names the key that stood at the place the server points at, by kind of pick', () => {
    expect(pickSentenceOf({ kind: 'skill', index: 1, sentence: 'a' }, SENT)).toEqual({
      kind: 'skill',
      key: 'skill:stealth',
      sentence: 'a',
    });
    expect(pickSentenceOf({ kind: 'expertise', index: 0, sentence: 'b' }, SENT)?.key).toBe(
      'skill:stealth',
    );
    expect(pickSentenceOf({ kind: 'tool', index: 1, sentence: 'c' }, SENT)?.key).toBe(
      'f1#tool=tool:herbalism-kit',
    );
  });

  it('is null when no key stands there', () => {
    expect(pickSentenceOf({ kind: 'skill', index: 5, sentence: 'a' }, SENT)).toBeNull();
  });
});

describe('sentencesOf', () => {
  const pick = { kind: 'skill', key: 'skill:stealth', sentence: 'a' } as const;

  it('gives the sentence of a held pick of that kind', () => {
    expect(sentencesOf(pick, 'skill', () => true)).toEqual(new Map([['skill:stealth', 'a']]));
  });

  it('gives none for another kind, a pick the draft no longer holds, or no refusal', () => {
    expect(sentencesOf(pick, 'tool', () => true).size).toBe(0);
    expect(sentencesOf(pick, 'skill', () => false).size).toBe(0);
    expect(sentencesOf(null, 'skill', () => true).size).toBe(0);
  });
});
