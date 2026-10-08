import { catalogChanged, offControlOf, offersChanged } from './catalog-changes';
import type { RulesCatalogVm } from './character-editor.types';

const base = (): RulesCatalogVm => ({
  races: [
    {
      key: 'race:gnome',
      namePt: 'Gnomo',
      constitutionBonus: 0,
      choiceBonuses: [],
      fromTable: false,
      archived: false,
      off: false,
      subraces: [
        {
          key: 'subrace:rock',
          namePt: 'Gnomo da Rocha',
          constitutionBonus: 1,
          fromTable: false,
          archived: false,
          off: false,
        },
      ],
    },
  ],
  classes: [
    {
      key: 'class:wizard',
      namePt: 'Mago',
      hitDie: 6,
      isCaster: true,
      preparation: null,
      subclasses: [{ key: 'subclass:evocation', namePt: 'Evocação' }],
      subclassLevel: 2,
      spellcastingFirstLevel: 1,
    } as never,
  ],
  backgrounds: [{ key: 'background:acolyte', namePt: 'Acólito' } as never],
  skills: [],
  armor: [],
  weapons: [],
  spells: [
    {
      key: 'spell:light',
      namePt: 'Luz',
      level: 0,
      classKeys: ['class:wizard'],
      fromTable: false,
      archived: false,
      off: false,
    },
  ],
  challengeRatings: [],
  toolsAndLanguages: [],
  viewerIsMaster: false,
});

describe("catalogChanged: what the master's switches move in the pickers", () => {
  it("sees an option switched off for the players come and go in the master's own lists", () => {
    const off = { ...base(), races: [{ ...base().races[0], off: true }] };
    expect(catalogChanged(base(), off)).toBe(true);
  });

  it('is false for the same lists', () => {
    expect(catalogChanged(base(), base())).toBe(false);
  });

  it('sees a race, a subrace, a class, a subclass, a background or a spell come or go', () => {
    expect(catalogChanged(base(), { ...base(), races: [] })).toBe(true);
    expect(
      catalogChanged(base(), { ...base(), races: [{ ...base().races[0], subraces: [] }] }),
    ).toBe(true);
    expect(catalogChanged(base(), { ...base(), classes: [] })).toBe(true);
    expect(
      catalogChanged(base(), { ...base(), classes: [{ ...base().classes[0], subclasses: [] }] }),
    ).toBe(true);
    expect(catalogChanged(base(), { ...base(), backgrounds: [] })).toBe(true);
    expect(catalogChanged(base(), { ...base(), spells: [] })).toBe(true);
    // A spell that left a class's list too.
    expect(
      catalogChanged(base(), {
        ...base(),
        spells: [
          {
            key: 'spell:light',
            namePt: 'Luz',
            level: 0,
            classKeys: [],
            fromTable: false,
            archived: false,
            off: false,
          },
        ],
      }),
    ).toBe(true);
  });
});

describe('catalogChanged: what the editor computes from', () => {
  const withClass = (over: object): RulesCatalogVm => ({
    ...base(),
    classes: [{ ...base().classes[0], ...over }],
  });

  it('sees a class whose hit die or skill count changed', () => {
    expect(catalogChanged(base(), withClass({ hitDie: 10 }))).toBe(true);
    expect(catalogChanged(base(), withClass({ skillChoose: 3 }))).toBe(true);
  });

  it('sees a spell that moved between cantrip and leveled', () => {
    const moved = { ...base(), spells: [{ ...base().spells[0], level: 1 }] };
    expect(catalogChanged(base(), moved)).toBe(true);
  });

  it('is false for the same catalog read again', () => {
    expect(catalogChanged(base(), base())).toBe(false);
  });

  it('does not count those edits as a change of the offers the person picks from', () => {
    expect(offersChanged(base(), withClass({ hitDie: 10 }))).toBe(false);
    expect(offersChanged(base(), { ...base(), races: [] })).toBe(true);
  });
});

describe('offControlOf: which control holds the refused key', () => {
  const form = {
    race: 'race:gnome',
    subrace: 'subrace:rock',
    className: 'class:wizard',
    subclassName: 'subclass:evocation',
    background: 'background:acolyte',
  };

  it('names the control', () => {
    expect(offControlOf('race:gnome', form)).toBe('race');
    expect(offControlOf('subrace:rock', form)).toBe('subrace');
    expect(offControlOf('class:wizard', form)).toBe('className');
    expect(offControlOf('subclass:evocation', form)).toBe('subclassName');
    expect(offControlOf('background:acolyte', form)).toBe('background');
  });

  it('is null when no control holds it: a spell, or a choice the person already changed', () => {
    expect(offControlOf('spell:light', form)).toBeNull();
    expect(offControlOf('race:elf', form)).toBeNull();
  });
});
