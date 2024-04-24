package metrics

import (
	"database/sql"
	"fmt"
	"math"

	database "monitor/internal/db"
	"monitor/internal/utils"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// these are all the custom metrics supported by the tool
var (
	checkpointsSigned = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "checkpoints_signed",
		Help: "The number of checkpoints signed by a validator for the given range",
	}, []string{"validator", "range"})

	checkpointsTotal = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "checkpoints_total",
		Help: "The total number of checkpoints for the given range",
	}, []string{"range"})

	validatorPerformance = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "validator_performance",
		Help: "The percentage of checkpoints signed for the given range",
	}, []string{"validator", "range"})

	CurrentCheckpoint = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "current_checkpoint",
		Help: "The latest checkpoint processed by the monitor",
	})

	CurrentBlockNumber = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "current_block_number",
		Help: "The latest ETH block number processed by the monitor",
	})

	CurrentPerformanceBenchmark = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "current_performance_benchmark",
		Help: "The performance benchmark as of the last checkpoint processed by the monitor.",
	})

	checkpointsToPB = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "checkpoints_to_performance_benchmark",
		Help: "How many checkpoints the associated validator must miss to fall below the performance benchmark.",
	}, []string{"validator"})

	checkpointsToReduction = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "checkpoints_to_reduction",
		Help: "How many checkpoints the associated validator has to go through until it gets the first improvement in PB.",
	}, []string{"validator"})
)

// calculateCheckpointsToPB calculates and returns how many more checkpoints the
// validator has to miss to fall below the *current* performance benchmark. The
// performance benchmark and the number of checkpoints signed by the validator
// (assuming of the last 700) are to be passed to the function.
func calculateCheckpointsToPB(pb float64, checkpointsSigned int) int {
	// convert the pb to percentage signed, assuming a 700 checkpoint range
	pbSigned := int(math.Floor(pb * float64(700))) // this is the maximum we can sign and still be below the pb

	// if validator has already reached or is below pb
	if checkpointsSigned <= pbSigned {
		return 0
	}

	// otherwise return difference
	return checkpointsSigned - pbSigned
}

// checkpointsToMissReduce calculates how many checkpoints the passed validator
// has to go through before seeing an improvement in their performance. It
// essentially gets the first checkpoint missed of the past 700 and calculates
// how many checkpoints remain from that checkpoint + 700.
func checkpointsToMissReduce(signerKey string, checkpointNumber int) (int, error) {
	// get the first checkpoint the validator missed within the 700 checkpoint
	// range
	firstMiss, err := database.GetFirstMissedCheckpointRange(signerKey, checkpointNumber-699, checkpointNumber)
	if err != nil {
		return 0, err
	}

	// return how many checkpoints remain until we arrive at a point where the
	// miss is no longer considered in the performance benchmark (i.e. the
	// checkpoint in which the validator missed, will no longer be part of the
	// past 700, thus not used in the performance benchmark)
	return firstMiss + 700 - checkpointNumber, nil
}

// UpdateCheckpointsSignedMetrics updates metrics related to checkpoints and the
// performance of validators. It does not take any passed values, instead
// getting all values from the database.
func UpdateCheckpointsSignedMetrics() error {
	// update last checkpoint metric
	lastCheckpoint, err := database.GetLastCheckpointNumber()
	if err == nil {
		CurrentCheckpoint.Set(float64(lastCheckpoint))

		// get the number of checkpoints we have of the last 700, and the
		// number of these that were signed by the tracked validators
		checkpointCount700, checkpointPerformance700, err := database.GetCheckpointCount(lastCheckpoint-699, lastCheckpoint)
		if err != nil {
			return err
		}

		// update the total number of checkpoints (of the last 700) metric
		checkpointsTotal.WithLabelValues("700").Set(float64(checkpointCount700))

		// for every tracked validator, update the metrics relating to the
		// number of checkpoints they signed, and their performance (of the last
		// 700)
		for publicKey, value := range checkpointPerformance700 {
			checkpointsSigned.WithLabelValues(publicKey, "700").Set(float64(value))
			validatorPerformance.WithLabelValues(publicKey, "700").Set(float64(value) / float64(checkpointCount700))
		}

		// get the total number of checkpoints we have, and the number of these
		// that were signed by the tracked validators
		checkpointCountTotal, checkpointPerformanceTotal, err := database.GetCheckpointCount(0, lastCheckpoint)
		if err != nil {
			return err
		}

		// update the total number of checkpoints metric
		checkpointsTotal.WithLabelValues("total").Set(float64(checkpointCountTotal))

		// for every tracked validator, update the metrics relating to the
		// number of checkpoints they signed, and their performance
		for publicKey, value := range checkpointPerformanceTotal {
			checkpointsSigned.WithLabelValues(publicKey, "total").Set(float64(value))
			validatorPerformance.WithLabelValues(publicKey, "total").Set(float64(value) / float64(checkpointCountTotal))
		}

		// update performance benchmark metrics
		pb, err := database.GetPBAtCheckpoint(lastCheckpoint)
		if err == nil {
			// call fn to calculate checkpoints to pb for tracked validators
			for publicKey, value := range checkpointPerformance700 {
				// update the respective metric, for the respective validator
				checkpointsToPB.WithLabelValues(publicKey).Set(float64(calculateCheckpointsToPB(pb, value)))
				if value == 700 {
					// if we signed all the past 700 checkpoints, then this
					// value should be 0
					checkpointsToReduction.WithLabelValues(publicKey).Set(float64(0))
				} else {
					// otherwise, calculate it
					checkpointsToReduce, err := checkpointsToMissReduce(publicKey, lastCheckpoint)
					if err != nil {
						return err
					}
					// and update the metric
					checkpointsToReduction.WithLabelValues(publicKey).Set(float64(checkpointsToReduce))
				}
			}
		} else {
			if err == sql.ErrNoRows {
				// we do not have the data to calculate the metric
				return nil
			} else {
				switch err.(type) {
				case *utils.CheckpointNotFoundError:
					// it is somewhat impossible to get to this point, but in
					// case we do, do not panic
					return nil
				default:
					return err
				}
			}
		}

	} else {
		// if the database is empty (i.e. has no processed any checkpoint yet)
		// then we cannot set any metrics
		if err == sql.ErrNoRows {
			fmt.Println("WARN: Database is empty, no metrics to update.")
			return nil
		}
	}

	// in case of any other error, return it
	return err
}
