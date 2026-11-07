package play

import "github.com/PuraFome/meuRPG/backend/internal/authz"

// tvViewer is the member the table screen reads as: a player with no account and no character.
//
// This is the only place the TV's viewer is made. An empty user id is the whole table and nobody in
// particular: it owns nothing, has nothing of its own, and reads a scene or an image only when it is open
// to everyone. Every read of the TV runs as this member, and the projector copies a whitelist of fields
// from what it answers (tablescreen_project.go).
func tvViewer(campaignID string) authz.Membership {
	return authz.Membership{CampaignID: campaignID, Role: authz.RolePlayer, UserID: ""}
}

// tvReader is the reader of free rolls for the TV: not the master, no user.
func tvReader(tv authz.Membership) viewer {
	return viewer{master: tv.Role == authz.RoleMaster, userID: tv.UserID}
}
