import { article } from '../format/article';
import { distanceText } from '../units';
import { LogContext } from './combat-log-types';

/** "no Toren", "na Brisa". */
export function inThe(name: string): string {
  return `${article(name) === 'a' ? 'na' : 'no'} ${name}`;
}

/** "o Goblin 1", "a Brisa". */
export function the(name: string): string {
  return `${article(name)} ${name}`;
}

/** The part a removal line names: "o Ataque Furtivo", "outra fonte" for a part hidden from the reader, "um extra" when none is named. */
export function removedName(name: string): string {
  if (name === '') {
    return 'um extra';
  }
  return name.toLowerCase() === 'outra fonte' ? 'outra fonte' : the(name);
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

/**
 * What the log says of a push after a failed save: "empurrão de 3 m · 2 quadrados", "empurrão de 1,5 m · 1 quadrado, que
 * parou antes: parede, porta, borda do mapa ou criatura", "não saiu do lugar: parede, porta, borda do mapa ou criatura", or, in a
 * combat with no map, that the table places the creature. Empty when nobody was pushed.
 */
export function pushText(push: {
  pushedFt: number;
  pushStopped: boolean;
  pushNoMap: boolean;
}): string {
  if (push.pushNoMap) {
    return 'o mestre posiciona a criatura, porque o combate não tem mapa';
  }
  const blockers = 'parede, porta, borda do mapa ou criatura';
  if (push.pushedFt > 0) {
    return `empurrão de ${distanceText(push.pushedFt)}${push.pushStopped ? `, que parou antes: ${blockers}` : ''}`;
  }
  return push.pushStopped ? `não saiu do lugar: ${blockers}` : '';
}
