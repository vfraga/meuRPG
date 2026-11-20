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
import { formatModifier } from '../../core/characters/character-labels';
import { isTableKey } from '../../core/content/catalog';
import { newKey } from '../../core/connect/idempotency';
import { LevelUpClient } from '../../core/levelup/levelup-client';
import { LevelUpDraft } from '../../core/levelup/levelup-draft';
import { describeLevelUpFailure } from '../../core/levelup/levelup-errors';
import { changeRows } from '../../core/levelup/levelup-summary';
import { type ChangeRow } from '../../core/levelup/levelup-summary-rows';
import { type GiveRow, givesOf } from './level-up-gives';
import { LevelUpPreview } from './level-up-preview';

const LIST = new Intl.ListFormat('pt-BR', { type: 'conjunction' });

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

  /** The sheet has two or more classes: the level lines say which class's level they mean. */
  readonly multiclass: boolean;

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
    this.multiclass = (character.derived?.classes.length ?? 0) >= 2 || options.isNewClass;
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

  /** "o nível 4", or "o nível 2 de Clérigo" on a sheet with two classes: the level the class gains, not the total. */
  get levelWords(): string {
    const o = this.options;
    return this.multiclass ? `o nível ${o.toLevel} de ${o.classNamePt}` : `o nível ${o.toLevel}`;
  }

  /** A die was already rolled for this level in another class: the server keeps one die per level, so this class cannot roll
   * another nor use that one. Empty when the die is free; else the words, with the class and the result of that die. */
  readonly rollOtherClass = computed<string>(() => {
    const o = this.options;
    const keptClass = o.keptHitPointRollClassKey;
    if (o.keptHitPointRoll <= 0 || keptClass === '' || keptClass === o.classKey) {
      return '';
    }
    const other = this.before.classes.find((c) => c.classKey === keptClass)?.namePt ?? '';
    const faces = this.draft.catalog.classes?.find((c) => c.key === keptClass)?.hitDie ?? 0;
    const result = `${o.keptHitPointRoll}${faces > 0 ? ` no d${faces}` : ''}`;
    return other === ''
      ? `O dado deste nível já foi rolado para outra classe (${result}). Volte à outra classe para usar esse resultado, ou fique com a média.`
      : `O dado deste nível já foi rolado para o ${other} (${result}). Volte ao ${other} para usar esse resultado, ou fique com a média.`;
  });

  readonly after = computed<DerivedSheet>(() => this.preview.state().after ?? this.before);

  /** The die's faces, and the average the class gives, before the Constitution modifier. */
  get die(): number {
    return this.options.hitDie;
  }

  /** The Constitution modifier of the new level: the ability increase may have moved it. */
  readonly conModifier = computed(
    () => this.after().abilities.find((a) => a.ability === Ability.CONSTITUTION)?.modifier ?? 0,
  );

  /** "Média 4 + Constituição +3", or with the die that was rolled, and the per-level bonus of a trait when there is one. */
  private hpSub(): string {
    const d = this.draft;
    const rolled = d.hpCard() === 'roll' ? d.rolled() : null;
    const base = rolled ? rolled.value : this.options.hitPointAverage;
    const word = rolled ? 'Rolado' : 'Média';
    // A bonus the sheet gives at every level (the hill dwarf's Dwarven Toughness, +1 per level) is not in the
    // die or the Constituição: the difference of what the effects add, before and after, says it.
    const bonus = this.after().hitPointsFromEffects - this.before.hitPointsFromEffects;
    const extra = bonus > 0 ? ` + ${bonus} de bônus por nível (traço ou característica)` : '';
    return `${word} ${this.withCon(base)}${extra}`;
  }

  /** Every change of the level, before → after, from the server's derived sheets. */
  readonly rows = computed<ChangeRow[]>(() => {
    const d = this.draft;
    const name = (key: string) => d.names().get(key) ?? 'uma opção que saiu da lista';
    const prepared = d.prepared();
    const rows = changeRows(this.before, this.after(), {
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
      ...(this.options.isNewClass
        ? {
            multiclass: {
              newClassName: this.options.classNamePt,
              summary: this.preview.state().multiclass,
              newFeatures: this.options.newFeatures.map((f) => f.namePt),
            },
          }
        : {}),
    });
    return [...rows, ...d.swap.rows()];
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
    if (this.rolling() || this.rollOtherClass() !== '') {
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
    // A pick made on the old screen before it re-renders lands on the new draft too.
    this.draft.handOver(next.draft);
    this.stop();
  }

  /** The session on screen now: this one, or the last one it was handed over to. */
  private current(): LevelUpSession {
    return this.next ? this.next.current() : this;
  }

  /** The number that came out of a physical die, typed. */
  typed(value: number): void {
    this.current().draft.rolled.set({ kind: 'physical', value });
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
  readonly gives = computed<GiveRow[]>(() =>
    givesOf({ draft: this.draft, options: this.options, before: this.before, after: this.after() }),
  );

  /** What the master adds in the sheet editor: the level's features the flow does not cover. */
  readonly masterAdds = computed(() => LIST.format(this.options.masterAdds.map((m) => m.namePt)));

  stop(): void {
    this.preview.stop();
  }
}
