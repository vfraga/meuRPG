import { Injectable } from '@angular/core';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap, provideRouter } from '@angular/router';
import { of } from 'rxjs';

import { fakeContentWatcher } from '../../core/content/content-testing';
import { CharacterEditor } from './character-editor';
import {
  CharacterEditorSource,
  CharacterForEdit,
  ClassOptionVm,
  CreateCharacterInput,
  RulesCatalogVm,
  SpellOptionVm,
  SubclassOptionVm,
  UpdateCharacterInput,
} from './character-editor.types';

// The editor with the table's own content (MR-025, RN-23, slice 10.12b): a table class, race and
// background, the "Outro" background, a sheet of several classes (E10-02 states 5 and 6) and the
// spell step per class (E10-11 state 4). The numbers are the server's: these specs give the
// editor a catalog, as the server would, and read the request it builds.

const SRD = { fromTable: false, archived: false, off: false };
const TABLE = { fromTable: true, archived: false, off: false };

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
  fromTable: key.endsWith('@mesa'),
  archived: false,
  off: false,
});

const CIRCLES = [1, 1, 2, 2, 3, 3, 4, 4, 5, 5, 6, 6, 7, 7, 8, 8, 9, 9, 9, 9];

function catalog(): RulesCatalogVm {
  return {
    races: [
      {
        key: 'race:gnome',
        namePt: 'Gnomo',
        constitutionBonus: 0,
        choiceBonuses: [],
        subraces: [],
        ...SRD,
      },
      {
        key: 'race:corujeiro@mesa',
        namePt: 'Corujeiro',
        constitutionBonus: 0,
        choiceBonuses: [2, 1],
        subraces: [],
        ...TABLE,
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
        subclasses: [
          sub({ key: 'subclass:evocation', namePt: 'Escola de Evocação' }),
          sub({ key: 'subclass:ink@mesa', namePt: 'Tradição da Tinta', ...TABLE }),
        ],
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
          sub({ key: 'subclass:life', namePt: 'Domínio da Vida' }),
          sub({ key: 'subclass:path@mesa', namePt: 'Domínio do Caminho', ...TABLE }),
        ],
      }),
      cls({
        key: 'class:guardiao@mesa',
        namePt: 'Guardião do Vale',
        hitDie: 10,
        skillChoose: 2,
        savingThrows: ['str', 'wis'],
        ...TABLE,
      }),
      cls({
        key: 'class:fighter',
        namePt: 'Guerreiro',
        hitDie: 10,
        subclasses: [
          sub({ key: 'subclass:champion', namePt: 'Campeão' }),
          sub({
            key: 'subclass:ink-blade@mesa',
            namePt: 'Lâmina de Tinta',
            ...TABLE,
            casting: {
              preparation: 'known',
              listClassKey: 'class:wizard',
              firstLevel: 3,
              maxSpellLevelByLevel: [0, 0, 1, 1, 1, 1, 2, 2, 2, 2, 3, 3, 3, 3, 3, 3, 4, 4, 4, 4],
            },
          }),
        ],
      }),
      cls({ key: 'class:bard', namePt: 'Bardo' }),
      cls({ key: 'class:druid', namePt: 'Druida' }),
    ],
    backgrounds: [
      { key: 'background:acolyte', namePt: 'Acólito', equipmentPt: '', ...SRD },
      {
        key: 'background:cartografo@mesa',
        namePt: 'Cartógrafo do Vale',
        equipmentPt: 'Uma luneta e um rolo de corda',
        ...TABLE,
      },
    ],
    skills: [
      { key: 'skill:arcana', namePt: 'Arcanismo', ability: 'int' },
      { key: 'skill:survival', namePt: 'Sobrevivência', ability: 'wis' },
      { key: 'skill:perception', namePt: 'Percepção', ability: 'wis' },
    ],
    armor: [],
    weapons: [],
    spells: [
      spell('spell:fire-bolt', 'Raio de Fogo', 0, ['class:wizard']),
      spell('spell:shield', 'Escudo Arcano', 1, ['class:wizard']),
      spell('spell:detect-magic', 'Detectar Magia', 1, ['class:wizard', 'class:cleric']),
      spell('spell:bless', 'Bênção', 1, ['class:cleric']),
      spell('spell:ink-blade@mesa', 'Lâmina de Nanquim', 1, ['class:wizard']),
      spell('spell:animal-friendship', 'Amizade Animal', 1, ['class:bard', 'class:druid']),
    ],
    viewerIsMaster: false,
    toolsAndLanguages: [
      { key: 'proficiency:cartographers-tools', namePt: 'Ferramentas de cartógrafo', kind: 'tool' },
      { key: 'proficiency:thieves-tools', namePt: 'Ferramentas de ladrão', kind: 'tool' },
      { key: 'language:elvish', namePt: 'Élfico', kind: 'language' },
      { key: 'language:dwarvish', namePt: 'Anão', kind: 'language' },
    ],
    challengeRatings: [],
  };
}

@Injectable()
class FakeSource {
  catalogCalls: (string | undefined)[] = [];
  catalogOver: (c: RulesCatalogVm) => RulesCatalogVm = (c) => c;
  created: CreateCharacterInput[] = [];
  updated: UpdateCharacterInput[] = [];
  forEdit: CharacterForEdit | null = null;
  loadCatalog(_campaign: string, characterId?: string) {
    this.catalogCalls.push(characterId);
    return Promise.resolve(this.catalogOver(catalog()));
  }
  loadSpellDetails() {
    return Promise.reject(new Error('unused'));
  }
  loadCharacterForEdit() {
    return this.forEdit ? Promise.resolve(this.forEdit) : Promise.reject(new Error('not stubbed'));
  }
  loadAbilityTable() {
    return Promise.resolve(null);
  }
  rollAbilityScores() {
    return Promise.reject(new Error('unused'));
  }
  createCharacter(input: CreateCharacterInput) {
    this.created.push(input);
    return Promise.resolve({ characterId: 'new' });
  }
  updateCharacter(input: UpdateCharacterInput) {
    this.updated.push(input);
    return Promise.resolve({ revision: 2 });
  }
}

const flush = () => new Promise<void>((resolve) => setTimeout(resolve, 0));

async function render(
  params: Record<string, string> = { id: 'camp-1' },
  setup: (fake: FakeSource) => void = () => undefined,
) {
  TestBed.configureTestingModule({
    imports: [CharacterEditor],
    providers: [
      provideRouter([]),
      { provide: CharacterEditorSource, useClass: FakeSource },
      { provide: ActivatedRoute, useValue: { paramMap: of(convertToParamMap(params)) } },
    ],
  });
  const fake = TestBed.inject(CharacterEditorSource) as unknown as FakeSource;
  setup(fake);
  const fixture = TestBed.createComponent(CharacterEditor);
  fixture.detectChanges();
  await flush();
  await fixture.whenStable();
  fixture.detectChanges();
  return {
    fixture,
    fake,
    el: fixture.nativeElement as HTMLElement,
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    cmp: fixture.componentInstance as any,
  };
}

async function settle(fixture: ComponentFixture<CharacterEditor>) {
  fixture.detectChanges();
  await fixture.whenStable();
  fixture.detectChanges();
}

async function openStep(fixture: ComponentFixture<CharacterEditor>, label: string) {
  const tab = Array.from(
    (fixture.nativeElement as HTMLElement).querySelectorAll<HTMLElement>('[role="tab"]'),
  ).find((t) => t.textContent?.includes(label));
  tab!.click();
  await settle(fixture);
}

const button = (el: HTMLElement, text: string) =>
  Array.from(el.querySelectorAll<HTMLButtonElement>('button')).find((b) =>
    b.textContent?.includes(text),
  );

// A warm-up render outside any test, so the cold first render never lands on whichever test runs first.
beforeAll(async () => {
  await render();
  TestBed.resetTestingModule();
});

describe('a table class, race and background in the editor (E10-02 state 5)', () => {
  it('sends the keys of the table content, never their names', async () => {
    const { fake, cmp } = await render();
    cmp.fullForm.patchValue({
      name: 'Ícaro',
      race: 'race:corujeiro@mesa',
      className: 'class:guardiao@mesa',
      level: 1,
      background: 'background:cartografo@mesa',
    });
    await cmp.submit();
    const full = fake.created[0].full!;
    expect([full.race, full.className, full.background]).toEqual([
      'race:corujeiro@mesa',
      'class:guardiao@mesa',
      'background:cartografo@mesa',
    ]);
    expect(full.extraClasses).toEqual([]);
  });

  it("tells the class's skill count and saving throws from the server's entry, and where a race's free bonuses go", async () => {
    const { fixture, el, cmp } = await render();
    cmp.fullForm.patchValue({ race: 'race:corujeiro@mesa', className: 'class:guardiao@mesa' });
    await settle(fixture);
    const text = (el.textContent ?? '').replace(/\s+/g, ' ');
    expect(text).toContain('Dá +2 e +1 nas habilidades que você escolher');
    await openStep(fixture, 'Perícias');
    expect((el.textContent ?? '').replace(/\s+/g, ' ')).toContain(
      'O Guardião do Vale escolhe 2 perícias ao começar e dá proficiência nos testes de resistência de Força e Sabedoria.',
    );
  });

  it("shows a table background's equipment as the master wrote it", async () => {
    const { fixture, el, cmp } = await render();
    cmp.fullForm.patchValue({ background: 'background:cartografo@mesa' });
    await settle(fixture);
    expect(el.textContent ?? '').toContain('Equipamento: Uma luneta e um rolo de corda');
  });

  it('marks the entries of the table with "Da mesa" in the lists, and nothing else', async () => {
    const { fixture, el } = await render();
    const trigger = (label: string) =>
      Array.from(el.querySelectorAll<HTMLElement>('mat-form-field'))
        .find((f) => f.querySelector('mat-label')?.textContent?.includes(label))!
        .querySelector<HTMLElement>('.mat-mdc-select-trigger')!;
    trigger('Classe').click();
    await settle(fixture);
    const options = Array.from(document.querySelectorAll('mat-option')).map((o) =>
      (o.textContent ?? '')
        .replace(/menu_book|inventory_2/g, '')
        .replace(/\s+/g, ' ')
        .trim(),
    );
    expect(options).toContain('Guardião do Vale Da mesa');
    expect(options).toContain('Mago');
    expect(options.filter((o) => o.includes('Da mesa'))).toEqual(['Guardião do Vale Da mesa']);
  });
});

describe('the "Outro" background (E10-02 state 5, question 82)', () => {
  it('asks for the name, two skills, two tools or languages, the feature and the equipment, and sends them all', async () => {
    const { fixture, fake, el, cmp } = await render();
    cmp.fullForm.patchValue({
      name: 'Davi',
      race: 'race:gnome',
      className: 'class:fighter',
      background: 'custom',
      customBackgroundName: 'Batedor de torre',
      customBackgroundFeatureName: 'Olho no horizonte',
      customBackgroundFeatureText: 'Você sempre acha o ponto mais alto de um lugar.',
      customBackgroundEquipment: 'Uma luneta, um rolo de corda e 10 PO',
    });
    cmp.toggleCustomBackgroundSkill('skill:perception');
    cmp.toggleCustomBackgroundSkill('skill:survival');
    cmp.setCustomProficiencies([
      'proficiency:cartographers-tools',
      'language:elvish',
      'language:dwarvish',
    ]);
    await settle(fixture);
    const text = (el.textContent ?? '').replace(/\s+/g, ' ');
    expect(text).toContain('Personalizar um antecedente');
    expect(text).toContain('2 de 2 escolhidas');
    await cmp.submit();
    expect(fake.created[0].full).toMatchObject({
      background: 'custom',
      customBackgroundName: 'Batedor de torre',
      customBackgroundSkills: ['skill:perception', 'skill:survival'],
      // A third is never taken: two, like the skills.
      customBackgroundProficiencies: ['proficiency:cartographers-tools', 'language:elvish'],
      customBackgroundFeatureName: 'Olho no horizonte',
      customBackgroundFeatureText: 'Você sempre acha o ponto mais alto de um lugar.',
      customBackgroundEquipment: 'Uma luneta, um rolo de corda e 10 PO',
    });
  });

  it('sends fewer than two as they are: the server shows an issue on the sheet, never an error', async () => {
    const { fake, cmp } = await render();
    cmp.fullForm.patchValue({
      name: 'Davi',
      race: 'race:gnome',
      className: 'class:fighter',
      background: 'custom',
      customBackgroundName: 'Batedor',
    });
    cmp.setCustomProficiencies(['language:elvish']);
    await cmp.submit();
    expect(fake.created[0].full).toMatchObject({
      customBackgroundProficiencies: ['language:elvish'],
      customBackgroundSkills: null,
    });
  });
});

describe('several classes at creation (E10-02 state 6)', () => {
  async function maga() {
    const r = await render();
    r.cmp.fullForm.patchValue({
      name: 'Corvina',
      race: 'race:gnome',
      className: 'class:wizard',
      level: 3,
      subclassName: 'subclass:ink@mesa',
      background: 'background:acolyte',
    });
    await settle(r.fixture);
    return r;
  }

  it("adds a block per class, with the total level and the table's subclass in each", async () => {
    const { fixture, fake, el, cmp } = await maga();
    expect(el.querySelectorAll('app-class-block').length).toBe(0);
    button(el, 'Adicionar classe')!.click();
    await settle(fixture);
    const blocks = el.querySelectorAll('app-class-block');
    expect(blocks.length).toBe(2);
    expect(blocks[0].textContent).toContain('Classe 1');
    expect(blocks[1].textContent).toContain('Classe 2');
    // The total is read, never typed: the first class's level and the second's.
    cmp.changeBlock(1, { classKey: 'class:cleric', level: 1, subclassKey: 'subclass:path@mesa' });
    await settle(fixture);
    expect(cmp.totalLevelValue()).toBe(4);
    expect(el.querySelector('.xp-read__value')?.textContent?.trim()).toBe('4');
    await cmp.submit();
    expect(fake.created[0].full).toMatchObject({
      className: 'class:wizard',
      level: 3,
      subclassName: 'subclass:ink@mesa',
      extraClasses: [
        {
          classKey: 'class:cleric',
          level: 1,
          subclassKey: 'subclass:path@mesa',
          customSubclassName: '',
        },
      ],
    });
  });

  it('keeps the subclass shut until the class level that chooses it', async () => {
    const { fixture, el, cmp } = await render();
    cmp.fullForm.patchValue({ className: 'class:wizard', level: 1 });
    cmp.addClass();
    cmp.changeBlock(1, { classKey: 'class:fighter', level: 2 });
    await settle(fixture);
    const second = el.querySelectorAll('app-class-block')[1];
    expect(second.textContent).toContain('O Guerreiro escolhe a subclasse no nível 3.');
    expect(
      second.querySelector('mat-select[aria-disabled="true"], .mat-mdc-select-disabled'),
    ).not.toBeNull();
    cmp.changeBlock(1, { level: 3 });
    await settle(fixture);
    expect(el.querySelectorAll('app-class-block')[1].textContent).not.toContain(
      'O Guerreiro escolhe a subclasse no nível 3.',
    );
  });

  it('removes a class and refuses a blank block or a total above 20 before saving', async () => {
    const { fixture, fake, el, cmp } = await maga();
    cmp.addClass();
    await settle(fixture);
    await cmp.submit();
    expect(fake.created).toHaveLength(0);
    expect(cmp.invalidSummary()).toContain('Classe 2');
    cmp.changeBlock(1, { classKey: 'class:cleric', level: 19 });
    await settle(fixture);
    expect(cmp.totalLevelProblem()).toBe('O nível total vai até 20.');
    await cmp.submit();
    expect(fake.created).toHaveLength(0);
    expect(cmp.invalidSummary()).toContain('o nível total vai até 20');
    // "Remover" takes the block out, and the sheet is a one-class sheet again.
    button(el, 'Remover')!.click();
    await settle(fixture);
    expect(el.querySelectorAll('app-class-block').length).toBe(0);
    await cmp.submit();
    expect(fake.created).toHaveLength(1);
    expect(fake.created[0].full?.extraClasses).toEqual([]);
  });

  it('rolls the hit points level by level with the die of the class that gives it', async () => {
    const { fixture, cmp } = await maga();
    cmp.changeBlock(0, { level: 2 });
    cmp.addClass();
    cmp.changeBlock(1, { classKey: 'class:cleric', level: 1 });
    cmp.fullForm.patchValue({ hitPointsMethod: 'rolled' });
    await settle(fixture);
    // Mago 2 (d6), Clérigo 1 (d8): the 2nd level of the wizard, then the cleric's first.
    expect(cmp.levelDice()).toEqual([6, 8]);
    expect(cmp.rollsNeeded()).toBe(2);
  });

  it('writes, rolls and checks each level of the hit points with the die of its own class', async () => {
    const { fixture, el, cmp } = await maga();
    cmp.changeBlock(0, { level: 1 });
    cmp.addClass();
    cmp.changeBlock(1, { classKey: 'class:cleric', level: 2 });
    cmp.fullForm.patchValue({ hitPointsMethod: 'rolled' });
    await settle(fixture);
    // Mago 1 (d6), Clérigo 2 (d8): levels 2 and 3 are the cleric's.
    await openStep(fixture, 'Habilidades');
    const labels = Array.from(el.querySelectorAll('app-hit-points-rolls mat-label')).map((l) =>
      l.textContent?.trim(),
    );
    expect(labels).toEqual(['Nível 2 (1d8)', 'Nível 3 (1d8)']);
  });
});

describe('the spell step per class (E10-11 state 4)', () => {
  async function corvina() {
    const r = await render();
    r.cmp.fullForm.patchValue({
      name: 'Corvina',
      race: 'race:gnome',
      className: 'class:wizard',
      level: 3,
      background: 'background:acolyte',
    });
    r.cmp.addClass();
    r.cmp.changeBlock(1, { classKey: 'class:cleric', level: 1 });
    await settle(r.fixture);
    await openStep(r.fixture, 'Magias');
    return r;
  }

  it("gives each class its own section: the wizard's list up to the 2nd circle, the cleric's up to the 1st", async () => {
    const { el } = await corvina();
    const headings = Array.from(el.querySelectorAll('.spell-section__title')).map((h) =>
      h.textContent?.trim(),
    );
    expect(headings).toEqual([
      'Mago · até o 2º nível de magia',
      'Clérigo · até o 1º nível de magia',
    ]);
    const sections = Array.from(el.querySelectorAll('.spell-section'));
    const names = (s: Element) =>
      Array.from(s.querySelectorAll('mat-checkbox')).map((c) =>
        c.textContent
          ?.replace(/menu_book|inventory_2|visibility_off|Da mesa/g, '')
          .replace(/\s+/g, ' ')
          .trim(),
      );
    expect(names(sections[0])).toEqual([
      'Raio de Fogo',
      'Detectar Magia (1º nível)',
      'Escudo Arcano (1º nível)',
      'Lâmina de Nanquim (1º nível)',
      'Detectar Magia (1º nível)',
      'Escudo Arcano (1º nível)',
      'Lâmina de Nanquim (1º nível)',
    ]);
    expect(names(sections[1])).toEqual(['Bênção (1º nível)', 'Detectar Magia (1º nível)']);
  });

  it('greys out a spell that no class of the sheet lists, with the reason and a link to "Magias"', async () => {
    const { fixture, el, cmp } = await corvina();
    cmp.setSpellFilter(1, 'prepared', 'amizade');
    await settle(fixture);
    const clericSection = el.querySelectorAll('.spell-section')[1];
    const out = clericSection.querySelector('.picker__out')!;
    expect(out.textContent).toContain('Amizade Animal');
    expect(out.textContent).toContain('Fora da lista das suas classes');
    expect(out.textContent).toContain(
      'Esta magia é de Bardo e Druida. Você a lê em “Magias”, mas não a escolhe nesta ficha.',
    );
    expect(out.querySelector('mat-checkbox')).toBeNull();
    expect(out.querySelector('a')?.getAttribute('href')).toBe('/campaigns/camp-1/spells');
  });

  it("sends only picks that are on a list of the sheet, from the table's spells too", async () => {
    const { fake, cmp } = await corvina();
    cmp.toggleSpellKnown('spell:ink-blade@mesa');
    cmp.toggleSpellPrepared('spell:bless');
    cmp.toggleSpellPrepared('spell:animal-friendship');
    await cmp.submit();
    expect(fake.created[0].full?.spellsKnown).toEqual(['spell:ink-blade@mesa']);
    expect(fake.created[0].full?.spellsPrepared).toEqual(['spell:bless']);
  });

  it("offers a third caster's subclass the list it casts from, from its level on", async () => {
    const { fixture, el, cmp } = await render();
    cmp.fullForm.patchValue({
      name: 'Rúnico',
      race: 'race:gnome',
      className: 'class:fighter',
      level: 2,
      subclassName: 'subclass:ink-blade@mesa',
      background: 'background:acolyte',
    });
    await settle(fixture);
    // Level 2: the subclass does not cast yet, so there is no Magias step.
    expect(
      Array.from(el.querySelectorAll('[role="tab"]')).some((t) =>
        t.textContent?.includes('Magias'),
      ),
    ).toBe(false);
    cmp.fullForm.patchValue({ level: 3 });
    await settle(fixture);
    await openStep(fixture, 'Magias');
    const names = Array.from(el.querySelectorAll('.spell-section mat-checkbox')).map((c) =>
      c.textContent
        ?.replace(/menu_book|inventory_2|visibility_off|Da mesa/g, '')
        .replace(/\s+/g, ' ')
        .trim(),
    );
    expect(names).toContain('Escudo Arcano (1º nível)');
    expect(names).toContain('Lâmina de Nanquim (1º nível)');
    expect(names).not.toContain('Bênção (1º nível)');
  });
});

describe('an edit of a sheet of several classes', () => {
  it('opens every class in its block and saves them all in the order they were', async () => {
    TestBed.configureTestingModule({
      imports: [CharacterEditor],
      providers: [
        provideRouter([]),
        { provide: CharacterEditorSource, useClass: FakeSource },
        {
          provide: ActivatedRoute,
          useValue: { paramMap: of(convertToParamMap({ id: 'camp-1', characterId: 'ch-1' })) },
        },
      ],
    });
    const fake = TestBed.inject(CharacterEditorSource) as unknown as FakeSource;
    fake.forEdit = {
      kind: 'player',
      revision: 3,
      blocked: null,
      sheetLocked: false,
      basic: null,
      grantedSpellKeys: ['spell:bless'],
      full: {
        name: 'Corvina',
        race: 'race:gnome',
        subrace: '',
        className: 'class:wizard',
        subclassName: 'subclass:ink@mesa',
        customSubclassName: '',
        level: 3,
        background: 'background:acolyte',
        customBackgroundName: '',
        customBackgroundSkills: null,
        customBackgroundProficiencies: [],
        customBackgroundFeatureName: '',
        customBackgroundFeatureText: '',
        customBackgroundEquipment: '',
        extraClasses: [
          {
            classKey: 'class:cleric',
            level: 1,
            subclassKey: 'subclass:path@mesa',
            customSubclassName: '',
          },
        ],
        skillProficiencies: [],
        expertiseSkillKeys: [],
        abilities: { str: 10, dex: 10, con: 10, int: 10, wis: 10, cha: 10 },
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
        experiencePoints: 2700,
        challengeRating: '',
        xpValue: 0,
        portraitImageId: '',
        alignment: '',
        customFeaturesText: '',
      },
    };
    const fixture = TestBed.createComponent(CharacterEditor);
    fixture.detectChanges();
    await flush();
    await settle(fixture);
    const el = fixture.nativeElement as HTMLElement;
    expect(el.querySelectorAll('app-class-block').length).toBe(2);
    expect(el.querySelector('.xp-read__value')?.textContent?.trim()).toBe('4');

    // The spells the subclass always prepares are read, with the word and the lock, never a checkbox.
    await openStep(fixture, 'Magias');
    const granted = el.querySelector('.granted')!;
    expect(granted.textContent).toContain('Bênção');
    expect(granted.textContent).toContain('Sempre preparada');
    expect(granted.querySelector('mat-checkbox')).toBeNull();

    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    await (fixture.componentInstance as any).submit();
    expect(fake.updated[0].full?.extraClasses).toEqual([
      {
        classKey: 'class:cleric',
        level: 1,
        subclassKey: 'subclass:path@mesa',
        customSubclassName: '',
      },
    ]);
  });
});

describe('what the master retired or switched off (RN-23, 10.1d)', () => {
  const withRetired = (c: RulesCatalogVm): RulesCatalogVm => ({
    ...c,
    races: [
      ...c.races,
      {
        key: 'race:velha@mesa',
        namePt: 'Velha raça',
        constitutionBonus: 0,
        choiceBonuses: [],
        subraces: [],
        fromTable: true,
        archived: true,
        off: false,
      },
    ],
    classes: [
      ...c.classes,
      cls({ key: 'class:arquivada@mesa', namePt: 'Classe arquivada', ...TABLE, archived: true }),
      cls({ key: 'class:desligada@mesa', namePt: 'Classe desligada', ...TABLE, off: true }),
    ],
  });
  const optionsOf = () =>
    Array.from(document.querySelectorAll('mat-option')).map((o) =>
      (o.textContent ?? '')
        .replace(/menu_book|inventory_2|visibility_off/g, '')
        .replace(/\s+/g, ' ')
        .trim(),
    );
  const openSelect = async (
    fixture: ComponentFixture<CharacterEditor>,
    el: HTMLElement,
    label: string,
  ) => {
    Array.from(el.querySelectorAll<HTMLElement>('mat-form-field'))
      .find((f) => f.querySelector('mat-label')?.textContent?.includes(label))!
      .querySelector<HTMLElement>('.mat-mdc-select-trigger')!
      .click();
    await settle(fixture);
  };

  it('never offers an archived entry as a new choice, not even to the master', async () => {
    const { fixture, el } = await render(
      { id: 'camp-1' },
      (f) => (f.catalogOver = (c) => ({ ...withRetired(c), viewerIsMaster: true })),
    );
    await openSelect(fixture, el, 'Classe');
    expect(optionsOf()).not.toContain('Classe arquivada Arquivada');
    expect(optionsOf().some((o) => o.includes('Classe arquivada'))).toBe(false);
    document.body.click();
  });

  it('offers a switched-off one to the master, tagged, and to nobody else', async () => {
    const master = await render(
      { id: 'camp-1' },
      (f) => (f.catalogOver = (c) => ({ ...withRetired(c), viewerIsMaster: true })),
    );
    await openSelect(master.fixture, master.el, 'Classe');
    expect(
      optionsOf().some(
        (o) =>
          o.includes('Classe desligada') &&
          o.includes('Desligada para os jogadores') &&
          o.includes('Da mesa'),
      ),
    ).toBe(true);
    TestBed.resetTestingModule();
    const player = await render(
      { id: 'camp-1' },
      (f) => (f.catalogOver = (c) => ({ ...withRetired(c), viewerIsMaster: false })),
    );
    await openSelect(player.fixture, player.el, 'Classe');
    expect(optionsOf().some((o) => o.includes('Classe desligada'))).toBe(false);
  });

  it('keeps what the form already has, with its tag, so the field is never blank', async () => {
    const { fixture, el, cmp } = await render(
      { id: 'camp-1' },
      (f) => (f.catalogOver = withRetired),
    );
    cmp.fullForm.patchValue({ className: 'class:arquivada@mesa', race: 'race:velha@mesa' });
    await settle(fixture);
    const text = (el.textContent ?? '').replace(/\s+/g, ' ');
    expect(text).toContain('Classe arquivada');
    expect(text).toContain('Velha raça');
    expect(text).toContain('Arquivada');
  });

  it('says a value the catalog does not list at all instead of staying blank', async () => {
    const { fixture, el, cmp } = await render();
    cmp.fullForm.patchValue({ className: 'class:ghost@mesa' });
    await settle(fixture);
    await openSelect(fixture, el, 'Classe');
    expect(optionsOf()).toContain('Classe que saiu da lista');
    expect(el.textContent).not.toContain('ghost');
  });

  it('asks for the catalog with the sheet when editing, and without when creating', async () => {
    const created = await render();
    expect(created.fake.catalogCalls).toEqual([undefined]);
    TestBed.resetTestingModule();
    const edited = await render({ id: 'camp-1', characterId: 'ch-9' }, (f) => {
      f.forEdit = emptyEdit();
    });
    expect(edited.fake.catalogCalls).toEqual(['ch-9']);
  });
});

function emptyEdit(over: Partial<CharacterForEdit['full'] & object> = {}): CharacterForEdit {
  return {
    kind: 'player',
    revision: 1,
    blocked: null,
    sheetLocked: false,
    basic: null,
    full: {
      name: 'Corvina',
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
      abilities: { str: 10, dex: 10, con: 10, int: 10, wis: 10, cha: 10 },
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
      ...over,
    },
  };
}

describe('the class blocks (10.12b fix round 1)', () => {
  it('does not offer a class another block already has, and says "Escolha a classe." for a blank one', async () => {
    const { fixture, el, cmp } = await render();
    cmp.fullForm.patchValue({ className: 'class:wizard', level: 3 });
    cmp.addClass();
    await settle(fixture);
    const second = el.querySelectorAll('app-class-block')[1];
    second.querySelector<HTMLElement>('.mat-mdc-select-trigger')!.click();
    await settle(fixture);
    const options = Array.from(document.querySelectorAll('mat-option')).map((o) =>
      (o.textContent ?? '').replace(/\s+/g, ' ').trim(),
    );
    expect(options).not.toContain('Mago');
    expect(options).toContain('Clérigo');
    document.body.click();
    // A save with the blank block touches it: the field's own error says so.
    await cmp.submit();
    await settle(fixture);
    expect(second.querySelector('mat-error')?.textContent).toContain('Escolha a classe.');
  });

  it('marks the block a refusal points at', async () => {
    const { fixture, el, cmp } = await render();
    cmp.fullForm.patchValue({
      name: 'X',
      race: 'race:gnome',
      className: 'class:wizard',
      level: 2,
      background: 'background:acolyte',
    });
    cmp.addClass();
    cmp.changeBlock(1, { classKey: 'class:cleric', level: 1 });
    cmp.serverClassProblem.set(1);
    await settle(fixture);
    await settle(fixture);
    expect(
      el.querySelectorAll('app-class-block')[1].querySelector('mat-error')?.textContent,
    ).toContain('Essa classe se repete ou não existe.');
    expect(el.querySelectorAll('app-class-block')[0].querySelector('mat-error')).toBeNull();
  });
});

describe('the always-prepared spells in a class section (E10-11 state 4)', () => {
  const withDomain = (c: RulesCatalogVm): RulesCatalogVm => ({
    ...c,
    classes: c.classes.map((k) =>
      k.key === 'class:cleric'
        ? {
            ...k,
            subclasses: k.subclasses.map((s) =>
              s.key === 'subclass:path@mesa'
                ? {
                    ...s,
                    alwaysPrepared: [
                      { spellKey: 'spell:detect-magic', classLevel: 1 },
                      { spellKey: 'spell:bless', classLevel: 5 },
                    ],
                  }
                : s,
            ),
          }
        : k,
    ),
  });

  it('shows them locked inside the class that has the subclass, from the catalog, at creation, from the class level they start at', async () => {
    const { fixture, el, cmp } = await render(
      { id: 'camp-1' },
      (f) => (f.catalogOver = withDomain),
    );
    cmp.fullForm.patchValue({ className: 'class:wizard', level: 3 });
    cmp.addClass();
    cmp.changeBlock(1, { classKey: 'class:cleric', level: 1, subclassKey: 'subclass:path@mesa' });
    await settle(fixture);
    await openStep(fixture, 'Magias');
    const cleric = el.querySelectorAll('.spell-section')[1];
    const locked = Array.from(cleric.querySelectorAll('.granted__row')).map((r) =>
      (r.textContent ?? '')
        .replace(/lock|help_outline/g, '')
        .replace(/\s+/g, ' ')
        .trim(),
    );
    // Detectar Magia from level 1; Bênção waits for class level 5.
    expect(locked).toEqual(['Detectar Magia (1º nível)Domínio do CaminhoSempre preparada']);
    expect(cleric.querySelector('.granted mat-checkbox')).toBeNull();
    // Not said twice: the Mago's section has none, and no block of "Já na ficha" at creation.
    expect(el.querySelectorAll('.spell-section')[0].querySelector('.granted')).toBeNull();
    expect(el.querySelector('app-granted-spells[title="Já na ficha"]')).toBeNull();
  });

  it('never offers an always-prepared spell as a pick: not in the lists of its class, at creation or on an edit, in one class or several', async () => {
    const names = (el: Element) =>
      Array.from(el.querySelectorAll('mat-checkbox')).map((c) =>
        (c.textContent ?? '').replace(/\s+/g, ' ').trim(),
      );
    // Several classes, at creation: the cleric's section lists Bênção but not Detectar Magia (always prepared); the wizard's list still has it.
    const multi = await render({ id: 'camp-1' }, (f) => (f.catalogOver = withDomain));
    multi.cmp.fullForm.patchValue({ className: 'class:wizard', level: 3 });
    multi.cmp.addClass();
    multi.cmp.changeBlock(1, {
      classKey: 'class:cleric',
      level: 1,
      subclassKey: 'subclass:path@mesa',
    });
    await settle(multi.fixture);
    await openStep(multi.fixture, 'Magias');
    const cleric = multi.el.querySelectorAll('.spell-section')[1];
    expect(names(cleric).some((n) => n.startsWith('Bênção'))).toBe(true);
    expect(names(cleric).some((n) => n.startsWith('Detectar Magia'))).toBe(false);
    expect(
      names(multi.el.querySelectorAll('.spell-section')[0]).some((n) =>
        n.startsWith('Detectar Magia'),
      ),
    ).toBe(true);
    TestBed.resetTestingModule();
    // One class, on an edit, even when the sheet already carries it in its prepared list: still not a pick, and never counted.
    const edit = await render({ id: 'camp-1', characterId: 'ch-1' }, (f) => {
      f.catalogOver = withDomain;
      f.forEdit = {
        ...emptyEdit({
          className: 'class:cleric',
          level: 1,
          subclassName: 'subclass:path@mesa',
          spellsPrepared: ['spell:bless', 'spell:detect-magic'],
        }),
        preparedMax: { 'class:cleric': 5 },
      };
    });
    await openStep(edit.fixture, 'Magias');
    const lists = Array.from(edit.el.querySelectorAll('app-spell-picker')).flatMap((p) => names(p));
    expect(lists.some((n) => n.startsWith('Detectar Magia'))).toBe(false);
    expect(edit.el.textContent).toContain('Preparadas 1 de 5');
  });

  it('on an edit says how many the class prepares, from the saved sheet, never counting the locked ones', async () => {
    const { fixture, el } = await render({ id: 'camp-1', characterId: 'ch-1' }, (f) => {
      f.catalogOver = withDomain;
      f.forEdit = {
        ...emptyEdit({
          className: 'class:cleric',
          level: 1,
          subclassName: 'subclass:path@mesa',
          spellsPrepared: ['spell:bless', 'spell:detect-magic'],
        }),
        preparedMax: { 'class:cleric': 3 },
      };
    });
    await openStep(fixture, 'Magias');
    // Bênção is a pick; Detectar Magia is always prepared and free.
    expect(el.textContent).toContain('Preparadas 1 de 3');
  });

  it('says where the spells the sheet got from elsewhere come from, apart', async () => {
    const { fixture, el } = await render({ id: 'camp-1', characterId: 'ch-1' }, (f) => {
      f.forEdit = { ...emptyEdit(), grantedSpellKeys: ['spell:detect-magic'] };
    });
    await openStep(fixture, 'Magias');
    const text = (el.textContent ?? '').replace(/\s+/g, ' ');
    expect(text).toContain('Já na ficha');
    expect(text).toContain('Vêm da raça, da classe ou de uma característica');
    expect(text).not.toContain('Vêm da subclasse');
  });

  // "Já na ficha" on an edit
  const edit = (f: FakeSource) => {
    f.catalogOver = withDomain;
    f.forEdit = {
      ...emptyEdit({
        className: 'class:cleric',
        level: 1,
        subclassName: 'subclass:path@mesa',
        spellsPrepared: ['spell:bless', 'spell:detect-magic'],
      }),
      preparedMax: { 'class:cleric': 5 },
      grantedSpellKeys: ['spell:detect-magic'],
    };
  };

  it('does not list a spell of the subclass the sheet had once the subclass is changed: it is not from the race, the class or a feature', async () => {
    const { fixture, el, cmp } = await render({ id: 'camp-1', characterId: 'ch-1' }, edit);
    await openStep(fixture, 'Magias');
    expect(el.querySelector('app-granted-spells[title="Já na ficha"]')).toBeNull();
    cmp.fullForm.patchValue({ subclassName: 'subclass:life' });
    await settle(fixture);
    expect(el.querySelector('app-granted-spells[title="Já na ficha"]')).toBeNull();
  });

  it('lists a spell that no subclass of the sheet explains, whatever the subclass is (positive control)', async () => {
    const { fixture, el, cmp } = await render({ id: 'camp-1', characterId: 'ch-1' }, (f) => {
      edit(f);
      f.forEdit = { ...f.forEdit!, grantedSpellKeys: ['spell:detect-magic', 'spell:bless'] };
    });
    await openStep(fixture, 'Magias');
    expect(el.textContent).toContain('Já na ficha');
    cmp.fullForm.patchValue({ subclassName: 'subclass:life' });
    await settle(fixture);
    expect(el.textContent).toContain('Já na ficha');
  });
});

describe('the catalog read again (10.1d)', () => {
  it('reads it again, with the sheet, on a content_changed hint while a session is open, keeping the form', async () => {
    const watcher = fakeContentWatcher();
    TestBed.overrideComponent(CharacterEditor, { set: { providers: [watcher.provider] } });
    TestBed.configureTestingModule({
      imports: [CharacterEditor],
      providers: [
        provideRouter([]),
        { provide: CharacterEditorSource, useClass: FakeSource },
        {
          provide: ActivatedRoute,
          useValue: { paramMap: of(convertToParamMap({ id: 'camp-1', characterId: 'ch-9' })) },
        },
      ],
    });
    const fake = TestBed.inject(CharacterEditorSource) as unknown as FakeSource;
    fake.forEdit = emptyEdit();
    const fixture = TestBed.createComponent(CharacterEditor);
    fixture.detectChanges();
    await flush();
    await settle(fixture);
    expect(watcher.following()).toBe('camp-1');
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const cmp = fixture.componentInstance as any;
    cmp.fullForm.patchValue({ name: 'Mudei o nome' });
    fake.catalogCalls.length = 0;
    watcher.hint();
    await flush();
    await settle(fixture);
    expect(fake.catalogCalls).toEqual(['ch-9']);
    expect(cmp.fullForm.value.name).toBe('Mudei o nome');
  });

  it('keeps the newest reading when two reads of the lists answer out of order', async () => {
    const watcher = fakeContentWatcher();
    TestBed.overrideComponent(CharacterEditor, { set: { providers: [watcher.provider] } });
    TestBed.configureTestingModule({
      imports: [CharacterEditor],
      providers: [
        provideRouter([]),
        { provide: CharacterEditorSource, useClass: FakeSource },
        {
          provide: ActivatedRoute,
          useValue: { paramMap: of(convertToParamMap({ id: 'camp-1', characterId: 'ch-9' })) },
        },
      ],
    });
    const fake = TestBed.inject(CharacterEditorSource) as unknown as FakeSource;
    fake.forEdit = emptyEdit();
    const fixture = TestBed.createComponent(CharacterEditor);
    fixture.detectChanges();
    await flush();
    await settle(fixture);
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const cmp = fixture.componentInstance as any;
    const reads: ((c: RulesCatalogVm) => void)[] = [];
    fake.loadCatalog = (() =>
      new Promise<RulesCatalogVm>((resolve) => reads.push(resolve))) as never;
    watcher.hint();
    watcher.hint();
    await flush();
    expect(reads).toHaveLength(2);
    const spells = () => cmp.state().catalog.spells.length as number;
    const newer = catalog();
    expect(newer.spells.length).toBeGreaterThan(0);
    reads[1](newer);
    await flush();
    await settle(fixture);
    // The older read lands last, with a catalog that has lost every spell: it must not win.
    reads[0]({ ...newer, spells: [] });
    await flush();
    await settle(fixture);
    expect(spells()).toBe(newer.spells.length);
  });

  it('shows the reading of a single read of the lists (positive control)', async () => {
    const watcher = fakeContentWatcher();
    TestBed.overrideComponent(CharacterEditor, { set: { providers: [watcher.provider] } });
    TestBed.configureTestingModule({
      imports: [CharacterEditor],
      providers: [
        provideRouter([]),
        { provide: CharacterEditorSource, useClass: FakeSource },
        {
          provide: ActivatedRoute,
          useValue: { paramMap: of(convertToParamMap({ id: 'camp-1', characterId: 'ch-9' })) },
        },
      ],
    });
    const fake = TestBed.inject(CharacterEditorSource) as unknown as FakeSource;
    fake.forEdit = emptyEdit();
    const fixture = TestBed.createComponent(CharacterEditor);
    fixture.detectChanges();
    await flush();
    await settle(fixture);
    fake.catalogOver = (c) => ({ ...c, spells: [] });
    watcher.hint();
    await flush();
    await settle(fixture);
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    expect((fixture.componentInstance as any).state().catalog.spells).toHaveLength(0);
  });

  it("does not open the session's stream for the master's editor: a stream would keep the page from ever being quiet", async () => {
    const run = async (master: boolean) => {
      TestBed.resetTestingModule();
      const watcher = fakeContentWatcher();
      TestBed.overrideComponent(CharacterEditor, { set: { providers: [watcher.provider] } });
      TestBed.configureTestingModule({
        imports: [CharacterEditor],
        providers: [
          provideRouter([]),
          { provide: CharacterEditorSource, useClass: FakeSource },
          {
            provide: ActivatedRoute,
            useValue: { paramMap: of(convertToParamMap({ id: 'camp-1' })) },
          },
        ],
      });
      (TestBed.inject(CharacterEditorSource) as unknown as FakeSource).catalogOver = (c) => ({
        ...c,
        viewerIsMaster: master,
      });
      const fixture = TestBed.createComponent(CharacterEditor);
      fixture.detectChanges();
      await flush();
      await settle(fixture);
      return watcher.following();
    };
    // The master's editor follows nothing; a player's follows the campaign while it has an open session.
    expect(await run(true)).toBe('');
    expect(await run(false)).toBe('camp-1');
  });
});
