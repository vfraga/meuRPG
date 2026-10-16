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
  defaultWay,
  endsConcentrationOf,
  isLong,
  minutesOf,
  minutesWords,
  needsTarget,
  resultSentences,
  resultTitle,
  rollLine,
  ritualTimeText,
  spellLine,
  targetHeading,
  targetRows,
  targetRuleOf,
  toggledTarget,
  waysOf,
} from '../../../core/casting/cast-out-flow';
import {
  CastingClient,
  type CastDice,
  type OutsideCastRequest,
} from '../../../core/casting/casting-client';
import { castingErrorText } from '../../../core/casting/casting-errors';
import { type SlotRow, slotRows } from '../../../core/combat/cast-flow';
import { ActionKey } from '../../../core/connect/idempotency';
import { abilityMissing, needsAbility } from '../../../core/effects/ability-choice';
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
import { castNotice, creaturesText } from '../../../core/creatures/summon-labels';
import { type ChoiceRow, ChoiceCards } from './choice-cards';
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
  protected readonly way = signal<CastWay>('slot');
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
  protected readonly casterRows = computed<readonly ChoiceRow[]>(() =>
    (this.data.npcCasters ?? []).map((n) => ({
      id: n.id,
      title: n.name,
      detail: n.detail,
      enabled: true,
      reason: '',
    })),
  );

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

  protected readonly spellRows = computed<readonly ChoiceRow[]>(() =>
    (this.options()?.spells ?? []).map((s) => ({
      id: s.spell?.key ?? '',
      title: s.spell?.namePt ?? '',
      detail: spellLine(s),
      enabled: s.canCast || s.ritualAllowed,
      reason: s.canCast || s.ritualAllowed ? '' : 'Sem espaço de magia livre.',
    })),
  );
  /** A summoning spell (Convocar Familiar) has no target: the creature sheet casts it, as the character's sheet does. */
  protected readonly summons = computed(() => !!this.spell()?.summons && !this.data.npcCasters);
  protected readonly ways = computed<CastWay[]>(() => {
    const s = this.spell();
    return s ? waysOf(s) : [];
  });
  protected readonly wayRows = computed<readonly ChoiceRow[]>(() => {
    const s = this.spell();
    if (!s) {
      return [];
    }
    return this.ways().map((w) =>
      w === 'slot'
        ? {
            id: 'slot',
            title: 'Com espaço de magia',
            detail:
              s.castingMinutes > 0 ? `Tempo: ${minutesWords(s.castingMinutes)}` : s.castingTimePt,
            enabled: true,
            reason: '',
          }
        : {
            id: 'ritual',
            title: 'Como ritual',
            detail: `${ritualTimeText(s)} · sem espaço de magia`,
            enabled: true,
            reason: '',
          },
    );
  });
  protected readonly slots = computed<readonly SlotRow[]>(() => {
    const s = this.spell();
    if (!s || this.way() !== 'slot' || (s.spell?.level ?? 0) === 0) {
      return [];
    }
    return slotRows(s.spell?.level ?? 1, s.slots, []);
  });
  protected readonly level = computed(() => this.slot()?.level ?? this.spell()?.spell?.level ?? 0);
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
  protected readonly targetChoices = computed<readonly ChoiceRow[]>(() => {
    const s = this.spell();
    return s
      ? targetRows(s, this.targets(), this.casterId()).map((t) => ({
          id: t.id,
          title: t.self ? `${t.name} (você)` : t.name,
          detail: t.detail,
          enabled: t.enabled,
          reason: t.reason,
        }))
      : [];
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
  protected readonly dice = computed(() => {
    const m = /^(\d+)d(\d+)$/.exec(this.spell()?.rollDice ?? '');
    return m ? { count: Number(m[1]), sides: Number(m[2]) } : { count: 1, sides: 4 };
  });
  /** What the slot, the way and the targets still lack, as the button says it. */
  protected readonly missing = computed(() => {
    const s = this.spell();
    if (!s) {
      return '';
    }
    if (this.way() === 'slot' && (s.spell?.level ?? 0) > 0 && !this.slot()) {
      return 'Escolha o espaço de magia.';
    }
    if (this.showTargets() && needsTarget(s) && this.chosen().length === 0) {
      return 'Escolha em quem a magia age.';
    }
    return abilityMissing(this.spellKey(), this.ability());
  });
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
    this.way.set(s ? defaultWay(s) : 'slot');
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
    this.way.set(id === 'ritual' ? 'ritual' : 'slot');
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
    const slot =
      this.way() === 'ritual' || (this.spell()?.spell?.level ?? 0) === 0 ? null : this.slot();
    return {
      campaignId: this.data.campaignId,
      casterId: this.casterId(),
      spellKey: this.spellKey(),
      asRitual: this.way() === 'ritual',
      slot: slot ? { level: slot.level, pact: slot.pact } : null,
      targetIds: this.chosen(),
      dice: this.rollsDice() ? (dice ?? { inApp: true as const }) : null,
      ...(this.asksAbility() ? { abilityKey: this.ability() } : {}),
    };
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
        const arrived =
          r.names.length === 1
            ? `${r.names[0]} chegou.`
            : r.count === 1
              ? 'A criatura chegou.'
              : `${creaturesText(r.count)} chegaram.`;
        this.summoned.set(castNotice(arrived, r.spellName, r.ritual, r.castingTime, r.dismissed));
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
        openCastConfirm(this.dialog, this.bottomSheet, {
          title: `Isso encerra ${ends}`,
          body: `Conjurar ${s.spell?.namePt ?? ''} encerra a concentração em ${ends}, e os efeitos dela terminam.`,
          confirm: `Encerrar ${ends} e conjurar`,
          cancel: 'Cancelar',
        }).subscribe((r) => resolve(r === true));
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
