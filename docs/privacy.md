[Português (Brasil)](pt-BR/privacidade.md)

# Privacy and data protection

MeuRPG keeps only what a table needs to play, keeps it in São Paulo, and treats each data subject right as a feature.
This document says which personal data exists, why, for how long, who processes it, and what every change must respect.

> **This is not legal advice.**
> It is engineering guidance, written by developers.
> The decision behind it is ADR-0010 (private repository): "privacy by design, LGPD as the base and GDPR as the yardstick".
> The privacy notice for people who use the app is the page `/privacy`, and the terms of use are `/terms` (see [The privacy notice and the terms](#the-privacy-notice-and-the-terms)).
> Both say on their face that no lawyer has reviewed them.

## In short

- **The law that applies is the LGPD** (Brazilian Law 13.709/2018).
  The GDPR does not apply, because the app is not offered to the European Union.
  On each topic we still follow the stricter of the two.
- **MeuRPG is the controller.**
  The data protection officer's (encarregado) channel is `privacidade@meurpg.app`, the contact on both public pages.
- **Everyone signs in with Google.**
  The app receives the account identifier and the verified e-mail, and nothing else.
  A player sign-in without Google, with no e-mail or real name, is a proposal ([RN-17](product/rules.md)).
- **The legal basis is the contract.**
  The app needs this data to work.
  Security and logs rest on legitimate interest.
  Nothing depends on consent.
- **No third-party cookies, analytics, pixels, CDN fonts or scripts.**
  The only cookies are the session cookie and a short-lived login cookie, both strictly necessary, so there is no banner.
  The app keeps nothing else in the browser.
- **The app is for people aged 18 or older** (see [Minors](#minors)).
- **It serves one table**, by invitation, and charges nothing.
  Opening it to other tables changes the size class of the processing agent, so that decision comes back first.

## Privacy checklist for PRs

Copy this into any PR that touches data, logs, screens or vendors:

- [ ] **Logs:** only IDs and closed enums.
  No header, query, body, IP, token, e-mail, name or free text ([what the log never holds](architecture/logs.md#what-never-goes-in-the-log)).
- [ ] **URLs:** no secret or personal data in a path or query string.
  Tokens go in the fragment (`#t=`), which no server receives.
- [ ] **Connect GET:** only a method whose request carries no ID or personal data may use GET.
  Everything else stays on POST ([API](architecture/api.md)).
- [ ] **Responses with personal data** go out with `Cache-Control: no-store`.
- [ ] **New column or table with personal data:** added to the [inventory](#personal-data-inventory) with purpose and retention.
- [ ] **Only what is needed:** every new field has a reason.
- [ ] **New free text:** the screen warns "É ficção: não escreva dados reais de pessoas."
  Free text never goes into a log or a session event.
- [ ] **Hidden things:** a response for a player never carries master notes (RN-11) or what the master has not revealed (RN-10).
- [ ] **Images:** they enter only through the gallery upload, which re-encodes the image and drops its metadata.
  No external image URL.
- [ ] **Browser:** no third-party script, font, pixel or iframe, and no `localStorage`, `sessionStorage`, IndexedDB, script-written cookie or worker keeping a token or personal data.
  The only storage on the device is the `HttpOnly` `__Host-` cookies ([Web](architecture/web.md#nothing-in-the-browser-besides-the-session-cookie)).
- [ ] **New vendor, or data leaving the server:** update the [processors table](#processors-and-where-data-lives) and open the contract and international-transfer question.
- [ ] **Secrets:** only in Secret Manager.
- [ ] **Impact trigger:** answer the [impact questions](#impact-questions).

## Personal data inventory

"While the campaign exists" means until the campaign is erased, which happens only when its master's account is erased (see [Delete the account](#delete-the-account)).
After any erasure, copies still disappear from backups and logs within 30 days.
Tables are described in [Data model](data/README.md).

### Account and sign-in

| Data | Purpose | Legal basis | Retention |
| --- | --- | --- | --- |
| Google account identifier (`sub`) and its issuer | Recognise the account at sign-in | Contract | Until the account is erased |
| Google e-mail | Security contact (an incident, a data request) and the list of who may create campaigns. Never shown to anyone. Stored only when Google says it is verified, and updated or erased at every sign-in | Contract; legitimate interest | Until the account is erased |
| Google name and photo | Never requested and never stored | — | — |
| Display name: typed in the app, 1 to 40 characters, never taken from Google. Shown to the members of the person's campaigns | Show who is who at the table | Contract | Until the person clears it in the profile or the account is erased |
| Session: a hash of the cookie value, creation and expiry times, the sign-in time and the last use (written at most every 10 minutes). No IP and no device data | Keep the sign-in | Contract | Valid for at most 30 days and ends after 14 days without use ("Sair dos outros dispositivos" ends the others at once). The expired row is swept by the database within a day |
| Sign-in state: a hash of `state`, the PKCE verifier, the nonce, where to return to (without fragment) and, when the person came from an invite or a claim link, its kind and a hash of its token (never the token) | Complete the Google sign-in and then accept the invite or claim | Contract | 10 minutes; erased at the callback, or swept by the database |
| The claim link a person signed in with (hash only) | Show the character card after sign-in without the secret in the address | Contract | 10 minutes; erased when the character is claimed |
| Invite: token hash, uses, expiry, whether it needs approval, who created it | Join a campaign (RN-07, RN-15) | Contract | Valid for at most 30 days; the row is swept 30 days after it expires, or at once with the campaign or the creator's account |
| Claim link of a reserved character (MR-049): token hash, who made it, when, validity, when it was used or revoked and by whom | Hand a master-made character to the player who takes it | Contract | Swept 30 days after it ended (used, revoked or expired), or at once with the campaign, the character or the master's account. "Used by" is cleared if that player's account is erased |
| Contact e-mail: a message sent to `privacidade@meurpg.app` or another `@meurpg.app` address (sender address and name, subject, text) | Answer a data request or a contact | Legal obligation (art. 18) for a data request; legitimate interest otherwise | ImprovMX, which forwards it, logs sender, recipient and subject for 7 days, never the text. In the mailboxes of the people who answer: 1 year after the answer, then deleted |

### What the table writes and plays

Campaigns, members, characters, maps, combats and the rest of the game are the content of the table.
The rows below say what each kind holds and who may read it.
Rows that hold only IDs, numbers and content keys say nothing about a person.

| Data | Who writes it, who reads it | Purpose | Legal basis | Retention |
| --- | --- | --- | --- | --- |
| Campaigns: name, XP mode, dice mode, table rules (including up to 20 house rules in the master's words) | The master writes; every active member reads | Play | Contract | While the campaign exists |
| Members and roles: who, in which campaign, the dice preference, and whether the member still waits for the master's approval | The master and the member | Control access (RN-05, RN-15) | Contract | While the campaign exists. A pending member with no character is removed after 30 days, or at once when the master rejects the character or removes the member |
| Character sheet and story: name, equipment, languages, traits, the "Outro" background, personality, appearance, history, allies. Free text, written as fiction | The player writes; the master and the owner read; other players never read it | Play. No field for the player's real name or e-mail | Contract | While the character exists. Characters are not deleted in the normal flow (RN-03). A pending character the master rejects is erased at once. When the player's account is erased, the character stays with the campaign and the master, with no link to the account (RN-16); one left with no player and no campaign is swept by the database within a day. An NPC goes with the master's account |
| Master notes about a character | The master only (RN-11) | Prepare the game | Contract | While the character exists; empty notes erase the row |
| The master's reason when they ask for changes on a pending character ("Pedir ajustes"), up to 500 characters | The master and the owner only; a list carries the status, never the reason | Tell the player what to fix | Contract | Until the character is approved or refused, or the character, campaign or player's account is erased |
| Campaign document, hooks of an RP scene, clues the master prepares, and the encounter saved on a battle point | The master only | Prepare the game | Contract | While the campaign exists |
| Clues revealed to a player (with a copy of the text), and the scenes the group, or the player alone, discovered | The master and the player who received them | Let the player re-read what was said even if the master edits the clue | Contract | While the campaign exists, or until the player's account is erased. If the master erases the clue, the copy stays with the player |
| The player's private notes (1 to 2,000 characters, with an optional scene tag) | The author only: not the master, not another player | Let the player note what happens | Contract | Until the author erases the note, the author's account is erased, or the campaign is erased |
| The scene's conversation (MR-051, RN-36): each line's author (account ID), speaker (narrator, an NPC, a character, or out of character), text (1 to 2,000 characters, free text), an optional gallery image, the time, the readers when it is not for everyone, an idempotency key, and the voice of a narrator or NPC line once someone asked to hear it | The master reads every line. A player reads the lines for everyone, the ones that chose them and their own: a line for other readers is never sent to them (RN-10) | Let the table talk during the session, also with a remote master | Contract | While the campaign exists. When the author's account is erased the line stays and the author becomes "Jogador removido" |
| Puzzles: the master's text (name, clue, hints, "Ao resolver", "Ao errar", split information) and the solution; per session, each move with the account ID, the character and **what the player typed** (a riddle answer, a deciphered message), and each attempt to earn a hint | The master writes. A player never reads the solution, unreleased hints or another player's text; the master reads what each player typed | Run puzzles live (MR-038, RN-27) | Contract | Puzzles while the campaign exists (they are archived, not erased). Moves and attempts while the session exists. Erasing an account clears its ID from moves and attempts; the typed text stays, anonymous, until the session goes |
| Reasons and names the master or a player writes in play: a combat name, the reason for a roll mode or an XP award (up to 120 characters), planned milestones, an NPC group's base name, the names a player gives a familiar or other creature, table content (classes, spells, backgrounds, creatures and so on) | The master (and the owning player, for a creature name and a table creature their character holds); an XP award's reason is read by every member | Play | Contract | While the campaign exists. The XP history records who gave the award; the link is cleared if that account is erased |
| Gallery images (maps, portraits, illustrations): the pixels re-encoded without metadata, the thumbnail, the name taken from the file (editable) and who uploaded them | The master uploads and lists them; a player downloads an image only while it is visible to them (RN-10) | Play (MR-019) | Contract | Until the master erases the image: the row and the files go at once, and Cloud Storage keeps the file 7 more days in soft delete. When a campaign is erased the rows go; its files do not (see [Delete the account](#delete-the-account)) |
| Image requests for AI generation: who asked, the master's text, the style, the references, the model, the state and, for a request from a map, the map and its size | The master | Generate images (MR-039, RN-28); count the monthly limit | Contract | While the campaign exists |
| Maps, points of interest, painted layers, tokens, doors, traps, treasures and their finders, lights, scene actions, the stage and the session's shown and left images | The master writes; a player reads only what is revealed to them (RN-10). Names and descriptions are the master's free text | Play (MR-008 and following) | Contract | While the campaign exists |
| What each player's characters have seen of a map with fog of war: one bitmap of squares per player and map | The server only. The player receives the state of each square; the master receives the same in "Ver como" | Show, darkened, what a character already saw (MR-036) | Contract | Until the master clears it ("Esquecer o que foi visto"), changes the map's grid or image, or the map, the player's account or the campaign is erased |
| Combats, combatants, rolls, pending damage, reactions, death saves, HP, spell slots, hit dice, level-up records, rolled ability scores, conditions and the rest of the live game state | Per RN-20, a player reads only their own numbers and the state word of enemies | Play | Contract | While the campaign exists. The player's account ID on a combatant is cleared if the account is erased. Rolled ability scores not yet used go with the account |
| Session history (`session_events`): who made the change (account ID), the character and combat IDs, numbers before and after, an idempotency key, the time. Never a name or free text. In combat on a fog-of-war map, the IDs of the accounts that saw an NPC act; in a scene open to only some players, the IDs of the accounts it was opened to and of the ones in it when a check was rolled | The master and, filtered, the players | Audit, undo and the combat log | Contract | While the campaign exists. The actor's ID is cleared if the account is erased, and so is the ID in every who-saw list inside a payload (see [Delete the account](#delete-the-account)) |
| Live stream subscriptions: account ID, whether master, campaign | Server memory | Deliver each change only to who may see it | Contract | While the stream is open (at most 10 minutes); never in the database or the log |

### Campaign package (MR-050)

| Data | Purpose | Legal basis | Retention |
| --- | --- | --- | --- |
| An export file, and the parts of an upload | Move a campaign between servers | Contract | An export 24 hours after it is done (an hour for a failed one); upload parts 1 hour after the last part. Both are also removed, with their rows, by the next sweep (within 10 minutes) once the campaign or the account that asked is gone |

### Logs, memory and infrastructure

| Data | Where | Purpose | Legal basis | Retention |
| --- | --- | --- | --- | --- |
| Account ID, campaign ID and other game IDs, request path and result, in the API log (no IP, e-mail or name) | Cloud Logging, São Paulo (production); stdout locally | Find the cause of an error and follow a session | Legitimate interest | 30 days in production; see [API logs](#api-logs) |
| IP, user agent and URL of each request, with Cloud Armor's decision | Cloud Run and load balancer logs in Cloud Logging, São Paulo | Operate and protect the platform | Legitimate interest | 30 days |
| Client IP in the rate limiters (sign-in and the whole API) | Server memory | Slow down brute force and abuse | Legitimate interest | Minutes; never in the database or the log |
| A task of the queue of detached work: the kind of work and IDs (campaign, request, export), never a name or free text | Cloud Tasks, São Paulo (the service's region) | Run an image, an export or a sweep after the call that asked for it has answered | Contract | Until the work runs. A task that keeps failing is dropped at most 1 hour after its first attempt (the queue's maximum retry duration, [Operations](operations/runbook.md#11-cloud-tasks)) |
| Database backups | Cockroach Labs, São Paulo | Disaster recovery | Legitimate interest | At most 30 days ([Operations](operations/reference.md)) |

### Cookies and the browser

| Cookie | Purpose | Basis | Lifetime |
| --- | --- | --- | --- |
| `__Host-meurpg_session` | Keep the sign-in | Strictly necessary | Until sign-out (which erases the session on the server), 14 days without use, or 30 days |
| `__Host-meurpg_login` | Bind the sign-in to the browser that began it (against login CSRF) | Strictly necessary | 10 minutes; erased at the callback |

Both are `HttpOnly` and never read by the page.
The app stores nothing else in the browser, and `web/src/no-web-storage.spec.ts` fails the build if source code touches a storage API.

### API logs

API logs keep the account ID (a UUID, a pseudonym) and game IDs, and nothing that identifies a person by itself.
Each line carries a `request_id` and, when they exist, `user_id` and `campaign_id`.
Rpc, stream and event lines say what was done with only IDs and enums.
The fields and levels are in [Logs](architecture/logs.md).

| Log data | Why | Retention |
| --- | --- | --- |
| `user_id` | Follow a session and find the cause of an error. It becomes a person only with the database, and it points to nobody once the account is erased | The log's: 30 days in Cloud Logging's `_Default` bucket ([Operations](operations/logs.md)); 14 days in the rehearsal ELK |
| `campaign_id` and other game IDs | The same | The log's |
| `request_id` | Join the lines of one request; returned in the `X-Request-Id` header | The log's |
| Request path (without the query) and result code | Operate the service | The log's |

The API log never holds the IP, user agent, cookies, headers, URL query, e-mail, any name (display, character, NPC, campaign), any typed text, or request and response bodies.
A guard in the log handler drops attributes with names such as `email`, `token`, `name` or `text`.
The IP and user agent appear only in the request log that Cloud Run itself writes, the platform row above.
Erasing an account does not rewrite the logs: the line expires with its retention.
A log is never a source for answering a data subject request.

## How the app protects what it keeps

- **Sign-in asks Google for `openid` and `email` only.**
  A failed sign-in logs a reason, never a token, code, state, cookie, e-mail or ID.
- **The e-mail leaves the database nowhere.**
  The profile response carries the account ID and the display name.
- **Secrets travel in the fragment.**
  An invite or claim token is sent in the body of a POST, stored only as a hash, and never reaches a URL query, a log or the browser's storage.
  A person who opens an invite while signed out carries it through sign-in inside the sign-in state, which holds only its hash.
- **Responses with personal data carry `Cache-Control: no-store`.**
  The sign-in routes also carry `Referrer-Policy: no-referrer`.
- **A non-member cannot discover a campaign.**
  The answer is the same `not_found` as for a campaign that does not exist.
  A pending member sees only the campaign name and their own character.
- **A player reads only what the rules let them.**
  Master notes, the campaign document, hidden map points, the solution of a puzzle, another player's notes and an enemy's exact HP never reach a player.
  Every read and every stream event is filtered on the server, and the leak tests prove it ([Testing](testing.md)).
- **A reserved character has no player data.**
  While it has no owner, no player reads it; a claim link shows only its name, race, class and level, the campaign name and the display name of whoever sent the link, and only to a signed-in person who holds the link.
  Every refusal of a link reads the same, so a stranger cannot tell a used link from one that never existed.
- **Images carry no metadata.**
  The server decodes each upload and encodes it again from the pixels, which drops EXIF (GPS position, phone model, date), XMP, ICC profiles and anything hidden after the image.
  The original file is never written.
  Next to every "Enviar imagem", the screen warns: "Use imagens do jogo. Não envie fotos de pessoas sem a autorização delas."
- **Images live in private storage.**
  Locally, in a Docker volume; in production, in a private Cloud Storage bucket in São Paulo with no public URL.
  Only the API reads it, and it checks the session and the membership on every request.
  A player's browser may keep an image but asks again on every use, so it stops seeing it as soon as it is no longer visible.
  What was already drawn on a screen, saved or photographed is out of our reach.
- **Fog-of-war images are personal.**
  A player never receives the whole image of a fog map, only pieces with the pixels of the squares they saw or remember and opaque black elsewhere.
  The pieces live only in server memory and are not personal data.
  See [Maps](architecture/maps/images.md).
- **The stream stores nothing.**
  Every 60 seconds it checks the session and the membership again; signing out, a revoked session or leaving the campaign ends it.

### What the campaign package does (MR-050)

The campaign package lets a master export a campaign as one file and create a campaign from such a file ([Campaign package](architecture/campaign-package.md)).

- **What it carries:** what the master prepared (document, notes, table rules and content, gallery images re-encoded, maps, NPCs, scenes, puzzles, points) and the players' characters with their stories, with no owner.
- **What it never carries:** the players' private notes, the session history and logs, accounts (IDs, e-mails, display names), members, invites, claim links, each player's fog memory and what lives only during a combat.
  The file format has no field for them, and a test plants a marker in each and looks for it in the file.
- **Where the file lives:** in the private bucket under `campaigns/<id>/exports/`, for the time in the inventory.
  The bucket has a lifecycle rule as a second net ([Operations](operations/reference.md#campaign-package)).
- **Who can download it:** only the master of the campaign, checked again on every request.
  The response is an attachment with `no-store`.
- **Who can upload:** the account that began the upload; any other account gets the `404` of an upload that does not exist.
- **Importing:** the file is read as untrusted.
  Every image is decoded and encoded again, and every text goes through the app's checks.
  A real person's data typed into a free-text field is the master's responsibility.

## Data subject rights and how we meet them

Anything the person can do alone, they do in the app.
Everything else goes through the data protection officer's channel and is answered within **15 days**.

| Right | LGPD | How |
| --- | --- | --- |
| Confirmation and access | Art. 18, I and II | E-mail to `privacidade@meurpg.app` |
| Portability | Art. 18, V | E-mail to the same address. A master can export a campaign's content in the app ([MR-050](product/stories.md)); that file never carries account data |
| Correction | Art. 18, III | The display name, in the profile. The text of a character after its sheet is locked (RN-01): the player edits it when the master unlocks it, otherwise by e-mail |
| Anonymisation, blocking or deletion of what is unnecessary | Art. 18, IV | E-mail |
| Deletion | Art. 18, VI | E-mail; see [Delete the account](#delete-the-account) |
| Who we share with | Art. 18, VII | The processors in this document and in the privacy notice |
| Objection and other requests | Art. 18, §2 | E-mail |
| Withdraw consent | Art. 18, IX | Nothing depends on consent. An optional feature added later will have an off switch |
| Complain to the ANPD | Art. 18, §1 | At gov.br/anpd, at any time |

A person can also end every other sign-in with "Sair dos outros dispositivos" in the profile.

### Delete the account

**A person asks by e-mail: there is no screen and no automatic deletion.**
Self-service export and deletion, and deletion after inactivity, are proposals in [RN-16](product/rules.md#rn-16-account-deletion-and-inactivity).
Nothing is erased for inactivity.

What erasing an account does, by the database's own rules:

| What | Result |
| --- | --- |
| Identity, e-mail, display name, sessions, memberships, the person's private notes, clue copies, review reasons and rolled ability scores | Erased |
| A master's campaigns, with everything they own: members, invites, characters of the campaign, NPCs, maps, sessions, history, puzzles, requests for images | Erased |
| A player's characters | Stay with the campaign and its master, with no link to the account. A character left with no player and no campaign is swept within a day |
| What the person typed in a puzzle (a riddle answer, a deciphered message), kept in the moves of a session | Stays, anonymous: the answers typed stay without the account, and the master still reads them, until the session goes. Nothing in the text is searched or redacted |
| Lines of the scene's conversation (MR-051) | Stay, with the author's ID cleared: they read "Jogador removido". A line that was for chosen readers stays for the master and the readers still left, never for everyone |
| Session history, combatants, XP awards, uploads, puzzle moves | Stay, with the person's ID cleared. The IDs of players who saw an NPC act, or who were in a scene open to only some players, kept inside some history payloads, are taken out of those lists in the same statement that erases the account |
| The files of a deleted campaign's images in the bucket | **Not erased by the database.** The operator deletes the `campaigns/<id>/` prefix by hand; this is not automated |
| Backups and logs | Gone within 30 days |

A campaign is erased only through its master's account: the app has no campaign deletion and no way for an accepted member to leave.

### The tension between keeping the character and erasing the identity

Keeping a deleted player's character with the master (RN-16) must not become a way of keeping that person's identity.
The link to the account goes away, but the character's free text (story, appearance, personality, allies) was written by the person and may hold personal data about them or others, despite the notice "É ficção".
That text is not filtered or redacted.
It is game content, and the master decides what to do with it, as with an NPC.
This is a product choice, not a legal guarantee that no personal data remains, so it stays a question for a lawyer (see [To be defined](#to-be-defined)).
A person who prefers can ask, by e-mail, for their characters to be erased too.
See ADR-0010, section 4.3.

## Processors and where data lives

Data lives in São Paulo (`southamerica-east1`).
A vendor outside Brazil that can reach it still makes an international transfer, so each needs a contract and a transfer mechanism (ANPD Resolution CD/ANPD no. 19/2024).

| Vendor | What it does | Data | Contract |
| --- | --- | --- | --- |
| Google Cloud | Cloud Run, the load balancer and Cloud Armor in front of it, Cloud Logging, Cloud Storage, Cloud Tasks, Secret Manager | Everything, in São Paulo | Data Processing Addendum with the Brazilian standard clauses |
| Cockroach Labs (CockroachDB Cloud) | The managed database and its backups, on Google Cloud in São Paulo | The database | **To be defined:** the current contract covers the GDPR |
| Google (sign-in) | Sign in with Google | Google controls its own account; the app receives only the identifier and the e-mail | Not our processor |
| Google (Gemini API) | Generates the images the master asks for and the voice of the narrator's and NPCs' lines, through the server | See below. Google may keep data briefly or in cache **in any country**: an international transfer | [Gemini API Paid Services terms](https://ai.google.dev/gemini-api/terms). **To be defined:** the transfer mechanism |
| ImprovMX | Forwards the e-mail sent to `@meurpg.app` to the people who answer ([Operations](operations/runbook.md#dns-certificates-and-e-mail)) | The message in transit. Its log keeps sender, recipient and subject for 7 days, never the text. It does not say where its servers are: counted as an international transfer | **To be defined:** the contract and the transfer mechanism |

### The legacy app database

The previous app's hosting (Render for the server, GitHub Pages for the pages) and its separate CockroachDB cluster are not part of this system.
They hold no data of the current app.
Turning them off, and deleting that cluster after the one-off character import ([Data model](data/README.md)), is the project owner's task; the providers' backups disappear within 30 days.

### What generated images send to Google (MR-039, RN-28)

Only the master generates images.
The call goes to the Gemini API with `store=false`, so nothing is kept on our Google account; under the paid-services terms Google may still keep data briefly or in cache, in any country.

- **What goes:** the text the master writes (as written), the style, the aspect ratio, the gallery images picked as reference (already re-encoded) and, in a refinement, the previous image with the first request's text and the refinements so far.
- **What never goes:** a person's name, e-mail, sheet, player name or ID.
  The request has no field for them, and a test checks the body.
  The master's free text goes as written, so the dialog warns under each field ("Não escreva nomes de pessoas.", "Não use foto de pessoa.") and the footer says every generated image carries Google's invisible SynthID mark.
- **From a map,** the request also carries a drawing the server makes from the map's layers, never from the uploaded image, which may show a secret.

| Way | The drawing | The room list |
| --- | --- | --- |
| Scene art and isometric view | Floor and walls of the squares the players' characters see now, the rest black; a blue disc per player character and a red one per NPC they see; an unrevealed secret door as a wall. No door, number, name, hidden token, trap or treasure | No |
| Textured map | The whole map, floor and walls only, the secret door as a wall, no creatures | Only for a generated dungeon: the number and size of each room, nothing else |

- **Portraits** of NPCs go only in the scene art and the isometric view, and only for NPCs the players see now.
  An NPC's name and a player character's name never go.
- **The drawing is stored nowhere;** it is redone at each request.
- **In our database** stays the request row (inventory above).
  The text is the master's data and goes with the campaign.
  The text and the API key never reach a log.
- **The image Google returns** is stored like an upload, re-encoded, with the same retention.
  A 1024 px JPEG reference derived from it is stored beside it and deleted with it.
- **Only the master asks,** and the app is for people aged 18 or older, so a minor's data never goes.

### What the voice sends to Google (MR-051)

Only the narrator's and the NPCs' lines can be heard, and only the line's readers can ask.
The server calls the Gemini API's text-to-speech (`generateContent`, which keeps no interaction on our account; under the paid-services terms Google may still keep data briefly or in cache, in any country).

- **What goes:** the line's text, as the master wrote it, and the name of one of a short list of voices. The text is free text: the master is warned in the composer not to write names of real people.
- **What never goes:** a player's line (players' lines are text only), a name, an e-mail, an account or campaign ID, the readers or the image.
- **Once per line:** the audio is made the first time a reader taps "Ouvir" and kept in the blob store beside the line, so the text is sent once. It is deleted with the campaign.
- **Only the readers hear it:** the audio route answers `404` to anyone who may not read the line (RN-36).
- **The key and the text never reach a log.** A daily cap on the whole server limits the Gemini bill.

### Logs in ELK

The ELK that stores the API logs in the rehearsal environment (and in production, if it comes) deletes each line after 14 days, within the 30-day cap on logs ([Operations](operations/logs.md#locally-with-elk)).
Only the operations team reads it, through a read-only role or Kibana with a login.
The lines hold `user_id` and `campaign_id`, and no e-mail, name or free text.
Erasing an account does not delete lines already written; they expire on their own.
Run by us, ELK is not a processor; if production uses Elastic Cloud, it joins the table above.

## Minors

The app is for people aged 18 or older, as the terms say.
It stores no date of birth and no age declaration.
Invites are personal, and the app has no table search, public profile or chat with strangers.
A minor at a table plays without an account of their own: the master looks after the sheet, with no personal data of the minor.
Before the app opens to the public, a lawyer must assess the Brazilian rules for minors online (ECA Digital, Law 15.211/2025) and an age check must follow the ANPD's final guidance.
The GDPR sets a minimum age only when the basis is consent, and the basis here is the contract.

## Incidents

- We assess within 24 hours whether an incident may cause relevant risk or harm.
  A leaked password hash or session counts as "authentication data" in the ANPD's criteria.
- We notify the ANPD and the affected people within 72 calendar hours, by message in the app and e-mail.
  The law gives 3 business days; 72 hours fits that and the GDPR's deadline.
- We record every incident for 5 years, even those that need no notification.

The runbook is in the private repository.

## Impact questions

Every PR answers these.
One "yes" needs a short section of risks and measures in the PR.
Two "yes" answers need a data protection impact report (RIPD) before the merge.

1. Does it collect a new type of personal data (a sensitive datum, a document, a photo of a person, a location)?
2. Does it involve a child or teenager, or age verification?
3. Does it send data to a new third party, or outside Brazil?
4. Does it decide anything about people automatically, build a profile, or use AI with personal data?
5. Does it observe behaviour (analytics, tracking)?
6. Does it make something public or easy to find (public profile, table search, chat)?
7. Does it change the scale (opening to other tables)?
8. Does it use data for a new purpose, or cross databases?

## To be defined

- ImprovMX: the contract and the transfer mechanism, or its replacement by e-mail on the organization's Google account.
- The LGPD contract with Cockroach Labs.
- Whether the CockroachDB Cloud backups stay in São Paulo and are kept for at most 30 days (check in the console); the plan is under evaluation ([Operations](operations/reference.md)).
- The transfer mechanism for Google's Gemini API (Resolution CD/ANPD no. 19/2024).
- Lawyer review of the privacy notice, the terms and the treatment of free text left with the master, before the app opens to other tables.
- Self-service export and account deletion, and deleting a campaign's image files when it is erased (the `campaigns/<id>/` prefix). Until they exist, the operator does it on request.
- Before reading rule PDFs with AI (MR-027, after the MVP): the processor, what leaves the server, and the copyright of official books.
  The PDF would be kept only while processed and deleted right after.

## The privacy notice and the terms

The privacy notice is the page `/privacy` and the terms of use are `/terms`.
They are public pages (no sign-in), linked from the footer of every page, in plain Portuguese, dated ("Última atualização") and with a summary box.
The text lives in `web/src/app/pages/legal/` and must say what this document says: a change to the inventory, the processors, the retention periods or the cookies is not done until the page changes too.

The notice has 14 sections: who looks after the data, the data we use and why, the Google account data, where data lives and the processors, retention, cookies, what we do not do, who sees what, rights, account deletion, security and incidents, the minimum age, changes and the contact.
The terms have 12.
Export and account deletion are handled by e-mail, and both pages say so.

## See also

- [Architecture](architecture/README.md): modules, sign-in and logs.
- [Data model](data/README.md): the tables cited here.
- [Operations](operations/README.md): secrets, logs, storage and costs.
- [Business rules](product/rules.md): RN-10 and RN-11, which protect what a player sees; RN-16, account deletion; RN-17, sign-in without Google.
- `docs/adr/` (private repository): ADR-0002 (session), ADR-0009 (player sign-in without Google), ADR-0010 (privacy), ADR-0011 (roles per campaign).
