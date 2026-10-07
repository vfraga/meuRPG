import { ComponentFixture, TestBed } from '@angular/core/testing';
import { Router, provideRouter } from '@angular/router';
import { create } from '@bufbuild/protobuf';

import { DicePreference, XpMode } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  CharacterSchema,
  FullSheetSchema,
  type LevelUpOptions,
} from '../../../gen/meurpg/characters/v1/characters_pb';
import { fakeContentWatcher } from '../../core/content/content-testing';

import { LevelUpClient } from '../../core/levelup/levelup-client';
import {
  SKILLS,
  SPELLS,
  WIZARD_KEYS,
  fighterOptions,
  pensantus,
  wizardOptions,
} from '../../core/levelup/levelup-testing';
import { LevelUpPage } from './level-up';

const settle = () => new Promise((r) => setTimeout(r, 220));

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

describe('Review15 U15-1: a hit-point roll of another level is not carried over by "Ler a ficha de novo"', () => {
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
  let watcher = fakeContentWatcher();

  beforeEach(() => {
    // jsdom has no layout: scrolling does nothing.
    Element.prototype.scrollIntoView = vi.fn();
    window.scrollTo = vi.fn();
  });

  async function setup(
    options: LevelUpOptions = wizardOptions({ preparedMaxAfter: 3 }),
    char = character(),
    optionsError?: Error,
  ) {
    client.character.mockReset().mockResolvedValue(char);
    client.options.mockReset();
    if (optionsError) {
      client.options.mockRejectedValue(optionsError);
    } else {
      client.options.mockResolvedValue(options);
    }
    client.catalog.mockReset().mockResolvedValue({ spells: SPELLS, skills: SKILLS });
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
    vi.spyOn(TestBed.inject(Router), 'navigate').mockResolvedValue(true);
    const fixture = TestBed.createComponent(LevelUpPage);
    await load(fixture);
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
  async function atVida() {
    const f = await setup();
    await click(f, el(f).querySelector('.row__input'));
    await click(f, button(f, 'Próximo'));
    return f;
  }

  async function rollInApp(f: ComponentFixture<LevelUpPage>) {
    await click(
      f,
      Array.from(el(f).querySelectorAll('.dice-choice__card')).find((c) =>
        c.textContent?.includes('Rolar 1d6'),
      ),
    );
    await click(f, button(f, /Rolar no app/));
    expect(text(f)).toContain('Rolado no app: 7 no d6');
  }

  it('drops an app roll made for level 3->4 when the re-read options are for level 4->5', async () => {
    const f = await atVida();
    client.rollHitPoints.mockResolvedValue({ die: 6, value: 7, alreadyRolled: false });
    await rollInApp(f);
    // Another tab confirmed level 4: the sheet and the options are the next level now, with no kept roll.
    client.character.mockResolvedValue(character({ revision: 6 }));
    client.options.mockResolvedValue(
      wizardOptions({ fromLevel: 4, toLevel: 5, totalFromLevel: 4, totalToLevel: 5, keptHitPointRoll: 0 }),
    );
    await (f.componentInstance as unknown as { rereadSheet(): Promise<boolean> }).rereadSheet();
    f.detectChanges();
    expect(text(f)).not.toContain('Rolado no app: 7');
    expect(el(f).querySelector('app-roll-picker')).not.toBeNull();
  });

  it('drops an app roll when the re-read options are for another class', async () => {
    const f = await atVida();
    client.rollHitPoints.mockResolvedValue({ die: 6, value: 7, alreadyRolled: false });
    await rollInApp(f);
    client.character.mockResolvedValue(character({ revision: 6 }));
    client.options.mockResolvedValue(
      fighterOptions({ fromLevel: 4, toLevel: 5, keptHitPointRoll: 0 }),
    );
    await (f.componentInstance as unknown as { rereadSheet(): Promise<boolean> }).rereadSheet();
    f.detectChanges();
    expect(text(f)).not.toContain('Rolado no app: 7');
  });
});
