import { Injectable } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap, provideRouter, Router } from '@angular/router';
import { BehaviorSubject } from 'rxjs';
import type { SpellDetailsVm } from '../../shared/spell-details/spell-details.types';
import { CharacterEditor } from './character-editor';
import {
  AbilityRollsVm,
  AbilityTableVm,
  CharacterEditorSource,
  CharacterForEdit,
  CreateCharacterInput,
  RulesCatalogVm,
  UpdateCharacterInput,
} from './character-editor.types';

const STORED_ROLLS: AbilityRollsVm = {
  sets: [
    { dice: [6, 5, 5, 2], total: 16 },
    { dice: [5, 5, 4, 1], total: 14 },
    { dice: [5, 4, 4, 3], total: 13 },
    { dice: [4, 4, 4, 2], total: 12 },
    { dice: [4, 3, 3, 2], total: 10 },
    { dice: [3, 3, 2, 1], total: 8 },
  ],
  typed: false,
  rolledAt: new Date('2026-10-05T20:14:00'),
};

@Injectable()
class FakeCharacterEditorSource {
  loadCatalogFn: (campaignId: string) => Promise<RulesCatalogVm> = () => Promise.resolve(catalog());
  loadCharacterForEditFn: (campaignId: string, characterId: string) => Promise<CharacterForEdit> =
    () => Promise.reject(new Error('not stubbed'));
  createCharacterCalls: CreateCharacterInput[] = [];
  createCharacterFn: (input: CreateCharacterInput) => Promise<{ characterId: string }> = () =>
    Promise.resolve({ characterId: 'new-char' });
  updateCharacterCalls: UpdateCharacterInput[] = [];
  updateCharacterFn: (input: UpdateCharacterInput) => Promise<{ revision: number }> = () =>
    Promise.resolve({ revision: 2 });

  /** The table's ways of making scores: `null` is the master's free editor (and every spec written before the rules). */
  abilityTable: AbilityTableVm | null = null;
  loadAbilityTableCalls: string[] = [];
  loadAbilityTable(campaignId: string): Promise<AbilityTableVm | null> {
    this.loadAbilityTableCalls.push(campaignId);
    return Promise.resolve(this.abilityTable);
  }
  rollCalls: (readonly (readonly number[])[] | undefined)[] = [];
  rollFn: (typed?: readonly (readonly number[])[]) => Promise<AbilityRollsVm> = () =>
    Promise.resolve(STORED_ROLLS);
  rollAbilityScores(
    _campaignId: string,
    typed?: readonly (readonly number[])[],
  ): Promise<AbilityRollsVm> {
    this.rollCalls.push(typed);
    return this.rollFn(typed);
  }

  loadCatalog(campaignId: string): Promise<RulesCatalogVm> {
    return this.loadCatalogFn(campaignId);
  }
  loadSpellDetailsCalls: string[] = [];
  loadSpellDetailsFn: (spellKey: string) => Promise<SpellDetailsVm> = (key) =>
    Promise.resolve(knockDetails(key));
  loadSpellDetails(_campaignId: string, spellKey: string): Promise<SpellDetailsVm> {
    this.loadSpellDetailsCalls.push(spellKey);
    return this.loadSpellDetailsFn(spellKey);
  }
  loadCharacterForEdit(campaignId: string, characterId: string): Promise<CharacterForEdit> {
    return this.loadCharacterForEditFn(campaignId, characterId);
  }
  createCharacter(input: CreateCharacterInput): Promise<{ characterId: string }> {
    this.createCharacterCalls.push(input);
    return this.createCharacterFn(input);
  }
  updateCharacter(input: UpdateCharacterInput): Promise<{ revision: number }> {
    this.updateCharacterCalls.push(input);
    return this.updateCharacterFn(input);
  }
}

function knockDetails(key: string): SpellDetailsVm {
  return {
    key,
    namePt: 'Arrombar',
    nameEn: 'Knock',
    level: 2,
    schoolNamePt: 'Transmutação',
    ritual: false,
    concentration: false,
    castingTime: { amount: 1, unit: 'action', trigger: '', raw: '1 action' },
    range: { kind: 'ranged', distanceFt: 60, raw: '60 feet' },
    components: { verbal: true, somatic: false, material: false, materialText: '' },
    duration: {
      kind: 'instantaneous',
      amount: 0,
      unit: '',
      upTo: false,
      concentration: false,
      raw: 'Instantaneous',
    },
    description: ['Choose an object that you can see within range.'],
    higherLevel: [],
  };
}

const SRD = { fromTable: false, archived: false, off: false };

function catalog(): RulesCatalogVm {
  return {
    races: [
      {
        key: 'race:gnome',
        namePt: 'Gnomo',
        constitutionBonus: 0,
        choiceBonuses: [],
        fromTable: false,
        archived: false,
        off: false,
        subraces: [
          { key: 'subrace:rock-gnome', namePt: 'Gnomo da Rocha', constitutionBonus: 1, ...SRD },
        ],
      },
    ],
    classes: [
      {
        key: 'class:wizard',
        namePt: 'Mago',
        hitDie: 6,
        isCaster: true,
        ...SRD,
        skillChoose: 2,
        savingThrows: ['int', 'wis'],
        spellListClassKey: 'class:wizard',
        preparation: 'spellbook',
        subclasses: [
          {
            key: 'subclass:evocation',
            namePt: 'Evocação',
            casting: null,
            alwaysPrepared: [],
            ...SRD,
          },
        ],
        subclassLevel: 2,
        spellcastingFirstLevel: 1,
        // Levels 1-5: circles 1, 1, 2, 2, 3.
        maxSpellLevelByLevel: [1, 1, 2, 2, 3, 3, 4, 4, 5, 5, 6, 6, 7, 7, 8, 8, 9, 9, 9, 9],
      },
      {
        key: 'class:paladin',
        namePt: 'Paladino',
        hitDie: 10,
        isCaster: true,
        ...SRD,
        skillChoose: 2,
        savingThrows: ['wis', 'cha'],
        spellListClassKey: 'class:paladin',
        preparation: 'prepared',
        subclasses: [
          {
            key: 'subclass:devotion',
            namePt: 'Devoção',
            casting: null,
            alwaysPrepared: [],
            ...SRD,
          },
        ],
        subclassLevel: 3,
        spellcastingFirstLevel: 2,
        // No leveled spells at level 1.
        maxSpellLevelByLevel: [0, 1, 1, 1, 2, 2, 2, 2, 3, 3, 3, 3, 4, 4, 4, 4, 5, 5, 5, 5],
      },
    ],
    backgrounds: [{ key: 'background:acolyte', namePt: 'Acólito', equipmentPt: '', ...SRD }],
    skills: [
      { key: 'skill:arcana', namePt: 'Arcanismo', ability: 'int' },
      { key: 'skill:history', namePt: 'História', ability: 'int' },
    ],
    armor: [{ key: 'equipment:leather-armor', namePt: 'Armadura de Couro' }],
    weapons: [
      { key: 'equipment:quarterstaff', namePt: 'Bordão' },
      { key: 'equipment:dagger', namePt: 'Adaga' },
    ],
    spells: [
      {
        key: 'spell:fire-bolt',
        namePt: 'Raio de Fogo',
        level: 0,
        classKeys: ['class:wizard'],
        fromTable: false,
        archived: false,
        off: false,
      },
      {
        key: 'spell:ray-of-frost',
        namePt: 'Raio de Gelo',
        level: 0,
        classKeys: ['class:wizard'],
        fromTable: false,
        archived: false,
        off: false,
      },
      {
        key: 'spell:magic-missile',
        namePt: 'Mísseis Mágicos',
        level: 1,
        classKeys: ['class:wizard'],
        fromTable: false,
        archived: false,
        off: false,
      },
      {
        key: 'spell:shield',
        namePt: 'Escudo Arcano',
        level: 1,
        classKeys: ['class:wizard'],
        fromTable: false,
        archived: false,
        off: false,
      },
      {
        key: 'spell:fireball',
        namePt: 'Bola de Fogo',
        level: 3,
        classKeys: ['class:wizard'],
        fromTable: false,
        archived: false,
        off: false,
      },
      {
        key: 'spell:bless',
        namePt: 'Bênção',
        level: 1,
        classKeys: ['class:paladin'],
        fromTable: false,
        archived: false,
        off: false,
      },
      // Not on the Wizard's list — proves the picker filters by class.
      {
        key: 'spell:cure-wounds',
        namePt: 'Curar Ferimentos',
        level: 1,
        classKeys: ['class:cleric'],
        fromTable: false,
        archived: false,
        off: false,
      },
    ],
    viewerIsMaster: false,
    toolsAndLanguages: [
      { key: 'proficiency:thieves-tools', namePt: 'Ferramentas de ladrão', kind: 'tool' },
      { key: 'language:elvish', namePt: 'Élfico', kind: 'language' },
      { key: 'language:dwarvish', namePt: 'Anão', kind: 'language' },
    ],
    // The SRD's ND to XP table, in the server's order (first rows are enough).
    challengeRatings: [
      { rating: '0', xp: 10 },
      { rating: '1/8', xp: 25 },
      { rating: '1/4', xp: 50 },
      { rating: '1/2', xp: 100 },
      { rating: '1', xp: 200 },
      { rating: '2', xp: 450 },
    ],
  };
}

function flush(): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, 0));
}

function deferred<T>() {
  let resolve!: (v: T) => void;
  let reject!: (e: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function fullSheet(over: object = {}): CharacterForEdit {
  return {
    kind: 'player',
    revision: 7,
    blocked: null,
    sheetLocked: false,
    basic: null,
    full: {
      name: 'Pensantus',
      race: 'race:gnome',
      subrace: 'subrace:rock-gnome',
      className: 'class:wizard',
      subclassName: 'subclass:evocation',
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
      skillProficiencies: ['skill:arcana'],
      expertiseSkillKeys: [],
      abilities: { str: 8, dex: 14, con: 16, int: 18, wis: 12, cha: 10 },
      extraAbilityBonuses: { str: 0, dex: 0, con: 0, int: 0, wis: 0, cha: 0 },
      hitPointsMethod: 'average',
      hitPointsRolls: [],
      isCaster: true,
      cantrips: ['spell:fire-bolt'],
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
      size: 0,
      alignment: '',
      customFeaturesText: '',
      ...over,
    },
  } as CharacterForEdit;
}

const basicSheet = (): CharacterForEdit => ({
  kind: 'story',
  revision: 2,
  blocked: null,
  sheetLocked: false,
  full: null,
  basic: {
    name: 'Mira',
    hitPointsMax: 9,
    armorClass: 11,
    speedFt: 30,
    initiativeBonus: 2,
    attacks: [],
    legacyDamage: '',
    legacyAttackBonus: 0,
    description: '',
    challengeRating: '',
    xpValue: 0,
    portraitImageId: '',
    size: 0,
  },
});

describe('Review15 U15-4: character editor races (late answers, hit point rolls)', () => {
  let fake: FakeCharacterEditorSource;
  let params$: BehaviorSubject<ReturnType<typeof convertToParamMap>>;

  function configure(params: Record<string, string>): void {
    params$ = new BehaviorSubject(convertToParamMap(params));
    TestBed.configureTestingModule({
      imports: [CharacterEditor],
      providers: [
        provideRouter([]),
        { provide: CharacterEditorSource, useClass: FakeCharacterEditorSource },
        { provide: ActivatedRoute, useValue: { paramMap: params$ } },
      ],
    });
    fake = TestBed.inject(CharacterEditorSource) as unknown as FakeCharacterEditorSource;
  }

  async function render() {
    const fixture = TestBed.createComponent(CharacterEditor);
    fixture.detectChanges();
    await flush();
    await fixture.whenStable();
    fixture.detectChanges();
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    return { fixture, cmp: fixture.componentInstance as any };
  }

  beforeAll(async () => {
    configure({ id: 'camp-warm-up' });
    await render();
    TestBed.resetTestingModule();
  });

  function fillValid(cmp: any, over: object = {}): void {
    cmp.fullForm.patchValue({
      name: 'Pensantus',
      race: 'race:gnome',
      className: 'class:wizard',
      level: 3,
      background: 'background:acolyte',
      ...over,
    });
  }

  it('A: a create that resolves after the person left the editor does not navigate to the new sheet', async () => {
    configure({ id: 'camp-1' });
    const { fixture, cmp } = await render();
    const router = TestBed.inject(Router);
    const navigate = vi.spyOn(router, 'navigate').mockResolvedValue(true);
    const pending = deferred<{ characterId: string }>();
    fake.createCharacterFn = () => pending.promise;
    fillValid(cmp);

    const done = cmp.submit() as Promise<void>;
    await flush();
    expect(fake.createCharacterCalls.length).toBe(1);

    // The person pressed "Cancelar" / followed another link: the page is destroyed.
    fixture.destroy();
    pending.resolve({ characterId: 'new-char' });
    await done;

    expect(navigate).not.toHaveBeenCalled();
  });

  it('A (edit): an update that resolves after the editor was destroyed does not navigate', async () => {
    configure({ id: 'camp-1', characterId: 'char-1' });
    fake.loadCharacterForEditFn = () => Promise.resolve(fullSheet());
    const { fixture, cmp } = await render();
    const navigate = vi.spyOn(TestBed.inject(Router), 'navigate').mockResolvedValue(true);
    const pending = deferred<{ revision: number }>();
    fake.updateCharacterFn = () => pending.promise;

    const done = cmp.submit() as Promise<void>;
    await flush();
    expect(fake.updateCharacterCalls.length).toBe(1);
    fixture.destroy();
    pending.resolve({ revision: 8 });
    await done;

    expect(navigate).not.toHaveBeenCalled();
  });

  it('B: a late answer for character A does not overwrite character B after the route params changed', async () => {
    configure({ id: 'camp-1', characterId: 'A' });
    const slowA = deferred<CharacterForEdit>();
    fake.loadCharacterForEditFn = (_c, id) =>
      id === 'A'
        ? slowA.promise
        : Promise.resolve(
            fullSheet({ name: 'Bruxa B' }) && {
              ...fullSheet({ name: 'Bruxa B' }),
              revision: 20,
            },
          );
    const { fixture, cmp } = await render();
    // Same component instance, new params (/characters/A/edit -> /characters/B/edit).
    params$.next(convertToParamMap({ id: 'camp-1', characterId: 'B' }));
    await flush();
    await fixture.whenStable();
    expect(cmp.fullForm.getRawValue().name).toBe('Bruxa B');

    slowA.resolve({ ...fullSheet({ name: 'Ana A' }), revision: 5 });
    await flush();
    await fixture.whenStable();

    const s = cmp.state();
    expect(cmp.fullForm.getRawValue().name).toBe('Bruxa B');
    expect(s.characterId).toBe('B');
    expect(s.revision).toBe(20);
  });

  it('B2: opening a basic NPC after a full sheet leaves none of the full sheet\'s signals behind', async () => {
    configure({ id: 'camp-1', characterId: 'A' });
    fake.loadCharacterForEditFn = (_c, id) =>
      Promise.resolve(id === 'A' ? fullSheet({ hitPointsRolls: [3, 4] }) : basicSheet());
    const { fixture, cmp } = await render();
    expect(cmp.selectedSkills().size).toBe(1);

    params$.next(convertToParamMap({ id: 'camp-1', characterId: 'B' }));
    await flush();
    await fixture.whenStable();
    expect(cmp.state().kind).toBe('story');

    expect(cmp.selectedSkills().size).toBe(0);
    expect(cmp.selectedCantrips().size).toBe(0);
    expect(cmp.hitPointsRolls()).toEqual([]);
  });

  it('C: a roll the screen counts as missing (20 on a d6) is not sent to the server', async () => {
    configure({ id: 'camp-1' });
    const { cmp } = await render();
    fillValid(cmp, { level: 3, hitPointsMethod: 'rolled' });
    cmp.hitPointsRolls.set([4, 20]);

    await cmp.submit();

    const sent = fake.createCharacterCalls[0]?.full?.hitPointsRolls;
    // Correct: the save is stopped (and the level named), not sent with a roll known invalid.
    expect(sent).toBeUndefined();
    expect(String(cmp.saveState().message ?? '')).toContain('nível');
  });

  it('C2: a roll that fits no die the class has (11 on d6) is not sent either', async () => {
    configure({ id: 'camp-1' });
    const { cmp } = await render();
    fillValid(cmp, { level: 2, hitPointsMethod: 'rolled' });
    cmp.hitPointsRolls.set([11]);
    await cmp.submit();
    expect(fake.createCharacterCalls.length).toBe(0);
  });
});
