package play

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/platform/ratelimit"
)

// The conversation of the scene (MR-051, RN-36). These tests need the database
// (MEURPG_TEST_DATABASE_URL).

const (
	asNarrator  = playv1.SessionMessageSpeakerKind_SESSION_MESSAGE_SPEAKER_KIND_NARRATOR
	asNPC       = playv1.SessionMessageSpeakerKind_SESSION_MESSAGE_SPEAKER_KIND_NPC
	asCharacter = playv1.SessionMessageSpeakerKind_SESSION_MESSAGE_SPEAKER_KIND_CHARACTER
	asOOC       = playv1.SessionMessageSpeakerKind_SESSION_MESSAGE_SPEAKER_KIND_OUT_OF_CHARACTER
)

// say sends a line as u with a new key; edit shapes the request.
func (a *armed) say(u *user, kind playv1.SessionMessageSpeakerKind, text string, edit ...func(*playv1.SendSessionMessageRequest)) (*playv1.SessionMessage, error) {
	a.h.t.Helper()
	req := &playv1.SendSessionMessageRequest{CampaignId: a.campaignID, SpeakerKind: kind, Text: text, IdempotencyKey: newKey()}
	for _, e := range edit {
		e(req)
	}
	res, err := u.play.SendSessionMessage(a.h.t.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return res.Msg.GetMessage(), nil
}

func (a *armed) mustSay(u *user, kind playv1.SessionMessageSpeakerKind, text string, edit ...func(*playv1.SendSessionMessageRequest)) *playv1.SessionMessage {
	a.h.t.Helper()
	m, err := a.say(u, kind, text, edit...)
	if err != nil {
		a.h.t.Fatalf("SendSessionMessage(%q) error = %v", text, err)
	}
	return m
}

func to(ids ...string) func(*playv1.SendSessionMessageRequest) {
	return func(r *playv1.SendSessionMessageRequest) { r.ReaderUserIds = ids }
}

func speaker(id string) func(*playv1.SendSessionMessageRequest) {
	return func(r *playv1.SendSessionMessageRequest) { r.SpeakerId = id }
}

func onlyMaster(r *playv1.SendSessionMessageRequest) { r.OnlyMaster = true }

// texts lists the texts of the lines u may read in the open session, oldest first.
func (a *armed) texts(u *user) []string {
	a.h.t.Helper()
	res, err := u.play.ListSessionMessages(a.h.t.Context(), connect.NewRequest(&playv1.ListSessionMessagesRequest{CampaignId: a.campaignID}))
	if err != nil {
		a.h.t.Fatalf("ListSessionMessages() error = %v", err)
	}
	var out []string
	for _, m := range res.Msg.GetMessages() {
		out = append(out, m.GetText())
	}
	return out
}

func wantTexts(t *testing.T, who string, got []string, want ...string) {
	t.Helper()
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("%s reads %q, want %q", who, got, want)
	}
}

func TestMessageForEveryoneReachesTheWholeTable(t *testing.T) {
	a := newArmed(t)
	ana, caio, master := a.ana.watch(t, a.campaignID), a.caio.watch(t, a.campaignID), a.master.watch(t, a.campaignID)
	for _, w := range []*watcher{ana, caio, master} {
		w.ready(t)
	}
	sent := a.mustSay(a.master, asNarrator, "A porta range.")
	if !sent.GetForEveryone() || sent.GetSpeakerKind() != asNarrator || !sent.GetCanHear() {
		t.Errorf("sent = %v, want a narration for everyone that can be heard", sent)
	}
	a.master.markCurrentMap(t, a.campaignID, a.mapID, ana, caio, master)
	for name, w := range map[string]*watcher{"Ana": ana, "Caio": caio, "the master": master} {
		got := w.beforeMarker(t)
		if len(got) != 1 || got[0].GetSessionMessageAdded().GetMessage().GetText() != "A porta range." {
			t.Errorf("%s's stream got %v, want the line once", name, got)
		}
	}
	for name, u := range map[string]*user{"Ana": a.ana, "Caio": a.caio, "the master": a.master, "Bia": a.bia} {
		wantTexts(t, name, a.texts(u), "A porta range.")
	}
}

func TestMessageForSomeReadersIsInvisibleToTheRest(t *testing.T) {
	a := newArmed(t)
	ana, caio, master := a.ana.watch(t, a.campaignID), a.caio.watch(t, a.campaignID), a.master.watch(t, a.campaignID)
	for _, w := range []*watcher{ana, caio, master} {
		w.ready(t)
	}
	sent := a.mustSay(a.master, asNPC, "Psiu, só você ouve.", speaker(a.goblin.GetId()), to(a.ana.id))
	if sent.GetForEveryone() || len(sent.GetReaderUserIds()) != 1 || sent.GetSpeakerName() != "Goblin" {
		t.Errorf("sent = %v, want a line for Ana alone, spoken by the Goblin", sent)
	}
	a.master.markCurrentMap(t, a.campaignID, a.mapID, ana, caio, master)
	if got := ana.beforeMarker(t); len(got) != 1 || got[0].GetSessionMessageAdded() == nil {
		t.Errorf("Ana's stream got %v, want the line", got)
	}
	if got := master.beforeMarker(t); len(got) != 1 || got[0].GetSessionMessageAdded() == nil {
		t.Errorf("the master's stream got %v, want the line once", got)
	}
	if got := caio.beforeMarker(t); len(got) != 0 {
		t.Errorf("Caio's stream got %v, want nothing (RN-36)", got)
	}
	wantTexts(t, "Ana", a.texts(a.ana), "Psiu, só você ouve.")
	wantTexts(t, "the master", a.texts(a.master), "Psiu, só você ouve.")
	wantTexts(t, "Caio", a.texts(a.caio))
	wantTexts(t, "Bia", a.texts(a.bia))
}

func TestMessageReadersAreCheckedAgainstTheTable(t *testing.T) {
	a := newArmed(t)
	stranger := a.h.newUser("Intruso")
	wantCode(t, "a reader who is not a member", errOf(a.say(a.master, asNarrator, "x", to(stranger.id))), connect.CodeNotFound)
	wantCode(t, "a reader who is the master", errOf(a.say(a.master, asNarrator, "x", to(a.master.id))), connect.CodeNotFound)
	wantCode(t, "a malformed reader", errOf(a.say(a.master, asNarrator, "x", to("x"))), connect.CodeNotFound)
}

func errOf(_ *playv1.SessionMessage, err error) error { return err }

func TestPlayerSpeaksOnlyAsTheirOwnCharacter(t *testing.T) {
	a := newArmed(t)
	if m := a.mustSay(a.caio, asCharacter, "Eu entro.", speaker(a.toren.GetId())); m.GetSpeakerName() != "Toren" || m.GetAuthorUserId() != a.caio.id {
		t.Errorf("sent = %v, want Toren, by Caio", m)
	}
	wantCode(t, "as another player's character (RN-18)", errOf(a.say(a.caio, asCharacter, "x", speaker(a.pens.GetId()))), connect.CodeNotFound)
	wantCode(t, "as an NPC", errOf(a.say(a.caio, asCharacter, "x", speaker(a.goblin.GetId()))), connect.CodeNotFound)
	wantCode(t, "as the narrator", errOf(a.say(a.caio, asNarrator, "x")), connect.CodePermissionDenied)
	wantCode(t, "as an NPC, kind", errOf(a.say(a.caio, asNPC, "x", speaker(a.goblin.GetId()))), connect.CodePermissionDenied)
	wantCode(t, "the master as a character", errOf(a.say(a.master, asCharacter, "x", speaker(a.toren.GetId()))), connect.CodePermissionDenied)
	ooc := a.mustSay(a.ana, asOOC, "Posso ir ao banheiro?")
	if ooc.GetSpeakerName() != "Ana" {
		t.Errorf("out of character = %v, want the player's name", ooc)
	}
}

func TestMasterSpeaksAsAnNPCOfTheCampaignOnly(t *testing.T) {
	a := newArmed(t)
	wantCode(t, "an unknown NPC", errOf(a.say(a.master, asNPC, "x", speaker(newKey()))), connect.CodeNotFound)
	wantCode(t, "a player's character", errOf(a.say(a.master, asNPC, "x", speaker(a.toren.GetId()))), connect.CodeNotFound)
	wantCode(t, "no speaker", errOf(a.say(a.master, asNPC, "x")), connect.CodeNotFound)
	wantCode(t, "a speaker for the narrator", errOf(a.say(a.master, asNarrator, "x", speaker(a.goblin.GetId()))), connect.CodeInvalidArgument)
}

func TestPlayerWritesToEveryoneOrToTheMasterOnly(t *testing.T) {
	a := newArmed(t)
	a.mustSay(a.caio, asOOC, "para o mestre", onlyMaster)
	a.mustSay(a.caio, asOOC, "para todos")
	wantTexts(t, "Caio", a.texts(a.caio), "para o mestre", "para todos")
	wantTexts(t, "the master", a.texts(a.master), "para o mestre", "para todos")
	wantTexts(t, "Ana", a.texts(a.ana), "para todos")
	wantCode(t, "a player choosing readers", errOf(a.say(a.caio, asOOC, "x", to(a.ana.id))), connect.CodePermissionDenied)
	wantCode(t, "a player attaching an image", errOf(a.say(a.caio, asOOC, "x", func(r *playv1.SendSessionMessageRequest) { r.ImageId = newKey() })), connect.CodePermissionDenied)
	wantCode(t, "the master writing to the master", errOf(a.say(a.master, asNarrator, "x", onlyMaster)), connect.CodePermissionDenied)
}

func TestMessageTextRules(t *testing.T) {
	a := newArmed(t)
	for name, text := range map[string]string{"empty": "", "blanks": " \n\t ", "too long": strings.Repeat("a", MaxMessageLength+1)} {
		wantCode(t, name, errOf(a.say(a.master, asNarrator, text)), connect.CodeInvalidArgument)
	}
	if m := a.mustSay(a.master, asNarrator, strings.Repeat("ã", MaxMessageLength)); len([]rune(m.GetText())) != MaxMessageLength {
		t.Errorf("a line of %d characters was changed", MaxMessageLength)
	}
	wantCode(t, "no key", errOf(a.say(a.master, asNarrator, "x", func(r *playv1.SendSessionMessageRequest) { r.IdempotencyKey = "" })), connect.CodeInvalidArgument)
}

func TestMessageImageMustBeInTheGallery(t *testing.T) {
	a := newArmed(t)
	attach := func(id string) func(*playv1.SendSessionMessageRequest) {
		return func(r *playv1.SendSessionMessageRequest) { r.ImageId = id }
	}
	wantCode(t, "an image that does not exist", errOf(a.say(a.master, asNarrator, "x", attach(newKey()))), connect.CodeNotFound)
	wantCode(t, "a malformed image", errOf(a.say(a.master, asNarrator, "x", attach("x"))), connect.CodeNotFound)
	image := a.galleryImage(a.campaignID)
	sent := a.mustSay(a.master, asNarrator, "Veja.", attach(image), to(a.ana.id))
	if sent.GetImage().GetId() == "" || sent.GetImage().GetId() != image {
		t.Fatalf("image = %v, want the gallery image %s", sent.GetImage(), image)
	}
	// The one thing a line lets its readers open is its image, and only to its readers (RN-36).
	for name, c := range map[string]struct {
		user string
		want bool
	}{"Ana": {a.ana.id, true}, "Caio": {a.caio.id, false}, "Ana, another image": {a.ana.id, false}} {
		id := image
		if name == "Ana, another image" {
			id = a.galleryImage(a.campaignID)
		}
		got, err := a.h.svc.ImageInConversation(t.Context(), a.campaignID, c.user, id)
		if err != nil || got != c.want {
			t.Errorf("ImageInConversation(%s) = %v, %v; want %v", name, got, err, c.want)
		}
	}
}

func (a *armed) galleryImage(campaign string) string {
	a.h.t.Helper()
	id := newKey()
	if _, err := a.h.pool.Exec(a.h.t.Context(),
		`INSERT INTO gallery_images (id, campaign_id, name, content_type, width, height, byte_size, created_at) VALUES ($1, $2, 'imagem', 'image/png', 40, 30, 100, now())`,
		id, campaign); err != nil {
		a.h.t.Fatalf("insert a gallery image: %v", err)
	}
	return id
}

func TestMessageIsIdempotent(t *testing.T) {
	a := newArmed(t)
	key := newKey()
	send := func(text string) (*playv1.SessionMessage, error) {
		return a.say(a.master, asNarrator, text, func(r *playv1.SendSessionMessageRequest) { r.IdempotencyKey = key })
	}
	first, err := send("Uma vez.")
	if err != nil {
		t.Fatalf("SendSessionMessage() error = %v", err)
	}
	again, err := send("Uma vez.")
	if err != nil || again.GetId() != first.GetId() {
		t.Errorf("the retry = %v, %v; want the same line", again, err)
	}
	wantTexts(t, "the master", a.texts(a.master), "Uma vez.")
	wantCode(t, "the key for another line", errOf(send("Outra.")), connect.CodeInvalidArgument)
}

func TestMessageRetryTellsTheSameLineAgain(t *testing.T) {
	a := newArmed(t)
	ana := a.ana.watch(t, a.campaignID)
	ana.ready(t)
	key := newKey()
	for range 2 {
		a.mustSay(a.master, asNarrator, "Eco.", func(r *playv1.SendSessionMessageRequest) { r.IdempotencyKey = key })
	}
	a.master.markCurrentMap(t, a.campaignID, a.mapID, ana)
	// A retry of the key tells the line again, to the same readers, so a hint lost on the first call is not lost for good; the
	// screens read it once (the line has one id).
	if got := ana.beforeMarker(t); len(got) != 2 {
		t.Errorf("Ana's stream got %d events, want 2 (the line and its retelling)", len(got))
	}
}

func TestMessageRateLimit(t *testing.T) {
	a := newArmed(t)
	a.h.svc.msgs.send = ratelimit.New(ratelimit.Config{
		PerClient: ratelimit.Rate{Burst: 2, Every: time.Hour}, Global: ratelimit.Rate{Burst: 100, Every: time.Millisecond}, MaxClients: 100,
	})
	a.mustSay(a.master, asNarrator, "um")
	a.mustSay(a.master, asNarrator, "dois")
	wantCode(t, "the third line", errOf(a.say(a.master, asNarrator, "três")), connect.CodeResourceExhausted)
	a.mustSay(a.caio, asOOC, "outro jogador não espera")
}

func TestMessagesNeedAnOpenSessionAndMembership(t *testing.T) {
	a := newArmed(t)
	stranger := a.h.newUser("Intruso")
	wantCode(t, "a stranger sending", errOf(a.say(stranger, asOOC, "x")), connect.CodeNotFound)
	_, err := stranger.play.ListSessionMessages(t.Context(), connect.NewRequest(&playv1.ListSessionMessagesRequest{CampaignId: a.campaignID}))
	wantCode(t, "a stranger listing", err, connect.CodeNotFound)
	started := a.master.liveSession(t, a.campaignID).GetGameSession()
	a.master.end(t, started)
	wantBlocked(t, "SendSessionMessage", errOf(a.say(a.master, asNarrator, "x")), playv1.GameSessionBlockedReason_GAME_SESSION_BLOCKED_REASON_NO_OPEN_SESSION)
}

func TestListMessagesPagesNewestLast(t *testing.T) {
	a := newArmed(t)
	for _, text := range []string{"1", "2", "3", "4", "5"} {
		a.mustSay(a.master, asNarrator, text)
	}
	a.mustSay(a.master, asNarrator, "secreta", to(a.ana.id))
	list := func(u *user, before string) *playv1.ListSessionMessagesResponse {
		res, err := u.play.ListSessionMessages(t.Context(), connect.NewRequest(&playv1.ListSessionMessagesRequest{CampaignId: a.campaignID, PageSize: 2, BeforeId: before}))
		if err != nil {
			t.Fatalf("ListSessionMessages() error = %v", err)
		}
		return res.Msg
	}
	page := list(a.caio, "")
	if len(page.GetMessages()) != 2 || page.GetMessages()[1].GetText() != "5" || !page.GetHasOlder() {
		t.Fatalf("Caio's first page = %v, want 4 and 5 and more", page.GetMessages())
	}
	older := list(a.caio, page.GetMessages()[0].GetId())
	if len(older.GetMessages()) != 2 || older.GetMessages()[0].GetText() != "2" || !older.GetHasOlder() {
		t.Errorf("Caio's second page = %v, want 2 and 3 and more", older.GetMessages())
	}
	// A cursor on a line the caller may not read is the same answer as a line that does not exist.
	secret := list(a.master, "").GetMessages()[1].GetId()
	_, err := a.caio.play.ListSessionMessages(t.Context(), connect.NewRequest(&playv1.ListSessionMessagesRequest{CampaignId: a.campaignID, BeforeId: secret}))
	wantCode(t, "a cursor on an unreadable line", err, connect.CodeNotFound)
	_, err = a.caio.play.ListSessionMessages(t.Context(), connect.NewRequest(&playv1.ListSessionMessagesRequest{CampaignId: a.campaignID, BeforeId: newKey()}))
	wantCode(t, "a cursor on no line", err, connect.CodeNotFound)
	if got := list(a.ana, "").GetMessages()[1].GetText(); got != "secreta" {
		t.Errorf("Ana's last line = %q, want the secret", got)
	}
}

func TestListMessagesOfAnotherSessionIsAskedByID(t *testing.T) {
	a := newArmed(t)
	a.mustSay(a.master, asNarrator, "da primeira sessão")
	first := a.master.end(t, a.master.liveSession(t, a.campaignID).GetGameSession()).GetId()
	a.master.start(t, a.campaignID)
	a.mustSay(a.master, asNarrator, "da segunda")
	wantTexts(t, "the open session", a.texts(a.caio), "da segunda")
	res, err := a.caio.play.ListSessionMessages(t.Context(), connect.NewRequest(&playv1.ListSessionMessagesRequest{CampaignId: a.campaignID, GameSessionId: first}))
	if err != nil || len(res.Msg.GetMessages()) != 1 || res.Msg.GetMessages()[0].GetText() != "da primeira sessão" {
		t.Errorf("the first session = %v, %v", res, err)
	}
}

func TestErasedAuthorBecomesARemovedPlayer(t *testing.T) {
	a := newArmed(t)
	a.mustSay(a.bia, asOOC, "Até mais.")
	if _, err := a.h.pool.Exec(t.Context(), `UPDATE session_messages SET author_user_id = NULL WHERE author_user_id = $1`, a.bia.id); err != nil {
		t.Fatalf("erase the author: %v", err)
	}
	res, err := a.caio.play.ListSessionMessages(t.Context(), connect.NewRequest(&playv1.ListSessionMessagesRequest{CampaignId: a.campaignID}))
	if err != nil || len(res.Msg.GetMessages()) != 1 {
		t.Fatalf("ListSessionMessages() = %v, %v; want the line to stay", res, err)
	}
	if m := res.Msg.GetMessages()[0]; !m.GetAuthorRemoved() || m.GetAuthorUserId() != "" || m.GetText() != "Até mais." {
		t.Errorf("line = %v, want a removed author and the text", m)
	}
}

func TestMasterTypingGoesToThePlayersOnly(t *testing.T) {
	a := newArmed(t)
	ana, master := a.ana.watch(t, a.campaignID), a.master.watch(t, a.campaignID)
	ana.ready(t)
	master.ready(t)
	typing := func(u *user) error {
		_, err := u.play.SignalMasterTyping(t.Context(), connect.NewRequest(&playv1.SignalMasterTypingRequest{CampaignId: a.campaignID}))
		return err
	}
	wantCode(t, "a player signaling", typing(a.caio), connect.CodePermissionDenied)
	for range 3 {
		if err := typing(a.master); err != nil {
			t.Fatalf("SignalMasterTyping() error = %v", err)
		}
	}
	a.master.markCurrentMap(t, a.campaignID, a.mapID, ana, master)
	got := ana.beforeMarker(t)
	if len(got) != 1 || got[0].GetMasterTyping().GetExpiresInSeconds() <= 0 {
		t.Errorf("Ana's stream got %v, want one master_typing (the rest wait out the cooldown)", got)
	}
	if got := master.beforeMarker(t); len(got) != 0 {
		t.Errorf("the master's stream got %v, want nothing", got)
	}
}

func TestSummaryHoldsOnlyTheLinesTheReaderMayRead(t *testing.T) {
	a := newArmed(t)
	a.mustSay(a.master, asNarrator, "para todos")
	a.mustSay(a.master, asNarrator, "só Ana", to(a.ana.id))
	a.mustSay(a.caio, asOOC, "só o mestre", onlyMaster)
	id := a.master.end(t, a.master.liveSession(t, a.campaignID).GetGameSession()).GetId()
	summary := func(u *user) []string {
		res, err := u.play.GetSessionSummary(t.Context(), connect.NewRequest(&playv1.GetSessionSummaryRequest{CampaignId: a.campaignID, GameSessionId: id}))
		if err != nil {
			t.Fatalf("GetSessionSummary() error = %v", err)
		}
		var out []string
		for _, m := range res.Msg.GetSummary().GetConversation() {
			out = append(out, m.GetText())
		}
		return out
	}
	wantTexts(t, "the master", summary(a.master), "para todos", "só Ana", "só o mestre")
	wantTexts(t, "Ana", summary(a.ana), "para todos", "só Ana")
	wantTexts(t, "Caio", summary(a.caio), "para todos", "só o mestre")
	wantTexts(t, "Bia", summary(a.bia), "para todos")
}

// audioReply is what the audio route answered: the status, the two headers the tests read, and the body.
type audioReply struct {
	status       int
	contentType  string
	cacheControl string
	body         []byte
}

// audio asks for the voice of a line as userID ("" is nobody signed in).
func (a *armed) audio(userID, messageID string) audioReply {
	a.h.t.Helper()
	req, err := http.NewRequestWithContext(a.h.t.Context(), http.MethodGet, a.h.server.URL+AudioPath+messageID+"/audio", nil)
	if err != nil {
		a.h.t.Fatalf("NewRequest() error = %v", err)
	}
	if userID != "" {
		req.Header.Set(testUserHeader, userID)
	}
	res, err := a.h.server.Client().Do(req)
	if err != nil {
		a.h.t.Fatalf("GET audio error = %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	body, _ := io.ReadAll(res.Body)
	return audioReply{status: res.StatusCode, contentType: res.Header.Get("Content-Type"), cacheControl: res.Header.Get("Cache-Control"), body: body}
}

func (a *armed) wantAudioStatus(who, messageID, userID string, want int) {
	a.h.t.Helper()
	if r := a.audio(userID, messageID); r.status != want {
		a.h.t.Errorf("audio for %s = %d, want %d", who, r.status, want)
	}
}

func TestAudioIsForTheReadersOfTheLineOnly(t *testing.T) {
	a := newArmed(t)
	line := a.mustSay(a.master, asNPC, "Não conte a ninguém.", speaker(a.goblin.GetId()), to(a.ana.id))
	if !line.GetCanHear() || !strings.HasSuffix(line.GetAudioUrl(), line.GetId()+"/audio") {
		t.Errorf("line = %v, want audio for its readers", line)
	}
	res := a.audio(a.ana.id, line.GetId())
	if res.status != http.StatusOK || res.contentType != "audio/wav" || len(res.body) < 44 || string(res.body[:4]) != "RIFF" {
		t.Fatalf("Ana got %d %q (%d bytes), want a WAV", res.status, res.contentType, len(res.body))
	}
	if cc := res.cacheControl; !strings.Contains(cc, "private") {
		t.Errorf("Cache-Control = %q, want private (no cache shared across users)", cc)
	}
	a.wantAudioStatus("the master", line.GetId(), a.master.id, http.StatusOK)
	a.wantAudioStatus("Caio, who is not a reader", line.GetId(), a.caio.id, http.StatusNotFound)
	a.wantAudioStatus("nobody signed in", line.GetId(), "", http.StatusUnauthorized)
	a.wantAudioStatus("a stranger", line.GetId(), a.h.newUser("Intruso").id, http.StatusNotFound)
	a.wantAudioStatus("a line that does not exist", newKey(), a.ana.id, http.StatusNotFound)
	a.wantAudioStatus("an id that is not one", "x", a.ana.id, http.StatusNotFound)
	// Not hearing is the same answer whether the line exists or not, and says nothing more.
	missing := a.audio(a.caio.id, newKey())
	unread := a.audio(a.caio.id, line.GetId())
	if missing.status != unread.status || !bytes.Equal(missing.body, unread.body) {
		t.Errorf("an unreadable line answers %d %q, a missing one %d %q: they must match", unread.status, unread.body, missing.status, missing.body)
	}
}

func TestPlayersLinesCannotBeHeard(t *testing.T) {
	a := newArmed(t)
	ooc := a.mustSay(a.caio, asOOC, "oi")
	char := a.mustSay(a.caio, asCharacter, "olá", speaker(a.toren.GetId()))
	for _, m := range []*playv1.SessionMessage{ooc, char} {
		if m.GetCanHear() || m.GetAudioUrl() != "" {
			t.Errorf("line %v offers audio, want text only", m)
		}
		a.wantAudioStatus("the master", m.GetId(), a.master.id, http.StatusNotFound)
	}
}

func TestAudioIsMadeOnceAndKept(t *testing.T) {
	a := newArmed(t)
	line := a.mustSay(a.master, asNarrator, "Um trovão ecoa.")
	a.wantAudioStatus("Ana", line.GetId(), a.ana.id, http.StatusOK)
	a.wantAudioStatus("Caio", line.GetId(), a.caio.id, http.StatusOK)
	a.wantAudioStatus("Ana again", line.GetId(), a.ana.id, http.StatusOK)
	if n := a.h.voice.Calls(); n != 1 {
		t.Errorf("the voice was made %d times, want 1", n)
	}
}

func TestAudioIsMadeOnceForSimultaneousRequests(t *testing.T) {
	a := newArmed(t)
	line := a.mustSay(a.master, asNarrator, "Todos pedem juntos.")
	var wg sync.WaitGroup
	for _, u := range []*user{a.ana, a.caio, a.bia, a.master} {
		wg.Go(func() { a.wantAudioStatus(u.id, line.GetId(), u.id, http.StatusOK) })
	}
	wg.Wait()
	if n := a.h.voice.Calls(); n != 1 {
		t.Errorf("the voice was made %d times, want 1", n)
	}
}

func TestAudioDailyCap(t *testing.T) {
	a := newArmed(t)
	a.h.svc.msgs.dailyCap = 1
	first := a.mustSay(a.master, asNarrator, "primeira")
	second := a.mustSay(a.master, asNarrator, "segunda")
	a.wantAudioStatus("the first line", first.GetId(), a.ana.id, http.StatusOK)
	res := a.audio(a.ana.id, second.GetId())
	if res.status != http.StatusTooManyRequests || !strings.Contains(string(res.body), "DAILY_LIMIT_REACHED") {
		t.Errorf("the second voice = %d %s, want the day's limit", res.status, res.body)
	}
	a.wantAudioStatus("the first line, kept", first.GetId(), a.caio.id, http.StatusOK)
	if n := a.h.voice.Calls(); n != 1 {
		t.Errorf("the voice was made %d times, want 1", n)
	}
}

func TestAudioIsOffWithoutAVoice(t *testing.T) {
	a := newArmed(t)
	a.h.svc.msgs.voice = nil
	line := a.mustSay(a.master, asNarrator, "sem voz")
	if line.GetCanHear() {
		t.Errorf("line = %v, want no \"Ouvir\" while the voice is off", line)
	}
	res := a.audio(a.ana.id, line.GetId())
	if res.status != http.StatusPreconditionFailed || !strings.Contains(string(res.body), `"reason":"OFF"`) {
		t.Errorf("audio = %d %s, want failed_precondition OFF", res.status, res.body)
	}
	// What the caller may not read is still a 404, voice or not.
	b := a.mustSay(a.master, asNarrator, "segredo", to(a.ana.id))
	a.wantAudioStatus("Caio, voice off", b.GetId(), a.caio.id, http.StatusNotFound)
	list, err := a.ana.play.ListSessionMessages(t.Context(), connect.NewRequest(&playv1.ListSessionMessagesRequest{CampaignId: a.campaignID}))
	if err != nil || list.Msg.GetVoiceEnabled() {
		t.Errorf("voice_enabled = %v, %v; want false", list.Msg.GetVoiceEnabled(), err)
	}
}
