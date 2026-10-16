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
  viewChild,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { timestampDate } from '@bufbuild/protobuf/wkt';

import { XpMode } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import type { DiceOption } from '../../../core/campaigns/dice-labels';
import {
  TableRulesClient,
  XP_MODE_WORDS,
  tableRulesError,
  xpModeBlocked,
  type XpModeBlocked,
} from '../../../core/campaigns/table-rules';
import { formatXp } from '../../../core/format/text';
import { DiceChoice } from '../../../shared/dice-choice/dice-choice';
import { focusWithRing } from '../../../core/creatures/focus-ring';
import { MapAsk } from '../../../shared/map-ask/map-ask';

const OPTIONS: readonly DiceOption<XpMode>[] = [
  { value: XpMode.ENEMIES, title: 'Por inimigos', description: 'Cada inimigo derrotado dá XP.' },
  {
    value: XpMode.MILESTONES,
    title: 'Por marcos',
    description: 'O nível sobe quando você marca um marco.',
  },
  {
    value: XpMode.GOLD,
    title: 'Por ouro',
    description: 'O ouro encontrado vira XP em “Voltar à cidade”.',
  },
];

const WHEN = new Intl.DateTimeFormat('pt-BR', {
  day: '2-digit',
  month: '2-digit',
  year: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
});

/**
 * "Experiência" on "Regras da mesa" (RN-09, E10-03 state 3): the three ways the campaign awards XP, changed after the
 * campaign was made. The change has its own flow, apart from "Salvar regras": a switch with nothing awarded yet is made at
 * once; with XP already given the server answers `XpModeChangeBlocked`, and the question opens in place ("Mudar para
 * “por marcos”?", "Voltar" and the one filled button), with the focus on its title. What was awarded stays as it is.
 *
 * Choosing a card only picks it: the change is made by "Mudar para …" (arrow keys moving through the cards never
 * change the campaign). The button is `aria-disabled`, not `disabled`, so the focus never drops to the page.
 */
@Component({
  selector: 'app-xp-mode-panel',
  imports: [DiceChoice, MapAsk, MatButtonModule, MatIconModule],
  templateUrl: './xp-mode-panel.html',
  styleUrl: './xp-mode-panel.scss',
})
export class XpModePanel {
  private readonly client = inject(TableRulesClient);
  private readonly injector = inject(Injector);

  readonly campaignId = input.required<string>();
  /** The mode the campaign has now. */
  readonly mode = input.required<XpMode>();
  /** Whether the question is open: the page mutes its own filled button meanwhile, so one filled button shows. */
  readonly questionOpen = output<boolean>();

  protected readonly options = OPTIONS;
  protected readonly current = signal<XpMode | null>(null);
  /** The card the master chose and has not applied yet. */
  protected readonly picked = signal<XpMode | null>(null);
  protected readonly asking = signal<{ target: XpMode; blocked: XpModeBlocked } | null>(null);
  protected readonly busy = signal(false);
  protected readonly error = signal('');
  protected readonly changedLine = signal('');
  private readonly box = viewChild<ElementRef<HTMLElement>>('box');
  private readonly applyButton = viewChild('applyBtn', { read: ElementRef });

  private readonly inForce = computed(() => this.current() ?? this.mode());
  /** The card on: what is picked, else the mode in force. */
  protected readonly shown = computed(() => this.picked() ?? this.inForce());
  protected readonly changes = computed(() => this.shown() !== this.inForce());
  protected readonly word = (m: XpMode): string => XP_MODE_WORDS[m] ?? '';
  protected readonly xp = formatXp;

  constructor() {
    effect(() => this.questionOpen.emit(this.asking() !== null));
  }

  protected pick(target: XpMode): void {
    this.picked.set(target === this.inForce() ? null : target);
    this.error.set('');
    this.changedLine.set('');
  }

  /** "Mudar para …": the one action that changes the mode. */
  protected async apply(): Promise<void> {
    const target = this.picked();
    if (target === null || this.busy() || this.asking()) {
      return;
    }
    this.error.set('');
    await this.send(target, false);
  }

  protected async confirm(): Promise<void> {
    const a = this.asking();
    if (a && !this.busy()) {
      await this.send(a.target, true);
    }
  }

  protected cancel(): void {
    this.asking.set(null);
    this.picked.set(null);
    this.error.set('');
    afterNextRender(() => focusWithRing(this.applyButton()?.nativeElement), {
      injector: this.injector,
    });
  }

  private async send(target: XpMode, confirm: boolean): Promise<void> {
    this.busy.set(true);
    try {
      const res = await this.client.setXpMode(this.campaignId(), target, confirm);
      this.current.set(res.xpMode);
      this.picked.set(null);
      this.asking.set(null);
      const at = res.changedAt ? WHEN.format(timestampDate(res.changedAt)).replace(',', ' às') : '';
      this.changedLine.set(
        `Modo de XP mudado para ${XP_MODE_WORDS[res.xpMode]}${at ? `, em ${at}` : ''}. Vale daqui para frente.`,
      );
    } catch (err) {
      const blocked = xpModeBlocked(err);
      if (blocked && !confirm) {
        this.asking.set({ target, blocked });
        // The whole question in view, above the page's sticky bar (the page keeps room for it in `scroll-padding-bottom`).
        afterNextRender(() => this.box()?.nativeElement.scrollIntoView?.({ block: 'nearest' }), {
          injector: this.injector,
        });
      } else {
        this.error.set(tableRulesError(err, 'mudar o modo de XP'));
      }
    } finally {
      this.busy.set(false);
    }
  }
}
