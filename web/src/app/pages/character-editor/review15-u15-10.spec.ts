import { Injectable } from '@angular/core';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap, provideRouter } from '@angular/router';
import { of } from 'rxjs';

import { Role } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { CONNECT_TRANSPORT } from '../../core/connect/transport';
import { CharacterEditor } from './character-editor';
import { CharacterEditorSourceLive } from './character-editor-source.live';
import {
  CharacterEditorSource,
  ClassOptionVm,
  CreateCharacterInput,
  RulesCatalogVm,
} from './character-editor.types';

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
    ],
    classes: [
      cls({ key: 'class:fighter', namePt: 'Guerreiro', hitDie: 10 }),
      cls({ key: 'class:wizard', namePt: 'Mago', hitDie: 6, subclassLevel: 2 }),
      cls({ key: 'class:barbarian', namePt: 'Bárbaro', hitDie: 12 }),
    ],
    backgrounds: [{ key: 'background:acolyte', namePt: 'Acólito', equipmentPt: '', ...SRD }],
    skills: [],
    armor: [],
    weapons: [],
    spells: [],
    viewerIsMaster: false,
    toolsAndLanguages: [],
    challengeRatings: [],
  };
}

@Injectable()
class FakeSource {
  created: CreateCharacterInput[] = [];
  loadCatalog() {
    return Promise.resolve(catalog());
  }
  loadSpellDetails() {
    return Promise.reject(new Error('unused'));
  }
  loadCharacterForEdit() {
    return Promise.reject(new Error('not stubbed'));
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
  updateCharacter() {
    return Promise.resolve({ revision: 2 });
  }
}

const flush = () => new Promise<void>((resolve) => setTimeout(resolve, 0));

async function settle(fixture: ComponentFixture<CharacterEditor>) {
  fixture.detectChanges();
  await fixture.whenStable();
  fixture.detectChanges();
}

async function render() {
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
  await flush();
  await settle(fixture);
  return {
    fixture,
    fake,
    el: fixture.nativeElement as HTMLElement,
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    cmp: fixture.componentInstance as any,
  };
}

async function openStep(fixture: ComponentFixture<CharacterEditor>, label: string) {
  const tab = Array.from(
    (fixture.nativeElement as HTMLElement).querySelectorAll<HTMLElement>('[role="tab"]'),
  ).find((t) => t.textContent?.includes(label));
  tab!.click();
  await settle(fixture);
}

describe('Review15 U15-10: character editor hit point rolls and catalog', () => {
  it('A: a multiclass Fighter 1 + Wizard 3 rolls the wizard levels with the wizard d6', async () => {
    const { fixture, el, cmp } = await render();
    cmp.fullForm.patchValue({
      name: 'Misto',
      race: 'race:gnome',
      className: 'class:fighter',
      level: 1,
      background: 'background:acolyte',
      hitPointsMethod: 'rolled',
    });
    cmp.addClass();
    cmp.changeBlock(1, { classKey: 'class:wizard', level: 3 });
    await settle(fixture);
    expect(cmp.levelDice()).toEqual([6, 6, 6]);
    await openStep(fixture, 'Habilidades');
    const labels = Array.from(el.querySelectorAll('app-hit-points-rolls mat-label')).map((l) =>
      l.textContent?.trim(),
    );
    expect(labels).toEqual(['Nível 2 (1d6)', 'Nível 3 (1d6)', 'Nível 4 (1d6)']);
  });

  it('B: does not send a rolled sheet with missing (0) or out-of-die rolls', async () => {
    const { fixture, fake, cmp } = await render();
    cmp.fullForm.patchValue({
      name: 'Rolador',
      race: 'race:gnome',
      className: 'class:barbarian',
      level: 4,
      background: 'background:acolyte',
      hitPointsMethod: 'rolled',
    });
    await settle(fixture);
    // Level 2 and 3 never rolled (0), level 4 typed 15 on a d12.
    cmp.hitPointsRolls.set([0, 0, 15]);
    await cmp.submit();
    const sent = fake.created[0]?.full?.hitPointsRolls ?? [];
    // Correct: the form refuses (nothing sent), or what is sent fits the d12 and has no 0.
    expect(sent.every((r: number) => Number.isInteger(r) && r >= 1 && r <= 12)).toBe(true);
    expect(sent.length === 0 || sent.length === 3).toBe(true);
    expect(fake.created.length === 0 || sent.length === 3).toBe(true);
  });

  it('C: a failed getCampaign does not silently turn the master into a non-master', async () => {
    TestBed.configureTestingModule({
      providers: [CharacterEditorSourceLive, { provide: CONNECT_TRANSPORT, useValue: {} }],
    });
    const source = TestBed.inject(CharacterEditorSourceLive);
    (source as unknown as { contentClient: unknown }).contentClient = {
      listContent: () =>
        Promise.resolve({
          content: {
            races: [],
            subraces: [],
            classes: [],
            subclasses: [],
            backgrounds: [],
            skills: [],
            armor: [],
            weapons: [],
            spells: [],
            proficiencies: [],
            languages: [],
            challengeRatings: [],
          },
        }),
    };
    let calls = 0;
    (source as unknown as { campaignClient: unknown }).campaignClient = {
      getCampaign: () => {
        calls++;
        return calls === 1
          ? Promise.reject(new Error('transient'))
          : Promise.resolve({ campaign: { myRole: Role.MASTER } });
      },
    };
    const outcome = await source.loadCatalog('camp-1').then(
      (c) => c.viewerIsMaster,
      () => 'rejected',
    );
    // Correct: the load fails (the editor shows its retry) or the role is retried; never a silent false.
    expect(outcome === 'rejected' || outcome === true).toBe(true);
  });
});
