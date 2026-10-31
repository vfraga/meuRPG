import { menu } from './content-testing-menu';
import { emptySubclass, unreadableSubclassPaths } from './subclass-draft';

describe('unreadableSubclassPaths', () => {
  it('names the start level of the casting while its text is no whole number, only for a subclass that casts', () => {
    const d = emptySubclass('class:fighter');
    d.casting = { ...d.casting, startLevel: Number.NaN };
    expect(unreadableSubclassPaths(d, menu())).toEqual([]);
    d.conjures = true;
    expect(unreadableSubclassPaths(d, menu())).toEqual(['table_subclass.casting.start_level']);
  });
});
