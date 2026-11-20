import {
  type Combatant,
  type Encounter,
  MountSaveCause,
  type MountSavePrompt,
  type MountSaveResult,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { RollMode } from '../../../gen/meurpg/play/v1/combat_rolls_pb';
import { withArticle } from '../format/article';
import { dcLabel } from '../format/save-dc';
import { joinDots, upFirst } from '../format/text';
import { modifierText } from './combat-dice';
import { isPlayer, signed } from './combat-view';
import { isDismountWindow } from './reaction-dismount';
import { sheetWindow } from './reactions';
import { whoseTurn } from './whose-turn';

/**
 * The Dexterity saving throw of a rider whose mount was moved against its will, or that was knocked prone while it rode
 * (SRD 5.1, "Mounted Combat"): `Encounter.mount_save`, read by the rider's player and the master and answered with
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

/** Whether the combat asks this player a question about the mount: the rider's save to stay on it, or the dismount after its fall. */
export function owesMountQuestion(e: Encounter): boolean {
  const window = sheetWindow(e);
  return ownMountSave(e) !== undefined || (window !== undefined && isDismountWindow(window));
}

/** "de Tor" for a character, "do Wight" for a creature. */
function ofWho(c: Combatant): string {
  return isPlayer(c) ? `de ${c.label}` : withArticle(c.label, 'de');
}

/** The subject of a sentence about the rider: "Tor" for a character, "O Wight" for a creature. */
function theRider(c: Combatant): string {
  return isPlayer(c) ? c.label : upFirst(withArticle(c.label));
}

/** The sum of the d20 that counted: "15 + 2 = 17", with what a die added after it (a Bardic Inspiration die) as one more term; the bare d20 when nothing is added. */
function sumOf(r: MountSaveResult): string {
  const roll = r.save;
  if (!roll) {
    return '';
  }
  const kept = roll.faces[roll.countedIndex] ?? roll.faces[0] ?? 0;
  const dice = roll.total - kept - roll.modifier;
  const added = `${modifierText(roll.modifier)}${modifierText(dice)}`;
  return added ? `${kept}${added} = ${roll.total}` : `${kept}`;
}

/** What the sheet of the save says before the roll. */
export interface MountSaveView {
  readonly title: string;
  /** "Teste de resistência de Destreza", with "· CD 10" when the server sent the DC. */
  readonly subtitle: string;
  /** "O Cavalo de guerra sofreu um empurrão." or "Você cai da montaria." */
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

/** "O Cavalo de guerra sofreu um empurrão." or "Você cai da montaria.", for the reader (`who` is "Você" or the rider as a subject). */
function causeOf(
  p: MountSavePrompt,
  mount: Combatant | undefined,
  mine: boolean,
  who: string,
): string {
  if (p.cause === MountSaveCause.PRONE) {
    return mine ? 'Você cai da montaria.' : `${who} cai da montaria (Derrubado).`;
  }
  return mount
    ? `${upFirst(withArticle(mount.label))} sofreu um empurrão.`
    : `A ${mine ? 'sua ' : ''}montaria sofreu um empurrão.`;
}

/** "Você tem desvantagem neste teste. Role dois d20; vale o menor."; empty for a normal roll. */
function modeLineOf(mode: MountSaveView['mode'], who: string): string {
  if (mode === 'normal') {
    return '';
  }
  const adv = mode === 'advantage';
  return `${who} tem ${adv ? 'vantagem' : 'desvantagem'} neste teste. Role dois d20; vale o ${adv ? 'maior' : 'menor'}.`;
}

/** "do Cavalo de guerra", or "da montaria" for a mount the reader does not see. */
function besideOf(mount: Combatant | undefined): string {
  return mount ? withArticle(mount.label, 'de') : 'da montaria';
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
    // With the DC (a table with `show_save_dc`, or the master) the title gains it: "Você cai da montaria? · CD 10".
    title: joinDots([`${who} cai da montaria?`, ...(dc ? [dc] : [])]),
    subtitle: SAVE,
    cause: causeOf(p, mount, mine, who),
    body: mine
      ? `Passe para seguir na montaria; se falhar, você cai da montaria (Derrubado), num quadrado ao lado ${beside}.`
      : `Se passar, segue na montaria; se falhar, cai da montaria (Derrubado), num quadrado ao lado ${beside}.`,
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

/** The pill and the line of a save that was rolled. */
export interface MountSaveResultView {
  readonly word: 'Passou' | 'Falhou' | 'Falha automática';
  readonly good: boolean;
  readonly line: string;
}

/** `names.mount` is `''` when the reader does not see the mount; a character is named with no article, a creature with one. */
export function mountSaveResultView(
  r: MountSaveResult,
  mine: boolean,
  names: { readonly rider: string; readonly riderIsPlayer: boolean; readonly mount: string },
): MountSaveResultView {
  const who = mine
    ? 'você'
    : names.riderIsPlayer
      ? names.rider
      : names.rider
        ? withArticle(names.rider)
        : 'quem cavalgava';
  const beside = names.mount ? `ao lado ${withArticle(names.mount, 'de')}` : 'ao lado da montaria';
  if (r.autoFail || !r.save) {
    const fell = `${who} caiu da montaria (Derrubado) ${beside}, sem rolar.`;
    return {
      word: r.held ? 'Passou' : 'Falha automática',
      good: r.held,
      line: upFirst(fell),
    };
  }
  const dc = dcLabel(r.dc);
  const sum = `${sumOf(r)}${dc ? ` contra ${dc}` : ''}`;
  if (r.held) {
    return { word: 'Passou', good: true, line: `${sum}: ${who} segue na montaria.` };
  }
  const where = r.noRoomNextToMount
    ? `no quadrado livre mais perto: não havia lugar ${beside}`
    : beside;
  return {
    word: 'Falhou',
    good: false,
    line: `${sum}: ${who} caiu da montaria (Derrubado) ${where}.`,
  };
}

/** What the master reads while a rider's save holds the table. */
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
    rollLabel: `Rolar ${player ? `por ${rider.label}` : withArticle(rider.label, 'por')}`,
    why: `Espera o teste ${ofWho(rider)}`,
  };
}
