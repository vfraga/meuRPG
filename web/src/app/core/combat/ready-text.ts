import type { ReadyKind } from './ready';
import { tight } from '../format/text';
import { metersText } from '../units';

/** The words of the Ready sheet that are not a list or a refusal (boards 582-01 and 582-02). */

/** One line of the sheet's "what it costs" box. */
export interface ReadyLegendLine {
  readonly icon: string;
  readonly lead: string;
  readonly text: string;
}

const TURN_LOSS = 'Se o gatilho não vier até a sua próxima vez, se perde.';

/** What Ready costs and when it is lost, for the kind that is picked (and the slot, for a leveled spell). */
export function readyLegend(kind: ReadyKind, slotLevel: number): readonly ReadyLegendLine[] {
  if (kind === 'spell') {
    return [
      {
        icon: 'circle',
        lead: slotLevel > 0 ? `Ação e espaço de ${slotLevel}º círculo:` : 'Ação:',
        text: 'gastos agora, ao preparar.',
      },
      {
        icon: 'square',
        lead: 'Reação:',
        text: 'só se o mestre disparar e você soltar a magia.',
      },
      {
        icon: 'hourglass_empty',
        lead: '',
        text:
          slotLevel > 0
            ? 'Se a concentração acabar, ou o gatilho não vier até a sua vez, a magia e o espaço se perdem.'
            : 'Se a concentração acabar, ou o gatilho não vier até a sua vez, a magia se perde.',
      },
    ];
  }
  return [
    { icon: 'circle', lead: 'Ação:', text: 'gasta agora, ao preparar.' },
    { icon: 'square', lead: 'Reação:', text: 'só se o mestre disparar e você usar.' },
    { icon: 'hourglass_empty', lead: '', text: TURN_LOSS },
  ];
}

/** What a readied move says it may do: up to the speed the combatant has now, a reaction that provokes as any walk does. */
export function readyMoveText(speedFt: number): string {
  const far = speedFt > 0 ? `até ${metersText(speedFt)}` : 'até o seu deslocamento';
  return tight(
    `Você anda ${far} quando o mestre disparar. Provoca ataques de oportunidade, como qualquer movimento.`,
  );
}

/** The warning over a held spell: the concentration the readier keeps now ends when it readies, and holds the spell until it fires. */
export function concentrationNotice(
  current: string,
  spell: string,
): { readonly lead: string; readonly rest: string } | null {
  if (!current) {
    return null;
  }
  return {
    lead: `A concentração em ${current} acaba agora.`,
    rest: `Você se concentra em ${spell} até disparar.`,
  };
}
