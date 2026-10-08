import { computed, signal } from '@angular/core';

import { DicePreference } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  LevelUpDiceRule,
  LevelUpHitPointsRule,
  LevelUpSpellsKind,
  type Character,
  type LevelUpOptions,
} from '../../../gen/meurpg/characters/v1/characters_pb';
import { Ability, type DerivedSheet } from '../../../gen/meurpg/rules/v1/rules_pb';
import { formatModifier, spellLevelLabel } from '../../core/characters/character-labels';
import { isTableKey } from '../../core/content/catalog';
import { newKey } from '../../core/connect/idempotency';
import { LevelUpClient } from '../../core/levelup/levelup-client';
import { LevelUpDraft } from '../../core/levelup/levelup-draft';
import { describeLevelUpFailure } from '../../core/levelup/levelup-errors';
import { changeRows, type ChangeRow } from '../../core/levelup/levelup-summary';
import { LevelUpPreview } from './level-up-preview';

const LIST = new Intl.ListFormat('pt-BR', { type: 'conjunction' });

/** One line of "O que o nível N dá": what it is, and whether it is chosen, still to choose, or automatic. */
export interface GiveRow {
  readonly title: string;
  readonly sub: string;
  readonly tag: 'chosen' | 'choose' | 'auto';
}

/**
 * Everything the steps of one guided level-up share (MR-040): the character as the
 * server sent it (`before`), what the level gives (`options`), the player's picks
 * (`draft`), the server's answer to them (`preview`) and how the die is rolled. The page
 * makes one after it read all of that; the step components read it through one input.
 */
export class LevelUpSession {
  readonly draft: LevelUpDraft;
  readonly preview: LevelUpPreview;
  readonly before: DerivedSheet;
  readonly options: LevelUpOptions;
  /** The revision `LevelUpCharacter` is sent with; read again after a stale one. */
  readonly revision = signal(0);

  /** "Rolar no app" and "Digitar o resultado" as the campaign's dice setting allows (RN-18). */
  readonly canApp: boolean;
  readonly canType: boolean;
  readonly preferApp: boolean;

  /** What the table's rule leaves of the hit points choice (RN-24): `null` while the player chooses, else the one way. */
  readonly hpFixed: 'roll' | 'average' | null;

  readonly rolling = signal(false);
  readonly rollError = signal('');

  /** The session the page made after this one (a content change read everything again): a roll still on its way when
   * that happened lands there, not on this one, which is no longer on screen. */
  private next: LevelUpSession | null = null;

  constructor(
    readonly campaignId: string,
    readonly character: Character,
    options: LevelUpOptions,
    draft: LevelUpDraft,
    private readonly client: LevelUpClient,
    readonly preference: DicePreference,
    /** Opens the "?" of a spell. */
    readonly describeSpell: (key: string, name: string) => void,
  ) {
    this.options = options;
    this.draft = draft;
    this.before = character.derived!;
    this.revision.set(character.revision);
    this.preview = new LevelUpPreview(client, campaignId, character.id);
    const rule = options.diceRule;
    this.canApp = rule !== LevelUpDiceRule.FORCED_PHYSICAL;
    this.canType = rule !== LevelUpDiceRule.FORCED_IN_APP;
    this.preferApp = this.canApp && (!this.canType || preference !== DicePreference.PHYSICAL);
    const hp = options.hitPointsRule;
    this.hpFixed =
      hp === LevelUpHitPointsRule.ROLL_ONLY
        ? 'roll'
        : hp === LevelUpHitPointsRule.AVERAGE_ONLY
          ? 'average'
          : null;
    if (this.hpFixed === 'roll') {
      // Only the die is offered: a roll the server kept is taken back at once, as the die card does.
      this.chooseRoll();
    }
  }

  readonly after = computed<DerivedSheet>(() => this.preview.state().after ?? this.before);

  /** The die's faces, and the average the class gives, before the Constitution modifier. */
  get die(): number {
    return this.options.hitDie;
  }

  /** The Constitution modifier of the new level: the ability increase may have moved it. */
  readonly conModifier = computed(
    () => this.after().abilities.find((a) => a.ability === Ability.CONSTITUTION)?.modifier ?? 0,
  );

  /** "Média 4 + Constituição +3", or with the die that was rolled. */
  private hpSub(): string {
    const d = this.draft;
    const rolled = d.hpCard() === 'roll' ? d.rolled() : null;
    const base = rolled ? rolled.value : this.options.hitPointAverage;
    const word = rolled ? 'Rolado' : 'Média';
    return `${word} ${this.withCon(base)}`;
  }

  /** Every change of the level, before → after, from the server's derived sheets. */
  readonly rows = computed<ChangeRow[]>(() => {
    const d = this.draft;
    const name = (key: string) => d.names().get(key) ?? 'uma opção que saiu da lista';
    const prepared = d.prepared();
    return changeRows(this.before, this.after(), {
      classKey: this.options.classKey,
      spellListClassKey: d.effective().spellListClassKey,
      hpSub: this.preview.state().after ? this.hpSub() : '',
      cantrips: [...d.cantrips()].map(name),
      spells: [...d.spells()].map(name),
      prepared: [...prepared].map(name),
      spellbook: d.effective().spellsKind === LevelUpSpellsKind.SPELLBOOK,
      spellsMissing: Math.max(0, d.spellsAsked() - d.spells().size),
      table: this.fromTable(),
      // A table class's own features are what the master wrote, so the summary names them; the SRD's are on the sheet already.
      newFeatures: isTableKey(this.options.classKey) ? this.newFeatureNames() : [],
      learnsSpells: d.effective().spellsKind !== LevelUpSpellsKind.UNSPECIFIED,
    });
  });

  /** The features the level gives, by name, the ones with a choice too (the choice is its own step). */
  private newFeatureNames(): string[] {
    return this.options.newFeatures.map((f) => f.namePt);
  }

  /** The spell slots come from the table when the class is the table's, or the subclass that casts is (a third caster):
   * the one picked now, or the one the sheet has. */
  private fromTable(): boolean {
    if (isTableKey(this.options.classKey) || isTableKey(this.draft.subclassKey())) {
      return true;
    }
    const sheet = this.character.sheet?.content;
    const own =
      sheet?.case === 'full'
        ? sheet.value.classes.find((c) => c.classKey === this.options.classKey)?.subclass
        : undefined;
    return own?.case === 'subclassKey' && isTableKey(own.value);
  }

  /** "4 + Constituição +3", or "4 − 1 de Constituição": the die and what the Constituição adds, as words. */
  withCon(base: number | string): string {
    const mod = this.conModifier();
    return mod < 0
      ? `${base} − ${-mod} de Constituição`
      : `${base} + Constituição ${formatModifier(mod)}`;
  }

  /** The hit points before, and with the average, as the server derives them. */
  readonly average = computed(() => ({
    from: this.before.hitPointsMax,
    to: this.preview.state().afterAverage?.hitPointsMax ?? this.before.hitPointsMax,
  }));

  /** The Constituição case: the ability the player raised and the hit points it gives, both from the preview. */
  readonly constitution = computed(() => {
    const raised = this.draft.abilityIncrease().con;
    const avg = this.preview.state().afterAverage;
    if (!raised || !avg) {
      return null;
    }
    const was = this.before.abilities.find((a) => a.ability === Ability.CONSTITUTION);
    const now = avg.abilities.find((a) => a.ability === Ability.CONSTITUTION);
    if (!was || !now) {
      return null;
    }
    return {
      title: `Com Constituição ${now.score}`,
      score: `${was.score} → ${now.score} (${formatModifier(was.modifier)} → ${formatModifier(now.modifier)})`,
      hp: `${this.before.hitPointsMax} → ${avg.hitPointsMax}`,
    };
  });

  /** The die was rolled in the app: the server keeps the result, and asking again returns the same one. */
  async rollInApp(): Promise<void> {
    if (this.rolling()) {
      return;
    }
    this.rolling.set(true);
    this.rollError.set('');
    try {
      const res = await this.client.rollHitPoints(
        this.campaignId,
        this.character.id,
        this.options.classKey,
        newKey(),
      );
      this.current().draft.rolled.set({ kind: 'app', value: res.value });
    } catch (err) {
      this.current().rollError.set(describeLevelUpFailure(err).message);
    } finally {
      this.rolling.set(false);
    }
  }

  /** The page replaced this session with `next` (the same level, read again). */
  handOver(next: LevelUpSession): void {
    this.next = next;
    this.stop();
  }

  /** The session on screen now: this one, or the last one it was handed over to. */
  private current(): LevelUpSession {
    return this.next ? this.next.current() : this;
  }

  /** The number that came out of a physical die, typed. */
  typed(value: number): void {
    this.draft.rolled.set({ kind: 'physical', value });
  }

  /** The die card: a kept roll of the server's is taken back on the spot, as the proto says. */
  chooseRoll(): void {
    const d = this.draft;
    d.setHpCard('roll');
    const kept = this.options.keptHitPointRoll;
    const keptClass = this.options.keptHitPointRollClassKey;
    if (
      this.canApp &&
      kept > 0 &&
      (keptClass === '' || keptClass === this.options.classKey) &&
      d.rolled() === null
    ) {
      d.rolled.set({ kind: 'app', value: kept });
    }
  }

  chooseAverage(): void {
    this.draft.setHpCard('average');
  }

  /** The step number of `step` in "Passo 3": where a choice will be made. */
  stepNumber(step: 'abilities' | 'picks' | 'spells'): number {
    return this.draft.steps().indexOf(step) + 1;
  }

  /** "O que o nível N dá", in the order of the steps: chosen, still to choose, and automatic. */
  readonly gives = computed<GiveRow[]>(() => {
    const d = this.draft;
    const o = this.options;
    const t = d.totals();
    const out: GiveRow[] = [];
    const choice = (title: string, step: 'abilities' | 'picks' | 'spells', done: boolean): void => {
      const n = this.stepNumber(step);
      out.push({
        title,
        sub: done ? `Feito no passo ${n}` : `Passo ${n}`,
        tag: done ? 'chosen' : 'choose',
      });
    };
    const pending = (step: 'picks' | 'spells', ids: readonly string[]) =>
      d
        .missing()
        .some((m) => m.step === step && ids.some((id) => m.id === id || m.id.startsWith(id)));
    if (o.abilityScoreImprovement) {
      choice(
        'Incremento no Valor de Habilidade',
        'abilities',
        d.missingIn('abilities').length === 0,
      );
    }
    if (o.subclassDue) {
      choice('Subclasse', 'picks', d.subclassKey() !== '');
    }
    for (const f of t.featureChoices) {
      const i = t.featureChoices.indexOf(f);
      choice(f.feature?.namePt ?? 'Característica', 'picks', !pending('picks', [`feature-${i}`]));
    }
    if (t.skills > 0) {
      choice(
        t.skills === 1 ? 'Perícia nova' : `${t.skills} perícias novas`,
        'picks',
        !pending('picks', ['skills']),
      );
    }
    if (t.expertise > 0) {
      choice('Especialização', 'picks', !pending('picks', ['expertise']));
    }
    const bits = [
      t.cantrips > 0 ? (t.cantrips === 1 ? 'Truque novo' : `${t.cantrips} truques novos`) : '',
      t.spells > 0
        ? `${t.spells === 1 ? '1 magia' : `${t.spells} magias`} ${d.effective().spellsKind === LevelUpSpellsKind.SPELLBOOK ? 'para o livro' : 'conhecidas'}`
        : '',
    ].filter((s) => s !== '');
    if (bits.length > 0) {
      choice(bits.join(' e '), 'spells', !pending('spells', ['cantrips', 'spells']));
    }
    if (d.effective().prepares && d.preparedMaxAfter() > 0) {
      choice(
        `Magias preparadas: até ${d.preparedMaxAfter()}`,
        'spells',
        !pending('spells', ['prepared']),
      );
    }
    const auto = (title: string, change: string): void => {
      out.push({ title, sub: `Entra sozinho · ${change}`, tag: 'auto' });
    };
    const slots = (arr: readonly number[], i: number) => arr[i] ?? 0;
    for (let i = 0; i < Math.max(o.spellSlotsBefore.length, o.spellSlotsAfter.length); i++) {
      const was = slots(o.spellSlotsBefore, i);
      const now = slots(o.spellSlotsAfter, i);
      if (was !== now) auto(`Espaços de ${spellLevelLabel(i + 1)}`, `${was} → ${now}`);
    }
    const pb = (n: number) => `${formatModifier(n)}`;
    if (o.proficiencyBonusBefore !== o.proficiencyBonusAfter) {
      auto(
        'Bônus de proficiência',
        `${pb(o.proficiencyBonusBefore)} → ${pb(o.proficiencyBonusAfter)}`,
      );
    }
    const dice = (s: DerivedSheet) => s.hitDice.map((x) => `${x.count}d${x.faces}`).join(' + ');
    out.push({
      title: 'Dados de vida',
      sub: `Entra sozinho · ${dice(this.before)} → ${dice(this.after())}`,
      tag: 'auto',
    });
    const chosen = new Set(t.featureChoices.map((f) => f.feature?.key));
    const skip = new Set(o.masterAdds.map((m) => m.key));
    for (const f of o.newFeatures) {
      if (chosen.has(f.key) || skip.has(f.key) || /ability-score-improvement|subclass/.test(f.key))
        continue;
      out.push({ title: f.namePt, sub: 'Característica do nível', tag: 'auto' });
    }
    return out;
  });

  /** What the master adds in the sheet editor: the level's features the flow does not cover. */
  readonly masterAdds = computed(() => LIST.format(this.options.masterAdds.map((m) => m.namePt)));

  stop(): void {
    this.preview.stop();
  }
}
