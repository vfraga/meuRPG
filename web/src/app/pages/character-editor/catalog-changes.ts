import type { RulesCatalogVm } from './character-editor.types';

/** The keys a catalog offers the pickers that the master's switches move: races and sub-races, classes and subclasses, backgrounds, spells. */
function signature(c: RulesCatalogVm): string {
  return JSON.stringify([
    c.races.map((r) => [
      r.key,
      r.namePt,
      r.archived,
      r.off,
      r.subraces.map((s) => [s.key, s.archived, s.off]),
    ]),
    c.classes.map((x) => [
      x.key,
      x.namePt,
      x.archived,
      x.off,
      x.subclasses.map((s) => [s.key, s.archived, s.off]),
    ]),
    c.backgrounds.map((b) => [b.key, b.archived, b.off]),
    c.spells.map((s) => [s.key, s.archived, s.off, s.classKeys]),
  ]);
}

/** Whether the lists the person picks from are not the ones on screen (the master turned something on or off, or wrote an entry). */
export function offersChanged(before: RulesCatalogVm, after: RulesCatalogVm): boolean {
  return signature(before) !== signature(after);
}

/** Whether anything the catalog carries differs (hit dice, skill counts, spell circles, bonuses...): the editor computes from all of it. */
export function catalogChanged(before: RulesCatalogVm, after: RulesCatalogVm): boolean {
  return JSON.stringify(before) !== JSON.stringify(after);
}

/** The controls of the form that can hold a content key the master switched off. */
export type OffControl = 'race' | 'subrace' | 'className' | 'subclassName' | 'background';

/** Which control of the form holds `key` (null when none does: a spell, or a choice the person already changed). */
export function offControlOf(
  key: string,
  form: {
    race: string;
    subrace: string;
    className: string;
    subclassName: string;
    background: string;
  },
): OffControl | null {
  const controls: OffControl[] = ['race', 'subrace', 'className', 'subclassName', 'background'];
  return controls.find((c) => form[c] === key) ?? null;
}
