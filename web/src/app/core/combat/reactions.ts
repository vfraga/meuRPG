import {
  AttackOutcome,
  type ConcentrationSavePrompt,
  type ConcentrationSaveResult,
  type HellishRebukeOption,
  type Encounter,
  type ReactionResult,
  ReactionKind,
  ReactionRollKind,
  type ReactionWindow,
  ReactionWindowStatus,
} from '../../../gen/meurpg/play/v1/combat_pb';
import type { SlotChoice } from '../../../gen/meurpg/rules/v1/rules_pb';
import { joinDots } from '../format/text';
import { metersText } from '../units';
import { circleLabel } from './combat-grid';
import { article } from './combat-log';
import { rollFormula, rollText } from './combat-dice';
import { luckyText } from './lucky';
import { freeText } from './cast-flow';
import { ofThe } from './move-plan';

/**
 * The reaction window on screen (PM-04): what the server's `Encounter.reaction_windows` and `reaction_wait` say, in
 * words. The server decides who may react, to what and in which order, and writes what each person may read; the
 * browser only phrases the prompt and the result it was sent. Pure functions, tested without a DOM.
 */

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

/** A window that still waits for an answer. */
export function isOpen(w: ReactionWindow): boolean {
  return (
    w.status !== ReactionWindowStatus.ANSWERED && w.status !== ReactionWindowStatus.CLOSED_BY_ITSELF
  );
}

/** The windows that are open, in the order the server answers them. */
export function openWindows(e: Encounter): readonly ReactionWindow[] {
  // The master's question about a hidden creature an area hit is a window of the server's, drawn by its own card; a contest
  // (W7-X) waits in a window of kind CONTEST that RespondContest and ResolveShove answer, never AnswerReaction: its sheets
  // read it with `contestWindows`, and the reaction cards and queues never see it.
  return e.reactionWindows.filter(
    (w) => isOpen(w) && w.kind !== ReactionKind.HIDDEN_REVEAL && w.kind !== ReactionKind.CONTEST,
  );
}

/** The contests that wait (windows of kind CONTEST that are open), as the caller may read them: the ones the caller answers
 * have `forYou`, and `contest.contestId` names the contest (read with `GetContestState`). */
export function contestWindows(e: Encounter): readonly ReactionWindow[] {
  return e.reactionWindows.filter((w) => isOpen(w) && w.kind === ReactionKind.CONTEST);
}

/** The contest id of a window of kind CONTEST, or `''` for any other. */
export function contestIdOf(w: ReactionWindow): string {
  return w.kind === ReactionKind.CONTEST && w.prompt.case === 'contest'
    ? w.prompt.value.contestId
    : '';
}

/** The window a player answers now: the first one that is theirs to answer. */
export function windowForPlayer(e: Encounter): ReactionWindow | undefined {
  return openWindows(e).find((w) => w.forYou);
}

/** The window whose sheet opens by itself for a player: the first they answer that has a prompt of its own (the
 * master's check and the aggressor's saving throw are the master's cards). */
export function sheetWindow(e: Encounter): ReactionWindow | undefined {
  return openWindows(e).find(
    (w) =>
      w.forYou &&
      w.prompt.case !== undefined &&
      w.prompt.case !== 'masterCheck' &&
      w.prompt.case !== 'hellishRebukeSave',
  );
}

/** What the combat waits for, as the caller may read it: the title (a live region) and the line under it. */
export function reactionWait(e: Encounter): { title: string; detail: string } | null {
  const wait = e.reactionWait;
  return wait && wait.titlePt ? { title: wait.titlePt, detail: wait.detailPt } : null;
}

/** "Rodada 2" with the words of the round. */
function roundWord(round: number): string {
  return `Rodada ${round}`;
}

function reach(distanceFt: number | undefined): string {
  return distanceFt === undefined ? '' : `a ${metersText(distanceFt)}`;
}

/** "Alcance 18 m: está a 9 m". */
function reachChip(distanceFt: number | undefined): string {
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

function ofRollKind(kind: ReactionRollKind): { noun: string; verb: string } {
  switch (kind) {
    case ReactionRollKind.TEST:
      return { noun: 'do teste', verb: 'vai fazer um teste' };
    case ReactionRollKind.DAMAGE:
      return { noun: 'de dano', verb: 'vai causar dano' };
    default:
      return { noun: 'de ataque', verb: 'vai atacar' };
  }
}

function theName(label: string): string {
  return `${article(label)} ${label}`;
}

function capitalized(text: string): string {
  return text.charAt(0).toUpperCase() + text.slice(1);
}

/** What a reaction costs, for the prompts that do not already say it (SRD 5.1, Reactions: one per round, back at the start of the creature's next turn). */
const REACTION_SPENT = 'Gasta a sua reação, que só volta no começo do seu próximo turno.';

type PromptCase = NonNullable<ReactionWindow['prompt']['case']>;
type PromptOf<K extends PromptCase> = Extract<ReactionWindow['prompt'], { case: K }>['value'];

function shieldPrompt(s: PromptOf<'shield'>, rodada: string): PromptView {
  const name = s.spellNamePt || REACTION_NAMES.shield;
  return {
    title: 'Você foi atingido',
    subtitle: s.attackerLabel
      ? joinDots([s.attackerLabel, ...(s.attackNamePt ? [s.attackNamePt] : []), rodada])
      : rodada,
    icon: 'shield',
    name,
    question: s.magicMissile
      ? `Usar ${name}? Você é alvo de Mísseis Mágicos: o ${name} segura todo o dano. Gasta a sua reação e um espaço de magia.`
      : `Usar ${name}? A sua CA sobe 5 até o começo do seu próximo turno, e o ataque pode virar erro. Gasta a sua reação e um espaço de magia.`,
    costs: [],
    note: '',
    useLabel: `Conjurar ${name}`,
    ariaLabel: `Você foi atingido: usar ${name}?`,
  };
}

function uncannyDodgePrompt(u: PromptOf<'uncannyDodge'>, rodada: string): PromptView {
  const name = REACTION_NAMES.uncannyDodge;
  return {
    title: 'Você foi atingido',
    subtitle: joinDots([u.attackerLabel, ...(u.attackNamePt ? [u.attackNamePt] : []), rodada]),
    icon: 'directions_run',
    name,
    question: `Usar ${name}? O ataque causaria ${u.damage} de dano; ele cai pela metade, para ${u.halved}. Gasta a sua reação.`,
    costs: ['Reação', 'Sem espaço de magia'],
    note: 'O ataque já acertou; o dano ainda não foi aplicado. O dano é do seu personagem: você o vê, como numa mesa.',
    useLabel: `Usar ${name}`,
    ariaLabel: `Você foi atingido: usar ${name}?`,
  };
}

function hellishRebukePrompt(h: PromptOf<'hellishRebuke'>, rodada: string): PromptView {
  const name = REACTION_NAMES.hellishRebuke;
  const dice = h.options[0]?.diceCount ?? 2;
  return {
    title: 'Você sofreu dano',
    subtitle: joinDots([
      h.aggressorLabel,
      ...(h.attackNamePt ? [h.attackNamePt] : []),
      ...(h.distanceFt !== undefined ? [reach(h.distanceFt)] : []),
      rodada,
    ]),
    icon: 'local_fire_department',
    name,
    question: `Usar ${name} contra ${theName(h.aggressorLabel)}? A criatura faz um teste de resistência de Destreza (CD ${h.saveDc}) e sofre ${dice}d10 de fogo, ou metade se passar. ${REACTION_SPENT}`,
    costs: ['Reação', reachChip(h.distanceFt)],
    note: '',
    useLabel: `Usar ${name}`,
    ariaLabel: `Você sofreu dano: usar ${name}?`,
  };
}

function counterspellPrompt(c: PromptOf<'counterspell'>, rodada: string): PromptView {
  const name = REACTION_NAMES.counterspell;
  return {
    title: `${c.casterLabel} está conjurando`,
    subtitle: joinDots([
      c.casterLabel,
      ...(c.distanceFt !== undefined ? [`${reach(c.distanceFt)} de você`] : []),
      rodada,
    ]),
    icon: 'block',
    name,
    question: `Usar ${name}? Se a magia for de nível igual ou menor que o espaço que você gastar, ela falha e não tem efeito. Se for maior, você faz um teste de habilidade de conjuração (CD 10 + o nível da magia). ${REACTION_SPENT}`,
    costs: ['Reação', reachChip(c.distanceFt)],
    note: 'Você vê que há uma conjuração, mas não sabe qual magia é nem o nível dela.',
    useLabel: `Usar ${name}`,
    ariaLabel: `${c.casterLabel} está conjurando: usar ${name}?`,
  };
}

function cuttingWordsPrompt(c: PromptOf<'cuttingWords'>, rodada: string): PromptView {
  const name = REACTION_NAMES.cuttingWords;
  const what = ofRollKind(c.rollKind);
  return {
    title: `${capitalized(theName(c.rollerLabel))} ${what.verb}`,
    subtitle: joinDots([
      c.targetLabel ? `${c.rollerLabel} ataca ${theName(c.targetLabel)}` : c.rollerLabel,
      ...(c.distanceFt !== undefined ? [`${reach(c.distanceFt)} de você`] : []),
      rodada,
    ]),
    icon: 'record_voice_over',
    name,
    question: `Usar ${name}? Você subtrai o dado de Inspiração de Bardo (d${c.dieSides}) da rolagem ${what.noun}. O resultado ainda não foi dito. ${REACTION_SPENT}`,
    costs: [
      'Reação',
      `Inspiração de Bardo: ${c.usesLeft} ${c.usesLeft === 1 ? 'uso' : 'usos'}`,
      reachChip(c.distanceFt),
    ],
    note: 'O total e a CA do NPC não aparecem. Vale para uma jogada de ataque, um teste de habilidade ou uma jogada de dano de uma criatura que você vê e que pode ouvir você.',
    useLabel: `Usar ${name}`,
    ariaLabel: `${capitalized(theName(c.rollerLabel))} ${what.verb}: usar ${name}?`,
  };
}

function deflectMissilesPrompt(d: PromptOf<'deflectMissiles'>, rodada: string): PromptView {
  const name = REACTION_NAMES.deflectMissiles;
  return {
    title: 'Você foi atingido à distância',
    subtitle: joinDots([d.attackerLabel, ...(d.attackNamePt ? [d.attackNamePt] : []), rodada]),
    icon: 'swap_calls',
    name,
    question: `Usar ${name}? O dano cai 1d${d.dieSides} + ${d.dexMod} (Destreza) + ${d.monkLevel} (nível de monge). Se ele chegar a 0, você pode apanhar o projétil. ${REACTION_SPENT}`,
    costs: ['Reação', 'Defletir não gasta chi'],
    note: `O ataque já acertou e causaria ${d.damage} de dano; ele ainda não foi aplicado.`,
    useLabel: `Usar ${name}`,
    ariaLabel: `Você foi atingido à distância: usar ${name}?`,
  };
}

function maneuverReducePrompt(d: PromptOf<'maneuverReduce'>, rodada: string): PromptView {
  const first = d.options[0];
  const plus =
    first && first.flatBonus !== 0
      ? ` ${first.flatBonus < 0 ? '−' : '+'} ${Math.abs(first.flatBonus)}`
      : '';
  const sides =
    d.options.length === 1 && first ? `1d${first.dieSides}${plus}` : 'o dado da manobra';
  return {
    title: 'Você foi atingido corpo a corpo',
    subtitle: joinDots([d.attackerLabel, ...(d.attackNamePt ? [d.attackNamePt] : []), rodada]),
    icon: 'shield',
    name: 'Manobra',
    question: `Usar uma manobra? O dano cai ${sides}. ${REACTION_SPENT}`,
    costs: ['Reação', 'Gasta um uso da manobra'],
    note: `O ataque já acertou e causaria ${d.damage} de dano; ele ainda não foi aplicado.`,
    useLabel: 'Usar a manobra',
    ariaLabel: 'Você foi atingido corpo a corpo: usar uma manobra?',
  };
}

function featherFallPrompt(f: PromptOf<'featherFall'>, rodada: string): PromptView {
  const name = REACTION_NAMES.featherFall;
  const one = f.falling.length === 1 ? f.falling[0] : undefined;
  return {
    title: one ? `${one.label} está caindo` : `${f.falling.length} criaturas estão caindo`,
    subtitle: joinDots([
      `queda de ${metersText(f.fallFt)}`,
      ...(one?.distanceFt !== undefined ? [`${reach(one.distanceFt)} de você`] : []),
      rodada,
    ]),
    icon: 'arrow_upward',
    name,
    question: `Usar ${name}? As criaturas que você escolher descem 18 m por rodada e não sofrem dano de queda. ${REACTION_SPENT}`,
    costs: ['Reação', 'Dura 1 minuto'],
    note: '',
    useLabel: `Usar ${name}`,
    ariaLabel: `${one ? `${one.label} está caindo` : 'Há criaturas caindo'}: usar ${name}?`,
  };
}

function concentrationSavePrompt(c: PromptOf<'concentrationSave'>, rodada: string): PromptView {
  const spell = c.spellNamePt;
  const source = c.sourceLabel ? `${c.sourceLabel} · ` : '';
  return {
    title: 'Concentração em risco',
    subtitle: `${source}você sofreu ${c.damageTaken} de dano · ${rodada}`,
    icon: 'hourglass_empty',
    name: 'Teste de concentração',
    question: `Você mantém a concentração em ${spell}. Faça um teste de resistência de Constituição contra CD ${c.dc} (metade dos ${c.damageTaken} de dano; o mínimo é 10).`,
    costs: [],
    note: `Se falhar, a concentração em ${spell} acaba. O ataque que deu o dano espera o seu teste.`,
    useLabel: 'Rolar no app',
    ariaLabel: `Concentração em risco: teste de resistência de Constituição contra CD ${c.dc}`,
  };
}

const PROMPTS: { [K in PromptCase]?: (value: PromptOf<K>, rodada: string) => PromptView } = {
  shield: shieldPrompt,
  uncannyDodge: uncannyDodgePrompt,
  hellishRebuke: hellishRebukePrompt,
  counterspell: counterspellPrompt,
  cuttingWords: cuttingWordsPrompt,
  deflectMissiles: deflectMissilesPrompt,
  maneuverReduce: maneuverReducePrompt,
  featherFall: featherFallPrompt,
  concentrationSave: concentrationSavePrompt,
};

/** The prompt of a window, or `null` for a kind the player has no sheet for (the master's check). */
export function promptView(w: ReactionWindow, round: number): PromptView | null {
  const p = w.prompt;
  const build = p.case
    ? (PROMPTS[p.case] as ((v: unknown, r: string) => PromptView) | undefined)
    : undefined;
  return build ? build(p.value, roundWord(round)) : null;
}

/** The slots a window offers, as the server lists them (Shield's, Counterspell's, Feather Fall's). */
export function slotChoices(w: ReactionWindow): readonly SlotChoice[] {
  const p = w.prompt;
  switch (p.case) {
    case 'shield':
      return p.value.slots;
    case 'counterspell':
      return p.value.slotOptions;
    case 'featherFall':
      return p.value.slotOptions;
    default:
      return [];
  }
}

/** The kinds whose "Usar" asks for a die the app or the table rolls up front. */
export function dieSidesOf(w: ReactionWindow): number {
  const p = w.prompt;
  switch (p.case) {
    case 'cuttingWords':
      return p.value.dieSides;
    case 'deflectMissiles':
      return p.value.dieSides;
    case 'maneuverReduce':
      return p.value.options[0]?.dieSides ?? 0;
    default:
      return 0;
  }
}

/** One row of the Repreensão Infernal's list: the Infernal Legacy or a slot. */
export interface RebukeRow {
  readonly racial: boolean;
  readonly level: number;
  readonly pact: boolean;
  readonly free: number;
  readonly title: string;
  readonly detail: string;
  readonly enabled: boolean;
  readonly diceCount: number;
}

function rebukeRow(o: HellishRebukeOption): RebukeRow {
  if (o.racial) {
    const uses = o.usesLeft === 1 ? '1 uso' : `${o.usesLeft} usos`;
    return {
      racial: true,
      level: o.level,
      pact: false,
      free: o.usesLeft,
      title: 'Legado Infernal',
      detail: `${circleLabel(o.level)}, sem gastar espaço · ${uses} por descanso longo`,
      enabled: o.usesLeft > 0,
      diceCount: o.diceCount,
    };
  }
  const free = o.slot?.free ?? 0;
  const level = o.slot?.level ?? o.level;
  return {
    racial: false,
    level,
    pact: o.slot?.pact ?? false,
    free,
    title: o.slot?.pact ? 'Espaço de pacto' : circleLabel(level),
    detail: `${circleLabel(o.level)} · ${freeText(free, null)}`,
    enabled: free > 0,
    diceCount: o.diceCount,
  };
}

/** The ways to pay for Repreensão Infernal, in the order the sheet offers them. */
export function rebukeRows(w: ReactionWindow): readonly RebukeRow[] {
  const p = w.prompt;
  return p.case === 'hellishRebuke' ? p.value.options.map(rebukeRow) : [];
}

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

function slotChip(after: SlotAfter | null): string[] {
  return after
    ? [`Espaços de ${circleLabel(after.level)}: ${freeText(after.free, after.total)}`]
    : [];
}

function outcomeWord(outcome: AttackOutcome): string {
  return outcome === AttackOutcome.HIT || outcome === AttackOutcome.CRITICAL_HIT
    ? 'acertou'
    : 'errou';
}

type ResultCase = NonNullable<ReactionResult['result']['case']>;
type ResultOf<K extends ResultCase> = Extract<ReactionResult['result'], { case: K }>['value'];

interface ResultContext {
  readonly round: number;
  readonly after: SlotAfter | null;
  readonly armorClass: number | null;
}

/** The pieces every result shares. */
function base(ctx: ResultContext): {
  subtitle: string;
  used: string[];
} {
  return { subtitle: roundWord(ctx.round), used: ['Reação usada'] };
}

/** What Escudo Arcano adds to the armor class. */
const SHIELD_ARMOR_CLASS = 5;

function shieldResult(
  v: ResultOf<'shield'>,
  asked: ReactionWindow,
  ctx: ResultContext,
): ResultView {
  const prompt = asked.prompt.case === 'shield' ? asked.prompt.value : null;
  const name = prompt?.spellNamePt || REACTION_NAMES.shield;
  const by = prompt?.attackerLabel ? ` ${ofThe([prompt.attackerLabel])}` : '';
  return {
    title: `${name} conjurado`,
    subtitle: '',
    icon: 'shield',
    pill: { word: v.stopped ? 'Errou' : 'Acertou', good: v.stopped },
    text: [
      v.stopped
        ? `O ${name} segurou o ataque${by}.`
        : `Mesmo com o ${name}, o ataque acertou. O dano segue para o mestre.`,
      ctx.armorClass === null
        ? `Sua CA sobe ${SHIELD_ARMOR_CLASS} até o começo do seu próximo turno.`
        : `Sua CA é ${ctx.armorClass + SHIELD_ARMOR_CLASS} até o começo do seu próximo turno.`,
    ],
    chips: [...base(ctx).used, ...slotChip(ctx.after)],
    note: '',
    throwBack: false,
  };
}

function uncannyDodgeResult(
  v: ResultOf<'uncannyDodge'>,
  _asked: ReactionWindow,
  ctx: ResultContext,
): ResultView {
  return {
    title: `${REACTION_NAMES.uncannyDodge} usada`,
    subtitle: base(ctx).subtitle,
    icon: 'directions_run',
    pill: null,
    text: [`O dano do ataque caiu de ${v.damageBefore} para ${v.damageAfter}.`],
    chips: base(ctx).used,
    note: '',
    throwBack: false,
  };
}

function hellishRebukeResult(
  v: ResultOf<'hellishRebuke'>,
  asked: ReactionWindow,
  ctx: ResultContext,
): ResultView {
  const prompt = asked.prompt.case === 'hellishRebuke' ? asked.prompt.value : null;
  const who = prompt ? theName(prompt.aggressorLabel) : 'a criatura';
  const outcome = v.saved ? 'passou e sofreu metade do fogo' : 'falhou e sofreu o fogo';
  return {
    title: `${REACTION_NAMES.hellishRebuke} usada`,
    subtitle: base(ctx).subtitle,
    icon: 'local_fire_department',
    pill: v.save ? { word: v.saved ? 'Passou' : 'Falhou', good: !v.saved } : null,
    text: [
      v.save
        ? `Teste de Destreza ${ofThe([prompt?.aggressorLabel ?? ''])}: ${rollText(v.save)} contra CD ${v.saveDc}. ${capitalized(who)} ${outcome}: ${v.damage} de dano.`
        : `O teste de ${who} é do mestre: o resultado aparece no registro do combate.`,
      // "Sortudo: 1 → 14": a halfling player's 1 the app rolled again by itself; the roll above already has the new face.
      luckyText(v.save),
    ].filter(Boolean),
    chips: [...base(ctx).used, ...(v.racial ? ['Legado Infernal usado'] : slotChip(ctx.after))],
    note: '',
    throwBack: false,
  };
}

function counterspellResult(
  v: ResultOf<'counterspell'>,
  asked: ReactionWindow,
  ctx: ResultContext,
): ResultView {
  const caster = asked.prompt.case === 'counterspell' ? asked.prompt.value.casterLabel : '';
  const spell = `${v.spellNamePt}, de ${circleLabel(v.spellLevel)}`;
  const by = caster ? ` ${ofThe([caster])}` : '';
  return {
    title: `${REACTION_NAMES.counterspell} usada`,
    subtitle: caster ? joinDots([caster, base(ctx).subtitle]) : base(ctx).subtitle,
    icon: 'block',
    pill: { word: v.countered ? 'Anulada' : 'Não anulada', good: v.countered },
    text: [
      v.countered
        ? `A magia${by} foi anulada: ${spell}.`
        : `A magia${by} não foi anulada: ${spell}.`,
      v.check
        ? `Teste de habilidade de conjuração: ${rollText(v.check)} contra CD ${v.checkDc}.`
        : 'O espaço usado era de nível igual ou maior, então não houve teste.',
    ],
    chips: [...base(ctx).used, ...slotChip(ctx.after)],
    note: '',
    throwBack: false,
  };
}

/** The sentence about the roll the die changed: only "acertou" or "errou" for an attack, never a total. */
function cuttingSubject(v: ResultOf<'cuttingWords'>, roller: string, target: string): string {
  if (v.rollKind !== ReactionRollKind.ATTACK) {
    return `A rolagem ${ofRollKind(v.rollKind).noun} ${ofThe([roller])} mudou.`;
  }
  const outcome = outcomeWord(v.outcome);
  return `O ataque ${ofThe([roller])} ${outcome}${target ? ` ${theName(target)}` : ''}.`;
}

function cuttingWordsResult(
  v: ResultOf<'cuttingWords'>,
  asked: ReactionWindow,
  ctx: ResultContext,
): ResultView {
  const prompt = asked.prompt.case === 'cuttingWords' ? asked.prompt.value : null;
  const roller = prompt?.rollerLabel ?? '';
  return {
    title: `${REACTION_NAMES.cuttingWords} usadas`,
    subtitle: joinDots([...(roller ? [roller] : []), base(ctx).subtitle]),
    icon: 'record_voice_over',
    pill: null,
    text: v.effective
      ? [
          `O d${v.die?.diceSides ?? ''} saiu ${v.die?.total ?? ''} e foi subtraído da rolagem. ${cuttingSubject(v, roller, prompt?.targetLabel ?? '')}`,
        ]
      : ['Sem efeito.'],
    chips: base(ctx).used,
    note: 'O total e a CA não aparecem: só “acertou” ou “errou”.',
    throwBack: false,
  };
}

function deflectMissilesResult(
  v: ResultOf<'deflectMissiles'>,
  _asked: ReactionWindow,
  ctx: ResultContext,
): ResultView {
  const left = v.damageAfter === 0 ? 'a 0' : `para ${v.damageAfter}`;
  return {
    title: v.caught ? 'Você apanhou a flecha' : `${REACTION_NAMES.deflectMissiles} usado`,
    subtitle: base(ctx).subtitle,
    icon: 'swap_calls',
    pill: null,
    text: [
      `${v.reduction ? rollFormula(v.reduction) : ''}: o dano, de ${v.damageBefore}, caiu ${left}.${v.caught ? ' Você apanhou o projétil com a mão livre.' : ''}`,
    ],
    chips: v.throwBackAvailable ? [] : base(ctx).used,
    note: '',
    throwBack: v.throwBackAvailable,
  };
}

function maneuverReduceResult(
  v: ResultOf<'maneuverReduce'>,
  _asked: ReactionWindow,
  ctx: ResultContext,
): ResultView {
  const left = v.damageAfter === 0 ? 'a 0' : `para ${v.damageAfter}`;
  return {
    title: `${v.namePt} usada`,
    subtitle: base(ctx).subtitle,
    icon: 'shield',
    pill: null,
    text: [
      `${v.reduction ? rollFormula(v.reduction) : ''}: o dano, de ${v.damageBefore}, caiu ${left}.`,
    ],
    chips: base(ctx).used,
    note: '',
    throwBack: false,
  };
}

function featherFallResult(
  v: ResultOf<'featherFall'>,
  asked: ReactionWindow,
  ctx: ResultContext,
): ResultView {
  const prompt = asked.prompt.case === 'featherFall' ? asked.prompt.value : null;
  const names = (prompt?.falling ?? [])
    .filter((f) => v.savedIds.includes(f.combatantId))
    .map((f) => f.label);
  const many = names.length > 1;
  const who = names.length > 0 ? names.join(', ') : 'As criaturas escolhidas';
  return {
    title: `${REACTION_NAMES.featherFall} usada`,
    subtitle: base(ctx).subtitle,
    icon: 'arrow_upward',
    pill: null,
    text: [
      `${who} ${many ? 'descem' : 'desce'} devagar e não ${many ? 'sofreram' : 'sofreu'} dano de queda. A magia dura 1 minuto (ou até ${many ? 'pousarem' : 'pousar'}).`,
    ],
    chips: [...base(ctx).used, ...slotChip(ctx.after)],
    note: '',
    throwBack: false,
  };
}

const RESULTS: {
  [K in ResultCase]?: (value: ResultOf<K>, asked: ReactionWindow, ctx: ResultContext) => ResultView;
} = {
  shield: shieldResult,
  uncannyDodge: uncannyDodgeResult,
  hellishRebuke: hellishRebukeResult,
  counterspell: counterspellResult,
  cuttingWords: cuttingWordsResult,
  deflectMissiles: deflectMissilesResult,
  maneuverReduce: maneuverReduceResult,
  featherFall: featherFallResult,
};

/** The result sheet of an answered reaction: what the server's `ReactionResult` says, for the one who answered. */
export function resultView(
  res: ReactionResult,
  asked: ReactionWindow,
  ctx: ResultContext,
): ResultView {
  const r = res.result;
  const build = r.case
    ? (RESULTS[r.case] as
        ((v: unknown, a: ReactionWindow, c: ResultContext) => ResultView) | undefined)
    : undefined;
  if (build) {
    return build(r.value, asked, ctx);
  }
  return {
    title: res.used ? 'Reação usada' : 'Reação deixada passar',
    subtitle: base(ctx).subtitle,
    icon: 'check',
    pill: null,
    text: [],
    chips: res.used ? base(ctx).used : [],
    note: '',
    throwBack: false,
  };
}

/** The question of the monk's second step: throw the caught missile back, for 1 ki, as part of the same reaction. */
export function throwText(kiLeft: number, normalFt: number, longFt: number): string {
  return `Devolver o ataque? Gasta 1 de chi (${kiLeft} ${kiLeft === 1 ? 'restante' : 'restantes'}) e faz parte da mesma reação: um ataque à distância com o projétil, com proficiência, alcance ${metersText(normalFt)} e ${metersText(longFt)}.`;
}

/** How many d20 the concentration save takes: two with advantage or disadvantage (SRD 5.1), as the server rolls it. */
export type ConcentrationMode = 'normal' | 'advantage' | 'disadvantage';

/** The mode the server says the concentration save is rolled with. */
export function concentrationMode(prompt: ConcentrationSavePrompt): ConcentrationMode {
  return prompt.mode === 'advantage' || prompt.mode === 'disadvantage' ? prompt.mode : 'normal';
}

/** The mode in words, with its sources; empty when the roll is normal (nothing to say). `who` is the subject for the master ("Pensantus"); the owner reads "Você". */
export function concentrationModeLine(prompt: ConcentrationSavePrompt, who = 'Você'): string {
  const mode = concentrationMode(prompt);
  if (mode === 'normal') {
    return '';
  }
  const sources = prompt.modeSourcesPt.length > 0 ? ` (${prompt.modeSourcesPt.join('; ')})` : '';
  return `${who} tem ${mode === 'advantage' ? 'vantagem' : 'desvantagem'} neste teste${sources}. Role dois d20; vale o ${mode === 'advantage' ? 'maior' : 'menor'}.`;
}

/** The result of the concentration save, for its owner. */
export function concentrationResultView(res: ConcentrationSaveResult, round: number): ResultView {
  const spell = res.spellNamePt;
  return {
    title: res.kept ? 'Você manteve a concentração' : 'Você perdeu a concentração',
    subtitle: joinDots([spell, roundWord(round)]),
    icon: 'hourglass_empty',
    pill: { word: res.kept ? 'Concentração mantida' : 'Concentração perdida', good: res.kept },
    text: [
      `${res.save ? rollText(res.save) : ''} contra CD ${res.dc}. ${
        res.kept ? `A concentração em ${spell} continua.` : `A concentração em ${spell} acabou.`
      }`,
      // "Sortudo: 1 → 14": the d20 the halfling's Lucky rolled again; the roll above already has the new face.
      luckyText(res.save),
    ].filter(Boolean),
    chips: [],
    note: '',
    throwBack: false,
  };
}
