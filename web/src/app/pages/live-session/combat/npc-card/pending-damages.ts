import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  Injector,
  afterNextRender,
  computed,
  effect,
  inject,
  input,
  output,
  signal,
  untracked,
  viewChild,
  viewChildren,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatIconModule } from '@angular/material/icon';
import { MatInputModule } from '@angular/material/input';

import {
  type Encounter,
  type PendingDamage,
  PendingDamageStatus,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { newKey } from '../../../../core/connect/idempotency';
import { RollAnimator, showOfDice } from '../../../../shared/roll-overlay/roll-animator';
import { combatErrorMessage } from '../../../../core/combat/combat-errors';
import type { CombatState } from '../../../../core/combat/combat-state';
import { article } from '../../../../core/combat/combat-log';
import { DamageBreakdown } from '../damage-parts/damage-breakdown';
import { pendingCard } from './pending-damage-card';
import { RollPicker } from '../../../../shared/roll-picker/roll-picker';

/**
 * The damage the master owes a hit (E6-11, E6-12, RN-02): "Rolar dano" for
 * an NPC's hit whose damage is not rolled yet, and, once rolled on a
 * player's character, the damage with the target's hit points ("Toren: 26
 * de 31 PV, depois 21") and "Aplicar 5 de dano" / "Não aplicar". It lists
 * every pending damage of the turn, also those a player rolled. "Aplicar"
 * is a 2px accent outline, because "Próximo turno" owns the fill. "Não
 * aplicar" asks in place ("Descartar o dano de 5?"), the focus on "Voltar".
 * The page's `CombatState` gets each answer's combat.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-pending-damages',
  imports: [
    DamageBreakdown,
    MatButtonModule,
    MatFormFieldModule,
    MatIconModule,
    MatInputModule,
    RollPicker,
  ],
  templateUrl: './pending-damages.html',
  styleUrl: './pending-damages.scss',
})
export class PendingDamages {
  private readonly api = inject(CombatClient);
  private readonly animator = inject(RollAnimator);
  private readonly injector = inject(Injector);

  readonly pendings = input.required<readonly PendingDamage[]>();
  readonly encounter = input.required<Encounter>();
  readonly campaignId = input.required<string>();
  readonly state = input.required<CombatState>();
  /** Inside the attack's own result box: no box of its own. */
  readonly flat = input(false);
  /** A damage was applied or discarded: the card stays on screen for its note. */
  readonly settledNote = output<void>();

  protected readonly busy = signal(false);
  /** The master types a physical roll: only then the critical's line is said (the app's dice are the server's). */
  protected readonly typing = signal(false);
  protected readonly error = signal('');
  protected readonly discarding = signal<string | null>(null);
  /** What was just settled, for the live region ("Dano de 5 aplicado…"). */
  protected readonly settled = signal('');
  /** "Aplicar outro valor" (E6-31): the damage whose number the master is typing, and the text. */
  protected readonly other = signal<string | null>(null);
  protected readonly otherText = signal('');
  /** The reminder after a hit on someone who concentrates (RN-22): the save DC. */
  protected readonly reminder = signal('');
  /** The sources of resistance the master told the app to leave out, by damage. */
  protected readonly ignored = signal<Readonly<Record<string, readonly string[] | undefined>>>({});
  private readonly keys = new Map<string, string>();

  /** The note of a settled damage belongs to the turn it was settled on: once the turn passes, it goes. */
  private readonly turnKey = computed(
    () => `${this.encounter().round}:${this.encounter().currentCombatantId}`,
  );

  constructor() {
    effect(() => {
      this.turnKey();
      untracked(() => {
        this.settled.set('');
        this.reminder.set('');
      });
    });
  }
  private readonly safe = viewChild('safe', { read: ElementRef<HTMLButtonElement> });
  private readonly otherInput = viewChild('otherInput', { read: ElementRef<HTMLInputElement> });
  private readonly otherLinks = viewChildren('otherLink', { read: ElementRef<HTMLButtonElement> });

  protected readonly items = computed(() =>
    this.pendings()
      .filter(
        (p) =>
          p.status === PendingDamageStatus.ROLLED ||
          p.status === PendingDamageStatus.AWAITING_ROLL ||
          p.status === PendingDamageStatus.AWAITING_REACTION,
      )
      .map((p) => pendingCard(p, this.encounter(), this.pendings())),
  );

  private keyFor(id: string): string {
    let key = this.keys.get(id);
    if (!key) {
      key = newKey();
      this.keys.set(id, key);
    }
    return key;
  }

  /** The answer came: the next request for the same damage is a new one (after "Desfazer" the same pending damage is
   * rolled or applied again), so it gets a new key. A key kept would make the server replay the old answer, whose combat
   * is older than the one on screen and is dropped: the panel would stay as it was. */
  private renewKey(name: string): void {
    this.keys.delete(name);
  }

  private async run(work: () => Promise<void>): Promise<void> {
    if (this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      await work();
    } catch (err) {
      this.error.set(combatErrorMessage(err, 'resolver o dano'));
    } finally {
      this.busy.set(false);
    }
  }

  protected rollInApp(p: PendingDamage): Promise<void> {
    return this.rolled(p, { inApp: true });
  }

  protected rollTyped(p: PendingDamage, sum: number): Promise<void> {
    return this.rolled(p, { sum });
  }

  private rolled(p: PendingDamage, die: { inApp: true } | { sum: number }): Promise<void> {
    return this.run(async () => {
      const name = `roll:${p.id}:${JSON.stringify(die)}`;
      const res = await this.api.attacks.rollDamage(
        this.campaignId(),
        this.encounter().id,
        p.id,
        die,
        this.keyFor(name),
      );
      this.renewKey(name);
      this.state().apply(res.encounter);
      // The master's own dice roll for the NPC, as the damage line on this card shows it (never for a typed sum).
      const show =
        'inApp' in die
          ? showOfDice('Dano', res.pending.roll, {
              withTotal: true,
              note: res.pending.damageTypePt || undefined,
            })
          : null;
      if (show) {
        this.animator.play(show);
      }
    });
  }

  /** The number typed for "Aplicar outro valor": a whole number from 0 to 9,999, or `null`. */
  protected readonly otherValue = computed(() => {
    const text = this.otherText().trim();
    if (!/^\d{1,4}$/.test(text)) {
      return null;
    }
    return Number(text);
  });

  protected askOther(p: PendingDamage): void {
    this.other.set(p.id);
    this.otherText.set(String(p.amountAfterSteps ?? p.amount));
    // The button that opened the field is gone: the focus goes to the number,
    // selected, so typing replaces it.
    afterNextRender(
      () => {
        const field = this.otherInput()?.nativeElement;
        field?.focus();
        field?.select();
      },
      { injector: this.injector },
    );
  }

  /** "Voltar": back to the rolled amount, with the focus on this damage's
   * "Aplicar outro valor" again. */
  protected closeOther(id: string): void {
    this.other.set(null);
    afterNextRender(
      () =>
        this.otherLinks()
          .find((b) => b.nativeElement.dataset['pending'] === id)
          ?.nativeElement.focus(),
      { injector: this.injector },
    );
  }

  protected onOther(event: Event): void {
    this.otherText.set((event.target as HTMLInputElement).value);
  }

  /** "Aplicar 5 de dano", or, with `amount`, the master's own number (RN-02). */
  protected apply(p: PendingDamage, amount?: number): Promise<void> {
    return this.run(async () => {
      const ignore = this.ignored()[p.id] ?? [];
      const name = `apply:${p.id}:${amount ?? 'rolled'}:${ignore.join(',')}`;
      const res = await this.api.attacks.applyDamage(
        this.campaignId(),
        this.encounter().id,
        p.id,
        amount,
        this.keyFor(name),
        ignore,
      );
      this.renewKey(name);
      this.state().apply(res.encounter);
      const total = p.amountAfterSteps ?? p.amount;
      const done = amount ?? total;
      const target = res.encounter.combatants.find((c) => c.id === p.targetId);
      const hp =
        target?.hitPointsMax !== undefined
          ? ` ${target.label}: ${target.hitPointsCurrent ?? 0} de ${target.hitPointsMax} PV.`
          : '';
      this.settled.set(
        amount === undefined || amount === total
          ? `Dano de ${done} aplicado.${hp}`
          : `${done} de dano ${done === 1 ? 'aplicado' : 'aplicados'} (o dado deu ${total}).${hp}`,
      );
      this.other.set(null);
      this.reminder.set(this.reminderFor(res.pending.concentrationDc, target));
      this.settledNote.emit();
    });
  }

  /** "Toren está concentrado em Bênção. Teste de Constituição, CD 10. ..." */
  private reminderFor(
    dc: number | undefined,
    target: { label: string; concentrationSpellNamePt: string } | undefined,
  ): string {
    if (dc === undefined || !target) {
      return '';
    }
    const he = article(target.label) === 'a' ? 'ela' : 'ele';
    return `${target.label} está concentrad${article(target.label) === 'a' ? 'a' : 'o'} em ${target.concentrationSpellNamePt}. Teste de Constituição, CD ${dc}. O app só lembra: se ${he} falhar, encerre a concentração em "Condições…".`;
  }

  protected setIgnored(id: string, keys: readonly string[]): void {
    this.ignored.update((all) => ({ ...all, [id]: keys }));
  }

  /** "Tirar": the extra leaves the damage, with the reason the master gave. */
  protected takeOut(p: PendingDamage, part: { partKey: string; reason: string }): Promise<void> {
    return this.run(async () => {
      const res = await this.api.attacks.removeDamagePart(
        this.campaignId(),
        this.encounter().id,
        p.id,
        part.partKey,
        part.reason,
        this.keyFor(`remove:${p.id}:${part.partKey}:${part.reason}`),
      );
      this.state().apply(res.encounter);
    });
  }

  protected askDiscard(id: string): void {
    this.discarding.set(id);
    afterNextRender(() => this.safe()?.nativeElement.focus(), { injector: this.injector });
  }

  protected discard(p: PendingDamage): Promise<void> {
    return this.run(async () => {
      const res = await this.api.attacks.discardDamage(
        this.campaignId(),
        this.encounter().id,
        p.id,
      );
      this.state().apply(res.encounter);
      this.discarding.set(null);
      this.settled.set(`Dano de ${p.amountAfterSteps ?? p.amount} descartado.`);
      this.reminder.set('');
      this.settledNote.emit();
    });
  }
}
