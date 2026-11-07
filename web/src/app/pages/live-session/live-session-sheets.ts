import { computed } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';

import { DiceMode } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';

import { focusWithRing } from '../../core/creatures/focus-ring';
import { applyVitals, partyRowSub } from '../../core/play/vitals';
import type { VitalsVm } from '../../core/play/live-session.types';
import { openNotesSheet } from '../../shared/notes/notes-sheet';
import { AdjustVitals } from './adjust-vitals/adjust-vitals';
import type { AdjustVitalsData, AdjustVitalsResult } from './adjust-vitals/adjust-vitals.types';
import { DiceDialog, type DiceDialogData } from './combat/dice-dialog';
import { openRevivifySheet } from './combat/revivify-sheet/revivify-sheet';
import type { LiveSessionLoader } from './live-session-loader';
import type { LiveSessionParts, LiveSessionServices } from './live-session-deps';
import type { LiveSessionState } from './live-session-state';
import type { LiveSessionView } from './live-session-view';
import { openFreeDice } from './free-dice/free-dice-sheet';
import { openTableScreenSheet } from './table-screen-sheet/table-screen-sheet';
import { openTrapSearch } from './traps/trap-search-sheet/trap-search-sheet';

/** The sheets and dialogs the session page opens: each one answers back into the page's state. */
export class LiveSessionSheets {
  constructor(
    private readonly services: LiveSessionServices,
    private readonly state: LiveSessionState,
    private readonly parts: LiveSessionParts,
    private readonly view: LiveSessionView,
    private readonly loader: LiveSessionLoader,
  ) {}

  /** "Revivificar" outside a combat: the sheet, for the character that has the spell ready. */
  openRevivify(): void {
    const { state, services } = this;
    const own = state.ownVitals();
    if (!own) {
      return;
    }
    openRevivifySheet(services.dialog, services.bottomSheet, {
      campaignId: state.campaignId(),
      casterName: own.name,
      classes: state.playerSheet()?.classes ?? '',
      combat: null,
      casterCharacterId: own.characterId,
      reload: state.revivifyTick,
    }).subscribe();
  }

  /** "Mudar" on the initiative screen: the player's dice choice (RN-18). */
  changeDice(): void {
    const { state } = this;
    const campaign = state.campaign();
    if (!campaign) {
      return;
    }
    this.services.dialog
      .open<DiceDialog, DiceDialogData, boolean>(DiceDialog, {
        data: {
          campaignId: state.campaignId(),
          campaignName: campaign.name,
          mode: campaign.diceMode,
          preference: campaign.dicePreference,
        },
        width: '560px',
        maxWidth: 'calc(100vw - 32px)',
        autoFocus: 'first-heading',
      })
      .afterClosed()
      .subscribe(() => void this.loader.refreshCampaign());
  }

  /** "Procurar armadilhas": the player's search sheet (E9-08). */
  searchTraps(): void {
    const { state, services, parts } = this;
    const campaign = state.campaign();
    if (!campaign) {
      return;
    }
    const opener = services.document.activeElement as HTMLElement | null;
    openTrapSearch(services.dialog, services.bottomSheet, {
      campaignId: state.campaignId(),
      skills: this.view.trapSkills(),
      diceMode: campaign.diceMode,
      preference: campaign.dicePreference,
      state: parts.mapState,
      inCombat: false,
    })
      .pipe(takeUntilDestroyed(services.destroyRef))
      .subscribe(() => {
        focusWithRing(opener);
        void parts.trapBoard.refreshActivity();
      });
  }

  /** "Dados" in the header: the free dice roller, a bottom sheet on a phone and a dialog from a tablet up (MR-052). */
  openDice(): void {
    const { state, services, parts } = this;
    const opener = services.document.activeElement as HTMLElement | null;
    openFreeDice(services.dialog, services.bottomSheet, {
      state: parts.freeDice,
      isMaster: state.isMaster(),
      // Without the stream there is no table to roll for: the roll waits for it.
      online: computed(
        () => state.connection() !== 'reconnecting' && state.connection() !== 'paused',
      ),
      // RN-18 applies to the game's rolls: the sheet only says the app rolls this one.
      physical: state.campaign()?.diceMode === DiceMode.PHYSICAL,
    })
      .pipe(takeUntilDestroyed(services.destroyRef))
      .subscribe(() => focusWithRing(opener));
  }

  /** "Tela da mesa": the master's panel of the TV tab, from the bar's button or the phone's menu. */
  openTableScreen(): void {
    const { state, services, parts } = this;
    const opener = services.document.activeElement as HTMLElement | null;
    openTableScreenSheet(services.dialog, services.bottomSheet, {
      panel: parts.tableScreen,
      campaignId: state.campaignId(),
      names: computed(() => ({
        map: parts.mapState.map()?.name ?? '',
        image: state.shownImage()?.name ?? '',
        scene: parts.scene.scene()?.name ?? '',
      })),
    })
      .pipe(takeUntilDestroyed(services.destroyRef))
      .subscribe(() => focusWithRing(opener));
  }

  /** "Anotações": the bar's button, or the notice's "Abrir anotações". */
  openNotes(): void {
    const { services, parts } = this;
    openNotesSheet(services.dialog, services.bottomSheet, {
      state: parts.notes,
      openScene: () => parts.scene.scene()?.pointId ?? '',
    })
      .pipe(takeUntilDestroyed(services.destroyRef))
      .subscribe(() => {
        // Drawn twice (phone and wide): the one on screen takes the focus.
        const bars = Array.from(services.document.querySelectorAll<HTMLElement>('.notes-bar'));
        bars.find((b) => b.offsetParent !== null)?.focus();
      });
  }

  /** "Ajustar": a bottom sheet on a phone, a dialog from a tablet up, with
   * the same content (README-A). */
  adjust(vitals: VitalsVm): void {
    const { state, services } = this;
    const info = state.partyInfo().get(vitals.characterId);
    const data: AdjustVitalsData = {
      campaignId: state.campaignId(),
      vitals,
      sub: partyRowSub(info),
      playerName: info?.playerName ?? null,
      onStale: () => void this.loader.rereadVitals(),
    };
    const onPhone =
      services.document.defaultView?.matchMedia('(max-width: 767.98px)').matches ?? false;
    const closed = onPhone
      ? services.bottomSheet
          .open<AdjustVitals, AdjustVitalsData, AdjustVitalsResult>(AdjustVitals, {
            data,
            injector: services.injector,
            ariaLabel: `Ajustar ${vitals.name}`,
            // The sheet scrolls inside itself, so its container may use almost the whole phone screen.
            panelClass: 'mr-sheet',
            // The title first, so a stray Enter can't take 5 HP.
            autoFocus: 'first-heading',
          })
          .afterDismissed()
      : services.dialog
          .open<AdjustVitals, AdjustVitalsData, AdjustVitalsResult>(AdjustVitals, {
            data,
            injector: services.injector,
            width: '440px',
            maxWidth: 'calc(100vw - 32px)',
            ariaLabelledBy: 'adjust-title',
            autoFocus: 'first-heading',
          })
          .afterClosed();
    closed.pipe(takeUntilDestroyed(services.destroyRef)).subscribe((result) => {
      if (result?.kind === 'saved') {
        state.vitals.update((list) => applyVitals(list, result.vitals));
      } else if (result?.kind === 'ended') {
        this.loader.ended();
      }
    });
  }
}
