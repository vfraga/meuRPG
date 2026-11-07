package migrations

import platformdb "github.com/PuraFome/meuRPG/backend/internal/platform/db"

// lostTheRace reports whether err says another run won the lock: runs that
// send the take statement together can get 40001 for the loser.
func lostTheRace(err error) bool {
	return platformdb.IsSerializationFailure(err)
}
