import { Component, ElementRef, computed, effect, inject, signal, viewChild } from '@angular/core';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatButtonModule } from '@angular/material/button';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';
import type { Observable } from 'rxjs';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import type { SceneActionView, SceneRoll } from '../../../../../gen/meurpg/play/v1/scene_pb';
import { effectivePreference } from '../../../../core/campaigns/dice-labels';
import { newKey } from '../../../../core/combat/combat-client';
import { SceneClient, type SceneDie } from '../../../../core/play/scene-client';
import { sceneBlocked, sceneErrorMessage } from '../../../../core/play/scene-errors';
import type { SceneState } from '../../../../core/play/scene-state';
import { passText, sceneRollFormula, signedBonus } from '../../../../core/play/scene-view';
import { actionSubtitle, actionTitle } from '../../../../core/maps/scene-actions';
import { joinDots } from '../../../../core/format/text';
import { mediaQuery } from '../../../../shared/map-view/media-query';
import { RollPicker } from '../../combat/roll-picker/roll-picker';
import { SheetFrame } from '../../combat/sheet-frame/sheet-frame';
import { injectSheet, openSheet } from '../../combat/sheet-host';

/** What the player's block hands the roll sheet. */
export interface SceneRollSheetData {
  readonly campaignId: string;
  readonly action: SceneActionView;
  readonly diceMode: DiceMode;
  readonly preference: DicePreference;
  readonly state: SceneState;
}

/** The roll sheet of one action: a bottom sheet on a phone, a dialog from a
 * tablet up. It answers `true` when the roll was made. */
export function openSceneRollSheet(
  dialog: MatDialog,
  bottomSheet: MatBottomSheet,
  data: SceneRollSheetData,
): Observable<boolean | undefined> {
  return openSheet<SceneRollSheet, SceneRollSheetData, boolean>(
    dialog,
    bottomSheet,
    SceneRollSheet,
    {
      data,
      ariaLabel: `Rolar ${actionTitle(data.action)}`,
    },
  );
}

/**
 * Rolling one action of the scene (E7-04, MR-015, RN-18): the check and the
 * player's own bonus, then the two ways to roll as the combat has them
 * ("Rolar no app" is the filled button, "Digitar o resultado" a link; the
 * preference "Como você rola os dados" picks which opens first, a forced mode
 * hides the other). A typed die takes 1 to 20 and the total shows live. The
 * result is the player's own total with its formula, and "Passou · CD 12" or
 * "Não passou · CD 10" only when the master shows the DC (RN-20); otherwise
 * there is no DC and no "passou". The key is made once, so a tap sent again after a lost answer never
 * rolls twice. Focus: the title opens first, and the result's "Voltar à cena".
 */
@Component({
  selector: 'app-scene-roll-sheet',
  imports: [MatButtonModule, MatIconModule, RollPicker, SheetFrame],
  templateUrl: './scene-roll-sheet.html',
  styleUrl: './scene-roll-sheet.scss',
})
export class SceneRollSheet {
  private readonly api = inject(SceneClient);
  private readonly sheet = injectSheet<SceneRollSheetData, boolean>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;
  protected readonly action = this.data.action;

  protected readonly typing = signal(false);
  /** A short screen (320×568): the typed form uses the picker's tighter size, so the field,
   * the live total and the footer all fit with nothing covered (E7-04). */
  protected readonly short = mediaQuery('(max-height: 600px)');
  protected readonly busy = signal(false);
  /** A request in the air: Esc and the backdrop do not close the sheet under it. */
  protected readonly lockWhileBusy = effect(() => this.sheet.lock(this.busy()));
  protected readonly error = signal('');
  protected readonly roll = signal<SceneRoll | null>(null);

  private readonly key = newKey();
  private readonly back = viewChild('back', { read: ElementRef<HTMLButtonElement> });
  private readonly frame = viewChild(SheetFrame);

  protected readonly canApp = this.data.diceMode !== DiceMode.PHYSICAL;
  protected readonly canType = this.data.diceMode !== DiceMode.APP;
  protected readonly preferApp =
    effectivePreference(this.data.diceMode, this.data.preference) === DicePreference.APP;
  protected readonly bonus = this.action.bonus ?? 0;
  protected readonly bonusText = signedBonus(this.bonus);
  /** "1d20 + 6", the signs and numbers held together. */
  protected readonly diceLine = `1d20\u00a0${this.bonus < 0 ? '−' : '+'}\u00a0${Math.abs(this.bonus)}`;
  protected readonly name = actionTitle(this.action);
  protected readonly checkLine = actionSubtitle(this.action);

  protected readonly title = computed(() => {
    if (this.roll()) {
      return this.name;
    }
    return this.typing() ? 'Digite o resultado do dado' : this.name;
  });
  protected readonly subtitle = computed(() => {
    const r = this.roll();
    if (r) {
      return joinDots([this.checkLine, r.roll?.physical ? 'dado físico' : 'rolado no app']);
    }
    return this.typing()
      ? joinDots([this.name, this.action.checkName])
      : joinDots([this.checkLine, `bônus ${this.bonusText}`]);
  });
  protected readonly formula = computed(() => {
    const r = this.roll();
    return r ? sceneRollFormula(r) : '';
  });
  /** "Passou · CD 12", only when the master shows the DC: the server sends `passed` then and not otherwise. */
  protected readonly pass = computed(() => {
    const passed = this.roll()?.passed;
    return passed === undefined ? null : { ok: passed, text: passText(passed, this.action.dc) };
  });
  protected readonly rollLabel = `Role 1d20 para ${this.action.checkName} (${this.bonusText})`;

  constructor() {
    // After the result the focus goes to its one action, as soon as it is drawn.
    effect(() => this.back()?.nativeElement.focus());
    // An error opens at the top of the scrolling body, where it is seen.
    effect(() => {
      if (this.error()) {
        this.frame()?.scrollToTop();
      }
    });
  }

  protected async rollWith(die: SceneDie): Promise<void> {
    if (this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const made = await this.api.roll(this.data.campaignId, this.action.id, die, this.key);
      this.roll.set(made);
      this.typing.set(false);
      // The row under the sheet turns into "Rolada" as soon as the read comes back.
      void this.data.state.refresh();
    } catch (err) {
      this.error.set(sceneErrorMessage(err, 'rolar a ação'));
      if (sceneBlocked(err)) {
        // The scene is not what the sheet's row shows (closed, no attempts left): read it again.
        void this.data.state.refresh();
      }
    } finally {
      this.busy.set(false);
    }
  }

  protected close(): void {
    this.sheet.close(this.roll() !== null);
  }
}
