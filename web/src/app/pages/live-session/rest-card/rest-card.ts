import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  Injector,
  afterNextRender,
  computed,
  inject,
  input,
  output,
  signal,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import { type GetRestPreviewResponse, RestKind } from '../../../../gen/meurpg/play/v1/resources_pb';
import { ActionKey } from '../../../core/connect/idempotency';
import { focusWithRing } from '../../../core/creatures/focus-ring';
import { dieName } from '../../../core/resources/hit-dice-text';
import {
  type HitDiceChoice,
  hitDiceChoices,
  restDoneText,
  restLabel,
  restSummary,
  type RechargeOf,
} from '../../../core/resources/rest-preview';
import { ResourceClient } from '../../../core/resources/resources-client';
import { restErrorMessage } from '../../../core/resources/resources-errors';
import { CountStepper } from '../../../shared/count-stepper/count-stepper';
import { HiddenSwitch } from '../../../shared/hidden-switch/hidden-switch';
import { toVitalsVm } from '../../../core/play/live-session-source.live';
import type { VitalsVm } from '../../../core/play/live-session.types';

/** The die sizes' counts of each character that chooses which hit dice come back: character, then die size. */
type DiceCounts = Readonly<Record<string, Readonly<Record<number, number>>>>;

/**
 * "Descanso" (PM-07b 9, decision of the owner): the master's two buttons, "Descanso curto" and "Descanso longo". A tap
 * asks the server what the rest would give back (`GetRestPreview`, which writes nothing) and the card asks, in place
 * (an `alertdialog`, no dialog window: the board draws it inside the card), "Começar o descanso longo?" with the
 * list computed from the preview. The focus stays on the button that was tapped, with its ring; "Descansar" takes the
 * rest (`TakeRest`, one idempotency key for the request: the same request again is a retry, the next rest a new one) and
 * "Cancelar" or Esc closes the question and gives the focus back. A long rest of a character with dice of several sizes
 * spent, and room for fewer than are spent, asks which sizes come back (a small stepper per size, at most the limit in
 * all, the server's default to start with). The new vitals go to the page (`restTaken`), and the stream sends them to
 * the other screens. Only the master's page draws this card.
 */
@Component({
  selector: 'app-rest-card',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [CountStepper, HiddenSwitch, MatButtonModule, MatIconModule],
  templateUrl: './rest-card.html',
  styleUrl: './rest-card.scss',
})
export class RestCard {
  readonly campaignId = input.required<string>();
  /** The party as the page has it: which resources also come back on a short rest. */
  readonly party = input<readonly VitalsVm[]>([]);
  /** The vitals of every character the rest touched. */
  readonly restTaken = output<readonly VitalsVm[]>();

  private readonly api = inject(ResourceClient);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);
  private readonly injector = inject(Injector);

  protected readonly kinds = [
    { kind: RestKind.SHORT, key: 'short', label: restLabel(RestKind.SHORT), icon: 'schedule' },
    { kind: RestKind.LONG, key: 'long', label: restLabel(RestKind.LONG), icon: 'bedtime' },
  ] as const;

  /** The rest being asked about; `null` while no question is open. */
  protected readonly asking = signal<RestKind | null>(null);
  private readonly preview = signal<GetRestPreviewResponse | null>(null);
  protected readonly loading = signal<RestKind | null>(null);
  protected readonly saving = signal(false);
  /** A refusal of the preview (above the buttons) or of the rest (inside the question). */
  protected readonly error = signal('');
  protected readonly done = signal('');
  private readonly counts = signal<DiceCounts>({});
  /** A long rest the table had no food or drink for: no level of exhaustion comes off (SRD 5.1, Resting). */
  protected readonly withoutFood = signal(false);

  private readonly key = new ActionKey();

  protected readonly summary = computed(() => {
    const kind = this.asking();
    const preview = this.preview();
    return kind === null || preview === null ? null : restSummary(kind, preview, this.rechargeOf);
  });

  /** Who chooses the hit dice that come back (a long rest only), with what each size may be. */
  protected readonly choices = computed(() =>
    this.asking() === RestKind.LONG && this.preview() ? hitDiceChoices(this.preview()!) : [],
  );

  protected readonly dieName = dieName;
  protected readonly long = RestKind.LONG;

  private readonly rechargeOf: RechargeOf = (characterId, key) => {
    const recharge = this.party()
      .find((v) => v.characterId === characterId)
      ?.resources?.find((r) => r.key === key)?.recharge;
    return recharge === 'short_rest' || recharge === 'long_rest' ? recharge : undefined;
  };

  /** How many of this size come back now: the master's choice, or the server's default. */
  protected countOf(characterId: string, faces: number, start: number): number {
    return this.counts()[characterId]?.[faces] ?? start;
  }

  /** The most this size can take: what is spent of it, and what the limit leaves after the other sizes. */
  protected maxOf(choice: HitDiceChoice, faces: number): number {
    const others = choice.sizes
      .filter((s) => s.faces !== faces)
      .reduce((sum, s) => sum + this.countOf(choice.characterId, s.faces, s.start), 0);
    const spent = choice.sizes.find((s) => s.faces === faces)?.spent ?? 0;
    return Math.max(0, Math.min(spent, choice.limit - others));
  }

  protected setCount(characterId: string, faces: number, value: number): void {
    this.counts.update((all) => ({
      ...all,
      [characterId]: { ...all[characterId], [faces]: value },
    }));
  }

  /** The buttons say what is happening, with the word the question will use. */
  protected pressed(kind: RestKind): boolean {
    return this.asking() === kind;
  }

  /** A tap on "Descanso curto" or "Descanso longo": the server says what comes back, then the question opens. */
  protected async ask(kind: RestKind): Promise<void> {
    if (this.loading() !== null || this.saving() || this.asking() === kind) {
      return;
    }
    this.loading.set(kind);
    this.error.set('');
    this.done.set('');
    try {
      const preview = await this.api.restPreview(this.campaignId(), kind);
      this.preview.set(preview);
      this.counts.set({});
      this.withoutFood.set(false);
      this.asking.set(kind);
      this.focusButton(kind);
    } catch (err) {
      this.asking.set(null);
      this.error.set(restErrorMessage(err));
    } finally {
      this.loading.set(null);
    }
  }

  protected cancel(): void {
    const kind = this.asking();
    if (kind === null || this.saving()) {
      return;
    }
    this.close();
    this.focusButton(kind);
  }

  /** "Descansar": takes the rest, shows the confirmation line and closes the question. */
  protected async rest(): Promise<void> {
    const kind = this.asking();
    if (kind === null || this.saving()) {
      return;
    }
    const hitDiceChoicesSent = this.choices().map((c) => ({
      characterId: c.characterId,
      dice: c.sizes.map((s) => ({
        faces: s.faces,
        count: this.countOf(c.characterId, s.faces, s.start),
      })),
    }));
    // Only a long rest has the question; a short one never takes a level of exhaustion off.
    const withoutFood = kind === RestKind.LONG && this.withoutFood();
    this.saving.set(true);
    this.error.set('');
    try {
      const vitals = await this.api.takeRest(
        this.campaignId(),
        kind,
        this.key.keyFor({ kind, hitDiceChoicesSent, withoutFood }),
        hitDiceChoicesSent,
        withoutFood,
      );
      this.key.renew();
      this.restTaken.emit(vitals.map(toVitalsVm));
      this.close();
      this.done.set(restDoneText(kind));
      this.focusButton(kind);
    } catch (err) {
      this.error.set(restErrorMessage(err));
    } finally {
      this.saving.set(false);
    }
  }

  private close(): void {
    this.asking.set(null);
    this.preview.set(null);
    this.counts.set({});
    this.withoutFood.set(false);
    this.error.set('');
  }

  private focusButton(kind: RestKind): void {
    afterNextRender(
      () =>
        focusWithRing(
          this.host.nativeElement.querySelector<HTMLElement>(
            `[data-rest="${kind === RestKind.LONG ? 'long' : 'short'}"]`,
          ),
        ),
      { injector: this.injector },
    );
  }
}
