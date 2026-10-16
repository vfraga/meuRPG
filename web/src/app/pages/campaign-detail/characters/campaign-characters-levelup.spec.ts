import { Injectable, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { create } from '@bufbuild/protobuf';
import { timestampFromMs } from '@bufbuild/protobuf/wkt';

import {
  LevelUpChoicesSchema,
  LevelUpSchema,
} from '../../../../gen/meurpg/characters/v1/characters_pb';
import { LevelUpClient } from '../../../core/levelup/levelup-client';
import { LevelUpFeed } from '../../../core/levelup/levelup-feed';
import { OpenSessions } from '../../../core/play/open-sessions';
import { XpWatcher } from '../../../core/play/xp-watcher';
import { CampaignCharacters } from './campaign-characters';
import { CampaignCharactersSource, type CampaignCharactersVm } from './campaign-characters.types';

@Injectable()
class Source {
  openChoices() {
    return Promise.resolve([]);
  }
  listCharacters(): Promise<CampaignCharactersVm> {
    return Promise.resolve({
      playerCharacters: [
        {
          id: 'p1',
          name: 'Pensantus',
          kind: 'player',
          state: 'locked',
          classSummary: 'Mago 4',
          playerDisplayName: 'Vinicius',
        },
      ],
      npcs: [],
      hasLivingCharacter: false,
    });
  }
}

const levelUp = (ago: number) =>
  create(LevelUpSchema, {
    id: `l${ago}`,
    characterId: 'p1',
    characterName: 'Pensantus',
    toLevel: 4,
    createdAt: timestampFromMs(Date.now() - ago),
    choices: create(LevelUpChoicesSchema, {}),
  });

describe("the master's characters list and the stream (MR-040)", () => {
  const list = vi.fn();
  const follow = vi.fn();

  async function setup() {
    list.mockReset().mockResolvedValue({ levelUps: [], nextPageToken: '' });
    follow.mockReset();
    TestBed.configureTestingModule({
      imports: [CampaignCharacters],
      providers: [
        provideRouter([]),
        LevelUpFeed,
        { provide: CampaignCharactersSource, useClass: Source },
        { provide: LevelUpClient, useValue: { list } },
        // The campaign has an open session, so the page follows its stream.
        { provide: OpenSessions, useValue: { sessions: signal([{ campaignId: 'camp-1' }]) } },
        { provide: XpWatcher, useValue: { follow } },
      ],
    });
    const fixture = TestBed.createComponent(CampaignCharacters);
    fixture.componentRef.setInput('campaignId', 'camp-1');
    fixture.componentRef.setInput('isMaster', true);
    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();
    return fixture;
  }

  it('follows the open session and, on `xp_changed`, reads the level-ups again: "Subiu para o nível 4" appears', async () => {
    const f = await setup();
    const el = f.nativeElement as HTMLElement;
    expect(follow).toHaveBeenCalledWith('camp-1', expect.any(Function), expect.any(Function));
    expect(el.textContent).not.toContain('Subiu para o nível 4');

    // The player confirms a level-up: the stream says `xp_changed`, the callback reads again.
    list.mockResolvedValue({ levelUps: [levelUp(1000)], nextPageToken: '' });
    follow.mock.calls.at(-1)?.[1]();
    await f.whenStable();
    await TestBed.inject(LevelUpFeed).refresh();
    f.detectChanges();

    expect(el.textContent).toContain('Subiu para o nível 4');
    expect(el.querySelector('button.changes-toggle')).not.toBeNull();
  });

  it('opens the "O que mudou" of the character the status line points at, and focuses it', async () => {
    list.mockReset();
    const f = await setup();
    list.mockResolvedValue({ levelUps: [levelUp(1000)], nextPageToken: '' });
    await TestBed.inject(LevelUpFeed).refresh();
    f.detectChanges();
    Element.prototype.scrollIntoView = vi.fn();
    TestBed.inject(LevelUpFeed).revealCharacter('p1');
    f.detectChanges();
    await f.whenStable();
    f.detectChanges();
    expect((f.nativeElement as HTMLElement).textContent).toContain(
      'O que Pensantus escolheu no nível 4',
    );
  });
});
