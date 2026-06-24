package store

import (
	"database/sql"
	"fmt"
)

func SetMeta(tx *sql.Tx, kv map[string]string) error {
	for k, v := range kv {
		if _, err := tx.Exec(`INSERT OR REPLACE INTO meta(key,value) VALUES(?,?)`, k, v); err != nil {
			return fmt.Errorf("set meta %s: %w", k, err)
		}
	}
	return nil
}
