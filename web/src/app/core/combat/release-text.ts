import { joinDots, tight } from '../format/text';
import { article } from '../format/article';
import { ofThe } from './move-plan';
import { circleLabel } from './combat-grid';

/**
 * The words of a readied spell or move released on the map (#857, #856, boards 857-02 to 857-09): the titles of the cast sheet and the
 * Mover page in release mode, their band, the master's foot and what a window that went away says. Pure functions.
 */

/** The button that goes back to the window (step 1 of the cast, the Mover page) and the one that leaves the result. */
export const RELEASE_BACK = 'Voltar';
export const RELEASE_DONE = 'Voltar à sessão';

/** A band's pieces: a bold lead, the trigger in quotes and the rest of the line. */
export interface ReleaseBand {
  readonly lead: string;
  readonly quote: string;
  readonly detail: string;
}

/** "Soltar Bola de Fogo", or "Soltar Bola de Fogo por Pensantus" when the master releases it. */
export function releaseTitle(name: string, master: boolean, reactor: string): string {
  const what = `Soltar ${name || 'a magia preparada'}`;
  return master ? `${what} por ${reactor}` : what;
}

/** The bold word that opens the subtitle of a release, drawn with a filled marker before it (boards 857-02, 857-04). */
export const RELEASE_LEAD = 'Reação';

/** "Reação · a ação preparada de Pensantus", and for the master "· você responde por Pensantus". */
export function releaseSubtitle(reactor: string, master: boolean): string {
  const line = `${RELEASE_LEAD} · a ação preparada de ${reactor}`;
  return master ? `${line} · você responde por ${reactor}` : line;
}

/** The band of a held spell: "Preparada: “…”" over "Espaço de 3º nível · gasto ao preparar". */
export function releaseBand(trigger: string, slotLevel: number): ReleaseBand {
  return {
    lead: 'Preparada:',
    quote: `“${trigger}”`,
    detail:
      slotLevel > 0
        ? `Espaço de ${tight(circleLabel(slotLevel))} · gasto ao preparar`
        : 'Truque · sem espaço',
  };
}

/** "vez do Ogro" for an NPC, "vez de Brisa" for a player's character; `''` when nobody is on turn. */
export function turnOfText(label: string, player: boolean): string {
  if (!label) {
    return '';
  }
  return player ? `vez de ${label}` : `vez ${ofThe([label])}`;
}

/** The band of the Mover page of a readied move: the player's own, or the master's for the player's character. */
export function moveBand(o: {
  readonly trigger: string;
  readonly reactor: string;
  readonly turnOf: string;
  readonly master: boolean;
}): ReleaseBand {
  const detail = o.master
    ? [
        `Fora da vez de ${o.reactor}`,
        o.turnOf,
        `gasta a ■ reação de ${o.reactor}`,
        `você responde por ${o.reactor}`,
      ]
    : ['Fora da sua vez', o.turnOf, 'gasta a ■ reação'];
  return {
    lead: o.master ? `Ação preparada de ${o.reactor}:` : 'Ação preparada:',
    quote: `“${o.trigger}”`,
    detail: joinDots(detail.filter(Boolean)),
  };
}

/** The cost card's second line on the Mover page of a readied move. */
export function moveNote(reactor: string, master: boolean): string {
  return master
    ? `É o movimento preparado de ${reactor}: gasta a ■ Reação de ${reactor} e não tira nada do deslocamento da vez de ${reactor}. Vale um movimento só.`
    : 'É o movimento preparado: gasta a ■ Reação e não tira nada do deslocamento da sua vez. Vale um movimento só.';
}

/** The foot of the master's screens: the player may still answer, and the first answer wins. */
export function masterFoot(reactor: string): string {
  return `${reactor} ainda pode responder no próprio aparelho: vale a primeira resposta.`;
}

/** Why "Mover agora" is off: the grapple or no speed at all (the budget of the window is 0). */
export function moveOffText(grappled: boolean): string {
  return grappled
    ? 'Condição Agarrado: o seu deslocamento é 0. Dá para deixar passar.'
    : 'O seu deslocamento é 0. Dá para deixar passar.';
}

/** What a release says when its window is no longer in the combat: a bold lead and the rest. */
export interface GoneView {
  readonly lead: string;
  readonly rest: string;
}

/** The held spell's name with its article ("a Bola de Fogo"). */
function theSpell(name: string): string {
  return name ? `${article(name)} ${name}` : 'a magia preparada';
}

/** "A Bola de Fogo" at the start of a sentence. */
function upFirst(text: string): string {
  return text.charAt(0).toUpperCase() + text.slice(1);
}

/**
 * The words of a release whose window went away (857-09 card 1): the master undid the trigger (the readied row still waits), or the row
 * no longer waits and the reason is the reaction used, the fall, the time, or nothing the wire says.
 */
export function releaseGoneView(o: {
  readonly spell: string;
  readonly move: boolean;
  readonly stillWaiting: boolean;
  readonly reactionUsed: boolean;
  readonly down: boolean;
  readonly timedOut: boolean;
  readonly afterMap: boolean;
}): GoneView {
  if (o.stillWaiting) {
    return {
      lead: 'O mestre desfez o gatilho:',
      rest: o.move ? 'o movimento continua preparado.' : `${theSpell(o.spell)} continua preparada.`,
    };
  }
  if (o.down) {
    return { lead: 'Condição Caído.', rest: o.move ? '' : 'A magia preparada se perde.' };
  }
  if (o.reactionUsed) {
    return { lead: 'A janela fechou e a sua reação foi usada.', rest: '' };
  }
  if (o.timedOut) {
    return {
      lead: 'Passou sozinho:',
      rest: `você não respondeu em ${o.afterMap ? '2 min' : '30 s'}.`,
    };
  }
  return {
    lead: 'A janela fechou.',
    rest: o.move
      ? 'Você não se moveu, e a reação continua livre.'
      : `${upFirst(theSpell(o.spell))} não saiu; a magia preparada e o espaço dela se perderam.`,
  };
}
