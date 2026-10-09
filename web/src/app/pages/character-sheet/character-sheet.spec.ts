import { Injectable, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { By } from '@angular/platform-browser';
import { ActivatedRoute, Router, convertToParamMap } from '@angular/router';
import { Code, ConnectError } from '@connectrpc/connect';
import { BehaviorSubject, of } from 'rxjs';

import { ABILITY_KEYS } from '../../core/characters/characters.types';
import { OpenSessions, type OpenSessionVm } from '../../shell/live-notice/open-sessions';
import { CharacterSheetPage } from './character-sheet';
import { NotesClient } from '../../core/notes/notes-client';
import { CreaturesClient } from '../../core/creatures/creatures-client';
import { CreaturesPanel } from './creatures-panel/creatures-panel';
import { XpWatcher } from './xp-watcher';
import {
  BasicSheetVm,
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
    pactSlots: null,
    cantripNames: [],
    spellNames: [],
    features: [],
    languages: [],
    proficiencies: [],
    equipment: [],
    backgroundEquipment: '',
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

function basicSheet(overrides: Partial<BasicSheetVm> = {}): BasicSheetVm {
  return {
    kind: 'basic',
    hitPointsMax: 7,
    armorClass: 12,
    speedWalkFt: 30,
    initiativeBonus: 2,
    attacks: [
      {
        name: 'Cimitarra',
        attackBonus: 4,
        damageDiceCount: 1,
        damageDiceSides: 6,
        damageBonus: 2,
        damageType: 'slashing',
      },
      {
        name: 'Arco curto',
        attackBonus: 4,
        damageDiceCount: 2,
        damageDiceSides: 4,
        damageBonus: -1,
        damageType: 'piercing',
      },
    ],
    legacyDamage: '',
    description: 'Um goblin arisco.',
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

/** What the XP block listens with: no open session, so no stream. */
const openSessions = signal<readonly OpenSessionVm[]>([]);
const xpWatcher = {
  follow:
    vi.fn<
      (
        campaignId: string | null,
        onChange: () => void,
        onCreatures?: () => void,
        onContent?: () => void,
        onForm?: (characterId: string | null) => void,
      ) => void
    >(),
};
/** The player's notes panel reads this; nothing here talks to a server. */
const notesApi = {
  list: vi.fn(() => Promise.resolve({ notes: [], noteCount: 0, maxNotes: 300 })),
  scenes: vi.fn(() => Promise.resolve([])),
};
function xpProviders() {
  return [
    { provide: NotesClient, useValue: notesApi },
    // No creatures: the panel stays out of these tests (its own spec covers it).
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
  ];
}

function activatedRouteFor(campaignId: string, characterId: string) {
  return { paramMap: of(convertToParamMap({ id: campaignId, characterId })) };
}

function flush(): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, 0));
}

/** The `<dd>` right after the `<dt>` with exactly this text (the page's
 * `<dl>`s: header fields, medallions, combat numbers, the story). */
function ddAfter(el: HTMLElement, term: string): HTMLElement | null {
  const dt = Array.from(el.querySelectorAll('dt')).find((d) => d.textContent?.trim() === term);
  return (dt?.nextElementSibling as HTMLElement | null) ?? null;
}

function sectionTitled(el: HTMLElement, title: string): HTMLElement {
  const heading = Array.from(el.querySelectorAll('h2')).find(
    (h) => h.textContent?.trim() === title,
  );
  return heading!.closest('section')!;
}

function buttonWithText(el: HTMLElement, text: string): HTMLButtonElement | undefined {
  return Array.from(el.querySelectorAll('button')).find((b) => b.textContent?.trim() === text);
}

describe('CharacterSheetPage', () => {
  let fake: FakeCharacterSheetSource;

  function configure(campaignId = 'camp-1', characterId = 'char-1'): void {
    TestBed.configureTestingModule({
      imports: [CharacterSheetPage],
      providers: [
        { provide: CharacterSheetSource, useClass: FakeCharacterSheetSource },
        { provide: ActivatedRoute, useValue: activatedRouteFor(campaignId, characterId) },
        ...xpProviders(),
      ],
    });
    fake = TestBed.inject(CharacterSheetSource) as unknown as FakeCharacterSheetSource;
  }

  async function render(): Promise<HTMLElement> {
    const fixture = TestBed.createComponent(CharacterSheetPage);
    fixture.detectChanges();
    await flush();
    await fixture.whenStable();
    fixture.detectChanges();
    return fixture.nativeElement as HTMLElement;
  }

  it('shows exactly the server-sent numbers, even an inconsistent modifier', async () => {
    configure();
    fake.getCharacterSheetFn = () =>
      Promise.resolve(
        vm({
          sheet: fullSheet({
            abilities: [
              { key: 'int', score: 18, modifier: 9 },
              { key: 'str', score: 10, modifier: 0 },
              { key: 'dex', score: 10, modifier: 0 },
              { key: 'con', score: 10, modifier: 0 },
              { key: 'wis', score: 10, modifier: 0 },
              { key: 'cha', score: 10, modifier: 0 },
            ],
          }),
        }),
      );

    const el = await render();
    // The medallion: the modifier large, the score in the pill, verbatim.
    expect(ddAfter(el, 'Inteligência')?.textContent?.trim()).toBe('+9');
    expect(ddAfter(el, 'Inteligência')?.nextElementSibling?.textContent).toContain('18');
  });

  it('shows a weapon attack with its bonus, and a damage cantrip with its save DC', async () => {
    configure();
    fake.getCharacterSheetFn = () =>
      Promise.resolve(
        vm({
          sheet: fullSheet({
            attacks: [
              {
                key: 'equipment:quarterstaff',
                namePt: 'Bordão',
                kind: 'weapon',
                attackBonus: 5,
                damage: '1d6+3',
                damageTypePt: 'concussão',
                versatileDamage: '',
                saveDc: 0,
                saveAbility: null,
                beams: 0,
              },
              {
                key: 'spell:fire-bolt',
                namePt: 'Raio de Fogo',
                kind: 'spell',
                attackBonus: 6,
                damage: '1d10',
                damageTypePt: 'fogo',
                versatileDamage: '',
                saveDc: 0,
                saveAbility: null,
                beams: 0,
              },
              {
                key: 'spell:poison-spray',
                namePt: 'Rajada de Veneno',
                kind: 'spell',
                attackBonus: 0,
                damage: '1d12',
                damageTypePt: 'veneno',
                versatileDamage: '',
                saveDc: 14,
                saveAbility: 'con',
                beams: 0,
              },
            ],
          }),
        }),
      );

    const el = await render();
    expect(el.textContent).toContain('Bordão');
    expect(el.textContent).toContain('+5');
    expect(el.textContent).toContain('1d6+3 concussão');
    expect(el.textContent).toContain('Raio de Fogo');
    expect(el.textContent).toContain('+6');
    expect(el.textContent).toContain('Rajada de Veneno');
    expect(el.textContent).toContain('CD 14');
    expect(el.textContent).toContain('Constituição');
    expect(el.textContent).toContain('1d12 veneno');
  });

  it('shows how many beams a cantrip fires, and keeps the unarmed strike out of the equipment', async () => {
    configure();
    fake.getCharacterSheetFn = () =>
      Promise.resolve(
        vm({
          sheet: fullSheet({
            attacks: [
              {
                key: 'attack:unarmed-strike',
                namePt: 'Golpe desarmado',
                kind: 'weapon',
                attackBonus: 2,
                damage: '1',
                damageTypePt: 'concussão',
                saveDc: 0,
                saveAbility: null,
                versatileDamage: '',
                beams: 0,
              },
              {
                key: 'spell:eldritch-blast',
                namePt: 'Rajada Mística',
                kind: 'spell',
                attackBonus: 9,
                damage: '1d10',
                damageTypePt: 'energia',
                saveDc: 0,
                saveAbility: null,
                versatileDamage: '',
                beams: 3,
              },
            ],
          }),
        }),
      );

    const el = await render();
    expect(el.textContent).toContain('3 raios, cada um com ataque e dano próprios');
    expect(el.textContent).toContain('Golpe desarmado');
    expect(sectionTitled(el, 'Equipamento').textContent).not.toContain('Golpe desarmado');
  });

  it('shows spell slots as a readable, separated list using "nível" (integrator fix)', async () => {
    configure();
    fake.getCharacterSheetFn = () =>
      Promise.resolve(
        vm({
          sheet: fullSheet({
            spellSlots: [4, 2],
            spellcasting: [
              {
                className: 'Mago',
                ability: 'int',
                saveDc: 14,
                attackBonus: 6,
                cantripsKnown: 3,
                spellsPreparedMax: 7,
                spellsKnownMax: 0,
              },
            ],
          }),
        }),
      );

    const el = await render();
    // One row per level, "nível" as the term, and the slots as circles
    // with an accessible count.
    const rows = Array.from(el.querySelectorAll('.slots__row'));
    expect(rows.map((r) => r.querySelector('.slots__level')?.textContent?.trim())).toEqual([
      '1º nível',
      '2º nível',
    ]);
    expect(rows.map((r) => r.querySelectorAll('.slots__circle').length)).toEqual([4, 2]);
    expect(rows.map((r) => r.querySelector('[role="img"]')?.getAttribute('aria-label'))).toEqual([
      '4 espaços',
      '2 espaços',
    ]);
    // The old bug: two <span>s with nothing between them rendered as
    // "1º nível: 42º nível: 2" — no separator, and the wrong term.
    expect(el.textContent).not.toContain('42º');
    expect(el.textContent).not.toContain('nível: 4');
  });

  it("shows a Warlock's pact slots in their own list, since the sheet has no other slots", async () => {
    configure();
    fake.getCharacterSheetFn = () =>
      Promise.resolve(
        vm({
          sheet: fullSheet({
            spellSlots: [],
            pactSlots: { level: 1, count: 2 },
            spellcasting: [
              {
                className: 'Bruxo',
                ability: 'cha',
                saveDc: 13,
                attackBonus: 5,
                cantripsKnown: 2,
                spellsPreparedMax: 2,
                spellsKnownMax: 0,
              },
            ],
          }),
        }),
      );

    const el = await render();
    expect(el.querySelector('[aria-label="Espaços de magia"]')).toBeNull();
    const pact = el.querySelector('[aria-label="Espaços do pacto"]');
    expect(pact?.querySelector('.slots__level')?.textContent?.trim()).toBe('Pacto · 1º nível');
    expect(pact?.querySelectorAll('.slots__circle').length).toBe(2);
    expect(pact?.querySelector('[role="img"]')?.getAttribute('aria-label')).toBe('2 espaços');
  });

  it("renders every official-sheet section as an <h2>, in the paper sheet's column order", async () => {
    configure();
    fake.getCharacterSheetFn = () =>
      Promise.resolve(
        vm({
          sheet: fullSheet({
            spellcasting: [
              {
                className: 'Mago',
                ability: 'int',
                saveDc: 14,
                attackBonus: 6,
                cantripsKnown: 3,
                spellsPreparedMax: 7,
                spellsKnownMax: 0,
              },
            ],
          }),
        }),
      );

    const el = await render();
    const headings = Array.from(el.querySelectorAll('h2')).map((h) => h.textContent?.trim());
    // Document order is the paper sheet's column order (the medallions;
    // saves and skills; combat, spells and equipment; features and story),
    // the same on every screen size: the phone shows it in one column.
    // "Habilidades" and "Combate" are for screen readers only; the
    // medallions and the shield are their visible titles. No "Ataques"
    // here: this sheet has no attacks.
    expect(headings).toEqual([
      'Habilidades',
      'Testes de resistência',
      'Perícias',
      'Combate',
      'Magias de mago',
      'Equipamento',
      // The player's own notes, the first block of the fourth column (E8-07).
      'Anotações',
      'Características e traços',
      'História',
    ]);
  });

  it("gives the player the 'Anotações' panel and never the master, even on the player's sheet (RN-20)", async () => {
    configure();
    fake.getCharacterSheetFn = () => Promise.resolve(vm({ isMaster: false }));
    const asPlayer = await render();
    expect(asPlayer.querySelector('app-notes-panel')).not.toBeNull();
    expect(notesApi.list).toHaveBeenCalledWith('camp-1');
    TestBed.resetTestingModule();
    notesApi.list.mockClear();
    configure();
    fake.getCharacterSheetFn = () =>
      Promise.resolve(vm({ isMaster: true, canAccessMasterNotes: true }));
    const asMaster = await render();
    expect(asMaster.querySelector('app-notes-panel')).toBeNull();
    expect(
      Array.from(asMaster.querySelectorAll('h2')).map((h) => h.textContent?.trim()),
    ).not.toContain('Anotações');
    expect(notesApi.list).not.toHaveBeenCalled();
  });

  it('shows the six saving throws in their own "Testes de resistência" section, proficiency marked like skills', async () => {
    configure();
    fake.getCharacterSheetFn = () =>
      Promise.resolve(
        vm({
          sheet: fullSheet({
            savingThrows: [
              { key: 'str', bonus: 5, proficient: true },
              { key: 'dex', bonus: 0, proficient: false },
              { key: 'con', bonus: 1, proficient: false },
              { key: 'int', bonus: 6, proficient: true },
              { key: 'wis', bonus: 1, proficient: false },
              { key: 'cha', bonus: 0, proficient: false },
            ],
          }),
        }),
      );

    const el = await render();
    const section = sectionTitled(el, 'Testes de resistência');
    const row = (name: string) =>
      Array.from(section.querySelectorAll('li')).find((li) => li.textContent?.includes(name))!;
    expect(section.querySelectorAll('li').length).toBe(6);
    expect(row('Força').textContent).toContain('+5');
    expect(row('Força').textContent).toContain('proficiente');
    expect(row('Força').querySelector('.dot--proficient')).toBeTruthy();
    expect(row('Destreza').textContent).toContain('+0');
    expect(row('Destreza').textContent).toContain('sem proficiência');
    expect(row('Destreza').querySelector('.dot--proficient')).toBeNull();
    // No longer buried, unlabelled, at the top of Perícias.
    expect(sectionTitled(el, 'Perícias').textContent).not.toContain('Força');
  });

  it('shows each skill with its ability abbreviation and its proficiency level', async () => {
    configure();
    fake.getCharacterSheetFn = () =>
      Promise.resolve(
        vm({
          sheet: fullSheet({
            skills: [
              {
                key: 'skill:arcana',
                namePt: 'Arcanismo',
                ability: 'int',
                bonus: 6,
                proficiency: 'proficient',
              },
              {
                key: 'skill:history',
                namePt: 'História',
                ability: 'int',
                bonus: 8,
                proficiency: 'expertise',
              },
              {
                key: 'skill:stealth',
                namePt: 'Furtividade',
                ability: 'dex',
                bonus: 3,
                proficiency: 'none',
              },
            ],
          }),
        }),
      );

    const el = await render();
    const rows = Array.from(sectionTitled(el, 'Perícias').querySelectorAll('li'));
    expect(rows[0].textContent).toContain('+6');
    expect(rows[0].querySelector('abbr')?.textContent).toBe('Int');
    expect(rows[0].querySelector('abbr')?.getAttribute('title')).toBe('Inteligência');
    expect(rows[0].textContent).toContain('proficiente');
    expect(rows[1].querySelector('.dot--expertise')).toBeTruthy();
    expect(rows[1].textContent).toContain('expertise');
    expect(rows[2].querySelector('abbr')?.textContent).toBe('Des');
    expect(rows[2].textContent).toContain('sem proficiência');
  });

  it('shows the locked banner and hides "Editar ficha" when the player cannot edit', async () => {
    configure();
    fake.getCharacterSheetFn = () =>
      Promise.resolve(
        vm({
          state: 'locked',
          canEdit: false,
          sheetLockedAt: new Date('2026-09-29T12:00:00Z'),
        }),
      );

    const el = await render();
    expect(el.textContent).toContain('Ficha travada desde 29/09/2026');
    expect(el.textContent).not.toContain('Editar ficha');
    // The tag says it too, with a lock icon.
    const tags = el.querySelector('[aria-label="Estado do personagem"]')!;
    expect(tags.textContent).toContain('Travada');
    expect(tags.querySelector('mat-icon')?.textContent).toContain('lock');
  });

  it('shows "Editar ficha" for the master even when the sheet is locked', async () => {
    configure();
    fake.getCharacterSheetFn = () =>
      Promise.resolve(
        vm({
          state: 'locked',
          canEdit: true,
          isMaster: true,
          canAccessMasterNotes: true,
          sheetLockedAt: new Date('2026-09-29T12:00:00Z'),
        }),
      );
    fake.getMasterNotesFn = () => Promise.resolve('');

    const el = await render();
    expect(el.textContent).toContain('Editar ficha');
  });

  it('shows the master notes panel and "Marcar como morto" only for the master', async () => {
    configure();
    fake.getCharacterSheetFn = () => Promise.resolve(vm({ isMaster: false }));

    const el = await render();
    expect(el.textContent).not.toContain('Notas do mestre');
    expect(el.textContent).not.toContain('Marcar como morto');
    // RN-11: the notes RPC is never even called for a player.
    expect(fake.getMasterNotesCalls).toEqual([]);
  });

  it('loads and shows the master notes panel for the master, never for a player', async () => {
    configure();
    fake.getCharacterSheetFn = () =>
      Promise.resolve(vm({ isMaster: true, canAccessMasterNotes: true, canMarkDead: true }));
    fake.getMasterNotesFn = () => Promise.resolve('Esconde um segredo.');

    const el = await render();
    expect(el.textContent).toContain('Notas do mestre');
    expect(el.textContent).toContain('Marcar como morto');
    expect(fake.getMasterNotesCalls).toEqual(['char-1']);
  });

  it('hides "Editar história" for a player once the sheet has locked it', async () => {
    configure();
    fake.getCharacterSheetFn = () =>
      Promise.resolve(
        vm({ isMaster: false, state: 'locked', canEdit: false, canEditStory: false }),
      );

    const el = await render();
    expect(el.textContent).not.toContain('Editar história');
  });

  it('shows "Editar história" for a player once the master has unlocked it', async () => {
    configure();
    fake.getCharacterSheetFn = () =>
      Promise.resolve(vm({ isMaster: false, state: 'locked', canEdit: false, canEditStory: true }));

    const el = await render();
    expect(el.textContent).toContain('Editar história');
  });

  it("shows the master's story toggle, labeled by storyEditingAllowed, and flips it on click", async () => {
    configure();
    fake.getCharacterSheetFn = () =>
      Promise.resolve(
        vm({
          isMaster: true,
          canAccessMasterNotes: true,
          canToggleStoryEditing: true,
          storyEditingAllowed: false,
        }),
      );
    fake.getMasterNotesFn = () => Promise.resolve('');
    fake.setStoryEditingAllowedFn = () =>
      Promise.resolve(
        vm({
          isMaster: true,
          canToggleStoryEditing: true,
          storyEditingAllowed: true,
          // SetStoryEditing never changes the revision (integrator fix,
          // phase 2) — the response keeps it exactly as it was.
          revision: 1,
        }),
      );

    const el = await render();
    expect(el.textContent).toContain('Permitir editar a história');

    const toggle = Array.from(el.querySelectorAll('button')).find((b) =>
      b.textContent?.includes('Permitir editar a história'),
    ) as HTMLButtonElement;
    toggle.click();
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(fake.setStoryEditingAllowedCalls).toEqual([{ id: 'char-1', allowed: true }]);
  });

  it('never shows the story toggle for a player', async () => {
    configure();
    fake.getCharacterSheetFn = () =>
      Promise.resolve(vm({ isMaster: false, canToggleStoryEditing: false }));

    const el = await render();
    expect(el.textContent).not.toContain('Permitir editar a história');
    expect(el.textContent).not.toContain('Travar a história');
  });

  it('renders an NPC basic sheet with its basic stats, and no ability grid', async () => {
    configure();
    fake.getCharacterSheetFn = () =>
      Promise.resolve(
        vm({
          characterKind: 'minion',
          playerDisplayName: null,
          story: null,
          sheet: basicSheet({ armorClass: 13, hitPointsMax: 7 }),
        }),
      );

    const el = await render();
    expect(ddAfter(el, 'Classe de Armadura')?.textContent?.trim()).toBe('13');
    expect(ddAfter(el, 'Pontos de vida máximos')?.textContent?.trim()).toBe('7');
    expect(ddAfter(el, 'Iniciativa')?.textContent?.trim()).toBe('+2');
    const attacks = Array.from(el.querySelectorAll('.attacks__item')).map((li) =>
      li.textContent?.replace(/\s+/g, ' ').trim(),
    );
    expect(attacks).toEqual([
      'Cimitarra +4 · 1d6 + 2 cortante',
      'Arco curto +4 · 2d4 − 1 perfurante',
    ]);
    expect(el.textContent).toContain('Um goblin arisco.');
    expect(el.querySelector('app-ability-medallions')).toBeNull();
    // An NPC's tag is its kind: it never leaves "Rascunho", so that isn't shown.
    const tags = el.querySelector('[aria-label="Estado do personagem"]')!;
    expect(tags.textContent).toContain('Minion');
    expect(tags.textContent).not.toContain('Rascunho');
  });

  it('shows the old damage text as a note when it could not become an attack', async () => {
    configure();
    fake.getCharacterSheetFn = () =>
      Promise.resolve(
        vm({
          characterKind: 'minion',
          playerDisplayName: null,
          story: null,
          sheet: basicSheet({ attacks: [], legacyDamage: 'mordida venenosa' }),
        }),
      );

    const el = await render();
    expect(el.querySelector('.attacks__item')).toBeNull();
    expect(el.textContent).toContain('Sem ataques.');
    expect(el.textContent).toContain('Dano antigo: mordida venenosa');
  });

  it('shows the alignment and XP in the header, read from the stored sheet (integrator follow-up)', async () => {
    configure();
    fake.getCharacterSheetFn = () =>
      Promise.resolve(vm({ alignmentLabel: 'Caótico e bom', experiencePoints: 900 }));

    const el = await render();
    expect(ddAfter(el, 'Tendência')?.textContent?.trim()).toBe('Caótico e bom');
    // The XP is a block to read (E7-10), not a field of the identity row.
    expect(ddAfter(el, 'Experiência')).toBeNull();
    expect(el.querySelector('app-xp-block .xp__n')?.textContent?.trim()).toBe('900\u00a0XP');
    expect(ddAfter(el, 'Classe e nível')?.textContent?.trim()).toBe('Mago 3');
    expect(ddAfter(el, 'Raça')?.textContent?.trim()).toBe('Gnomo da Rocha');
    expect(ddAfter(el, 'Antecedente')?.textContent?.trim()).toBe('Sábio');
    expect(ddAfter(el, 'Jogador')?.textContent?.trim()).toBe('Vinicius');
  });

  it('shows 0 XP (a real value), but hides alignment when it is unset', async () => {
    configure();
    fake.getCharacterSheetFn = () =>
      Promise.resolve(vm({ alignmentLabel: '', experiencePoints: 0 }));

    const el = await render();
    expect(el.querySelector('app-xp-block .xp__n')?.textContent?.trim()).toBe('0\u00a0XP');
    expect(ddAfter(el, 'Tendência')).toBeNull();
  });

  it('shows "Jogador sem nome" for a player without a display name, and no player field for an NPC', async () => {
    configure();
    fake.getCharacterSheetFn = () => Promise.resolve(vm({ playerDisplayName: null }));
    const el = await render();
    expect(ddAfter(el, 'Jogador')?.textContent?.trim()).toBe('Jogador sem nome');
    expect(el.textContent).not.toContain('Sem nome');
  });

  it('hides XP for an NPC basic sheet, which has none', async () => {
    configure();
    fake.getCharacterSheetFn = () =>
      Promise.resolve(
        vm({
          characterKind: 'minion',
          story: null,
          alignmentLabel: '',
          experiencePoints: null,
          sheet: basicSheet(),
        }),
      );

    const el = await render();
    expect(ddAfter(el, 'Experiência')).toBeNull();
    expect(ddAfter(el, 'Jogador')).toBeNull();
    expect(el.textContent).not.toContain('XP');
  });

  it('lists the armor, shield and weapons carried, not just free-text items (integrator fix)', async () => {
    configure();
    fake.getCharacterSheetFn = () =>
      Promise.resolve(
        vm({
          sheet: fullSheet({
            armorClassDescription: 'Armadura de couro + escudo',
            wearsArmor: true,
            hasShield: true,
            attacks: [
              {
                key: 'equipment:shortsword',
                namePt: 'Espada curta',
                kind: 'weapon',
                attackBonus: 4,
                damage: '1d6+2',
                damageTypePt: 'perfurante',
                versatileDamage: '',
                saveDc: 0,
                saveAbility: null,
                beams: 0,
              },
              {
                key: 'spell:fire-bolt',
                namePt: 'Raio de Fogo',
                kind: 'spell',
                attackBonus: 6,
                damage: '1d10',
                damageTypePt: 'fogo',
                versatileDamage: '',
                saveDc: 0,
                saveAbility: null,
                beams: 0,
              },
            ],
            equipment: [{ name: 'Corda (15m)', quantity: 1 }],
          }),
        }),
      );

    const el = await render();
    const section = sectionTitled(el, 'Equipamento');
    const items = Array.from(section.querySelectorAll('li')).map((li) => li.textContent?.trim());
    // The armour's own name, without the AC description's shield suffix.
    expect(items).toContain('Armadura de couro');
    expect(items).toContain('Escudo');
    expect(section.textContent).toContain('Espada curta');
    // A damage cantrip is not a weapon — never listed as equipment.
    expect(section.textContent).not.toContain('Raio de Fogo');
    expect(section.textContent).toContain('Corda (15m)');
    expect(section.textContent).not.toContain('Nenhum item cadastrado');
    // No coins: said once, in words.
    expect(section.textContent).toContain('Sem moedas');
  });

  it('shows the background equipment text in "Equipamento", and nothing when there is none', async () => {
    configure();
    fake.getCharacterSheetFn = () =>
      Promise.resolve(
        vm({ sheet: fullSheet({ backgroundEquipment: 'Um livro de orações e 15 PO' }) }),
      );
    const el = await render();
    expect(sectionTitled(el, 'Equipamento').textContent).toContain(
      'Do antecedente: Um livro de orações e 15\u00a0PO',
    );

    fake.getCharacterSheetFn = () => Promise.resolve(vm({ sheet: fullSheet() }));
    const without = await render();
    expect(sectionTitled(without, 'Equipamento').textContent).not.toContain('Do antecedente');
  });

  it('shows the two-handed damage of a versatile weapon under its damage', async () => {
    configure();
    fake.getCharacterSheetFn = () =>
      Promise.resolve(
        vm({
          sheet: fullSheet({
            attacks: [
              {
                key: 'equipment:quarterstaff',
                namePt: 'Bordão',
                kind: 'weapon',
                attackBonus: 2,
                damage: '1d6',
                damageTypePt: 'concussão',
                versatileDamage: '1d8',
                saveDc: 0,
                saveAbility: null,
                beams: 1,
              },
              {
                key: 'equipment:dagger',
                namePt: 'Adaga',
                kind: 'weapon',
                attackBonus: 2,
                damage: '1d4',
                damageTypePt: 'perfurante',
                versatileDamage: '',
                saveDc: 0,
                saveAbility: null,
                beams: 1,
              },
            ],
          }),
        }),
      );
    const el = await render();
    const rows = Array.from(sectionTitled(el, 'Ataques').querySelectorAll('tbody tr'));
    expect(rows[0].textContent).toContain('Com duas mãos: 1d8');
    expect(rows[1].textContent).not.toContain('duas mãos');
  });

  it('lists only the coins carried', async () => {
    configure();
    fake.getCharacterSheetFn = () =>
      Promise.resolve(vm({ sheet: fullSheet({ coins: { cp: 0, sp: 3, ep: 0, gp: 15, pp: 0 } }) }));

    const el = await render();
    const coins = sectionTitled(el, 'Equipamento').querySelector('[aria-label="Moedas"]')!;
    expect(Array.from(coins.querySelectorAll('li')).map((li) => li.textContent?.trim())).toEqual([
      '15 PO',
      '3 PP',
    ]);
    expect(el.textContent).not.toContain('Sem moedas');
  });

  it('shows "Sem armadura" and no "Escudo" line when neither is carried', async () => {
    configure();
    fake.getCharacterSheetFn = () =>
      Promise.resolve(vm({ sheet: fullSheet({ armorClassDescription: 'Sem armadura' }) }));

    const el = await render();
    const section = sectionTitled(el, 'Equipamento');
    expect(section.textContent).toContain('Sem armadura');
    expect(section.textContent).not.toContain('Escudo');
  });

  it('says "Sem armadura" when a feature, not armor, gives the AC (Unarmored Defense)', async () => {
    configure();
    fake.getCharacterSheetFn = () =>
      Promise.resolve(
        vm({
          sheet: fullSheet({ armorClassDescription: 'Defesa sem Armadura', wearsArmor: false }),
        }),
      );

    const el = await render();
    const section = sectionTitled(el, 'Equipamento');
    expect(section.textContent).toContain('Sem armadura');
    expect(section.textContent).not.toContain('Defesa sem Armadura');
  });

  it('shows each feature as a compact row, its English description collapsed by default; issues in the notice under the header, hints as reminders', async () => {
    configure();
    fake.getCharacterSheetFn = () =>
      Promise.resolve(
        vm({
          sheet: fullSheet({
            features: [
              {
                name: 'Recuperação Arcana',
                sourcePt: 'Mago 1',
                description: 'You have learned to regain some of your magical energy.',
              },
            ],
            issues: [
              {
                code: 'unknown_key',
                field: 'full.armor_key',
                message: 'A armadura escolhida não existe no conteúdo srd51@test.',
              },
            ],
            hints: [
              {
                sourceKey: 'trait:gnome-cunning',
                text: 'Vantagem em testes de resistência de INT, SAB e CAR contra magia.',
              },
            ],
          }),
        }),
      );

    const el = await render();
    const details = el.querySelector('details');
    expect(details).toBeTruthy();
    const summary = details!.querySelector('summary')!;
    expect(summary.querySelector('.feature__name')?.textContent?.trim()).toBe('Recuperação Arcana');
    expect(summary.querySelector('.feature__source')?.textContent?.trim()).toBe('Mago 1');
    expect(details!.open).toBe(false);
    const description = details!.querySelector('p')!;
    expect(description.textContent).toContain(
      'You have learned to regain some of your magical energy.',
    );
    // The SRD text is English: marked so, for screen readers and translators.
    expect(description.getAttribute('lang')).toBe('en');

    // The issue: in the warning notice under the header, its title in bold.
    const notice = el.querySelector('.mr-notice--warning')!;
    expect(notice.querySelector('strong')?.textContent).toBe('Escolha inválida.');
    expect(notice.textContent).toContain('A armadura escolhida não existe no conteúdo srd51@test.');
    // The hint: a quiet reminder in "Características e traços", not a problem.
    expect(notice.textContent).not.toContain('Vantagem em testes');
    const features = sectionTitled(el, 'Características e traços');
    const lembretes = Array.from(features.querySelectorAll('h3')).find(
      (h) => h.textContent?.trim() === 'Lembretes',
    );
    expect(lembretes).toBeTruthy();
    expect(features.textContent).toContain(
      'Vantagem em testes de resistência de INT, SAB e CAR contra magia.',
    );
  });

  it('"A classe mudou": the changed entry\'s sentences above the sheet, and the same issue never listed twice (RN-23)', async () => {
    configure();
    const sentence = 'Guardião do Vale agora dá 2 perícias no nível 1; esta ficha tem 3.';
    fake.getCharacterSheetFn = () =>
      Promise.resolve(
        vm({
          sheet: fullSheet({
            changedContent: [
              {
                key: 'class:guardiao-do-vale@mesa',
                namePt: 'Guardião do Vale',
                changedAt: new Date(2026, 9, 5),
                messages: [sentence],
              },
            ],
            issues: [
              {
                code: 'table_content_changed',
                field: 'full.classes[0].class_key',
                message: sentence,
              },
              {
                code: 'unknown_key',
                field: 'full.armor_key',
                message: 'A armadura escolhida não existe no conteúdo srd51@test.',
              },
            ],
          }),
        }),
      );

    const el = await render();
    const change = el.querySelector('app-changed-content')!;
    expect(change.textContent).toContain('A classe mudou.');
    expect(change.textContent).toContain(sentence);
    // The other issue stays in its own notice; the sentence is not repeated there.
    const issues = el.querySelector('[aria-label="Pendências de regra"]')!;
    expect(issues.textContent).toContain('A armadura escolhida não existe');
    expect(issues.textContent).not.toContain(sentence);
  });

  it('shows no "A classe mudou" notice for a sheet nothing changed under', async () => {
    configure();
    fake.getCharacterSheetFn = () => Promise.resolve(vm());
    const el = await render();
    expect(el.querySelector('app-changed-content')?.textContent?.trim() ?? '').toBe('');
  });

  it('shows no rules notice and no reminders when there are no issues or hints', async () => {
    configure();
    fake.getCharacterSheetFn = () => Promise.resolve(vm());

    const el = await render();
    expect(el.querySelector('.mr-notice--warning')).toBeNull();
    const lembretes = Array.from(el.querySelectorAll('h3')).find(
      (h) => h.textContent?.trim() === 'Lembretes',
    );
    expect(lembretes).toBeUndefined();
  });

  it('never shows the rules content version (nothing internal on screen)', async () => {
    configure();
    fake.getCharacterSheetFn = () =>
      Promise.resolve(vm({ sheet: fullSheet({ contentVersion: 'srd51@abc123' }) }));

    const el = await render();
    expect(el.textContent).not.toContain('srd51@abc123');
    expect(el.textContent).not.toContain('Conteúdo de regras');
  });

  it('"Marcar como morto" asks to confirm before marking the character dead', async () => {
    configure();
    let calls = 0;
    fake.getCharacterSheetFn = () =>
      Promise.resolve(vm({ state: 'locked', isMaster: true, canMarkDead: true }));
    fake.markCharacterDeadFn = () => {
      calls++;
      return Promise.resolve(
        vm({
          state: 'dead',
          isMaster: true,
          canMarkDead: false,
          diedAt: new Date(2026, 8, 30, 21, 0),
        }),
      );
    };
    const fixture = TestBed.createComponent(CharacterSheetPage);
    fixture.detectChanges();
    await flush();
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;

    buttonWithText(el, 'Marcar como morto')!.click();
    fixture.detectChanges();
    expect(calls).toBe(0); // nothing yet: one more click
    expect(el.textContent).toContain('A morte não se desfaz.');

    buttonWithText(el, 'Cancelar')!.click();
    fixture.detectChanges();
    expect(buttonWithText(el, 'Confirmar morte')).toBeUndefined();

    buttonWithText(el, 'Marcar como morto')!.click();
    fixture.detectChanges();
    buttonWithText(el, 'Confirmar morte')!.click();
    await flush();
    fixture.detectChanges();

    expect(calls).toBe(1);
    expect(el.querySelector('[aria-label="Estado do personagem"]')?.textContent).toContain('Morto');
    expect(el.textContent).toContain('Morreu em 30/09/2026');
    expect(buttonWithText(el, 'Marcar como morto')).toBeUndefined();
  });

  it('shows only the story fields that are filled in, and says when there is nothing yet', async () => {
    configure();
    fake.getCharacterSheetFn = () =>
      Promise.resolve(
        vm({
          story: {
            ...emptyStory(),
            appearance: { ...emptyStory().appearance, height: '1,05 m' },
            backstory: 'Cresceu entre livros.',
          },
        }),
      );
    const el = await render();
    const story = sectionTitled(el, 'História');
    expect(ddAfter(story, 'Altura')?.textContent?.trim()).toBe('1,05 m');
    expect(ddAfter(story, 'Antecedentes')?.textContent?.trim()).toBe('Cresceu entre livros.');
    expect(ddAfter(story, 'Idade')).toBeNull();
    expect(ddAfter(story, 'Aliados')).toBeNull();
    expect(story.textContent).not.toContain('—');

    TestBed.resetTestingModule();
    configure();
    fake.getCharacterSheetFn = () => Promise.resolve(vm({ story: emptyStory() }));
    const empty = await render();
    expect(sectionTitled(empty, 'História').textContent).toContain('Nada escrito ainda.');
  });

  it('tells a player whose story is locked that the master can unlock it', async () => {
    configure();
    fake.getCharacterSheetFn = () =>
      Promise.resolve(
        vm({ isMaster: false, state: 'locked', canEdit: false, canEditStory: false }),
      );
    const el = await render();
    expect(sectionTitled(el, 'História').textContent).toContain(
      'A história está travada. O mestre pode liberar a edição até a próxima sessão.',
    );
  });

  it('saves the story through the panel and shows what the server sent back', async () => {
    configure();
    fake.getCharacterSheetFn = () => Promise.resolve(vm({ revision: 4 }));
    let saved: { revision: number; backstory: string } | null = null;
    fake.updateCharacterStoryFn = (_c, _id, revision, story) => {
      saved = { revision, backstory: story.backstory };
      return Promise.resolve(
        vm({ revision: 5, story: { ...emptyStory(), backstory: 'Do servidor.' } }),
      );
    };
    const fixture = TestBed.createComponent(CharacterSheetPage);
    fixture.detectChanges();
    await flush();
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;

    buttonWithText(el, 'Editar história')!.click();
    fixture.detectChanges();
    const backstory = Array.from(el.querySelectorAll('mat-form-field'))
      .find((f) => f.textContent?.includes('Antecedentes'))!
      .querySelector('textarea')!;
    backstory.value = 'Cresceu entre livros.';
    backstory.dispatchEvent(new Event('input'));
    buttonWithText(el, 'Salvar história')!.click();
    await flush();
    fixture.detectChanges();

    expect(saved).toEqual({ revision: 4, backstory: 'Cresceu entre livros.' });
    expect(ddAfter(el, 'Antecedentes')?.textContent?.trim()).toBe('Do servidor.');
  });
});

describe('CharacterSheetPage: approval (MR-024)', () => {
  let fake: FakeCharacterSheetSource;

  beforeEach(() => {
    TestBed.configureTestingModule({
      imports: [CharacterSheetPage],
      providers: [
        { provide: CharacterSheetSource, useClass: FakeCharacterSheetSource },
        { provide: ActivatedRoute, useValue: activatedRouteFor('camp-1', 'char-1') },
        ...xpProviders(),
      ],
    });
    fake = TestBed.inject(CharacterSheetSource) as unknown as FakeCharacterSheetSource;
  });

  const pendingForMaster = () =>
    vm({ state: 'pending', canApprove: true, isMaster: true, canAccessMasterNotes: true });

  async function render() {
    const fixture = TestBed.createComponent(CharacterSheetPage);
    fixture.detectChanges();
    await flush();
    await fixture.whenStable();
    fixture.detectChanges();
    return fixture;
  }

  function button(el: HTMLElement, text: string): HTMLButtonElement | undefined {
    return Array.from(el.querySelectorAll('button')).find((b) => b.textContent?.trim() === text);
  }

  it('the pending player sees "Esperando a aprovação do mestre", and no approval buttons', async () => {
    fake.getCharacterSheetFn = () => Promise.resolve(vm({ state: 'pending', canApprove: false }));
    const el = (await render()).nativeElement as HTMLElement;

    expect(el.querySelector('[aria-label="Estado do personagem"]')?.textContent).toContain(
      'Pendente',
    );
    expect(el.textContent).toContain('Esperando a aprovação do mestre');
    expect(button(el, 'Aprovar personagem')).toBeUndefined();
    expect(button(el, 'Recusar personagem')).toBeUndefined();
    // Still editable while waiting.
    expect(el.textContent).toContain('Editar ficha');
  });

  it('a pending character shows no notes and no creatures panel, only when they will appear', async () => {
    fake.getCharacterSheetFn = () => Promise.resolve(vm({ state: 'pending', canApprove: false }));
    const el = (await render()).nativeElement as HTMLElement;

    expect(el.querySelector('app-notes-panel')).toBeNull();
    expect(el.querySelector('app-creatures-panel')).toBeNull();
    const notes = Array.from(el.querySelectorAll('.sheet__pending-note')).map((n) =>
      n.textContent?.trim(),
    );
    expect(notes).toEqual([
      'Aparece quando o mestre aprovar o personagem.',
      'Aparece quando o mestre aprovar o personagem.',
    ]);
  });

  it('an approved character shows the notes and the creatures panels', async () => {
    fake.getCharacterSheetFn = () => Promise.resolve(vm({ state: 'draft', canApprove: false }));
    const el = (await render()).nativeElement as HTMLElement;

    expect(el.querySelector('app-notes-panel')).not.toBeNull();
    expect(el.querySelector('app-creatures-panel')).not.toBeNull();
    expect(el.querySelector('.sheet__pending-note')).toBeNull();
  });

  it('"Aprovar personagem" approves and shows the character as a draft', async () => {
    fake.getCharacterSheetFn = () => Promise.resolve(pendingForMaster());
    fake.approveCharacterFn = () =>
      Promise.resolve(
        vm({ state: 'draft', canApprove: false, isMaster: true, canAccessMasterNotes: true }),
      );
    const fixture = await render();
    const el = fixture.nativeElement as HTMLElement;

    button(el, 'Aprovar personagem')!.click();
    await flush();
    fixture.detectChanges();

    expect(el.querySelector('[aria-label="Estado do personagem"]')?.textContent).toContain(
      'Rascunho',
    );
    expect(button(el, 'Aprovar personagem')).toBeUndefined();
  });

  it('"Recusar personagem" asks to confirm, then rejects and goes back to the campaign', async () => {
    fake.getCharacterSheetFn = () => Promise.resolve(pendingForMaster());
    fake.rejectCharacterFn = () => Promise.resolve();
    const navigate = vi.spyOn(TestBed.inject(Router), 'navigate').mockResolvedValue(true);
    const fixture = await render();
    const el = fixture.nativeElement as HTMLElement;

    button(el, 'Recusar personagem')!.click();
    fixture.detectChanges();
    expect(fake.rejectCharacterCalls).toEqual([]); // nothing yet: one more click
    expect(button(el, 'Cancelar')).toBeTruthy();

    button(el, 'Confirmar recusa')!.click();
    await flush();
    fixture.detectChanges();

    expect(fake.rejectCharacterCalls).toEqual(['char-1']);
    expect(navigate).toHaveBeenCalledWith(['/campaigns', 'camp-1']);
  });

  it("shows the server's reason when the rejection fails", async () => {
    fake.getCharacterSheetFn = () => Promise.resolve(pendingForMaster());
    fake.rejectCharacterFn = () => Promise.reject(new ConnectError('gone', Code.NotFound));
    const fixture = await render();
    const el = fixture.nativeElement as HTMLElement;

    button(el, 'Recusar personagem')!.click();
    fixture.detectChanges();
    button(el, 'Confirmar recusa')!.click();
    await flush();
    fixture.detectChanges();

    expect(el.querySelector('[role="alert"]')?.textContent).toContain('Personagem não encontrado');
  });

  it('a sheet the server does not show (not_found) reads "Personagem não encontrado", not an error (RN-20)', async () => {
    fake.getCharacterSheetFn = () =>
      Promise.reject(new ConnectError('character not found', Code.NotFound));
    const fixture = await render();
    const el = fixture.nativeElement as HTMLElement;

    expect(el.querySelector('h1')?.textContent).toContain('Personagem não encontrado');
    expect(el.textContent).toContain('Esse personagem não existe, ou você não pode vê-lo.');
    expect(el.textContent).not.toContain('Não foi possível abrir a ficha');
  });
});

describe('CharacterSheetPage: the XP block (MR-016, RN-12, E7-10)', () => {
  let fake: FakeCharacterSheetSource;
  const nbsp = ' ';

  beforeEach(() => {
    openSessions.set([]);
    xpWatcher.follow.mockClear();
    TestBed.configureTestingModule({
      imports: [CharacterSheetPage],
      providers: [
        { provide: CharacterSheetSource, useClass: FakeCharacterSheetSource },
        { provide: ActivatedRoute, useValue: activatedRouteFor('camp-1', 'char-1') },
        ...xpProviders(),
      ],
    });
    fake = TestBed.inject(CharacterSheetSource) as unknown as FakeCharacterSheetSource;
  });

  async function render() {
    const fixture = TestBed.createComponent(CharacterSheetPage);
    fixture.detectChanges();
    await flush();
    await fixture.whenStable();
    fixture.detectChanges();
    return fixture;
  }

  const block = (el: HTMLElement) => el.querySelector<HTMLElement>('app-xp-block');

  it('shows the XP as a block to read, with the bar and what is missing', async () => {
    fake.getCharacterSheetFn = () =>
      Promise.resolve(vm({ experiencePoints: 2366, totalLevel: 3, nextLevelXp: 2700 }));
    const el = (await render()).nativeElement as HTMLElement;

    const xp = block(el)!;
    expect(xp.querySelector('.xp__n')?.textContent?.trim()).toBe(`2.366${nbsp}XP`);
    expect(xp.querySelector('.xp__line')?.textContent?.trim()).toBe(
      `2.366 de${nbsp}2.700${nbsp}XP para o nível 4. Faltam${nbsp}334${nbsp}XP.`,
    );
    expect(xp.querySelector('app-level-up-tag')).toBeNull();
    // Read only: nothing to type in, and the bar is decoration.
    expect(xp.querySelector('input')).toBeNull();
    expect(xp.querySelector('.xp__bar')?.getAttribute('aria-hidden')).toBe('true');
  });

  it('is a polite live region, so an award given meanwhile is announced', async () => {
    fake.getCharacterSheetFn = () => Promise.resolve(vm());
    const el = (await render()).nativeElement as HTMLElement;

    const region = block(el)!.querySelector('[role="status"]');
    expect(region?.getAttribute('aria-live')).toBe('polite');
  });

  it('says "Pode subir de nível" and what to do when the XP reached the next level', async () => {
    fake.getCharacterSheetFn = () =>
      Promise.resolve(
        vm({ experiencePoints: 2716, totalLevel: 3, nextLevelXp: 2700, canLevelUp: true }),
      );
    const el = (await render()).nativeElement as HTMLElement;

    expect(block(el)!.querySelector('app-level-up-tag')?.textContent).toContain(
      'Pode subir de nível',
    );
    expect(block(el)!.querySelector('.xp__line')?.textContent).toContain(
      'O mestre sobe o seu nível na ficha.',
    );
  });

  it("tells the master the same, in the master's words", async () => {
    fake.getCharacterSheetFn = () =>
      Promise.resolve(
        vm({
          experiencePoints: 2716,
          nextLevelXp: 2700,
          canLevelUp: true,
          isMaster: true,
          canAccessMasterNotes: true,
        }),
      );
    const el = (await render()).nativeElement as HTMLElement;

    expect(block(el)!.querySelector('.xp__line')?.textContent).toContain('Suba o nível na ficha.');
  });

  it('says "Nível máximo." at level 20, where there is no next level', async () => {
    fake.getCharacterSheetFn = () =>
      Promise.resolve(vm({ experiencePoints: 400000, totalLevel: 20, nextLevelXp: 0 }));
    const el = (await render()).nativeElement as HTMLElement;

    expect(block(el)!.querySelector('.xp__line')?.textContent?.trim()).toBe('Nível máximo.');
    expect(block(el)!.querySelector('app-level-up-tag')).toBeNull();
  });

  it('has no XP block in a milestones campaign, only the tag when it can level up', async () => {
    fake.xpMode = 'milestones';
    fake.getCharacterSheetFn = () =>
      Promise.resolve(vm({ canLevelUp: true, state: 'locked', isMaster: true }));
    const el = (await render()).nativeElement as HTMLElement;

    expect(block(el)).toBeNull();
    expect(el.textContent).not.toContain('XP');
    expect(el.querySelector('.head__tags app-level-up-tag')?.textContent).toContain(
      'Pode subir de nível',
    );
  });

  it('leaves the tag to the level-up block when the owner of a locked sheet can level up (MR-040)', async () => {
    fake.xpMode = 'milestones';
    fake.getCharacterSheetFn = () =>
      Promise.resolve(vm({ canLevelUp: true, state: 'locked', levelUpReason: 'milestone' }));
    const el = (await render()).nativeElement as HTMLElement;

    expect(el.querySelector('.head__tags app-level-up-tag')).toBeNull();
    expect(el.querySelector('app-level-up-banner a')?.textContent).toContain(
      'Subir para o nível 4',
    );
  });

  it('has no tag in a milestones campaign when the character cannot level up', async () => {
    fake.xpMode = 'milestones';
    fake.getCharacterSheetFn = () => Promise.resolve(vm({ canLevelUp: false }));
    const el = (await render()).nativeElement as HTMLElement;

    expect(el.querySelector('app-level-up-tag')).toBeNull();
  });

  it('has no XP block on an NPC sheet, but shows the master the ND and the XP it gives', async () => {
    fake.getCharacterSheetFn = () =>
      Promise.resolve(
        vm({
          characterKind: 'enemy',
          playerDisplayName: null,
          experiencePoints: 0,
          challengeRating: '1',
          xpValue: 200,
          isMaster: true,
          canAccessMasterNotes: true,
        }),
      );
    const el = (await render()).nativeElement as HTMLElement;

    expect(block(el)).toBeNull();
    expect(ddAfter(el, 'Nível de desafio')?.textContent?.trim()).toBe('ND 1');
    expect(ddAfter(el, 'XP ao derrotar')?.textContent?.trim()).toBe(`200${nbsp}XP`);
  });

  it("never shows a player the NPC's ND or XP (RN-20)", async () => {
    fake.getCharacterSheetFn = () =>
      Promise.resolve(
        vm({
          characterKind: 'enemy',
          experiencePoints: 0,
          challengeRating: '1',
          xpValue: 200,
          isMaster: false,
        }),
      );
    const el = (await render()).nativeElement as HTMLElement;

    expect(ddAfter(el, 'XP ao derrotar')).toBeNull();
    expect(ddAfter(el, 'Nível de desafio')).toBeNull();
  });

  it('listens for XP only while the campaign has an open session, and stops when the page goes away', async () => {
    fake.getCharacterSheetFn = () => Promise.resolve(vm());
    openSessions.set([]);
    const fixture = await render();
    // No open session: it follows nothing.
    expect(xpWatcher.follow).toHaveBeenLastCalledWith(
      null,
      expect.any(Function),
      expect.any(Function),
      expect.any(Function),
      expect.any(Function),
    );

    openSessions.set([
      {
        sessionId: 's1',
        campaignId: 'camp-1',
        campaignName: 'Mirathel',
        sessionNumber: 5,
        startedAt: new Date(),
        isMaster: false,
      },
    ]);
    fixture.detectChanges();
    await fixture.whenStable();
    expect(xpWatcher.follow).toHaveBeenLastCalledWith(
      'camp-1',
      expect.any(Function),
      expect.any(Function),
      expect.any(Function),
      expect.any(Function),
    );

    fixture.destroy();
    expect(xpWatcher.follow).toHaveBeenLastCalledWith(null, expect.any(Function));
  });

  it("does not listen for an NPC's sheet: it has no XP to keep fresh", async () => {
    fake.getCharacterSheetFn = () =>
      Promise.resolve(vm({ characterKind: 'enemy', experiencePoints: 0 }));
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
    await render();
    expect(xpWatcher.follow).not.toHaveBeenCalledWith(
      'camp-1',
      expect.any(Function),
      expect.any(Function),
    );
  });

  it('reads the character again, without the loading state, when the master gives XP', async () => {
    let xp = 2366;
    fake.getCharacterSheetFn = () =>
      Promise.resolve(vm({ experiencePoints: xp, nextLevelXp: 2700 }));
    openSessions.set([
      {
        sessionId: 's1',
        campaignId: 'camp-1',
        campaignName: 'Mirathel',
        sessionNumber: 5,
        startedAt: new Date(),
        isMaster: false,
      },
    ]);
    const fixture = await render();
    const el = fixture.nativeElement as HTMLElement;
    expect(block(el)!.querySelector('.xp__n')?.textContent?.trim()).toBe(`2.366${nbsp}XP`);

    // `xp_changed` arrives: the follower's callback is what the page handed it.
    xp = 2716;
    const onChange = xpWatcher.follow.mock.calls.at(-1)![1];
    onChange();
    await flush();
    await fixture.whenStable();
    fixture.detectChanges();

    expect(block(el)!.querySelector('.xp__n')?.textContent?.trim()).toBe(`2.716${nbsp}XP`);
    expect(el.textContent).not.toContain('Carregando a ficha');
  });

  it("tells the creatures panel when this character's vitals or the combat change (a Wild Shape form ends that way), not another character's", async () => {
    fake.getCharacterSheetFn = () =>
      Promise.resolve(vm({ sheet: fullSheet({ hasWildShape: true }) }));
    openSessions.set([
      {
        sessionId: 's1',
        campaignId: 'camp-1',
        campaignName: 'Mirathel',
        sessionNumber: 5,
        startedAt: new Date(),
        isMaster: false,
      },
    ]);
    const fixture = await render();
    const ticks = () =>
      fixture.debugElement.query(By.directive(CreaturesPanel)).componentInstance.formReload();
    const onForm = xpWatcher.follow.mock.calls.at(-1)![4]!;
    const bump = async (who: string | null) => {
      onForm(who);
      fixture.detectChanges();
      await fixture.whenStable();
    };
    expect(ticks()).toBe(0);
    await bump('char-2');
    expect(ticks()).toBe(0);
    await bump('char-1');
    expect(ticks()).toBe(1);
    await bump(null);
    expect(ticks()).toBe(2);
  });

  it('reads the character again when the table\'s content changes (content_changed, "A classe mudou"), on the same stream', async () => {
    let reads = 0;
    fake.getCharacterSheetFn = () => {
      reads++;
      return Promise.resolve(vm());
    };
    openSessions.set([
      {
        sessionId: 's1',
        campaignId: 'camp-1',
        campaignName: 'Mirathel',
        sessionNumber: 5,
        startedAt: new Date(),
        isMaster: false,
      },
    ]);
    const fixture = await render();
    const before = reads;
    const onContent = xpWatcher.follow.mock.calls.at(-1)![3]!;
    onContent();
    await flush();
    await fixture.whenStable();
    fixture.detectChanges();
    expect(reads).toBe(before + 1);
    expect((fixture.nativeElement as HTMLElement).textContent).not.toContain('Carregando a ficha');
  });
});

describe('CharacterSheetPage: answers that arrive late', () => {
  let fake: FakeCharacterSheetSource;
  let params: BehaviorSubject<ReturnType<typeof convertToParamMap>>;

  function deferred<T>() {
    let resolve!: (v: T) => void;
    const promise = new Promise<T>((r) => (resolve = r));
    return { promise, resolve };
  }

  beforeEach(() => {
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
    xpWatcher.follow.mockClear();
    params = new BehaviorSubject(convertToParamMap({ id: 'camp-1', characterId: 'char-1' }));
    TestBed.configureTestingModule({
      imports: [CharacterSheetPage],
      providers: [
        { provide: CharacterSheetSource, useClass: FakeCharacterSheetSource },
        { provide: ActivatedRoute, useValue: { paramMap: params } },
        ...xpProviders(),
      ],
    });
    fake = TestBed.inject(CharacterSheetSource) as unknown as FakeCharacterSheetSource;
  });

  afterEach(() => openSessions.set([]));

  it('keeps the dead sheet when a reload that began before the death lands afterwards', async () => {
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

    xpWatcher.follow.mock.calls.at(-1)![1]();
    expect(reads).toBe(2);
    buttonWithText(el, 'Marcar como morto')!.click();
    fixture.detectChanges();
    buttonWithText(el, 'Confirmar morte')!.click();
    await flush();
    fixture.detectChanges();
    expect(el.querySelector('[aria-label="Estado do personagem"]')?.textContent).toContain('Morto');

    reload.resolve(alive);
    await flush();
    fixture.detectChanges();

    expect(el.querySelector('[aria-label="Estado do personagem"]')?.textContent).toContain('Morto');
    expect(el.textContent).toContain('Morreu em 30/09/2026');
    expect(buttonWithText(el, 'Marcar como morto')).toBeUndefined();
  });

  it('takes the answer of a reload that is still the latest', async () => {
    let reads = 0;
    fake.getCharacterSheetFn = () =>
      Promise.resolve(vm({ name: ++reads === 1 ? 'Antes' : 'Depois' }));
    const fixture = TestBed.createComponent(CharacterSheetPage);
    fixture.detectChanges();
    await flush();
    xpWatcher.follow.mock.calls.at(-1)![1]();
    await flush();
    fixture.detectChanges();
    expect((fixture.nativeElement as HTMLElement).textContent).toContain('Depois');
  });

  it('shows the sheet of the route, not a slower answer for the sheet the person left', async () => {
    const a = deferred<CharacterSheetVm>();
    const b = deferred<CharacterSheetVm>();
    fake.getCharacterSheetFn = (_c, id) => (id === 'char-A' ? a.promise : b.promise);
    params.next(convertToParamMap({ id: 'camp-1', characterId: 'char-A' }));

    const fixture = TestBed.createComponent(CharacterSheetPage);
    fixture.detectChanges();
    // The router reuses the page for the same route with another parameter.
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

  it('drops a quiet reload of the sheet the person left', async () => {
    const reloadA = deferred<CharacterSheetVm>();
    let reads = 0;
    fake.getCharacterSheetFn = (_c, id) => {
      if (id === 'char-B') return Promise.resolve(vm({ id: 'char-B', name: 'Bravo' }));
      return reads++ === 0 ? Promise.resolve(vm({ id: 'char-A', name: 'Alfa' })) : reloadA.promise;
    };
    params.next(convertToParamMap({ id: 'camp-1', characterId: 'char-A' }));

    const fixture = TestBed.createComponent(CharacterSheetPage);
    fixture.detectChanges();
    await flush();
    fixture.detectChanges();
    const el = fixture.nativeElement as HTMLElement;
    expect(el.textContent).toContain('Alfa');

    xpWatcher.follow.mock.calls.at(-1)![1]();
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
