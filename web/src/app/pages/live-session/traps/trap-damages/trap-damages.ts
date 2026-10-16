import {
  Component,
  ElementRef,
  Injector,
  afterNextRender,
  computed,
  inject,
  input,
  output,
  signal,
  viewChild,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import { PendingDamageStatus } from '../../../../../gen/meurpg/play/v1/combat_pb';
import type { TrapDamage } from '../../../../../gen/meurpg/play/v1/traps_pb';
import { focusWithRing } from '../../../../core/creatures/focus-ring';
import { hitPointsAfter } from '../../../../core/combat/attack-flow';
import { CombatClient } from '../../../../core/combat/combat-client';
import type { CombatState } from '../../../../core/combat/combat-state';
import { damageFormula } from '../../../../core/combat/combat-dice';
import { newKey } from '../../../../core/connect/idempotency';
import { trapErrorMessage } from '../../../../core/traps/trap-errors';
import { TrapsClient } from '../../../../core/traps/traps-client';
import { joinDots } from '../../../../core/format/text';
import type { VitalsVm } from '../../../../core/play/live-session.types';

/** A number typed for the damage to apply: a whole number from 0 to 1000 (what `ApplyTrapDamage` takes), or `null`. */
export function parseAmount(text: string): number | null {
  const t = text.trim();
  if (!/^\d{1,4}$/.test(t)) {
    return null;
  }
  const n = Number(t);
  return n <= 1000 ? n : null;
}

/**
 * The damage a fired trap did to a player's character, waiting for the master (E9-08 4, MR-035, RN-02): it
 * appears on its own (a polite live region says who and how much), in a combat or outside one. The number
 * is editable ("Mude o número se houver resistência: o app não calcula."), the card shows the character's
 * hit points before and after, "Aplicar N de dano" is the one filled button and "Não aplicar" asks in place.
 * In a combat the damage is a pending one, applied with the combat's own call (the combat on screen reads the
 * answer); outside one, with `ApplyTrapDamage` (the character's new vitals arrive on the stream).
 */
@Component({
  selector: 'app-trap-damages',
  imports: [MatButtonModule, MatIconModule],
  templateUrl: './trap-damages.html',
  styleUrl: './trap-damages.scss',
})
export class TrapDamages {
  private readonly traps = inject(TrapsClient);
  private readonly combatApi = inject(CombatClient);
  private readonly injector = inject(Injector);

  readonly damages = input.required<readonly TrapDamage[]>();
  readonly campaignId = input.required<string>();
  readonly vitals = input<readonly VitalsVm[]>([]);
  /** The combat on screen, which reads the answer of an apply made in it. */
  readonly combat = input<CombatState | null>(null);
  /** An apply or a discard went through: the board reads again. */
  readonly settled = output<string>();

  protected readonly waiting = computed(() =>
    this.damages()
      .filter((d) => d.status === PendingDamageStatus.ROLLED)
      .map((d) => this.describe(d)),
  );
  protected readonly typed = signal<ReadonlyMap<string, string>>(new Map());
  protected readonly discarding = signal<string | null>(null);
  protected readonly busy = signal(false);
  protected readonly error = signal('');
  protected readonly done = signal('');
  private readonly keys = new Map<string, string>();
  private readonly safe = viewChild('safe', { read: ElementRef<HTMLButtonElement> });

  /** What a screen reader hears when a damage appears. */
  protected readonly announcement = computed(() =>
    this.waiting()
      .map((w) => `${w.title}: ${w.formula}, esperando você aplicar.`)
      .join(' '),
  );

  private describe(d: TrapDamage) {
    const vitals = this.vitals().find((v) => v.characterId === d.characterId);
    const text = this.typed().get(d.id) ?? String(d.amount);
    const amount = parseAmount(text);
    const typeWord = d.damageTypePt ? `de ${d.damageTypePt}` : '';
    return {
      d,
      title: `${d.trapName || 'A armadilha'} pegou ${d.characterName || 'um personagem'}`,
      formula: d.roll ? damageFormula(d.roll, d.damageTypePt) : `${d.amount} de dano`,
      sub: joinDots(
        [d.half ? 'metade, o teste passou' : '', 'esperando você aplicar'].filter(Boolean),
      ),
      label: d.damageTypePt ? `Dano ${typeWord} a aplicar` : 'Dano a aplicar',
      text,
      amount,
      hp: vitals
        ? {
            name: d.characterName,
            now: vitals.hitPointsCurrent,
            max: vitals.hitPointsMax,
            after:
              amount === null
                ? vitals.hitPointsCurrent
                : hitPointsAfter(vitals.hitPointsCurrent, vitals.hitPointsTemporary, amount),
          }
        : null,
    };
  }

  protected type(id: string, event: Event): void {
    const value = (event.target as HTMLInputElement).value;
    this.typed.update((m) => new Map(m).set(id, value));
  }

  private keyFor(scope: string, id: string): string {
    const k = `${scope}:${id}`;
    let key = this.keys.get(k);
    if (!key) {
      key = newKey();
      this.keys.set(k, key);
    }
    return key;
  }

  protected askDiscard(id: string): void {
    this.discarding.set(id);
    afterNextRender(() => focusWithRing(this.safe()?.nativeElement), { injector: this.injector });
  }

  /** "Aplicar N de dano": the typed number when it is not the rolled one. */
  protected async apply(d: TrapDamage, amount: number | null): Promise<void> {
    if (amount === null || this.busy()) {
      return;
    }
    const changed = amount !== d.amount ? amount : undefined;
    await this.run(async () => {
      if (d.encounterId) {
        const res = await this.combatApi.attacks.applyDamage(
          this.campaignId(),
          d.encounterId,
          d.id,
          changed,
          this.keyFor(`apply-${amount}`, d.id),
        );
        this.combat()?.apply(res.encounter);
      } else {
        await this.traps.applyDamage(
          this.campaignId(),
          d.id,
          changed,
          this.keyFor(`apply-${amount}`, d.id),
        );
      }
      this.done.set(`Dano de ${amount} aplicado a ${d.characterName}.`);
      this.settled.emit(d.id);
    });
  }

  protected async discard(d: TrapDamage): Promise<void> {
    await this.run(async () => {
      if (d.encounterId) {
        const res = await this.combatApi.attacks.discardDamage(
          this.campaignId(),
          d.encounterId,
          d.id,
        );
        this.combat()?.apply(res.encounter);
      } else {
        await this.traps.discardDamage(this.campaignId(), d.id, this.keyFor('discard', d.id));
      }
      this.discarding.set(null);
      this.done.set(`Dano de ${d.amount} descartado.`);
      this.settled.emit(d.id);
    });
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
      this.error.set(trapErrorMessage(err, 'resolver o dano'));
    } finally {
      this.busy.set(false);
    }
  }
}
