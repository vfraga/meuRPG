import {
  ChangeDetectionStrategy,
  Component,
  type Signal,
  computed,
  effect,
  inject,
  signal,
  viewChild,
} from '@angular/core';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatButtonModule } from '@angular/material/button';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';
import type { Observable } from 'rxjs';

import { DiceMode, DicePreference } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { effectivePreference } from '../../../core/campaigns/dice-labels';
import { ActionKey } from '../../../core/connect/idempotency';
import {
  type HitDieSize,
  diceLeft,
  dieName,
  hitDieRollLine,
  songOfRestAsk,
  songOfRestOf,
} from '../../../core/resources/hit-dice-text';
import { RollAnimator, showOfDie } from '../../../shared/roll-overlay/roll-animator';
import { type HitDieRoll, ResourceClient } from '../../../core/resources/resources-client';
import { ResourceBlockedReason } from '../../../../gen/meurpg/play/v1/resources_pb';
import {
  arcaneRecoveryErrorMessage,
  hitDiceErrorMessage,
  resourceBlocked,
} from '../../../core/resources/resources-errors';
import { SheetFrame } from '../../../shared/sheet/sheet-frame/sheet-frame';
import { injectSheet, openSheet } from '../../../shared/sheet/sheet-host';
import { RollPicker } from '../../../shared/roll-picker/roll-picker';
import { toVitalsVm } from '../../../core/play/live-session-source.live';
import type { VitalsVm } from '../../../core/play/live-session.types';
import { arcaneRecoveryState, expendedSlots, levelsWords } from './arcane-recovery';

/** What the page hands "Gastar dados de vida". */
export interface HitDiceSheetData {
  readonly campaignId: string;
  /** The character's vitals as the page has them: the sheet follows the stream's changes too. */
  readonly vitals: Signal<VitalsVm>;
  readonly diceMode: DiceMode;
  readonly preference: DicePreference;
  /** The vitals the server answered after a die: the page takes them in, like any other change. */
  readonly apply: (vitals: VitalsVm) => void;
}

/** "Gastar dados de vida": a bottom sheet on a phone, a dialog from a tablet up. Answers `true` when a die was spent. */
export function openHitDice(
  dialog: MatDialog,
  bottomSheet: MatBottomSheet,
  data: HitDiceSheetData,
): Observable<boolean | undefined> {
  return openSheet<HitDiceSheet, HitDiceSheetData, boolean>(dialog, bottomSheet, HitDiceSheet, {
    data,
    ariaLabel: 'Gastar dados de vida',
    labelledBy: 'hit-dice-t',
    width: '480px',
    // The opener takes the focus back, with the focus ring (the browser draws none for a focus a dialog restores).
    restoreFocus: false,
  });
}

/**
 * "Gastar dados de vida" (decisions batch 2, B 6; SRD 5.1, "Short Rest"): in a short rest the player spends hit dice
 * to heal, one at a time, and decides after each roll whether to spend another. The sheet lists the dice left by size
 * as radio cards, then rolls the chosen size: in the app ("Rolar d10 no app") or, when the table rolls real dice
 * (the campaign's dice setting and the player's choice, as in the attack sheet), with the face of the real die typed
 * in. The answer is one line, "Rolou 7 + 2 = 9 · recuperou 9 PV", and the sheet is ready for the next die. Each
 * die has its own idempotency key: the same request again (a second tap, or a try after a lost answer) is the same
 * key and never spends twice; once a die worked, the next one, even of the same size, is a new key. The server
 * refuses a die of a size with none left, and any die while a combat is open.
 */
@Component({
  selector: 'app-hit-dice-sheet',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [MatButtonModule, MatIconModule, RollPicker, SheetFrame],
  templateUrl: './hit-dice-sheet.html',
  styleUrl: './hit-dice-sheet.scss',
})
export class HitDiceSheet {
  private readonly api = inject(ResourceClient);
  private readonly animator = inject(RollAnimator);
  private readonly sheet = injectSheet<HitDiceSheetData, boolean>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;

  protected readonly canApp = this.data.diceMode !== DiceMode.PHYSICAL;
  protected readonly canType = this.data.diceMode !== DiceMode.APP;
  protected readonly preferApp =
    effectivePreference(this.data.diceMode, this.data.preference) === DicePreference.APP;

  /** The newest copy of the vitals: the page's, or the answer of a die, whichever has the larger revision. */
  private readonly answered = signal<VitalsVm | null>(null);
  protected readonly vitals = computed(() => {
    const own = this.data.vitals();
    const answer = this.answered();
    return answer && answer.revision > own.revision ? answer : own;
  });

  protected readonly sizes = computed(() =>
    this.vitals().hitDiceSizes.map((s) => ({ ...s, left: diceLeft(s) })),
  );
  protected readonly anyLeft = computed(() => this.sizes().some((s) => s.left > 0));

  /** The size the player picked; it gives way to another one when it runs out. */
  private readonly picked = signal<number | null>(null);
  protected readonly faces = computed(() => {
    const sizes = this.sizes();
    const wanted = sizes.find((s) => s.faces === this.picked() && s.left > 0);
    return (wanted ?? sizes.find((s) => s.left > 0))?.faces ?? null;
  });
  protected readonly chosenName = computed(() => dieName(this.faces() ?? 0));

  protected readonly typing = signal(false);
  protected readonly busy = signal(false);
  /** A request in the air: Esc and the backdrop do not close the sheet under it. */
  protected readonly lockWhileBusy = effect(() => this.sheet.lock(this.busy()));
  protected readonly error = signal('');
  /** What each die did, newest first. */
  protected readonly rolls = signal<readonly string[]>([]);
  /** The newest line, for the live region. */
  protected readonly latest = computed(() => this.rolls().at(0) ?? '');
  private readonly picker = viewChild(RollPicker);

  protected readonly subtitle = computed(() => {
    const v = this.vitals();
    return `${v.name} · ${v.hitPointsCurrent} de ${v.hitPointsMax} PV`;
  });

  /** One key for each die: the request again is a retry, a worked die frees the key. */
  private readonly key = new ActionKey();

  protected pick(faces: number): void {
    this.picked.set(faces);
    this.error.set('');
    this.picker()?.clear();
  }

  protected leftWords(size: HitDieSize & { left: number }): string {
    if (size.left === 0) {
      return 'Nenhum restante';
    }
    return `${size.left === 1 ? 'Resta' : 'Restam'} ${size.left} de ${size.total}${dieName(size.faces)}`;
  }

  protected readonly dieName = dieName;

  protected rollInApp(): Promise<void> {
    return this.spend({ inApp: true });
  }

  protected rollTyped(face: number): Promise<void> {
    const ask = this.songAsk();
    // The second number asked for: the song's die, with the face of the hit die already typed.
    return this.spend(ask ? { face: ask.face, songFace: face } : { face });
  }

  /**
   * A bard's Song of Rest (SRD 5.1) gives this character an extra die with the first hit die of the rest. With a real
   * die the server asks for its face after the first one was typed: the sheet keeps that face and asks for the
   * song's, in the same picker.
   */
  protected readonly songAsk = signal<{ readonly sides: number; readonly face: number } | null>(
    null,
  );
  protected readonly pickerMax = computed(() => this.songAsk()?.sides ?? this.faces() ?? 1);
  protected readonly pickerLabel = computed(() => {
    const ask = this.songAsk();
    return ask ? songOfRestAsk(ask.sides) : 'Role 1' + this.chosenName() + ' para o dado de vida';
  });
  protected readonly pickerHint = computed(() => {
    const ask = this.songAsk();
    const name = ask ? dieName(ask.sides) : this.chosenName();
    return `Role o seu ${name} e digite o número que saiu (1 a ${this.pickerMax()}).`;
  });

  protected cancelSong(): void {
    this.songAsk.set(null);
    this.picker()?.reset();
  }

  private async spend(roll: HitDieRoll): Promise<void> {
    const faces = this.faces();
    if (faces === null || this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const res = await this.api.spendHitDice(
        this.data.campaignId,
        this.vitals().characterId,
        faces,
        roll,
        this.key.keyFor({ faces, roll }),
      );
      this.key.renew();
      this.songAsk.set(null);
      if (res.vitals) {
        const vitals = toVitalsVm(res.vitals);
        this.answered.set(vitals);
        this.data.apply(vitals);
      }
      if ('inApp' in roll) {
        const show = showOfDie(
          'Dado de vida',
          faces,
          res.face,
          hitDieRollLine(res.face, res.constitutionModifier, res.healed, songOfRestOf(res)),
        );
        if (show) {
          this.animator.play(show);
        }
      }
      this.rolls.update((all) => [
        hitDieRollLine(res.face, res.constitutionModifier, res.healed, songOfRestOf(res)),
        ...all,
      ]);
      this.typing.set(false);
      this.picker()?.reset();
    } catch (err) {
      const blocked = resourceBlocked(err);
      if (
        blocked?.reason === ResourceBlockedReason.SONG_OF_REST_FACE_NEEDED &&
        !('inApp' in roll)
      ) {
        this.songAsk.set({ sides: blocked.needed, face: roll.face });
        this.picker()?.reset();
      } else {
        this.error.set(hitDiceErrorMessage(err));
      }
    } finally {
      this.busy.set(false);
    }
  }

  // --- Recuperação Arcana (SRD 5.1, Wizard): after a short rest, expended slots up to half the wizard level. ---

  protected readonly arcane = computed(() => arcaneRecoveryState(this.vitals()));
  protected readonly arcaneSlots = computed(() => expendedSlots(this.vitals()));
  protected readonly arcaneHelp = computed(
    () =>
      `Recupere espaços de magia que somem até ${levelsWords(this.arcane()?.allowance ?? 0)}; nenhum de 6º nível ou mais.`,
  );
  /** How many slots of each spell level the player picked to recover. */
  protected readonly arcanePicked = signal<Readonly<Record<number, number>>>({});
  /** The combined level of the picked slots. */
  protected readonly arcaneTotal = computed(() =>
    Object.entries(this.arcanePicked()).reduce((sum, [level, n]) => sum + Number(level) * n, 0),
  );
  protected readonly arcaneBusy = signal(false);
  protected readonly arcaneKey = new ActionKey();

  protected arcaneCount(level: number): number {
    return this.arcanePicked()[level] ?? 0;
  }

  protected canAddArcane(slot: { level: number; expended: number }): boolean {
    return (
      this.arcaneCount(slot.level) < slot.expended &&
      this.arcaneTotal() + slot.level <= (this.arcane()?.allowance ?? 0)
    );
  }

  protected stepArcane(level: number, by: 1 | -1): void {
    const count = Math.max(this.arcaneCount(level) + by, 0);
    this.arcanePicked.update((all) => ({ ...all, [level]: count }));
    this.error.set('');
  }

  protected async recoverArcane(): Promise<void> {
    const slots = Object.entries(this.arcanePicked())
      .map(([level, count]) => ({ level: Number(level), count }))
      .filter((s) => s.count > 0);
    if (slots.length === 0 || this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const res = await this.api.useArcaneRecovery(
        this.data.campaignId,
        this.vitals().characterId,
        slots,
        this.arcaneKey.keyFor(slots),
      );
      this.arcaneKey.renew();
      if (res.vitals) {
        const vitals = toVitalsVm(res.vitals);
        this.answered.set(vitals);
        this.data.apply(vitals);
      }
      this.arcanePicked.set({});
      this.arcaneDone.set(
        `Recuperou espaços de magia que somam ${levelsWords(res.recoveredLevels)}.`,
      );
    } catch (err) {
      this.error.set(arcaneRecoveryErrorMessage(err));
    } finally {
      this.busy.set(false);
    }
  }

  /** What Recuperação Arcana just did, for the live region. */
  protected readonly arcaneDone = signal('');

  protected close(): void {
    if (this.busy()) {
      return;
    }
    this.sheet.close(this.rolls().length > 0 || this.arcaneDone() !== '');
  }
}
