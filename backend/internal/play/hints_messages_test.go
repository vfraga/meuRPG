package play

import (
	"testing"

	"connectrpc.com/connect"

	playv1 "github.com/PuraFome/meuRPG/backend/gen/meurpg/play/v1"
)

// A line of the conversation (#735 part 2, 4d) goes through Service.tx: the session's number moves once with the line, the hints
// carry it, only the readers hear it, a repeat of the key moves nothing and tells again, and a refused line tells nothing.

const kMessageAdded = "*playv1.WatchGameSessionResponse_SessionMessageAdded_"

// sayCall sends a line as u with the key, or with a new one for each call when key is empty.
func (a *armed) sayCall(u *user, key, text string, edit ...func(*playv1.SendSessionMessageRequest)) func() error {
	return func() error {
		_, err := a.say(u, asNarrator, text, append([]func(*playv1.SendSessionMessageRequest){func(r *playv1.SendSessionMessageRequest) {
			if key != "" {
				r.IdempotencyKey = key
			}
		}}, edit...)...)
		return err
	}
}

func (a *armed) oocCall(u *user, key, text string, edit ...func(*playv1.SendSessionMessageRequest)) func() error {
	return func() error {
		_, err := a.say(u, asOOC, text, append([]func(*playv1.SendSessionMessageRequest){func(r *playv1.SendSessionMessageRequest) { r.IdempotencyKey = key }}, edit...)...)
		return err
	}
}

func TestSendSessionMessageHints(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	g := a.stageHints(t)
	heard := g.first(a.sayCall(a.master, "", "O vento muda."), true)
	heardKinds(t, heard, map[string][]string{"master": {kMessageAdded}, "Ana": {kMessageAdded}, "Caio": {kMessageAdded}, "Bia": {kMessageAdded}})
}

func TestSendSessionMessageToChosenReadersHints(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	g := a.stageHints(t, "Caio", "Bia")
	heard := g.first(a.sayCall(a.master, "", "Psiu.", to(a.ana.id)), true)
	heardKinds(t, heard, map[string][]string{"master": {kMessageAdded}, "Ana": {kMessageAdded}})
}

func TestSendSessionMessageToTheMasterAloneHints(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	g := a.stageHints(t, "Ana", "Bia")
	heard := g.first(a.oocCall(a.caio, newKey(), "Mestre, uma pergunta.", onlyMaster), true)
	heardKinds(t, heard, map[string][]string{"master": {kMessageAdded}, "Caio": {kMessageAdded}})
}

func TestSendSessionMessageKeyedHints(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	g := a.stageHints(t, "Caio", "Bia")
	key := newKey()
	g.keyed(a.sayCall(a.master, key, "Psiu.", to(a.ana.id)), a.sayCall(a.master, key, "Psiu.", to(a.ana.id)), a.sayCall(a.master, key, "Outra coisa.", to(a.ana.id)))
}

func TestSendSessionMessageLostHints(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	g := a.stageHints(t)
	g.lost(a.sayCall(a.master, newKey(), "O vento muda."))
}

func TestSendSessionMessageRefusedTellsNothing(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	g := a.stageHints(t)
	g.refused(a.sayCall(a.ana, "", "Sou o narrador.")) // a player does not narrate
}

func TestSendSessionMessageWithNoOpenSessionRefusesAsBefore(t *testing.T) {
	t.Parallel()
	a := newArmed(t)
	a.master.end(t, a.master.liveSession(t, a.campaignID).GetGameSession())
	if err := a.sayCall(a.master, "", "Ninguém ouve.")(); connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("SendSessionMessage(no open session) error = %v, want failed_precondition", err)
	}
}
