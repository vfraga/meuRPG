# Conversa on a phone: new-lines notice (harness captures)

**These are HARNESS CAPTURES, not the live stack.** The real `UnreadNotice`, `SessionTabs` and `ConversationSlot` (commit `41f9a72b9`, branch `fix/conversa-phone-badge`) were rendered in Chromium inside a throwaway page that stands in for the session page: a header bar, the page title, a placeholder "Sua vez" card with three buttons and a placeholder map, laid out in the same order as `live-session.html` (notice first in the main column, tabs last). Unread comes from a fake signal (3 or 0). Nothing ran against the server or the stream, and the board is not the real board. Fonts are the app's self-hosted ones.

Files: `<unread-3|unread-0>-<width>-<light|dark>.png`, viewport 390x844 and 768x844 at device scale 2.

Measured in Chromium (390 px, both themes):

- Notice box: top 131 px, height 46 px (a 44 px button inside a 1 px border), bottom 177 px: inside the first 844 px.
- With 3 unread the first action button is at y 238 px; with the notice removed from the same page it is at 180 px, which is also its position with 0 unread (the notice is not drawn at 0, so nothing moves). With news the actions sit 58 px lower (46 px row + 12 px gap).
- 768 px: the notice and the tabs are phone-only (under 768 px), so the page is identical with 3 and with 0 unread (first action at y 182 px in both).
