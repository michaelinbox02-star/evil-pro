package database

import "github.com/tidwall/buntdb"

// authTokenPrefix namespaces panel session tokens in the store.
const authTokenPrefix = "auth_session:"

// StoreAuthToken maps an opaque token to a value (the panel stores
// "user|expiry" here). Tokens survive a restart because they live in the DB.
func (d *Database) StoreAuthToken(token string, value string) error {
	return d.db.Update(func(tx *buntdb.Tx) error {
		_, _, err := tx.Set(authTokenPrefix+token, value, nil)
		return err
	})
}

// GetAuthToken returns the stored value for a token.
func (d *Database) GetAuthToken(token string) (string, error) {
	var val string
	err := d.db.View(func(tx *buntdb.Tx) error {
		v, err := tx.Get(authTokenPrefix + token)
		if err != nil {
			return err
		}
		val = v
		return nil
	})
	return val, err
}

// DeleteAuthToken removes a token (logout / expiry cleanup).
func (d *Database) DeleteAuthToken(token string) error {
	return d.db.Update(func(tx *buntdb.Tx) error {
		_, err := tx.Delete(authTokenPrefix + token)
		return err
	})
}
