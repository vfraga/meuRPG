import { Injectable, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap } from '@angular/router';
import { BehaviorSubject } from 'rxjs';

import { ABILITY_KEYS } from '../../core/characters/characters.types';
import { OpenSessions, type OpenSessionVm } from '../../shell/live-notice/open-sessions';
import { CharacterSheetPage } from './character-sheet';
import { NotesClient } from '../../core/notes/notes-client';
import { CreaturesClient } from '../../core/creatures/creatures-client';
import { XpWatcher } from './xp-watcher';
import {
  CampaignXpMode,
  CharacterSheetSource,
  CharacterSheetVm,
  CharacterStoryVm,
  FullSheetVm,
} from './character-sheet.types';

@Injectable()
class FakeCharacterSheetSource {
  getCharacterSheetFn: (campaignId: string, characterId: string) => Promise<CharacterSheetVm> =
    () => Promise.reject(new Error('not stubbed'));
  xpMode: CampaignXpMode = 'enemies';
  reads = 0;
  getMasterNotesCalls: string[] = [];
  getMasterNotesFn: (campaignId: string, characterId: string) => Promise<string> = () =>
    Promise.resolve('');
  updateMasterNotesFn: (campaignId: string, characterId: string, notes: string) => Promise<void> =
    () => Promise.resolve();
  markCharacterDeadFn: (campaignId: string, characterId: string) => Promise<CharacterSheetVm> =
    () => Promise.reject(new Error('not stubbed'));
  updateCharacterStoryFn: (
    campaignId: string,
    characterId: string,
    revision: number,
    story: CharacterStoryVm,
  ) => Promise<CharacterSheetVm> = () => Promise.reject(new Error('not stubbed'));
  setStoryEditingAllowedCalls: Array<{ id: string; allowed: boolean }> = [];
  setStoryEditingAllowedFn: (
    campaignId: string,
    characterId: string,
    allowed: boolean,
  ) => Promise<CharacterSheetVm> = () => Promise.reject(new Error('not stubbed'));

  approveCharacterFn: (campaignId: string, characterId: string) => Promise<CharacterSheetVm> = () =>
    Promise.reject(new Error('not stubbed'));
  rejectCharacterFn: (campaignId: string, characterId: string) => Promise<void> = () =>
    Promise.reject(new Error('not stubbed'));
  rejectCharacterCalls: string[] = [];

  getXpMode(): Promise<CampaignXpMode> {
    return Promise.resolve(this.xpMode);
  }
  getCharacterSheet(campaignId: string, characterId: string): Promise<CharacterSheetVm> {
    return this.getCharacterSheetFn(campaignId, characterId);
  }
  approveCharacter(campaignId: string, characterId: string): Promise<CharacterSheetVm> {
    return this.approveCharacterFn(campaignId, characterId);
  }
  rejectCharacter(campaignId: string, characterId: string): Promise<void> {
    this.rejectCharacterCalls.push(characterId);
    return this.rejectCharacterFn(campaignId, characterId);
  }
  getMasterNotes(campaignId: string, characterId: string): Promise<string> {
    this.getMasterNotesCalls.push(characterId);
    return this.getMasterNotesFn(campaignId, characterId);
  }
  updateMasterNotes(campaignId: string, characterId: string, notes: string): Promise<void> {
    return this.updateMasterNotesFn(campaignId, characterId, notes);
  }
  markCharacterDead(campaignId: string, characterId: string): Promise<CharacterSheetVm> {
    return this.markCharacterDeadFn(campaignId, characterId);
  }
  updateCharacterStory(
    campaignId: string,
    characterId: string,
    revision: number,
    story: CharacterStoryVm,
  ): Promise<CharacterSheetVm> {
    return this.updateCharacterStoryFn(campaignId, characterId, revision, story);
  }
  setStoryEditingAllowed(
    campaignId: string,
    characterId: string,
    allowed: boolean,
  ): Promise<CharacterSheetVm> {
    this.setStoryEditingAllowedCalls.push({ id: characterId, allowed });
    return this.setStoryEditingAllowedFn(campaignId, characterId, allowed);
  }
}

function emptyStory(): CharacterStoryVm {
  return {
    personality: { traits: '', ideals: '', bonds: '', flaws: '' },
    appearance: { age: '', height: '', weight: '', eyes: '', skin: '', hair: '', description: '' },
    backstory: '',
    allies: '',
  };
}

function fullSheet(overrides: Partial<FullSheetVm> = {}): FullSheetVm {
  return {
    kind: 'full',
    abilities: ABILITY_KEYS.map((key) => ({ key, score: 10, modifier: 0 })),
    proficiencyBonus: 2,
    savingThrows: [],
    skills: [],
    passivePerception: 10,
    passiveInvestigation: 10,
    passiveInsight: 10,
    initiative: 0,
    armorClass: 10,
    armorClassDescription: 'Sem armadura',
    wearsArmor: false,
    hasShield: false,
    hitPointsMax: 10,
    hitDice: '1d6',
    speedWalkFt: 25,
    senses: [],
    attacks: [],
    spellcasting: [],
    spellSlots: [],
    cantripNames: [],
    spellNames: [],
    features: [],
    languages: [],
    proficiencies: [],
    equipment: [],
    coins: { cp: 0, sp: 0, ep: 0, gp: 0, pp: 0 },
    customFeaturesText: '',
    issues: [],
    changedContent: [],
    hints: [],
    hasWildShape: false,
    contentVersion: 'srd51@test',
    ...overrides,
  };
}

function vm(overrides: Partial<CharacterSheetVm> = {}): CharacterSheetVm {
  return {
    id: 'char-1',
    campaignId: 'camp-1',
    characterKind: 'player',
    name: 'Pensantus',
    portraitImageId: '',
    state: 'draft',
    canEdit: true,
    sheetLockedAt: null,
    diedAt: null,
    revision: 1,
    sheet: fullSheet(),
    story: emptyStory(),
    canEditStory: true,
    storyEditingAllowed: false,
    canToggleStoryEditing: false,
    canMarkDead: false,
    canAccessMasterNotes: false,
    canApprove: false,
    isMaster: false,
    playerDisplayName: 'Vinicius',
    raceLabel: 'Gnomo da Rocha',
    classSummary: 'Mago 3',
    backgroundLabel: 'Sábio',
    alignmentLabel: 'Neutro e bom',
    experiencePoints: 2700,
    totalLevel: 3,
    nextLevelXp: 2700,
    canLevelUp: false,
    levelUpReason: null,
    challengeRating: '',
    xpValue: 0,
    ...overrides,
  };
}


const openSessions = signal<readonly OpenSessionVm[]>([]);
const xpWatcher = {
  follow: vi.fn<
    (
      campaignId: string | null,
      onChange: () => void,
      onCreatures?: () => void,
      onContent?: () => void,
    ) => void
  >(),
};
const notesApi = {
  list: vi.fn(() => Promise.resolve({ notes: [], noteCount: 0, maxNotes: 300 })),
  scenes: vi.fn(() => Promise.resolve([])),
};

interface Deferred<T> {
  promise: Promise<T>;
  resolve: (v: T) => void;
}
function deferred<T>(): Deferred<T> {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((r) => (resolve = r));
  return { promise, resolve };
}
function flush(): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, 0));
}
function buttonWithText(el: HTMLElement, text: string): HTMLButtonElement | undefined {
  return Array.from(el.querySelectorAll('button')).find((b) => b.textContent?.trim() === text);
}

describe('Review15 U15-3: character sheet has no sequence guard on its async reads', () => {
  let fake: FakeCharacterSheetSource;
  let params: BehaviorSubject<ReturnType<typeof convertToParamMap>>;

  function configure(): void {
    params = new BehaviorSubject(convertToParamMap({ id: 'camp-1', characterId: 'char-1' }));
    TestBed.configureTestingModule({
      imports: [CharacterSheetPage],
      providers: [
        { provide: CharacterSheetSource, useClass: FakeCharacterSheetSource },
        { provide: ActivatedRoute, useValue: { paramMap: params } },
        { provide: NotesClient, useValue: notesApi },
        {
          provide: CreaturesClient,
          useValue: {
            list: () => Promise.resolve([]),
            summonOptions: () => Promise.resolve({ spells: [], slots: [] }),
            statBlock: () => Promise.reject(new Error('none')),
          },
        },
        { provide: OpenSessions, useValue: { sessions: openSessions } },
        { provide: XpWatcher, useValue: xpWatcher },
      ],
    });
    fake = TestBed.inject(CharacterSheetSource) as unknown as FakeCharacterSheetSource;
  }

  function openSession(): void {
    openSessions.set([
      {
        sessionId: 's1',
        campaignId: 'camp-1',
        campaignName: 'Mirathel',
        sessionNumber: 5,
        startedAt: new Date(),
        isMaster: true,
      },
    ]);
  }

  afterEach(() => {
    openSessions.set([]);
    xpWatcher.follow.mockClear();
  });

  it('(a) an older reloadQuietly answer (alive) must not overwrite the dead vm that markDead already set', async () => {
    configure();
    openSession();
    const alive = vm({ state: 'locked', isMaster: true, canMarkDead: true, canEdit: true });
    const dead = vm({
      state: 'dead',
      isMaster: true,
      canMarkDead: false,
      canEdit: false,
      diedAt: new Date(2026, 8, 30, 21, 0),
    });
    const reload = deferred<CharacterSheetVm>();
    let reads = 0;
    fake.getCharacterSheetFn = () => (++reads === 1 ? Promise.resolve(alive) : reload.promise);
    fake.markCharacterDeadFn = () => Promise.resolve(dead);

    const fixture = TestBed.createComponent(CharacterSheetPage);
    fixture.detectChanges();
    await flush();
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;

    // A stream hint starts a slow reload.
    const onChange = xpWatcher.follow.mock.calls.at(-1)![1];
    onChange();
    expect(reads).toBe(2);

    // Meanwhile the master confirms the death, which answers first.
    buttonWithText(el, 'Marcar como morto')!.click();
    fixture.detectChanges();
    buttonWithText(el, 'Confirmar morte')!.click();
    await flush();
    fixture.detectChanges();
    expect(el.querySelector('[aria-label="Estado do personagem"]')?.textContent).toContain('Morto');

    // The older reload (taken before the death) lands late, still alive.
    reload.resolve(alive);
    await flush();
    fixture.detectChanges();

    expect(el.querySelector('[aria-label="Estado do personagem"]')?.textContent).toContain('Morto');
    expect(el.textContent).toContain('Morreu em 30/09/2026');
    expect(buttonWithText(el, 'Marcar como morto')).toBeUndefined();
  });

  it('(b) after navigating A -> B, a late load() answer for A must not overwrite the sheet of B', async () => {
    configure();
    const a = deferred<CharacterSheetVm>();
    const b = deferred<CharacterSheetVm>();
    fake.getCharacterSheetFn = (_c, id) => (id === 'char-A' ? a.promise : b.promise);
    params.next(convertToParamMap({ id: 'camp-1', characterId: 'char-A' }));

    const fixture = TestBed.createComponent(CharacterSheetPage);
    fixture.detectChanges();
    // Same component instance, new param (the router reuses it for the same route).
    params.next(convertToParamMap({ id: 'camp-1', characterId: 'char-B' }));
    b.resolve(vm({ id: 'char-B', name: 'Bravo' }));
    await flush();
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    expect(el.textContent).toContain('Bravo');

    a.resolve(vm({ id: 'char-A', name: 'Alfa' }));
    await flush();
    fixture.detectChanges();

    expect(el.textContent).toContain('Bravo');
    expect(el.textContent).not.toContain('Alfa');
  });

  it('(b2) after navigating A -> B, a late reloadQuietly answer for A must not overwrite the sheet of B', async () => {
    configure();
    openSession();
    const reloadA = deferred<CharacterSheetVm>();
    fake.getCharacterSheetFn = (_c, id) => {
      if (id === 'char-B') return Promise.resolve(vm({ id: 'char-B', name: 'Bravo' }));
      return fake.reads++ === 0
        ? Promise.resolve(vm({ id: 'char-A', name: 'Alfa' }))
        : reloadA.promise;
    };

    const fixture = TestBed.createComponent(CharacterSheetPage);
    fixture.detectChanges();
    await flush();
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    expect(el.textContent).toContain('Alfa');

    xpWatcher.follow.mock.calls.at(-1)![1](); // reload for A starts, slow
    params.next(convertToParamMap({ id: 'camp-1', characterId: 'char-B' }));
    await flush();
    fixture.detectChanges();
    expect(el.textContent).toContain('Bravo');

    reloadA.resolve(vm({ id: 'char-A', name: 'Alfa' }));
    await flush();
    fixture.detectChanges();

    expect(el.textContent).toContain('Bravo');
    expect(el.textContent).not.toContain('Alfa');
  });
});
