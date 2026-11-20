import type { ChangedContentVm } from '../character-sheet.types';

/**
 * The words around the server's change sentences ("A classe mudou", RN-23, question 80). The
 * sentences themselves (`ChangedContent.messages`) are shown exactly as the server wrote them; this
 * only names the kind of entry from its key and writes the date.
 */

interface Kind {
  /** "A classe mudou." */
  readonly title: string;
  /** "a classe", for "O mestre mudou a classe Guardião do Vale em 05/10/2026." */
  readonly noun: string;
}

const KINDS: Record<string, Kind> = {
  class: { title: 'A classe mudou.', noun: 'a classe' },
  subclass: { title: 'A subclasse mudou.', noun: 'a subclasse' },
  race: { title: 'A raça mudou.', noun: 'a raça' },
  subrace: { title: 'A sub-raça mudou.', noun: 'a sub-raça' },
  background: { title: 'O antecedente mudou.', noun: 'o antecedente' },
  spell: { title: 'A magia mudou.', noun: 'a magia' },
  weapon: { title: 'A arma mudou.', noun: 'a arma' },
  feat: { title: 'O talento mudou.', noun: 'o talento' },
  option: { title: 'A opção mudou.', noun: 'a opção' },
  classfeature: { title: 'A característica mudou.', noun: 'a característica' },
};

const FALLBACK: Kind = { title: 'O conteúdo mudou.', noun: 'o conteúdo' };

function kindOf(key: string): Kind {
  return KINDS[key.split(':')[0]] ?? FALLBACK;
}

const DATE = new Intl.DateTimeFormat('pt-BR', {
  day: '2-digit',
  month: '2-digit',
  year: 'numeric',
});

export function changeTitle(change: ChangedContentVm): string {
  return kindOf(change.key).title;
}

/** "O mestre mudou a classe Guardião do Vale em 05/10/2026. Os números da ficha já usam as regras novas. O que não combina mais:" */
export function changeIntro(change: ChangedContentVm): string {
  const when = change.changedAt ? ` em ${DATE.format(change.changedAt)}` : '';
  return `O mestre mudou ${kindOf(change.key).noun} ${change.namePt}${when}. Os números da ficha já usam as regras novas. O que não combina mais:`;
}

/** The title of the sheet "O que mudou no Guardião do Vale". */
export function changeSheetTitle(change: ChangedContentVm): string {
  return `O que mudou: ${change.namePt}`;
}

/** The live-region sentence: "A classe mudou: 1 aviso." / "2 avisos". */
export function changeAnnouncement(change: ChangedContentVm): string {
  const n = change.messages.length;
  return `${changeTitle(change).replace(/\.$/, '')}: ${n} ${n === 1 ? 'aviso' : 'avisos'}.`;
}

/** The code of the notice that a table weapon now takes both hands (`rules` package: `IssueShieldIdle`). */
const SHIELD_IDLE = 'shield_idle';

/**
 * Whether line `index` of the entry's sentences is the notice that a table weapon now takes both hands, so the equipped
 * shield does not count (580-16): the code at that place in `ChangedContent.codes` is `shield_idle`. Decided for each line,
 * never for the whole entry: the owner of the sheet ends this line by stowing the shield or the weapon, while any other
 * line of the same entry waits for the numbers. Told by the code the server sends, never by the words.
 */
export function isShieldLine(change: ChangedContentVm, index: number): boolean {
  return change.codes[index] === SHIELD_IDLE;
}

const WHO_SHIELD =
  'Quem ajusta: o dono da ficha, guardando o escudo ou a arma. O aviso some quando um dos dois deixa de estar em uso.';

/** Who adjusts a line and when it goes: the shield line has its own sentences, any other is about the numbers. */
export function whoAdjusts(shield: boolean, canEdit: boolean): string {
  return shield
    ? WHO_SHIELD
    : `Quem ajusta: ${canEdit ? 'você, na ficha' : 'o mestre, na ficha'}. O aviso some sozinho quando os números voltam a combinar.`;
}

/** The "Quem ajusta" sentences of an entry, one for each kind of line it has, in the order the lines first appear. */
export function whoLines(change: ChangedContentVm, canEdit: boolean): string[] {
  const kinds = change.messages.map((_, i) => isShieldLine(change, i));
  return [...new Set(kinds)].map((shield) => whoAdjusts(shield, canEdit));
}

/** A sentence of the server with "CA 13 → 11" tied by no-break spaces, so a line never breaks inside the two numbers. */
export function tieAcChange(message: string): string {
  return message.replace(/CA (\d+) → (\d+)/g, 'CA\u00a0$1\u00a0→\u00a0$2');
}
