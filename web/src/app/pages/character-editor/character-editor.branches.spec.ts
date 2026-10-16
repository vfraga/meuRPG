import { Injectable } from '@angular/core';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap, provideRouter, Router } from '@angular/router';
import { Code, ConnectError } from '@connectrpc/connect';
import { of } from 'rxjs';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { create } from '@bufbuild/protobuf';

import {
  CharacterBlockedReason,
  CharacterBlockedSchema,
  ChoiceGroupSchema,
  ChoiceKind,
  ChoiceOptionSchema,
  ChoiceOrigin,
  ChoiceSchema,
} from '../../../gen/meurpg/characters/v1/characters_pb';
import { fakeContentWatcher } from '../../core/content/content-testing';
import { CharacterEditor } from './character-editor';
import {
  AbilityTableVm,
  CharacterEditorSource,
  CharacterForEdit,
  CharacterFormValue,
  CharacterPreviewVm,
  ChoicesPreviewVm,
  ClassOptionVm,
  CreateCharacterInput,
  PreviewCharacterInput,
  RulesCatalogVm,
  SpellOptionVm,
  SubclassOptionVm,
  UpdateCharacterInput,
} from './character-editor.types';
import { nextTurn } from '../../../test-settle';

// `character-editor.ts` off the happy path: the page before its catalog arrives, the texts and numbers it derives, the
// class blocks, the toggles, the ways a save or a read fails, and the server's answers for the draft. It reads the
// component through its protected members, as the specs next to this one do.

/** The component seen from a spec: protected members and all. */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
type Cmp = any;

const SRD = { fromTable: false, archived: false, off: false };

const cls = (
  over: Partial<ClassOptionVm> & Pick<ClassOptionVm, 'key' | 'namePt'>,
): ClassOptionVm => ({
  hitDie: 8,
  isCaster: false,
  preparation: null,
  subclasses: [],
  subclassLevel: 3,
  spellcastingFirstLevel: 0,
  maxSpellLevelByLevel: [],
  skillChoose: 2,
  savingThrows: ['str', 'wis'],
  spellListClassKey: '',
  ...SRD,
  ...over,
});
const sub = (
  over: Partial<SubclassOptionVm> & Pick<SubclassOptionVm, 'key' | 'namePt'>,
): SubclassOptionVm => ({ casting: null, alwaysPrepared: [], ...SRD, ...over });
const spell = (key: string, namePt: string, level: number, classKeys: string[]): SpellOptionVm => ({
  key,
  namePt,
  level,
  classKeys,
  ...SRD,
});

const CIRCLES = [1, 1, 2, 2, 3, 3, 4, 4, 5, 5, 6, 6, 7, 7, 8, 8, 9, 9, 9, 9];

function catalog(): RulesCatalogVm {
  return {
    races: [
      {
        key: 'race:gnome',
        namePt: 'Gnomo',
        constitutionBonus: 1,
        choiceBonuses: [],
        skillKeys: ['skill:arcana'],
        subraces: [
          {
            key: 'subrace:rock-gnome',
            namePt: 'Gnomo da Rocha',
            constitutionBonus: 1,
            skillKeys: ['skill:history'],
            ...SRD,
          },
        ],
        ...SRD,
      },
      {
        key: 'race:human',
        namePt: 'Humano',
        constitutionBonus: 0,
        choiceBonuses: [],
        skillKeys: ['skill:religion'],
        subraces: [],
        ...SRD,
      },
    ],
    classes: [
      cls({
        key: 'class:wizard',
        namePt: 'Mago',
        hitDie: 6,
        isCaster: true,
        preparation: 'spellbook',
        spellcastingFirstLevel: 1,
        maxSpellLevelByLevel: CIRCLES,
        spellListClassKey: 'class:wizard',
        subclassLevel: 2,
        savingThrows: ['int', 'wis'],
        subclasses: [sub({ key: 'subclass:evocation', namePt: 'Evocação' })],
      }),
      cls({
        key: 'class:cleric',
        namePt: 'Clérigo',
        isCaster: true,
        preparation: 'prepared',
        spellcastingFirstLevel: 1,
        maxSpellLevelByLevel: CIRCLES,
        spellListClassKey: 'class:cleric',
        subclassLevel: 1,
        subclasses: [
          // The domain prepares a spell the catalog does not have at all (retired and gone): it is skipped.
          sub({
            key: 'subclass:life',
            namePt: 'Vida',
            alwaysPrepared: [
              { spellKey: 'spell:ghost', classLevel: 1 },
              { spellKey: 'spell:bless', classLevel: 1 },
            ],
          }),
        ],
      }),
      cls({
        key: 'class:bard',
        namePt: 'Bardo',
        isCaster: true,
        preparation: 'known',
        spellcastingFirstLevel: 1,
        maxSpellLevelByLevel: CIRCLES,
        spellListClassKey: 'class:bard',
      }),
      cls({ key: 'class:fighter', namePt: 'Guerreiro', skillChoose: 1, savingThrows: [] }),
      cls({ key: 'class:rogue', namePt: 'Ladino', skillChoose: 0 }),
    ],
    backgrounds: [
      {
        key: 'background:acolyte',
        namePt: 'Acólito',
        equipmentPt: 'Um símbolo sagrado',
        skillKeys: ['skill:religion'],
        ...SRD,
      },
    ],
    skills: [
      { key: 'skill:arcana', namePt: 'Arcanismo', ability: 'int' },
      { key: 'skill:history', namePt: 'História', ability: 'int' },
      { key: 'skill:religion', namePt: 'Religião', ability: 'int' },
      { key: 'skill:nature', namePt: 'Natureza', ability: 'int' },
    ],
    armor: [],
    weapons: [],
    spells: [
      spell('spell:fire-bolt', 'Raio de Fogo', 0, ['class:wizard']),
      spell('spell:shield', 'Escudo Arcano', 1, ['class:wizard']),
      spell('spell:bless', 'Bênção', 1, ['class:cleric']),
      spell('spell:vicious-mockery', 'Zombaria Viciosa', 1, ['class:bard']),
    ],
    viewerIsMaster: false,
    toolsAndLanguages: [
      { key: 'proficiency:thieves-tools', namePt: 'Ferramentas de ladrão', kind: 'tool' },
      { key: 'language:elvish', namePt: 'Élfico', kind: 'language' },
    ],
    challengeRatings: [{ rating: '0', xp: 10 }],
  };
}

function savedFull(over: Partial<CharacterFormValue> = {}): CharacterFormValue {
  return {
    name: 'Pensantus',
    race: 'race:gnome',
    subrace: '',
    className: 'class:wizard',
    subclassName: '',
    customSubclassName: '',
    level: 3,
    background: 'background:acolyte',
    customBackgroundName: '',
    customBackgroundSkills: null,
    customBackgroundProficiencies: [],
    customBackgroundFeatureName: '',
    customBackgroundFeatureText: '',
    customBackgroundEquipment: '',
    extraClasses: [],
    skillProficiencies: [],
    expertiseSkillKeys: [],
    abilities: { str: 8, dex: 14, con: 16, int: 18, wis: 12, cha: 10 },
    extraAbilityBonuses: { str: 0, dex: 0, con: 0, int: 0, wis: 0, cha: 0 },
    hitPointsMethod: 'average',
    hitPointsRolls: [],
    isCaster: true,
    cantrips: [],
    spellsKnown: [],
    spellsPrepared: [],
    armor: '',
    shield: false,
    weapons: [],
    equipmentText: '',
    languagesText: '',
    toolProficienciesText: '',
    experiencePoints: 0,
    challengeRating: '',
    xpValue: 0,
    portraitImageId: '',
    alignment: '',
    customFeaturesText: '',
    featureChoiceKeys: [],
    featureChoiceText: {},
    cuttingWordsAsk: 'only-attacks',
    ...over,
  };
}

const forEdit = (over: Partial<CharacterForEdit> = {}): CharacterForEdit => ({
  kind: 'player',
  revision: 4,
  blocked: null,
  sheetLocked: false,
  full: savedFull(),
  basic: null,
  ...over,
});

@Injectable()
class FakeSource {
  catalogFn: (campaignId: string, characterId?: string) => Promise<RulesCatalogVm> = () =>
    Promise.resolve(catalog());
  catalogCalls: (string | undefined)[] = [];
  editFn: () => Promise<CharacterForEdit> = () => Promise.reject(new Error('not stubbed'));
  abilityTable: AbilityTableVm | null = null;
  abilityTableCalls = 0;
  previews: PreviewCharacterInput[] = [];
  previewFn: (input: PreviewCharacterInput) => Promise<CharacterPreviewVm> = () =>
    Promise.reject(new Error('not stubbed'));
  choicePreviews: PreviewCharacterInput[] = [];
  choicesFn: (input: PreviewCharacterInput) => Promise<ChoicesPreviewVm> = () =>
    Promise.resolve({ groups: [], done: 0, total: 0, notOffered: [], spellSources: [] });
  created: CreateCharacterInput[] = [];
  createFn: (input: CreateCharacterInput) => Promise<{ characterId: string }> = () =>
    Promise.resolve({ characterId: 'new-char' });
  updated: UpdateCharacterInput[] = [];
  updateFn: () => Promise<{ revision: number }> = () => Promise.resolve({ revision: 5 });

  loadCatalog(campaignId: string, characterId?: string) {
    this.catalogCalls.push(characterId);
    return this.catalogFn(campaignId, characterId);
  }
  loadSpellDetails() {
    return Promise.reject(new Error('unused'));
  }
  loadCharacterForEdit() {
    return this.editFn();
  }
  loadAbilityTable() {
    this.abilityTableCalls++;
    return Promise.resolve(this.abilityTable);
  }
  rollAbilityScores() {
    return Promise.reject(new Error('unused'));
  }
  previewCharacter(input: PreviewCharacterInput) {
    this.previews.push(input);
    return this.previewFn(input);
  }
  previewChoices(input: PreviewCharacterInput) {
    this.choicePreviews.push(input);
    return this.choicesFn(input);
  }
  createCharacter(input: CreateCharacterInput) {
    this.created.push(input);
    return this.createFn(input);
  }
  updateCharacter(input: UpdateCharacterInput) {
    this.updated.push(input);
    return this.updateFn();
  }
}

type Params = Record<string, string>;

function configure(
  params: Params,
  data: Record<string, unknown> = {},
): { fake: FakeSource; watcher: ReturnType<typeof fakeContentWatcher> } {
  const watcher = fakeContentWatcher();
  TestBed.overrideComponent(CharacterEditor, { set: { providers: [watcher.provider] } });
  TestBed.configureTestingModule({
    imports: [CharacterEditor],
    providers: [
      provideRouter([]),
      { provide: CharacterEditorSource, useClass: FakeSource },
      {
        provide: ActivatedRoute,
        useValue: { paramMap: of(convertToParamMap(params)), snapshot: { data } },
      },
    ],
  });
  return { fake: TestBed.inject(CharacterEditorSource) as unknown as FakeSource, watcher };
}

async function render(): Promise<{
  fixture: ComponentFixture<CharacterEditor>;
  el: HTMLElement;
  cmp: Cmp;
}> {
  const fixture = TestBed.createComponent(CharacterEditor);
  fixture.detectChanges();
  await nextTurn();
  await fixture.whenStable();
  fixture.detectChanges();
  return { fixture, el: fixture.nativeElement as HTMLElement, cmp: fixture.componentInstance };
}

/** A render of a new player character with the catalog above. */
async function ready(params: Params = { id: 'camp-1' }, data: Record<string, unknown> = {}) {
  const { fake, watcher } = configure(params, data);
  return { fake, watcher, ...(await render()) };
}

async function settleView(fixture: ComponentFixture<CharacterEditor>): Promise<void> {
  fixture.detectChanges();
  await nextTurn();
  await fixture.whenStable();
  fixture.detectChanges();
}

function blocked(reason: CharacterBlockedReason, contentKey = ''): ConnectError {
  return new ConnectError('x', Code.FailedPrecondition, undefined, [
    { desc: CharacterBlockedSchema, value: create(CharacterBlockedSchema, { reason, contentKey }) },
  ]);
}

/** The pause the editor lets the draft rest before it asks the server. */
const PAUSE = 350;

const fill = (cmp: Cmp, over: Record<string, unknown> = {}) =>
  cmp.fullForm.patchValue({
    name: 'Tharn',
    race: 'race:gnome',
    className: 'class:wizard',
    background: 'background:acolyte',
    ...over,
  });

afterEach(() => {
  vi.useRealTimers();
});

describe('CharacterEditor, before the catalog arrives', () => {
  async function loading() {
    const { fake, watcher } = configure({ id: 'camp-1' });
    fake.catalogFn = () => new Promise(() => undefined);
    return { fake, watcher, ...(await render()) };
  }

  it('shows nothing derived from a catalog it does not have', async () => {
    const { cmp } = await loading();
    expect(cmp.state().status).toBe('loading');
    expect(cmp.availableSubraces()).toEqual([]);
    expect(cmp.views.nameOfKey('race:gnome')).toBeUndefined();
    expect(cmp.raceOptions()).toEqual([]);
    expect(cmp.classOptions()).toEqual([]);
    expect(cmp.backgroundOptions()).toEqual([]);
    expect(cmp.subclassOptions()).toEqual([]);
    expect(cmp.raceUnlisted()).toBe(false);
    expect(cmp.classUnlisted()).toBe(false);
    expect(cmp.backgroundUnlisted()).toBe(false);
    expect(cmp.selectedClass()).toBeUndefined();
    expect(cmp.selectedRace()).toBeUndefined();
    expect(cmp.selectedSubrace()).toBeUndefined();
    expect(cmp.selectedSubclass()).toBeUndefined();
    expect(cmp.selectedBackgroundVm()).toBeUndefined();
    expect(cmp.levelDice()).toEqual([]);
    expect(cmp.sections()).toEqual([]);
    expect(cmp.views.availableCantrips()).toEqual([]);
    expect(cmp.views.availableSpells()).toEqual([]);
    expect(cmp.sectionViews()).toEqual([]);
    expect(cmp.magiasLink()).toEqual([]);
    expect(cmp.choiceBonusesHint()).toBe('');
    expect(cmp.backgroundEquipment()).toBe('');
    expect(cmp.toolOptions()).toEqual([]);
    expect(cmp.languageOptions()).toEqual([]);
    expect(cmp.skillNote()).toBe('');
    expect(cmp.choicesSubtitle()).toBe('');
    expect(cmp.choicesBlocked()).toBe(false);
    expect(cmp.master()).toBe(false);
    expect(cmp.hpRule()).toBe('player_chooses');
  });

  it('shows the page texts of a form that is not open yet', async () => {
    const { cmp } = await loading();
    expect(cmp.pageTitle()).toBe('Criar personagem');
    expect(cmp.createLabel()).toBe('Criar personagem');
    expect(cmp.reserved()).toBe(false);
    expect(cmp.lead()).toBe('');
    expect(cmp.actionsNote()).toBe('');
    expect(cmp.cancelLink()).toEqual(['/']);
    expect(cmp.invalidSummary()).toBe('');
    // The Constitution is the typed score alone: no race, no subrace, no manual bonus.
    expect(cmp.finalConstitution()).toBe(10);
  });

  it('does nothing when asked to save, to read again or to describe a spell', async () => {
    const { cmp, fake } = await loading();
    await cmp.submit();
    await cmp.refreshCatalog();
    await cmp.rereadAbilityTable();
    cmp.describeSpell({ key: 'spell:shield', namePt: 'Escudo Arcano' });
    expect(fake.created).toEqual([]);
    expect(fake.catalogCalls.length).toBe(1);
    expect(fake.abilityTableCalls).toBe(1);
  });

  it('refuses to ask the server for a draft while the page is not ready', async () => {
    const { cmp } = await loading();
    await expect(cmp.server.hitPoints.ask()).rejects.toThrow('the page is not ready');
    await expect(cmp.server.choices.ask()).rejects.toThrow('the page is not ready');
  });

  it('keeps every pick when it has no catalog to judge them by', async () => {
    const { cmp } = await loading();
    expect(cmp.draft.intersectWithAvailable(new Set(['spell:any']), [])).toEqual(['spell:any']);
  });
});

describe('CharacterEditor, the texts and numbers it derives', () => {
  it('names the draft on the Escolhas step: name, class and level, then the subrace or the race', async () => {
    const { cmp } = await ready();
    expect(cmp.choicesSubtitle()).toBe('');
    cmp.fullForm.patchValue({ name: '  Tharn ' });
    expect(cmp.choicesSubtitle()).toBe('Tharn');
    cmp.fullForm.patchValue({ className: 'class:wizard', level: 3, race: 'race:gnome' });
    expect(cmp.choicesSubtitle()).toBe('Tharn · Mago 3 · Gnomo');
    cmp.fullForm.patchValue({ subrace: 'subrace:rock-gnome' });
    expect(cmp.choicesSubtitle()).toBe('Tharn · Mago 3 · Gnomo da Rocha');
  });

  it('adds the race, the subrace and the manual bonus to the typed Constitution', async () => {
    const { cmp } = await ready();
    cmp.fullForm.patchValue({
      race: 'race:gnome',
      subrace: 'subrace:rock-gnome',
      abilities: { str: 10, dex: 10, con: 14, int: 10, wis: 10, cha: 10 },
      extraAbilityBonuses: { str: 0, dex: 0, con: 2, int: 0, wis: 0, cha: 0 },
    });
    expect(cmp.finalConstitution()).toBe(14 + 1 + 1 + 2);
  });

  it('heads a caster section by its class, its subclass and how far its spells reach', async () => {
    const { cmp } = await ready();
    expect(cmp.sectionHeading({ namePt: 'Clérigo', subclassNamePt: '', maxCircle: null })).toBe(
      'Clérigo',
    );
    expect(cmp.sectionHeading({ namePt: 'Clérigo', subclassNamePt: 'Vida', maxCircle: 0 })).toBe(
      'Clérigo · Vida · só truques',
    );
    expect(cmp.sectionHeading({ namePt: 'Mago', subclassNamePt: '', maxCircle: 2 })).toBe(
      'Mago · até o 2º nível de magia',
    );
  });

  it('says what the first class asks of the skills, with or without saving throws and in the singular', async () => {
    const { cmp } = await ready();
    cmp.fullForm.patchValue({ className: 'class:wizard' });
    expect(cmp.skillNote()).toBe(
      'O Mago escolhe 2 perícias ao começar e dá proficiência nos testes de resistência de Inteligência e Sabedoria.',
    );
    cmp.fullForm.patchValue({ className: 'class:fighter' });
    expect(cmp.skillNote()).toBe('O Guerreiro escolhe 1 perícia ao começar.');
    cmp.fullForm.patchValue({ className: 'class:rogue' });
    expect(cmp.skillNote()).toBe('');
  });

  it('flags a race and a class the catalog does not list, but not a custom background', async () => {
    const { cmp } = await ready();
    cmp.fullForm.patchValue({
      race: 'race:old',
      className: 'class:old',
      background: 'background:old',
    });
    expect(cmp.raceUnlisted()).toBe(true);
    expect(cmp.classUnlisted()).toBe(true);
    expect(cmp.backgroundUnlisted()).toBe(true);
    cmp.fullForm.patchValue({ background: 'custom' });
    expect(cmp.backgroundUnlisted()).toBe(false);
  });

  it('takes the skills of a custom background from the two the person picked', async () => {
    const { cmp } = await ready();
    cmp.fullForm.patchValue({ race: 'race:gnome', background: 'custom' });
    cmp.toggleCustomBackgroundSkill('skill:religion');
    expect(cmp.grantedSkills().get('skill:religion')).toBe('do antecedente');
    expect(cmp.grantedSkills().get('skill:arcana')).toBe('da raça');
  });
});

describe('CharacterEditor, the class blocks', () => {
  it('writes a change of the first block into the form, field by field', async () => {
    const { cmp } = await ready();
    cmp.changeBlock(0, { classKey: 'class:cleric' });
    cmp.changeBlock(0, { level: 4 });
    cmp.changeBlock(0, { subclassKey: 'subclass:life' });
    cmp.changeBlock(0, { customSubclassName: 'Outra' });
    expect(cmp.fullForm.getRawValue()).toMatchObject({
      className: 'class:cleric',
      level: 4,
      subclassName: 'subclass:life',
      customSubclassName: 'Outra',
    });
    cmp.changeBlock(0, {});
    expect(cmp.fullForm.getRawValue().level).toBe(4);
  });

  it('changes only the extra class the block names', async () => {
    const { cmp } = await ready();
    cmp.addClass();
    cmp.addClass();
    cmp.changeBlock(1, { classKey: 'class:fighter', level: 2 });
    expect(cmp.picks.extraClasses()).toEqual([
      { classKey: 'class:fighter', level: 2, subclassKey: '', customSubclassName: '' },
      { classKey: '', level: 1, subclassKey: '', customSubclassName: '' },
    ]);
    expect(cmp.multiclass()).toBe(true);
    expect(cmp.otherClassKeys(1)).toEqual([]);
    cmp.fullForm.patchValue({ className: 'class:wizard' });
    expect(cmp.otherClassKeys(1)).toEqual(['class:wizard']);
    cmp.removeClass(1);
    expect(cmp.picks.extraClasses().length).toBe(1);
  });

  it('lists what the blocks still lack, in words, and holds the save', async () => {
    const { cmp, fake } = await ready();
    fill(cmp, { level: 20 });
    cmp.addClass();
    cmp.changeBlock(1, { level: 25 });
    expect(cmp.draft.classProblems()).toEqual([
      'Classe 2',
      'Nível da classe 2',
      'o nível total vai até 20',
    ]);
    await cmp.submit();
    expect(fake.created).toEqual([]);
    expect(cmp.invalidSummary()).toContain('Classe 2');
  });

  it('lists the levels whose hit-point roll is missing or does not fit the die', async () => {
    const { cmp } = await ready();
    fill(cmp, { level: 3, hitPointsMethod: 'rolled' });
    expect(cmp.draft.rollProblems()).toEqual([
      'Dado de vida do nível 2',
      'Dado de vida do nível 3',
    ]);
    cmp.hitPointsRolls.set([4, 9]);
    expect(cmp.draft.rollProblems()).toEqual(['Dado de vida do nível 3']);
    cmp.hitPointsRolls.set([4, 6]);
    expect(cmp.draft.rollProblems()).toEqual([]);
    // A class not chosen yet has no die: nothing to roll against.
    cmp.fullForm.patchValue({ className: '' });
    expect(cmp.draft.rollProblems()).toEqual([]);
  });

  it('rolls a level of a block with no class against the first die', async () => {
    const { cmp } = await ready();
    fill(cmp, { level: 1, hitPointsMethod: 'rolled' });
    cmp.addClass();
    expect(cmp.rollsNeeded()).toBe(1);
    expect(cmp.draft.rollProblems()).toEqual(['Dado de vida do nível 2']);
  });
});

describe('CharacterEditor, the spell and skill picks', () => {
  it('keeps the search of an extra list apart from the others', async () => {
    const { cmp } = await ready();
    expect(cmp.spellFilterOf('0:patron')).toBe('');
    cmp.setExtraFilter('0:patron', 'fog');
    expect(cmp.spellFilterOf('0:patron')).toBe('fog');
    const spells = [spell('a', 'Fogo', 1, []), spell('b', 'Gelo', 1, [])];
    expect(cmp.filterSpells(spells, '0:patron').map((s: SpellOptionVm) => s.key)).toEqual(['a']);
    expect(cmp.filterSpells(spells, '0:other').length).toBe(2);
  });

  it('shows only the always-prepared spells the catalog has, and takes them out of the lists', async () => {
    const { cmp } = await ready();
    fill(cmp, { className: 'class:cleric', level: 3, subclassName: 'subclass:life' });
    const [view] = cmp.sectionViews();
    expect(view.always.map((s: SpellOptionVm) => s.key)).toEqual(['spell:bless']);
    expect(view.prepared.all.map((s: SpellOptionVm) => s.key)).not.toContain('spell:bless');
  });

  it('keeps a pick the catalog does not know and drops one that left the lists', async () => {
    const { cmp } = await ready();
    fill(cmp, { className: 'class:wizard', level: 3 });
    cmp.selectedSpellsKnown.set(new Set(['spell:unheard-of', 'spell:shield', 'spell:bless']));
    expect(cmp.draft.build().spellsKnown.sort()).toEqual(['spell:shield', 'spell:unheard-of']);
  });

  it('counts the cantrips the server says the class knows', async () => {
    const { cmp, fixture, fake } = await ready();
    fake.previewFn = () =>
      Promise.resolve({
        hitPointsMax: 10,
        hitPointsFromEffects: 0,
        spellcasting: [
          { classKey: 'class:wizard', cantripsKnown: 3, spellsKnown: 0, preparedMax: 5 },
        ],
      });
    vi.useFakeTimers();
    fill(cmp, { level: 3 });
    fixture.detectChanges();
    await vi.advanceTimersByTimeAsync(PAUSE);
    fixture.detectChanges();
    const [view] = cmp.sectionViews();
    expect(view.cantripsLimit).toBe(3);
    expect(view.preparedLimit).toBe(5);
    expect(view.knownLimit).toBeNull();
  });

  it('does not toggle a skill the race gives, and takes one the race does not', async () => {
    const { cmp } = await ready();
    fill(cmp);
    cmp.toggleSkill('skill:arcana');
    expect(cmp.picks.selectedSkills().size).toBe(0);
    cmp.toggleSkill('skill:nature');
    expect(cmp.picks.selectedSkills().has('skill:nature')).toBe(true);
    // Expertise counts the skills the race gives as proficient.
    cmp.toggleExpertise('skill:arcana');
    expect(cmp.expertiseSkills().has('skill:arcana')).toBe(true);
  });

  it('counts a skill the race and the background both give once, as the race gives it', async () => {
    const { cmp } = await ready();
    fill(cmp, { race: 'race:human', background: 'background:acolyte' });
    expect([...cmp.grantedSkills()]).toEqual([['skill:religion', 'da raça']]);
  });

  it('counts the spells a known caster knows against the number the server gives', async () => {
    const { cmp, fixture, fake } = await ready();
    fake.previewFn = () =>
      Promise.resolve({
        hitPointsMax: 8,
        hitPointsFromEffects: 0,
        spellcasting: [
          { classKey: 'class:bard', cantripsKnown: 2, spellsKnown: 4, preparedMax: 0 },
        ],
      });
    vi.useFakeTimers();
    fill(cmp, { className: 'class:bard' });
    fixture.detectChanges();
    await vi.advanceTimersByTimeAsync(PAUSE);
    fixture.detectChanges();
    const [view] = cmp.sectionViews();
    expect(view.knownLimit).toBe(4);
    expect(view.preparedLimit).toBeNull();
  });
});

describe('CharacterEditor, saving', () => {
  it('lists what a minion still lacks instead of saving', async () => {
    const { cmp, fake } = await ready({ id: 'camp-1', kind: 'minion' });
    await cmp.submit();
    expect(fake.created).toEqual([]);
    expect(cmp.saveState().status).toBe('idle');
    expect(cmp.invalidSummary()).not.toBe('');
  });

  it('opens "Bônus manuais" when the bonus is what is wrong', async () => {
    const { cmp, fake } = await ready();
    fill(cmp);
    cmp.fullForm.patchValue({
      extraAbilityBonuses: { str: 99, dex: 0, con: 0, int: 0, wis: 0, cha: 0 },
    });
    await cmp.submit();
    expect(fake.created).toEqual([]);
    expect(cmp.bonusesOpen()).toBe(true);
  });

  it('opens no step when nothing is wrong, or when there is no stepper to open it on', async () => {
    const { cmp } = await ready();
    fill(cmp);
    cmp.submitting.openFirstInvalidStep();
    expect(cmp.bonusesOpen()).toBe(false);
    cmp.fullForm.patchValue({
      extraAbilityBonuses: { str: 99, dex: 0, con: 0, int: 0, wis: 0, cha: 0 },
    });
    cmp.stepper = undefined;
    cmp.submitting.openFirstInvalidStep();
    expect(cmp.bonusesOpen()).toBe(true);
  });

  it('names an archived option by its name in the refusal', async () => {
    const { cmp, fake } = await ready();
    fill(cmp);
    fake.createFn = () =>
      Promise.reject(blocked(CharacterBlockedReason.ARCHIVED_CONTENT, 'race:gnome'));
    await cmp.submit();
    expect(cmp.saveState().message).toContain('Gnomo');
  });

  it('says a switched-off spell above the buttons, since no field holds it', async () => {
    const { cmp, fake } = await ready();
    fill(cmp);
    fake.createFn = () =>
      Promise.reject(blocked(CharacterBlockedReason.SWITCHED_OFF_CONTENT, 'spell:shield'));
    await cmp.submit();
    expect(cmp.saveState().status).toBe('error');
    expect(cmp.saveState().message).toContain('Escudo Arcano');
    expect(fake.catalogCalls.length).toBe(1);
  });

  it('sends the key of a reserved character as made for a player, and goes to the campaign', async () => {
    const { cmp, fake, fixture } = await ready({ id: 'camp-1' }, { reserved: true });
    const navigate = vi.spyOn(TestBed.inject(Router), 'navigate').mockResolvedValue(true);
    fill(cmp);
    await cmp.submit();
    expect(fake.created[0].forPlayer).toBe(true);
    expect(navigate).toHaveBeenCalledWith(['/campaigns', 'camp-1']);
    await settleView(fixture);
  });
});

describe('CharacterEditor, reading the page', () => {
  it('does nothing without a campaign in the route', async () => {
    const { cmp, fake } = await ready({});
    expect(fake.catalogCalls).toEqual([]);
    expect(cmp.pageTitle()).toBe('Ficha');
  });

  it('makes an enemy of an NPC kind the route does not know', async () => {
    const { cmp } = await ready({ id: 'camp-1', kind: 'dragon' });
    expect(cmp.state().kind).toBe('enemy');
    expect(cmp.fullForm.getRawValue()).toMatchObject({ challengeRating: '0', xpValue: 10 });
  });

  it('gives a story NPC no challenge rating and no XP', async () => {
    const { cmp } = await ready({ id: 'camp-1', kind: 'story' });
    expect(cmp.basicForm.getRawValue()).toMatchObject({ challengeRating: '', xpValue: 0 });
  });

  it('says the campaign could not be read, with a way back', async () => {
    const { fake, watcher } = configure({ id: 'camp-1' });
    void watcher;
    fake.catalogFn = () => Promise.reject(new ConnectError('x', Code.PermissionDenied));
    const { cmp } = await render();
    expect(cmp.state()).toMatchObject({
      status: 'error',
      message: 'Você não tem permissão para fazer isso.',
      backLink: ['/campaigns', 'camp-1'],
      backLabel: 'Voltar para a campanha',
    });
    expect(cmp.pageTitle()).toBe('Não foi possível abrir o formulário');
  });

  it('opens an existing sheet with the numbers of the saved prepared spells and the bard setting', async () => {
    const { fake } = configure({ id: 'camp-1', characterId: 'char-1' });
    fake.editFn = () =>
      Promise.resolve(
        forEdit({
          preparedMax: { 'class:wizard': 6 },
          featureKeys: ['feature:cutting-words'],
          grantedSpellKeys: ['spell:shield', 'spell:bless'],
        }),
      );
    const { cmp } = await render();
    expect(cmp.picks.preparedMaxByClass()).toEqual({ 'class:wizard': 6 });
    expect(cmp.hasCuttingWords()).toBe(true);
    expect(cmp.picks.grantedSpells().map((s: SpellOptionVm) => s.key)).toEqual([
      'spell:shield',
      'spell:bless',
    ]);
  });

  it('reads the table of ways to make scores again only when the sheet has one', async () => {
    const { fake } = configure({ id: 'camp-1', kind: 'enemy' });
    const { cmp } = await render();
    await cmp.rereadAbilityTable();
    expect(fake.abilityTableCalls).toBe(0);

    const second = await (async () => {
      TestBed.resetTestingModule();
      const again = configure({ id: 'camp-1' });
      again.fake.abilityTable = {
        standardArray: true,
        pointBuy: true,
        rolled4d6: true,
        typed: true,
        standardValues: [15, 14, 13, 12, 10, 8],
        pointBuyCosts: [0, 1, 2, 3, 4, 5, 7, 9],
        pointBuyMinScore: 8,
        pointBuyBudget: 27,
        typedMin: 3,
        typedMax: 18,
        hitPoints: 'player_chooses',
        physicalDice: false,
        diceForced: false,
        rolls: null,
      };
      return { ...again, ...(await render()) };
    })();
    expect(second.cmp.abilityTable()).not.toBeNull();
    // The table is gone from the server: the step keeps the one it has.
    second.fake.abilityTable = null;
    await second.cmp.rereadAbilityTable();
    expect(second.cmp.abilityTable()).not.toBeNull();
  });

  it('starts a new player sheet at the XP of its level, and of level 1 when the level is not a number', async () => {
    const { fake } = configure({ id: 'camp-1' });
    fake.catalogFn = () =>
      Promise.resolve({ ...catalog(), xpMode: 'enemies', levelXp: [0, 300, 900] });
    const { cmp, fixture } = await render();
    cmp.fullForm.patchValue({ level: 3 });
    fixture.detectChanges();
    expect(cmp.fullForm.controls.experiencePoints.value).toBe(900);
    cmp.fullForm.patchValue({ level: 0 });
    fixture.detectChanges();
    expect(cmp.fullForm.controls.experiencePoints.value).toBe(0);
  });
});

describe('CharacterEditor, what the server says about the draft', () => {
  const style = create(ChoiceSchema, {
    key: 'choice:fighter-style',
    featureKey: 'feature:fighter-style',
    kind: ChoiceKind.OPTIONS,
    titlePt: 'Estilo de Luta',
    labelPt: 'Estilo de Luta',
    picks: 1,
    picked: [],
    missing: 1,
    options: [
      create(ChoiceOptionSchema, {
        key: 'defense',
        storedKey: 'feature:style-defense',
        namePt: 'DEFENSE',
        summaryPt: 'resumo',
      }),
    ],
  });
  const askStyle = (): Promise<ChoicesPreviewVm> =>
    Promise.resolve({
      groups: [
        create(ChoiceGroupSchema, {
          origin: ChoiceOrigin.CLASS,
          sourceNamePt: 'Guerreiro',
          choices: [style],
        }),
      ],
      done: 0,
      total: 1,
      notOffered: ['feature:gone'],
      spellSources: [],
    });

  it('asks as the master of a reserved character for a player, and as the owner of an edit', async () => {
    const { cmp, fixture, fake } = await ready({ id: 'camp-1' }, { reserved: true });
    vi.useFakeTimers();
    fill(cmp, { level: 3 });
    fixture.detectChanges();
    await vi.advanceTimersByTimeAsync(PAUSE);
    expect(fake.previews[0].forPlayer).toBe(true);
    expect(fake.previews[0].characterId).toBeNull();
    vi.useRealTimers();
    TestBed.resetTestingModule();

    const edit = configure({ id: 'camp-1', characterId: 'char-1' });
    edit.fake.editFn = () => Promise.resolve(forEdit());
    const again = await render();
    vi.useFakeTimers();
    again.cmp.fullForm.patchValue({ level: 4 });
    again.fixture.detectChanges();
    await vi.advanceTimersByTimeAsync(PAUSE);
    expect(edit.fake.previews.at(-1)).toMatchObject({ characterId: 'char-1', kind: 'player' });
    expect(edit.fake.choicePreviews.at(-1)?.characterId).toBe('char-1');
    expect(edit.fake.previews.at(-1)?.forPlayer).toBeUndefined();
  });

  it('forgets the picks and texts of a choice the draft no longer offers', async () => {
    const { cmp, fixture, fake } = await ready();
    fake.choicesFn = askStyle;
    cmp.picks.featureChoiceKeys.set(['feature:gone', 'feature:style-defense']);
    cmp.picks.featureChoiceText.set({
      'choice:fighter-style#0': 'kept',
      'choice:gone#0': 'dropped',
    });
    vi.useFakeTimers();
    fill(cmp, { className: 'class:fighter' });
    fixture.detectChanges();
    await vi.advanceTimersByTimeAsync(PAUSE);
    fixture.detectChanges();
    expect(cmp.picks.featureChoiceKeys()).toEqual(['feature:style-defense']);
    expect(cmp.picks.featureChoiceText()).toEqual({ 'choice:fighter-style#0': 'kept' });
  });

  it('leads a blocked save to the Escolhas step, also when it is already open', async () => {
    const { cmp, fixture, fake, el } = await ready();
    fake.choicesFn = askStyle;
    vi.useFakeTimers();
    fill(cmp, { className: 'class:fighter' });
    fixture.detectChanges();
    await vi.advanceTimersByTimeAsync(PAUSE);
    fixture.detectChanges();
    vi.useRealTimers();
    await settleView(fixture);
    expect(cmp.choicesBlocked()).toBe(true);
    const tab = Array.from(el.querySelectorAll<HTMLElement>('[role="tab"]')).find((t) =>
      t.textContent?.includes('Escolhas'),
    )!;
    tab.click();
    await settleView(fixture);
    await cmp.submit();
    await settleView(fixture);
    expect(fake.created).toEqual([]);
    expect((document.activeElement as HTMLElement).closest('[data-pending]')).not.toBeNull();
  });

  it('has nowhere to lead a blocked save when there is no stepper or no Escolhas step', async () => {
    const { cmp } = await ready();
    cmp.stepper = undefined;
    cmp.submitting.focusFirstPendingChoice();
    const { cmp: other } = await (async () => {
      TestBed.resetTestingModule();
      configure({ id: 'camp-1' });
      return render();
    })();
    other.submitting.focusFirstPendingChoice();
    expect(other.state().status).toBe('ready');
  });
});
