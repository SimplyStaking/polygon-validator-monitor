package database

import (
	"database/sql"
	"fmt"

	"monitor/internal/utils"

	_ "github.com/mattn/go-sqlite3"
)

// getCheckpointId gets the checkpoint ID for the checkpoint with the number
// passed.
func getCheckpointId(checkpointNumber uint64) (int, error) {
	db, err := sql.Open("sqlite3", utils.Config.DatabaseLocation)
	if err != nil {
		fmt.Printf("ERR: Could not open database, error: %v\n", err)
		return 0, err
	}
	defer db.Close()

	selectSQL := `SELECT id
			FROM checkpoints 
			WHERE number = ?`

	statement, err := db.Prepare(selectSQL)
	if err != nil {
		fmt.Printf("ERR: Error while preparing SQL statement, error: %v\n", err)
		return 0, err
	}
	defer statement.Close()

	rows, err := statement.Query(checkpointNumber)
	if err != nil {
		fmt.Printf("ERR: Error while querying for validator id using public key, error: %v\n", err)
		return 0, err
	}
	defer rows.Close()

	for rows.Next() {
		var id int

		err = rows.Scan(&id)
		if err != nil {
			fmt.Printf("ERR: Error while reading row from db, error: %v\n", err)
			return 0, err
		}

		return id, nil
	}

	fmt.Printf("ERR: Could not find checkpoint number %d in database.\n", checkpointNumber)
	return 0, &utils.CheckpointNotFoundError{GenericError: utils.GenericError{Message: "checkpoint with provided number not found"}}
}

// checkIfCheckpointExists checks in the passed checkpoint number exists in the
// database or not.
func checkIfCheckpointExists(checkpointNumber uint64) (bool, error) {
	// open the database
	db, err := sql.Open("sqlite3", utils.Config.DatabaseLocation)
	if err != nil {
		fmt.Printf("ERR: Could not open database, error: %v\n", err)
		return false, err
	}
	defer db.Close()

	selectSQL := `SELECT id, number
			FROM checkpoints 
			WHERE number = ?`

	// prepare the SQL
	statement, err := db.Prepare(selectSQL)
	if err != nil {
		fmt.Printf("ERR: Error while preparing SQL statement, error: %v\n", err)
		return false, err
	}
	defer statement.Close()

	// query the database
	rows, err := statement.Query(checkpointNumber)
	if err != nil {
		fmt.Printf("ERR: Error while checking if checkpoint exists in db, error: %v\n", err)
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var id int
		var number int

		err = rows.Scan(&id, &number)
		if err != nil {
			fmt.Printf("ERR: Error while reading row from db, error: %v\n", err)
			return false, err
		}

		// if there were no errors, then we found it - return true
		return true, nil
	}

	return false, nil
}

// InsertCheckpoint inserts a new checkpoint in the database given the passed
// NewHeaderBlockEvent struct and timestamp.
func InsertCheckpoint(headerEvent utils.NewHeaderBlockEvent, timestamp uint64) error {
	// check if checkpoint already exists in db
	checkpointExists, err := checkIfCheckpointExists(headerEvent.HeaderBlockId.Uint64())
	if err != nil {
		return err
	}

	// if checkpoint already exists in the db, return
	if checkpointExists {
		return nil
	}

	// get proposer validator id
	proposerId, err := getValidatorIdDB(headerEvent.ProposerAddress.String())
	if err != nil {
		switch err.(type) {
		case *utils.ValidatorNotFoundError:
			// in case we cannot find the proposer, set the proposer ID to -1
			// and insert a blank validator
			proposerId = -1
			fmt.Printf("WARN: Could not find validator ID for proposer with signing key %s. The signing key has most likely been changed.\n", headerEvent.ProposerAddress.String())
			err2 := insertBlankValidator()
			if err2 != nil {
				return err2
			}
		default:
			return err
		}
	}

	if !checkpointExists {
		// open the database
		db, err := sql.Open("sqlite3", utils.Config.DatabaseLocation)
		if err != nil {
			fmt.Printf("ERR: Could not open database, error: %v\n", err)
			return err
		}
		defer db.Close()

		insertSQL := `INSERT INTO checkpoints(number, block_number, timestamp, proposer_id, reward)
				VALUES(?, ?, ?, ?, ?)`

		statement, err := db.Prepare(insertSQL)
		if err != nil {
			fmt.Printf("ERR: Error while preparing SQL statement, error: %v\n", err)
			return err
		}
		defer statement.Close()

		_, err = statement.Exec(headerEvent.HeaderBlockId.Int64(), headerEvent.BlockNumber, timestamp, proposerId, headerEvent.Reward.Int64())
		if err != nil {
			fmt.Printf("ERR: Error while executing checkpoint insert, error: %v\n", err)
			return err
		}

		return nil
	}
	// if checkpoint is already in db, nothing to do, return
	return nil
}

// InsertValidatorsSignedCheckpoint updates the validators signed checkpoints
// table with the respective signers for the given checkpoint number. If temp is
// true, they are inserted in the temporary table instead.
func InsertValidatorsSignedCheckpoint(checkpointNumber uint64, signers []string, temp bool) error {
	// if we are not tracking any validators and we are not inserting in temp,
	// then there is nothing to do
	if len(utils.Config.PublicKeys) == 0 && !temp {
		fmt.Println("WARN: No public keys provided in config to track. The tool will not be tracking the performance of any validator.")
		return nil
	}

	// see if we are tracking all validators
	trackAll := utils.CheckIfTrackAll()

	// get checkpoint id from database
	checkpointId, err := getCheckpointId(checkpointNumber)
	if err != nil {
		return err
	}

	db, err := sql.Open("sqlite3", utils.Config.DatabaseLocation)
	if err != nil {
		fmt.Printf("ERR: Could not open database, error: %v\n", err)
		return err
	}
	defer db.Close()

	insertSQL := ``

	// different SQL if inserting into temp
	if !temp {
		insertSQL = `INSERT INTO validators_signed_checkpoints(checkpoint_id, validator_id)
			VALUES(?, ?)`
	} else {
		insertSQL = `INSERT INTO temp_validators_signed_checkpoints(checkpoint_id, validator_id)
			VALUES(?, ?)`
	}

	statement, err := db.Prepare(insertSQL)
	if err != nil {
		fmt.Printf("ERR: Error while preparing SQL statement, error: %v\n", err)
		return err
	}
	defer statement.Close()

	for _, validator := range signers {
		if temp || trackAll || utils.ContainsString(utils.Config.PublicKeys, validator) {
			// get validator id from database
			validatorId, err := getValidatorIdDB(validator)
			validatorFound := true
			if err != nil {
				switch err.(type) {
				case *utils.ValidatorNotFoundError:
					fmt.Printf("WARN: Could not find validator with signer key %s in database. This validator most likely changed the signing key.\n", validator)
					validatorFound = false
				default:
					return err
				}
			}

			if validatorFound {
				alreadyInserted := false
				if temp {
					// check if we have already inserted this checkpointId validatorId combo
					alreadyInserted, err = checkIfValidatorInTemp(checkpointId, validatorId)
					if err != nil {
						return err
					}
				} else {
					alreadyInserted, err = checkIfValidatorInSigned(checkpointId, validatorId)
					if err != nil {
						return err
					}
				}

				// if we have already inserted, do not insert again
				if alreadyInserted {
					continue
				}

				_, err = statement.Exec(checkpointId, validatorId)
				if err != nil {
					fmt.Printf("ERR: Error while executing checkpoint and validator insert, error: %v\n", err)
					return err
				}
			}
		}
	}

	return nil
}

// GetFirstMissedCheckpointRange gets the first checkpoint a particular
// validator missed within the range provided.
func GetFirstMissedCheckpointRange(signerKey string, startNumber int, endNumber int) (int, error) {
	// first get the validator's id
	validatorId, err := getValidatorIdDB(signerKey)
	if err != nil {
		return 0, err
	}

	// open the database
	db, err := sql.Open("sqlite3", utils.Config.DatabaseLocation)
	if err != nil {
		fmt.Printf("ERR: Could not open database, error: %v\n", err)
		return 0, err
	}
	defer db.Close()

	selectSQL := `SELECT MIN(c.number)
				FROM checkpoints c
				LEFT JOIN temp_validators_signed_checkpoints vc
				ON vc.checkpoint_id = c.id AND vc.validator_id = ?
				WHERE c.number >= ?
				AND c.number <= ?
				AND vc.checkpoint_id IS NULL`

	statement, err := db.Prepare(selectSQL)
	if err != nil {
		fmt.Printf("ERR: Error while preparing SQL statement, error: %v\n", err)
		return 0, err
	}
	defer statement.Close()

	rows, err := statement.Query(validatorId, startNumber, endNumber)
	if err != nil {
		fmt.Printf("ERR: Error while querying for the first missed checkpoint in range, error: %v\n", err)
		return 0, err
	}
	defer rows.Close()

	for rows.Next() {
		var checkpointNumber int

		err = rows.Scan(&checkpointNumber)
		if err != nil {
			fmt.Printf("ERR: Error while reading row from db, error: %v\n", err)
			return 0, err
		}

		return checkpointNumber, nil
	}

	fmt.Printf("ERR: Could not find first missed checkpoint for validator %v in database.\n", signerKey)
	return 0, &utils.CheckpointNotFoundError{GenericError: utils.GenericError{Message: "first missed checkpoint not found"}}
}

// CheckIfCheckpointExistsInTemp checks in the passed checkpointNumber exists in
// the temporary table.
func CheckIfCheckpointExistsInTemp(checkpointNumber uint64) (bool, error) {

	// get checkpoint id from database
	checkpointId, err := getCheckpointId(checkpointNumber)
	if err != nil {
		return false, err
	}

	db, err := sql.Open("sqlite3", utils.Config.DatabaseLocation)
	if err != nil {
		fmt.Printf("ERR: Could not open database, error: %v\n", err)
		return false, err
	}
	defer db.Close()

	selectSQL := `SELECT checkpoint_id, validator_id
			FROM temp_validators_signed_checkpoints 
			WHERE checkpoint_id = ?`

	statement, err := db.Prepare(selectSQL)
	if err != nil {
		fmt.Printf("ERR: Error while preparing SQL statement, error: %v\n", err)
		return false, err
	}
	defer statement.Close()

	rows, err := statement.Query(checkpointId)
	if err != nil {
		fmt.Printf("ERR: Error while checking if checkpoint exists in temporary db, error: %v\n", err)
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var checkpoint_id sql.NullInt64
		var validator_id sql.NullInt64

		err = rows.Scan(&checkpoint_id, &validator_id)
		if err != nil {
			fmt.Printf("ERR: Error while reading row from db, error: %v\n", err)
			return false, err
		}
		if checkpoint_id.Valid {
			return true, nil
		} else {
			return false, nil
		}
	}

	return false, nil
}

// GetSignedCheckpointsCountPerValidator queries the temporary validators
// signed checkpoints table to get a count of how many checkpoints each
// validator signed.
func GetSignedCheckpointsCountPerValidator(startNumber int, endNumber int) (int, map[int]int, error) {
	// get the number of checkpoints in range
	numOfCheckpoints, err := getNumberOfCheckpointsBetweenRange(startNumber, endNumber)
	if err != nil {
		return 0, nil, err
	}

	db, err := sql.Open("sqlite3", utils.Config.DatabaseLocation)
	if err != nil {
		fmt.Printf("ERR: Could not open database, error: %v\n", err)
		return 0, nil, err
	}
	defer db.Close()

	selectSQL := `SELECT v.id, COUNT(*) 
			FROM temp_validators_signed_checkpoints vc
			LEFT JOIN checkpoints c
			ON vc.checkpoint_id = c.id
			LEFT JOIN validators v
			ON vc.validator_id = v.id
			WHERE c.number >= ?
			AND c.number <= ?
			GROUP BY v.id`

	statement, err := db.Prepare(selectSQL)
	if err != nil {
		fmt.Printf("ERR: Error while preparing SQL statement, error: %v\n", err)
		return 0, nil, err
	}
	defer statement.Close()

	rows, err := statement.Query(startNumber, endNumber)
	if err != nil {
		fmt.Printf("ERR: Error while querying for validators' signed checkpoints in range, error: %v\n", err)
		return 0, nil, err
	}
	defer rows.Close()

	results := map[int]int{}
	for rows.Next() {
		var validatorId int
		var count int

		err = rows.Scan(&validatorId, &count)
		if err != nil {
			fmt.Printf("ERR: Error while reading row from db, error: %v\n", err)
			return 0, nil, err
		}
		results[validatorId] = count
	}

	return numOfCheckpoints, results, nil
}

// DeleteTempCheckpoints deletes all checkpoints from the temporary table which
// have a checkpoint number smaller than the one passed. For tracked validators,
// that data is still available in the validators_signed_checkpoints table.
func DeleteTempCheckpoints(endNumber uint64) error {
	db, err := sql.Open("sqlite3", utils.Config.DatabaseLocation)
	if err != nil {
		fmt.Printf("ERR: Could not open database, error: %v\n", err)
		return err
	}
	defer db.Close()

	deleteSQL := `DELETE FROM temp_validators_signed_checkpoints
				WHERE checkpoint_id IN (
					SELECT id
					FROM checkpoints
					WHERE number < ?
				)`

	statement, err := db.Prepare(deleteSQL)
	if err != nil {
		fmt.Printf("ERR: Error while preparing SQL statement, error: %v\n", err)
		return err
	}
	defer statement.Close()

	_, err = statement.Exec(endNumber)
	if err != nil {
		fmt.Printf("ERR: Error while deleting checkpoints from the temporary signed checkpoints table, error: %v\n", err)
		return err
	}

	return nil
}

// InsertPerformanceBenchmark inserts the performance benchmark for the given
// checkpoint number in the checkpoints table.
func InsertPerformanceBenchmark(pb float64, checkpointNumber int) error {
	db, err := sql.Open("sqlite3", utils.Config.DatabaseLocation)
	if err != nil {
		fmt.Printf("ERR: Could not open database, error: %v\n", err)
		return err
	}
	defer db.Close()

	insertSQL := `UPDATE checkpoints
			SET performance_benchmark = ?
			WHERE number = ?`

	statement, err := db.Prepare(insertSQL)
	if err != nil {
		fmt.Printf("ERR: Error while preparing SQL statement, error: %v\n", err)
		return err
	}
	defer statement.Close()

	_, err = statement.Exec(pb, checkpointNumber)
	if err != nil {
		fmt.Printf("ERR: Error while executing performance benchmark insert, error: %v\n", err)
		return err
	}

	return nil
}

// GetLastCheckpointNumber gets the last / largest checkpoint number in the
// checkpoints table.
func GetLastCheckpointNumber() (int, error) {
	db, err := sql.Open("sqlite3", utils.Config.DatabaseLocation)
	if err != nil {
		fmt.Printf("ERR: Could not open database, error: %v\n", err)
		return 0, err
	}
	defer db.Close()

	selectSQL := `SELECT MAX(number)
			FROM checkpoints`

	statement, err := db.Prepare(selectSQL)
	if err != nil {
		fmt.Printf("ERR: Error while preparing SQL statement, error: %v\n", err)
		return 0, err
	}
	defer statement.Close()

	rows, err := statement.Query()
	if err != nil {
		fmt.Printf("ERR: Error while querying for maximum checkpoint number, error: %v\n", err)
		return 0, err
	}
	defer rows.Close()

	for rows.Next() {
		var checkpointNumber sql.NullInt64

		err = rows.Scan(&checkpointNumber)
		if err != nil {
			fmt.Printf("ERR: Error while reading row from db, error: %v\n", err)
			return 0, err
		}
		if checkpointNumber.Valid {
			return int(checkpointNumber.Int64), nil
		} else {
			return 0, sql.ErrNoRows
		}
	}

	return 0, &utils.CheckpointNotFoundError{GenericError: utils.GenericError{Message: "maximum checkpoint number not found, the checkpoints table might be empty"}}
}

// getNumberOfCheckpointsBetweenRange returns the number of rows between two
// numbers, both inclusive.
func getNumberOfCheckpointsBetweenRange(startNumber int, endNumber int) (int, error) {
	db, err := sql.Open("sqlite3", utils.Config.DatabaseLocation)
	if err != nil {
		fmt.Printf("ERR: Could not open database, error: %v\n", err)
		return 0, err
	}
	defer db.Close()

	selectSQL := `SELECT COUNT(*)
			FROM checkpoints
			WHERE number >= ? AND number <= ?`

	statement, err := db.Prepare(selectSQL)
	if err != nil {
		fmt.Printf("ERR: Error while preparing SQL statement, error: %v\n", err)
		return 0, err
	}
	defer statement.Close()

	rows, err := statement.Query(startNumber, endNumber)
	if err != nil {
		fmt.Printf("ERR: Error while querying for number of checkpoints between range, error: %v\n", err)
		return 0, err
	}
	defer rows.Close()

	for rows.Next() {
		var number int

		err = rows.Scan(&number)
		if err != nil {
			fmt.Printf("ERR: Error while reading row from db, error: %v\n", err)
			return 0, err
		}

		return number, nil
	}

	return 0, &utils.CheckpointNotFoundError{GenericError: utils.GenericError{Message: "error while trying to get count of checkpoints between two numbers"}}
}

// getSignedCheckpointsCount gets the number of signed checkpoints for the given
// signer key, for the given range. The validator must be tracked, as this
// does not query the temporary table.
func getSignedCheckpointsCount(startNumber int, endNumber int, signerKey string) (int, error) {
	db, err := sql.Open("sqlite3", utils.Config.DatabaseLocation)
	if err != nil {
		fmt.Printf("ERR: Could not open database, error: %v\n", err)
		return 0, err
	}
	defer db.Close()

	selectSQL := `SELECT COUNT(*)
			FROM validators_signed_checkpoints vc
			LEFT JOIN checkpoints c
			ON vc.checkpoint_id = c.id
			LEFT JOIN validators v
			ON vc.validator_id = v.id
			WHERE c.number >= ?
			AND c.number <= ?
			AND v.signer_key LIKE ?`

	statement, err := db.Prepare(selectSQL)
	if err != nil {
		fmt.Printf("ERR: Error while preparing SQL statement, error: %v\n", err)
		return 0, err
	}
	defer statement.Close()

	rows, err := statement.Query(startNumber, endNumber, signerKey)
	if err != nil {
		fmt.Printf("ERR: Error while querying for number of signed checkpoints in range, error: %v\n", err)
		return 0, err
	}
	defer rows.Close()

	for rows.Next() {
		var count int

		err = rows.Scan(&count)
		if err != nil {
			fmt.Printf("ERR: Error while reading row from db, error: %v\n", err)
			return 0, err
		}
		return count, nil
	}

	return 0, &utils.GenericError{Message: "error while trying to get signed checkpoints by public key"}
}

// GetLastBlockNumber gets the last block number from the last checkpoint in the
// database.
func GetLastBlockNumber() (uint64, error) {
	db, err := sql.Open("sqlite3", utils.Config.DatabaseLocation)
	if err != nil {
		fmt.Printf("ERR: Could not open database, error: %v\n", err)
		return 0, err
	}
	defer db.Close()

	selectSQL := `SELECT MAX(block_number)
			FROM checkpoints`

	statement, err := db.Prepare(selectSQL)
	if err != nil {
		fmt.Printf("ERR: Error while preparing SQL statement, error: %v\n", err)
		return 0, err
	}
	defer statement.Close()

	rows, err := statement.Query()
	if err != nil {
		fmt.Printf("ERR: Error while querying for maximum block number, error: %v\n", err)
		return 0, err
	}
	defer rows.Close()

	for rows.Next() {
		var blockNumber sql.NullInt64

		err = rows.Scan(&blockNumber)
		if err != nil {
			fmt.Printf("ERR: Error while reading row from db, error: %v\n", err)
			return 0, err
		}
		if blockNumber.Valid {
			return uint64(blockNumber.Int64), nil
		} else {
			return 0, sql.ErrNoRows
		}
	}

	return 0, &utils.CheckpointNotFoundError{GenericError: utils.GenericError{Message: "maximum block number not found, the checkpoints table might be empty"}}
}

// GetPBAtCheckpoint gets the performance benchmark at the provided checkpoint
// number.
func GetPBAtCheckpoint(checkpointNumber int) (float64, error) {
	db, err := sql.Open("sqlite3", utils.Config.DatabaseLocation)
	if err != nil {
		fmt.Printf("ERR: Could not open database, error: %v\n", err)
		return 0, err
	}
	defer db.Close()

	selectSQL := `SELECT performance_benchmark
			FROM checkpoints
			WHERE number = ?`

	statement, err := db.Prepare(selectSQL)
	if err != nil {
		fmt.Printf("ERR: Error while preparing SQL statement, error: %v\n", err)
		return 0, err
	}
	defer statement.Close()

	rows, err := statement.Query(checkpointNumber)
	if err != nil {
		fmt.Printf("ERR: Error while getting maximum validator id from database, error: %v\n", err)
		return 0, err
	}
	defer rows.Close()

	for rows.Next() {
		var performanceBenchmark sql.NullFloat64

		err = rows.Scan(&performanceBenchmark)
		if err != nil {
			fmt.Printf("ERR: Error while reading row from db, error: %v\n", err)
			return 0, err
		}
		if performanceBenchmark.Valid {
			return performanceBenchmark.Float64, nil
		} else {
			return 0, sql.ErrNoRows
		}
	}

	return 0, &utils.CheckpointNotFoundError{GenericError: utils.GenericError{Message: "performance benchmark for given checkpoint not found, the checkpoints table might be empty"}}

}

// getCheckpointPerformanceRangeOnly gets the number of signed checkpoints by
// a validator for the given range.
func getCheckpointPerformanceRangeOnly(startNumber int, endNumber int, publicKey string) (int, error) {

	// get the number of checkpoints the passed public key signed for the same period
	numSignedCheckpoints, err := getSignedCheckpointsCount(startNumber, endNumber, publicKey)
	if err != nil {
		return 0, err
	}

	return numSignedCheckpoints, nil
}

// getCheckpointPerformanceRangeAll gets the number of signed checkpoints by
// all the validators that are tracked, for the given range.
func getCheckpointPerformanceRangeAll(startNumber int, endNumber int) (int, map[string]int, error) {

	// get the number of checkpoints in range
	numOfCheckpoints, err := getNumberOfCheckpointsBetweenRange(startNumber, endNumber)
	if err != nil {
		return 0, nil, err
	}

	if len(utils.Config.PublicKeys) == 0 {
		// no keys to check performance for, just return number of checkpoints in range
		return numOfCheckpoints, nil, nil
	}

	trackAll := false
	if len(utils.Config.PublicKeys) == 1 {
		// if we have a '*', then we are monitoring the performance of all validators
		if utils.Config.PublicKeys[0] == "*" {
			trackAll = true
		}
	}

	if trackAll {
		// in this case, get all validators' performance
		publicKeys, err := getAllValidatorsSignerKeys()
		if err != nil {
			return 0, nil, err
		}
		if len(publicKeys) == 0 {
			// no pubkeys to check, return num of checpoints only
			return numOfCheckpoints, nil, nil
		} else {
			results := map[string]int{}

			for _, publicKey := range publicKeys {
				signedCheckpoints, err := getCheckpointPerformanceRangeOnly(startNumber, endNumber, publicKey)
				if err != nil {
					return numOfCheckpoints, nil, err
				}
				results[publicKey] = signedCheckpoints
			}

			return numOfCheckpoints, results, nil
		}
	} else {
		results := map[string]int{}
		for _, publicKey := range utils.Config.PublicKeys {
			signedCheckpoints, err := getCheckpointPerformanceRangeOnly(startNumber, endNumber, publicKey)
			if err != nil {
				return numOfCheckpoints, nil, err
			}
			results[publicKey] = signedCheckpoints
		}
		return numOfCheckpoints, results, nil
	}
}

// GetCheckpointCount is a helper function that calls another function to get
// the total checkpoints and how many of those were signed by the validators
// we are tracking, for the range provided.
func GetCheckpointCount(startNumber int, endNumber int) (int, map[string]int, error) {
	totalCheckpoints, signedCheckpointsMap, err := getCheckpointPerformanceRangeAll(startNumber, endNumber)
	if err != nil {
		return 0, nil, err
	}
	return totalCheckpoints, signedCheckpointsMap, nil
}
