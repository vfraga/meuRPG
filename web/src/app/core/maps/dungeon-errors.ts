import { Code, ConnectError } from '@connectrpc/connect';

import { DungeonOptionRefusedSchema } from '../../../gen/meurpg/maps/v1/dungeons_pb';
import { MapBlockedReason } from '../../../gen/meurpg/maps/v1/maps_pb';
import { describeConnectError, isRateLimited } from '../connect/connect-errors';
import { mapBlockedReason } from './map-errors';

/** The options the page has a control for, by the name the server uses in `DungeonOptionRefused`. */
export type OptionField =
  | 'size'
  | 'room_side_min'
  | 'room_side_max'
  | 'mask'
  | 'corridor_style'
  | 'door_mix'
  | 'deadend_removal'
  | 'stairs';

/** What the server refused in the options: the option it names (`''` when no single option is at fault), or `null` when it was another error. */
export function refusedOption(err: unknown): string | null {
  const connectErr = ConnectError.from(err, Code.Unavailable);
  if (connectErr.code !== Code.InvalidArgument) {
    return null;
  }
  return connectErr.findDetails(DungeonOptionRefusedSchema)[0]?.option ?? null;
}

/** The page's control for a server option name (the width and the height are the one "Tamanho"). */
export function optionField(option: string): OptionField | null {
  switch (option) {
    case 'width':
    case 'height':
      return 'size';
    case 'room_side_min':
    case 'room_side_max':
    case 'mask':
    case 'corridor_style':
    case 'door_mix':
    case 'deadend_removal':
    case 'stairs':
      return option;
    default:
      return null;
  }
}

/** The reason beside a refused option, in the words of the field (E10-05 3). */
const OPTION_TEXT: Readonly<Record<OptionField, string>> = {
  size: 'O tamanho vai de 21 a 121 quadrados.',
  room_side_min: 'O menor lado das salas vai de 3 a 15 quadrados e não pode passar do maior.',
  room_side_max: 'O maior lado das salas vai até 31 quadrados e não pode ser menor que o menor.',
  mask: 'Escolha um dos formatos da lista.',
  corridor_style: 'Escolha um dos tipos de corredor.',
  door_mix: 'Escolha um dos tipos de porta.',
  deadend_removal: 'Os becos vão de 0 a 100 %.',
  stairs: 'As escadas vão de 0 a 4.',
};

/** Said when the options are refused as a whole (the silhouette and the size leave no room for a room). */
export const NO_ROOM_TEXT =
  'Com esse tamanho e esse formato não cabe nenhuma sala. Aumente o tamanho ou mude o formato.';

/** What a failed preview looks like to the page: a refusal it can put on a field, or a failure with "Tentar de novo". */
export type PreviewFailure =
  | { readonly kind: 'refused'; readonly field: OptionField | null; readonly text: string }
  | { readonly kind: 'busy' }
  | { readonly kind: 'failed'; readonly text: string };

export function previewFailure(err: unknown): PreviewFailure {
  const option = refusedOption(err);
  if (option !== null) {
    const field = optionField(option);
    return { kind: 'refused', field, text: field ? OPTION_TEXT[field] : NO_ROOM_TEXT };
  }
  const code = ConnectError.from(err, Code.Unavailable).code;
  if (code === Code.ResourceExhausted && !isRateLimited(err)) {
    // A preview of this campaign is already running: the page asks again with the last options.
    return { kind: 'busy' };
  }
  if (code === Code.DeadlineExceeded) {
    return {
      kind: 'failed',
      text: 'O gerador demorou demais com essas opções. Diminua o tamanho ou mude o formato.',
    };
  }
  // A rate limit says how long to wait; the other codes (and a plain network failure) say the preview could not be made.
  return {
    kind: 'failed',
    text: describeConnectError(err, { [Code.Unavailable]: 'Não deu para gerar a prévia.' }),
  };
}

/** The words of a failed "Criar o mapa": the refused option goes on its field (`field`), anything else is the notice's text. */
export function createFailure(err: unknown): { field: OptionField | null; text: string } {
  const option = refusedOption(err);
  if (option !== null) {
    const field = optionField(option);
    return { field, text: field ? OPTION_TEXT[field] : NO_ROOM_TEXT };
  }
  return {
    field: null,
    text: describeConnectError(err, {
      [Code.InvalidArgument]: 'Não deu para criar o mapa: confira o nome e as opções.',
      [Code.NotFound]: 'Essa campanha não existe, ou você não é o mestre dela.',
      [Code.ResourceExhausted]:
        'A campanha chegou ao limite de 200 mapas ou da galeria, ou foram criadas masmorras demais agora há pouco. Espere um pouco e tente de novo; se continuar, apague um mapa ou uma imagem.',
      [Code.DeadlineExceeded]:
        'O gerador demorou demais com essas opções. Diminua o tamanho ou mude o formato.',
      [Code.Aborted]: 'O servidor estava ocupado. Tente de novo.',
      [Code.Unavailable]:
        'As imagens estão desligadas neste servidor, ou o servidor não respondeu. Tente de novo em instantes.',
    }),
  };
}

/** The words of a failed "Redesenhar", by the reason the server gives (never by its message). */
export function redrawFailure(err: unknown): string {
  switch (mapBlockedReason(err)) {
    case MapBlockedReason.IMAGE_CHANGED:
      return 'Este mapa já não é a masmorra que o app desenhou: a imagem foi trocada, a grade mudou ou as camadas foram apagadas. Não dá mais para redesenhar.';
    case MapBlockedReason.NO_GRID:
      return 'O mapa ficou sem grade. Defina a grade de novo para redesenhar.';
    default:
      break;
  }
  return describeConnectError(err, {
    [Code.NotFound]:
      'Este mapa não é mais uma masmorra gerada, ou não existe mais. Recarregue a página.',
    [Code.Aborted]:
      'As paredes ou as portas mudaram enquanto a imagem era desenhada. Tente de novo.',
    [Code.ResourceExhausted]:
      'Foram criadas ou redesenhadas masmorras demais nesta campanha agora há pouco. Espere uns 15 segundos e tente de novo.',
    [Code.Unavailable]:
      'As imagens estão desligadas neste servidor, ou o servidor não respondeu. Tente de novo em instantes.',
  });
}

/** The words of a failed "Pôr uma cena nesta sala". */
export function placeSceneFailure(err: unknown): string {
  return describeConnectError(err, {
    [Code.NotFound]: 'Essa sala não existe mais neste mapa. Recarregue a página.',
    [Code.ResourceExhausted]: 'O mapa chegou ao limite de 200 pontos. Apague um para pôr a cena.',
    [Code.Unavailable]: 'Não deu para pôr a cena agora. Tente de novo em instantes.',
  });
}
