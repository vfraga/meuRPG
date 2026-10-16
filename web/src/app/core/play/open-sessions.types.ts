/**
 * The shapes of the open-sessions poll. Apart from `open-sessions.ts` so that the lazy client
 * (`open-sessions-client.ts`, which `open-sessions.ts` loads with `import()`) can name them without importing
 * the service back: a file importing the one that loads it is an import cycle.
 */

/** An open game session of one of the signed-in person's campaigns
 * (`PlayService.ListOpenGameSessions`), as the notice and the "Ao vivo"
 * pill need it. */
export interface OpenSessionVm {
  readonly sessionId: string;
  readonly campaignId: string;
  readonly campaignName: string;
  readonly sessionNumber: number;
  readonly startedAt: Date;
  /** The person is the campaign's master (they started it themself). */
  readonly isMaster: boolean;
}

/** One `ListOpenGameSessions` call, newest session first. */
export type OpenSessionsFetcher = () => Promise<OpenSessionVm[]>;
