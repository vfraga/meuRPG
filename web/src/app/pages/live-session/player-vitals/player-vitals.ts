import {
  Component,
  DOCUMENT,
  DestroyRef,
  computed,
  effect,
  inject,
  input,
  output,
  signal,
  untracked,
} from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatButtonModule } from '@angular/material/button';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';
import { RouterLink } from '@angular/router';

import { DiceMode, DicePreference } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { article } from '../../../core/combat/combat-log';
import { countsSentence } from '../../../core/combat/death-saves';
import {
  SHIELD_ENDED_MS,
  aidBar,
  aidLabel,
  aidTag,
  armorClassWithShield,
  shieldEndedParts,
  shieldLabelForOwner,
  shieldSum,
  sheetMaximum,
  vitalsSpeech,
} from '../../../core/combat/combat-effects';
import { EffectPill } from '../../../shared/effect-pill/effect-pill';
import { type EffectCardView, exhaustionLabel } from '../../../core/effects/effects';
import { EffectCards } from '../effects/effect-cards/effect-cards';
import { focusWithRing } from '../../../core/creatures/focus-ring';
import { hitDiceLeftWords, totalDiceLeft } from '../../../core/resources/hit-dice-text';
import { arcaneRecoveryState } from '../hit-dice-sheet/arcane-recovery';
import { openHitDice } from '../hit-dice-sheet/hit-dice-sheet';
import { WildPools } from '../../../shared/wild-shape/wild-pools';
import { PlayerSheetVm, VitalsVm } from '../../../core/play/live-session.types';
import { SlotDots } from '../../../shared/slot-dots/slot-dots';
import {
  betterArmorClass,
  freeWords,
  hitPointsPercent,
  slotLevelLabel,
  slotRowLabel,
} from '../../../core/play/vitals';

/** How long "Acordado · testes contra a morte zerados" stays under the cards. */
const WOKE_NOTICE_MS = 30000;

interface SlotRowVm {
  readonly key: string;
  readonly label: string;
  readonly total: number;
  readonly used: number;
}

/**
 * The player's own vitals on the session page (RN-02, artboards E5-02 and
 * E5-03): the sheet's combat block with the current values filled in. The
 * PV box first (the number they came for), then the CA shield, temporary
 * HP and hit dice, then the spell slots, a link to the full sheet and who
 * changes these numbers. A player only ever sees their own character
 * (decision row 28): the server sends nothing else.
 */
@Component({
  selector: 'app-player-vitals',
  imports: [
    EffectCards,
    EffectPill,
    MatButtonModule,
    MatIconModule,
    RouterLink,
    SlotDots,
    WildPools,
  ],
  templateUrl: './player-vitals.html',
  styleUrl: './player-vitals.scss',
})
export class PlayerVitals {
  readonly vitals = input.required<VitalsVm>();
  /** From the character's sheet; `null` until it loads. */
  readonly sheet = input<PlayerSheetVm | null>(null);
  readonly campaignId = input.required<string>();
  /** While the druid is a beast: the armor class of the beast's book, in place of the character's own (the server sends none). */
  readonly beastAc = input<number | null>(null);
  /** The combat's version (E6-05): the PV box and the shield side by side,
   * then the slots; no temporary HP, hit dice or footer. */
  readonly compact = input(false);
  /** The Escudo Arcano's bonus on the character's own combatant while a combat runs (0 without the spell); `null`
   * when there is no combat or no combatant, which is not the shield ending. */
  readonly armorClassBonus = input<number | null>(null);
  /** The death saves of the player's own combatant while a combat runs; `null` without one (the pill then says only "Inconsciente"). */
  readonly ownDeathSaves = input<{ successes: number; failures: number } | null>(null);

  /** Whether it is the character's own turn in the combat: the death save is rolled then (SRD 5.1), so off turn the card says so. */
  readonly onTurn = input(false);
  /** The hint of a character at 0 hit points in a combat, off turn: when the death save is rolled. */
  protected readonly deathSaveHint = computed(
    () => this.down() && !this.pools() && this.ownDeathSaves() !== null && !this.onTurn(),
  );

  /** "Seus efeitos": the cards of the effects the server lets this player read (on the combatant in a combat, on the
   * character outside one); none draws nothing. */
  readonly effectCards = input<readonly EffectCardView[]>([]);
  /** The conditions those effects hold ("Paralisado"), said once in a label under the cards. */
  readonly conditionNames = input<readonly string[]>([]);

  /** How the campaign has the players roll their dice and the player's own choice: the hit die sheet follows them (RN-18). */
  readonly diceMode = input<DiceMode>(DiceMode.PLAYERS_CHOOSE);
  readonly dicePreference = input<DicePreference>(DicePreference.APP);
  /** The vitals the server answered after a hit die was spent: the page takes them in like any other change. */
  readonly vitalsChange = output<VitalsVm>();

  private readonly document = inject(DOCUMENT);
  private readonly destroyRef = inject(DestroyRef);
  private readonly dialog = inject(MatDialog);
  private readonly bottomSheet = inject(MatBottomSheet);

  /** The two reserves of a druid in a beast form, the beast's first: they take the place of the hit points box. */
  protected readonly pools = computed(() => {
    const v = this.vitals();
    const w = v.wildShape;
    if (!w) {
      return null;
    }
    const of = (name: string) => `PV d${article(name) === 'a' ? 'a' : 'o'} ${name}`;
    return {
      beast: { label: of(w.beastNamePt), current: w.hitPointsCurrent, max: w.hitPointsMax },
      character: { label: of(v.name), current: v.hitPointsCurrent, max: v.hitPointsMax },
    };
  });
  /** The armor class on the shield: the beast's while it is one, else the sheet's. */
  private readonly baseArmorClass = computed(() =>
    this.vitals().wildShape
      ? this.beastAc()
      : betterArmorClass(this.sheet()?.armorClass ?? null, this.vitals().armorClassBase),
  );
  /** What the shield shows: the armor class already summed with the Escudo Arcano ("18"). */
  protected readonly armorClass = computed(() => {
    const base = this.baseArmorClass();
    return base === null ? null : armorClassWithShield(base, this.shieldBonus());
  });
  protected readonly shieldBonus = computed(() => this.armorClassBonus() ?? 0);
  /** The small sum under the shield: "13 + 5"; empty without the spell. */
  protected readonly shieldSum = computed(() => {
    const base = this.baseArmorClass();
    return base !== null && this.shieldBonus() > 0 ? shieldSum(base, this.shieldBonus()) : '';
  });
  protected readonly shieldLabel = shieldLabelForOwner;
  protected readonly conditions = computed(() => this.conditionNames());
  /** The level of exhaustion the master set, 0 to 6. */
  protected readonly exhaustion = computed(() => this.vitals().exhaustionLevel ?? 0);
  protected readonly exhaustionLabel = exhaustionLabel;
  /** At 0 hit points in the character's own shape: "Inconsciente" and the death save counts. */
  protected readonly down = computed(() => this.vitals().hitPointsCurrent <= 0);
  /** "1 sucesso, 1 falha", or empty when no combat tells the counts. */
  protected readonly deathSaves = computed(() => {
    const s = this.ownDeathSaves();
    return s ? countsSentence(s.successes, s.failures) : '';
  });
  /** Ajuda raised the hit points of a character at 0: it is awake and the death saves are cleared, for a while. */
  protected readonly woke = signal(false);
  private wokeTimer: ReturnType<typeof setTimeout> | undefined;

  /** Ajuda's bonus: 0 without it. */
  protected readonly aid = computed(() => Math.max(0, this.vitals().hitPointsMaxBonus ?? 0));
  protected readonly aidTag = aidTag;
  protected readonly aidLabel = aidLabel;
  protected readonly sheetMaximum = computed(() =>
    sheetMaximum(this.vitals().hitPointsMax, this.aid()),
  );
  protected readonly speech = computed(() => {
    const v = this.vitals();
    return vitalsSpeech(v.hitPointsCurrent, v.hitPointsMax, this.aid());
  });
  protected readonly barLabel = computed(() => {
    const v = this.vitals();
    return `${v.hitPointsCurrent} de ${v.hitPointsMax} pontos de vida`;
  });
  protected readonly bar = computed(() => {
    const v = this.vitals();
    return aidBar(v.hitPointsCurrent, v.hitPointsMax, this.aid());
  });
  /** The sentence of a shield that just ended, for a few seconds. */
  protected readonly shieldEnded = signal<{ lead: string; rest: string } | null>(null);
  private endedTimer: ReturnType<typeof setTimeout> | undefined;
  private previousBonus: number | null = null;

  /** "Classe de Armadura", or "CA do Lobo" while a beast (the shield is narrow). */
  protected readonly acLabel = computed(() => {
    const w = this.vitals().wildShape;
    return w
      ? `CA d${article(w.beastNamePt) === 'a' ? 'a' : 'o'} ${w.beastNamePt}`
      : 'Classe de Armadura';
  });

  /** "3 de 5d10 e 1 de 1d6": the dice left by size. */
  protected readonly diceLeft = computed(() => hitDiceLeftWords(this.vitals().hitDiceSizes));
  protected readonly freeWords = freeWords;
  /** "Gastar dados de vida" has nothing to spend when every die is used. */
  protected readonly noDiceLeft = computed(
    () => totalDiceLeft(this.vitals().hitDiceSizes) === 0 && !this.arcaneOpen(),
  );
  /** A wizard whose Recuperação Arcana is not spent keeps the short rest sheet open even without a die. */
  private readonly arcaneOpen = computed(() => {
    const state = arcaneRecoveryState(this.vitals());
    return state !== null && !state.spent;
  });

  /** "Gastar dados de vida": the sheet where a short rest's dice are spent one by one. */
  protected spendHitDice(): void {
    if (this.noDiceLeft()) {
      return;
    }
    const opener = this.document.activeElement as HTMLElement | null;
    openHitDice(this.dialog, this.bottomSheet, {
      campaignId: this.campaignId(),
      vitals: this.vitals,
      diceMode: this.diceMode(),
      preference: this.dicePreference(),
      apply: (v) => this.vitalsChange.emit(v),
    })
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe(() => focusWithRing(opener));
  }

  protected readonly percent = computed(() => hitPointsPercent(this.vitals()));

  protected readonly slotRows = computed<SlotRowVm[]>(() => {
    const v = this.vitals();
    const rows: SlotRowVm[] = v.spellSlots.map((s) => ({
      key: `level-${s.level}`,
      label: slotLevelLabel(s.level),
      total: s.total,
      used: s.used,
    }));
    if (v.pactSlots) {
      rows.push({
        key: 'pact',
        label: `Pacto, ${slotLevelLabel(v.pactSlots.slotLevel)}`,
        total: v.pactSlots.total,
        used: v.pactSlots.used,
      });
    }
    return rows;
  });

  protected readonly slotRowLabel = slotRowLabel;

  /** Said once by screen readers when the master changes a number: the
   * screen changes under the player's eyes, and this says what changed. */
  protected readonly announcement = signal('');
  private previous: VitalsVm | null = null;

  constructor() {
    inject(DestroyRef).onDestroy(() => {
      clearTimeout(this.endedTimer);
      clearTimeout(this.wokeTimer);
    });
    // The shield that the combat takes away (the bonus goes from above 0 to 0 on this player's own combatant) is said once,
    // politely, and goes away by itself; a combat that ended or a combatant that is gone is not "the shield ended".
    effect(() => {
      const bonus = this.armorClassBonus();
      const before = this.previousBonus;
      this.previousBonus = bonus;
      if (before !== null && before > 0 && bonus === 0) {
        this.shieldEnded.set(shieldEndedParts(untracked(() => this.baseArmorClass())));
        clearTimeout(this.endedTimer);
        this.endedTimer = setTimeout(() => this.shieldEnded.set(null), SHIELD_ENDED_MS);
      }
    });
    effect(() => {
      const v = this.vitals();
      const before = this.previous;
      this.previous = v;
      this.updateWoke(before, v);
      if (!before || before.characterId !== v.characterId || before.revision === v.revision) {
        return;
      }
      this.announcement.set(
        `O mestre ajustou: ${v.hitPointsCurrent} de ${v.hitPointsMax} pontos de vida, ` +
          `${v.hitPointsTemporary} temporários.`,
      );
    });
  }

  /** From 0 to above 0 while Ajuda is on: the "Acordado" pill, for WOKE_NOTICE_MS; back to 0 or no Ajuda clears it. */
  private updateWoke(before: VitalsVm | null, v: VitalsVm): void {
    const bonus = v.hitPointsMaxBonus ?? 0;
    if (
      before &&
      before.characterId === v.characterId &&
      before.hitPointsCurrent <= 0 &&
      v.hitPointsCurrent > 0 &&
      bonus > 0
    ) {
      this.woke.set(true);
      clearTimeout(this.wokeTimer);
      this.wokeTimer = setTimeout(() => this.woke.set(false), WOKE_NOTICE_MS);
    } else if (v.hitPointsCurrent <= 0 || bonus <= 0) {
      this.woke.set(false);
    }
  }
}
