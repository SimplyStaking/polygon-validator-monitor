package utils

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

// unpackDataAndSigs returns the signatures and data in the payload that is
// contained within the submitCheckpoint function of the Rootchain ABI.
func unpackDataAndSigs(payload []byte, abi abi.ABI) (data []byte, sigs [][3]*big.Int, err error) {
	// get Method using abi
	method := abi.Methods["submitCheckpoint"]

	// it is done like this in https://github.com/maticnetwork/heimdall/blob/249aa798c2f23c533d2421f2101127c11684c76e/helper/util.go#L749
	// will not decode any other way. not entirely sure why its [4:]
	decodedPayload := payload[4:]

	// create map to decode payload
	inputDataMap := make(map[string]interface{})

	// unpack the data into the map
	err = method.Inputs.UnpackIntoMap(inputDataMap, decodedPayload)
	if err != nil {
		return
	}

	// update the data and sigs values
	data = inputDataMap["data"].([]byte)
	sigs = inputDataMap["sigs"].([][3]*big.Int)

	return
}

// GetABI returns the contract's ABI struct from on its JSON representation
func GetABI(data string) (abi.ABI, error) {
	return abi.JSON(strings.NewReader(data))
}

// GetValidatorInfoStakeManagerConcurrent is the concurrent function of the
// function with the same name. It gets all information about the validator
// with the given validator ID at the provided block number. It saves the
// results in a channel of type ValidatorError.
func GetValidatorInfoStakeManagerConcurrent(validatorId int, blockNumber int, stakeManagerABI abi.ABI, ethClient ethclient.Client, contractAddress common.Address, validators chan<- ValidatorError, wg *sync.WaitGroup) {

	defer wg.Done()

	// pack the data for the query we are making
	callData, err := stakeManagerABI.Pack("validators", big.NewInt(int64(validatorId)))
	if err != nil {
		fmt.Printf("ERR: Failed to pack data for StakeManager contract call (method: validators), error: %v\n", err)
		validators <- ValidatorError{Validator: Validator{}, Error: err}
		return
	}

	var result []byte

	// try to query the smart contract
	for i := 0; i < RETRIES; i++ {
		if blockNumber > 0 {
			// if a block number was passed, query at that point
			result, err = ethClient.CallContract(context.Background(), ethereum.CallMsg{
				To:   &contractAddress,
				Data: callData,
			}, big.NewInt(int64(blockNumber)))
		} else {
			// if no block number was passed, use 'nil' and get the data as of
			// the last block
			result, err = ethClient.CallContract(context.Background(), ethereum.CallMsg{
				To:   &contractAddress,
				Data: callData,
			}, nil)
		}
		if err == nil {
			break
		}

		// sleep between retries
		time.Sleep(time.Second * RETRY_WAIT)
	}

	if err != nil {
		fmt.Printf("ERR: Failed to query StakeManager contract (method: validators), error: %v\n", err)
		validators <- ValidatorError{Validator: Validator{}, Error: err}
		return
	}

	// prepare a struct for the smart contract response
	var response struct {
		Amount                *big.Int
		Reward                *big.Int
		ActivationEpoch       *big.Int
		DeactivationEpoch     *big.Int
		JailTime              *big.Int
		Signer                common.Address
		ContractAddress       common.Address
		Status                uint8
		CommissionRate        *big.Int
		LastCommissionUpdate  *big.Int
		DelegatorsReward      *big.Int
		DelegatedAmount       *big.Int
		InitialRewardPerStake *big.Int
	}

	// unpack the response into the struct
	err = stakeManagerABI.UnpackIntoInterface(&response, "validators", result)
	if err != nil {
		fmt.Printf("ERR: Failed to unpack the StakeManager contract call request (method: validators), error: %v\n", err)
		validators <- ValidatorError{Validator: Validator{}, Error: err}
		return
	}

	// if the passed validator ID is greater than the highest validator id (i.e.
	// does not exist), we will get 0 for the activation epoch
	if response.ActivationEpoch.Uint64() == 0 {
		// in this case, do not error - return -1 to signify that we encountered
		// this case
		validators <- ValidatorError{Validator: Validator{ValidatorId: -1}, Error: nil}
		return
	}

	// get the owner of the validator, using the validator id, and querying
	// ownerOf on the smart contract
	callData, err = stakeManagerABI.Pack("ownerOf", big.NewInt(int64(validatorId)))
	if err != nil {
		fmt.Printf("ERR: Failed to pack data for StakeManager contract call (method: ownerOf), error: %v\n", err)
		validators <- ValidatorError{Validator: Validator{}, Error: err}
		return
	}

	// try querying the contract
	for i := 0; i < RETRIES; i++ {
		if blockNumber > 0 {
			result, err = ethClient.CallContract(context.Background(), ethereum.CallMsg{
				To:   &contractAddress,
				Data: callData,
			}, big.NewInt(int64(blockNumber)))
		} else {
			result, err = ethClient.CallContract(context.Background(), ethereum.CallMsg{
				To:   &contractAddress,
				Data: callData,
			}, nil)
		}
		if err == nil {
			break
		} else if err.Error() == "execution reverted" {
			// in this case do not retry, no point
			// this usually entails that the validator is no longer active
			break
		}
		time.Sleep(time.Second * RETRY_WAIT)
	}

	if err != nil {
		if err.Error() == "execution reverted" {
			fmt.Printf("WARN: Validator with ID %d has no owner.\n", validatorId)
			// it could be that the validator has no owner, such as validator
			// with id = 11
			// in this case, ignore the error

			validators <- ValidatorError{Validator: Validator{
				ValidatorId:       validatorId,
				ActivationEpoch:   response.ActivationEpoch.Uint64(),
				DeactivationEpoch: response.DeactivationEpoch.Uint64(),
				SignerAddress:     response.Signer,
			}, Error: nil}
			return

		}
	}

	if err != nil {
		fmt.Printf("ERR: Failed to query StakeManager contract (method: ownerOf), error: %v\n", err)
		validators <- ValidatorError{Validator: Validator{}, Error: err}
		return
	}

	// if we had no errors, proceed with unpacking the response
	var ownerAddress common.Address
	err = stakeManagerABI.UnpackIntoInterface(&ownerAddress, "ownerOf", result)
	if err != nil {
		fmt.Printf("ERR: Failed to unpack the StakeManager contract call request (method: ownerOf), error: %v\n", err)
		validators <- ValidatorError{Validator: Validator{}, Error: err}
		return
	}

	// return the validator
	validators <- ValidatorError{Validator: Validator{
		ValidatorId:       validatorId,
		ActivationEpoch:   response.ActivationEpoch.Uint64(),
		DeactivationEpoch: response.DeactivationEpoch.Uint64(),
		OwnerAddress:      ownerAddress,
		SignerAddress:     response.Signer,
	}, Error: nil}

}

// GetValidatorInfoStakeManager gets all information about the validator with
// the given validator ID at the provided block number. It returns the compiled
// validator, or an error in the case that something goes wrong.
func GetValidatorInfoStakeManager(validatorId int, blockNumber int, stakeManagerABI abi.ABI, ethClient ethclient.Client, contractAddress common.Address) (Validator, error) {

	// pack the data for the query we are making
	callData, err := stakeManagerABI.Pack("validators", big.NewInt(int64(validatorId)))
	if err != nil {
		fmt.Printf("ERR: Failed to pack data for StakeManager contract call (method: validators), error: %v\n", err)
		return Validator{}, err
	}

	var result []byte

	// try to query the smart contract
	for i := 0; i < RETRIES; i++ {
		if blockNumber > 0 {
			// if a block number was provided, query at that point
			result, err = ethClient.CallContract(context.Background(), ethereum.CallMsg{
				To:   &contractAddress,
				Data: callData,
			}, big.NewInt(int64(blockNumber)))
		} else {
			// if no block number was provided, get the data as of the last
			// block
			result, err = ethClient.CallContract(context.Background(), ethereum.CallMsg{
				To:   &contractAddress,
				Data: callData,
			}, nil)
		}
		if err == nil {
			break
		}
		time.Sleep(time.Second * RETRY_WAIT)
	}

	if err != nil {
		fmt.Printf("ERR: Failed to query StakeManager contract (method: validators), error: %v\n", err)
		return Validator{}, err
	}

	// prepare the struct for the smart contract response
	var response struct {
		Amount                *big.Int
		Reward                *big.Int
		ActivationEpoch       *big.Int
		DeactivationEpoch     *big.Int
		JailTime              *big.Int
		Signer                common.Address
		ContractAddress       common.Address
		Status                uint8
		CommissionRate        *big.Int
		LastCommissionUpdate  *big.Int
		DelegatorsReward      *big.Int
		DelegatedAmount       *big.Int
		InitialRewardPerStake *big.Int
	}

	// unpack the response into the struct
	err = stakeManagerABI.UnpackIntoInterface(&response, "validators", result)
	if err != nil {
		fmt.Printf("ERR: Failed to unpack the StakeManager contract call request (method: validators), error: %v\n", err)
		return Validator{}, err
	}

	// if the validator ID is greater than the highest validator id (i.e. does
	// not exist), we will get 0 for the activation epoch
	if response.ActivationEpoch.Uint64() == 0 {
		// in such case, return a Validator with ValidatorId set to -1 to
		// signify this
		return Validator{ValidatorId: -1}, nil
	}

	// get the owner of the validator, using the validator id
	callData, err = stakeManagerABI.Pack("ownerOf", big.NewInt(int64(validatorId)))
	if err != nil {
		fmt.Printf("ERR: Failed to pack data for StakeManager contract call (method: ownerOf), error: %v\n", err)
		return Validator{}, err
	}

	// try querying the contract
	for i := 0; i < RETRIES; i++ {
		if blockNumber > 0 {
			result, err = ethClient.CallContract(context.Background(), ethereum.CallMsg{
				To:   &contractAddress,
				Data: callData,
			}, big.NewInt(int64(blockNumber)))
		} else {
			result, err = ethClient.CallContract(context.Background(), ethereum.CallMsg{
				To:   &contractAddress,
				Data: callData,
			}, nil)
		}
		if err == nil {
			break
		} else if err.Error() == "execution reverted" {
			// in this case do not retry, no point - usually entails the
			// validator has been deactivated and is no longer active
			break
		}
		time.Sleep(time.Second * RETRY_WAIT)
	}

	if err != nil {
		if err.Error() == "execution reverted" {
			fmt.Printf("WARN: Validator with ID %d has no owner.\n", validatorId)
			// it could be that the validator has no owner, such as validator
			// with id = 11
			// in this case, ignore the error

			return Validator{
				ValidatorId:       validatorId,
				ActivationEpoch:   response.ActivationEpoch.Uint64(),
				DeactivationEpoch: response.DeactivationEpoch.Uint64(),
				SignerAddress:     response.Signer,
			}, nil

		}
	}

	if err != nil {
		fmt.Printf("ERR: Failed to query StakeManager contract (method: ownerOf), error: %v\n", err)
		return Validator{}, err
	}

	// if we had no errors, continue with unpacking the response
	var ownerAddress common.Address
	err = stakeManagerABI.UnpackIntoInterface(&ownerAddress, "ownerOf", result)
	if err != nil {
		fmt.Printf("ERR: Failed to unpack the StakeManager contract call request (method: ownerOf), error: %v\n", err)
		return Validator{}, err
	}

	// return the validator with all the info
	return Validator{
		ValidatorId:       validatorId,
		ActivationEpoch:   response.ActivationEpoch.Uint64(),
		DeactivationEpoch: response.DeactivationEpoch.Uint64(),
		OwnerAddress:      ownerAddress,
		SignerAddress:     response.Signer,
	}, nil

}
