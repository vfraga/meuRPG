import {
  type ConcentrationSaveResult,
  type ReactionResult,
  ReactionRollKind,
  type ReactionWindow,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { joinDots, upFirst } from '../format/text';
import { circleLabel } from './combat-grid';
import { rollFormula, rollText } from './combat-dice';
import { luckyText } from './lucky';
import { ofThe } from './move-plan';
import {
  REACTION_NAMES,
  ResultView,
  SlotAfter,
  ofRollKind,
  outcomeWord,
  roundWord,
  slotChip,
  theName,
} from './reaction-view';

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
        ? `Teste de Destreza ${ofThe([prompt?.aggressorLabel ?? ''])}: ${rollText(v.save)} contra CD ${v.saveDc}. ${upFirst(who)} ${outcome}: ${v.damage} de dano.`
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
