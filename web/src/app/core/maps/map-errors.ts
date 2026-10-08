import { Code, ConnectError } from '@connectrpc/connect';

import { MapBlockedReason, MapBlockedSchema } from '../../../gen/meurpg/maps/v1/maps_pb';
import { describeConnectError } from '../connect/connect-errors';

/** The Portuguese message for a failed map call: what happened and how to
 * fix it (maps.proto lists which method returns what). */
export function mapErrorMessage(err: unknown, what = 'salvar'): string {
  return describeConnectError(err, {
    [Code.InvalidArgument]: `Não deu para ${what}: confira os campos e tente de novo.`,
    [Code.NotFound]: 'Esse mapa não existe mais, ou a imagem saiu da galeria. Recarregue a página.',
    [Code.PermissionDenied]: 'Só o mestre da campanha muda os mapas.',
    [Code.Aborted]: 'O mapa mudou em outra aba. Recarregue a página e tente de novo.',
    [Code.ResourceExhausted]: 'A campanha chegou ao limite de mapas ou de pontos.',
  });
}

/** The Portuguese message for a failed scene-action call (maps.proto,
 * `AddSceneAction` and the others): `resource_exhausted` is the 20-action limit. */
export function sceneActionErrorMessage(err: unknown, what = 'salvar a ação'): string {
  return describeConnectError(err, {
    [Code.InvalidArgument]: `Não deu para ${what}: o nome vai até 60 caracteres e a CD de 1 a 30.`,
    [Code.NotFound]: 'Esse ponto não existe mais. Recarregue a página.',
    [Code.PermissionDenied]: 'Só o mestre da campanha muda as ações da cena.',
    [Code.ResourceExhausted]: 'Limite de 20 ações. Remova uma para adicionar outra.',
  });
}

/** The Portuguese message for a failed clue call on a point (maps.proto,
 * `AddSceneClue` and the others): `resource_exhausted` is the 30-clue limit. */
export function sceneClueErrorMessage(err: unknown, what = 'salvar a pista'): string {
  return describeConnectError(err, {
    [Code.InvalidArgument]: `Não deu para ${what}: a pista vai de 1 a 500 caracteres, numa linha só.`,
    [Code.NotFound]: 'Essa pista ou esse ponto não existe mais. Recarregue a página.',
    [Code.PermissionDenied]: 'Só o mestre da campanha muda as pistas da cena.',
    [Code.ResourceExhausted]: 'Limite de 30 pistas. Remova uma para adicionar outra.',
  });
}

/** The Portuguese message for a failed `RevealSceneClue`. */
export function revealErrorMessage(err: unknown): string {
  return describeConnectError(err, {
    [Code.InvalidArgument]: 'Marque pelo menos um jogador para receber a pista.',
    [Code.NotFound]: 'A pista, ou um desses personagens, não existe mais. Feche e tente de novo.',
    [Code.PermissionDenied]: 'Só o mestre da campanha revela pistas.',
  });
}

/** The reason of a map call refused with `failed_precondition` (`MapBlocked`), or `null`. By the typed detail, never the message. */
export function mapBlockedReason(err: unknown): MapBlockedReason | null {
  const connectErr = ConnectError.from(err, Code.Unavailable);
  if (connectErr.code !== Code.FailedPrecondition) {
    return null;
  }
  return connectErr.findDetails(MapBlockedSchema)[0]?.reason ?? null;
}

/** The editor's calls that can be refused, each with the reasons and the limit it can answer (maps.proto lists them per method). */
export type EditorCall =
  /** `PaintMapCells`. */
  | 'paint'
  /** `SetMapGrid`. */
  | 'grid'
  /** `UpdateMap` with another image. */
  | 'image'
  /** `SetMapFog`. */
  | 'fog'
  /** `ForgetMapVision`. */
  | 'forget'
  /** `CreateMapPoint`. */
  | 'pointNew'
  /** `UpdateMapPoint`, `DeleteMapPoint` and the treasure marks. */
  | 'point'
  /** `DeleteMap`. */
  | 'delete';

/** What a full gallery means for the fog: it needs a copy of the image. */
export const GALLERY_FULL =
  'A galeria da campanha está cheia: apague uma imagem para ligar a névoa.';

/** What a full gallery means for a new image: it needs a copy of it. */
export const IMAGE_GALLERY_FULL =
  'A galeria da campanha está cheia: apague uma imagem para usar esta no mapa.';

/** The map's point limit (`CreateMapPoint`). */
export const POINTS_FULL = 'O mapa chegou ao limite de 200 pontos. Apague um para pôr outro.';

const BLOCKED_TEXT: Readonly<Record<number, string>> = {
  [MapBlockedReason.NO_GRID]: 'Defina a grade para pintar e ligar a névoa.',
  [MapBlockedReason.COMBAT_RUNNING]:
    'Há um combate neste mapa: a grade e a imagem só mudam depois dele.',
  [MapBlockedReason.TREASURE_CONVERTED]:
    'Esse tesouro já virou XP. Para mexer nele, desfaça esse XP na página da campanha.',
  [MapBlockedReason.TREASURE_FOUND]: 'Esse tesouro foi encontrado. Desmarque antes de apagar.',
};

const PROFILES: Readonly<
  Record<EditorCall, { blocked: readonly MapBlockedReason[]; exhausted?: string }>
> = {
  paint: { blocked: [MapBlockedReason.NO_GRID] },
  grid: { blocked: [MapBlockedReason.COMBAT_RUNNING] },
  image: { blocked: [MapBlockedReason.COMBAT_RUNNING], exhausted: IMAGE_GALLERY_FULL },
  fog: { blocked: [MapBlockedReason.NO_GRID], exhausted: GALLERY_FULL },
  forget: { blocked: [] },
  pointNew: { blocked: [], exhausted: POINTS_FULL },
  point: { blocked: [MapBlockedReason.TREASURE_CONVERTED, MapBlockedReason.TREASURE_FOUND] },
  delete: {
    blocked: [
      MapBlockedReason.COMBAT_RUNNING,
      MapBlockedReason.TREASURE_CONVERTED,
      MapBlockedReason.TREASURE_FOUND,
    ],
  },
};

/**
 * The Portuguese message for a failed call of the map editor. **Each call maps its own reasons and its own limit** (`call`): a refusal
 * the method cannot give (the gallery's limit on a new point, say) is never explained as one it can, and the generic words of the map
 * calls (`mapErrorMessage`, by code) answer the rest.
 */
export function editorErrorMessage(err: unknown, call: EditorCall, what: string): string {
  const profile = PROFILES[call];
  const reason = mapBlockedReason(err);
  if (reason !== null && profile.blocked.includes(reason)) {
    return call === 'delete' && reason === MapBlockedReason.COMBAT_RUNNING
      ? 'Há um combate neste mapa: ele só pode ser apagado depois do combate.'
      : BLOCKED_TEXT[reason];
  }
  const connectErr = ConnectError.from(err, Code.Unavailable);
  if (connectErr.code === Code.ResourceExhausted && profile.exhausted !== undefined) {
    return profile.exhausted;
  }
  return mapErrorMessage(err, what);
}
