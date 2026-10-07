import { Injectable } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap, provideRouter } from '@angular/router';
import { of } from 'rxjs';

import { fakeContentWatcher } from '../../core/content/content-testing';
import { catalogChanged } from './catalog-changes';
import { CharacterEditor } from './character-editor';
import { CharacterEditorSource, RulesCatalogVm } from './character-editor.types';

const mk = (over: { hitDie?: number; skillChoose?: number; spellLevel?: number }): RulesCatalogVm =>
  ({
    races: [],
    classes: [
      {
        key: 'class:fighter',
        namePt: 'Guerreiro',
        hitDie: over.hitDie ?? 8,
        skillChoose: over.skillChoose ?? 2,
        savingThrows: ['str', 'con'],
        spellListClassKey: '',
        isCaster: false,
        preparation: null,
        subclasses: [],
        subclassLevel: 3,
        spellcastingFirstLevel: 0,
        maxSpellLevelByLevel: [],
        fromTable: true,
        archived: false,
        off: false,
      },
    ],
    backgrounds: [],
    skills: [],
    armor: [],
    weapons: [],
    spells: [
      {
        key: 'spell:light',
        namePt: 'Luz',
        level: over.spellLevel ?? 0,
        classKeys: ['class:fighter'],
        fromTable: true,
        archived: false,
        off: false,
      },
    ],
    toolsAndLanguages: [],
    challengeRatings: [],
    viewerIsMaster: false,
  }) as unknown as RulesCatalogVm;

describe('Review15 U15-2: catalogChanged ignores edits that leave keys/names/flags alone', () => {
  it('sees a class whose hit die changed', () => {
    expect(catalogChanged(mk({}), mk({ hitDie: 10 }))).toBe(true);
  });
  it('sees a class whose skill count changed', () => {
    expect(catalogChanged(mk({}), mk({ skillChoose: 3 }))).toBe(true);
  });
  it('sees a spell that moved between cantrip and leveled', () => {
    expect(catalogChanged(mk({}), mk({ spellLevel: 1 }))).toBe(true);
  });
});

@Injectable()
class FakeSource {
  calls = 0;
  loadCatalog() {
    this.calls++;
    return Promise.resolve(this.calls === 1 ? mk({}) : mk({ hitDie: 10 }));
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
}

const flush = () => new Promise<void>((resolve) => setTimeout(resolve, 0));

describe('Review15 U15-2: the open editor after a content_changed hint', () => {
  it('takes the class hit die the master just changed (d8 -> d10)', async () => {
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
    const fixture = TestBed.createComponent(CharacterEditor);
    fixture.detectChanges();
    await flush();
    await fixture.whenStable();
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const cmp = fixture.componentInstance as any;
    expect(cmp.state().catalog.classes[0].hitDie).toBe(8);
    watcher.hint();
    await flush();
    await fixture.whenStable();
    expect(cmp.state().catalog.classes[0].hitDie).toBe(10);
  });
});
