import type { KeyName } from './feat-text';

/**
 * What a table class feature is, in Portuguese (MR-025): the class and level it comes at and the feature it replaces, as phrases
 * for the master's list and the read page. The server decides what the class has.
 */

/** The name of a feature key: the table's by `nameOf`, else the last word of its key (after `--` for a table feature's). */
export function featureKeyName(key: string, nameOf: KeyName): string {
  const named = nameOf(key);
  if (named !== key) {
    return named;
  }
  const last =
    key
      .split('--')
      .pop()
      ?.split(':')
      .pop()
      ?.replace(/@mesa$/, '') ?? key;
  const words = last.replace(/-/g, ' ');
  return words.charAt(0).toUpperCase() + words.slice(1);
}

/** "Guardião · 1º nível", then "Substitui Favored enemy 1" when it replaces one. */
export function classFeatureSupport(
  classKey: string,
  level: number,
  replaces: string,
  nameOf: KeyName,
): string[] {
  const out = [`${nameOf(classKey)} · ${level}º nível`];
  if (replaces !== '') {
    out.push(`Substitui ${featureKeyName(replaces, nameOf)}`);
  }
  return out;
}
