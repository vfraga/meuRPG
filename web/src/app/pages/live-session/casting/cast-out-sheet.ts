import {
  ChangeDetectionStrategy,
  Component,
  computed,
  effect,
  inject,
  signal,
} from '@angular/core';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatButtonModule } from '@angular/material/button';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';

import { DiceMode, DicePreference } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import type {
  CastingSpell,
  CastingTarget,
  GetCastOptionsResponse,
  OutsideCast,
} from '../../../../gen/meurpg/play/v1/casting_pb';
import { effectivePreference } from '../../../core/campaigns/dice-labels';
import {
  type CastWay,
  castButton,
  endsConcentrationOf,
  isLong,
  minutesOf,
  needsTarget,
  resultSentences,
  resultTitle,
  rollLine,
  spellLine,
  targetHeading,
  targetRows,
  targetRuleOf,
  toggledTarget,
} from '../../../core/casting/cast-out-flow';
import { defaultWayId, grantNumbers, pickOfWay } from '../../../core/casting/cast-grants';
import {
  CastingClient,
  type CastDice,
  type OutsideCastRequest,
} from '../../../core/casting/casting-client';
import { castingErrorText } from '../../../core/casting/casting-errors';
import { type SlotRow, castLevelOf, slotRows } from '../../../core/combat/cast-flow';
import { ActionKey } from '../../../core/connect/idempotency';
import { needsAbility } from '../../../core/effects/ability-choice';
import { SheetFrame } from '../../../shared/sheet/sheet-frame/sheet-frame';
import { injectSheet, openSheet } from '../../../shared/sheet/sheet-host';
import { AbilityPicker } from '../effects/ability-picker/ability-picker';
import { RollPicker } from '../../../shared/roll-picker/roll-picker';
import { SlotPicker } from '../../../shared/slot-picker/slot-picker';
import {
  SummonSheet,
  type SummonSheetData,
  type SummonSheetResult,
} from '../../../shared/summon-sheet/summon-sheet';
import { ChoiceCards } from './choice-cards';
import {
  castRequestOf,
  casterRowsOf,
  diceOf,
  diceTextAt,
  endsConfirmText,
  missingOf,
  spellRowsOf,
  summonedNotice,
  targetChoicesOf,
  wayCardsOf,
} from './cast-out-view';
import { openCastConfirm } from './confirm-sheet';

/** What the page hands the sheet: whose cast it is and how the table rolls. */
export interface CastOutData {
  readonly campaignId: string;
  readonly casterId: string;
  readonly casterName: string;
  /** The master casts (for an NPC or for a character): not held to the number of targets or to the reach. */
  readonly master: boolean;
  readonly diceMode: DiceMode;
  readonly preference: DicePreference;
  /** The spell to start on (a cast opened from a spell of the sheet); empty: the list. */
  readonly spellKey?: string;
  /** The master casting as an NPC: the NPCs with a full sheet to pick "Quem conjura" from; the sheet asks first. */
  readonly npcCasters?: readonly {
    readonly id: string;
    readonly name: string;
    readonly detail: string;
  }[];
}

/** Opens "Conjurar": a bottom sheet on a phone, a dialog from a tablet up. It answers the cast when one was made. */
export function openCastOut(dialog: MatDialog, bottomSheet: MatBottomSheet, data: CastOutData) {
  return openSheet<CastOutSheet, CastOutData, OutsideCast>(dialog, bottomSheet, CastOutSheet, {
    data,
    ariaLabel: `Conjurar: ${data.casterName}`,
    labelledBy: 'cast-out-t',
    width: '520px',
    tall: true,
  });
}

/**
 * "Conjurar" outside a combat (MR-048): the spell, the way (a slot, or a ritual), the targets as the spell takes them and
 * the cast. The server says what is allowed (the slots free, who is in reach, who may cast a ritual); the sheet words it
 * and shows what a refusal says. One idempotency key per request: a tap repeated after a lost answer is the same cast.
 * A spell that needs a roll asks for it in the footer (RN-18); a concentration over another one asks first.
 */
@Component({
  selector: 'app-cast-out-sheet',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [
    AbilityPicker,
    ChoiceCards,
    MatButtonModule,
    MatIconModule,
    RollPicker,
    SheetFrame,
    SlotPicker,
  ],
  templateUrl: './cast-out-sheet.html',
  styleUrl: './cast-out-sheet.scss',
})
export class CastOutSheet {
  private readonly api = inject(CastingClient);
  private readonly dialog = inject(MatDialog);
  private readonly bottomSheet = inject(MatBottomSheet);
  private readonly sheet = injectSheet<CastOutData, OutsideCast>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;

  /** Who casts: the person's own character, or the NPC the master picked. */
  protected readonly casterId = signal(this.data.casterId);
  protected readonly casterName = signal(this.data.casterName);
  protected readonly options = signal<GetCastOptionsResponse | null>(null);
  protected readonly loadError = signal('');
  protected readonly spellKey = signal(this.data.spellKey ?? '');
  /** The way picked: the class's ("slot", "ritual") or a granted spell's ("grant:<source>"). */
  protected readonly wayId = signal('slot');
  protected readonly slot = signal<SlotRow | null>(null);
  protected readonly chosen = signal<string[]>([]);
  protected readonly ability = signal('');
  protected readonly typing = signal(false);
  protected readonly busy = signal(false);
  protected readonly error = signal('');
  protected readonly cast = signal<OutsideCast | null>(null);
  /** What a summoning spell brought, once the creature sheet cast it ("Nanquim chegou. ..."). */
  protected readonly summoned = signal('');
  protected readonly lockWhileBusy = effect(() => this.sheet.lock(this.busy()));
  private readonly key = new ActionKey();

  protected readonly canApp = this.data.diceMode !== DiceMode.PHYSICAL;
  protected readonly canType = this.data.diceMode !== DiceMode.APP;
  protected readonly preferApp =
    effectivePreference(this.data.diceMode, this.data.preference) === DicePreference.APP;

  protected readonly askCaster = computed(() => !!this.data.npcCasters && this.casterId() === '');
  protected readonly casterRows = computed(() => casterRowsOf(this.data.npcCasters ?? []));

  protected readonly spell = computed<CastingSpell | null>(
    () => this.options()?.spells.find((s) => s.spell?.key === this.spellKey()) ?? null,
  );
  protected readonly title = computed(() => {
    const c = this.cast();
    if (c) {
      return c.status === 1 ? `${c.spellNamePt}: conjurando` : resultTitle(c);
    }
    return this.spell()?.spell?.namePt ?? `Conjurar: ${this.casterName() || 'como NPC'}`;
  });
  protected readonly subtitle = computed(() => {
    const s = this.spell();
    return s && !this.cast() ? spellLine(s) : this.casterName();
  });

  protected readonly spellRows = computed(() => spellRowsOf(this.options()?.spells ?? []));
  /** A summoning spell (Convocar Familiar) has no target: the creature sheet casts it, as the character's sheet does. */
  protected readonly summons = computed(() => !!this.spell()?.summons && !this.data.npcCasters);
  protected readonly pick = computed(() => {
    const s = this.spell();
    return s ? pickOfWay(s, this.wayId()) : null;
  });
  protected readonly way = computed<CastWay>(() => this.pick()?.way ?? 'slot');
  /** The grant the cast goes through, for the numbers it shows; `null` for the class's own way. */
  protected readonly grantNote = computed(() => {
    const g = this.pick()?.grant;
    return g ? `${g.sourceNamePt}: ${grantNumbers(g)}.` : '';
  });
  protected readonly wayRows = computed(() => {
    const s = this.spell();
    return s ? wayCardsOf(s) : [];
  });
  /** The ways are asked when there is a choice, or when the only way is not the plain slot (a ritual, a grant). */
  protected readonly showWays = computed(
    () => this.wayRows().length > 1 || (this.wayRows()[0] && this.wayRows()[0].id !== 'slot'),
  );
  protected readonly slots = computed<readonly SlotRow[]>(() => {
    const s = this.spell();
    if (!s || !(this.pick()?.slot ?? false) || (s.spell?.level ?? 0) === 0) {
      return [];
    }
    return slotRows(s.spell?.level ?? 1, s.slots, []);
  });
  /** The level the cast scales at: the slot's, else the one the grant casts at, else the spell's own. */
  protected readonly level = computed(() => {
    const pick = this.pick();
    return castLevelOf(
      this.slot(),
      pick?.slot ? null : pick?.grant,
      this.spell()?.spell?.level ?? 0,
    );
  });
  protected readonly rule = computed(() => {
    const s = this.spell();
    return s ? targetRuleOf(s, this.level(), this.data.master) : { kind: 'none' as const, max: 0 };
  });
  protected readonly showTargets = computed(() => {
    const s = this.spell();
    return !!s && this.rule().kind !== 'none' && (needsTarget(s) || s.maxTargets > 0);
  });
  private readonly targets = computed<readonly CastingTarget[]>(
    () => this.options()?.targets ?? [],
  );
  protected readonly targetChoices = computed(() => {
    const s = this.spell();
    return s ? targetChoicesOf(targetRows(s, this.targets(), this.casterId())) : [];
  });
  protected readonly targetTitle = computed(() => {
    const s = this.spell();
    return s ? targetHeading(s, this.rule()) : '';
  });
  protected readonly long = computed(() => {
    const s = this.spell();
    return !!s && isLong(s, this.way());
  });
  protected readonly minutes = computed(() => {
    const s = this.spell();
    return s ? minutesOf(s, this.way()) : 0;
  });
  protected readonly asksAbility = computed(() => needsAbility(this.spellKey()));
  protected readonly rollsDice = computed(() => !!this.spell()?.rollsDice && !this.long());
  /** The dice the cast rolls at the level it is cast at ("3d8"). */
  protected readonly diceText = computed(() => {
    const s = this.spell();
    return s ? diceTextAt(s, this.level()) : '';
  });
  protected readonly dice = computed(() => diceOf(this.diceText()));
  /** What the slot, the way and the targets still lack, as the button says it. */
  protected readonly missing = computed(() =>
    missingOf(this.spell(), {
      way: this.way(),
      pick: this.pick(),
      slot: this.slot(),
      showTargets: this.showTargets(),
      chosen: this.chosen(),
      spellKey: this.spellKey(),
      ability: this.ability(),
    }),
  );
  protected readonly ready = computed(() => this.missing() === '' && !this.busy());
  protected readonly buttonLabel = computed(() => {
    const s = this.spell();
    if (!s) {
      return 'Conjurar';
    }
    const picked = targetRows(s, this.targets(), this.casterId()).filter((t) =>
      this.chosen().includes(t.id),
    );
    return castButton(s.spell?.namePt ?? '', this.way(), this.long(), picked, s.effect === 2);
  });
  protected readonly ends = computed(() => {
    const s = this.spell();
    return s
      ? endsConcentrationOf(s, this.way(), this.options()?.concentrating ?? this.options()?.casting)
      : null;
  });
  protected readonly resultLines = computed(() => {
    const c = this.cast();
    return c ? resultSentences(c) : [];
  });
  protected readonly resultRoll = computed(() => {
    const c = this.cast();
    return c ? rollLine(c, 0) : '';
  });

  constructor() {
    if (this.casterId()) {
      void this.load();
    }
  }

  protected pickCaster(id: string): void {
    this.casterId.set(id);
    this.casterName.set(this.data.npcCasters?.find((n) => n.id === id)?.name ?? '');
    this.options.set(null);
    this.spellKey.set('');
    this.error.set('');
    void this.load();
  }

  private async load(): Promise<void> {
    try {
      const o = await this.api.options(this.data.campaignId, this.casterId());
      this.options.set(o);
      if (this.spellKey() && this.spell()) {
        this.pickSpell(this.spellKey());
      }
    } catch (err) {
      this.loadError.set(castingErrorText(err));
    }
  }

  protected pickSpell(key: string): void {
    this.spellKey.set(key);
    const s = this.spell();
    this.wayId.set(s ? defaultWayId(s) : 'slot');
    this.slot.set(null);
    this.chosen.set([]);
    this.ability.set('');
    this.error.set('');
    const rows = this.slots();
    const free = rows.filter((r) => r.enabled);
    if (free.length === 1) {
      this.slot.set(free[0]);
    }
  }

  protected pickWay(id: string): void {
    this.wayId.set(id);
    this.slot.set(null);
    this.error.set('');
    const free = this.slots().filter((r) => r.enabled);
    if (free.length === 1) {
      this.slot.set(free[0]);
    }
  }

  protected pickSlot(r: SlotRow): void {
    this.slot.set(r);
    this.chosen.update((c) => c.slice(0, this.rule().max));
  }

  protected toggle(id: string): void {
    this.chosen.set(toggledTarget(this.rule(), this.chosen(), id));
  }

  protected back(): void {
    this.spellKey.set('');
    this.slot.set(null);
    this.chosen.set([]);
    this.error.set('');
  }

  protected close(): void {
    const c = this.cast();
    this.sheet.close(c ?? undefined);
  }

  /** What the cast sends: the way, the slot, the targets, the dice and the ability when the spell asks for one. */
  private request(dice: CastDice | null): OutsideCastRequest {
    return castRequestOf({
      campaignId: this.data.campaignId,
      casterId: this.casterId(),
      spellKey: this.spellKey(),
      way: this.way(),
      pick: this.pick(),
      spell: this.spell(),
      slot: this.slot(),
      chosen: this.chosen(),
      dice: this.rollsDice() ? (dice ?? { inApp: true as const }) : null,
      ability: this.asksAbility() ? this.ability() : null,
    });
  }

  /** "Escolher a criatura": the creature sheet picks the form, the slot or ritual and casts (CastSummon). */
  protected openSummon(): void {
    const s = this.spell();
    if (!s) {
      return;
    }
    openSheet<SummonSheet, SummonSheetData, SummonSheetResult>(
      this.dialog,
      this.bottomSheet,
      SummonSheet,
      {
        data: {
          campaignId: this.data.campaignId,
          characterId: this.casterId(),
          spellKey: this.spellKey(),
        },
        ariaLabel: s.spell?.namePt ?? '',
        labelledBy: 'summon-t',
        width: '520px',
      },
    ).subscribe((r) => {
      if (r) {
        this.summoned.set(summonedNotice(r));
      }
    });
  }

  /** "Conjurar": asks first when it ends a concentration, then casts. */
  protected async castNow(dice: CastDice | null = null): Promise<void> {
    const s = this.spell();
    if (!s || !this.ready()) {
      return;
    }
    const ends = this.ends();
    if (ends) {
      const sure = await new Promise<boolean>((resolve) => {
        openCastConfirm(
          this.dialog,
          this.bottomSheet,
          endsConfirmText(s.spell?.namePt ?? '', ends),
        ).subscribe((r) => resolve(r === true));
      });
      if (!sure) {
        return;
      }
    }
    this.busy.set(true);
    this.error.set('');
    const request = this.request(dice);
    try {
      const res = await this.api.cast(request, this.key.keyFor(request));
      this.cast.set(res.cast ?? null);
    } catch (err) {
      this.error.set(castingErrorText(err));
    } finally {
      this.busy.set(false);
    }
  }
}
