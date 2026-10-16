import {
  type ConcentrationSavePrompt,
  type HellishRebukeOption,
  type ReactionWindow,
} from '../../../gen/meurpg/play/v1/combat_pb';
import type { SlotChoice } from '../../../gen/meurpg/rules/v1/rules_pb';
import { joinDots, upFirst } from '../format/text';
import { metersText } from '../units';
import { circleLabel } from './combat-grid';
import { freeText } from './cast-flow';
import {
  PromptView,
  REACTION_NAMES,
  REACTION_SPENT,
  ofRollKind,
  reach,
  reachChip,
  roundWord,
  theName,
} from './reaction-view';

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
    title: `${upFirst(theName(c.rollerLabel))} ${what.verb}`,
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
    ariaLabel: `${upFirst(theName(c.rollerLabel))} ${what.verb}: usar ${name}?`,
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
