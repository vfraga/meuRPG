import {
  TableScreenKind,
  TableScreenNotice,
  TableScreenUnavailable,
} from '../../../gen/meurpg/play/v1/table_screen_pb';

/** What the master's panel and the session bar know of the TV. */
export interface TableScreenLabelState {
  readonly open: boolean;
  /** When the last TV tab lost its stream; set only while none is open. */
  readonly lostAt: Date | null;
  readonly mirror: boolean;
  /** The pinned kind (read only while not mirroring). */
  readonly pin: TableScreenKind;
  /** The kind the server resolved for the stage now. */
  readonly shown: TableScreenKind;
}

const KIND_NAMES: Partial<Record<TableScreenKind, string>> = {
  [TableScreenKind.IDLE]: 'Espera',
  [TableScreenKind.MAP]: 'Mapa',
  [TableScreenKind.IMAGE]: 'Imagem',
  [TableScreenKind.SCENE]: 'Cena',
  [TableScreenKind.COMBAT]: 'Combate',
};

/** "Mapa", "Imagem", "Cena", "Combate" or "Espera"; an unknown kind reads as the waiting screen. */
export function kindName(kind: TableScreenKind): string {
  return KIND_NAMES[kind] ?? 'Espera';
}

/** The hour and minute of a moment, "21:14", in the viewer's time zone. */
export function clockText(at: Date): string {
  return new Intl.DateTimeFormat('pt-BR', { hour: '2-digit', minute: '2-digit' }).format(at);
}

function lost(state: TableScreenLabelState): boolean {
  return !state.open && state.lostAt !== null;
}

/** The session bar's button: "Tela da mesa", "Na TV: Cena", "Na TV: Mapa" with the pin marker, "TV sem conexão". */
export function barButtonLabel(state: TableScreenLabelState): { text: string; pinned: boolean } {
  if (lost(state)) {
    return { text: 'TV sem conexão', pinned: false };
  }
  if (!state.open) {
    return { text: 'Tela da mesa', pinned: false };
  }
  const kind = state.mirror ? state.shown : state.pin;
  return { text: `Na TV: ${kindName(kind)}`, pinned: !state.mirror };
}

/** The second line of "Tela da mesa…" in the phone's menu. */
export function menuLine(state: TableScreenLabelState): string {
  if (lost(state)) {
    return `Sem conexão desde ${clockText(state.lostAt as Date)}`;
  }
  if (!state.open) {
    return 'Nenhuma tela aberta';
  }
  const kind = kindName(state.mirror ? state.shown : state.pin);
  return `Na TV: ${kind} · ${state.mirror ? 'espelhando' : 'fixo'}`;
}

const UNAVAILABLE_TEXTS: Partial<Record<TableScreenUnavailable, string>> = {
  [TableScreenUnavailable.NO_COMBAT]: 'Nenhum combate agora.',
  [TableScreenUnavailable.NO_IMAGE]: 'Nenhuma imagem à mostra.',
  [TableScreenUnavailable.NO_SCENE]: 'Nenhuma cena aberta.',
  [TableScreenUnavailable.SCENE_NOT_FOR_ALL]: 'A cena está aberta só para alguns.',
  [TableScreenUnavailable.NO_MAP]: 'Nenhum mapa agora.',
  [TableScreenUnavailable.NEEDS_GROUP_VISION]: 'Precisa da Visão do grupo (editor do mapa).',
};

/** Why an option cannot be picked now; empty when nothing says why. */
export function unavailableText(reason: TableScreenUnavailable): string {
  return UNAVAILABLE_TEXTS[reason] ?? '';
}

const PIN_ENDED_TEXTS: Partial<Record<TableScreenKind, string>> = {
  [TableScreenKind.COMBAT]: 'O combate acabou.',
  [TableScreenKind.SCENE]: 'A cena fechou.',
  [TableScreenKind.IMAGE]: 'A imagem saiu da mesa.',
  [TableScreenKind.MAP]: 'O mapa saiu da mesa.',
};

/** Why the stage is not what the master asked for; empty without a notice. `pin` is the pinned kind that ended. */
export function noticeText(notice: TableScreenNotice, pin: TableScreenKind): string {
  switch (notice) {
    case TableScreenNotice.PIN_ENDED:
      return `${PIN_ENDED_TEXTS[pin] ?? 'O que estava fixo acabou.'} A TV foi para a tela de espera.`;
    case TableScreenNotice.SCENE_NOT_FOR_ALL:
      return 'A cena está aberta só para alguns jogadores, então a TV não a mostra.';
    case TableScreenNotice.MAP_NEEDS_GROUP_VISION:
      return 'O mapa tem névoa e a Visão do grupo está desligada, então a TV não o mostra.';
    default:
      return '';
  }
}
