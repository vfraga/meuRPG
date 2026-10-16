import { signal, untracked } from '@angular/core';
import type {
  GetTurnOptionsResponse,
  OpportunityOffer,
  ReactionWindow,
} from '../../../../gen/meurpg/play/v1/combat_pb';
import { reactorAttacks } from '../../../core/combat/opportunity';
import { openWindows, promptView } from '../../../core/combat/reactions';
import { openSheet } from '../../../shared/sheet/sheet-host';
import {
  EffectSaveSheet,
  type EffectSaveSheetData,
} from '../effects/effect-save-sheet/effect-save-sheet';
import { AttackSheet, type AttackSheetData } from './attack-sheet/attack-sheet';
import { throwBackAttack, THROW_BACK_KEY } from './attack-sheet/attack-sheet-data';
import {
  OpportunitySheet,
  type OpportunityAnswer,
  type OpportunitySheetData,
} from './opportunity/opportunity-sheet';
import {
  ReactionSheet,
  type ReactionSheetData,
  type ReactionSheetResult,
} from './reaction-sheet/reaction-sheet';
import type { AttackOpen, CombatSheetsContext } from './combat-sheets-context';

/**
 * The sheets that answer a question the combat asks: the reactions (Escudo, Contramágica... and the saving throw of an
 * effect), the missile the monk caught, and the opportunity attacks. It keeps what the screen must know about them: the
 * reaction sheet that is on screen (no second one opens over it) and the windows whose sheet already opened by itself.
 */
export class ReactionSheets {
  /** The reaction sheet is on screen: no second one opens over it. */
  readonly reactionOpen = signal(false);
  /** The reaction windows whose sheet already opened by itself: it opens once for each, never again after it closed. */
  readonly reactionHandled = new Set<string>();

  constructor(
    private readonly ctx: CombatSheetsContext,
    /** The attack sheet of the screen (the throw-back attack opens it). */
    private readonly openAttack: (req: AttackOpen) => void,
  ) {}

  /** "Você foi atingido: usar Escudo?" and the other reactions: opens by itself, and has to be answered. */
  reaction(window: ReactionWindow): void {
    const e = this.ctx.encounter();
    const own = this.ctx.own();
    const vitals = this.ctx.vitals().find((v) => v.characterId === own?.characterId);
    if (!e) {
      return;
    }
    if (window.prompt.case === 'effectSave') {
      this.effectSave(window);
      return;
    }
    const data: ReactionSheetData = {
      campaignId: this.ctx.campaignId(),
      encounterId: e.id,
      window,
      round: e.round,
      usage: vitals?.spellSlots ?? [],
      pact: vitals?.pactSlots ?? null,
      state: this.ctx.state(),
      armorClass: this.ctx.armorClass(),
      diceMode: this.ctx.diceMode(),
      preference: this.ctx.preference(),
    };
    this.reactionOpen.set(true);
    openSheet<ReactionSheet, ReactionSheetData, ReactionSheetResult>(
      this.ctx.dialog,
      this.ctx.bottomSheet,
      ReactionSheet,
      {
        data,
        ariaLabel: promptView(window, e.round)?.ariaLabel ?? 'Reação',
        alert: true,
      },
    ).subscribe({
      next: (result) => {
        if (result?.throwWindowId) {
          this.reactionHandled.add(result.throwWindowId);
          this.throwBack(result.throwWindowId);
        }
      },
      complete: () => this.reactionOpen.set(false),
    });
  }

  /** The saving throw an effect asks at the end or the start of this player's turn (RN-22): opens by itself, and again from "Rolar o teste". */
  effectSave(window: ReactionWindow): void {
    const e = this.ctx.encounter();
    if (!e || this.reactionOpen()) {
      return;
    }
    const data: EffectSaveSheetData = {
      campaignId: this.ctx.campaignId(),
      encounterId: e.id,
      window,
      round: e.round,
      state: this.ctx.state(),
      diceMode: this.ctx.diceMode(),
      preference: this.ctx.preference(),
    };
    this.reactionOpen.set(true);
    openSheet<EffectSaveSheet, EffectSaveSheetData, void>(
      this.ctx.dialog,
      this.ctx.bottomSheet,
      EffectSaveSheet,
      { data, ariaLabel: 'Teste de resistência do fim do turno', labelledBy: 'sheet-t' },
    ).subscribe({ complete: () => this.reactionOpen.set(false) });
  }

  /**
   * "Devolver (1 de chi)": the ranged attack with the missile the monk caught, part of the same reaction (see
   * `throwBackAttack`). Closing the sheet without the roll keeps the missile (the window is passed), so the combat never
   * waits on it.
   */
  private throwBack(windowId: string): void {
    const window = this.ctx.encounter()?.reactionWindows.find((x) => x.id === windowId);
    const thrown = throwBackAttack(window, this.ctx.options());
    if (thrown) {
      this.openAttack({
        key: THROW_BACK_KEY,
        asReaction: true,
        catchWindowId: windowId,
        thrown,
      });
    }
  }

  /** The throw-back sheet closed without a roll: the missile is kept, so the window does not hang. */
  keepMissile(windowId: string): void {
    const e = this.ctx.encounter();
    if (!e || !openWindows(e).some((w) => w.id === windowId)) {
      return;
    }
    const answer = { use: false };
    void this.ctx.api.attacks
      .answerReaction(this.ctx.campaignId(), e.id, windowId, answer, `keep-${windowId}`)
      .then((r) => this.ctx.state().apply(r.encounter))
      .catch(() => undefined);
  }

  /** The player's prompt: "O Goblin 2 está saindo do seu alcance. Ataque de oportunidade?" It reads the
   * reactor's attacks itself (its buttons are off until they are in), so an offer is never stuck on a failed read. */
  opportunity(offer: OpportunityOffer): void {
    const e = this.ctx.encounter();
    if (!e) {
      return;
    }
    const data: OpportunitySheetData = {
      campaignId: this.ctx.campaignId(),
      encounterId: e.id,
      offer,
      round: e.round,
      load: async () => {
        const options = await this.ctx.api.moves.turnOptions(
          this.ctx.campaignId(),
          e.id,
          offer.reactorId,
        );
        return {
          options,
          attacks: reactorAttacks(
            offer,
            (options.options?.attacks ?? []).flatMap((a) => (a.attack ? [a.attack] : [])),
          ),
        };
      },
      state: this.ctx.state(),
    };
    openSheet<OpportunitySheet, OpportunitySheetData, OpportunityAnswer>(
      this.ctx.dialog,
      this.ctx.bottomSheet,
      OpportunitySheet,
      {
        data,
        ariaLabel: 'Ataque de oportunidade',
        alert: true,
      },
    ).subscribe((answer) => {
      if (answer) {
        this.opportunityAttack(offer, answer.attackKey, answer.options, false);
      }
    });
  }

  /** The offer is still pending after the attack sheet closed (nobody rolled): ask again, it must be answered. */
  private rearm(offer: OpportunityOffer): void {
    const still =
      this.ctx.encounter()?.opportunityOffers.some((o) => o.id === offer.id && o.forYou) ?? false;
    if (still && !this.ctx.isMaster()) {
      untracked(() => this.opportunity(offer));
    }
  }

  /** "Atacar com <arma>": the attack sheet, with the mover as its target. */
  opportunityAttack(
    offer: OpportunityOffer,
    key: string,
    options: GetTurnOptionsResponse | null | undefined,
    byMaster: boolean,
  ): void {
    const e = this.ctx.encounter();
    const attack = options?.options?.attacks.find((a) => a.attack?.key === key)?.attack;
    if (!e || !attack) {
      return;
    }
    const data: AttackSheetData = {
      campaignId: this.ctx.campaignId(),
      encounterId: e.id,
      attackerId: offer.reactorId,
      round: e.round,
      attack,
      targets: [],
      diceMode: this.ctx.diceMode(),
      preference: this.ctx.preference(),
      state: this.ctx.state(),
      asReaction: true,
      opportunity: {
        offerId: offer.id,
        targetId: offer.moverId,
        targetLabel: offer.moverLabel,
        byMaster,
      },
    };
    openSheet<AttackSheet, AttackSheetData, boolean>(
      this.ctx.dialog,
      this.ctx.bottomSheet,
      AttackSheet,
      {
        data,
        ariaLabel: `Ataque de oportunidade com ${attack.namePt || attack.name}`,
        labelledBy: 'sheet-t',
      },
    ).subscribe(() => this.rearm(offer));
  }
}
