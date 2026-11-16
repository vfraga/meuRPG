import { isUnlimited } from '../../../core/combat/unlimited-uses';
import { hoursText, tight } from '../../../core/format/text';
import type { CharacterVitals } from '../../../../gen/meurpg/play/v1/play_pb';

/** What the sheet reads of the druid's Wild Shape in the vitals: the uses, and the form with its time and its pool. */
export interface WildRead {
  readonly uses: {
    readonly left: number;
    readonly total: number;
    readonly recharge: string;
  } | null;
  readonly form: string;
  /** "52 min restantes", as the server writes it; empty for a reader who may not have it. */
  readonly time: string;
  readonly beastKey: string;
  /** The whole duration of the form, in seconds; 0 when it has no clock. */
  readonly total: number;
  readonly hp: { readonly current: number; readonly max: number } | null;
}

export function readWild(v: Pick<CharacterVitals, 'resources' | 'wildShape'> | null): WildRead {
  const r = v?.resources.find((x) => x.key === 'wild_shape');
  const shape = v?.wildShape;
  return {
    uses: r
      ? {
          left: r.total - r.used,
          total: r.total,
          recharge: r.recharge === 2 ? 'long_rest' : 'short_rest',
        }
      : null,
    form: shape?.beastNamePt ?? '',
    time: shape?.timeLeftPt ?? '',
    beastKey: shape?.beastKey ?? '',
    total: shape?.secondsTotal ?? 0,
    hp: shape ? { current: shape.hitPointsCurrent, max: shape.hitPointsMax } : null,
  };
}

/** What the stay says of the beast: its pool and that the clock goes back to the form's whole length ("e o tempo volta a 2 h"). */
export function stayBeast(
  form: string,
  hp: { current: number } | null,
  totalSeconds: number,
): string {
  if (!hp) {
    return '';
  }
  const clock = totalSeconds > 0 ? `volta a ${hoursText(totalSeconds)}` : 'recomeça';
  return `: o ${form} fica com ${hp.current} PV e o tempo ${clock}`;
}

/** "Restam 2 de 2 usos · volta no descanso curto ou longo", or what stops Wild Shape outside a session. */
export function wildLineOf(u: WildRead['uses'], live: boolean): string {
  if (!live) {
    return 'Só durante um combate ou uma sessão.';
  }
  if (!u) {
    return '';
  }
  const back =
    u.recharge === 'long_rest' ? 'volta no descanso longo' : 'volta no descanso curto ou longo';
  return tight(
    u.left === 0
      ? `Sem usos · ${back}`
      : isUnlimited(u.total)
        ? `Usos ilimitados · ${back}`
        : `Restam ${u.left} de ${u.total} ${u.total === 1 ? 'uso' : 'usos'} · ${back}`,
  );
}
