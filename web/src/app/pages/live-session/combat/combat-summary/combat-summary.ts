import {
  Component,
  ElementRef,
  computed,
  input,
  output,
  signal,
  viewChild,
  afterNextRender,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import { CombatantState } from '../../../../../gen/meurpg/play/v1/combat_pb';
import type { Combatant, Encounter } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { combatantInitial, isPlayer, stateWord } from '../../../../core/combat/combat-view';
import { isCreature } from '../../../../core/combat/creature-names';
import { formatXp } from '../../../../core/format/text';
import { CombatantToken } from '../../../../shared/combatant-token/combatant-token';
import type { VitalsVm } from '../../../../core/play/live-session.types';
import type { CombatantInfo } from '../combat-info';
import { CombatHighlights } from '../combat-highlights/combat-highlights';
import { CombatXp, type CombatXpState } from '../combat-xp/combat-xp';

/** One player of "O grupo agora": the numbers as the master sees them. */
interface GroupRow {
  readonly c: Combatant;
  readonly hp: { current: number; max: number } | null;
  readonly slots: string;
  readonly down: string;
}

/**
 * "Combate encerrado" (E6-16): what the combat came to. The master reads the
 * rounds, who fell and how the party stands now (the live numbers, which the
 * combat never touches: they stay as they are, and "Ajustar" corrects them);
 * a player reads the same, without hit points. The master's "Destaques do combate"
 * (MR-032) sit between the tiles and the lists below. "Voltar à sessão" leaves the
 * summary for the session page; it is the page's one filled button.
 */
@Component({
  selector: 'app-combat-summary',
  imports: [CombatantToken, CombatHighlights, CombatXp, MatButtonModule, MatIconModule],
  templateUrl: './combat-summary.html',
  styleUrl: './combat-summary.scss',
})
export class CombatSummary {
  readonly encounter = input.required<Encounter>();
  readonly isMaster = input(false);
  /** The campaign, for the master's "Experiência do combate". */
  readonly campaignId = input('');
  readonly sessionNumber = input(0);
  /** The heading is kept for a screen reader and for focus, but not drawn (a card above says the same). */
  readonly quietTitle = input(false);
  readonly info = input<ReadonlyMap<string, CombatantInfo>>(new Map());
  /** The party's live numbers (the master's `vitals`). */
  readonly vitals = input<readonly VitalsVm[]>([]);

  readonly leave = output<void>();

  /** Where the combat's XP stands (master only). While it is still to give, "Voltar à
   * sessão" is outlined and the block's button is the screen's one filled one; it
   * turns filled when the XP is given or left for later, or when there is no XP block. */
  protected readonly xpState = signal<CombatXpState>('loading');
  protected readonly xpBlock = computed(() => this.isMaster() && this.xpState() !== 'none');
  protected readonly leaveFilled = computed(
    () => !this.isMaster() || !['loading', 'open'].includes(this.xpState()),
  );

  private readonly title = viewChild.required<ElementRef<HTMLElement>>('title');

  protected readonly players = computed(() => this.encounter().combatants.filter(isPlayer));
  /** The enemies: a character's creature (a familiar, a summon) is the party's, not an enemy. */
  protected readonly npcs = computed(() =>
    this.encounter().combatants.filter((c) => !isPlayer(c) && !isCreature(c)),
  );
  protected readonly defeated = computed(() => this.npcs().filter((c) => c.defeated));
  /** The player characters that died: a dead combatant has no hit points to read, so the state says it. */
  protected readonly dead = computed(() =>
    this.players().filter((c) => c.state === CombatantState.DEAD || c.defeated),
  );
  protected readonly standing = computed(
    () =>
      this.players().filter((c) => !this.dead().includes(c) && (c.hitPointsCurrent ?? 1) > 0)
        .length,
  );
  protected readonly group = computed<GroupRow[]>(() =>
    this.players().map((c) => {
      const v = this.vitals().find((x) => x.characterId === c.characterId);
      const slots = (v?.spellSlots ?? [])
        .map((s) =>
          `${s.level}º nível: ${s.total - s.used} livres de ${s.total}`.replace(
            /(\d) livres de (\d)/,
            '$1\u00a0livres\u00a0de\u00a0$2',
          ),
        )
        .join(' · ');
      // A dead character needs no healing: it is not "caída" either.
      const dead = c.state === CombatantState.DEAD;
      const down =
        v && v.hitPointsCurrent === 0 && !dead
          ? `Caída: ${c.deathSuccesses} ${c.deathSuccesses === 1 ? 'sucesso' : 'sucessos'}, ${c.deathFailures} ${c.deathFailures === 1 ? 'falha' : 'falhas'}. Precisa de cura.`
          : '';
      return {
        c,
        hp: v ? { current: v.hitPointsCurrent, max: v.hitPointsMax } : null,
        slots,
        down,
      };
    }),
  );
  protected readonly downNotice = computed(() => this.group().filter((g) => g.down));
  protected readonly rounds = computed(() => Math.max(1, this.encounter().round));

  constructor() {
    // The summary replaces the combat, so the screen reader starts at it.
    afterNextRender(() => this.title().nativeElement.focus());
  }

  protected xp(c: Combatant): string {
    return formatXp(c.xpValue);
  }

  protected deadWord(c: Combatant): string {
    return stateWord(CombatantState.DEAD, c.label);
  }

  protected creature(c: Combatant): boolean {
    return isCreature(c);
  }

  protected initial(c: Combatant): string {
    return combatantInitial(c.label);
  }

  protected sub(c: Combatant): string {
    const info = this.info().get(c.characterId);
    return isPlayer(c) ? (info?.classSummary ?? '') : (info?.kindLabel ?? 'NPC');
  }

  protected percent(g: GroupRow): number {
    return g.hp && g.hp.max > 0 ? Math.round((g.hp.current / g.hp.max) * 100) : 0;
  }
}
