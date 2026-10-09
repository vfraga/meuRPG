import { Injectable, inject } from '@angular/core';
import { createClient } from '@connectrpc/connect';
import { timestampDate } from '@bufbuild/protobuf/wkt';

import {
  CharacterService,
  OpenChoiceKind,
} from '../../../../gen/meurpg/characters/v1/characters_pb';
import { GameSession, PlayService } from '../../../../gen/meurpg/play/v1/play_pb';
import { CONNECT_TRANSPORT } from '../../../core/connect/transport';
import {
  GameSessionSource,
  GameSessionVm,
  OpenChoiceKindVm,
  OpenChoicesVm,
  StartGameSessionResultVm,
} from './game-session-card.types';

const OPEN_CHOICE_KINDS: Partial<Record<OpenChoiceKind, OpenChoiceKindVm>> = {
  [OpenChoiceKind.SKILLS]: 'skills',
  [OpenChoiceKind.CANTRIPS]: 'cantrips',
  [OpenChoiceKind.SPELLS_KNOWN]: 'spellsKnown',
  [OpenChoiceKind.SPELLS_PREPARED]: 'spellsPrepared',
};

function toVm(gameSession: GameSession): GameSessionVm {
  return {
    id: gameSession.id,
    sessionNumber: gameSession.sessionNumber,
    startedAt: gameSession.startedAt ? timestampDate(gameSession.startedAt) : new Date(0),
    endedAt: gameSession.endedAt ? timestampDate(gameSession.endedAt) : null,
  };
}

/**
 * `GameSessionSource` over the generated `PlayService` client
 * (`meurpg.play.v1`, phase 2). Provided at the route level for
 * `/campaigns/:id` — see `../campaign-detail.routes.ts` — so this client
 * stays out of the eager bundle.
 */
@Injectable()
export class GameSessionSourceLive implements GameSessionSource {
  private readonly client = createClient(PlayService, inject(CONNECT_TRANSPORT));
  private readonly characters = createClient(CharacterService, inject(CONNECT_TRANSPORT));

  async listSessions(campaignId: string): Promise<readonly GameSessionVm[]> {
    const res = await this.client.listGameSessions({ campaignId });
    return res.gameSessions.map(toVm);
  }

  async startGameSession(
    campaignId: string,
    idempotencyKey: string,
  ): Promise<StartGameSessionResultVm> {
    const res = await this.client.startGameSession({ campaignId, idempotencyKey });
    return { session: toVm(res.gameSession!), lockedSheetCount: res.lockedSheetCount };
  }

  async endGameSession(campaignId: string, gameSessionId: string): Promise<GameSessionVm> {
    const res = await this.client.endGameSession({ campaignId, gameSessionId });
    return toVm(res.gameSession!);
  }

  async listOpenChoices(campaignId: string): Promise<readonly OpenChoicesVm[]> {
    const res = await this.characters.listCharacters({ campaignId });
    return res.characters
      .filter((c) => c.openChoices.length > 0)
      .map((c) => ({
        characterId: c.id,
        name: c.name,
        choices: c.openChoices.flatMap((o) => {
          const kind = OPEN_CHOICE_KINDS[o.kind];
          return kind ? [{ kind, missing: o.missing }] : [];
        }),
      }));
  }
}
