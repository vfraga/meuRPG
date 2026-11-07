import {
  characterBlockedMessage,
  describeCharacterError,
} from '../../core/characters/character-errors';
import { characterKindLabel } from '../../core/characters/character-labels';
import { CharacterKind, isFullSheetKind } from '../../core/characters/characters.types';
import type {
  AbilityTableVm,
  CharacterEditorMode,
  CharacterFormValue,
  CharacterForEdit,
  HitPointsMethod,
  PreviewCharacterInput,
  RulesCatalogVm,
} from './character-editor.types';

/** The NPC route's `:kind` segment (plan §5) to `CharacterKind`. */
export const NPC_ROUTE_KINDS: Record<string, CharacterKind> = {
  enemy: 'enemy',
  boss: 'boss',
  minion: 'minion',
  story: 'story',
};

export type ReadyState = {
  status: 'ready';
  mode: CharacterEditorMode;
  kind: CharacterKind;
  campaignId: string;
  characterId: string | null;
  revision: number;
  catalog: RulesCatalogVm;
  /** The sheet is locked (a session started, RN-01): the XP is no longer
   * typed here, only the master's awards change it (MR-016). */
  xpLocked: boolean;
  /** The character is reserved (MR-049): the master makes it for a player to claim, and the page says so. */
  reserved: boolean;
};

export type ErrorState = {
  status: 'error';
  message: string;
  /** Where "Voltar" goes: the sheet when editing one, else the campaign. */
  backLink: string[];
  backLabel: string;
  /** Set when nothing went wrong, but the sheet may not be edited now
   * (locked or dead): the page names that state instead of an error. */
  blocked?: { title: string };
};

export type PageState = { status: 'loading'; title: string } | ErrorState | ReadyState;

/** The page title: known from the route alone, so the loading state shows
 * it too. */
export function titleFor(mode: CharacterEditorMode, kind: CharacterKind, reserved = false): string {
  if (mode === 'edit') {
    return 'Editar ficha';
  }
  if (reserved) {
    return 'Criar personagem para um jogador';
  }
  return kind === 'player' ? 'Criar personagem' : 'Criar NPC';
}

export function loadingState(
  mode: CharacterEditorMode,
  kind: CharacterKind,
  reserved = false,
): PageState {
  return { status: 'loading', title: titleFor(mode, kind, reserved) };
}

/** A new sheet, once the lists are read. */
export function readyForCreate(
  kind: CharacterKind,
  campaignId: string,
  catalog: RulesCatalogVm,
  reserved: boolean,
): ReadyState {
  return {
    status: 'ready',
    mode: 'create',
    kind,
    campaignId,
    characterId: null,
    revision: 1,
    catalog,
    xpLocked: false,
    reserved,
  };
}

/** An existing sheet, once the lists and the sheet are read. */
export function readyForEdit(
  existing: CharacterForEdit,
  campaignId: string,
  characterId: string,
  catalog: RulesCatalogVm,
): ReadyState {
  return {
    status: 'ready',
    mode: 'edit',
    kind: existing.kind,
    campaignId,
    characterId,
    revision: existing.revision,
    catalog,
    xpLocked: existing.sheetLocked,
    reserved: existing.reserved === true,
  };
}

/** The lists of a new sheet could not be read. */
export function createFailed(err: unknown, campaignId: string): ErrorState {
  return {
    status: 'error',
    message: describeCharacterError(err),
    backLink: ['/campaigns', campaignId],
    backLabel: 'Voltar para a campanha',
  };
}

/** Say it before the form: a player who opens the edit URL of a locked sheet would otherwise fill it in and only
 * learn at "Salvar". */
export function editBlocked(
  blocked: NonNullable<CharacterForEdit['blocked']>,
  campaignId: string,
  characterId: string,
): ErrorState {
  return {
    status: 'error',
    message: characterBlockedMessage(blocked),
    backLink: ['/campaigns', campaignId, 'characters', characterId],
    backLabel: 'Voltar para a ficha',
    blocked: { title: blocked === 'character_dead' ? 'Personagem morto' : 'Ficha travada' },
  };
}

/** The sheet could not be read. */
export function editFailed(err: unknown, campaignId: string, characterId: string): ErrorState {
  return {
    status: 'error',
    message: describeCharacterError(err),
    backLink: ['/campaigns', campaignId, 'characters', characterId],
    backLabel: 'Voltar para a ficha',
  };
}

export function pageTitleOf(s: PageState): string {
  if (s.status === 'loading') {
    return s.title;
  }
  if (s.status === 'error') {
    return s.blocked?.title ?? 'Não foi possível abrir o formulário';
  }
  return titleFor(s.mode, s.kind, s.reserved);
}

/** The label of the create button: "Salvar como reservado" for a character made for a player to claim. */
export function createLabelOf(s: PageState): string {
  return s.status === 'ready' && s.reserved ? 'Salvar como reservado' : pageTitleOf(s);
}

/** Cancel goes back where the person came from: the sheet being edited, or the campaign. */
export function cancelLinkOf(s: PageState): string[] {
  if (s.status !== 'ready') {
    return ['/'];
  }
  return s.characterId
    ? ['/campaigns', s.campaignId, 'characters', s.characterId]
    : ['/campaigns', s.campaignId];
}

/** What the buttons do, in one line next to them. */
export function actionsNoteOf(s: PageState): string {
  if (s.status !== 'ready') {
    return '';
  }
  const full = isFullSheetKind(s.kind);
  if (s.mode === 'create') {
    return full
      ? 'Cria com o que estiver preenchido em todos os passos. Cancelar volta para a campanha sem criar.'
      : 'Cancelar volta para a campanha sem criar.';
  }
  return full
    ? 'Salva todos os passos de uma vez. Cancelar volta para a ficha sem salvar.'
    : 'Cancelar volta para a ficha sem salvar.';
}

/** What the buttons do on a phone, in one line.
 * The sheet's numbers come from the server, so the note says when they are calculated. */
export function phoneNoteOf(s: PageState): string {
  if (s.status !== 'ready' || !isFullSheetKind(s.kind)) {
    return actionsNoteOf(s);
  }
  return s.mode === 'create'
    ? 'CA, PV e modificadores são calculados ao criar.'
    : 'Salva todos os passos; CA e PV são recalculados.';
}

/** The line under the title: what this form is for, and that the sheet's numbers come from the server.
 * The first part is all a phone shows. The second a wide screen adds. */
export function leadPartsOf(s: PageState): { first: string; more: string } {
  if (s.status !== 'ready') {
    return { first: '', more: '' };
  }
  const kind = characterKindLabel(s.kind);
  if (s.mode === 'edit') {
    return isFullSheetKind(s.kind)
      ? {
          first: 'As mudanças valem quando você salvar.',
          more: 'Modificadores, Classe de Armadura e pontos de vida são recalculados na hora.',
        }
      : { first: `Ficha curta de ${kind.toLowerCase()}.`, more: '' };
  }
  if (s.reserved) {
    return {
      first: 'Preencha os passos na ordem que quiser.',
      more: 'O personagem fica reservado e nenhum jogador o vê até um deles assumir pelo link que você gerar. Modificadores, Classe de Armadura e pontos de vida são calculados quando você salvar.',
    };
  }
  if (s.kind === 'player') {
    return {
      first: 'Preencha os passos na ordem que quiser.',
      more: 'Modificadores, Classe de Armadura e pontos de vida são calculados quando você criar.',
    };
  }
  return {
    first: isFullSheetKind(s.kind)
      ? `${kind} com ficha completa, como a de um jogador. Só você vê os NPCs da campanha.`
      : `${kind}: ficha curta, só com o que se usa na mesa. Só você vê os NPCs da campanha.`,
    more: '',
  };
}

/** The hit points method the table leaves a new sheet, when it leaves only one: both ways stay open for the player
 * otherwise. */
export function tableHitPointsMethod(table: AbilityTableVm | null): HitPointsMethod | null {
  if (table?.hitPoints === 'roll') {
    return 'rolled';
  }
  return table?.hitPoints === 'average' ? 'average' : null;
}

/** What the server is asked to derive for the draft on screen: the hit points and the spell numbers (`PreviewCharacter`)
 * and the choices (`PreviewChoices`). A reserved character is previewed as the master's for a player. */
export function previewInputOf(
  s: ReadyState,
  full: CharacterFormValue,
  withReserved: boolean,
): PreviewCharacterInput {
  return {
    campaignId: s.campaignId,
    characterId: s.mode === 'edit' ? s.characterId : null,
    kind: s.kind,
    full,
    ...(withReserved && s.reserved && s.mode === 'create' ? { forPlayer: true } : {}),
  };
}

/** The XP a new player's sheet starts at in a campaign that levels by XP: its level's, until the person types in the
 * field; `null` where the sheet keeps the 0 (a milestone campaign, an NPC, an edit). */
export function startingXp(s: PageState, level: number): number | null {
  if (s.status !== 'ready' || s.mode !== 'create' || s.kind !== 'player') {
    return null;
  }
  const table = s.catalog.levelXp;
  if (s.catalog.xpMode !== 'enemies' || !table || table.length === 0) {
    return null;
  }
  return table[Math.min(Math.max(Math.trunc(level) || 1, 1), table.length) - 1];
}
