import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  Injector,
  afterNextRender,
  computed,
  inject,
  input,
  model,
  output,
  signal,
  viewChild,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import {
  type DamagePartRoll,
  type DamageStep,
  DamageStepKind,
} from '../../../../../gen/meurpg/play/v1/combat_rolls_pb';
import { partTotal, rerollText } from '../../../../core/combat/damage-parts';
import { reasonValid, stepLine } from '../../../../core/combat/roll-mode';

let nextId = 0;

/**
 * What each part of a rolled damage made: "Espada curta 1d8 (5) + 3 = 8", the
 * Great Weapon Fighting rerolls ("rolou de novo 1→4"), a part that does not
 * count (said in words), and the steps of resistance, vulnerability and
 * immunity ("Resistência a fogo (tiefling): 10 → 5"). The master's version is a
 * table by part: "Tirar" on an extra (a reason, 1 to 120 characters, and the
 * damage before and after) and, on each step still on, "Ignorar a resistência".
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-damage-breakdown',
  imports: [MatButtonModule, MatIconModule],
  templateUrl: './damage-breakdown.html',
  styleUrl: './damage-breakdown.scss',
})
export class DamageBreakdown {
  readonly rolls = input.required<readonly DamagePartRoll[]>();
  readonly steps = input<readonly DamageStep[]>([]);
  /** The damage before the steps, and after them when a step changed it. */
  readonly amount = input(0);
  readonly amountAfterSteps = input<number | undefined>(undefined);
  /** The master's table: a table by part with the controls. */
  readonly master = input(false);
  /** The keys of the extras the master may take out. */
  readonly removable = input<ReadonlySet<string>>(new Set());
  readonly busy = input(false);
  /** The keys of the sources the master told the app to leave out. */
  readonly ignored = model<readonly string[]>([]);

  readonly remove = output<{ partKey: string; reason: string }>();

  private readonly injector = inject(Injector);
  private readonly reasonField = viewChild('reasonField', { read: ElementRef<HTMLInputElement> });
  protected readonly id = `damage-breakdown-${nextId++}`;
  /** The part the master is about to take out. */
  protected readonly asking = signal<string | null>(null);
  protected readonly reason = signal('');
  protected readonly reasonOk = computed(() => reasonValid(this.reason()));
  protected readonly asked = computed(() => this.rolls().find((r) => r.partKey === this.asking()));
  protected readonly after = computed(() => this.amountAfterSteps());

  protected formula(r: DamagePartRoll): string {
    const dice = r.diceCount > 0 ? `${r.diceCount}d${r.diceSides}` : '';
    const faces = r.faces.length
      ? ` (${r.faces.join(', ')})`
      : r.diceCount > 0
        ? ' · dado físico'
        : '';
    const flat = r.flat === 0 ? '' : ` ${r.flat < 0 ? '−' : '+'} ${Math.abs(r.flat)}`;
    return dice ? `${dice}${faces}${flat} = ${partTotal(r)}` : `${partTotal(r)}`;
  }

  protected rerolls(r: DamagePartRoll): string {
    return rerollText(r);
  }

  protected step(s: DamageStep): string {
    return stepLine(s);
  }

  protected canIgnore(s: DamageStep): boolean {
    return this.master() && !s.ignored;
  }

  protected ignoreWord(s: DamageStep): string {
    switch (s.kind) {
      case DamageStepKind.VULNERABILITY:
        return 'Ignorar a vulnerabilidade';
      case DamageStepKind.IMMUNITY:
        return 'Ignorar a imunidade';
      case DamageStepKind.REDUCTION:
        return 'Ignorar a redução';
      default:
        return 'Ignorar a resistência';
    }
  }

  protected isIgnored(s: DamageStep): boolean {
    return s.sourceKeys.length > 0 && s.sourceKeys.every((k) => this.ignored().includes(k));
  }

  protected toggleIgnore(s: DamageStep): void {
    const on = this.isIgnored(s);
    const rest = this.ignored().filter((k) => !s.sourceKeys.includes(k));
    this.ignored.set(on ? rest : [...rest, ...s.sourceKeys]);
  }

  protected ask(r: DamagePartRoll): void {
    this.asking.set(r.partKey);
    this.reason.set('');
    afterNextRender(() => this.reasonField()?.nativeElement.focus(), { injector: this.injector });
  }

  protected question(r: DamagePartRoll): string {
    return `Tirar o ${r.labelPt}? O dano cai de ${this.amount()} para ${Math.max(0, this.amount() - partTotal(r))}.`;
  }

  protected onReason(event: Event): void {
    this.reason.set((event.target as HTMLInputElement).value);
  }

  protected confirm(): void {
    const key = this.asking();
    if (key && this.reasonOk() && !this.busy()) {
      this.remove.emit({ partKey: key, reason: this.reason().trim() });
      this.asking.set(null);
    }
  }
}
