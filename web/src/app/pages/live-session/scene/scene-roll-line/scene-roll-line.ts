import {
  Component,
  ElementRef,
  Injector,
  afterNextRender,
  computed,
  inject,
  input,
  signal,
  viewChild,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import type { OpenSceneInfo, SceneRoll } from '../../../../../gen/meurpg/play/v1/scene_pb';
import { ActionKey } from '../../../../core/connect/idempotency';
import { joinDots } from '../../../../core/format/text';
import { SceneClient } from '../../../../core/play/scene-client';
import { sceneBlocked, sceneErrorMessage } from '../../../../core/play/scene-errors';
import type { SceneState } from '../../../../core/play/scene-state';
import {
  canGrantAttempt,
  grantLabel,
  initialOf,
  passLabel,
  rollActionTitle,
  rollAttemptLine,
  rollClock,
  sceneRollFormula,
} from '../../../../core/play/scene-view';
import { CombatantToken } from '../../../../shared/combatant-token/combatant-token';
import { formatClock } from '../../../../shared/session-time/session-time';

/**
 * One roll of the scene log, as the master reads it (E7-02, E7-05, E8-13): who
 * and when, which action (with "dado físico" when the player typed a real
 * die), the formula with its total, and, only when the action had a DC, a pill
 * under the formula: "Passou · CD 12" with a check, "Não passou · CD 13" with a
 * cross (the words always, never colour alone). Beside it, which attempt this
 * was ("Tentativa 1 de 3").
 *
 * "Dar mais uma tentativa" (MR-015, question 55) is on the last roll of a
 * character who has no attempts left at the action (and, with the DC shown, only
 * when it failed). It asks in place first, in a warm notice that takes the
 * card: "Voltar" first and focused, the one filled button the only one; then a
 * status line says what was done. There is no undo: the attempt is spent by
 * rolling. The key is kept per (action, character) until the grant works, so a tap
 * sent again, even after "Voltar", never gives two.
 */
@Component({
  selector: 'app-scene-roll-line',
  imports: [CombatantToken, MatButtonModule, MatIconModule],
  templateUrl: './scene-roll-line.html',
  styleUrl: './scene-roll-line.scss',
  host: { class: 'rl', '[class.rl--asking]': "step() === 'asking'" },
})
export class SceneRollLine {
  private readonly api = inject(SceneClient);
  private readonly injector = inject(Injector);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);

  readonly campaignId = input.required<string>();
  readonly state = input.required<SceneState>();
  readonly scene = input.required<OpenSceneInfo>();
  readonly roll = input.required<SceneRoll>();

  protected readonly initial = computed(() => initialOf(this.roll().characterName));
  protected readonly clock = computed(() => rollClock(this.roll()));
  protected readonly actionTitle = computed(() => rollActionTitle(this.scene(), this.roll()));
  protected readonly action = computed(() =>
    this.roll().roll?.physical ? joinDots([this.actionTitle(), 'dado físico']) : this.actionTitle(),
  );
  protected readonly formula = computed(() => sceneRollFormula(this.roll()));
  protected readonly pass = computed(() => passLabel(this.scene(), this.roll()));
  protected readonly attempt = computed(() => rollAttemptLine(this.scene(), this.roll()));
  protected readonly canGrant = computed(() => canGrantAttempt(this.scene(), this.roll()));
  protected readonly grantAria = computed(() => grantLabel(this.scene(), this.roll()));

  protected readonly step = signal<'idle' | 'asking'>('idle');
  protected readonly busy = signal(false);
  protected readonly error = signal('');
  /** The status line after a grant: when it was given and how many the player has now. */
  protected readonly granted = signal<{ at: string; total: number; used: number } | null>(null);
  /** How many attempts the character has now, for the question: "de 1 para 2". */
  protected readonly attemptsNow = computed(
    () =>
      this.scene().rolls.filter(
        (r) => r.actionId === this.roll().actionId && r.characterId === this.roll().characterId,
      ).length,
  );

  private readonly back = viewChild('back', { read: ElementRef<HTMLButtonElement> });
  private readonly opener = viewChild('opener', { read: ElementRef<HTMLButtonElement> });
  private readonly status = viewChild('status', { read: ElementRef<HTMLElement> });
  /** One key per (action, character) grant, kept until it works: a question closed with "Voltar" and asked again
   * after a lost answer is the same grant, so the server does not give two. */
  private readonly key = new ActionKey();

  protected ask(): void {
    this.error.set('');
    this.step.set('asking');
    this.focusAfterRender(() => this.back());
    this.scrollIntoView();
  }

  protected cancel(): void {
    this.step.set('idle');
    this.focusAfterRender(() => this.opener());
  }

  protected async grant(): Promise<void> {
    if (this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    const roll = this.roll();
    const before = this.attemptsNow();
    try {
      const scene = await this.api.grantAttempt(
        this.campaignId(),
        roll.actionId,
        roll.characterId,
        this.key.keyFor({ actionId: roll.actionId, characterId: roll.characterId }),
      );
      this.key.renew();
      this.granted.set({ at: formatClock(new Date()), total: before + 1, used: before });
      this.step.set('idle');
      this.state().apply(scene);
      this.focusAfterRender(() => this.status());
    } catch (err) {
      this.error.set(sceneErrorMessage(err, 'dar a tentativa'));
      if (sceneBlocked(err)) {
        void this.state().refresh();
      }
    } finally {
      this.busy.set(false);
    }
  }

  private focusAfterRender(target: () => ElementRef<HTMLElement> | undefined): void {
    afterNextRender(() => target()?.nativeElement.focus({ preventScroll: true }), {
      injector: this.injector,
    });
  }

  /** The whole question, below the sticky app bar (`scroll-margin-top`). */
  private scrollIntoView(): void {
    afterNextRender(() => this.host.nativeElement.scrollIntoView({ block: 'nearest' }), {
      injector: this.injector,
    });
  }
}
