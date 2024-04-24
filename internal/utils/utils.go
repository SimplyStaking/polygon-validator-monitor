package utils

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

var configPath = "config/config.json"
var Config = GeneralSettings{}

const MAX_DEPOSITS = 10000
const ROOTCHAIN_ADDRESS = "0x86E4Dc95c7FBdBf52e33D563BbDB00823894C287"
const STAKEMANAGER_ADDRESS = "0x5e3Ef299fDDf15eAa0432E6e66473ace8c13D908"
const RETRIES = 3
const RETRY_WAIT = 3
const TIMEOUT = 300

// GeneralSettings is the representation of the options that can be
// contained in the config JSON file.
type GeneralSettings struct {
	ETHRpcUrl         string   `json:"ETHRpcUrl"`
	PrometheusPort    string   `json:"PrometheusPort"`
	DatabaseLocation  string   `json:"DatabaseLocation"`
	PublicKeys        []string `json:"PublicKeys"`
	ContinueFromBlock int      `json:"ContinueFromBlock"`
}

// updateConfigPath udpates the path to where the config file is located.
func UpdateConfigPath(path string) {
	configPath = path
	Config = openConfig(configPath)
}

// openConfig opens the config specified in the path path. It returns the
// parsed config as a GeneralSettings object.
func openConfig(path string) GeneralSettings {
	// open the passed file
	file, err := os.Open(path)
	if err != nil {
		log.Fatal("ERR Error while opening config file (", path, "): ", err)
	}
	defer file.Close()

	// read the file
	content, err := io.ReadAll(file)
	if err != nil {
		log.Fatal("ERR Error while reading content of config file (", path, "): ", err)
	}

	// unmarshal JSON data
	var config GeneralSettings
	err = json.Unmarshal(content, &config)
	if err != nil {
		log.Fatal("ERR Error while unmarshaling config file (", path, "): ", err)
	}

	return config
}

// convertSignature takes a signature in the form of a big.Int array, and
// converts them to bytes in the order that can be processed by other functions.
func convertSignature(sig [3]*big.Int) ([]byte, error) {
	// ensure 'v' is no longer than 1 byte
	if sig[2].BitLen() > 8 {
		fmt.Println("ERR: Signature 'v' length is longer than 1 byte.")
		return nil, errors.New("length of 'v' in signature is longer than 1 byte")
	}

	// check that v is either 27 or 28
	if !(sig[2].Cmp(big.NewInt(27)) == 0 || sig[2].Cmp(big.NewInt(28)) == 0) {
		fmt.Printf("ERR: Signature 'v' value is %d, expected 27 or 28.\n", sig[2])
		return nil, errors.New("value of 'v' in signature is neither 27 or 28")
	}

	// convert v from 27/28 to 0/1
	v := byte(sig[2].Uint64() - 27)

	// ensure the signature is valid
	if !crypto.ValidateSignatureValues(v, sig[0], sig[1], true) {
		fmt.Println("ERR: Signature is not valid.")
		return nil, errors.New("signature is invalid")
	}

	// convert r and s to bytes
	r, s := sig[0].Bytes(), sig[1].Bytes()

	// if r or s are not 32 bytes long, pad the left side with 0s
	if len(r) < 32 {
		r = common.LeftPadBytes(r, 32)
	}
	if len(s) < 32 {
		s = common.LeftPadBytes(s, 32)
	}

	// return the complete signature (65 byte array)
	return append(r, append(s, v)...), nil
}

// recoverAddress calls other functions to convert the message and signature
// to a public key as a string.
func recoverAddress(msg []byte, sig []byte) (string, error) {
	// get the public key using hash and signature
	sigPublicKeyECDSA, err := crypto.SigToPub(msg, sig)
	if err != nil {
		fmt.Printf("ERR: Could not convert hash and signature to public key, error: %v\n", err)
		return "", err
	}

	// return the public key in string form
	return crypto.PubkeyToAddress(*sigPublicKeyECDSA).String(), nil
}

// SignersFromTXData takes the data and signatures resulting from the
// submitCheckpoint method call and converts them into a list of public keys.
// It also returns the number of resulting errors.
func SignersFromTXData(data []byte, sigs [][3]*big.Int) ([]string, int) {
	// append byte of 01 to data due to https://etherscan.io/address/0x536c55cfe4892e581806e10b38dfe8083551bd03#code#L875
	data_new := append([]byte{1}, data...)

	// convert data to hash
	hash := crypto.Keccak256(data_new)

	// create new slice to hold results
	results := []string{}

	// create counter for errored recoveries
	errCount := 0

	// for each validator signature
	for i := 0; i < len(sigs); i++ {
		// convert the signature to bytes
		sig, err := convertSignature(sigs[i])
		if err != nil {
			// increment error count and move on to next signature
			errCount++
			continue
		}

		// get the public key using the hashed data and signature
		publicKey, err := recoverAddress(hash, sig)
		if err != nil {
			// increment error count
			errCount++
		} else {
			// append public key to results slice
			results = append(results, publicKey)
		}
	}

	return results, errCount
}

// GetBlockTimestamp calls other functions to get the timestamp of the passed
// block.
func GetBlockTimestamp(blockNumber uint64) (uint64, error) {
	var header types.Header
	var err error

	for i := 0; i < RETRIES; i++ {
		header, err = getHeaderByNumber(blockNumber)
		if err == nil {
			return header.Time, nil
		}

		// wait before next attempt
		time.Sleep(time.Second * RETRY_WAIT)
	}

	if err != nil {
		fmt.Printf("ERR: Error while trying to get timestamp of block number %d, error: %v\n", blockNumber, err)
		return 0, err
	}

	return 0, nil
}

// Contains is a helper function that returns true if the passed number is in
// the passed slice, and false otherwise.
func Contains(intSlice []int, number int) bool {
	for _, sliceNumber := range intSlice {
		if sliceNumber == number {
			return true
		}
	}
	return false
}

// ContainsString is a helper function that returns true if the passed string
// is in the passed slice, and false otherwise.
func ContainsString(stringSlice []string, search string) bool {
	for _, str := range stringSlice {
		if strings.EqualFold(str, search) {
			return true
		}
	}
	return false
}

// CheckIfDBExists is a simple function that checks if the database located as
// specified in the config exists or not.
func CheckIfDBExists() (bool, error) {
	_, err := os.Stat(Config.DatabaseLocation)
	if err != nil {
		return false, err
	}
	return true, nil
}

// CheckIfTrackAll checks if the public keys array in the config JSON file
// contains a '*', and is one element long. In such case, we are tracking the
// performance of all the validators in the set.
func CheckIfTrackAll() bool {
	if len(Config.PublicKeys) == 1 {
		return Config.PublicKeys[0] == "*"
	}
	return false
}

// from https://gosamples.dev/calculate-median/
// Median calculates the median from a slice of floats.
func Median(data []float64) float64 {
	dataCopy := make([]float64, len(data))
	copy(dataCopy, data)

	sort.Float64s(dataCopy)

	var median float64
	l := len(dataCopy)
	if l == 0 {
		return 0
	} else if l%2 == 0 {
		median = (dataCopy[l/2-1] + dataCopy[l/2]) / 2
	} else {
		median = dataCopy[l/2]
	}

	return median
}
