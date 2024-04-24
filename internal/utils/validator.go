package utils

import (
	"github.com/ethereum/go-ethereum/common"
)

// Validator is used to represent the fields related with each validator, that
// are needed and used by this tool.
type Validator struct {
	ValidatorId       int
	ActivationEpoch   uint64
	DeactivationEpoch uint64
	OwnerAddress      common.Address
	SignerAddress     common.Address
}

// ValidatorError contains a Validator, and an Error. It is used by concurrent
// functions to only have one return value.
type ValidatorError struct {
	Validator Validator
	Error     error
}

// CompareValidators compares all the fields in two Validator structs, and
// returns true if they are identical, and false otherwise.
func CompareValidators(validator1 Validator, validator2 Validator) bool {
	if validator1.ValidatorId == validator2.ValidatorId {
		if validator1.ActivationEpoch == validator2.ActivationEpoch {
			if validator1.DeactivationEpoch == validator2.DeactivationEpoch {
				if validator1.OwnerAddress == validator2.OwnerAddress {
					if validator1.SignerAddress == validator2.SignerAddress {
						return true
					}
				}
			}
		}
	}
	return false
}
