import {
  type GetTableScreenResponse,
  TableScreenKind,
  type TableScreenOption,
} from '../../../../gen/meurpg/play/v1/table_screen_pb';
import { unavailableText } from '../../../core/play/table-screen-labels';

/** The names of what the session has now, which the pin rows say under their titles. Empty where there is none. */
export interface TvNames {
  readonly map: string;
  readonly image: string;
  readonly scene: string;
}

/** One radio row of "Fixar". */
export interface TvPinRow {
  /** The row's name for tests and ids: "map", "image", "scene", "combat" or "idle". */
  readonly key: string;
  readonly kind: TableScreenKind;
  readonly title: string;
  readonly icon: string;
  /** What the row shows; the reason when it cannot be picked. */
  readonly sub: string;
  readonly available: boolean;
}

/** The "Fixar" rows in the board's order. */
const PIN_ORDER: readonly { key: string; kind: TableScreenKind; title: string; icon: string }[] = [
  { key: 'map', kind: TableScreenKind.MAP, title: 'Só o mapa', icon: 'map' },
  { key: 'image', kind: TableScreenKind.IMAGE, title: 'Imagem', icon: 'image' },
  { key: 'scene', kind: TableScreenKind.SCENE, title: 'Cena', icon: 'chat' },
  { key: 'combat', kind: TableScreenKind.COMBAT, title: 'Combate', icon: 'swords' },
  { key: 'idle', kind: TableScreenKind.IDLE, title: 'Tela de espera', icon: 'tv' },
];

function subtitle(kind: TableScreenKind, names: TvNames): string {
  switch (kind) {
    case TableScreenKind.MAP:
      return `${names.map || 'O mapa atual'}, com o que o grupo vê`;
    case TableScreenKind.IMAGE:
      return names.image || 'A imagem à mostra';
    case TableScreenKind.SCENE:
      return names.scene || 'A cena aberta';
    case TableScreenKind.COMBAT:
      return names.map ? `A ordem e o mapa da ${names.map}` : 'A ordem do combate';
    default:
      return 'Só o nome da campanha e da sessão';
  }
}

function rowOf(
  base: (typeof PIN_ORDER)[number],
  option: TableScreenOption,
  names: TvNames,
): TvPinRow {
  return {
    key: base.key,
    kind: base.kind,
    title: base.title,
    icon: base.icon,
    available: option.available,
    sub: option.available
      ? subtitle(base.kind, names)
      : unavailableText(option.unavailable) || 'Indisponível agora.',
  };
}

/** The pin rows the server offered, in the board's order; a kind it did not list has no row. */
export function pinRows(options: readonly TableScreenOption[], names: TvNames): TvPinRow[] {
  return PIN_ORDER.flatMap((base) => {
    const option = options.find((o) => o.kind === base.kind);
    return option ? [rowOf(base, option, names)] : [];
  });
}

/** "Na TV agora": the sentence's words before the name, the name (bold) and what follows it. */
export interface TvNowLine {
  readonly icon: string;
  readonly before: string;
  readonly name: string;
  readonly after: string;
}

/** What the TV shows now, from the server's stage; `pinned` adds "fixa". */
export function nowLine(data: GetTableScreenResponse, pinned: boolean): TvNowLine {
  const end = pinned ? ', fixa.' : '.';
  const body = data.stage?.body;
  switch (body?.case) {
    case 'map':
      return { icon: 'map', before: 'O mapa ', name: body.value.name, after: end };
    case 'image':
      return { icon: 'image', before: 'A imagem ', name: body.value.name, after: end };
    case 'scene':
      return { icon: 'chat', before: 'A cena ', name: body.value.name, after: end };
    case 'combat': {
      const map = body.value.map?.name;
      return {
        icon: 'swords',
        before: `O combate${pinned ? ', fixo' : ''}: a ordem${map ? ` e o mapa da ${map}` : ''}`,
        name: '',
        after: '.',
      };
    }
    default:
      return { icon: 'tv', before: 'A tela de espera', name: '', after: end };
  }
}

/** How the mirror picks, first that exists (the ⓘ of "Espelhar"). */
export const MIRROR_ORDER: readonly string[] = [
  'uma imagem mostrada a toda a mesa;',
  'o combate;',
  'a cena aberta a todos;',
  'o mapa;',
  'a tela de espera.',
];
