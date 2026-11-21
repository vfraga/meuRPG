import { Injectable } from '@angular/core';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap, provideRouter } from '@angular/router';
import { of } from 'rxjs';

import { create } from '@bufbuild/protobuf';

import {
  ChoiceGroupSchema,
  ChoiceKind,
  ChoiceOptionSchema,
  ChoiceOrigin,
  ChoiceSchema,
  ChoiceSpellSourceSchema,
} from '../../../gen/meurpg/characters/v1/characters_pb';
import { fakeContentWatcher } from '../../core/content/content-testing';
import { nextTurn } from '../../../test-settle';
import { CharacterEditor } from './character-editor';
import {
  CharacterEditorSource,
  ChoicesPreviewVm,
  CreateCharacterInput,
  PreviewCharacterInput,
  RulesCatalogVm,
} from './character-editor.types';

// The "Escolhas" step (PM-05): the class and race choices the server reads in the draft. These specs give the
// editor a server that answers like the real one (what is picked is what the draft stores) and read the requests.

const SRD = { fromTable: false, archived: false, off: false };
const STYLES = ['defense', 'dueling', 'archery'];

function styleChoice(keys: readonly string[]) {
  const picked = STYLES.filter((k) => keys.includes(`feature:style-${k}`));
  return create(ChoiceSchema, {
    key: 'choice:fighter-style',
    featureKey: 'feature:fighter-style',
    kind: ChoiceKind.OPTIONS,
    titlePt: 'Estilo de Luta',
    labelPt: 'Estilo de Luta',
    picks: 1,
    picked,
    missing: picked.length === 1 ? 0 : 1,
    options: STYLES.map((k) =>
      create(ChoiceOptionSchema, {
        key: k,
        storedKey: `feature:style-${k}`,
        namePt: k.toUpperCase(),
        summaryPt: `resumo ${k}`,
      }),
    ),
  });
}

function halfElfChoice(keys: readonly string[]) {
  const picked = ['str', 'dex', 'con'].filter((k) => keys.includes(`race:half-elf-${k}`));
  return create(ChoiceSchema, {
    key: 'choice:half-elf-bonus',
    featureKey: 'feature:half-elf-bonus',
    kind: ChoiceKind.OPTIONS,
    titlePt: 'Aumento de habilidade',
    labelPt: 'Aumento de habilidade',
    picks: 2,
    picked,
    missing: Math.max(2 - picked.length, 0),
    resultPt: picked.length === 2 ? 'Soma +1 em duas habilidades.' : '',
    options: ['str', 'dex', 'con'].map((k) =>
      create(ChoiceOptionSchema, {
        key: k,
        storedKey: `race:half-elf-${k}`,
        namePt: k.toUpperCase(),
        summaryPt: '+1',
      }),
    ),
  });
}

/** What the server answers for a draft: a fighter asks a fighting style, a half-elf two +1, a rogue nothing. */
function answer(input: PreviewCharacterInput): ChoicesPreviewVm {
  const keys = input.full.featureChoiceKeys;
  const groups = [];
  if (input.full.race === 'race:half-elf') {
    groups.push(
      create(ChoiceGroupSchema, {
        origin: ChoiceOrigin.RACE,
        sourceNamePt: 'Meio-elfo',
        choices: [halfElfChoice(keys)],
      }),
    );
  }
  if (input.full.className === 'class:fighter') {
    groups.push(
      create(ChoiceGroupSchema, {
        origin: ChoiceOrigin.CLASS,
        sourceNamePt: 'Guerreiro',
        level: 1,
        choices: [styleChoice(keys)],
      }),
    );
  }
  const open = groups.flatMap((g) => g.choices).reduce((n, c) => n + c.missing, 0);
  const total = groups.flatMap((g) => g.choices).reduce((n, c) => n + c.picks, 0);
  const spellSources =
    input.full.className === 'class:warlock'
      ? [
          create(ChoiceSpellSourceSchema, {
            classKey: 'class:warlock',
            patronSpells: [
              { spellKey: 'spell:burning-hands', classLevel: 1 },
              { spellKey: 'spell:fireball', classLevel: 5 },
            ],
          }),
        ]
      : [];
  return {
    groups,
    done: total - open,
    total,
    notOffered: [],
    spellSources,
    skillsAllowed: null,
    expertiseAllowed: null,
  };
}

function catalog(): RulesCatalogVm {
  const klass = (key: string, namePt: string, isCaster: boolean) => ({
    key,
    namePt,
    hitDie: 8,
    isCaster,
    preparation: isCaster ? ('known' as const) : null,
    subclasses: [],
    subclassLevel: 3,
    spellcastingFirstLevel: isCaster ? 1 : 0,
    maxSpellLevelByLevel: isCaster
      ? [1, 1, 2, 2, 3, 3, 4, 4, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5]
      : [],
    skillChoose: 2,
    savingThrows: ['str' as const, 'wis' as const],
    spellListClassKey: isCaster ? key : '',
    ...SRD,
  });
  const race = (key: string, namePt: string) => ({
    key,
    namePt,
    constitutionBonus: 0,
    choiceBonuses: [],
    subraces: [],
    ...SRD,
  });
  const spell = (key: string, namePt: string, level: number, classKeys: string[]) => ({
    key,
    namePt,
    level,
    classKeys,
    ...SRD,
  });
  return {
    races: [race('race:half-elf', 'Meio-elfo'), race('race:human', 'Humano')],
    classes: [
      klass('class:fighter', 'Guerreiro', false),
      klass('class:rogue', 'Ladino', false),
      klass('class:warlock', 'Bruxo', true),
    ],
    backgrounds: [{ key: 'background:acolyte', namePt: 'Acólito', equipmentPt: '', ...SRD }],
    skills: [],
    armor: [],
    weapons: [],
    spells: [
      spell('spell:eldritch-blast', 'Rajada Mística', 0, ['class:warlock']),
      spell('spell:hex', 'Maldição', 1, ['class:warlock']),
      spell('spell:burning-hands', 'Mãos Flamejantes', 1, ['class:wizard']),
      spell('spell:fireball', 'Bola de Fogo', 3, ['class:wizard']),
    ],
    toolsAndLanguages: [],
    viewerIsMaster: false,
    challengeRatings: [],
  };
}

/** The viewer of the next render is the campaign's master. */
let viewerIsMaster = false;

@Injectable()
class FakeSource {
  previews: PreviewCharacterInput[] = [];
  created: CreateCharacterInput[] = [];
  loadCatalog() {
    return Promise.resolve({ ...catalog(), viewerIsMaster });
  }
  loadSpellDetails() {
    return Promise.reject(new Error('unused'));
  }
  loadCharacterForEdit() {
    return Promise.reject(new Error('unused'));
  }
  loadAbilityTable() {
    return Promise.resolve(null);
  }
  rollAbilityScores() {
    return Promise.reject(new Error('unused'));
  }
  previewCharacter() {
    return Promise.reject(new Error('unused'));
  }
  previewChoices(input: PreviewCharacterInput) {
    this.previews.push(input);
    return Promise.resolve(answer(input));
  }
  createCharacter(input: CreateCharacterInput) {
    this.created.push(input);
    return Promise.resolve({ characterId: 'new' });
  }
  updateCharacter() {
    return Promise.resolve({ revision: 2 });
  }
}

const wait = (ms: number) => vi.advanceTimersByTimeAsync(ms);
/** Past the pause the editor lets the draft rest before asking the server. */
const PAUSE = 350;

async function settle(fixture: ComponentFixture<CharacterEditor>) {
  fixture.detectChanges();
  await fixture.whenStable();
  fixture.detectChanges();
}

async function render() {
  // The editor's pause before asking the server is crossed by advancing this clock, not by sleeping.
  vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] });
  const watcher = fakeContentWatcher();
  TestBed.overrideComponent(CharacterEditor, { set: { providers: [watcher.provider] } });
  TestBed.configureTestingModule({
    imports: [CharacterEditor],
    providers: [
      provideRouter([]),
      { provide: CharacterEditorSource, useClass: FakeSource },
      { provide: ActivatedRoute, useValue: { paramMap: of(convertToParamMap({ id: 'camp-1' })) } },
    ],
  });
  const fake = TestBed.inject(CharacterEditorSource) as unknown as FakeSource;
  const fixture = TestBed.createComponent(CharacterEditor);
  fixture.detectChanges();
  await nextTurn();
  await settle(fixture);
  const el = fixture.nativeElement as HTMLElement;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const cmp = fixture.componentInstance as any;
  /** Fills the draft and waits for the server's answer. */
  const draft = async (race: string, className: string, level = 1) => {
    cmp.fullForm.patchValue({
      name: 'Tharn',
      race,
      className,
      level,
      background: 'background:acolyte',
    });
    await wait(PAUSE);
    await settle(fixture);
  };
  const tabs = () => Array.from(el.querySelectorAll<HTMLElement>('[role="tab"]'));
  const tab = (label: string) => tabs().find((t) => t.textContent?.includes(label));
  const create = () => el.querySelector<HTMLButtonElement>('.editor__primary')!;
  return { fixture, fake, el, cmp, draft, tabs, tab, create };
}

describe('CharacterEditor, the Escolhas step (PM-05)', () => {
  afterEach(() => vi.useRealTimers());

  beforeAll(async () => {
    await render();
    TestBed.resetTestingModule();
  });

  it('has no step while the draft asks nothing, and gets it between Habilidades and Perícias', async () => {
    const { draft, tabs } = await render();
    const names = () => tabs().map((t) => t.querySelector('.stepper__label')?.textContent?.trim());
    expect(names()).toEqual(['Básico', 'Habilidades', 'Perícias', 'Equipamento']);

    await draft('race:human', 'class:rogue');
    expect(names()).toEqual(['Básico', 'Habilidades', 'Perícias', 'Equipamento']);

    await draft('race:human', 'class:fighter');
    expect(names()).toEqual(['Básico', 'Habilidades', 'Escolhas', 'Perícias', 'Equipamento']);

    await draft('race:human', 'class:rogue');
    expect(names()).not.toContain('Escolhas');
  });

  it('marks the step with "!" and "Escolha pendente" until the choice is made', async () => {
    const { draft, tab, cmp, fixture, fake } = await render();
    await draft('race:human', 'class:fighter');
    expect(tab('Escolhas')?.querySelector('.stepper__pending')?.textContent).toBe('!');
    expect(tab('Escolhas')?.textContent).toContain('Escolha pendente');

    const groups = cmp.server.choiceGroups();
    cmp.picks.pickChoice({ choice: groups[0].choices[0], optionKeys: ['dueling'] });
    await wait(PAUSE);
    await settle(fixture);

    expect(fake.previews.at(-1)?.full.featureChoiceKeys).toEqual(['feature:style-dueling']);
    expect(tab('Escolhas')?.querySelector('.stepper__pending')).toBeNull();
    expect(tab('Escolhas')?.textContent).not.toContain('Escolha pendente');
  });

  it('draws the choices of the server in the step, with what a pick gives', async () => {
    const { draft, tab, el, fixture } = await render();
    await draft('race:half-elf', 'class:fighter');
    tab('Escolhas')!.click();
    await settle(fixture);

    const titles = Array.from(el.querySelectorAll('app-choice-groups .choice__title')).map((t) =>
      t.textContent?.replace(/\s+/g, ' ').trim(),
    );
    expect(titles[0]).toContain('Aumento de habilidade');
    expect(titles[1]).toContain('Estilo de Luta');
    expect(el.querySelector('app-choice-groups .choices__counter')?.textContent).toContain(
      'Escolhas feitas: 0 de 3',
    );
  });

  it('keeps "Criar personagem" aria-disabled but focusable, described by what is missing', async () => {
    const { draft, create, fake, el, fixture } = await render();
    await draft('race:human', 'class:fighter');

    const button = create();
    expect(button.getAttribute('aria-disabled')).toBe('true');
    expect(button.disabled).toBe(false);
    button.focus();
    expect(document.activeElement).toBe(button);
    const sentence = el.querySelector(`#${button.getAttribute('aria-describedby')}`);
    expect(sentence?.textContent?.trim()).toBe('Falta uma escolha: Estilo de Luta.');

    // Activating it goes to the first choice still open instead of saving.
    button.click();
    await settle(fixture);
    expect(fake.created.length).toBe(0);
    expect((document.activeElement as HTMLElement).closest('[data-pending]')).not.toBeNull();
    expect(document.activeElement?.classList.contains('choice__title')).toBe(true);
    expect(el.querySelector('.stepper__panel:not([hidden]) app-choice-groups')).not.toBeNull();
  });

  it('creates with the picks and the texts once nothing is open, and sends them in order', async () => {
    const { draft, create, cmp, fixture, fake } = await render();
    await draft('race:half-elf', 'class:fighter');
    const [race, klass] = cmp.server.choiceGroups();

    cmp.picks.pickChoice({ choice: klass.choices[0], optionKeys: ['archery'] });
    cmp.picks.pickChoice({ choice: race.choices[0], optionKeys: ['dex'] });
    await wait(PAUSE);
    await settle(fixture);
    cmp.picks.pickChoice({
      choice: cmp.server.choiceGroups()[0].choices[0],
      optionKeys: ['dex', 'con'],
    });
    cmp.picks.editChoiceText({ choice: klass.choices[0], n: 1, text: 'Orcs' });
    await wait(PAUSE);
    await settle(fixture);

    expect(create().getAttribute('aria-disabled')).toBe('false');
    expect(fixture.nativeElement.querySelector('#choices-missing')).toBeNull();
    create().click();
    await settle(fixture);

    expect(fake.created.length).toBe(1);
    expect(fake.created[0].full?.featureChoiceKeys).toEqual([
      'feature:style-archery',
      'race:half-elf-dex',
      'race:half-elf-con',
    ]);
    expect(fake.created[0].full?.featureChoiceText).toEqual({ 'choice:fighter-style#1': 'Orcs' });
  });

  it('does not block the master, whose NPCs and edits the server does not refuse', async () => {
    viewerIsMaster = true;
    try {
      const { draft, create, tab } = await render();
      await draft('race:human', 'class:fighter');
      expect(tab('Escolhas')?.querySelector('.stepper__pending')).not.toBeNull();
      expect(create().getAttribute('aria-disabled')).toBe('false');
      expect(create().hasAttribute('aria-describedby')).toBe(false);
    } finally {
      viewerIsMaster = false;
    }
  });

  it('leaves the half-elf out of the manual bonuses: the +1 are picks on the Escolhas step', async () => {
    const { draft, tab, el, fixture } = await render();
    await draft('race:half-elf', 'class:rogue');

    tab('Habilidades')!.click();
    await settle(fixture);
    const bonuses = el.querySelector('.bonuses')!;
    expect(bonuses.textContent).not.toContain('raça');
    expect(bonuses.textContent).toContain('passo Escolhas');
    expect(el.querySelector('.bonuses__sub')?.textContent).not.toContain('escolhas de raça');

    tab('Escolhas')!.click();
    await settle(fixture);
    expect(el.querySelector('app-choice-groups')?.textContent).toContain('Aumento de habilidade');
  });

  it('adds the cantrip a choice asks for to the sheet, then reads the draft again', async () => {
    const { draft, cmp, fake } = await render();
    await draft('race:human', 'class:warlock');

    cmp.spellPicks.addCantrip('spell:eldritch-blast');
    await wait(PAUSE);

    expect(fake.previews.at(-1)?.full.cantrips).toEqual(['spell:eldritch-blast']);
  });

  it('drops a pick no choice offers any more when the class changes', async () => {
    const { draft, cmp, fixture, fake } = await render();
    await draft('race:human', 'class:fighter');
    cmp.picks.pickChoice({
      choice: cmp.server.choiceGroups()[0].choices[0],
      optionKeys: ['defense'],
    });
    await wait(PAUSE);
    await settle(fixture);
    expect(cmp.picks.featureChoiceKeys()).toEqual(['feature:style-defense']);

    fake.previewChoices = (input: PreviewCharacterInput) => {
      fake.previews.push(input);
      return Promise.resolve({ ...answer(input), notOffered: ['feature:style-defense'] });
    };
    await draft('race:human', 'class:rogue');
    await wait(PAUSE);
    await settle(fixture);

    expect(cmp.picks.featureChoiceKeys()).toEqual([]);
  });

  it("lists the patron's spells as an extra source of the Magias step, from the class level they come at", async () => {
    const { draft, tab, el, cmp, fixture } = await render();
    await draft('race:human', 'class:warlock', 1);
    tab('Magias')!.click();
    await settle(fixture);

    const extra = el.querySelector('.spell-grid__extra');
    expect(extra?.textContent).toContain('Do patrono');
    expect(extra?.textContent).toContain('Mãos Flamejantes');
    // Bola de Fogo comes at the class level 5: not yet.
    expect(extra?.textContent).not.toContain('Bola de Fogo');

    cmp.spellPicks.toggleKnown('spell:burning-hands');
    await settle(fixture);
    expect(cmp.draft.build().spellsKnown).toEqual(['spell:burning-hands']);
  });
});
