package utils

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/maticnetwork/heimdall/contracts/rootchain"
)

// NewHeaderBlockEvent represents the Rootchain ABI's NewHeaderBlock event, plus
// some extra information we may need.
type NewHeaderBlockEvent struct {
	TxHash          common.Hash
	ProposerAddress common.Address
	HeaderBlockId   big.Int
	Reward          big.Int
	BlockNumber     uint64
}

// GetCheckpointSignatures gets the data and signatures contained within a
// submitCheckpoint method call. It requires the transaction hash of where the
// method call occured.
func GetCheckpointSignatures(txHash common.Hash) ([]byte, [][3]*big.Int, error) {
	// create a timed context for the RPC call
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*TIMEOUT)
	defer cancel()

	// try to reach the ETH node
	ethRPCClient, err := rpc.Dial(Config.ETHRpcUrl)
	if err != nil {
		log.Println("ERR: Unable to dial ETH node (", Config.ETHRpcUrl, "), error:", err)
		return []byte{}, [][3]*big.Int{}, &DialError{GenericError{Message: "unable to dial ETH node"}}
	}
	ethClient := ethclient.NewClient(ethRPCClient)

	// get the transaction using the hash
	tx, isPending, err := ethClient.TransactionByHash(ctx, txHash)
	if err != nil {
		log.Println("ERR: Error while fetching transaction by hash from ETH rpc, error:", err)
		return []byte{}, [][3]*big.Int{}, &TxHashError{GenericError{Message: "unable to fetch transaction from ETH node"}}
	} else if isPending {
		log.Println("ERR: Error while fetching transaction by hash from ETH rpc: transaction is still pending.")
		return []byte{}, [][3]*big.Int{}, &PendingTxError{GenericError{Message: "transaction is still pending"}}
	}

	payload := tx.Data()
	var rootchainABI abi.ABI

	// get Rootchain ABI to decode tx data
	if ccAbi, err := GetABI(rootchain.RootchainABI); err != nil {
		log.Println("ERR: Error while fetching Rootchain ABI, error:", err)
		return []byte{}, [][3]*big.Int{}, errors.New("unable to fetch Rootchain ABI")
	} else {
		rootchainABI = ccAbi
	}

	// call function to decode data
	return unpackDataAndSigs(payload, rootchainABI)
}

// DecodeEvents queries the ETH RPC for activity between the range passed to it
// for the Rootchain address. It parses all relevant events and returns them as
// a slic of NewHeaderBlockEvent.
func DecodeEvents(startBlock uint64, endBlock uint64) ([]NewHeaderBlockEvent, error) {

	var ethRPCClient *rpc.Client
	var err error

	// try to reach the ETH node
	for i := 0; i < RETRIES; i++ {
		ethRPCClient, err = rpc.Dial(Config.ETHRpcUrl)
		if err == nil {
			break
		}
		time.Sleep(time.Second * RETRY_WAIT)
	}

	if err != nil {
		log.Printf("ERR: Unable to dial ETH node (%s), error: %v\n", Config.ETHRpcUrl, err)
		return []NewHeaderBlockEvent{}, &DialError{GenericError{Message: "unable to dial ETH node"}}
	}
	ethClient := ethclient.NewClient(ethRPCClient)

	rootchainABI := abi.ABI{}

	// get Rootchain ABI to decode tx data
	if ccAbi, err := GetABI(rootchain.RootchainABI); err != nil {
		log.Printf("ERR: Error while fetching Rootchain ABI, error: %v\n", err)
		return []NewHeaderBlockEvent{}, errors.New("unable to fetch Rootchain ABI")
	} else {
		rootchainABI = ccAbi
	}

	// instead of filtering by rootchain address, we can also filter by topics[0]
	query := ethereum.FilterQuery{
		FromBlock: big.NewInt(int64(startBlock)),
		ToBlock:   big.NewInt(int64(endBlock)),
		Addresses: []common.Address{
			common.HexToAddress(ROOTCHAIN_ADDRESS),
		},
	}

	var logs []types.Log

	// try to get the logs
	for i := 0; i < RETRIES; i++ {
		logs, err = ethClient.FilterLogs(context.Background(), query)
		if err == nil {
			break
		}
		time.Sleep(time.Second * RETRY_WAIT)
	}

	if err != nil {
		fmt.Printf("ERR: Error while trying to get logs from ETH node, error: %v\n", err)
		return []NewHeaderBlockEvent{}, &DialError{GenericError{Message: "unable to get logs from ETH node"}}
	}

	results := []NewHeaderBlockEvent{}

	if len(logs) == 0 {
		fmt.Println("WARN: No logs were found for given criteria.")
		return []NewHeaderBlockEvent{}, &NoLogsFoundError{GenericError{Message: "no logs found for given period"}}
	}

	for _, log := range logs {

		eventDataMap := make(map[string]interface{})

		event := rootchainABI.Events["NewHeaderBlock"]

		// try to unpack the event
		err := event.Inputs.UnpackIntoMap(eventDataMap, log.Data)
		if err != nil {
			fmt.Printf("ERR: Could not unpack event, error: %v\n", err)
			// in case we are unsuccessful, continue to next log
			continue
		}

		// update the information from the unpacked data
		proposer := common.BytesToAddress(log.Topics[1].Bytes())
		headerBlockId := new(big.Int).SetBytes(log.Topics[2][:])
		reward := new(big.Int).SetBytes(log.Topics[3][:])

		// construct the NewHeaderBlockEvent struct
		headerBlockEvent := NewHeaderBlockEvent{
			TxHash:          log.TxHash,
			ProposerAddress: proposer,
			HeaderBlockId:   *headerBlockId.Div(headerBlockId, big.NewInt(MAX_DEPOSITS)),
			Reward:          *reward,
			BlockNumber:     log.BlockNumber,
		}

		// append this result to list
		results = append(results, headerBlockEvent)
	}

	// return complete list of events
	return results, nil
}

// GetCurrentBlockNumber gets the latest block number from the ETH RPC.
func GetCurrentBlockNumber() (uint64, error) {
	// create a timed context for the RPC call
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*TIMEOUT)
	defer cancel()

	var ethRPCClient *rpc.Client
	var err error

	// try to reach the ETH node
	for i := 0; i < RETRIES; i++ {
		ethRPCClient, err = rpc.Dial(Config.ETHRpcUrl)
		if err == nil {
			break
		}

		time.Sleep(time.Second * RETRY_WAIT)
	}

	if err != nil {
		log.Printf("ERR: Unable to dial ETH node (%s), error: %v\n", Config.ETHRpcUrl, err)
		return 0, &DialError{GenericError{Message: "unable to dial ETH node"}}
	}

	ethClient := ethclient.NewClient(ethRPCClient)

	var blockNumber uint64

	// try to get current block number
	for i := 0; i < RETRIES; i++ {
		blockNumber, err = ethClient.BlockNumber(ctx)
		if err == nil {
			break
		}

		time.Sleep(time.Second * RETRY_WAIT)
	}

	if err != nil {
		fmt.Printf("ERR: Error while retrieving most recent block number, error: %v\n", err)
		return 0, &DialError{GenericError{Message: "error retrieving most recent block number, error: " + err.Error()}}
	}

	return blockNumber, nil
}

// getHeaderByNumber queries the passed block number from the ETH RPC, returning
// the header of the respective block.
func getHeaderByNumber(blockNumber uint64) (types.Header, error) {
	// create a timed context for the RPC call
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*TIMEOUT)
	defer cancel()

	// try to reach the ETH node
	ethRPCClient, err := rpc.Dial(Config.ETHRpcUrl)
	if err != nil {
		log.Printf("ERR: Unable to dial ETH node (%s), error: %v\n", Config.ETHRpcUrl, err)
		return types.Header{}, &DialError{GenericError{Message: "unable to dial ETH node"}}
	}
	ethClient := ethclient.NewClient(ethRPCClient)

	// get the header from the RPC
	header, err := ethClient.HeaderByNumber(ctx, big.NewInt(int64(blockNumber)))
	if err != nil {
		log.Printf("ERR: Unable to retrieve header from ETH node, error: %v\n", err)
		return types.Header{}, &DialError{GenericError{Message: "error retrieving header, error: " + err.Error()}}
	}
	return *header, nil
}
