package database

import (
	"database/sql"
	"fmt"
	"monitor/internal/utils"

	_ "github.com/mattn/go-sqlite3"
)

// createDatabase creates the database that is used by the tool, in the location
// defined in the config.
func CreateDatabase() error {
	db, err := sql.Open("sqlite3", utils.Config.DatabaseLocation)
	if err != nil {
		fmt.Printf("ERR: Could not open database, error: %v\n", err)
		return err
	}
	defer db.Close()

	fmt.Println("INFO: Creating new database.")

	// create validators table
	createValidatorsTableSQL := `CREATE TABLE IF NOT EXISTS validators (
		"id" INTEGER NOT NULL PRIMARY KEY,
		"owner_key" TEXT NOT NULL,
		"signer_key" TEXT NOT NULL,
		"activation_epoch" INTEGER,
		"deactivation_epoch" INTEGER
	)`

	_, err = db.Exec(createValidatorsTableSQL)
	if err != nil {
		fmt.Printf("ERR: Error while creating validators' table, error: %v\n", err)
		return err
	}

	// create checkpoints table
	createCheckpointsTableSQL := `CREATE TABLE IF NOT EXISTS checkpoints (
		"id" INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
		"number" INTEGER NOT NULL,
		"block_number" INTEGER NOT NULL,
		"timestamp" INTEGER NOT NULL,
		"proposer_id" INTEGER NOT NULL,
		"reward" INTEGER,
		"performance_benchmark" REAL,
		FOREIGN KEY(proposer_id) REFERENCES validators(id)
	)`

	_, err = db.Exec(createCheckpointsTableSQL)
	if err != nil {
		fmt.Printf("ERR: Error while creating checkpoints' table, error: %v\n", err)
		return err
	}

	// create validators signed checkpoints table
	createValidatorsSignedCheckpointsTableSQL := `CREATE TABLE IF NOT EXISTS validators_signed_checkpoints (
		"id" INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
		"checkpoint_id" INTEGER NOT NULL,
		"validator_id" INTEGER NOT NULL,
		UNIQUE(checkpoint_id, validator_id) ON CONFLICT REPLACE,
		FOREIGN KEY(checkpoint_id) REFERENCES checkpoints(id),
		FOREIGN KEY(validator_id) REFERENCES validators(id)
	)`

	_, err = db.Exec(createValidatorsSignedCheckpointsTableSQL)
	if err != nil {
		fmt.Printf("ERR: Error while creating validators' signed checkpoints table, error: %v\n", err)
		return err
	}

	// create temporary validators signed checkpoints table - used for calculating performance benchmark
	createTemporaryValidatorsSignedCheckpointsTableSQL := `CREATE TABLE IF NOT EXISTS temp_validators_signed_checkpoints (
		"id" INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
		"checkpoint_id" INTEGER NOT NULL,
		"validator_id" INTEGER NOT NULL,
		UNIQUE(checkpoint_id, validator_id) ON CONFLICT REPLACE,
		FOREIGN KEY(checkpoint_id) REFERENCES checkpoints(id),
		FOREIGN KEY(validator_id) REFERENCES validators(id)
	)`

	_, err = db.Exec(createTemporaryValidatorsSignedCheckpointsTableSQL)
	if err != nil {
		fmt.Printf("ERR: Error while creating temporary validators' signed checkpoints table, error: %v\n", err)
		return err
	}

	return nil
}
