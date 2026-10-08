import { Injectable, inject } from '@angular/core';
import type { MessageInitShape } from '@bufbuild/protobuf';
import { createClient } from '@connectrpc/connect';

import {
  type CreatePuzzleRequestSchema,
  type MasterPuzzleRun,
  type Puzzle,
  type PuzzleMoveSchema,
  type PuzzleRun,
  PuzzleService,
  type PuzzleSummary,
  type PreviewPuzzleStartResponse,
  type CipherSolutionSchema,
  type TryPuzzleHintResponse,
} from '../../../gen/meurpg/play/v1/puzzles_pb';
import { CONNECT_TRANSPORT } from '../connect/transport';

/** What the master writes about a puzzle, as `CreatePuzzle` takes it (`UpdatePuzzle` takes the same, with the ID). */
export type PuzzleInit = Omit<
  MessageInitShape<typeof CreatePuzzleRequestSchema>,
  'campaignId' | '$typeName'
>;

/** How the hint's d20 comes (RN-18): the app rolls it, or the player typed the face of a real die. */
export type HintDie = { readonly inApp: true } | { readonly face: number };

/** What a move answers: the run as the player reads it now, and whether this move solved the puzzle. */
export interface MoveAnswer {
  readonly run: PuzzleRun;
  readonly replayed: boolean;
  readonly solvedByThisMove: boolean;
  /** This move was judged wrong: the server's verdict, also on a replay of it. */
  readonly wrong: boolean;
}

/**
 * Thin wrapper around the generated `PuzzleService` client (MR-038, RN-27), in the same shape as
 * `MapsClient`. Callers map errors to Portuguese (`puzzleErrorMessage`). `providedIn: 'root'`, and only lazy code
 * imports it, so the generated puzzle code stays out of the initial bundle.
 *
 * `move` takes the idempotency key from the caller: a retry sends the same key, and the server then answers with
 * the run as it is now without playing the move again (`MovesSender` makes the key and does the retry).
 */
@Injectable({ providedIn: 'root' })
export class PuzzlesClient {
  private readonly client = createClient(PuzzleService, inject(CONNECT_TRANSPORT));

  async list(campaignId: string, includeArchived = false): Promise<Puzzle[]> {
    return (await this.client.listPuzzles({ campaignId, includeArchived })).puzzles;
  }

  async get(campaignId: string, puzzleId: string): Promise<Puzzle> {
    return need((await this.client.getPuzzle({ campaignId, puzzleId })).puzzle, 'GetPuzzle');
  }

  /** `idempotencyKey`: one per new puzzle, sent again on a retry (see `ActionKey`). */
  async create(campaignId: string, init: PuzzleInit, idempotencyKey: string): Promise<Puzzle> {
    return need(
      (await this.client.createPuzzle({ ...init, campaignId, idempotencyKey })).puzzle,
      'CreatePuzzle',
    );
  }

  async update(campaignId: string, puzzleId: string, init: PuzzleInit): Promise<Puzzle> {
    return need(
      (await this.client.updatePuzzle({ ...init, campaignId, puzzleId })).puzzle,
      'UpdatePuzzle',
    );
  }

  /** The start the form shows, and the fewest moves from it; `seed` 0 draws a new one. */
  previewStart(
    campaignId: string,
    config: PuzzleInit['config'],
    solution: PuzzleInit['solution'],
    seed: bigint,
  ): Promise<PreviewPuzzleStartResponse> {
    return this.client.previewPuzzleStart({ campaignId, config, solution, seed });
  }

  /** The message as the players will read it ("Como os jogadores a veem"): the server ciphers it, the browser never does. */
  async previewCipher(
    campaignId: string,
    solution: MessageInitShape<typeof CipherSolutionSchema>,
  ): Promise<string> {
    return (await this.client.previewPuzzleCipher({ campaignId, solution })).ciphertext;
  }

  async archive(campaignId: string, puzzleId: string): Promise<Puzzle> {
    return need(
      (await this.client.archivePuzzle({ campaignId, puzzleId })).puzzle,
      'ArchivePuzzle',
    );
  }

  async unarchive(campaignId: string, puzzleId: string): Promise<Puzzle> {
    return need(
      (await this.client.unarchivePuzzle({ campaignId, puzzleId })).puzzle,
      'UnarchivePuzzle',
    );
  }

  // The master's session.

  async listSession(campaignId: string): Promise<MasterPuzzleRun[]> {
    return (await this.client.listSessionPuzzles({ campaignId })).puzzles;
  }

  async masterRun(campaignId: string, puzzleId: string): Promise<MasterPuzzleRun> {
    return need(
      (await this.client.getMasterPuzzleRun({ campaignId, puzzleId })).run,
      'GetMasterPuzzleRun',
    );
  }

  async show(campaignId: string, puzzleId: string): Promise<MasterPuzzleRun> {
    return need((await this.client.showPuzzle({ campaignId, puzzleId })).run, 'ShowPuzzle');
  }

  /** The master's actions on a shown puzzle send `expectedRevision`, the revision of the run the card shows: when the
   * run is at another one, the server changes nothing and refuses with `STALE_REVISION` (0 makes no check). */
  async reset(
    campaignId: string,
    puzzleId: string,
    expectedRevision = 0,
  ): Promise<MasterPuzzleRun> {
    return need(
      (await this.client.resetPuzzle({ campaignId, puzzleId, expectedRevision })).run,
      'ResetPuzzle',
    );
  }

  async reseed(
    campaignId: string,
    puzzleId: string,
    expectedRevision = 0,
  ): Promise<MasterPuzzleRun> {
    return need(
      (await this.client.reseedPuzzle({ campaignId, puzzleId, expectedRevision })).run,
      'ReseedPuzzle',
    );
  }

  async close(campaignId: string, puzzleId: string): Promise<MasterPuzzleRun> {
    return need((await this.client.closePuzzle({ campaignId, puzzleId })).run, 'ClosePuzzle');
  }

  async releaseHint(
    campaignId: string,
    puzzleId: string,
    expectedRevision = 0,
  ): Promise<MasterPuzzleRun> {
    return need(
      (await this.client.releaseNextPuzzleHint({ campaignId, puzzleId, expectedRevision })).run,
      'ReleaseNextPuzzleHint',
    );
  }

  /** "Tocar a sequência": every player's phone shows it step by step. */
  async playSequence(
    campaignId: string,
    puzzleId: string,
    expectedRevision = 0,
  ): Promise<MasterPuzzleRun> {
    return need(
      (await this.client.playPuzzleSequence({ campaignId, puzzleId, expectedRevision })).run,
      'PlayPuzzleSequence',
    );
  }

  // The player's session (the master may read what a player reads).

  async listShown(campaignId: string): Promise<PuzzleSummary[]> {
    return (await this.client.listShownPuzzles({ campaignId })).puzzles;
  }

  async run(campaignId: string, puzzleId: string): Promise<PuzzleRun> {
    return need((await this.client.getPuzzleRun({ campaignId, puzzleId })).run, 'GetPuzzleRun');
  }

  async move(
    campaignId: string,
    puzzleId: string,
    move: MessageInitShape<typeof PuzzleMoveSchema>,
    idempotencyKey: string,
  ): Promise<MoveAnswer> {
    const res = await this.client.makePuzzleMove({ campaignId, puzzleId, move, idempotencyKey });
    return {
      run: need(res.run, 'MakePuzzleMove'),
      replayed: res.replayed,
      solvedByThisMove: res.solvedByThisMove,
      wrong: res.wrong,
    };
  }

  /** "Tentar uma dica": the d20 rolls in the app, or is the face of a real die the player typed (RN-18). */
  async tryHint(
    campaignId: string,
    puzzleId: string,
    die: HintDie,
    idempotencyKey: string,
  ): Promise<TryPuzzleHintResponse> {
    return this.client.tryPuzzleHint({
      campaignId,
      puzzleId,
      idempotencyKey,
      roll:
        'inApp' in die ? { case: 'rollInApp', value: true } : { case: 'd20Face', value: die.face },
    });
  }
}

function need<T>(value: T | undefined, call: string): T {
  if (value === undefined) {
    throw new Error(`${call} answered without its payload`);
  }
  return value;
}
