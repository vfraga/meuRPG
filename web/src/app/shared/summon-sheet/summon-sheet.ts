import { create } from '@bufbuild/protobuf';
import {
  Component,
  ElementRef,
  computed,
  effect,
  inject,
  signal,
  untracked,
  viewChild,
} from '@angular/core';
import { FormsModule } from '@angular/forms';
import { MatButtonModule } from '@angular/material/button';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatIconModule } from '@angular/material/icon';
import { MatInputModule } from '@angular/material/input';

import { DiceMode, DicePreference } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  type GetSummonOptionsResponse,
  GetSummonOptionsResponseSchema,
  type SummonOption,
  SummonSpellOptions,
} from '../../../gen/meurpg/characters/v1/characters_pb';
import type { Combatant } from '../../../gen/meurpg/play/v1/combat_pb';
import type { SlotRow } from '../../core/combat/cast-flow';
import { CombatClient } from '../../core/combat/combat-client';
import { combatErrorMessage } from '../../core/combat/combat-errors';
import type { CombatState } from '../../core/combat/combat-state';
import { parseSum } from '../../core/combat/combat-dice';
import { stateWord } from '../../core/combat/combat-view';
import { summonResult } from '../../core/combat/summon-result';
import { CreatureArt } from '../creatures/creature-art';
import { newKey } from '../../core/connect/idempotency';
import { CreaturesClient } from '../../core/creatures/creatures-client';
import { creatureErrorMessage } from '../../core/creatures/creature-errors';
import { CREATURE_NAME_MAX, nameCounter } from '../../core/creatures/creature-format';
import { FIND_FAMILIAR, castVerb } from '../../core/creatures/summon-labels';
import { SlotPicker } from '../slot-picker/slot-picker';
import { SheetFrame } from '../sheet/sheet-frame/sheet-frame';
import { injectSheet } from '../sheet/sheet-host';
import { type ChoiceRow, CreatureChoiceList } from '../creatures/creature-choice-list';
import { SummonLists } from './summon-lists';
import {
  concentrationLineOf,
  costLineOf,
  creatureKeysOf,
  diceFlagsOf,
  listLabelOf,
  missingOf,
  optionLabel,
  replacesNoteOf,
  resultLineOf,
  resultTitleOf,
  rowsOf,
  setupLineOf,
  slotId,
  slotRowsOf,
  subtitleOf,
} from './summon-views';

/** The sheet opened in a combat (Conjurar Animais, MR-037, E9-12): the cast goes through the combat. */
export interface SummonCombat {
  readonly encounterId: string;
  /** The caster's combatant (a UUID). */
  readonly casterId: string;
  readonly diceMode: DiceMode;
  readonly preference: DicePreference;
  /** The page's copy of the combat: the answer's combat goes there. */
  readonly state: CombatState;
  /** The spell the caster concentrates on now ("Teia"); empty when none: casting this ends it. */
  readonly concentrating: string;
}

/** What the panel (or the combat) hands the sheet. */
export interface SummonSheetData {
  readonly campaignId: string;
  readonly characterId: string;
  readonly spellKey: string;
  /** Set in a combat: the group's initiative roll and the result stage. */
  readonly combat?: SummonCombat;
}

/** What the sheet answers when it cast: for the panel's live notice. */
export interface SummonSheetResult {
  readonly spellName: string;
  readonly ritual: boolean;
  readonly castingTime: string;
  readonly names: readonly string[];
  readonly count: number;
  readonly dismissed: number;
}

/**
 * Casting a summoning spell outside a combat (E9-10, quadro 2). The sheet reads what the character
 * can do from the server (`GetSummonOptions`: the slots, what each circle may bring, what a casting
 * would send away) and keeps no rule of its own: a ritual when the server says the character can
 * cast it as one, otherwise the slot picker; the options and the creatures of the circle; a mix
 * of kinds when the option counts several creatures ("−" 1 "+" for each kind, the total must equal the
 * count). The same frame as the combat's sheets: the title, the name and search fields and the
 * footer stay put, and only the list scrolls.
 *
 * Before the cast it says what the casting would send away ("Isso encerra Conjurar Animais e dispensa 8
 * criaturas"); a refusal by the server stays here, in words, with the sheet open. The key of the cast is
 * made anew whenever a choice changes, so a tap repeated after a lost answer never casts twice.
 */
@Component({
  selector: 'app-summon-sheet',
  imports: [
    CreatureArt,
    CreatureChoiceList,
    FormsModule,
    MatButtonModule,
    MatFormFieldModule,
    MatIconModule,
    MatInputModule,
    SheetFrame,
    SlotPicker,
  ],
  templateUrl: './summon-sheet.html',
  styleUrl: './summon-sheet.scss',
})
export class SummonSheet {
  private readonly client = inject(CreaturesClient);
  private readonly combatApi = inject(CombatClient);
  private readonly sheet = injectSheet<SummonSheetData, SummonSheetResult>();
  protected readonly data = this.sheet.data;
  /** Set when the spell is cast in a combat. */
  protected readonly combat = this.data.combat;
  /** The result stage of a casting in a combat: the sentence and the creatures that joined. */
  protected readonly result = signal<{
    readonly text: string;
    readonly creatures: readonly Combatant[];
    readonly title: string;
  } | null>(null);
  /** The group's initiative d20 typed from a physical die (a combat only). */
  private readonly dice = diceFlagsOf(this.combat);
  protected readonly canApp = this.dice.canApp;
  protected readonly canType = this.dice.canType;
  /** The group's initiative d20 comes from a physical die: the campaign allows no other (RN-18), or the player prefers it where both are allowed. */
  protected readonly typing = signal(this.dice.typing);
  protected readonly typed = signal('');
  /** The face field shows, and the cast needs and sends the face. */
  protected readonly needsFace = computed(() => !!this.combat && (this.typing() || !this.canApp));
  private readonly back = viewChild('back', { read: ElementRef<HTMLButtonElement> });
  protected readonly inSheet = this.sheet.inSheet;
  protected readonly max = CREATURE_NAME_MAX;
  protected readonly nameCounter = nameCounter;

  protected readonly options = signal<GetSummonOptionsResponse | null>(null);
  protected readonly name = signal('');
  protected readonly slotKey = signal('');
  protected readonly option = signal(0);
  /** How many of each creature kind: one kind with 1 for a single creature. */
  protected readonly counts = signal<Readonly<Record<string, number>>>({});
  /** "Mudar": the slot and the quantity open again after a creature was chosen. */
  protected readonly editing = signal(false);
  protected readonly busy = signal(false);
  /** A request in the air: Esc and the backdrop do not close the sheet under it. */
  protected readonly lockWhileBusy = effect(() => this.sheet.lock(this.busy()));
  protected readonly error = signal('');
  private readonly frame = viewChild.required(SheetFrame);
  private key = newKey();
  private readonly lists = new SummonLists(this.client, this.data.campaignId, (message) =>
    this.error.set(message),
  );
  /** The book's armor class of each creature of the result (the combat sends none to a player). */
  protected readonly resultAc = this.lists.armorClasses;
  protected readonly forms = this.lists.forms;
  protected readonly beasts = this.lists.beasts;
  protected readonly beastBlocks = this.lists.beastBlocks;

  protected readonly spell = computed<SummonSpellOptions | null>(
    () => this.options()?.spells.find((s) => s.spellKey === this.data.spellKey) ?? null,
  );
  /** A ritual spends no slot: the server says whether the character can cast this one as one. */
  protected readonly ritual = computed(() => !this.combat && (this.spell()?.canRitual ?? false));
  protected readonly familiar = computed(() => this.data.spellKey === FIND_FAMILIAR);

  protected readonly subtitle = computed(() =>
    subtitleOf({
      spell: this.spell(),
      done: this.result() !== null,
      inCombat: !!this.combat,
      ritual: this.ritual(),
      slot: this.slot(),
    }),
  );
  protected readonly title = computed(
    () => this.result()?.title ?? this.spell()?.namePt ?? this.data.spellKey,
  );
  /** The d20 typed for the group's initiative: a number from 1 to 20, or `null`. */
  protected readonly face = computed(() => parseSum(this.typed(), 1, 20));
  /** In a combat: what losing the concentration does, and what casting it ends now, in the footer where it is always in view. */
  protected readonly concentrationLine = computed(() => {
    const s = this.spell();
    if (!this.combat || !s?.concentration) {
      return null;
    }
    const kinds = this.rows().filter((r) => (this.counts()[r.key] ?? 0) > 0);
    return concentrationLineOf({
      spell: s,
      count: this.count(),
      kindTitles: kinds.map((r) => r.title),
      concentrating: this.combat.concentrating,
    });
  });

  /** The slots this spell can use: from its circle up, only the circles the server lists for it. */
  protected readonly slotRows = computed<readonly SlotRow[]>(() =>
    slotRowsOf(this.spell(), this.options()?.slots ?? []),
  );
  protected readonly slot = computed(
    () => this.slotRows().find((r) => slotId(r) === this.slotKey()) ?? null,
  );
  /** The circle the cast is made at: the slot's, or the spell's own for a ritual. */
  protected readonly circle = computed(() =>
    this.ritual() ? (this.spell()?.level ?? 0) : (this.slot()?.level ?? 0),
  );
  protected readonly circleOptions = computed<readonly SummonOption[]>(
    () => this.spell()?.circles.find((c) => c.circle === this.circle())?.options ?? [],
  );
  protected readonly opt = computed<SummonOption | null>(
    () => this.circleOptions()[this.option()] ?? null,
  );
  protected readonly count = computed(() => this.opt()?.count ?? 0);
  protected readonly total = computed(() =>
    Object.values(this.counts()).reduce((a, b) => a + b, 0),
  );
  protected readonly several = computed(() => this.count() > 1);
  /** The one creature chosen, for the radios. */
  protected readonly single = computed(() =>
    this.several() ? '' : (Object.keys(this.counts())[0] ?? ''),
  );

  /** Is there a slot or a quantity to choose at all (a ritual of one creature has none)? */
  protected readonly hasSetup = computed(
    () => !this.ritual() && (this.slotRows().length > 0 || this.circleOptions().length > 1),
  );
  /** A creature was chosen: the slot and the quantity fold into one line, so the list gets the room. */
  protected readonly collapsed = computed(
    () => this.hasSetup() && this.total() > 0 && !this.editing(),
  );
  /** "3º nível · 2 feras de ND 1 ou menos". */
  protected readonly setupLine = computed(() => setupLineOf(this.slot(), this.opt()));

  protected readonly replacesNote = computed(() => replacesNoteOf(this.spell()));

  protected readonly rows = computed<readonly ChoiceRow[]>(() =>
    rowsOf({
      option: this.opt(),
      beasts: this.beasts(),
      beastBlocks: this.beastBlocks(),
      forms: this.forms(),
    }),
  );
  protected readonly loadingRows = computed(() => {
    const o = this.opt();
    return !!o && (o.forms.length === 0 ? this.beasts() === null : this.forms() === null);
  });
  protected readonly listLabel = computed(() =>
    listLabelOf(this.rows().length, this.familiar(), this.several(), this.total(), this.count()),
  );

  /** What is still missing, in one sentence naming everything; empty when the cast is ready. */
  protected readonly missing = computed(() =>
    missingOf({
      ritual: this.ritual(),
      hasSlot: this.slot() !== null,
      familiar: this.familiar(),
      name: this.name(),
      hasOption: this.opt() !== null,
      several: this.several(),
      count: this.count(),
      total: this.total(),
    }),
  );
  protected readonly ready = computed(
    () =>
      this.missing() === '' &&
      !this.busy() &&
      this.spell() !== null &&
      (!this.needsFace() || this.face() !== null),
  );

  /** "Conjurar como ritual · 1 hora · sem gastar espaço", or what the slot costs. */
  protected readonly costLine = computed(() =>
    costLineOf(this.spell(), this.ritual(), this.slot(), this.count()),
  );

  protected readonly buttonLabel = computed(() =>
    castVerb(this.data.spellKey, this.spell()?.namePt ?? ''),
  );

  constructor() {
    void this.load();
    // After the result, the focus goes to its one button, as soon as it is drawn.
    effect(() => this.back()?.nativeElement.focus());
    // The forms (their numbers) are read once the spell is known; the beasts again when the option changes.
    effect(() => {
      const o = this.opt();
      untracked(() => {
        if (!o) {
          return;
        }
        this.lists.load(o);
      });
    });
  }

  private async load(): Promise<void> {
    try {
      const options = await this.client.summonOptions(this.data.campaignId, this.data.characterId);
      this.options.set(options);
      const spell = options.spells.find((s) => s.spellKey === this.data.spellKey);
      if (!spell) {
        this.error.set('A ficha não conjura mais essa magia. Feche esta folha e olhe a ficha.');
        return;
      }
      if (this.familiar() && spell.replaces[0]) {
        this.name.set(spell.replaces[0].name);
      }
      const free = this.slotRows().find((r) => r.enabled);
      if (free) {
        this.slotKey.set(slotId(free));
      }
    } catch (err) {
      this.options.set(create(GetSummonOptionsResponseSchema, {}));
      this.error.set(creatureErrorMessage(err, 'read'));
    }
  }

  protected pickSlot(row: SlotRow): void {
    this.editing.set(false);
    this.slotKey.set(slotId(row));
    this.option.set(0);
    this.counts.set({});
    this.changed();
  }

  protected pickOption(index: number): void {
    this.editing.set(false);
    this.option.set(index);
    this.counts.set({});
    this.changed();
  }

  /** One creature: the radio's choice. */
  protected pick(key: string): void {
    this.editing.set(false);
    this.counts.set({ [key]: 1 });
    this.changed();
  }

  /** Several creatures: "−" and "+" of one kind. */
  protected step(change: { key: string; delta: number }): void {
    const next = Math.max(0, (this.counts()[change.key] ?? 0) + change.delta);
    if (change.delta > 0 && this.total() >= this.count()) {
      return;
    }
    this.editing.set(false);
    const counts = { ...this.counts(), [change.key]: next };
    if (next === 0) {
      delete counts[change.key];
    }
    this.counts.set(counts);
    this.changed();
  }

  protected setName(value: string): void {
    this.name.set(value);
    this.changed();
  }

  protected readonly optionLabel = optionLabel;

  /** A choice changed: the next cast is a new one (a new key). */
  protected changed(): void {
    this.key = newKey();
    this.error.set('');
  }

  protected async cast(): Promise<void> {
    const spell = this.spell();
    if (!spell || !this.ready()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    // The kinds in the list's order, each as many times as it was chosen.
    const keys = creatureKeysOf(this.rows(), this.counts());
    // Only a familiar is named by the table; any other creature takes the book's name, numbered when several.
    const names = this.familiar() && this.name().trim() !== '' ? [this.name().trim()] : [];
    const slot = this.slot();
    try {
      if (this.combat) {
        await this.castInCombat(spell, keys, names);
        return;
      }
      const res = await this.client.castSummon({
        campaignId: this.data.campaignId,
        characterId: this.data.characterId,
        spellKey: spell.spellKey,
        ritual: this.ritual(),
        slot: this.ritual() || !slot ? undefined : { level: slot.level, pact: slot.pact },
        summon: { option: this.option(), creatureKeys: keys, names },
        idempotencyKey: this.key,
      });
      this.sheet.close({
        spellName: spell.namePt,
        ritual: this.ritual(),
        castingTime: spell.castingTimePt,
        names,
        count: keys.length,
        dismissed: res.replacedIds.length,
      });
    } catch (err) {
      this.error.set(
        this.combat
          ? combatErrorMessage(err, 'conjurar a magia')
          : creatureErrorMessage(err, 'cast'),
      );
      // The refusal is at the top of the body: scroll there, where it is seen.
      setTimeout(() => this.frame().scrollToTop());
    } finally {
      this.busy.set(false);
    }
  }

  /** In a combat the cast goes through `CastSpell`: the d20 of the group's initiative (the app's, or the one typed), the slot and the choice. */
  private async castInCombat(
    spell: SummonSpellOptions,
    keys: readonly string[],
    names: readonly string[],
  ): Promise<void> {
    const combat = this.combat;
    const slot = this.slot();
    if (!combat || !slot) {
      return;
    }
    const face = this.face();
    const die = this.needsFace() && face !== null ? { face } : { inApp: true as const };
    const res = await this.combatApi.casts.castSpell(
      this.data.campaignId,
      combat.encounterId,
      combat.casterId,
      spell.spellKey,
      { level: slot.level, pact: slot.pact },
      [],
      die,
      this.key,
      { option: this.option(), creatureKeys: keys, names },
    );
    combat.state.apply(res.encounter);
    const made = summonResult(res.encounter, res.summoned);
    this.result.set({
      text: made.text,
      creatures: made.creatures,
      title: resultTitleOf(made.creatures),
    });
    this.lists.readArmorClasses(made.creatures.map((c) => c.monsterKey));
    queueMicrotask(() => this.frame().scrollToTop());
  }

  protected line(c: Combatant): string {
    return resultLineOf(c, this.resultAc().get(c.monsterKey));
  }

  protected word(c: Combatant): string {
    return stateWord(c.state);
  }

  protected close(): void {
    // A casting in a combat hands nothing back: the combat already has what it did.
    this.sheet.close();
  }
}
