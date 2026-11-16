import {
  type Combatant,
  type GetMoveOptionsResponse,
  MoveRefusal,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import type { Square } from '../../../../core/combat/combat-grid';
import type { JumpMode } from '../../../../core/combat/jump-plan';
import {
  LANDS_OUT_OF_REACH,
  type MoveIndex,
  type SquareVerdict,
  afterLeftText,
  costTitle,
  extraBreakdown,
  extrasOf,
  jumpCardText,
  knownTrapAt,
  leftLine,
  provokeWarning,
  provokedBy,
  refusalText,
} from '../../../../core/combat/move-plan';
import { metersFixed, reachSquares } from '../../../../core/units';
import type { Segment } from '../../../../shared/segmented/segmented';
import type { Reach } from '../../../../shared/combat-map/combat-map';
import type { MoveSummary } from './move-status';

/** Tenths of a foot in a foot: the combat counts distances in decifeet. */
const DFT_PER_FT = 10;

/** Nothing chosen, or a square the page has nothing to say about. */
export const IDLE_SUMMARY: MoveSummary = {
  kind: 'idle',
  title: '',
  detail: '',
  warning: '',
  trap: '',
};

const refused = (title: string, detail: string): MoveSummary => ({
  kind: 'refused',
  title,
  detail,
  warning: '',
  trap: '',
});

/** What the page knows about the jump that is picked. */
export interface JumpScene {
  readonly kind: JumpMode;
  /** The movement left that pays for a jump (the floor's), in decifeet. */
  readonly left: number;
  /** The counter where the jumper stands: the server's, at the speed of its surface; a high jump moves nobody, so what is left
   * after it is this less the height (no switch of surface to cross). */
  readonly leftHere: number;
  /** The high jump's height. */
  readonly height: number;
  readonly chosen: Square | null;
  readonly origin: Square;
  readonly verdict: SquareVerdict | null;
  /** What a jump can really do: the server's limit and no more than the movement left. */
  readonly cap: number;
  readonly limit: number;
  /** The length of the line a long jump draws. */
  readonly cost: number;
  readonly combatants: readonly Combatant[];
  readonly trapName: string;
}

function highJumpSummary(scene: JumpScene): MoveSummary {
  const h = scene.height;
  if (h <= 0) {
    return refused(
      'Sem salto em altura',
      'Com essa Força e esse movimento, não dá para subir nem um passo de 0,3 m.',
    );
  }
  return {
    kind: 'ok',
    title: `Saltar ${metersFixed(h / DFT_PER_FT)}`,
    detail: `Você sobe sem mudar de quadrado, para agarrar uma borda. ${afterLeftText(Math.max(scene.leftHere - h, 0))}`,
    warning: '',
    trap: '',
  };
}

/** A jump cannot cross a wall or land on a creature: what the reads show (a wall in the options' refusals, a creature
 * on the square) is refused here, so the button never promises what the server refuses. */
function blockedLanding(scene: JumpScene, to: Square): MoveSummary | null {
  const read = scene.verdict;
  if (read?.kind === 'refused' && read.reason === MoveRefusal.WALL) {
    return refused('Sem caminho', 'Um salto não atravessa parede. Escolha outro quadrado.');
  }
  const occupied = read?.kind === 'refused' && read.reason === MoveRefusal.OCCUPIED;
  if (occupied || scene.combatants.some((c) => c.placed && c.col === to.col && c.row === to.row)) {
    return refused('Ocupado', 'Há alguém nesse quadrado. Escolha onde cair.');
  }
  return null;
}

/** A landing the limit or the movement left does not pay. */
function tooFarLanding(scene: JumpScene): MoveSummary | null {
  if (scene.cost <= scene.cap) {
    return null;
  }
  return refused(
    'Longe demais',
    scene.cost > scene.limit
      ? `Seu salto vai até ${metersFixed(scene.limit / DFT_PER_FT)}. Escolha um quadrado dentro do círculo.`
      : `Você só tem ${metersFixed(scene.left / DFT_PER_FT)} de movimento. Escolha um quadrado dentro do círculo.`,
  );
}

function longJumpSummary(scene: JumpScene): MoveSummary {
  const to = scene.chosen;
  if (!to) {
    return IDLE_SUMMARY;
  }
  if (to.col === scene.origin.col && to.row === scene.origin.row) {
    return refused('Você já está aqui', 'Toque no quadrado onde quer cair.');
  }
  const stopped = blockedLanding(scene, to) ?? tooFarLanding(scene);
  if (stopped) {
    return stopped;
  }
  const read = scene.verdict;
  const names = read?.kind === 'ok' ? provokedBy(read.square, scene.combatants) : [];
  return {
    kind: 'ok',
    title: `Saltar ${metersFixed(scene.cost / DFT_PER_FT)}`,
    detail: jumpCardText(
      scene.cost,
      read?.kind === 'ok' ? read.square.leftAfterDft : null,
      scene.trapName,
    ),
    warning: names.length > 0 ? provokeWarning(names, 'salto') : '',
    trap: scene.trapName,
  };
}

/** The jump in words: the height of a high jump, or the landing square of a long one. */
export function jumpSummary(scene: JumpScene): MoveSummary {
  return scene.kind === 'high' ? highJumpSummary(scene) : longJumpSummary(scene);
}

/** The walk in words, from the options' cost for the chosen square. */
export function walkSummary(
  to: Square | null,
  v: SquareVerdict | null,
  left: number,
  combatants: readonly Combatant[],
): MoveSummary {
  if (!to || !v || v.kind === 'unknown') {
    return IDLE_SUMMARY;
  }
  if (v.kind === 'ok') {
    const names = provokedBy(v.square, combatants);
    const extra = extrasOf(v.square);
    return {
      kind: 'ok',
      title: costTitle(v.square.costDft),
      detail: [extraBreakdown(v.square), afterLeftText(v.square.leftAfterDft)]
        .filter(Boolean)
        .join(' '),
      warning: names.length > 0 ? provokeWarning(names) : '',
      trap: v.square.knownTrapName,
      ...(extra.parts.length > 0 ? { extra } : {}),
    };
  }
  const text = refusalText(v, left);
  return text ? { kind: 'refused', ...text, warning: '', trap: '' } : IDLE_SUMMARY;
}

/** The frame on the chosen square: the cost beside it when the move can be made, the refusal's title ("Sem caminho")
 * when it cannot. */
export function frameOf(
  square: Square | null,
  s: MoveSummary,
  jumping: boolean,
  kind: JumpMode,
  verdict: SquareVerdict | null,
  jumpCost: number,
) {
  if (!square || (jumping && kind === 'high')) {
    return null;
  }
  if (!jumping && verdict?.kind === 'unknown') {
    return { square, refused: false, label: undefined };
  }
  if (s.kind !== 'ok') {
    return {
      square,
      refused: true,
      label: s.title ? s.title.replace('Sem caminho reto', 'Sem caminho') : undefined,
    };
  }
  const dft = jumping ? jumpCost : verdict?.kind === 'ok' ? verdict.square.costDft : 0;
  return {
    square,
    refused: false,
    label: metersFixed(dft / DFT_PER_FT),
    // The square a jump lands on says it leaves the reach of someone who sees it (the same warning as the walk's).
    note: jumping && s.warning ? LANDS_OUT_OF_REACH : undefined,
  };
}

/** The line under the title: what is left to move, and what to tap. */
export function leadOf(
  jumping: boolean,
  kind: JumpMode,
  left: number,
  total: number,
  used: number,
  mount = '',
): string {
  let ask = 'Toque num quadrado destacado.';
  if (jumping) {
    ask = kind === 'long' ? 'Toque no quadrado onde quer cair.' : '';
  }
  const squares = jumping ? null : reachSquares(left / DFT_PER_FT);
  return [leftLine(left, total, used, squares, mount), ask].filter(Boolean).join(' ');
}

/** The footer button's words. */
export function goLabelOf(
  jumping: boolean,
  kind: JumpMode,
  disengaging: boolean,
  height: number,
): string {
  if (!jumping) {
    return 'Mover para cá';
  }
  if (kind === 'high') {
    return `Saltar ${metersFixed(height / DFT_PER_FT)} para cima`;
  }
  return disengaging ? 'Saltar e desengajar' : 'Saltar para cá';
}

/** The scroll box of the map: how much of it is seen, and where it is scrolled to. */
export interface ScrollBox {
  readonly left: number;
  readonly top: number;
  readonly width: number;
  readonly height: number;
}

/** Half a square: the middle of a square is that far from its edge. */
const HALF = 0.5;
/** A square and a half of margin around a square kept in view, for its label. */
const MARGIN_SQUARES = 1.5;

/** Where to scroll so the square is in the middle of the visible part. */
export function centreScroll(
  sq: Square,
  cell: number,
  box: ScrollBox,
): { left: number; top: number } {
  return {
    left: Math.max(0, (sq.col + HALF) * cell - box.width / 2),
    top: Math.max(0, (sq.row + HALF) * cell - box.height / 2),
  };
}

/** One axis of keeping a square in view: `from` is the square's edge, `at` where the box is scrolled to. */
function keepAxis(from: number, cell: number, pad: number, at: number, size: number): number {
  if (from - pad < at) {
    return Math.max(0, from - pad);
  }
  if (from + cell + pad > at + size) {
    return from + cell + pad - size;
  }
  return at;
}

/** Where to scroll to bring the square into view when it is not, with a square and a half of margin for its label. */
export function keepInViewScroll(
  sq: Square,
  cell: number,
  box: ScrollBox,
): { left: number; top: number } {
  const pad = cell * MARGIN_SQUARES;
  return {
    left: keepAxis(sq.col * cell, cell, pad, box.left, box.width),
    top: keepAxis(sq.row * cell, cell, pad, box.top, box.height),
  };
}

/** The reach the map draws: the walk's squares, or the circle of the jump (a high jump has no map). */
export function reachOf(
  readOnly: boolean,
  jumping: boolean,
  kind: JumpMode,
  origin: Square,
  cap: number,
  left: number,
  options: GetMoveOptionsResponse | null,
): Reach | null {
  if (readOnly || (jumping && kind === 'high')) {
    return null;
  }
  if (jumping) {
    return { origin, leftDft: cap, squares: [] };
  }
  return {
    origin,
    leftDft: left,
    squares: (options?.reachable ?? []).map((s) => ({ col: s.col, row: s.row })),
  };
}

/** "Mover para cá" may be pressed: a jump needs its height or a landing that works; a walk needs a square the options
 * allow (without the reach, unread or failed, the server decides and says why not). */
export function canMoveOf(
  busy: boolean,
  jumping: boolean,
  kind: JumpMode,
  height: number,
  chosen: Square | null,
  summaryKind: MoveSummary['kind'],
  verdict: SquareVerdict | null,
): boolean {
  if (busy) {
    return false;
  }
  if (jumping) {
    return kind === 'high' ? height > 0 : chosen !== null && summaryKind === 'ok';
  }
  return verdict?.kind === 'ok' || verdict?.kind === 'unknown';
}

/** A refusal belongs to the square it came for. */
export function errorShown(serverError: string, at: Square | null, to: Square | null): boolean {
  if (serverError === '') {
    return false;
  }
  return (!at && !to) || (!!at && !!to && at.col === to.col && at.row === to.row);
}

/** The two ways to move, and the two kinds of jump, as the segmented controls list them. */
export const MODES: readonly Segment<'walk' | 'jump'>[] = [
  { value: 'walk', label: 'Andar', icon: 'directions_walk' },
  { value: 'jump', label: 'Saltar', icon: 'north_east' },
];
export const KINDS: readonly Segment<JumpMode>[] = [
  { value: 'long', label: 'Distância', icon: 'straighten' },
  { value: 'high', label: 'Altura', icon: 'arrow_upward' },
];

/** The trap the character knows on the chosen square. The walk's reads know the trap squares; a long jump that lands
 * on one asks too (a high jump has no square). */
export function trapNameOf(
  jumping: boolean,
  kind: JumpMode,
  to: Square | null,
  index: MoveIndex,
  verdict: SquareVerdict | null,
): string {
  if (jumping) {
    return kind === 'long' && to ? knownTrapAt(index, to) : '';
  }
  return verdict?.kind === 'ok' ? verdict.square.knownTrapName : '';
}

/** What "Saltar para cá" asks for. */
export type JumpRequest =
  | {
      readonly kind: 'long';
      readonly square: Square;
      /** "Saltar e desengajar": the action is taken first, and then the jump leaves the reach with no attack. */
      readonly disengage?: true;
    }
  | { readonly kind: 'high'; readonly heightDft: number };

/** What the footer button asks for when pressed on a jump. */
export function jumpRequestOf(
  kind: JumpMode,
  height: number,
  to: Square | null,
  disengaging: boolean,
): JumpRequest | null {
  if (kind === 'high') {
    return { kind: 'high', heightDft: height };
  }
  if (!to) {
    return null;
  }
  return disengaging ? { kind: 'long', square: to, disengage: true } : { kind: 'long', square: to };
}

/** The icon of the footer button. */
export function goIconOf(jumping: boolean, kind: JumpMode): string {
  if (!jumping) {
    return 'arrow_forward';
  }
  return kind === 'high' ? 'arrow_upward' : 'north_east';
}

/** The server's answer for this origin, but never more than the combatant's live movement: after a move the options
 * are the old ones until they are fetched again, and the sheet would keep saying the distance from before. */
export function cappedMovement(fetched: number | undefined, live: number | undefined): number {
  if (fetched === undefined) {
    return live ?? 0;
  }
  return live === undefined ? fetched : Math.min(fetched, live);
}
