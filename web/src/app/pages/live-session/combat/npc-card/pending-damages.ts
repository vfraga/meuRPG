import {
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
import { CombatClient, newKey } from '../../../../core/combat/combat-client';
import {
  damageFormula,
  diceName,
  extraDiceOf,
  sumRange,
} from '../../../../core/combat/combat-dice';
import { RollAnimator, showOfDice } from '../../../../shared/roll-overlay/roll-animator';
import { criticalHint, criticalTypedHint, fixedParts } from '../../../../core/combat/critical';
import { combatErrorMessage } from '../../../../core/combat/combat-errors';
import type { CombatState } from '../../../../core/combat/combat-state';
import { hitPointsAfter, hitPointsLine } from '../../../../core/combat/attack-flow';
import { article } from '../../../../core/combat/combat-log';
import { isPlayer } from '../../../../core/combat/combat-view';
import { DamageBreakdown } from '../damage-parts/damage-breakdown';
import { heldReason } from '../../../../core/combat/reaction-master';
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
      .map((p) => this.describe(p)),
  );

  /**
   * Which part of a trap's hit a card is: "Armadilha: perfurante (2 de 4)". A trap that fires several times (darts) opens
   * one card per damage type per hit, all on the same target and trap point; the card carries no hit number, so the
   * position among the cards of the same target, point and type, in the order they were opened, stands for it.
   */
  private trapLabel(p: PendingDamage): string {
    if (!p.trapPointId) {
      return '';
    }
    const same = this.pendings().filter(
      (q) =>
        q.trapPointId === p.trapPointId &&
        q.targetId === p.targetId &&
        q.damageTypeKey === p.damageTypeKey,
    );
    const word = `${p.trapName || 'Armadilha'}: ${p.damageTypePt || 'dano'}`;
    return same.length > 1 ? `${word} (${same.indexOf(p) + 1} de ${same.length})` : word;
  }

  private describe(p: PendingDamage) {
    const e = this.encounter();
    const attacker = e.combatants.find((c) => c.id === p.attackerId);
    const target = e.combatants.find((c) => c.id === p.targetId);
    const rolled = p.status === PendingDamageStatus.ROLLED;
    // What "Aplicar" applies: the damage after resistance, vulnerability and immunity when a step changed it.
    const total = p.amountAfterSteps ?? p.amount;
    const range = sumRange(p.diceCount, p.diceSides);
    // A window the combat waits on holds the damage: "Rolar dano" is off, with the reason written (the answer is in
    // the master's cards, `ReactionQueue`).
    const held = p.status === PendingDamageStatus.AWAITING_REACTION || !rolled ? heldReason(e) : '';
    return {
      p,
      total,
      /** The extras the master may take out: the ones the player could choose, while they count. */
      removable: new Set((p.parts ?? []).filter((x) => x.choosable).map((x) => x.key)),
      rolls: p.partRolls ?? [],
      steps: p.steps ?? [],
      rolled,
      awaiting: p.status === PendingDamageStatus.AWAITING_REACTION,
      held:
        held ||
        (p.status === PendingDamageStatus.AWAITING_REACTION
          ? `Espere a reação ${article(target?.label ?? '') === 'a' ? 'da' : 'do'} ${target?.label ?? 'alvo'}.`
          : ''),
      // A player rolls the damage of their own attack; the master waits.
      waitsForPlayer: !rolled && !!attacker && isPlayer(attacker),
      trapLabel: this.trapLabel(p),
      attacker: attacker?.label ?? '',
      target: target?.label ?? '',
      dice: diceName(p.diceCount, p.diceSides),
      range,
      label:
        p.diceCount > 1
          ? `Role ${diceName(p.diceCount, p.diceSides)} para o dano: some os dois`
          : `Role ${diceName(p.diceCount, p.diceSides)} para o dano`,
      line: p.roll ? damageFormula(p.roll, p.damageTypePt, extraDiceOf(p), p.criticalMax) : '',
      // A critical hit follows the table's rule: what to roll, said the way it asks, and what the typed sum is (RN-24).
      crit: p.critical
        ? (criticalHint(p.criticalRule, p.diceCount, p.diceSides, p.criticalMax)?.line ?? '')
        : '',
      typedHint: criticalTypedHint(
        p.criticalRule,
        diceName(p.diceCount, p.diceSides),
        range.min,
        range.max,
        p.criticalMax,
      ),
      modifier: p.bonus + p.criticalMax,
      fixedText: fixedParts(p.criticalMax, p.bonus),
      effect: rolled && target ? this.effectLine(target, total) : '',
    };
  }

  /** The target's hit points before and after the damage. A druid in Wild Shape loses the beast's points first
   * ("Lobo: 11 de 11 PV, depois 4"); what the beast cannot take passes to the druid (SRD 5.1, "Wild Shape"). */
  private effectLine(target: Encounter['combatants'][number], total: number): string {
    const beastCurrent = target.wildShapeHitPointsCurrent;
    const beastMax = target.wildShapeHitPointsMax;
    if (target.wildShapeBeastKey !== '' && beastCurrent !== undefined && beastMax !== undefined) {
      const beast = target.wildShapeBeastNamePt || 'Forma animal';
      const line = hitPointsLine(
        beast,
        beastCurrent,
        beastMax,
        hitPointsAfter(beastCurrent, 0, total),
      );
      const over = total - beastCurrent;
      if (over > 0 && target.hitPointsMax !== undefined) {
        const own = target.hitPointsCurrent ?? 0;
        return `${line}; o resto (${over}) passa para ${hitPointsLine(target.label, own, target.hitPointsMax, hitPointsAfter(own, target.hitPointsTemporary ?? 0, over))}`;
      }
      return line;
    }
    return target.hitPointsMax === undefined
      ? ''
      : hitPointsLine(
          target.label,
          target.hitPointsCurrent ?? 0,
          target.hitPointsMax,
          hitPointsAfter(target.hitPointsCurrent ?? 0, target.hitPointsTemporary ?? 0, total),
        );
  }

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
      const res = await this.api.rollDamage(
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
      const res = await this.api.applyDamage(
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
      const res = await this.api.removeDamagePart(
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
      const res = await this.api.discardDamage(this.campaignId(), this.encounter().id, p.id);
      this.state().apply(res.encounter);
      this.discarding.set(null);
      this.settled.set(`Dano de ${p.amountAfterSteps ?? p.amount} descartado.`);
      this.reminder.set('');
      this.settledNote.emit();
    });
  }
}
