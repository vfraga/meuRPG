import { computed, signal } from '@angular/core';

import {
  type Choice,
  LevelUpHitPointsRule,
  LevelUpSpellsKind,
  type LevelUpOptions,
} from '../../../gen/meurpg/characters/v1/characters_pb';
import type { Ability } from '../../../gen/meurpg/rules/v1/rules_pb';
import type { AbilityKey } from '../characters/characters.types';
import { type ChoiceDrafts, keepDraftsOf, withSelection, withText } from './levelup-choices';
import { LevelUpAbilities } from './levelup-abilities';
import {
  type AsiMode,
  type HpCard,
  type LevelUpCatalog,
  type PickSet,
  type Rolled,
} from './levelup-draft-types';
import type { Missing, SheetKeys, StepKey } from './levelup-flow';
import { LevelUpLists } from './levelup-lists';
import { missingChoices } from './levelup-missing';
import {
  adoptedHitPoints,
  anyPicked,
  featureOptionKeys,
  flip,
  keepOnly,
  nextFeatures,
  offeredAmong,
  subclassPrepares,
  trim,
} from './levelup-picks';
import { choicesRequest } from './levelup-request';
import { LevelUpSwap } from './levelup-swap';
import type { LevelUpChoicesInit } from './levelup-client';

/**
 * Everything the player has chosen so far in one guided level-up (MR-040):
 * the ability increase, the hit points, the subclass and the new cantrips,
 * spells, feature options, skills and expertise, with the steps they make,
 * what is still missing and the `LevelUpChoices` to send. Nothing is saved
 * until "Confirmar o nível N" (the server checks every choice with the rules
 * engine); this only counts the picks against the counts the server gave.
 *
 * The picks and the ways to change them live here; what they are read against is built by `LevelUpLists` (the lists to
 * pick from and their counts) and `LevelUpAbilities` (the increase or the feat), whose members this class exposes as its own.
 */
export class LevelUpDraft {
  readonly abilityMode = signal<'one' | 'two'>('one');
  readonly abilityKeys = signal<readonly AbilityKey[]>([]);
  /** "Aumentar habilidades" or "Um talento": the second is offered only when the server lists feats. */
  readonly asiMode = signal<AsiMode>('increase');
  readonly featKey = signal('');
  /** "Trocar uma escolha": one earlier pick for another, where the table allows it. */
  readonly swap: LevelUpSwap;
  /** The abilities the picked feat raises (`increase.count` of `increase.from`). */
  readonly featAbilities = signal<readonly Ability[]>([]);
  /** The card the hit points start on: a table that makes everybody roll starts on the die (RN-24), the rest on the average. */
  private readonly startCard: HpCard;
  readonly hpCard = signal<HpCard>('average');
  readonly rolled = signal<Rolled | null>(null);
  readonly subclassKey = signal('');
  readonly cantrips: PickSet = signal<ReadonlySet<string>>(new Set());
  readonly spells: PickSet = signal<ReadonlySet<string>>(new Set());
  readonly prepared: PickSet = signal<ReadonlySet<string>>(new Set());
  /** The options of every feature choice, together: each option key is unique. */
  readonly features: PickSet = signal<ReadonlySet<string>>(new Set());
  readonly skills: PickSet = signal<ReadonlySet<string>>(new Set());
  readonly expertise: PickSet = signal<ReadonlySet<string>>(new Set());
  /** The picks on the choices an earlier level left open (`LevelUpOptions.late_choices`), by choice key (PM-05). */
  readonly lateDrafts = signal<ChoiceDrafts>(new Map());
  /** The picks on the new level's choices that are not options of a feature (`LevelUpOptions.new_choices`). */
  readonly newDrafts = signal<ChoiceDrafts>(new Map());
  /** The musical instrument a Bard taken as a later class picks (a proficiency key), or ''. */
  readonly instrument = signal('');
  private readonly preparedMax = signal(0);
  /** The new maximum of prepared spells, as `PreviewLevelUp` derives it (an ability increase moves it).
   * Setting it drops the prepared picks beyond the new maximum. */
  readonly preparedMaxAfter = Object.assign(() => this.preparedMax(), {
    set: (max: number): void => {
      this.preparedMax.set(max);
      this.reconcile();
    },
  });

  // What the picks are read against (see `LevelUpLists` and `LevelUpAbilities`).
  readonly effective: LevelUpLists['effective'];
  readonly listName: LevelUpLists['listName'];
  readonly totals: LevelUpLists['totals'];
  readonly more: LevelUpLists['more'];
  readonly steps: LevelUpLists['steps'];
  readonly cantripItems: LevelUpLists['cantripItems'];
  readonly spellItems: LevelUpLists['spellItems'];
  readonly preparedItems: LevelUpLists['preparedItems'];
  readonly skillItems: LevelUpLists['skillItems'];
  readonly instrumentItems: LevelUpLists['instrumentItems'];
  readonly instrumentAsked: LevelUpLists['instrumentAsked'];
  readonly expertiseItems: LevelUpLists['expertiseItems'];
  readonly names: LevelUpLists['names'];
  readonly cantripsAsked: LevelUpLists['cantripsAsked'];
  readonly spellsAsked: LevelUpLists['spellsAsked'];
  readonly preparedAsked: LevelUpLists['preparedAsked'];
  readonly skillsAsked: LevelUpLists['skillsAsked'];
  readonly expertiseAsked: LevelUpLists['expertiseAsked'];
  readonly outsidePicked: LevelUpLists['outsidePicked'];
  readonly outsideMax: LevelUpLists['outsideMax'];
  readonly abilityAsked: LevelUpAbilities['abilityAsked'];
  readonly feats: LevelUpAbilities['feats'];
  readonly hasFeats: LevelUpAbilities['hasFeats'];
  readonly taking: LevelUpAbilities['taking'];
  readonly feat: LevelUpAbilities['feat'];
  readonly featFixed: LevelUpAbilities['featFixed'];
  readonly featCapped: LevelUpAbilities['featCapped'];
  readonly featAsked: LevelUpAbilities['featAsked'];
  readonly featSummary: LevelUpAbilities['featSummary'];
  readonly pickedKeys: LevelUpAbilities['pickedKeys'];
  readonly abilityIncrease: LevelUpAbilities['abilityIncrease'];
  private readonly abilities: LevelUpAbilities;

  constructor(
    readonly options: LevelUpOptions,
    readonly have: SheetKeys,
    readonly catalog: LevelUpCatalog,
  ) {
    this.swap = new LevelUpSwap(options);
    this.preparedMax.set(options.preparedMaxAfter);
    this.startCard = options.hitPointsRule === LevelUpHitPointsRule.ROLL_ONLY ? 'roll' : 'average';
    this.hpCard.set(this.startCard);
    const lists = new LevelUpLists(options, have, catalog, {
      subclassKey: this.subclassKey,
      spells: this.spells,
      skills: this.skills,
      preparedMax: this.preparedMax,
    });
    this.effective = lists.effective;
    this.listName = lists.listName;
    this.totals = lists.totals;
    this.more = lists.more;
    this.steps = lists.steps;
    this.cantripItems = lists.cantripItems;
    this.spellItems = lists.spellItems;
    this.preparedItems = lists.preparedItems;
    this.skillItems = lists.skillItems;
    this.instrumentItems = lists.instrumentItems;
    this.instrumentAsked = lists.instrumentAsked;
    this.expertiseItems = lists.expertiseItems;
    this.names = lists.names;
    this.cantripsAsked = lists.cantripsAsked;
    this.spellsAsked = lists.spellsAsked;
    this.preparedAsked = lists.preparedAsked;
    this.skillsAsked = lists.skillsAsked;
    this.expertiseAsked = lists.expertiseAsked;
    this.outsidePicked = lists.outsidePicked;
    this.outsideMax = lists.outsideMax;
    const abilities = new LevelUpAbilities(options, this);
    this.abilities = abilities;
    this.abilityAsked = abilities.abilityAsked;
    this.feats = abilities.feats;
    this.hasFeats = abilities.hasFeats;
    this.taking = abilities.taking;
    this.feat = abilities.feat;
    this.featFixed = abilities.featFixed;
    this.featCapped = abilities.featCapped;
    this.featAsked = abilities.featAsked;
    this.featSummary = abilities.featSummary;
    this.pickedKeys = abilities.pickedKeys;
    this.abilityIncrease = abilities.abilityIncrease;
  }

  /** Every choice still to make, in step order, each with the picker that holds it. */
  readonly missing = computed<Missing[]>(() => missingChoices(this));

  /** What is missing in `step`, if anything: the step's "Próximo" waits for it. */
  missingIn(step: StepKey): readonly Missing[] {
    return this.missing().filter((m) => m.step === step);
  }

  /** Whether anything was chosen beyond the defaults: leaving then asks first. */
  readonly dirty = computed(() => anyPicked(this, this.startCard) || this.swap.started());

  private build(hp: Rolled | null): LevelUpChoicesInit {
    return choicesRequest(
      {
        options: this.options,
        abilityIncrease: this.abilityIncrease(),
        featKey: this.taking() ? this.featKey() : '',
        subclassKey: this.subclassKey(),
        cantrips: this.cantrips(),
        spells: this.spells(),
        prepared: this.prepared(),
        features: this.features(),
        skills: this.skills(),
        expertise: this.expertise(),
        instrument: this.instrument(),
        ...this.swap.request(),
        lateDrafts: this.lateDrafts(),
        newDrafts: this.newDrafts(),
      },
      hp,
    );
  }

  /** The choices to send. Incomplete is fine for a preview: the answer says what is missing. */
  readonly choices = computed<LevelUpChoicesInit>(() =>
    this.build(this.hpCard() === 'roll' ? this.rolled() : null),
  );

  /** The same choices with the average hit points: "Média: 4" shows its own gain whichever card is open. */
  readonly averageChoices = computed<LevelUpChoicesInit>(() => this.build(null));

  setAbilityMode(mode: 'one' | 'two'): void {
    if (this.forwarded((d) => d.setAbilityMode(mode))) {
      return;
    }
    if (mode !== this.abilityMode()) {
      this.abilityMode.set(mode);
      this.abilityKeys.set([]);
    }
  }

  /** One ability for +2, two for +1 each: a third replaces the oldest. */
  toggleAbility(key: AbilityKey): void {
    if (this.forwarded((d) => d.toggleAbility(key))) {
      return;
    }
    this.abilityKeys.set(this.abilities.nextAbilityKeys(key));
  }

  /** A pick on a choice an earlier level left open: `optionKeys` is the whole selection of the choice now. */
  selectLate(choice: Choice, optionKeys: readonly string[]): void {
    if (this.forwarded((d) => d.selectLate(choice, optionKeys))) {
      return;
    }
    this.lateDrafts.update((d) => withSelection(d, choice, optionKeys));
  }

  editLateText(choice: Choice, n: number, text: string): void {
    if (this.forwarded((d) => d.editLateText(choice, n, text))) {
      return;
    }
    this.lateDrafts.update((d) => withText(d, choice, n, text));
  }

  /** A pick on one of the new level's own choices. */
  selectNew(choice: Choice, optionKeys: readonly string[]): void {
    if (this.forwarded((d) => d.selectNew(choice, optionKeys))) {
      return;
    }
    this.newDrafts.update((d) => withSelection(d, choice, optionKeys));
  }

  editNewText(choice: Choice, n: number, text: string): void {
    if (this.forwarded((d) => d.editNewText(choice, n, text))) {
      return;
    }
    this.newDrafts.update((d) => withText(d, choice, n, text));
  }

  setAsiMode(mode: AsiMode): void {
    if (this.forwarded((d) => d.setAsiMode(mode))) {
      return;
    }
    if (mode !== this.asiMode() && (mode === 'increase' || this.hasFeats())) {
      this.asiMode.set(mode);
    }
  }

  /** Picks a feat the character qualifies for; a feat that raises every ability it lists needs no further choice. */
  setFeat(key: string): void {
    if (this.forwarded((d) => d.setFeat(key))) {
      return;
    }
    const feat = this.feats().find((f) => f.key === key);
    if (!feat?.qualifies || key === this.featKey()) {
      return;
    }
    this.featKey.set(key);
    this.featAbilities.set([]);
  }

  /** An ability of the feat's increase: picked or dropped, never more than `count`. */
  toggleFeatAbility(ability: Ability): void {
    if (this.forwarded((d) => d.toggleFeatAbility(ability))) {
      return;
    }
    const next = this.abilities.nextFeatAbilities(ability);
    if (next) {
      this.featAbilities.set(next);
    }
  }

  setHpCard(card: HpCard): void {
    if (this.forwarded((d) => d.setHpCard(card))) {
      return;
    }
    this.hpCard.set(card);
  }

  setSubclass(key: string): void {
    if (this.forwarded((d) => d.setSubclass(key))) {
      return;
    }
    if (key === this.subclassKey()) {
      return;
    }
    const before = this.subclassKey();
    const gone = featureOptionKeys(this.options, before);
    this.subclassKey.set(key);
    // A subclass that prepares (a third caster) says its own maximum until the preview gives the exact one.
    // One that does not leaves the maximum as it is: the pending ability increase may have moved it.
    if (subclassPrepares(this.options, before) || subclassPrepares(this.options, key)) {
      this.preparedMax.set(this.effective().preparedMaxAfter);
    }
    // Only what belonged to the previous subclass goes: its feature options. The cantrips, skills and
    // expertise that the level itself asks for stay, trimmed to what the new subclass's counts allow.
    this.features.set(new Set([...this.features()].filter((k) => !gone.has(k))));
    // The spells the subclass brought (a third caster's list) go with it: keep only what the lists now offer.
    this.cantrips.set(offeredAmong(this.cantrips(), this.cantripItems()));
    this.spells.set(offeredAmong(this.spells(), this.spellItems()));
    this.prepared.set(offeredAmong(this.prepared(), this.preparedItems()));
    this.reconcile();
  }

  /**
   * The draft that replaced this one when the page read the sheet again (`LevelUpSession.handOver`).
   * A pick made on the old screen in the moment before it re-renders goes there instead of being lost
   * on this one (the radio then stayed drawn as chosen while the new draft had nothing).
   */
  private next: LevelUpDraft | null = null;

  /** This draft was replaced by `next`: from now on its setters change `next` (or the one after it). */
  handOver(next: LevelUpDraft): void {
    this.next = next;
  }

  /** Runs `change` on the newest draft along the hand-overs when this one was replaced; true when it did. */
  private forwarded(change: (d: LevelUpDraft) => void): boolean {
    if (!this.next) {
      return false;
    }
    let d: LevelUpDraft = this.next;
    while (d.next) {
      d = d.next;
    }
    change(d);
    return true;
  }

  /** Takes over what `other` had picked (after the sheet was read again), keeping only what these
   * options and lists still allow, then trimmed to the counts. */
  adopt(other: LevelUpDraft): void {
    if (this.options.abilityScoreImprovement) {
      this.abilityMode.set(other.abilityMode());
      this.abilityKeys.set(other.abilityKeys().slice(0, this.abilityAsked()));
    }
    if (other.taking() && this.feats().some((f) => f.key === other.featKey() && f.qualifies)) {
      this.asiMode.set('feat');
      this.setFeat(other.featKey());
      this.featAbilities.set(this.abilities.adoptedFeatAbilities(other.featAbilities()));
    }
    this.swap.adopt(other.swap);
    this.adoptHitPoints(other);
    const sameLevel =
      this.options.classKey === other.options.classKey &&
      this.options.toLevel === other.options.toLevel;
    if (sameLevel) {
      this.preparedMax.set(other.preparedMaxAfter());
    }
    if (this.options.subclasses.some((c) => c.key === other.subclassKey())) {
      this.subclassKey.set(other.subclassKey());
    }
    this.cantrips.set(offeredAmong(other.cantrips(), this.cantripItems()));
    this.spells.set(offeredAmong(other.spells(), this.spellItems()));
    this.prepared.set(offeredAmong(other.prepared(), this.preparedItems()));
    const options = new Set(
      this.totals().featureChoices.flatMap((f) => f.options.map((o) => o.key)),
    );
    this.features.set(new Set([...other.features()].filter((k) => options.has(k))));
    this.lateDrafts.set(keepDraftsOf(this.options.lateChoices, other.lateDrafts()));
    this.newDrafts.set(keepDraftsOf(this.options.newChoices, other.newDrafts()));
    this.skills.set(
      offeredAmong(
        other.skills(),
        this.skillItems().filter((i) => !i.disabled),
      ),
    );
    if (this.instrumentItems().some((i) => i.key === other.instrument() && !i.disabled)) {
      this.instrument.set(other.instrument());
    }
    this.expertise.set(offeredAmong(other.expertise(), this.expertiseItems()));
    this.reconcile();
  }

  /** The hit points card and die result of `other`, as far as these options still allow them. */
  private adoptHitPoints(other: LevelUpDraft): void {
    const kept = adoptedHitPoints(this.options, other.options, other.hpCard(), other.rolled());
    this.hpCard.set(kept.card);
    this.rolled.set(kept.rolled);
  }

  /** Brings every pick set back in step with the counts: the picks beyond what a count asks go (the first
   * ones stay), the prepared spells stay among the offered ones, and expertise stays on trained skills only.
   * Run after anything that moves a count or a list. */
  private reconcile(): void {
    trim(this.cantrips, this.cantripsAsked());
    trim(this.spells, this.spellsAsked());
    const offered = new Set(this.preparedItems().map((i) => i.key));
    keepOnly(this.prepared, (k) => offered.has(k));
    trim(this.prepared, this.preparedAsked());
    trim(this.skills, this.skillsAsked());
    this.dropUntrainedExpertise();
    trim(this.expertise, this.expertiseAsked());
  }

  /** Expertise only in what is trained. */
  private dropUntrainedExpertise(): void {
    const trained = new Set([...this.have.skills, ...this.skills()]);
    keepOnly(this.expertise, (k) => trained.has(k));
  }

  toggleCantrip(key: string): void {
    if (this.forwarded((d) => d.toggleCantrip(key))) {
      return;
    }
    flip(this.cantrips, key, this.cantripsAsked());
  }

  toggleSpell(key: string): void {
    if (this.forwarded((d) => d.toggleSpell(key))) {
      return;
    }
    // Magical Secrets: only so many of the new spells may come from outside the class list.
    const item = this.spellItems().find((i) => i.key === key);
    if (item?.disabled && !this.spells().has(key)) {
      return;
    }
    flip(this.spells, key, this.spellsAsked());
    // A spell dropped from the book cannot stay prepared.
    const book = new Set([...this.have.known, ...this.spells()]);
    if (this.effective().spellsKind === LevelUpSpellsKind.SPELLBOOK) {
      this.prepared.set(new Set([...this.prepared()].filter((k) => book.has(k))));
    }
  }

  togglePrepared(key: string): void {
    if (this.forwarded((d) => d.togglePrepared(key))) {
      return;
    }
    flip(this.prepared, key, this.preparedAsked());
  }

  toggleSkill(key: string): void {
    if (this.forwarded((d) => d.toggleSkill(key))) {
      return;
    }
    if (this.skillItems().find((i) => i.key === key)?.disabled && !this.skills().has(key)) {
      return;
    }
    flip(this.skills, key, this.skillsAsked());
    this.dropUntrainedExpertise();
  }

  /** The instrument is one pick: tapping the picked one drops it. */
  toggleInstrument(key: string): void {
    if (this.forwarded((d) => d.toggleInstrument(key))) {
      return;
    }
    const item = this.instrumentItems().find((i) => i.key === key);
    if (item?.disabled) {
      return;
    }
    this.instrument.set(this.instrument() === key ? '' : key);
  }

  toggleExpertise(key: string): void {
    if (this.forwarded((d) => d.toggleExpertise(key))) {
      return;
    }
    flip(this.expertise, key, this.expertiseAsked());
  }

  /** A feature option: each choice (a fighting style, the metamagic) takes its own `choose`. */
  toggleFeature(key: string, group: readonly string[], choose: number): void {
    if (this.forwarded((d) => d.toggleFeature(key, group, choose))) {
      return;
    }
    this.features.set(nextFeatures(this.features(), key, group, choose));
  }
}
