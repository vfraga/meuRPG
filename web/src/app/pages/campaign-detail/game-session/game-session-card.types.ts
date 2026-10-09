/**
 * The view-model and port `GameSessionCard` needs. Phase 2 maps
 * `play.v1.PlayService`'s `GameSession` messages onto `GameSessionVm` —
 * nothing below imports from `../../../../gen/...`.
 */
export interface GameSessionVm {
  /** `GameSession.id` — `EndGameSession` needs it (integrator amendment,
   * 29/09/2026). */
  readonly id: string;
  readonly sessionNumber: number;
  readonly startedAt: Date;
  readonly endedAt: Date | null;
}

/** `StartGameSession`'s response: the new session plus how many player
 * sheets it just locked (RN-01) — shown via `lockedSheetCountLabel`
 * (`core/characters/character-labels.ts`), pt-BR singular/plural included. */
export interface StartGameSessionResultVm {
  readonly session: GameSessionVm;
  readonly lockedSheetCount: number;
}

/** The kinds of choice a sheet can still lack. */
export type OpenChoiceKindVm = 'skills' | 'cantrips' | 'spellsKnown' | 'spellsPrepared';

/** A living player character whose sheet has choices open: starting a session locks it as it is. */
export interface OpenChoicesVm {
  readonly characterId: string;
  readonly name: string;
  readonly choices: readonly { readonly kind: OpenChoiceKindVm; readonly missing: number }[];
}

/**
 * The port `GameSessionCard` depends on, provided at the route level for
 * `/campaigns/:id` (`campaign-detail.routes.ts`) by `GameSessionSourceLive`,
 * which wraps the generated `PlayService` client. No root fallback: a route
 * reached without this provider fails loudly (NG0201) instead of silently
 * degrading — see `app.config.ts`.
 */
export abstract class GameSessionSource {
  /** Every session of the campaign, newest first (`ListGameSessions`, not
   * paginated): the open one has no `endedAt`. */
  abstract listSessions(campaignId: string): Promise<readonly GameSessionVm[]>;
  /** RN-01: locks every unlocked player character's sheet, in the same
   * transaction, on the server (`play.LockSheets`, plan §4). */
  /** `idempotencyKey`: one per start, sent again on a retry (a lost answer, a second tap). */
  abstract startGameSession(
    campaignId: string,
    idempotencyKey: string,
  ): Promise<StartGameSessionResultVm>;
  abstract endGameSession(campaignId: string, gameSessionId: string): Promise<GameSessionVm>;
  /** The characters that starting a session would lock with skills or spells still to choose
   * (`ListCharacters`' `open_choices`, which only the master gets). */
  abstract listOpenChoices(campaignId: string): Promise<readonly OpenChoicesVm[]>;
}
