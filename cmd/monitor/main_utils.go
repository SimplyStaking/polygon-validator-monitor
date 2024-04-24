package main

import (
	"database/sql"
	"fmt"
	"math/big"
	database "monitor/internal/db"
	"monitor/internal/metrics"
	"monitor/internal/utils"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// getStartingBlock determines what ETH block we should start checking for
// checkpoints from, based on the config and the database. It returns the
// starting block.
func getStartingBlock() (uint64, error) {
	// check if the database exists
	dbExists, err := utils.CheckIfDBExists()
	if os.IsNotExist(err) {
		fmt.Println("WARN: Database file does not exist. A new database will be created.")
		err = database.CreateDatabase()
		if err != nil {
			return 0, err
		}
	} else if err != nil {
		fmt.Printf("ERR: Error while checking for database file, error: %v\n", err)
		return 0, err
	}

	// check if we are continuing from the last block in the database
	startingBlock := uint64(0)
	if utils.Config.ContinueFromBlock == 0 {
		startingBlock, err = database.GetLastBlockNumber()
		if err == sql.ErrNoRows {
			fmt.Println("WARN: No checkpoints found in database. Starting from current block - 100.")
		} else if err != nil {
			return 0, err
		}
	} else {
		startingBlock = uint64(utils.Config.ContinueFromBlock)
		if dbExists {
			// get the last block in the database (the block in which the last
			// processed checkpoint was submitted)
			lastBlockNumber, err := database.GetLastBlockNumber()
			if err != nil {
				switch {
				case err == sql.ErrNoRows:
					fmt.Printf("WARN: No checkpoints found in database. Starting from block provided in config (%d).\n", startingBlock)
					lastBlockNumber = 0
				default:
					fmt.Printf("ERR: Error while querying for last block in database, error: %v\n", err)
					return 0, err
				}
			} else {
				fmt.Printf("WARN: The database provided is not new, and monitoring will resume from the last block in the database (%d) rather than the one specified in the config (%d).\n", lastBlockNumber, startingBlock)
				fmt.Println("WARN: If you would like to start from the block number provided in the config, please delete or move the database file and restart the process.")

				// start from the last block we processed, to ensure that all information about the last processed checkpoint is consistent
				// it could be that the process was terminated while inserting, for example, leading to incorrect data
				startingBlock = lastBlockNumber
			}
		}
	}

	if startingBlock == 0 {
		// if we have no starting block, start from the current - 100
		currBlockNumber, err := utils.GetCurrentBlockNumber()
		if err != nil {
			return 0, err
		}

		startingBlock = currBlockNumber - 100
	} else {
		if utils.Config.ContinueFromBlock == 0 {
			// if we are continuing, use the last block in the database
			err := metrics.UpdateCheckpointsSignedMetrics()
			if err != nil {
				return startingBlock, err
			}
		}
	}

	return startingBlock, nil
}

// getNewEventsAndDecode is the main function that gets events between a given
// range and processes them, calling other functions to update the database and
// metrics. It returns an error in case something goes wrong.
func getNewEventsAndDecode(startBlock uint64, endBlock uint64) error {
	newHeaderBlockEvents, err := utils.DecodeEvents(startBlock, endBlock)
	if err != nil {
		switch err.(type) {
		case *utils.NoLogsFoundError:
			fmt.Printf("INFO: No checkpoints were found between blocks %d and %d.\n", startBlock, endBlock)
			return nil
		default:
			return err
		}
	}

	if len(newHeaderBlockEvents) > 0 {
		if len(newHeaderBlockEvents) == 1 {
			fmt.Printf("INFO: Processing checkpoint %d.\n", newHeaderBlockEvents[0].HeaderBlockId.Int64())
		} else {
			fmt.Printf("INFO: Processing checkpoints %d to %d.\n", newHeaderBlockEvents[0].HeaderBlockId.Int64(), newHeaderBlockEvents[len(newHeaderBlockEvents)-1].HeaderBlockId.Int64())
		}
	}

	for i, newEvent := range newHeaderBlockEvents {
		metrics.CurrentCheckpoint.Set(float64(newEvent.HeaderBlockId.Int64()))
		data, sigs := []byte{}, [][3]*big.Int{}

		// retry call in case of failure
		for i := 0; i < utils.RETRIES; i++ {
			data, sigs, err = utils.GetCheckpointSignatures(newEvent.TxHash)
			if err != nil {
				switch err.(type) {
				case *utils.DialError, *utils.TxHashError, *utils.PendingTxError:
					time.Sleep(time.Second * utils.RETRY_WAIT)
					continue
				default:
					fmt.Printf("ERR: Error while trying to get checkpoint signatures from transaction %v, error: %v\n", newEvent.TxHash, err)
					return err
				}
			} else {
				break
			}

		}
		if err != nil {
			fmt.Printf("ERR: Error while trying to get checkpoint signatures from transaction %v, error: %v\n", newEvent.TxHash, err)
			return err
		}

		signers, errCount := utils.SignersFromTXData(data, sigs)

		// get validators at this point
		err = database.UpdateValidatorsDB(newEvent.BlockNumber, newEvent.HeaderBlockId.Uint64())
		if err != nil {
			return err
		}

		blockTimestamp, err := utils.GetBlockTimestamp(newEvent.BlockNumber)
		if err != nil {
			return err
		}

		if errCount > 0 {
			fmt.Printf("WARN: There were %d errors while processing checkpoint number %d. The list of validators that signed it might be incomplete.", errCount, newEvent.HeaderBlockId.Uint64())
		}

		err = database.InsertCheckpoint(newEvent, blockTimestamp)
		if err != nil {
			return err
		}

		err = database.InsertValidatorsSignedCheckpoint(newEvent.HeaderBlockId.Uint64(), signers, false)
		if err != nil {
			return err
		}

		err = database.InsertValidatorsSignedCheckpoint(newEvent.HeaderBlockId.Uint64(), signers, true)
		if err != nil {
			return err
		}

		pb, err := calculateAndInsertPerformanceBenchmark700(newEvent.HeaderBlockId.Uint64(), newEvent.BlockNumber)
		if err != nil {
			switch err.(type) {
			case *utils.CheckpointNotFoundError:
				fmt.Printf("WARN: Could not calculate performance benchmark for checkpoint %d as we do not have enough data for the 700 checkpoints before it.\n", newEvent.HeaderBlockId.Uint64())
			default:
				return err
			}
		} else {
			metrics.CurrentPerformanceBenchmark.Set(pb)
		}

		err = metrics.UpdateCheckpointsSignedMetrics()
		if err != nil {
			return err
		}

		if pb != 0 {
			fmt.Printf("INFO: Processed checkpoint %d (ETH Block %d) - PB: %.5f%% [%.2f%%]\n", newEvent.HeaderBlockId.Int64(), newEvent.BlockNumber, pb*100, float64(i+1)/float64(len(newHeaderBlockEvents))*100)
		} else {
			fmt.Printf("INFO: Processed checkpoint %d (ETH Block %d) [%.2f%%]\n", newEvent.HeaderBlockId.Int64(), newEvent.BlockNumber, float64(i+1)/float64(len(newHeaderBlockEvents))*100)
		}
	}

	return nil
}

// calculateAndInsertPerformanceBenchmark700 calculates and inserts the
// performance benchmark in the database for the given checkpoint number. It
// returns the resulting performance benchmark.
func calculateAndInsertPerformanceBenchmark700(checkpointNumber uint64, blockNumber uint64) (float64, error) {
	// check if checkpointNumber - 699 exists first
	exists, err := database.CheckIfCheckpointExistsInTemp(checkpointNumber - 699)
	if err != nil {
		return 0, err
	}

	if !exists {
		return 0, &utils.CheckpointNotFoundError{GenericError: utils.GenericError{Message: "missing performance information for past 700 checkpoints, so unable to calculate performance benchmark"}}
	}

	// if exists, prune the temp table as we only use the last 700 checkpoints
	// we're keeping the performance of 1 additional checkpoint, just in case
	err = database.DeleteTempCheckpoints(checkpointNumber - 700)
	if err != nil {
		return 0, err
	}

	// get the performance of all the validators in the temp table
	checkpointCount, validatorsPerformance, err := database.GetSignedCheckpointsCountPerValidator(int(checkpointNumber)-699, int(checkpointNumber))
	if err != nil {
		return 0, err
	}

	performance := []float64{}
	for _, validatorPerformance := range validatorsPerformance {
		performance = append(performance, float64(validatorPerformance)/float64(checkpointCount))
	}

	// calculate the median performance
	medianPerformance := utils.Median(performance)

	// calculate performance benchmark
	performanceBenchmark := medianPerformance * float64(0.95)

	// get list of validators below threshold
	validatorsBelowThreshold := []int{}
	firstOutput := true
	for validatorId, validatorPerformance := range validatorsPerformance {
		performanceFloat := float64(validatorPerformance) / float64(checkpointCount)

		val, err := database.GetValidator(validatorId)
		if err != nil {
			return 0, err
		}

		if val.DeactivationEpoch == 0 && val.ActivationEpoch <= checkpointNumber-699 {
			if performanceFloat < performanceBenchmark {
				if firstOutput {
					firstOutput = false
					fmt.Printf("INFO: Validator(s) below PB threshold: ")
				}
				fmt.Printf("%d = %.5f\t", validatorId, performanceFloat*100)
				validatorsBelowThreshold = append(validatorsBelowThreshold, validatorId)
			}
		}
	}
	if len(validatorsBelowThreshold) > 0 {
		fmt.Println()
	}

	// insert the PB into the checkpoints table
	err = database.InsertPerformanceBenchmark(performanceBenchmark, int(checkpointNumber))
	if err != nil {
		return 0, err
	}

	return performanceBenchmark, nil
}

// mainLoop is the loop that calls other functions, constantly iterating over
// new blocks and looking for new checkpoint events.
func mainLoop(configPath string) {

	utils.UpdateConfigPath(configPath)

	go func() {
		// get the block number we are starting from
		startingBlock, err := getStartingBlock()
		if err != nil {
			os.Exit(1)
		}
		// update the current block number metric
		metrics.CurrentBlockNumber.Set(float64(startingBlock))

		// check if the validators table is empty
		emptyTable, err := database.ValidatorTableEmpty()
		if err != nil {
			os.Exit(1)
		}

		if emptyTable {
			// if the table is empty, insert validators in it
			err = database.UpdateValidatorsDB(startingBlock, 0)
			if err != nil {
				os.Exit(1)
			}
		}

		// get the current block number, which would be the last block in which
		// the tool will look for checkpoint events
		endBlock, err := utils.GetCurrentBlockNumber()
		if err != nil {
			os.Exit(1)
		}

		for {
			// call the function to process new events
			err := getNewEventsAndDecode(startingBlock, endBlock)
			if err != nil {
				os.Exit(1)
			}

			// increment the block number for the next iteration
			startingBlock = endBlock + 1
			metrics.CurrentBlockNumber.Set(float64(startingBlock))

			// sleep for a minute
			time.Sleep(1 * time.Minute)

			// get the latest block number again
			endBlock, err = utils.GetCurrentBlockNumber()
			if err != nil {
				os.Exit(1)
			}

		}
	}()

	// publish metrics on the configured port
	http.Handle("/metrics", promhttp.Handler())
	http.ListenAndServe(":"+utils.Config.PrometheusPort, nil)
}
