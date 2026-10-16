import { article } from '../format/article';
import { LogContext } from './combat-log-types';

/** "no Toren", "na Brisa". */
export function inThe(name: string): string {
  return `${article(name) === 'a' ? 'na' : 'no'} ${name}`;
}

/** "o Goblin 1", "a Brisa". */
export function the(name: string): string {
  return `${article(name)} ${name}`;
}

/** "o Goblin 1", "Brisa": a creature has an article, a player's character does not. */
export function subject(label: string, ctx: LogContext): string {
  return ctx.players.has(label) ? label : the(label);
}

/** "do Hobgoblin", "de Brisa". */
export function ofSubject(label: string, ctx: LogContext): string {
  return ctx.players.has(label)
    ? `de ${label}`
    : `${article(label) === 'a' ? 'da' : 'do'} ${label}`;
}
