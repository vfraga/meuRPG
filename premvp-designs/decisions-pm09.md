# Decisions on the PM-09 review (09/10 00:15), binding for the fix round and for G1/G2

Source: `/tmp/pm-review4/review.md`. Verified against the code: the invite keeps its token in the fragment
(`/invite#t=`, `history.replaceState`, the token in the POST body, `intent=campaign_invite` through `POST /auth/login`;
`docs/architecture.md` "The invite flow"). The claim link follows the same pattern.

## Security and privacy

1. **The link is `/claim#t=<token>`**, never a path or query. The page reads the fragment, calls `replaceState` at once,
   and sends the token only in POST bodies. Remove "a rota é mascarada" from the board; say "o código fica depois do #,
   que o navegador não envia ao servidor".
2. **Signing in never claims.** "Entrar com Google" goes through `POST /auth/login` with `intent=character_claim`; the
   intent only brings the person back to the confirmation card. Only "Assumir este personagem" claims.
3. **Signed out, the page is identical for every link**, valid or not: what a claim link is, and "Entrar com Google". No
   campaign name, no character. The card appears only after sign-in.
4. **Forwarded links.**
   - The signed-in card shows "Enviado por <nome do mestre>" and "Não é você? Entrar com outra conta".
   - The master can undo a claim: **"Devolver à reserva"** on a claimed character's row (danger-outline confirmation in
     place). The character goes back to reserved, ownerless; the person stays a member, and the existing "remover da
     campanha" handles membership.
   - No e-mail binding (the master may not know each player's Google e-mail; single use, 7-day expiry, "usado por" and
     "Devolver à reserva" cover it).
5. **Edge cases, each drawn as a small note or a row state:**
   - The master opens their own link: "Este link é para um jogador. Copie e envie para ele." (the master owns it, nothing
     leaks).
   - A person with a pending join request: the claim makes them a member and closes the request.
   - A player whose only character is dead may claim (RN-03 counts living characters).
   - A second claim of a used link, a claim racing a revoke, an expired link: the generic page.
   - The RN-03 refusal stays the one specific error.
6. **Token** (server notes, "novo no servidor"): 32 random bytes, SHA-256 stored, like the invite; one active link per
   reserved character (a new one revokes the old); rows of expired or used links deleted after 30 days (row TTL); the
   claim RPC rate-limited per user and per IP; never logged.
7. **What the package never carries** (complete the list): the players' private notes, the session history and logs,
   accounts (ids, e-mails, names), owners and members, invites and claim links, per-player fog memory. The master's own
   content goes whole (document, master notes, gallery). Images go through the normal pipeline on import (re-encoded, so
   metadata is dropped). `docs/privacy.md` (and the PT-BR twin) gain the package; that is G1's job, note it.
8. **The import's attack surface** (server notes):
   - package at most 200 MiB, at most 2,000 entries, no entry over 10 MiB, compression ratio capped; nothing is ever
     extracted to a path (entries map to ids), so zip-slip cannot happen;
   - a strict manifest (unknown fields refused) with the format version and the SHA-256 of every entry;
   - the upload is bound to the master's account, one at a time per person, parts kept 1 hour, then deleted; "Tentar de
     novo" reuses the parts already sent;
   - the campaign caps apply: `MAX_CAMPAIGNS_PER_USER`, `CAMPAIGN_CREATORS`, the gallery limit; draw the "Você já tem 10
     campanhas" refusal.
9. **All or nothing, really.** Any refused item blocks "Criar campanha"; the preview lists each problem with its reason and
   offers only "Escolher outro arquivo". No silent drops, no dangling references. Remove "2 itens foram recusados e não
   entram".
10. **The export file:** kept 24 hours, then deleted, and also deleted with the campaign or the account; the download
    re-checks that the person is the master, with `Content-Disposition: attachment`, `Cache-Control: no-store` and a
    sanitised file name. The export page shows the last finished export with "Baixar de novo" until it expires.
11. **RN-10:** a reserved character is invisible to players everywhere: the character list, the party view, the combat
    order, map tokens, the session summary, the portrait image. Add this list to the board's note; G2 adds a leak-test row.
12. **Reserved rows** get "Editar" (the editor in master mode) and "Excluir" (danger-outline confirmation; it revokes the
    live link).

## Layout and copy

- The validity chooser: "Válido por 1 dia / 7 dias / 30 dias", default 7.
- The link field is read-only and shows the whole link, wrapping. "Copiar link" uses the copy icon (`content_copy`).
- "Revogar este link" moves away from the close button, with a danger-outline confirmation in place, on the SAME
  character as the dialog. "Pronto" becomes "Fechar".
- The dialog's cards align to the content edge.
- Add 320 px frames of the state 3 row, the dialog and the claim card.
- State 3: label the editor as "o editor de sempre, no modo do mestre" (no need to redraw it).
- The app shell's nav links: match the real shell (do not restyle it on this board).
- Copy: "Este pacote é de uma versão mais nova do MeuRPG, e este servidor ainda não sabe abri-lo." (not "Atualize o
  app"); replace "tipo que este servidor não conhece" with plain words naming the item; "Agora não" says where it goes
  ("Voltar para minhas campanhas"); no text under 14 px; one wording, "NPCs e criaturas".
