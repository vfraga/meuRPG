package play

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
	"uuid"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
	"github.com/PuraFome/meuRPG/backend/internal/authz"
	"github.com/PuraFome/meuRPG/backend/internal/platform/idem"
	"github.com/PuraFome/meuRPG/backend/internal/play/link"
	"github.com/PuraFome/meuRPG/backend/internal/play/playdb"
)

// MaxMessageLength is the longest line of the conversation, in characters.
const MaxMessageLength = 2000

// Speaker kinds as the table stores them (session_messages.speaker_kind).
const (
	speakerNarrator       = "narrator"
	speakerNPC            = "npc"
	speakerCharacter      = "character"
	speakerOutOfCharacter = "out_of_character"
)

var speakerKindNames = map[playv1.SessionMessageSpeakerKind]string{
	playv1.SessionMessageSpeakerKind_SESSION_MESSAGE_SPEAKER_KIND_NARRATOR:         speakerNarrator,
	playv1.SessionMessageSpeakerKind_SESSION_MESSAGE_SPEAKER_KIND_NPC:              speakerNPC,
	playv1.SessionMessageSpeakerKind_SESSION_MESSAGE_SPEAKER_KIND_CHARACTER:        speakerCharacter,
	playv1.SessionMessageSpeakerKind_SESSION_MESSAGE_SPEAKER_KIND_OUT_OF_CHARACTER: speakerOutOfCharacter,
}

// newMessage is a SendSessionMessage request that passed every check, ready to store.
type newMessage struct {
	m           authz.Membership
	kind        string
	speakerRef  *string // the NPC or the character
	speakerName *string // its name as it is now, kept with the line
	speakerImg  *string // the NPC's portrait as it is now
	text        string
	imageID     string // the image asked for; the copy of it, when the maps module made one, is stored
	readers     []string
	everyone    bool
	scopedKey   *string
	requestHash *string
}

// SendSessionMessage implements playv1connect.PlayServiceHandler.
func (s *Service) SendSessionMessage(
	ctx context.Context,
	req *connect.Request[playv1.SendSessionMessageRequest],
) (*connect.Response[playv1.SendSessionMessageResponse], error) {
	in, err := s.readNewMessage(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	var imageCopy ShownCopy
	if in.imageID != "" {
		if _, imageCopy, _, err = s.prepareShownImage(ctx, in.m.CampaignID, &in.imageID); err != nil {
			return nil, err
		}
	}
	var sent sentMessage
	err = s.txOpen(ctx, in.m.CampaignID, func(tx pgx.Tx, h *hints, session playdb.GameSession) error {
		var err error
		sent, err = s.sendMessageTx(ctx, tx, h, session, in, imageCopy)
		return err
	})
	if imageCopy != nil && (err != nil || !sent.copyUsed) {
		imageCopy.Discard(ctx) // the copy's files were made for nothing
	}
	if isForeignKeyViolation(err) {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("image not found")) // deleted meanwhile
	}
	if err != nil {
		return nil, s.dbError(ctx, "send a message", err)
	}
	view, err := s.messageView(ctx, sent.row, viewer{master: in.m.Role == authz.RoleMaster, userID: in.m.UserID}, nil)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&playv1.SendSessionMessageResponse{Message: view}), nil
}

// readNewMessage makes SendSessionMessage's checks, in order: the role and what it may do, the text,
// the key, the speaker, the readers and the image's id.
func (s *Service) readNewMessage(ctx context.Context, msg *playv1.SendSessionMessageRequest) (newMessage, error) {
	m, err := authz.RequireCampaignMember(ctx, msg.GetCampaignId())
	if err != nil {
		return newMessage{}, err
	}
	if err := s.allowMessage(ctx, m.UserID); err != nil {
		return newMessage{}, err
	}
	kind, ok := speakerKindNames[msg.GetSpeakerKind()]
	if !ok {
		return newMessage{}, connect.NewError(connect.CodeInvalidArgument, errors.New("speaker_kind is required"))
	}
	master := m.Role == authz.RoleMaster
	if err := checkMayAttach(master, kind, msg); err != nil {
		return newMessage{}, err
	}
	text, err := cleanMessageText(msg.GetText())
	if err != nil {
		return newMessage{}, err
	}
	key, err := idem.Clean(msg.GetIdempotencyKey())
	if err != nil {
		return newMessage{}, err
	}
	if key == "" {
		return newMessage{}, connect.NewError(connect.CodeInvalidArgument, errors.New("idempotency_key is required"))
	}
	in := newMessage{
		m: m, kind: kind, text: text, imageID: msg.GetImageId(),
		scopedKey: idem.Scope(m.CampaignID+":"+m.UserID, key), requestHash: idem.Hash(msg),
	}
	if err := s.readSpeaker(ctx, &in, msg.GetSpeakerId()); err != nil {
		return newMessage{}, err
	}
	if err := s.readReaders(ctx, &in, msg); err != nil {
		return newMessage{}, err
	}
	if in.imageID != "" {
		if _, err := uuid.Parse(in.imageID); err != nil {
			return newMessage{}, connect.NewError(connect.CodeNotFound, errors.New("image not found"))
		}
	}
	return in, nil
}

// checkMayAttach refuses what the caller's role may not do: a kind that is the other role's, an
// image or readers from a player, only_master from the master.
func checkMayAttach(master bool, kind string, msg *playv1.SendSessionMessageRequest) error {
	masterKind := kind == speakerNarrator || kind == speakerNPC
	switch {
	case master != masterKind:
		return connect.NewError(connect.CodePermissionDenied, errors.New("this role cannot speak as that"))
	case !master && (msg.GetImageId() != "" || len(msg.GetReaderUserIds()) > 0):
		return connect.NewError(connect.CodePermissionDenied, errors.New("only the master attaches an image or chooses readers"))
	case master && msg.GetOnlyMaster():
		return connect.NewError(connect.CodePermissionDenied, errors.New("only a player writes to the master alone"))
	}
	return nil
}

// cleanMessageText trims the text and checks its length: 1 to MaxMessageLength characters.
func cleanMessageText(raw string) (string, error) {
	text := strings.TrimSpace(raw)
	if text == "" || !utf8.ValidString(text) {
		return "", connect.NewError(connect.CodeInvalidArgument, errors.New("text must not be empty"))
	}
	if utf8.RuneCountInString(text) > MaxMessageLength {
		return "", connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("text must have at most %d characters", MaxMessageLength))
	}
	return text, nil
}

// readSpeaker checks who the caller speaks as and keeps the name (and the NPC's portrait) the speaker
// has now: the master's NPC must be an NPC of the campaign; a player's character, their own living one (RN-18).
func (s *Service) readSpeaker(ctx context.Context, in *newMessage, rawID string) error {
	switch in.kind {
	case speakerNarrator, speakerOutOfCharacter:
		if rawID != "" {
			return connect.NewError(connect.CodeInvalidArgument, errors.New("speaker_id must be empty for this kind"))
		}
		return nil
	}
	id, err := uuid.Parse(rawID)
	if err != nil {
		return errCharacterNotFound()
	}
	chars, err := s.roster.CombatCharacters(ctx, nil, in.m.CampaignID, []string{id.String()})
	if err != nil {
		return err
	}
	if len(chars) != 1 {
		return errCharacterNotFound()
	}
	c := chars[0]
	switch {
	case in.kind == speakerNPC && (c.Player || c.CombatOnly):
		return errCharacterNotFound() // a player's character is not an NPC, nor a monster's
	case in.kind == speakerCharacter && (!c.Player || c.PlayerUserID != in.m.UserID):
		return errCharacterNotFound() // RN-18: only one's own character
	}
	in.speakerRef, in.speakerName = &c.ID, &c.Name
	if in.kind == speakerNPC && c.PortraitImageID != "" {
		in.speakerImg = &c.PortraitImageID
	}
	return nil
}

// readReaders works out who reads the line. The master names players of the campaign, and the table
// reads when none is named; a player writes to everyone or to the master alone.
func (s *Service) readReaders(ctx context.Context, in *newMessage, msg *playv1.SendSessionMessageRequest) error {
	in.everyone = true
	if in.m.Role != authz.RoleMaster {
		in.everyone = !msg.GetOnlyMaster()
		return nil
	}
	if len(msg.GetReaderUserIds()) == 0 {
		return nil
	}
	party, err := s.roster.CombatParty(ctx, nil, in.m.CampaignID)
	if err != nil {
		return err
	}
	for _, raw := range msg.GetReaderUserIds() {
		id, err := uuid.Parse(raw)
		if err != nil || !slices.ContainsFunc(party, func(c link.Character) bool { return c.PlayerUserID == id.String() }) {
			return connect.NewError(connect.CodeNotFound, errors.New("reader not found"))
		}
		if !slices.Contains(in.readers, id.String()) {
			in.readers = append(in.readers, id.String())
		}
	}
	in.everyone = false
	return nil
}

// sentMessage is what the transaction of a send ends with.
type sentMessage struct {
	row      playdb.SessionMessage
	replayed bool
	copyUsed bool // the image's copy is the one this call inserted
}

// sendMessageTx finds the line the key already made or inserts a new one with its readers, and has the line told to its readers
// after the commit (again, when the key made it already). The copy of a fog map's image gets its gallery row here, before the line
// that names it.
func (s *Service) sendMessageTx(ctx context.Context, tx pgx.Tx, h *hints, session playdb.GameSession, in newMessage, imageCopy ShownCopy) (sentMessage, error) {
	q := s.queries.WithTx(tx)
	var err error
	if in, err = s.withoutHiddenPortrait(ctx, q, session.ID, in); err != nil {
		return sentMessage{}, err
	}
	var out sentMessage
	out.row, out.replayed, err = idem.Create(ctx, in.scopedKey, in.requestHash, q.GetSessionMessageByCreateKey,
		func(r playdb.SessionMessage) *string { return r.CreateHash },
		func() (playdb.SessionMessage, error) {
			image := optionalString(in.imageID)
			if imageCopy != nil {
				id, created, err := imageCopy.Insert(ctx, tx)
				if err != nil {
					return playdb.SessionMessage{}, err
				}
				image, out.copyUsed = &id, created
			}
			return s.insertMessage(ctx, q, session.ID, in, image)
		})
	if err != nil {
		return out, err
	}
	if out.replayed {
		h.replay()
	}
	row := out.row
	h.touch()
	h.after(func(ctx context.Context, say sayFn) { s.sayMessage(ctx, say, row, in.readers) })
	return out, nil
}

// withoutHiddenPortrait takes the portrait off the line of an NPC the master keeps hidden on the stage: the
// line names the NPC for its readers, and nothing else of it reaches them, so not the portrait, which would
// open the image route (RN-10). "Dar a fala" shows the NPC; lines sent after that carry the portrait.
func (s *Service) withoutHiddenPortrait(ctx context.Context, q *playdb.Queries, sessionID string, in newMessage) (newMessage, error) {
	if in.kind != speakerNPC || in.speakerImg == nil || in.speakerRef == nil {
		return in, nil
	}
	stage, err := q.ListStage(ctx, sessionID)
	if err != nil {
		return in, fmt.Errorf("read the stage: %w", err)
	}
	if slices.ContainsFunc(stage, func(n playdb.StageNpc) bool { return n.CharacterID == *in.speakerRef && n.Hidden }) {
		in.speakerImg = nil
	}
	return in, nil
}

// insertMessage inserts the line and the rows of its readers. The key's unique index answers a lost
// race with pgx.ErrNoRows, for idem.Create to read.
func (s *Service) insertMessage(ctx context.Context, q *playdb.Queries, sessionID string, in newMessage, image *string) (playdb.SessionMessage, error) {
	row, err := q.InsertSessionMessage(ctx, playdb.InsertSessionMessageParams{
		CampaignID: in.m.CampaignID, GameSessionID: sessionID, AuthorUserID: &in.m.UserID, SpeakerKind: in.kind,
		SpeakerRef: in.speakerRef, SpeakerName: in.speakerName, SpeakerImageID: in.speakerImg, Text: in.text, ImageID: image,
		ForEveryone: in.everyone, CreateKey: in.scopedKey, CreateHash: in.requestHash, Now: s.now(),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return row, err
		}
		return row, fmt.Errorf("insert the message: %w", err)
	}
	for _, reader := range in.readers {
		if err := q.InsertSessionMessageReader(ctx, playdb.InsertSessionMessageReaderParams{MessageID: row.ID, UserID: reader}); err != nil {
			return row, fmt.Errorf("insert a reader: %w", err)
		}
	}
	return row, nil
}

func optionalString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
