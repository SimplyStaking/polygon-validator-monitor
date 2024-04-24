package database

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"monitor/internal/utils"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/maticnetwork/heimdall/contracts/stakemanager"
	_ "github.com/mattn/go-sqlite3"
)

// GetValidator gets the validator by its id, and returns all the information
// in a Validator struct.
func GetValidator(validatorId int) (utils.Validator, error) {
	// open the database
	db, err := sql.Open("sqlite3", utils.Config.DatabaseLocation)
	if err != nil {
		fmt.Printf("ERR: Could not open database, error: %v\n", err)
		return utils.Validator{}, err
	}
	defer db.Close()

	selectSQL := `SELECT id, owner_key, signer_key, activation_epoch, deactivation_epoch
			FROM validators 
			WHERE id = ?`

	// prepare the SQL statement
	statement, err := db.Prepare(selectSQL)
	if err != nil {
		fmt.Printf("ERR: Error while preparing SQL statement, error: %v\n", err)
		return utils.Validator{}, err
	}
	defer statement.Close()

	// query the database
	rows, err := statement.Query(validatorId)
	if err != nil {
		fmt.Printf("ERR: Error while checking if validator exists in db, error: %v\n", err)
		return utils.Validator{}, err
	}
	defer rows.Close()

	for rows.Next() {
		// prepare the variables
		var validatorId int
		var activationEpoch int
		var deactivationEpoch int
		var ownerKey string
		var signerKey string

		// populate the variables
		err = rows.Scan(&validatorId, &ownerKey, &signerKey, &activationEpoch, &deactivationEpoch)
		if err != nil {
			fmt.Printf("ERR: Error while reading row from db, error: %v\n", err)
			return utils.Validator{}, err
		}

		// prepare the struct
		validator := utils.Validator{
			ValidatorId:       validatorId,
			OwnerAddress:      common.HexToAddress(ownerKey),
			SignerAddress:     common.HexToAddress(signerKey),
			ActivationEpoch:   uint64(activationEpoch),
			DeactivationEpoch: uint64(deactivationEpoch),
		}

		return validator, nil
	}

	// in case of no rows, return an error
	return utils.Validator{}, &utils.ValidatorNotFoundError{GenericError: utils.GenericError{Message: "validator was not found in database"}}
}

// getMaxValidatorId gets the largest validator ID in the validators table in
// the database.
func getMaxValidatorId() (int, error) {
	// open the database
	db, err := sql.Open("sqlite3", utils.Config.DatabaseLocation)
	if err != nil {
		fmt.Printf("ERR: Could not open database, error: %v\n", err)
		return 0, err
	}
	defer db.Close()

	selectSQL := `SELECT MAX(id)
			FROM validators`

	// prepare the SQL query
	statement, err := db.Prepare(selectSQL)
	if err != nil {
		fmt.Printf("ERR: Error while preparing SQL statement, error: %v\n", err)
		return 0, err
	}
	defer statement.Close()

	// query the database
	rows, err := statement.Query()
	if err != nil {
		fmt.Printf("ERR: Error while getting maximum validator id from database, error: %v\n", err)
		return 0, err
	}
	defer rows.Close()

	for rows.Next() {
		var maxValId sql.NullInt64

		// store the value in the variable
		err = rows.Scan(&maxValId)
		if err != nil {
			fmt.Printf("ERR: Error while reading row from db, error: %v\n", err)
			return 0, err
		}
		if maxValId.Valid {
			// if we have a valid value, return it
			return int(maxValId.Int64), nil
		} else {
			// in case of a null value, return an error representing this
			return 0, sql.ErrNoRows
		}
	}

	// in case we get here, return an error
	return 0, &utils.ValidatorNotFoundError{GenericError: utils.GenericError{Message: "maximum validator number not found, the validators table might be empty"}}
}

// getValidatorIdDB gets the ID of the validator with the provided signer key.
func getValidatorIdDB(signerKey string) (int, error) {
	// open the database
	db, err := sql.Open("sqlite3", utils.Config.DatabaseLocation)
	if err != nil {
		fmt.Printf("ERR: Could not open database, error: %v\n", err)
		return 0, err
	}
	defer db.Close()

	selectSQL := `SELECT id
			FROM validators 
			WHERE signer_key LIKE ?`

	// prepare the SQL statement
	statement, err := db.Prepare(selectSQL)
	if err != nil {
		fmt.Printf("ERR: Error while preparing SQL statement, error: %v\n", err)
		return 0, err
	}
	defer statement.Close()

	// query the database
	rows, err := statement.Query(signerKey)
	if err != nil {
		fmt.Printf("ERR: Error while querying for validator id using public key, error: %v\n", err)
		return 0, err
	}
	defer rows.Close()

	for rows.Next() {
		var id int

		// get the id
		err = rows.Scan(&id)
		if err != nil {
			fmt.Printf("ERR: Error while reading row from db, error: %v\n", err)
			return 0, err
		}

		return id, nil
	}

	// in case of no rows, return relevant error
	fmt.Printf("ERR: Could not find validator with public key %v in database.\n", signerKey)
	return 0, &utils.ValidatorNotFoundError{GenericError: utils.GenericError{Message: "validator with provided pubkey not found"}}
}

// getAllValidatorsSignerKeys gets all the signer keys of all the validators in
// the database.
func getAllValidatorsSignerKeys() ([]string, error) {
	// open the database
	db, err := sql.Open("sqlite3", utils.Config.DatabaseLocation)
	if err != nil {
		fmt.Printf("ERR: Could not open database, error: %v\n", err)
		return nil, err
	}
	defer db.Close()

	selectSQL := `SELECT signer_key
			FROM validators`

	// prepare the SQL query
	statement, err := db.Prepare(selectSQL)
	if err != nil {
		fmt.Printf("ERR: Error while preparing SQL statement, error: %v\n", err)
		return nil, err
	}
	defer statement.Close()

	// query the database
	rows, err := statement.Query()
	if err != nil {
		fmt.Printf("ERR: Error while querying for validators, error: %v\n", err)
		return nil, err
	}
	defer rows.Close()

	var results []string // slice to hold signer keys
	for rows.Next() {
		var publicKey string

		err = rows.Scan(&publicKey)
		if err != nil {
			fmt.Printf("ERR: Error while reading row from db, error: %v\n", err)
			return nil, err
		}

		// append each signer key to slice
		results = append(results, publicKey)
	}

	return results, nil
}

// insertValidator inserts the passed validator in the database.
func insertValidator(validator utils.Validator) error {
	// open the database
	db, err := sql.Open("sqlite3", utils.Config.DatabaseLocation)
	if err != nil {
		fmt.Printf("ERR: Could not open database, error: %v\n", err)
		return err
	}
	defer db.Close()

	// insert validator SQL
	insertSQL := `INSERT INTO validators(id, owner_key, signer_key, activation_epoch, deactivation_epoch)
			VALUES(?, ?, ?, ?, ?)`

	// prepare the SQL
	statement, err := db.Prepare(insertSQL)
	if err != nil {
		fmt.Printf("ERR: Error while preparing SQL statement, error: %v\n", err)
		return err
	}
	defer statement.Close()

	// execute the SQL
	_, err = statement.Exec(validator.ValidatorId, validator.OwnerAddress.String(), validator.SignerAddress.String(), validator.ActivationEpoch, validator.DeactivationEpoch)
	if err != nil {
		fmt.Printf("ERR: Error while executing validator insert, error: %v\n", err)
		return err
	}

	return nil
}

// updateValidator updates the passed validator in the database.
func updateValidator(validator utils.Validator) error {
	// open the database
	db, err := sql.Open("sqlite3", utils.Config.DatabaseLocation)
	if err != nil {
		fmt.Printf("ERR: Could not open database, error: %v\n", err)
		return err
	}
	defer db.Close()

	// update validator SQL
	updateSQL := `UPDATE validators
			SET owner_key = ?, signer_key = ?, activation_epoch = ?, deactivation_epoch = ?
			WHERE id = ?`

	// prepare the SQL
	statement, err := db.Prepare(updateSQL)
	if err != nil {
		fmt.Printf("ERR: Error while preparing SQL statement, error: %v\n", err)
		return err
	}
	defer statement.Close()

	// execute the SQL
	_, err = statement.Exec(validator.OwnerAddress.String(), validator.SignerAddress.String(), validator.ActivationEpoch, validator.DeactivationEpoch, validator.ValidatorId)
	if err != nil {
		fmt.Printf("ERR: Error while executing validator insert, error: %v\n", err)
		return err
	}

	return nil
}

// insertBlankValidator caters for the rare situation where the proposer of a
// checkpoint is not found in the database. In such case, this blank validator
// would be the proposer of that checkpoint.
func insertBlankValidator() error {
	// open the database
	db, err := sql.Open("sqlite3", utils.Config.DatabaseLocation)
	if err != nil {
		fmt.Printf("ERR: Could not open database, error: %v\n", err)
		return err
	}
	defer db.Close()

	// insert validator SQL
	insertSQL := `INSERT OR IGNORE INTO validators(id, owner_key, signer_key, activation_epoch, deactivation_epoch)
			VALUES(-1, 0x0000000000000000000000000000000000000000, 0x0000000000000000000000000000000000000000, 0, 0)`

	// prepare the SQL
	statement, err := db.Prepare(insertSQL)
	if err != nil {
		fmt.Printf("ERR: Error while preparing SQL statement, error: %v\n", err)
		return err
	}
	defer statement.Close()

	// execute the statement
	_, err = statement.Exec()
	if err != nil {
		fmt.Printf("ERR: Error while executing validator insert, error: %v\n", err)
		return err
	}

	return nil
}

// checkIfValidatorInSigned checks if the passed validator is in the validators
// signed checkpoints table for the given checkpoint.
func checkIfValidatorInSigned(checkpointId int, validatorId int) (bool, error) {
	// open the database
	db, err := sql.Open("sqlite3", utils.Config.DatabaseLocation)
	if err != nil {
		fmt.Printf("ERR: Could not open database, error: %v\n", err)
		return false, err
	}
	defer db.Close()

	selectSQL := `SELECT id
			FROM validators_signed_checkpoints
			WHERE checkpoint_id = ? AND validator_id = ?`

	// prepare the SQL
	statement, err := db.Prepare(selectSQL)
	if err != nil {
		fmt.Printf("ERR: Error while preparing SQL statement, error: %v\n", err)
		return false, err
	}
	defer statement.Close()

	// query the database
	rows, err := statement.Query(checkpointId, validatorId)
	if err != nil {
		fmt.Printf("ERR: Error while checking if validators exists in signed db, error: %v\n", err)
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var id sql.NullInt64

		err = rows.Scan(&id)
		if err != nil {
			fmt.Printf("ERR: Error while reading row from db, error: %v\n", err)
			return false, err
		}
		if id.Valid {
			// if the id is valid and not null, return true
			return true, nil
		} else {
			// if the result is null, return false
			return false, nil
		}
	}

	return false, nil
}

// checkIfValidatorInTemp checks if the passed validator is in the temporary
// signed checkpoints table for the given checkpoint.
func checkIfValidatorInTemp(checkpointId int, validatorId int) (bool, error) {
	// open the database
	db, err := sql.Open("sqlite3", utils.Config.DatabaseLocation)
	if err != nil {
		fmt.Printf("ERR: Could not open database, error: %v\n", err)
		return false, err
	}
	defer db.Close()

	selectSQL := `SELECT id
			FROM temp_validators_signed_checkpoints
			WHERE checkpoint_id = ? AND validator_id = ?`

	// prepare the SQL
	statement, err := db.Prepare(selectSQL)
	if err != nil {
		fmt.Printf("ERR: Error while preparing SQL statement, error: %v\n", err)
		return false, err
	}
	defer statement.Close()

	// query the database
	rows, err := statement.Query(checkpointId, validatorId)
	if err != nil {
		fmt.Printf("ERR: Error while checking if validators exists in temporary db, error: %v\n", err)
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var id sql.NullInt64

		err = rows.Scan(&id)
		if err != nil {
			fmt.Printf("ERR: Error while reading row from db, error: %v\n", err)
			return false, err
		}
		if id.Valid {
			// if the id is valid and not null, return true
			return true, nil
		} else {
			// if the result is null, return false
			return false, nil
		}
	}

	return false, nil
}

// getDeactivatedValidators returns IDs of validators whose deactivation epoch
// is smaller than the passed epoch (checkpoint).
func getDeactivatedValidators(checkpoint int) ([]int, error) {
	db, err := sql.Open("sqlite3", utils.Config.DatabaseLocation)
	if err != nil {
		fmt.Printf("ERR: Could not open database, error: %v\n", err)
		return nil, err
	}
	defer db.Close()

	// deactivation_epoch = 0 usually implies the validator is still active
	selectSQL := `SELECT id
			FROM validators
			WHERE deactivation_epoch != 0
			AND deactivation_epoch <= ?`

	// prepare the SQL
	statement, err := db.Prepare(selectSQL)
	if err != nil {
		fmt.Printf("ERR: Error while preparing SQL statement, error: %v\n", err)
		return nil, err
	}
	defer statement.Close()

	// query the database
	rows, err := statement.Query(checkpoint)
	if err != nil {
		fmt.Printf("ERR: Error while querying for maximum block number, error: %v\n", err)
		return nil, err
	}
	defer rows.Close()

	validators := []int{}
	for rows.Next() {
		var validatorId sql.NullInt64

		err = rows.Scan(&validatorId)
		if err != nil {
			fmt.Printf("ERR: Error while reading row from db, error: %v\n", err)
			return nil, err
		}

		// if the validator is valid, add it to the results
		if validatorId.Valid {
			validators = append(validators, int(validatorId.Int64))
		} else {
			// in case of null, return error
			return nil, sql.ErrNoRows
		}
	}

	if len(validators) > 0 {
		return validators, nil
	} else {
		return nil, &utils.ValidatorNotFoundError{GenericError: utils.GenericError{Message: "no validators found with given criteria"}}
	}

}

// getAndInsertValidators calls other functions to fetch information about
// validators from the StakeManager smart contract. It then calls another
// function to update the fetched values in the database.
func getAndInsertValidators(blockNumber uint64, validatorStartingId int, deactivedValidators []int) error {
	var ethRPCClient *rpc.Client
	var err error

	// try to reach the ETH node
	for i := 0; i < utils.RETRIES; i++ {
		ethRPCClient, err = rpc.Dial(utils.Config.ETHRpcUrl)
		if err == nil {
			break
		}

		time.Sleep(time.Second * utils.RETRY_WAIT)
	}

	if err != nil {
		log.Printf("ERR: Unable to dial ETH node (%s), error: %v\n", utils.Config.ETHRpcUrl, err)
		return &utils.DialError{GenericError: utils.GenericError{Message: "unable to dial ETH node"}}
	}
	ethClient := ethclient.NewClient(ethRPCClient)

	contractAddress := common.HexToAddress(utils.STAKEMANAGER_ADDRESS)

	stakeManagerABI := abi.ABI{}

	// get StakeManager ABI to decode encode query and decode response
	if ccAbi, err := utils.GetABI(stakemanager.StakemanagerABI); err != nil {
		log.Printf("ERR: Error while fetching StakeManager ABI, error: %v\n", err)
		return errors.New("unable to fetch StakeManager ABI")
	} else {
		stakeManagerABI = ccAbi
	}

	// to store the results
	var validators []utils.Validator

	// check if the validators table is not empty, to get an estimate on number of requests
	valTableEmpty, err := ValidatorTableEmpty()
	if err != nil {
		return err
	}

	// at the time of this tool's development, there are 171 validator ids
	// if we do not have a value in the database, use this one
	lastValidatorId := 171
	if !valTableEmpty {
		lastValidatorId, err = getMaxValidatorId()
		if err != nil {
			return err
		}
	}

	validators = []utils.Validator{}
	if lastValidatorId == 0 {
		// non-concurrent part, basically deprecated as it can never enter here
		for ii := validatorStartingId; ; ii++ {
			if !utils.Contains(deactivedValidators, ii) {
				val, err := utils.GetValidatorInfoStakeManager(ii, int(blockNumber), stakeManagerABI, *ethClient, contractAddress)
				if err != nil {
					return err
				}
				if val.ValidatorId == -1 { // our termination condition
					break
				} else {
					validators = append(validators, val)
				}
			}
		}
	} else {
		// concurrent
		var wg sync.WaitGroup
		results := make(chan utils.ValidatorError)

		go func() {
			for ii := 1; ii <= lastValidatorId; ii++ {
				// check if validator is deactivated
				if !utils.Contains(deactivedValidators, ii) {
					wg.Add(1)
					// if not deactivated, call function to get info about
					// validator
					go utils.GetValidatorInfoStakeManagerConcurrent(ii, int(blockNumber), stakeManagerABI, *ethClient, contractAddress, results, &wg)
				}
			}
			wg.Wait()
			close(results)
		}()
		for result := range results {
			if result.Error != nil {
				// check each ValidatorError element to catch any errors
				return err
			} else if result.Validator.ValidatorId == -1 {
				// we requested for a validator id larger than the current
				// largest - this is not an issue
				continue
			} else {
				// if no error, append the validator to our slice
				validators = append(validators, result.Validator)
			}
		}

		// try for any other validators with a larger id, in case new validators
		// joined the set
		for ii := lastValidatorId + 1; ; ii++ {
			val, err := utils.GetValidatorInfoStakeManager(ii, int(blockNumber), stakeManagerABI, *ethClient, contractAddress)
			if err != nil {
				return err
			}
			if val.ValidatorId == -1 {
				// the termination condition - when we reach a validator that
				// does not exist
				break
			} else {
				validators = append(validators, val)
			}
		}
	}

	// insert or update the validators table in the database
	for _, validator := range validators {
		err = insertOrUpdateValidator(validator)
		if err != nil {
			return err
		}
	}

	return nil
}

// ValidatorTableEmpty checks if the validators table is empty or not. It
// assumes that it is not empty if validator with ID 1 is in the table.
func ValidatorTableEmpty() (bool, error) {
	_, err := GetValidator(1)
	if err != nil {
		switch err.(type) {
		case *utils.ValidatorNotFoundError:
			return true, nil
		default:
			return false, err
		}
	} else {
		return false, nil
	}
}

// insertOrUpdateValidator determines whether a passed validator requires
// updating or inserting in the database.
func insertOrUpdateValidator(validator utils.Validator) error {

	// try getting the validator first
	validatorDB, err := GetValidator(validator.ValidatorId)

	validatorFound := true
	if err != nil {
		switch err.(type) {
		case *utils.ValidatorNotFoundError:
			validatorFound = false
		default:
			return err
		}
	}

	if !validatorFound {
		// if the validator is new, insert it
		return insertValidator(validator)
	} else {
		// check if the passed validator is actually identical to the one we
		// have in the database
		if utils.CompareValidators(validator, validatorDB) {
			// if they are the same, nothing to update - return
			return nil
		}

		// check if new owner is 0x00..00 and we have an actual value in the
		// database, so that in that case we do not update it
		if validator.OwnerAddress.String() == "0x0000000000000000000000000000000000000000" && validatorDB.OwnerAddress.String() != "0x0000000000000000000000000000000000000000" {
			tempValidator := validator
			tempValidator.OwnerAddress = validatorDB.OwnerAddress

			// compare again
			if utils.CompareValidators(tempValidator, validatorDB) {
				return nil
			}

			// print out what we are updating
			fmt.Printf("INFO: Validator with ID %d is being updated: ", validator.ValidatorId)
			if validator.OwnerAddress != validatorDB.OwnerAddress {
				fmt.Printf("owner address is different; ")
			}

			if validator.SignerAddress != validatorDB.SignerAddress {
				fmt.Printf("signer address is different; ")
			}

			if validator.DeactivationEpoch != validatorDB.DeactivationEpoch {
				fmt.Printf("deactivation epoch is different;")
			}
			fmt.Println()

			return updateValidator(tempValidator)
		} else {
			// print out what we are updating
			fmt.Printf("INFO: Validator with ID %d is being updated: ", validator.ValidatorId)
			if validator.OwnerAddress != validatorDB.OwnerAddress {
				fmt.Printf("owner address is different; ")
			}

			if validator.SignerAddress != validatorDB.SignerAddress {
				fmt.Printf("signer address is different; ")
			}

			if validator.DeactivationEpoch != validatorDB.DeactivationEpoch {
				fmt.Printf("deactivation epoch is different;")
			}
			fmt.Println()
			return updateValidator(validator)
		}
	}
}

// UpdateValidatorsDB gets a list of the deactivated validators and then
// passes it to the function that inserts and updates validators.
func UpdateValidatorsDB(blockNumber uint64, checkpointNumber uint64) error {

	deactivatedVals := []int{}
	var err error
	// if the checkpointNumber is 0, it implies that the validators table does
	// not exist, or is empty
	if checkpointNumber > 0 {
		deactivatedVals, err = getDeactivatedValidators(int(checkpointNumber))
		if err != nil {
			switch err.(type) {
			case *utils.ValidatorNotFoundError: // this should imply that no validators are deactivated - which is possible depending on the checkpoint number you start from
				break
			default:
				return err
			}
		}
	}

	if len(deactivatedVals) == 0 {
		deactivatedVals = nil
	}

	// call function to update and insert validators
	err = getAndInsertValidators(blockNumber, 1, deactivatedVals)
	if err != nil {
		return err
	}
	return nil
}
