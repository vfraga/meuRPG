import { ComponentFixture, TestBed } from '@angular/core/testing';
import { Router, provideRouter } from '@angular/router';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import { DicePreference, XpMode } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  ChoiceGroupSchema,
  ChoiceKind,
  ChoiceOptionSchema,
  ChoiceSchema,
  LevelUpFeatureChoiceSchema,
  CharacterBlockedReason,
  CharacterBlockedSchema,
  CharacterSchema,
  FullSheetSchema,
  LevelUpClassUnavailable,
  LevelUpDiceRule,
  LevelUpHitPointsRule,
  LevelUpRefusalReason,
  LevelUpRefusalSchema,
  type LevelUpOptions,
} from '../../../gen/meurpg/characters/v1/characters_pb';
import { Ability, DerivedClassSchema } from '../../../gen/meurpg/rules/v1/rules_pb';
import { fakeContentWatcher } from '../../core/content/content-testing';
import { createRouterTransport } from '@connectrpc/connect';

import { CampaignService } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { CharacterService } from '../../../gen/meurpg/characters/v1/characters_pb';
import {
  ContentSchema,
  ContentService,
  ListContentResponseSchema,
} from '../../../gen/meurpg/rules/v1/rules_pb';
import { CONNECT_TRANSPORT } from '../../core/connect/transport';
import { LevelUpClient } from '../../core/levelup/levelup-client';
import {
  SKILLS,
  SPELLS,
  WIZARD_KEYS,
  classChoice,
  featOptions,
  fighterOptions,
  pensantus,
  pensantusClassChoices,
  wizardOptions,
} from '../../core/levelup/levelup-testing';
import { LevelUpPage } from './level-up';
import { newSpellsTitle } from './level-up-session';
import { QUIET_MS } from './level-up-preview';

// With --coverage test-setup.ts only hooks the first file of a worker, so the fake clock this file leaves on would
// reach the next file (a real setTimeout never fires on a fake clock).
afterEach(() => {
  vi.useRealTimers();
});

/** Lets every pending answer land, quiet period of the preview included: the spec's fake clock moves, the wall clock does not. */
const settle = () => vi.advanceTimersByTimeAsync(QUIET_MS * 2);

function character(over: object = {}, derived = pensantus()) {
  return create(CharacterSchema, {
    id: 'ch-1',
    name: 'Pensantus',
    revision: 5,
    canLevelUp: true,
    derived,
    sheet: {
      content: {
        case: 'full',
        value: create(FullSheetSchema, {
          cantripKeys: [...WIZARD_KEYS.cantrips],
          knownSpellKeys: [...WIZARD_KEYS.known],
          preparedSpellKeys: [...WIZARD_KEYS.prepared],
          skillProficiencyKeys: [...WIZARD_KEYS.skills],
        }),
      },
    },
    ...over,
  });
}

describe('LevelUpPage', () => {
  const client = {
    character: vi.fn(),
    options: vi.fn(),
    catalog: vi.fn(),
    dicePreference: vi.fn(),
    preview: vi.fn(),
    rollHitPoints: vi.fn(),
    levelUp: vi.fn(),
    spellDetails: vi.fn(),
    xpMode: vi.fn(),
  };
  let navigate: ReturnType<typeof vi.spyOn>;
  let watcher = fakeContentWatcher();

  beforeEach(() => {
    vi.useFakeTimers();
    // jsdom has no layout: scrolling does nothing.
    Element.prototype.scrollIntoView = vi.fn();
    window.scrollTo = vi.fn();
  });

  async function setup(
    options: LevelUpOptions = wizardOptions({ preparedMaxAfter: 3 }),
    char = character(),
    optionsError?: Error,
    classes?: { key: string; namePt: string; hitDie?: number }[],
    atClass = false,
  ) {
    client.character.mockReset().mockResolvedValue(char);
    client.options.mockReset();
    if (optionsError) {
      client.options.mockRejectedValue(optionsError);
    } else {
      client.options.mockResolvedValue(options);
    }
    client.catalog.mockReset().mockResolvedValue({ spells: SPELLS, skills: SKILLS, classes });
    client.dicePreference.mockReset().mockResolvedValue(DicePreference.APP);
    // The new maximum of prepared spells: 4, two more than the two prepared today.
    const after = pensantus(true);
    after.spellcasting[0].preparedMax = 4;
    client.preview.mockReset().mockResolvedValue({ after, refusal: undefined });
    client.xpMode.mockReset().mockResolvedValue(XpMode.MILESTONES);
    client.rollHitPoints.mockReset().mockResolvedValue({ die: 6, value: 5, alreadyRolled: false });
    client.levelUp.mockReset().mockResolvedValue(character({ canLevelUp: false }, pensantus(true)));
    watcher = fakeContentWatcher();
    TestBed.configureTestingModule({
      providers: [provideRouter([]), { provide: LevelUpClient, useValue: client }],
    });
    // The page makes its own client (so the catalog never outlives it): the test's takes its place there too, and the
    // session's stream is not opened (the fake watcher plays the hint).
    TestBed.overrideComponent(LevelUpPage, {
      set: { providers: [{ provide: LevelUpClient, useValue: client }, watcher.provider] },
    });
    // The route's params, read the way the page reads them.
    const { ActivatedRoute } = await import('@angular/router');
    TestBed.overrideProvider(ActivatedRoute, {
      useValue: {
        paramMap: (await import('rxjs')).of({
          get: (k: string) => (k === 'id' ? 'camp-1' : 'ch-1'),
        }),
      },
    });
    navigate = vi.spyOn(TestBed.inject(Router), 'navigate').mockResolvedValue(true);
    const fixture = TestBed.createComponent(LevelUpPage);
    await load(fixture);
    // The class step opens every flow ("Subir em qual classe?"); most specs are about the steps after it.
    const next = Array.from(el(fixture).querySelectorAll<HTMLButtonElement>('button')).find(
      (b) => b.textContent?.trim() === 'Próximo',
    );
    if (!atClass && next) {
      await click(fixture, next);
    }
    return fixture;
  }

  async function load(fixture: ComponentFixture<LevelUpPage>) {
    fixture.detectChanges();
    await fixture.whenStable();
    await settle();
    fixture.detectChanges();
  }

  const el = (f: ComponentFixture<LevelUpPage>) => f.nativeElement as HTMLElement;
  const text = (f: ComponentFixture<LevelUpPage>) => el(f).textContent?.replace(/\s+/g, ' ') ?? '';
  const button = (f: ComponentFixture<LevelUpPage>, name: string | RegExp) =>
    Array.from(el(f).querySelectorAll<HTMLButtonElement>('button')).find((b) =>
      typeof name === 'string' ? b.textContent?.trim() === name : name.test(b.textContent ?? ''),
    )!;
  async function click(f: ComponentFixture<LevelUpPage>, target: Element | null | undefined) {
    (target as HTMLElement).click();
    f.detectChanges();
    await f.whenStable();
    await settle();
    f.detectChanges();
  }
  const pickRow = (f: ComponentFixture<LevelUpPage>, name: string) =>
    Array.from(el(f).querySelectorAll<HTMLElement>('.row__main, .row')).find((r) =>
      r.textContent?.includes(name),
    )!;

  it('lists the five steps of Pensantus, the class first, and starts at Habilidades once it is chosen', async () => {
    const f = await setup();
    expect(text(f)).toContain('Subir para o nível 4');
    expect(text(f)).toContain('Pensantus · Mago 3 → Mago 4');
    expect(text(f)).toContain('Passo 2 de 5 · Habilidades');
    expect(Array.from(el(f).querySelectorAll('.step__label')).map((s) => s.textContent)).toEqual([
      'Classe',
      'Habilidades',
      'Vida',
      'Magias',
      'Resumo',
    ]);
  });

  it('offers "Aumentar habilidades" or "Um talento" when the table lists feats, and sends the feat with the abilities it raises', async () => {
    const f = await setup(wizardOptions({ preparedMaxAfter: 3, feats: featOptions() }));
    expect(text(f)).toContain('Incremento ou talento');
    expect(text(f)).toContain('Aumentar habilidades');
    expect(text(f)).not.toContain('Atleta');
    await click(
      f,
      el(f).querySelector<HTMLInputElement>('input[name="asi-take"][type="radio"]:not(:checked)'),
    );
    expect(text(f)).toContain('Atleta');
    expect(text(f)).toContain('Precisa de Força 15.');
    expect(text(f)).toContain('Falta escolher o talento.');
    const radios = Array.from(
      el(f).querySelectorAll<HTMLInputElement>('app-feat-picker input[type="radio"]'),
    );
    expect(radios.map((r) => r.disabled)).toEqual([false, false, true]);
    await click(f, radios[1]);
    expect(text(f)).toContain('Falta escolher 1 habilidade.');
    await click(f, el(f).querySelector<HTMLInputElement>('.ability input'));
    expect(text(f)).not.toContain('Falta escolher 1 habilidade.');
    const sent = client.preview.mock.calls.at(-1)![2];
    expect(sent).toMatchObject({ featKey: 'feat:atleta@mesa', abilityIncrease: { strength: 1 } });
  });

  it('keeps the step as it is today when the server lists no feats', async () => {
    const f = await setup();
    expect(text(f)).toContain('Incremento no Valor de Habilidade');
    expect(text(f)).not.toContain('Um talento');
    expect(text(f)).toContain('Esta mesa não usa talentos');
    expect(client.preview.mock.calls.at(-1)?.[2]).toMatchObject({ featKey: '' });
  });

  it('has only Vida and Resumo when the level has nothing else to choose (Toren)', async () => {
    const f = await setup(fighterOptions());
    expect(text(f)).toContain('Passo 2 de 3 · Vida');
    expect(text(f)).toContain('Neste nível não há mais nada para escolher');
    expect(text(f)).toContain('Ataque Extra');
  });

  it('keeps "Próximo" asking for the missing choice, and a tap puts the focus on it', async () => {
    const f = await setup();
    const next = button(f, 'Próximo');
    expect(next.getAttribute('aria-disabled')).toBe('true');
    expect(text(f)).toContain('Falta escolher 1 habilidade.');
    await click(f, next);
    expect(text(f)).toContain('Passo 2 de 5');
    expect(el(f).querySelector('#pick-abilities')?.contains(document.activeElement)).toBe(true);
  });

  it('goes on when the step is complete, and Voltar goes back', async () => {
    const f = await setup();
    await click(f, el(f).querySelector('.row--on, .row'));
    // The first ability row is Força.
    await click(f, button(f, 'Próximo'));
    expect(text(f)).toContain('Passo 3 de 5 · Vida');
    await click(f, button(f, 'Voltar'));
    expect(text(f)).toContain('Passo 2 de 5 · Habilidades');
  });

  describe('the class step: "Subir em qual classe?"', () => {
    const twoClassChoices = () => [
      classChoice({
        classKey: 'class:wizard',
        namePt: 'Mago',
        fromLevel: 3,
        toLevel: 4,
        subclassNamePt: 'Evocação',
      }),
      classChoice({ classKey: 'class:cleric', namePt: 'Clérigo', fromLevel: 1, toLevel: 2 }),
      ...pensantusClassChoices().slice(1),
    ];
    const cleric = (over: Parameters<typeof wizardOptions>[0] = {}) =>
      wizardOptions({
        classKey: 'class:cleric',
        classNamePt: 'Clérigo',
        classChoices: twoClassChoices(),
        fromLevel: 1,
        toLevel: 2,
        totalFromLevel: 4,
        totalToLevel: 5,
        hitDie: 8,
        hitPointAverage: 5,
        abilityScoreImprovement: false,
        cantrips: 0,
        spells: 0,
        prepares: false,
        preparedMax: 0,
        preparedMaxAfter: 0,
        spellsKind: 0,
        newFeatures: [{ key: 'feature:channel-divinity', namePt: 'Canalizar Divindade' }],
        ...over,
      });
    const twoClasses = () => {
      const derived = pensantus();
      derived.classes.push(
        create(DerivedClassSchema, { classKey: 'class:cleric', namePt: 'Clérigo', level: 1 }),
      );
      derived.totalLevel = 4;
      return character({}, derived);
    };
    const wizardTwo = () =>
      wizardOptions({
        totalToLevel: 5,
        preparedMaxAfter: 3,
        classChoices: twoClassChoices(),
      });
    /** The page at its class step, for a sheet with two classes. */
    async function multiclass(
      wizard: LevelUpOptions = wizardTwo(),
      clericOptions: LevelUpOptions = cleric(),
      classes?: { key: string; namePt: string; hitDie?: number }[],
    ) {
      const f = await setup(wizard, twoClasses(), undefined, classes, true);
      client.options.mockImplementation((_c: string, _ch: string, key = '') =>
        Promise.resolve(key === 'class:cleric' ? clericOptions : wizard),
      );
      return f;
    }
    const cards = (f: ComponentFixture<LevelUpPage>) =>
      Array.from(el(f).querySelectorAll<HTMLLabelElement>('app-class-pick .card'));
    const radio = (f: ComponentFixture<LevelUpPage>, name: string) =>
      cards(f)
        .find((c) => c.querySelector('.card__title')?.textContent?.trim() === name)!
        .querySelector<HTMLInputElement>('input')!;
    const step = (f: ComponentFixture<LevelUpPage>) =>
      el(f).querySelector('.head__step')?.textContent?.replace(/\s+/g, ' ').trim();

    it('opens every flow, even for a sheet with one class: the classes it has and "Uma classe nova"', async () => {
      const f = await setup(wizardOptions(), character(), undefined, undefined, true);
      expect(text(f)).toContain('Subir em qual classe?');
      expect(step(f)).toBe('Passo 1 de 5 · Classe');
      expect(
        cards(f).map((c) =>
          Array.from(c.querySelectorAll('.card__title, .card__desc'), (x) =>
            x.textContent?.replace(/\s+/g, ' ').trim(),
          ).join(' '),
        ),
      ).toEqual([
        'Mago nível 3 → 4 · Evocação',
        'Uma classe nova Entra com o nível 1 da classe: o seu nível total vai a 4.',
      ]);
      expect(radio(f, 'Mago').checked).toBe(true);
      expect(radio(f, 'Uma classe nova').checked).toBe(false);
      // The new classes only open when asked for.
      expect(text(f)).not.toContain('Qual classe nova?');
      expect(client.options).toHaveBeenCalledWith('camp-1', 'ch-1');
      expect(document.activeElement).toBe(radio(f, 'Mago'));
      // The class in force is chosen: "Próximo" is open.
      expect(button(f, 'Próximo').getAttribute('aria-disabled')).not.toBe('true');
    });

    it('draws a card for each class of the sheet, the first one checked, and reads the options with no class', async () => {
      const f = await multiclass();
      expect(
        cards(f)
          .slice(0, 3)
          .map((c) => c.querySelector('.card__title')?.textContent),
      ).toEqual(['Mago', 'Clérigo', 'Uma classe nova']);
      expect(radio(f, 'Mago').checked).toBe(true);
      expect(radio(f, 'Clérigo').checked).toBe(false);
      expect(client.options).toHaveBeenCalledWith('camp-1', 'ch-1');
      expect(text(f)).toContain('Subir para o nível 5');
      expect(text(f)).toContain('Pensantus · Mago 3 → Mago 4');
    });

    it('goes to the other class at once when nothing was chosen: its options, steps, subtitle and the lines of its level', async () => {
      const f = await multiclass();
      await click(f, radio(f, 'Clérigo'));
      expect(client.options).toHaveBeenLastCalledWith('camp-1', 'ch-1', 'class:cleric');
      expect(radio(f, 'Clérigo').checked).toBe(true);
      expect(text(f)).toContain('Subir para o nível 5');
      expect(text(f)).toContain('Pensantus · Clérigo 1 → Clérigo 2');
      expect(step(f)).toBe('Passo 1 de 3 · Classe');
      expect(el(f).querySelector('[role="status"].mr-visually-hidden')?.textContent).toContain(
        'Clérigo escolhido. O nível tem 3 passos.',
      );
      expect(document.activeElement).toBe(radio(f, 'Clérigo'));
      await click(f, button(f, 'Próximo'));
      expect(text(f)).toContain('Passo 2 de 3 · Vida');
      expect(text(f)).toContain('Só o que o nível 2 de Clérigo dá fica aberto.');
      expect(text(f)).toContain('O que o nível 2 de Clérigo dá');
      expect(text(f)).toContain('O Clérigo ganha 1d8 por nível');
      // The preview is asked for the class that gains the level.
      expect(client.preview.mock.calls.at(-1)?.[2].classKey).toBe('class:cleric');
    });

    it('says the level of the class, never the total, in a hit points step of a multiclass sheet', async () => {
      const f = await multiclass();
      await click(f, radio(f, 'Clérigo'));
      await click(f, button(f, 'Próximo'));
      expect(text(f)).not.toContain('O que o nível 5 dá');
      expect(text(f)).not.toContain('Só o que o nível 2 dá');
    });

    /** Past the class step to Habilidades, picks the first ability, and back to the class step. */
    async function chooseThenBack(f: ComponentFixture<LevelUpPage>) {
      await click(f, button(f, 'Próximo'));
      await click(f, el(f).querySelector('.row__input'));
      await click(f, button(f, 'Voltar'));
    }

    it('asks in place after a choice was made: "Continuar com o Mago" first and focused, and nothing is read', async () => {
      const f = await multiclass();
      await chooseThenBack(f);
      const reads = client.options.mock.calls.length;
      await click(f, radio(f, 'Clérigo'));
      const ask = el(f).querySelector('app-class-pick [role="group"]')!;
      expect(ask.textContent).toContain('Trocar de classe?');
      expect(ask.textContent).toContain('As escolhas deste nível são descartadas.');
      expect(Array.from(ask.querySelectorAll('button'), (b) => b.textContent?.trim())).toEqual([
        'Trocar para o Clérigo',
        'Continuar com o Mago',
      ]);
      expect(document.activeElement?.textContent?.trim()).toBe('Continuar com o Mago');
      // The class in force stays checked behind the question.
      await vi.advanceTimersByTimeAsync(0);
      expect(radio(f, 'Mago').checked).toBe(true);
      expect(radio(f, 'Clérigo').checked).toBe(false);
      expect(client.options.mock.calls.length).toBe(reads);
      await click(f, button(f, 'Continuar com o Mago'));
      expect(el(f).querySelector('app-class-pick [role="group"]')).toBeNull();
      expect(text(f)).toContain('Pensantus · Mago 3 → Mago 4');
      expect(document.activeElement).toBe(radio(f, 'Mago'));
    });

    it('"Trocar para o Clérigo" throws the choices away and reads the other class', async () => {
      const f = await multiclass();
      await chooseThenBack(f);
      await click(f, radio(f, 'Clérigo'));
      await click(f, button(f, 'Trocar para o Clérigo'));
      expect(client.options).toHaveBeenLastCalledWith('camp-1', 'ch-1', 'class:cleric');
      expect(text(f)).toContain('Pensantus · Clérigo 1 → Clérigo 2');
      expect(el(f).querySelector('app-class-pick [role="group"]')).toBeNull();
      // Back to the Mago: nothing of the first draft is left, so it does not ask again.
      await click(f, radio(f, 'Mago'));
      await click(f, button(f, 'Próximo'));
      expect(text(f)).toContain('Falta escolher 1 habilidade.');
    });

    it('the roll in the app is asked for the class that gains the level', async () => {
      const f = await multiclass();
      await click(f, radio(f, 'Clérigo'));
      await click(f, button(f, 'Próximo'));
      await click(
        f,
        Array.from(el(f).querySelectorAll('.dice-choice__card')).find((c) =>
          c.textContent?.includes('Rolar 1d8'),
        ),
      );
      await click(f, button(f, /Rolar no app/));
      expect(client.rollHitPoints).toHaveBeenCalledWith(
        'camp-1',
        'ch-1',
        'class:cleric',
        expect.any(String),
      );
    });

    it('a die already rolled for the other class: the die card is dashed with the reason in it, the average stays, and no roll is asked', async () => {
      const f = await multiclass(
        wizardTwo(),
        cleric({ keptHitPointRoll: 3, keptHitPointRollClassKey: 'class:wizard' }),
        [{ key: 'class:wizard', namePt: 'Mago', hitDie: 6 }],
      );
      await click(f, radio(f, 'Clérigo'));
      await click(f, button(f, 'Próximo'));
      const die = Array.from(el(f).querySelectorAll<HTMLElement>('.dice-choice__card')).find((c) =>
        c.textContent?.includes('Rolar 1d8'),
      )!;
      expect(die.classList).toContain('dice-choice__card--off');
      expect(die.querySelector('input')?.disabled).toBe(true);
      expect(die.textContent).toContain(
        'O dado deste nível já foi rolado para o Mago (3 no d6). Volte ao Mago para usar esse resultado, ou fique com a média.',
      );
      expect(text(f)).toContain('Média: 5');
      expect(el(f).querySelector<HTMLInputElement>('.dice-choice__card input:checked')?.value).toBe(
        '0',
      );
      expect(client.rollHitPoints).not.toHaveBeenCalled();
    });

    it('a die rolled for this very class is taken back as before (the other card is no different)', async () => {
      const f = await multiclass(
        wizardOptions({
          totalToLevel: 5,
          preparedMaxAfter: 3,
          classChoices: twoClassChoices(),
          keptHitPointRoll: 3,
          keptHitPointRollClassKey: 'class:wizard',
        }),
      );
      await click(f, button(f, 'Próximo'));
      await click(f, el(f).querySelector('.row__input'));
      await click(f, button(f, 'Próximo'));
      expect(el(f).querySelector('.dice-choice__card--off')).toBeNull();
      expect(text(f)).not.toContain('já foi rolado para');
    });

    describe('"Uma classe nova"', () => {
      const mage = () =>
        wizardOptions({
          classKey: 'class:wizard',
          classNamePt: 'Mago',
          classChoices: doranChoices(),
          isNewClass: true,
          fromLevel: 0,
          toLevel: 1,
          totalFromLevel: 5,
          totalToLevel: 6,
          hitDie: 6,
          hitPointAverage: 4,
          abilityScoreImprovement: false,
          cantrips: 3,
          spells: 6,
          preparedMaxAfter: 2,
          spellSlotsBefore: [],
          spellSlotsAfter: [2],
        });
      const fighterFive = () => {
        const derived = pensantus();
        derived.classes = [
          create(DerivedClassSchema, { classKey: 'class:fighter', namePt: 'Guerreiro', level: 5 }),
        ];
        derived.totalLevel = 5;
        return character({ name: 'Doran' }, derived);
      };
      const doranChoices = () => [
        classChoice({
          classKey: 'class:fighter',
          namePt: 'Guerreiro',
          fromLevel: 5,
          toLevel: 6,
          subclassNamePt: 'Campeão',
          prerequisiteAnyOf: true,
          prerequisites: [
            { ability: Ability.STRENGTH, minimum: 13, have: 16, met: true },
            { ability: Ability.DEXTERITY, minimum: 13, have: 14, met: true },
          ],
        }),
        classChoice({
          classKey: 'class:wizard',
          namePt: 'Mago',
          isNew: true,
          toLevel: 1,
          prerequisites: [{ ability: Ability.INTELLIGENCE, minimum: 13, have: 13, met: true }],
        }),
        classChoice({
          classKey: 'class:barbarian',
          namePt: 'Bárbaro',
          isNew: true,
          toLevel: 1,
          prerequisites: [{ ability: Ability.STRENGTH, minimum: 13, have: 16, met: true }],
        }),
        classChoice({
          classKey: 'class:bard',
          namePt: 'Bardo',
          isNew: true,
          toLevel: 1,
          available: false,
          prerequisiteMet: false,
          unavailable: LevelUpClassUnavailable.PREREQUISITE,
          prerequisites: [{ ability: Ability.CHARISMA, minimum: 13, have: 9, met: false }],
        }),
        classChoice({
          classKey: 'class:monk',
          namePt: 'Monge',
          isNew: true,
          toLevel: 1,
          available: false,
          prerequisiteMet: false,
          unavailable: LevelUpClassUnavailable.PREREQUISITE,
          prerequisites: [
            { ability: Ability.DEXTERITY, minimum: 13, have: 14, met: true },
            { ability: Ability.WISDOM, minimum: 13, have: 11, met: false },
          ],
        }),
      ];
      const fighter = () =>
        fighterOptions({
          fromLevel: 5,
          toLevel: 6,
          totalFromLevel: 5,
          totalToLevel: 6,
          classChoices: doranChoices(),
        });
      const barbarian = () =>
        wizardOptions({
          classKey: 'class:barbarian',
          classNamePt: 'Bárbaro',
          classChoices: doranChoices(),
          isNewClass: true,
          fromLevel: 0,
          toLevel: 1,
          totalFromLevel: 5,
          totalToLevel: 6,
          hitDie: 12,
          hitPointAverage: 7,
          abilityScoreImprovement: false,
          cantrips: 0,
          spells: 0,
          prepares: false,
          preparedMax: 0,
          preparedMaxAfter: 0,
          spellsKind: 0,
          spellSlotsBefore: [],
          spellSlotsAfter: [],
          newFeatures: [{ key: 'feature:rage', namePt: 'Fúria' }],
        });
      async function doran() {
        const f = await setup(fighter(), fighterFive(), undefined, undefined, true);
        client.options.mockImplementation((_c: string, _ch: string, key = '') =>
          Promise.resolve(
            key === 'class:wizard' ? mage() : key === 'class:barbarian' ? barbarian() : fighter(),
          ),
        );
        return f;
      }

      it('opens the other classes with what each asks for, and the ones the character does not qualify for dashed, with the reason', async () => {
        const f = await doran();
        await click(f, radio(f, 'Uma classe nova'));
        expect(text(f)).toContain(
          'Você cumpre o pré-requisito do Guerreiro: Força 13 ou Destreza 13 (Força 16, Destreza 14). Para uma classe nova, é preciso cumprir o das duas.',
        );
        expect(text(f)).toContain('Qual classe nova?');
        expect(text(f)).toContain('Duas estão ao seu alcance.');
        const mago = cards(f).find((c) => c.querySelector('.card__title')?.textContent === 'Mago')!;
        expect(mago.textContent).toContain('Exige Inteligência 13.');
        expect(mago.textContent).toContain('Você tem Inteligência 13.');
        const bardo = cards(f).find(
          (c) => c.querySelector('.card__title')?.textContent === 'Bardo',
        )!;
        expect(bardo.classList).toContain('card--closed');
        expect(bardo.textContent).toContain('Exige Carisma 13.');
        expect(bardo.textContent).toContain('Falta: Carisma 13 (você tem 9).');
        expect(bardo.querySelector('input')?.getAttribute('aria-disabled')).toBe('true');
        const monge = cards(f).find(
          (c) => c.querySelector('.card__title')?.textContent === 'Monge',
        )!;
        expect(monge.textContent).toContain('Exige Destreza 13 e Sabedoria 13.');
        expect(monge.textContent).toContain('Falta: Sabedoria 13 (você tem 11).');
        // A closed card cannot be chosen: nothing is read.
        const reads = client.options.mock.calls.length;
        await click(f, bardo.querySelector('input'));
        expect(client.options.mock.calls.length).toBe(reads);
      });

      it('waits for the class: "Próximo" says so, and opens once a class is picked', async () => {
        const f = await doran();
        await click(f, radio(f, 'Uma classe nova'));
        expect(button(f, 'Próximo').getAttribute('aria-disabled')).toBe('true');
        expect(text(f)).toContain('Escolha a classe nova para continuar.');
        await click(f, radio(f, 'Mago'));
        expect(client.options).toHaveBeenLastCalledWith('camp-1', 'ch-1', 'class:wizard');
        expect(text(f)).toContain('Doran · Guerreiro 5 → Guerreiro 5 · Mago 1');
        expect(text(f)).toContain('Subir para o nível 6');
        expect(radio(f, 'Mago').checked).toBe(true);
        expect(button(f, 'Próximo').getAttribute('aria-disabled')).not.toBe('true');
        // The steps follow the class's level 1: no Habilidades, Vida, Magias, Resumo.
        expect(
          Array.from(el(f).querySelectorAll('.step__label')).map((x) => x.textContent),
        ).toEqual(['Classe', 'Vida', 'Magias', 'Resumo']);
      });

      it('says the die of the new class in "Vida", and never the maximum', async () => {
        const f = await doran();
        await click(f, radio(f, 'Uma classe nova'));
        await click(f, radio(f, 'Mago'));
        await click(f, button(f, 'Próximo'));
        expect(text(f)).toContain('Pontos de vida de Mago 1');
        expect(text(f)).toContain(
          'O nível 1 de Mago dá um d6, e o personagem vai ao nível total 6. O dado cheio só vale no nível 1 do personagem.',
        );
        expect(text(f)).toContain('Média: 4');
      });

      it('asks in place before it adds the class: the footer becomes the question, with "Voltar" focused, and nothing is sent until it is confirmed', async () => {
        const f = await doran();
        client.levelUp.mockResolvedValue(
          character({ name: 'Doran', canLevelUp: false }, pensantus(true)),
        );
        await click(f, radio(f, 'Uma classe nova'));
        await click(f, radio(f, 'Bárbaro'));
        await click(f, button(f, 'Próximo')); // Vida
        await click(f, button(f, 'Próximo')); // Resumo
        expect(step(f)).toBe('Passo 3 de 3 · Resumo');
        await click(f, button(f, 'Confirmar o nível 6'));
        const ask = el(f).querySelector('[role="alertdialog"]')!;
        expect(ask.textContent).toContain('Subir em Bárbaro 1?');
        expect(ask.textContent).toContain(
          'Isso acrescenta uma classe nova à ficha: Bárbaro 1, e o nível total vai a 6. Não se desfaz.',
        );
        expect(Array.from(ask.querySelectorAll('button'), (b) => b.textContent?.trim())).toEqual([
          'Subir em Bárbaro 1',
          'Voltar',
        ]);
        expect(document.activeElement?.textContent?.trim()).toBe('Voltar');
        expect(client.levelUp).not.toHaveBeenCalled();
        // "Voltar" keeps everything: the footer is back, the button is the one of the step.
        await click(f, button(f, 'Voltar'));
        expect(el(f).querySelector('[role="alertdialog"]')).toBeNull();
        expect(button(f, 'Confirmar o nível 6')).toBeDefined();
        expect(client.levelUp).not.toHaveBeenCalled();
        // The question again, and this time it is answered.
        await click(f, button(f, 'Confirmar o nível 6'));
        await click(f, button(f, 'Subir em Bárbaro 1'));
        expect(client.levelUp).toHaveBeenCalledTimes(1);
        expect(client.levelUp.mock.calls[0][3].classKey).toBe('class:barbarian');
        expect(navigate).toHaveBeenCalled();
      });

      it('a class the character has is confirmed at once, with no question', async () => {
        const f = await doran();
        await click(f, button(f, 'Próximo'));
        await click(f, button(f, 'Próximo'));
        await click(f, button(f, 'Confirmar o nível 6'));
        expect(el(f).querySelector('[role="alertdialog"]')).toBeNull();
        expect(client.levelUp).toHaveBeenCalledTimes(1);
      });

      it('closes "Uma classe nova" with the reason when a class the character has does not meet its own prerequisite', async () => {
        const choices = doranChoices();
        choices[0].prerequisiteMet = false;
        choices[0].prerequisites = [
          { ability: Ability.STRENGTH, minimum: 13, have: 12, met: false },
          { ability: Ability.DEXTERITY, minimum: 13, have: 12, met: false },
        ] as (typeof choices)[0]['prerequisites'];
        for (const c of choices.slice(1)) {
          c.available = false;
          c.unavailable = LevelUpClassUnavailable.PREREQUISITE_CURRENT;
        }
        const f = await setup(
          fighterOptions({ fromLevel: 5, toLevel: 6, totalToLevel: 6, classChoices: choices }),
          fighterFive(),
          undefined,
          undefined,
          true,
        );
        expect(text(f)).toContain(
          'Você não cumpre o pré-requisito do Guerreiro: Força 13 ou Destreza 13 (Força 12, Destreza 12). Uma classe nova só entra com o pré-requisito das duas.',
        );
        const nova = radio(f, 'Uma classe nova');
        expect(nova.getAttribute('aria-disabled')).toBe('true');
        await click(f, nova);
        expect(text(f)).not.toContain('Qual classe nova?');
        // The Guerreiro still levels up.
        expect(radio(f, 'Guerreiro').checked).toBe(true);
        expect(button(f, 'Próximo').getAttribute('aria-disabled')).not.toBe('true');
      });
    });
  });

  describe('the discard question', () => {
    it('leaves at once when nothing was chosen', async () => {
      const f = await setup(undefined, undefined, undefined, undefined, true);
      await click(f, button(f, 'Cancelar'));
      expect(navigate).toHaveBeenCalledWith(['/campaigns', 'camp-1', 'characters', 'ch-1']);
    });

    it('asks in place once something was chosen, with "Continuar escolhendo" first and focused', async () => {
      const f = await setup();
      await click(f, el(f).querySelector('.row__input'));
      await click(f, button(f, 'Voltar'));
      await click(f, button(f, 'Cancelar'));
      expect(text(f)).toContain('Descartar as escolhas?');
      expect(navigate).not.toHaveBeenCalled();
      const buttons = Array.from(el(f).querySelectorAll('.ask button')).map((b) =>
        b.textContent?.trim(),
      );
      expect(buttons).toEqual(['Continuar escolhendo', 'Descartar e sair']);
      expect(document.activeElement?.textContent?.trim()).toBe('Continuar escolhendo');
      await click(f, button(f, 'Continuar escolhendo'));
      expect(text(f)).not.toContain('Descartar as escolhas?');
      await click(f, button(f, 'Cancelar'));
      await click(f, button(f, 'Descartar e sair'));
      expect(navigate).toHaveBeenCalledWith(['/campaigns', 'camp-1', 'characters', 'ch-1']);
    });

    it('asks too from "Voltar para a ficha" at the top, instead of following the link', async () => {
      const f = await setup();
      await click(f, el(f).querySelector('.row__input'));
      const back = el(f).querySelector('a.back') as HTMLAnchorElement;
      expect(back.getAttribute('href')).toBe('/campaigns/camp-1/characters/ch-1');
      const event = new MouseEvent('click', { cancelable: true, bubbles: true });
      back.dispatchEvent(event);
      f.detectChanges();
      expect(event.defaultPrevented).toBe(true);
      expect(text(f)).toContain('Descartar as escolhas?');
      expect(navigate).not.toHaveBeenCalled();
    });
  });

  describe('the hit points', () => {
    async function atVida() {
      const f = await setup();
      await click(f, el(f).querySelector('.row__input'));
      await click(f, button(f, 'Próximo'));
      return f;
    }

    it('shows the average, preselected, with what the server derives', async () => {
      const f = await atVida();
      expect(text(f)).toContain('Média: 4');
      expect(text(f)).toContain('4 + Constituição +3 · de 23 para 30');
      expect(text(f)).toContain('1d6 + Constituição +3 · o resultado fica no registro');
      expect(text(f)).toContain('Feito no passo 2');
    });

    it('rolls the die on the server and keeps the result', async () => {
      const f = await atVida();
      await click(
        f,
        Array.from(el(f).querySelectorAll('.dice-choice__card')).find((c) =>
          c.textContent?.includes('Rolar 1d6'),
        ),
      );
      expect(text(f)).toContain('Falta rolar o dado de vida.');
      await click(f, button(f, /Rolar no app/));
      expect(client.rollHitPoints).toHaveBeenCalledWith(
        'camp-1',
        'ch-1',
        'class:wizard',
        expect.any(String),
      );
      expect(text(f)).toContain('Rolado no app: 5 no d6');
      expect(button(f, 'Próximo').getAttribute('aria-disabled')).toBeNull();
      // The preview is sent with the roll.
      expect(client.preview.mock.calls.some((c) => c[2].hitPoints.value === 5)).toBe(true);
    });

    it('takes a kept roll back on the spot', async () => {
      const f = await setup(wizardOptions({ preparedMaxAfter: 3, keptHitPointRoll: 3 }));
      await click(f, el(f).querySelector('.row__input'));
      await click(f, button(f, 'Próximo'));
      await click(
        f,
        Array.from(el(f).querySelectorAll('.dice-choice__card')).find((c) =>
          c.textContent?.includes('Rolar 1d6'),
        ),
      );
      expect(text(f)).toContain('Rolado no app: 3 no d6');
      expect(client.rollHitPoints).not.toHaveBeenCalled();
    });

    it('accepts a physical die, 1 to the die, when the campaign lets the player choose', async () => {
      const f = await atVida();
      await click(
        f,
        Array.from(el(f).querySelectorAll('.dice-choice__card')).find((c) =>
          c.textContent?.includes('Rolar 1d6'),
        ),
      );
      await click(f, button(f, 'Digitar o resultado'));
      const field = el(f).querySelector('input.type__field') as HTMLInputElement;
      field.value = '9';
      field.dispatchEvent(new Event('input'));
      f.detectChanges();
      expect(text(f)).toContain('Digite um número de 1 a 6');
      field.value = '4';
      field.dispatchEvent(new Event('input'));
      f.detectChanges();
      await click(f, button(f, 'Confirmar 4'));
      expect(text(f)).toContain('Dado físico: 4 no d6');
      expect(client.rollHitPoints).not.toHaveBeenCalled();
    });

    it('follows the dice rule: no typing when everybody rolls in the app, no app roll when everybody rolls physical dice', async () => {
      const inApp = await setup(
        wizardOptions({ preparedMaxAfter: 3, diceRule: LevelUpDiceRule.FORCED_IN_APP }),
      );
      await click(inApp, el(inApp).querySelector('.row__input'));
      await click(inApp, button(inApp, 'Próximo'));
      await click(
        inApp,
        Array.from(el(inApp).querySelectorAll('.dice-choice__card')).find((c) =>
          c.textContent?.includes('Rolar 1d6'),
        ),
      );
      expect(button(inApp, /Rolar no app/)).toBeDefined();
      expect(button(inApp, 'Digitar o resultado')).toBeUndefined();
      TestBed.resetTestingModule();

      const physical = await setup(
        wizardOptions({ preparedMaxAfter: 3, diceRule: LevelUpDiceRule.FORCED_PHYSICAL }),
      );
      await click(physical, el(physical).querySelector('.row__input'));
      await click(physical, button(physical, 'Próximo'));
      await click(
        physical,
        Array.from(el(physical).querySelectorAll('.dice-choice__card')).find((c) =>
          c.textContent?.includes('Rolar 1d6'),
        ),
      );
      expect(button(physical, /Rolar no app/)).toBeUndefined();
      expect(el(physical).querySelector('input.type__field')).not.toBeNull();
    });

    describe("the table's rule for the hit points (RN-24)", () => {
      async function atVidaWith(rule: LevelUpHitPointsRule, over: object = {}) {
        const f = await setup(wizardOptions({ preparedMaxAfter: 3, hitPointsRule: rule, ...over }));
        await click(f, el(f).querySelector('.row__input'));
        await click(f, button(f, 'Próximo'));
        return f;
      }

      it('lets the player choose when the table does not decide: both cards', async () => {
        const f = await atVidaWith(LevelUpHitPointsRule.PLAYER_CHOOSES);
        expect(el(f).querySelectorAll('.dice-choice__card')).toHaveLength(2);
        expect(text(f)).not.toContain('A mesa');
      });

      it('with "rolar" shows no choice: it says so and goes straight to the die, and only the die rolls', async () => {
        const f = await atVidaWith(LevelUpHitPointsRule.ROLL_ONLY);
        expect(el(f).querySelectorAll('.dice-choice__card')).toHaveLength(0);
        expect(text(f)).toContain(
          'A mesa pede que todos rolem o dado de vida. A média não é oferecida.',
        );
        expect(text(f)).toContain('Falta rolar o dado de vida.');
        expect(button(f, /Rolar no app/)).toBeDefined();
        expect(button(f, 'Próximo').getAttribute('aria-disabled')).toBe('true');
        await click(f, button(f, /Rolar no app/));
        expect(text(f)).toContain('Rolado no app: 5 no d6');
        expect(client.preview.mock.calls.some((c) => c[2].hitPoints.value === 5)).toBe(true);
      });

      it('keeps a roll the server answers after a content change read the level again (the click was on the old reading)', async () => {
        const f = await atVidaWith(LevelUpHitPointsRule.ROLL_ONLY);
        let answer!: (v: { die: number; value: number; alreadyRolled: boolean }) => void;
        client.rollHitPoints.mockReturnValue(new Promise((resolve) => (answer = resolve)));
        await click(f, button(f, /Rolar no app/));
        // The master's change arrives while the roll is on its way: the page reads everything again.
        watcher.hint();
        await load(f);
        expect(client.options).toHaveBeenCalledTimes(2);
        answer({ die: 6, value: 4, alreadyRolled: false });
        await load(f);
        expect(text(f)).toContain('Rolado no app: 4 no d6');
        expect(text(f)).not.toContain('Falta rolar o dado de vida.');
      });

      it('with "rolar" takes back a roll the server kept, with no click', async () => {
        const f = await atVidaWith(LevelUpHitPointsRule.ROLL_ONLY, { keptHitPointRoll: 3 });
        expect(text(f)).toContain('Rolado no app: 3 no d6');
        expect(client.rollHitPoints).not.toHaveBeenCalled();
      });

      it('with "a média" shows no choice and no die: the average, with what the server derives', async () => {
        const f = await atVidaWith(LevelUpHitPointsRule.AVERAGE_ONLY);
        expect(el(f).querySelectorAll('.dice-choice__card')).toHaveLength(0);
        expect(text(f)).toContain(
          'A mesa usa a média: todos recebem o valor médio do dado de vida. O dado não é oferecido.',
        );
        expect(text(f)).toContain('Média: 4');
        expect(text(f)).toContain('4 + Constituição +3 · de 23 para 30');
        expect(button(f, /Rolar no app/)).toBeUndefined();
        expect(button(f, 'Próximo').getAttribute('aria-disabled')).toBeNull();
        expect(client.preview.mock.calls.every((c) => c[2].hitPoints.method === 1)).toBe(true);
      });

      it('with "rolar" the page counts as untouched until something else is chosen (leaving does not ask)', async () => {
        const f = await setup(
          wizardOptions({ preparedMaxAfter: 3, hitPointsRule: LevelUpHitPointsRule.ROLL_ONLY }),
          undefined,
          undefined,
          undefined,
          true,
        );
        await click(f, button(f, 'Cancelar'));
        expect(navigate).toHaveBeenCalledWith(['/campaigns', 'camp-1', 'characters', 'ch-1']);
      });
    });

    it('shows what an increase in Constituição does to the hit points', async () => {
      const f = await setup();
      const after = pensantus(true);
      // Constitution 16 → 18 with the average: 23 → 34.
      client.preview.mockResolvedValue({
        after: {
          ...after,
          hitPointsMax: 34,
          abilities: after.abilities.map((a) =>
            a.namePt === 'Constituição' ? { ...a, score: 18, modifier: 4 } : a,
          ),
        },
      });
      const con = pickRow(f, 'Constituição');
      await click(f, con.querySelector('input'));
      await click(f, button(f, 'Próximo'));
      expect(text(f)).toContain('Com Constituição 18');
      expect(text(f)).toContain('16 → 18 (+3 → +4)');
      expect(text(f)).toContain('23 → 34');
    });

    describe('after "Ler a ficha de novo"', () => {
      const reread = (f: ComponentFixture<LevelUpPage>) =>
        (f.componentInstance as unknown as { rereadSheet(): Promise<boolean> }).rereadSheet();

      async function rolledInApp() {
        const f = await atVida();
        client.rollHitPoints.mockResolvedValue({ die: 6, value: 7, alreadyRolled: false });
        await click(
          f,
          Array.from(el(f).querySelectorAll('.dice-choice__card')).find((c) =>
            c.textContent?.includes('Rolar 1d6'),
          ),
        );
        await click(f, button(f, /Rolar no app/));
        expect(text(f)).toContain('Rolado no app: 7 no d6');
        return f;
      }

      it('keeps the newest reading when two re-reads answer out of order', async () => {
        const f = await atVida();
        let older!: (o: LevelUpOptions) => void;
        let newer!: (o: LevelUpOptions) => void;
        client.options
          .mockReset()
          .mockReturnValueOnce(new Promise<LevelUpOptions>((r) => (older = r)))
          .mockReturnValueOnce(new Promise<LevelUpOptions>((r) => (newer = r)));
        client.character.mockResolvedValue(character({ revision: 6 }));
        const first = reread(f);
        const second = reread(f);
        newer(wizardOptions({ fromLevel: 4, toLevel: 5, totalFromLevel: 4, totalToLevel: 5 }));
        expect(await second).toBe(true);
        older(wizardOptions({ preparedMaxAfter: 3 }));
        expect(await first).toBe(false);
        f.detectChanges();
        expect(text(f)).toContain('Subir para o nível 5');
      });

      it('shows the reading of a re-read when it is the only one (positive control)', async () => {
        const f = await atVida();
        client.options.mockResolvedValue(
          wizardOptions({ fromLevel: 4, toLevel: 5, totalFromLevel: 4, totalToLevel: 5 }),
        );
        expect(await reread(f)).toBe(true);
        f.detectChanges();
        expect(text(f)).toContain('Subir para o nível 5');
      });

      it('drops an app roll made for one level when the options are now for the next', async () => {
        const f = await rolledInApp();
        // Another tab confirmed the level: the sheet and the options are the next level's, with no kept roll.
        client.character.mockResolvedValue(character({ revision: 6 }));
        client.options.mockResolvedValue(
          wizardOptions({
            fromLevel: 4,
            toLevel: 5,
            totalFromLevel: 4,
            totalToLevel: 5,
            keptHitPointRoll: 0,
          }),
        );
        await reread(f);
        f.detectChanges();
        expect(text(f)).not.toContain('Rolado no app: 7');
        expect(el(f).querySelector('app-roll-picker')).not.toBeNull();
      });

      it('drops an app roll when the options are now for another class', async () => {
        const f = await rolledInApp();
        client.character.mockResolvedValue(character({ revision: 6 }));
        client.options.mockResolvedValue(
          fighterOptions({ fromLevel: 4, toLevel: 5, keptHitPointRoll: 0 }),
        );
        await reread(f);
        f.detectChanges();
        expect(text(f)).not.toContain('Rolado no app: 7');
      });

      it('drops an app roll on the Vida step that stays on screen when the class is another one', async () => {
        const f = await rolledInApp();
        client.character.mockResolvedValue(character({ revision: 6 }));
        // Another class with the same steps, so the page is still on Vida; the server kept a 7 for the class that was left.
        client.options.mockResolvedValue(
          wizardOptions({
            classKey: 'class:sorcerer',
            classNamePt: 'Feiticeiro',
            keptHitPointRoll: 7,
            keptHitPointRollClassKey: 'class:wizard',
          }),
        );
        await reread(f);
        f.detectChanges();
        expect(text(f)).toContain('Passo 3 de 5 · Vida');
        expect(text(f)).not.toContain('Rolado no app: 7');
        expect(el(f).querySelector('app-roll-picker')).not.toBeNull();
      });

      it('keeps the app roll on the Vida step when the server kept it for this class (positive control)', async () => {
        const f = await rolledInApp();
        client.character.mockResolvedValue(character({ revision: 6 }));
        client.options.mockResolvedValue(
          wizardOptions({ keptHitPointRoll: 7, keptHitPointRollClassKey: 'class:wizard' }),
        );
        await reread(f);
        f.detectChanges();
        expect(text(f)).toContain('Passo 3 de 5 · Vida');
        expect(text(f)).toContain('Rolado no app: 7 no d6');
      });
    });
  });

  describe('choices left behind and new scoped choices (PM-05)', () => {
    const style = create(ChoiceSchema, {
      key: 'style',
      featureKey: 'feature:fighting-style',
      kind: ChoiceKind.OPTIONS,
      titlePt: 'Estilo de Luta',
      labelPt: 'Estilo de Luta (Guerreiro, nível 1)',
      picks: 1,
      missing: 1,
      options: [
        create(ChoiceOptionSchema, {
          key: 'defense',
          storedKey: 'stored:defense',
          namePt: 'Defesa',
        }),
        create(ChoiceOptionSchema, { key: 'duel', storedKey: 'stored:duel', namePt: 'Duelo' }),
      ],
    });
    const terrain = create(ChoiceSchema, {
      key: 'terrain',
      featureKey: 'feature:natural-explorer',
      kind: ChoiceKind.OPTIONS,
      titlePt: 'Explorador Natural',
      labelPt: 'Explorador Natural',
      picks: 1,
      missing: 1,
      options: [
        create(ChoiceOptionSchema, {
          key: 'forest',
          storedKey: 'stored:forest',
          namePt: 'Floresta',
        }),
      ],
    });
    const late = () => [
      create(ChoiceGroupSchema, { sourceNamePt: 'Estilo de Luta', level: 1, choices: [style] }),
    ];

    it('shows "Escolhas que ficaram para trás" in a step of its own and holds "Próximo" until they are made', async () => {
      const f = await setup(fighterOptions({ lateChoices: late() }));
      await click(f, button(f, 'Próximo'));
      expect(text(f)).toContain('Passo 3 de 4 · Escolhas');
      expect(text(f)).toContain('Escolhas que ficaram para trás');
      expect(text(f)).toContain('Falta escolher 1 escolha que ficou para trás.');
      expect(button(f, 'Próximo').getAttribute('aria-disabled')).toBe('true');

      await click(f, el(f).querySelector<HTMLElement>('#pick-late [role="radio"]'));
      expect(text(f)).not.toContain('Falta escolher 1 escolha que ficou para trás.');
      expect(button(f, 'Próximo').getAttribute('aria-disabled')).toBeNull();
    });

    it('sends the late picks in late_choice_keys and the new ones with the feature options, then confirms', async () => {
      const f = await setup(
        fighterOptions({
          lateChoices: late(),
          newChoices: [
            create(ChoiceGroupSchema, {
              sourceNamePt: 'Explorador Natural',
              level: 5,
              choices: [terrain],
            }),
          ],
        }),
      );
      await click(f, button(f, 'Próximo'));
      expect(text(f)).toContain('Escolhas do nível 5');
      const radios = Array.from(el(f).querySelectorAll<HTMLElement>('[role="radio"]'));
      await click(f, radios[1]);
      await click(f, el(f).querySelector<HTMLElement>('#pick-new [role="radio"]'));
      await click(f, button(f, 'Próximo'));
      await click(f, button(f, 'Confirmar o nível 5'));

      const choices = client.levelUp.mock.calls[0][3];
      expect(choices).toMatchObject({
        lateChoiceKeys: ['stored:duel'],
        featureChoiceKeys: ['stored:forest'],
      });
    });

    it('lists a blocked option dotted, off, with the reason, and never picks it', async () => {
      const f = await setup(
        fighterOptions({
          featureChoices: [
            create(LevelUpFeatureChoiceSchema, {
              feature: { key: 'feature:eldritch-invocations', namePt: 'Invocações Místicas' },
              choose: 1,
              options: [{ key: 'invocation:agonizing', namePt: 'Rajada Agonizante' }],
              blocked: [
                {
                  option: { key: 'invocation:lifedrinker', namePt: 'Sede de Vida' },
                  reasonPt: 'Exige o nível 12 de Bruxo. Você está no 5.',
                },
              ],
            }),
          ],
        }),
      );
      await click(f, button(f, 'Próximo'));
      const blocked = pickRow(f, 'Sede de Vida');
      expect(blocked.textContent).toContain('Exige o nível 12 de Bruxo. Você está no 5.');
      expect(blocked.querySelector<HTMLInputElement>('input')!.disabled).toBe(true);
    });

    it('has no such section when the level leaves nothing behind', async () => {
      const f = await setup(fighterOptions());
      expect(text(f)).not.toContain('Escolhas que ficaram para trás');
    });
  });

  describe('Magias and Resumo', () => {
    async function throughSpells() {
      const f = await setup();
      await click(f, pickRow(f, 'Inteligência').querySelector('input'));
      await click(f, button(f, 'Próximo'));
      await click(f, button(f, 'Próximo'));
      return f;
    }

    it('counts the picks, blocks "Próximo" with the reason, and unblocks it when complete', async () => {
      const f = await throughSpells();
      expect(text(f)).toContain('Passo 4 de 5 · Magias');
      expect(text(f)).toContain('0 de 1');
      expect(text(f)).toContain('0 de 2');
      expect(text(f)).toContain('Falta escolher 1 truque.');
      await click(f, pickRow(f, 'Prestidigitação').querySelector('input'));
      expect(text(f)).toContain('Faltam escolher 2 magias para o livro.');
      await click(f, pickRow(f, 'Passo Nebuloso').querySelector('input'));
      await click(f, pickRow(f, 'Reflexos').querySelector('input'));
      expect(text(f)).toContain('Faltam preparar 2 magias.');
      await click(f, el(f).querySelector('#pick-prepared .row__input'));
      expect(text(f)).toContain('Falta preparar 1 magia.');
      await click(f, el(f).querySelectorAll('#pick-prepared .row__input')[1]);
      expect(button(f, 'Próximo').getAttribute('aria-disabled')).toBeNull();
    });

    it('says how many spells are left to prepare, and drops the line once none is', async () => {
      const f = await throughSpells();
      await click(f, pickRow(f, 'Prestidigitação').querySelector('input'));
      await click(f, pickRow(f, 'Passo Nebuloso').querySelector('input'));
      await click(f, pickRow(f, 'Reflexos').querySelector('input'));
      const lead = () =>
        el(f).querySelector('#pick-prepared .list__lead--strong')?.textContent?.trim();

      expect(lead()).toBe('Prepare mais 2.');
      await click(f, el(f).querySelector('#pick-prepared .row__input'));
      expect(lead()).toBe('Prepare mais 1.');
      await click(f, el(f).querySelectorAll('#pick-prepared .row__input')[1]);
      expect(lead()).toBeUndefined();
      expect(text(f)).not.toContain('Prepare mais');
    });

    it('confirms with the choices only, never the sheet, and tells the sheet what to say', async () => {
      const f = await throughSpells();
      await click(f, pickRow(f, 'Prestidigitação').querySelector('input'));
      await click(f, pickRow(f, 'Passo Nebuloso').querySelector('input'));
      await click(f, pickRow(f, 'Reflexos').querySelector('input'));
      await click(f, el(f).querySelector('#pick-prepared .row__input'));
      await click(f, el(f).querySelectorAll('#pick-prepared .row__input')[1]);
      await click(f, button(f, 'Próximo'));
      expect(text(f)).toContain('Passo 5 de 5 · Resumo');
      expect(text(f)).toMatch(/18 para → ?20/);
      expect(text(f)).toContain('O resto da ficha não muda e continua travado.');
      await click(f, button(f, 'Confirmar o nível 4'));
      const [campaignId, characterId, revision, choices] = client.levelUp.mock.calls[0];
      expect([campaignId, characterId, revision]).toEqual(['camp-1', 'ch-1', 5]);
      expect(choices).toMatchObject({
        classKey: 'class:wizard',
        abilityIncrease: { intelligence: 2 },
        cantripKeys: ['spell:prestidigitation'],
      });
      expect(navigate).toHaveBeenCalledWith(['/campaigns', 'camp-1', 'characters', 'ch-1'], {
        replaceUrl: true,
        state: { levelUp: { name: 'Pensantus', level: 4 } },
      });
    });

    it('mentions what the master adds by the editor, quietly', async () => {
      const f = await setup(
        fighterOptions({
          masterAdds: [{ key: 'feature:favored-enemy', namePt: 'Inimigo Favorito' }],
        }),
      );
      await click(f, button(f, 'Próximo'));
      expect(text(f)).toContain('O mestre acrescenta pelo editor: Inimigo Favorito.');
    });

    it('lists the pact slots the level changes among what the level gives', async () => {
      const f = await setup(
        fighterOptions({
          pactMagicBefore: { slotLevel: 1, count: 1 },
          pactMagicAfter: { slotLevel: 1, count: 2 },
        }),
      );
      expect(text(f)).toContain('Espaços do pacto');
      expect(text(f)).toContain('1 espaço de 1º nível → 2 espaços de 1º nível');
    });

    it('words the pact slots by their count when the level of the slots changes too', async () => {
      const f = await setup(
        fighterOptions({
          pactMagicBefore: { slotLevel: 1, count: 2 },
          pactMagicAfter: { slotLevel: 2, count: 2 },
        }),
      );
      expect(text(f)).toContain('2 espaços de 1º nível → 2 espaços de 2º nível');
    });

    it('says "1 magia conhecida" for one new spell and "2 magias conhecidas" for two', async () => {
      const one = await setup(fighterOptions({ spells: 1 }));
      expect(text(one)).toContain('1 magia conhecida');
      expect(text(one)).not.toContain('1 magia conhecidas');
    });

    it('lists no pact slots among what the level gives when they stay', async () => {
      const f = await setup(
        fighterOptions({
          pactMagicBefore: { slotLevel: 1, count: 2 },
          pactMagicAfter: { slotLevel: 1, count: 2 },
        }),
      );
      expect(text(f)).not.toContain('Espaços do pacto');
    });

    async function atResumoOfToren() {
      const f = await setup(fighterOptions());
      await click(f, button(f, 'Próximo'));
      return f;
    }

    it('explains a bonus the sheet gives per level in the hit points line (the hill dwarf)', async () => {
      // The sheet's effects add one more hit point after the level than before: the Dwarven Toughness.
      const before = {
        ...pensantus(),
        hitPointsFromEffects: pensantus(true).hitPointsFromEffects - 1,
      };
      const f = await setup(fighterOptions(), character({}, before));
      await click(f, button(f, 'Próximo'));
      expect(text(f)).toContain('+ 1 de bônus por nível');
    });

    it('sends the level-up once on a double click, while the first answer is on its way', async () => {
      const f = await atResumoOfToren();
      let answer: (c: unknown) => void = () => undefined;
      client.levelUp.mockReset().mockReturnValue(new Promise((r) => (answer = r)));
      const confirm = button(f, 'Confirmar o nível 5');
      confirm.click();
      confirm.click();
      confirm.click();
      expect(client.levelUp).toHaveBeenCalledTimes(1);
      answer(character({ canLevelUp: false }, pensantus(true)));
      await settle();
    });

    it('does not send an incomplete level-up even when the button still gets the click', async () => {
      const f = await setup();
      const page = f.componentInstance as unknown as { confirm(): Promise<void> };
      await page.confirm();
      expect(client.levelUp).not.toHaveBeenCalled();
    });

    it('reads the sheet and what the level gives again after a stale revision, and keeps the picks that are still valid', async () => {
      const f = await atResumoOfToren();
      client.levelUp.mockRejectedValueOnce(new ConnectError('x', Code.Aborted));
      await click(f, button(f, 'Confirmar o nível 5'));
      // The master changed the sheet: the class's average is another now, and the revision is 6.
      client.character.mockResolvedValue(character({ revision: 6 }));
      client.options.mockResolvedValue(fighterOptions({ hitPointAverage: 7 }));
      await click(f, button(f, 'Ler a ficha de novo'));
      expect(client.options).toHaveBeenCalledTimes(2);
      expect(el(f).querySelector('.js-failure')).toBeNull();
      await click(f, button(f, 'Voltar'));
      expect(text(f)).toContain('Média: 7');
      await click(f, button(f, 'Próximo'));
      await click(f, button(f, 'Confirmar o nível 5'));
      expect(client.levelUp.mock.calls.at(-1)?.[2]).toBe(6);
    });

    it('shows a stale revision in place, and "Ler a ficha de novo" reads it again for the next try', async () => {
      const f = await atResumoOfToren();
      client.levelUp.mockRejectedValueOnce(new ConnectError('x', Code.Aborted));
      await click(f, button(f, 'Confirmar o nível 5'));
      expect(el(f).querySelector('.js-failure')?.textContent).toContain(
        'A ficha mudou enquanto você escolhia',
      );
      client.character.mockResolvedValue(character({ revision: 6 }));
      await click(f, button(f, 'Ler a ficha de novo'));
      expect(el(f).querySelector('.js-failure')).toBeNull();
      await click(f, button(f, 'Confirmar o nível 5'));
      expect(client.levelUp.mock.calls.at(-1)?.[2]).toBe(6);
    });

    it('takes the player to the sheet when a retry finds the level already applied', async () => {
      const f = await atResumoOfToren();
      // The first confirmation went through but its answer was lost: the retry is stale, and the sheet is already at level 5.
      client.levelUp.mockRejectedValueOnce(new ConnectError('x', Code.Aborted));
      client.character.mockResolvedValue(
        character({ revision: 6, name: 'Toren' }, pensantus(true, { totalLevel: 5 })),
      );
      await click(f, button(f, 'Confirmar o nível 5'));
      expect(navigate).toHaveBeenCalledWith(['/campaigns', 'camp-1', 'characters', 'ch-1'], {
        replaceUrl: true,
        state: { levelUp: { name: 'Toren', level: 5 } },
      });
      expect(el(f).querySelector('.js-failure')).toBeNull();
    });

    it('shows a refusal with its reason, and takes the player to the step that owns it', async () => {
      const f = await setup();
      await click(f, pickRow(f, 'Inteligência').querySelector('input'));
      await click(f, button(f, 'Próximo'));
      await click(f, button(f, 'Próximo'));
      await click(f, pickRow(f, 'Prestidigitação').querySelector('input'));
      await click(f, pickRow(f, 'Passo Nebuloso').querySelector('input'));
      await click(f, pickRow(f, 'Reflexos').querySelector('input'));
      await click(f, el(f).querySelector('#pick-prepared .row__input'));
      await click(f, el(f).querySelectorAll('#pick-prepared .row__input')[1]);
      await click(f, button(f, 'Próximo'));
      client.levelUp.mockRejectedValueOnce(
        new ConnectError('x', Code.FailedPrecondition, undefined, [
          {
            desc: LevelUpRefusalSchema,
            value: create(LevelUpRefusalSchema, {
              reason: LevelUpRefusalReason.CANTRIPS,
              field: 'full.cantrip_keys',
            }),
          },
        ]),
      );
      await click(f, button(f, 'Confirmar o nível 4'));
      expect(el(f).querySelector('.js-failure')?.textContent).toContain(
        'Escolha todos os truques novos do nível',
      );
      await click(f, button(f, 'Ir para Magias'));
      expect(text(f)).toContain('Passo 4 de 5 · Magias');
      expect(el(f).querySelector('.js-failure')).toBeNull();
    });
  });

  describe('when there is nothing to level up', () => {
    it('says why a character that cannot level up now cannot, with no steps (as for a locked sheet)', async () => {
      const blocked = new ConnectError('x', Code.FailedPrecondition, undefined, [
        {
          desc: CharacterBlockedSchema,
          value: create(CharacterBlockedSchema, { reason: CharacterBlockedReason.CANNOT_LEVEL_UP }),
        },
      ]);
      const f = await setup(wizardOptions(), character({ canLevelUp: false }), blocked);
      expect(text(f)).toContain('Ainda não dá para subir de nível');
      expect(text(f)).toContain('Falta o mestre marcar um marco para este personagem');
      expect(el(f).querySelector('a.blocked__back')?.textContent).toContain('Voltar para a ficha');
      expect(el(f).querySelector('app-steps-bar')).toBeNull();
    });

    it('says a character that is not there is not there', async () => {
      const f = await setup(wizardOptions(), character(), new ConnectError('x', Code.NotFound));
      expect(text(f)).toContain('Esse personagem não existe, ou você não pode vê-lo.');
    });

    it('tells the master that the player levels up, without asking for options', async () => {
      const f = await setup(wizardOptions(), character({ canAccessMasterNotes: true }));
      expect(text(f)).toContain('Ainda não dá para subir de nível');
      expect(text(f)).toContain('Quem sobe o nível é o jogador');
      expect(client.options).not.toHaveBeenCalled();
    });
  });
  describe("the table's content changed (content_changed, RN-23)", () => {
    it('reads the options and the lists again, keeps the choices that are still offered, and says so', async () => {
      const f = await setup();
      expect(watcher.following()).toBe('camp-1');
      await click(f, pickRow(f, 'Inteligência').querySelector('input'));
      await click(f, button(f, 'Próximo'));
      await click(f, button(f, 'Próximo'));
      await click(f, pickRow(f, 'Prestidigitação').querySelector('input'));
      expect(client.options).toHaveBeenCalledTimes(1);
      // A spell the wizard is offered is gone from the lists, and this one was not picked: the choices all stay.
      client.catalog.mockResolvedValue({
        spells: SPELLS.filter((sp) => sp.key !== 'spell:detect-magic'),
        skills: SKILLS,
      });
      watcher.hint();
      await load(f);
      expect(client.options).toHaveBeenCalledTimes(2);
      expect(client.catalog).toHaveBeenCalledTimes(2);
      expect(text(f)).toContain(
        'O mestre mudou as opções da mesa. As listas deste nível estão atualizadas.',
      );
      // Still on the same step, and the cantrip is still picked.
      expect(text(f)).toContain('Passo 4 de 5 · Magias');
      expect(
        (pickRow(f, 'Prestidigitação').querySelector('input') as HTMLInputElement).checked,
      ).toBe(true);
    });

    it('says a choice left the list when the master switched it off, and the row is gone', async () => {
      const f = await setup();
      await click(f, pickRow(f, 'Inteligência').querySelector('input'));
      await click(f, button(f, 'Próximo'));
      await click(f, button(f, 'Próximo'));
      await click(f, pickRow(f, 'Prestidigitação').querySelector('input'));
      client.catalog.mockResolvedValue({
        spells: SPELLS.filter((sp) => sp.key !== 'spell:prestidigitation'),
        skills: SKILLS,
      });
      watcher.hint();
      await load(f);
      expect(text(f)).toContain('uma das suas escolhas saiu da lista');
      expect(
        Array.from(el(f).querySelectorAll('.row__main, .row')).some((r) =>
          r.textContent?.includes('Prestidigitação'),
        ),
      ).toBe(false);
    });

    it('keeps the page as it was when the read fails', async () => {
      const f = await setup();
      client.options.mockRejectedValue(new Error('offline'));
      watcher.hint();
      await load(f);
      expect(text(f)).toContain('Passo 2 de 5 · Habilidades');
      expect(el(f).querySelector('.js-failure')).toBeNull();
      expect(text(f)).not.toContain('O mestre mudou');
    });

    it('reads the catalog with the character, so a sheet keeps what the master retired since', async () => {
      await setup();
      expect(client.catalog).toHaveBeenCalledWith('camp-1', 'ch-1');
    });

    it('reads the content fresh on a hint: the catalog is asked again with `fresh`', async () => {
      const f = await setup();
      client.catalog.mockClear();
      watcher.hint();
      await load(f);
      expect(client.catalog).toHaveBeenCalledWith('camp-1', 'ch-1', true);
    });

    it('says nothing when what the page shows did not change', async () => {
      const f = await setup();
      watcher.hint();
      await load(f);
      expect(text(f)).not.toContain('O mestre mudou');
    });
  });
});

describe('LevelUpPage with the real client: a content_changed hint really reads the lists again (RN-23)', () => {
  it('a spell the master switched off leaves the lists, though the client had already read the catalog', async () => {
    Element.prototype.scrollIntoView = vi.fn();
    window.scrollTo = vi.fn();
    let spells = [...SPELLS];
    const listContent = vi.fn(async () =>
      create(ListContentResponseSchema, {
        tableRevision: spells.length,
        content: create(ContentSchema, { spells, skills: SKILLS }),
      }),
    );
    const transport = createRouterTransport(({ service }) => {
      service(ContentService, { listContent });
      service(CharacterService, {
        getCharacter: async () => ({ character: character() }),
        getLevelUpOptions: async () => ({ options: wizardOptions({ preparedMaxAfter: 3 }) }),
        previewLevelUp: async () => ({ after: pensantus(true) }),
      });
      service(CampaignService, {
        getCampaign: async () => ({
          campaign: {
            id: 'camp-1',
            myDicePreference: DicePreference.APP,
            xpMode: XpMode.MILESTONES,
          },
        }),
      });
    });
    const watcher = fakeContentWatcher();
    TestBed.configureTestingModule({
      providers: [provideRouter([]), { provide: CONNECT_TRANSPORT, useValue: transport }],
    });
    TestBed.overrideComponent(LevelUpPage, {
      set: { providers: [LevelUpClient, watcher.provider] },
    });
    const { ActivatedRoute } = await import('@angular/router');
    TestBed.overrideProvider(ActivatedRoute, {
      useValue: {
        paramMap: (await import('rxjs')).of({
          get: (k: string) => (k === 'id' ? 'camp-1' : 'ch-1'),
        }),
      },
    });
    vi.spyOn(TestBed.inject(Router), 'navigate').mockResolvedValue(true);
    vi.useFakeTimers();
    const f = TestBed.createComponent(LevelUpPage);
    const go = async () => {
      f.detectChanges();
      await f.whenStable();
      await settle();
      f.detectChanges();
    };
    await go();
    const root = f.nativeElement as HTMLElement;
    const click = async (t: Element | null | undefined) => {
      (t as HTMLElement).click();
      await go();
    };
    const row = (name: string) =>
      Array.from(root.querySelectorAll<HTMLElement>('.row__main, .row')).find((r) =>
        r.textContent?.includes(name),
      );
    const next = () =>
      Array.from(root.querySelectorAll<HTMLButtonElement>('button')).find(
        (b) => b.textContent?.trim() === 'Próximo',
      )!;
    await click(next()); // past "Subir em qual classe?"
    await click(row('Inteligência')?.querySelector('input'));
    await click(next());
    await click(next());
    expect(row('Prestidigitação')).toBeTruthy();
    expect(listContent).toHaveBeenCalledTimes(1);
    // The master switches the spell off; the hint arrives.
    spells = spells.filter((sp) => sp.key !== 'spell:prestidigitation');
    watcher.hint();
    await go();
    expect(listContent).toHaveBeenCalledTimes(2);
    expect(row('Prestidigitação')).toBeUndefined();
    expect(root.textContent).toContain('O mestre mudou as opções da mesa');
  });
});

describe('newSpellsTitle', () => {
  it('agrees the participle with the count, and keeps "para o livro" for a spellbook', () => {
    expect(newSpellsTitle(1, false)).toBe('1 magia conhecida');
    expect(newSpellsTitle(2, false)).toBe('2 magias conhecidas');
    expect(newSpellsTitle(1, true)).toBe('1 magia para o livro');
    expect(newSpellsTitle(2, true)).toBe('2 magias para o livro');
    expect(newSpellsTitle(0, false)).toBe('');
  });
});
