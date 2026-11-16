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
