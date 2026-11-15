import {
  type Combatant,
  type Encounter,
  MountSaveCause,
  type MountSavePrompt,
  type MountSaveResult,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { RollMode } from '../../../gen/meurpg/play/v1/combat_rolls_pb';
import { article } from '../format/article';
import { dcLabel } from '../format/save-dc';
import { modifierText } from './combat-dice';
import { isPlayer, signed } from './combat-view';
import { whoseTurn } from './reaction-dismount';

/**
 * The Dexterity saving throw of a rider whose mount was moved against its will, or that was knocked prone while it rode
 * (#588; SRD 5.1, "Mounted Combat"): `Encounter.mount_save`, read by the rider's player and the master and answered with
 * `RollMountSave`. Everything written here comes from the prompt and the result: the DC is shown only when the server sent
 * one (a player gets it only on a table with `show_save_dc`, RN-24), and a mount the player does not see is never named.
 * Pure functions, tested without a DOM.
 */

/** The conditions that fail a Dexterity saving throw by themselves (SRD 5.1, Conditions), as the server keys them. */
const AUTO_FAILS: readonly string[] = [
  'condition:paralyzed',
  'condition:stunned',
  'condition:unconscious',
  'condition:petrified',
];

const SAVE = 'Teste de resistência de Destreza';

/** The save the player owes: the one of a rider that is theirs. */
export function ownMountSave(e: Encounter): MountSavePrompt | undefined {
  const p = e.mountSave;
  return p && e.combatants.some((c) => c.id === p.riderId && c.mine) ? p : undefined;
}

/** "de Tor" for a character, "do Wight" for a creature. */
function ofWho(c: Combatant): string {
  return isPlayer(c) ? `de ${c.label}` : `d${article(c.label)} ${c.label}`;
}

/** The subject of a sentence about the rider: "Tor" for a character, "O Wight" for a creature. */
function theRider(c: Combatant): string {
  return isPlayer(c) ? c.label : `${article(c.label) === 'a' ? 'A' : 'O'} ${c.label}`;
}

/** The sum of the d20 that counted: "15 + 2 = 17", with what a die added after it (a Bardic Inspiration die) as one more term. */
function sumOf(r: MountSaveResult): string {
  const roll = r.save;
  if (!roll) {
    return '';
  }
  const kept = roll.faces[roll.countedIndex] ?? roll.faces[0] ?? 0;
  const dice = roll.total - kept - roll.modifier;
  return `${kept}${modifierText(roll.modifier)}${modifierText(dice)} = ${roll.total}`;
}

/** "empurrado" or "empurrada", by the name. */
function ending(label: string): string {
  return article(label) === 'a' ? 'a' : 'o';
}

/** What the sheet of the save says before the roll (board 588-06). */
export interface MountSaveView {
  readonly title: string;
  /** "Teste de resistência de Destreza", with "· CD 10" when the server sent the DC. */
  readonly subtitle: string;
  /** "O Cavalo de guerra foi empurrado." or "Você foi derrubado." */
  readonly cause: string;
  /** What a pass keeps and what a failure does. */
  readonly body: string;
  /** "1d20 + 2", "Destreza +2" and, when it is shown, "CD 10". */
  readonly tags: readonly string[];
  /** "A vez do Wight espera este teste." */
  readonly wait: string;
  readonly mode: 'normal' | 'advantage' | 'disadvantage';
  /** The mode in words; empty when the roll is normal. */
  readonly modeLine: string;
  readonly autoFail: boolean;
  /** Why the rider fails with no d20: "Paralisado: cai sem rolar." */
  readonly autoFailLine: string;
  /** The number added to the typed d20 in its live total. */
  readonly bonus: number;
  /** "Rolar no app", or "Continuar" for a save with no d20. */
  readonly rollLabel: string;
}

function modeOf(p: MountSavePrompt): MountSaveView['mode'] {
  if (p.mode === RollMode.ADVANTAGE) {
    return 'advantage';
  }
  return p.mode === RollMode.DISADVANTAGE ? 'disadvantage' : 'normal';
}

/** The condition that fails the save, as the rider's row names it; `''` when the viewer reads none. */
function failingCondition(rider: Combatant | undefined): string {
  const i = rider?.conditions.findIndex((k) => AUTO_FAILS.includes(k)) ?? -1;
  return i >= 0 ? (rider?.conditionNamesPt[i] ?? '') : '';
}

function tagsOf(p: MountSavePrompt, mode: MountSaveView['mode']): string[] {
  if (p.autoFail) {
    return ['Falha automática'];
  }
  const dice = mode === 'normal' ? '1d20' : `2d20 (${mode === 'advantage' ? 'maior' : 'menor'})`;
  const dc = dcLabel(p.dc);
  return [
    `${dice}${modifierText(p.saveBonus)}`,
    p.bonusKnown ? `Destreza ${signed(p.saveBonus)}` : 'Sem bônus na ficha',
    ...(dc ? [dc] : []),
  ];
}

/** "O Cavalo de guerra foi empurrado." or "Você foi derrubado.", for the reader (`who` is "Você" or the rider as a subject). */
function causeOf(
  p: MountSavePrompt,
  mount: Combatant | undefined,
  rider: Combatant | undefined,
  mine: boolean,
  who: string,
): string {
  if (p.cause === MountSaveCause.PRONE) {
    return mine ? 'Você foi derrubado.' : `${who} foi derrubad${ending(rider?.label ?? '')}.`;
  }
  return mount
    ? `${article(mount.label) === 'a' ? 'A' : 'O'} ${mount.label} foi empurrad${ending(mount.label)}.`
    : `A ${mine ? 'sua ' : ''}montaria foi empurrada.`;
}

/** "Você tem desvantagem neste teste. Role dois d20; vale o menor."; empty for a normal roll. */
function modeLineOf(mode: MountSaveView['mode'], who: string): string {
  if (mode === 'normal') {
    return '';
  }
  const adv = mode === 'advantage';
  return `${who} tem ${adv ? 'vantagem' : 'desvantagem'} neste teste. Role dois d20; vale o ${adv ? 'maior' : 'menor'}.`;
}

/** "dele", "dela", or "da montaria" for a mount the reader does not see. */
function besideOf(mount: Combatant | undefined): string {
  if (!mount) {
    return 'da montaria';
  }
  return article(mount.label) === 'a' ? 'dela' : 'dele';
}

/** `mine`: the reader is the rider's player ("Você"); the master reads the rider by name. */
export function mountSaveView(e: Encounter, p: MountSavePrompt, mine: boolean): MountSaveView {
  const rider = e.combatants.find((c) => c.id === p.riderId);
  const mount = e.combatants.find((c) => c.id === p.mountId && p.mountId !== '');
  const name = rider ? theRider(rider) : 'O cavaleiro';
  const who = mine ? 'Você' : name;
  const mode = modeOf(p);
  const dc = dcLabel(p.dc);
  const beside = besideOf(mount);
  const condition = failingCondition(rider);
  return {
    title: `${who} cai da montaria?`,
    subtitle: dc ? `${SAVE} · ${dc}` : SAVE,
    cause: causeOf(p, mount, rider, mine, who),
    body: mine
      ? `Passe para continuar montado; se falhar, você cai Derrubado num quadrado ao lado ${beside}.`
      : `Se passar, continua montado; se falhar, cai Derrubado num quadrado ao lado ${beside}.`,
    tags: tagsOf(p, mode),
    wait: `${whoseTurn(e, mine ? p.riderId : '')} espera este teste.`,
    mode,
    modeLine: modeLineOf(mode, who),
    autoFail: p.autoFail,
    autoFailLine: condition
      ? `${condition}: cai sem rolar.`
      : `${who} não consegue se segurar: cai sem rolar.`,
    bonus: p.saveBonus,
    rollLabel: p.autoFail ? 'Continuar' : 'Rolar no app',
  };
}

/** The pill and the line of a save that was rolled (board 588-10, card 6). */
export interface MountSaveResultView {
  readonly word: 'Passou' | 'Falhou' | 'Falha automática';
  readonly good: boolean;
  readonly line: string;
}

/** `names.mount` is `''` when the reader does not see the mount. */
export function mountSaveResultView(
  r: MountSaveResult,
  mine: boolean,
  names: { readonly rider: string; readonly mount: string },
): MountSaveResultView {
  const who = mine ? 'você' : names.rider;
  const beside = names.mount
    ? `ao lado d${article(names.mount)} ${names.mount}`
    : 'ao lado da montaria';
  if (r.autoFail || !r.save) {
    const fell = `${who} caiu Derrubado ${beside}, sem rolar.`;
    return {
      word: r.held ? 'Passou' : 'Falha automática',
      good: r.held,
      line: `${fell.charAt(0).toUpperCase()}${fell.slice(1)}`,
    };
  }
  const dc = dcLabel(r.dc);
  const sum = `${sumOf(r)}${dc ? ` contra ${dc}` : ''}`;
  if (r.held) {
    return { word: 'Passou', good: true, line: `${sum}: ${who} continua montado.` };
  }
  const where = r.noRoomNextToMount
    ? `no quadrado livre mais perto: não havia lugar ${beside}`
    : beside;
  return { word: 'Falhou', good: false, line: `${sum}: ${who} caiu Derrubado ${where}.` };
}

/** What the master reads while a rider's save holds the table (board 588-08). */
export interface MountSaveWait {
  readonly prompt: MountSavePrompt;
  /** "Esperando o teste de Tor". */
  readonly title: string;
  /** The save, the DC, the cause and who answers. */
  readonly detail: string;
  /** "Rolar por Tor". */
  readonly rollLabel: string;
  /** "Espera o teste de Tor": the ink line under the dashed "Próximo turno". */
  readonly why: string;
}

export function mountSaveWait(e: Encounter): MountSaveWait | null {
  const p = e.mountSave;
  const rider = p ? e.combatants.find((c) => c.id === p.riderId) : undefined;
  if (!p || !rider) {
    return null;
  }
  const v = mountSaveView(e, p, false);
  // A sentence that opens with an article goes on in lower case after the colon; a name keeps its capital.
  const cause = v.cause.replace(/^[OA] /, (m) => m.toLowerCase());
  const dc = dcLabel(p.dc);
  const player = isPlayer(rider);
  return {
    prompt: p,
    title: `Esperando o teste ${ofWho(rider)}`,
    detail: `${SAVE}${dc ? ` ${dc}` : ''}: ${cause} ${
      player ? `${rider.label} está respondendo no celular.` : 'O teste é seu.'
    }`,
    rollLabel: `Rolar ${player ? `por ${rider.label}` : `pel${article(rider.label)} ${rider.label}`}`,
    why: `Espera o teste ${ofWho(rider)}`,
  };
}
