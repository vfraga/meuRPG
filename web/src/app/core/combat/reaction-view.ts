import { AttackOutcome, ReactionRollKind } from '../../../gen/meurpg/play/v1/combat_pb';
import { metersText } from '../units';
import { circleLabel } from './combat-grid';
import { article } from './combat-log';
import { freeText } from './cast-flow';

/** The names of the reactions, as `names_pt.json` writes them. */
export const REACTION_NAMES = {
  shield: 'Escudo Arcano',
  uncannyDodge: 'Esquiva Sobrenatural',
  hellishRebuke: 'Repreensão Infernal',
  counterspell: 'Contramágica',
  cuttingWords: 'Palavras de Interrupção',
  deflectMissiles: 'Defletir Projéteis',
  featherFall: 'Queda Suave',
} as const;

/** "Rodada 2" with the words of the round. */
export function roundWord(round: number): string {
  return `Rodada ${round}`;
}

export function reach(distanceFt: number | undefined): string {
  return distanceFt === undefined ? '' : `a ${metersText(distanceFt)}`;
}

/** "Alcance 18 m: está a 9 m". */
export function reachChip(distanceFt: number | undefined): string {
  return distanceFt === undefined
    ? 'Alcance 18 m'
    : `Alcance 18 m: está a ${metersText(distanceFt)}`;
}

/** How the sheet shows one prompt: the title, what it is about, the question and the small print. */
export interface PromptView {
  readonly title: string;
  readonly subtitle: string;
  readonly icon: string;
  /** The reaction's name ("Escudo Arcano"). */
  readonly name: string;
  readonly question: string;
  readonly costs: readonly string[];
  readonly note: string;
  /** The words of "Usar ...". */
  readonly useLabel: string;
  /** The alert dialog's name. */
  readonly ariaLabel: string;
}

export function ofRollKind(kind: ReactionRollKind): { noun: string; verb: string } {
  switch (kind) {
    case ReactionRollKind.TEST:
      return { noun: 'do teste', verb: 'vai fazer um teste' };
    case ReactionRollKind.DAMAGE:
      return { noun: 'de dano', verb: 'vai causar dano' };
    default:
      return { noun: 'de ataque', verb: 'vai atacar' };
  }
}

export function theName(label: string): string {
  return `${article(label)} ${label}`;
}

/** What a reaction costs, for the prompts that do not already say it (SRD 5.1, Reactions: one per round, back at the start of the creature's next turn). */
export const REACTION_SPENT = 'Gasta a sua reação, que só volta no começo do seu próximo turno.';

/** What the sheet knows about the slot that was spent, for "Espaços de 3º nível: 1 livre de 2". */
export interface SlotAfter {
  readonly level: number;
  readonly free: number;
  readonly total: number | null;
}

/** How the sheet shows what an answer did. */
export interface ResultView {
  readonly title: string;
  readonly subtitle: string;
  readonly icon: string;
  /** The pill: "Anulada", "Errou", "Concentração perdida". */
  readonly pill: { readonly word: string; readonly good: boolean } | null;
  /** The sentences, one paragraph each. */
  readonly text: readonly string[];
  readonly chips: readonly string[];
  readonly note: string;
  /** The monk caught the missile and may throw it back (the second step is the window `nextWindowId`). */
  readonly throwBack: boolean;
}

export function slotChip(after: SlotAfter | null): string[] {
  return after
    ? [`Espaços de ${circleLabel(after.level)}: ${freeText(after.free, after.total)}`]
    : [];
}

export function outcomeWord(outcome: AttackOutcome): string {
  return outcome === AttackOutcome.HIT || outcome === AttackOutcome.CRITICAL_HIT
    ? 'acertou'
    : 'errou';
}
