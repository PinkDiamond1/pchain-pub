// Copyright 2014 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package vm

import (
	"fmt"
	"math/big"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	//"github.com/ethereum/go-ethereum/logger/glog"
)

type (
	CanTransferFunc func(StateDB, common.Address, *big.Int) bool
	TransferFunc    func(StateDB, common.Address, common.Address, *big.Int)
	// GetHashFunc returns the nth block hash in the blockchain
	// and is used by the BLOCKHASH EVM op code.
	GetHashFunc func(uint64) common.Hash
)

// Context provides the EVM with auxiliary information. Once provided it shouldn't be modified.
type Context struct {
	// CanTransfer returns whether the account contains
	// sufficient ether to transfer the value
	CanTransfer CanTransferFunc
	// Transfer transfers ether from one account to the other
	Transfer TransferFunc
	// GetHash returns the hash corresponding to n
	GetHash GetHashFunc

	// Message information
	Origin   common.Address // Provides information for ORIGIN
	GasPrice *big.Int       // Provides information for GASPRICE

	// Block information
	Coinbase    common.Address // Provides information for COINBASE
	GasLimit    *big.Int       // Provides information for GASLIMIT
	BlockNumber *big.Int       // Provides information for NUMBER
	Time        *big.Int       // Provides information for TIME
	Difficulty  *big.Int       // Provides information for DIFFICULTY
}

// EVM provides information about external sources for the EVM
//
// The EVM should never be reused and is not thread safe.
type EVM struct {
	// Context provides auxiliary blockchain related information
	Context
	// StateDB gives access to the underlying state
	StateDB StateDB
	// Depth is the current call stack
	depth int

	// chainConfig contains information about the current chain
	chainConfig *params.ChainConfig
	// virtual machine configuration options used to initialise the
	// evm.
	vmConfig Config
	// global (to this context) ethereum virtual machine
	// used throughout the execution of the tx.
	interpreter *Interpreter
	// abort is used to abort the EVM calling operations
	// NOTE: must be set atomically
	abort int32
}

// NewEVM retutrns a new EVM evmironment.
func NewEVM(ctx Context, statedb StateDB, chainConfig *params.ChainConfig, vmConfig Config) *EVM {
	evm := &EVM{
		Context:     ctx,
		StateDB:     statedb,
		vmConfig:    vmConfig,
		chainConfig: chainConfig,
	}

	evm.interpreter = NewInterpreter(evm, vmConfig)
	return evm
}

// Cancel cancels any running EVM operation. This may be called concurrently and it's safe to be
// called multiple times.
func (evm *EVM) Cancel() {
	atomic.StoreInt32(&evm.abort, 1)
}

// Call executes the contract associated with the addr with the given input as parameters. It also handles any
// necessary value transfer required and takes the necessary steps to create accounts and reverses the state in
// case of an execution error or failed value transfer.
func (evm *EVM) Call(caller ContractRef, addr common.Address, input []byte, gas, value *big.Int) (ret []byte, err error) {
	if evm.vmConfig.NoRecursion && evm.depth > 0 {
		caller.ReturnGas(gas)

		return nil, nil
	}

	// Depth check execution. Fail if we're trying to execute above the
	// limit.
	if evm.depth > int(params.CallCreateDepth.Int64()) {
		caller.ReturnGas(gas)

		return nil, ErrDepth
	}
	if !evm.Context.CanTransfer(evm.StateDB, caller.Address(), value) {
		caller.ReturnGas(gas)

		return nil, ErrInsufficientBalance
	}

	var (
		to       Account
		snapshot = evm.StateDB.Snapshot()
	)
	if !evm.StateDB.Exist(addr) {
		if PrecompiledContracts[addr] == nil && evm.ChainConfig().IsEIP158(evm.BlockNumber) && value.BitLen() == 0 {
			caller.ReturnGas(gas)
			return nil, nil
		}

		to = evm.StateDB.CreateAccount(addr)
	} else {
		to = evm.StateDB.GetAccount(addr)
	}
	evm.Transfer(evm.StateDB, caller.Address(), to.Address(), value)

	// initialise a new contract and set the code that is to be used by the
	// E The contract is a scoped evmironment for this execution context
	// only.
	contract := NewContract(caller, to, value, gas)
	contract.SetCallCode(&addr, evm.StateDB.GetCodeHash(addr), evm.StateDB.GetCode(addr))
	defer contract.Finalise()

	ret, err = evm.interpreter.Run(contract, input)
	// When an error was returned by the EVM or when setting the creation code
	// above we revert to the snapshot and consume any gas remaining. Additionally
	// when we're in homestead this also counts for code storage gas errors.
	if err != nil {
		contract.UseGas(contract.Gas)

		evm.StateDB.RevertToSnapshot(snapshot)
	}
	return ret, err
}

// CallCode executes the contract associated with the addr with the given input as parameters. It also handles any
// necessary value transfer required and takes the necessary steps to create accounts and reverses the state in
// case of an execution error or failed value transfer.
//
// CallCode differs from Call in the sense that it executes the given address' code with the caller as context.
func (evm *EVM) CallCode(caller ContractRef, addr common.Address, input []byte, gas, value *big.Int) (ret []byte, err error) {
	if evm.vmConfig.NoRecursion && evm.depth > 0 {
		caller.ReturnGas(gas)

		return nil, nil
	}

	// Depth check execution. Fail if we're trying to execute above the
	// limit.
	if evm.depth > int(params.CallCreateDepth.Int64()) {
		caller.ReturnGas(gas)

		return nil, ErrDepth
	}
	if !evm.CanTransfer(evm.StateDB, caller.Address(), value) {
		caller.ReturnGas(gas)

		return nil, fmt.Errorf("insufficient funds to transfer value. Req %v, has %v", value, evm.StateDB.GetBalance(caller.Address()))
	}

	var (
		snapshot = evm.StateDB.Snapshot()
		to       = evm.StateDB.GetAccount(caller.Address())
	)
	// initialise a new contract and set the code that is to be used by the
	// E The contract is a scoped evmironment for this execution context
	// only.
	contract := NewContract(caller, to, value, gas)
	contract.SetCallCode(&addr, evm.StateDB.GetCodeHash(addr), evm.StateDB.GetCode(addr))
	defer contract.Finalise()

	ret, err = evm.interpreter.Run(contract, input)
	if err != nil {
		contract.UseGas(contract.Gas)

		evm.StateDB.RevertToSnapshot(snapshot)
	}

	return ret, err
}

// DelegateCall executes the contract associated with the addr with the given input as parameters.
// It reverses the state in case of an execution error.
//
// DelegateCall differs from CallCode in the sense that it executes the given address' code with the caller as context
// and the caller is set to the caller of the caller.
func (evm *EVM) DelegateCall(caller ContractRef, addr common.Address, input []byte, gas *big.Int) (ret []byte, err error) {
	if evm.vmConfig.NoRecursion && evm.depth > 0 {
		caller.ReturnGas(gas)

		return nil, nil
	}

	// Depth check execution. Fail if we're trying to execute above the
	// limit.
	if evm.depth > int(params.CallCreateDepth.Int64()) {
		caller.ReturnGas(gas)
		return nil, ErrDepth
	}

	var (
		snapshot = evm.StateDB.Snapshot()
		to       = evm.StateDB.GetAccount(caller.Address())
	)

	// Iinitialise a new contract and make initialise the delegate values
	contract := NewContract(caller, to, caller.Value(), gas).AsDelegate()
	contract.SetCallCode(&addr, evm.StateDB.GetCodeHash(addr), evm.StateDB.GetCode(addr))
	defer contract.Finalise()

	ret, err = evm.interpreter.Run(contract, input)
	if err != nil {
		contract.UseGas(contract.Gas)

		evm.StateDB.RevertToSnapshot(snapshot)
	}

	return ret, err
}

// Create creates a new contract using code as deployment code.
func (evm *EVM) Create(caller ContractRef, code []byte, gas, value *big.Int) (ret []byte, contractAddr common.Address, err error) {

	//glog.Infof("(evm *EVM) Create() 0, gas is %v\n", gas)
	if evm.vmConfig.NoRecursion && evm.depth > 0 {
		caller.ReturnGas(gas)

		return nil, common.Address{}, nil
	}
	//glog.Infof("(evm *EVM) Create() 1, gas is %v\n", gas)
	// Depth check execution. Fail if we're trying to execute above the
	// limit.
	if evm.depth > int(params.CallCreateDepth.Int64()) {
		caller.ReturnGas(gas)

		return nil, common.Address{}, ErrDepth
	}
	//glog.Infof("(evm *EVM) Create() 2, gas is %v\n", gas)
	if !evm.CanTransfer(evm.StateDB, caller.Address(), value) {
		caller.ReturnGas(gas)

		return nil, common.Address{}, ErrInsufficientBalance
	}
	//glog.Infof("(evm *EVM) Create() 3, gas is %v\n", gas)
	// Create a new account on the state
	nonce := evm.StateDB.GetNonce(caller.Address())
	evm.StateDB.SetNonce(caller.Address(), nonce+1)

	snapshot := evm.StateDB.Snapshot()
	contractAddr = crypto.CreateAddress(caller.Address(), nonce)
	to := evm.StateDB.CreateAccount(contractAddr)
	if evm.ChainConfig().IsEIP158(evm.BlockNumber) {
		evm.StateDB.SetNonce(contractAddr, 1)
	}
	evm.Transfer(evm.StateDB, caller.Address(), to.Address(), value)

	// initialise a new contract and set the code that is to be used by the
	// E The contract is a scoped evmironment for this execution context
	// only.
	//glog.Infof("(evm *EVM) Create() 4, gas is %v\n", gas)
	contract := NewContract(caller, to, value, gas)
	//glog.Infof("(evm *EVM) Create() 5, gas is %v\n", gas)
	contract.SetCallCode(&contractAddr, crypto.Keccak256Hash(code), code)
	defer contract.Finalise()

	ret, err = evm.interpreter.Run(contract, nil)
	//glog.Infof("(evm *EVM) Create() 6, len(ret) is %v, err is %v, ret is %v\n", len(ret), err, ret)

	// check whether the max code size has been exceeded
	maxCodeSizeExceeded := len(ret) > params.MaxCodeSize
	// if the contract creation ran successfully and no errors were returned
	// calculate the gas required to store the code. If the code could not
	// be stored due to not enough gas set an error and let it be handled
	// by the error checking condition below.
	if err == nil && !maxCodeSizeExceeded {
		dataGas := big.NewInt(int64(len(ret)))
		//glog.Infof("(evm *EVM) Create() 7, dataGas is %v, params.CreateDataGas is %v\n", dataGas, params.CreateDataGas)
		dataGas.Mul(dataGas, params.CreateDataGas)
		//glog.Infof("(evm *EVM) Create() 8, dataGas is %v\n", dataGas)

		if contract.UseGas(dataGas) {
			//glog.Infof("(evm *EVM) Create() 9%v\n")
			evm.StateDB.SetCode(contractAddr, ret)
		} else {
			//glog.Infof("(evm *EVM) Create() 10%v\n")
			err = ErrCodeStoreOutOfGas
		}
		//glog.Infof("(evm *EVM) Create() 11, contract.UsedGas is %v\n", contract.UsedGas)
	}

	// When an error was returned by the EVM or when setting the creation code
	// above we revert to the snapshot and consume any gas remaining. Additionally
	// when we're in homestead this also counts for code storage gas errors.
	if maxCodeSizeExceeded ||
		(err != nil && (evm.ChainConfig().IsHomestead(evm.BlockNumber) || err != ErrCodeStoreOutOfGas)) {
		contract.UseGas(contract.Gas)
		evm.StateDB.RevertToSnapshot(snapshot)
		//glog.Infof("(evm *EVM) Create() 12, contract.UsedGas is %v\n", contract.UsedGas)
		// Nothing should be returned when an error is thrown.

		if(maxCodeSizeExceeded && err == nil) {
			err = ErrCodeStoreOutOfGas
		}

		return nil, contractAddr, err
	}
	// If the vm returned with an error the return value should be set to nil.
	// This isn't consensus critical but merely to for behaviour reasons such as
	// tests, RPC calls, etc.
	if err != nil {
		ret = nil
	}
	//glog.Infof("(evm *EVM) Create() 13, contract.UsedGas is %v\n", contract.UsedGas)
	return ret, contractAddr, err
}

// ChainConfig returns the evmironment's chain configuration
func (evm *EVM) ChainConfig() *params.ChainConfig { return evm.chainConfig }

// Interpreter returns the EVM interpreter
func (evm *EVM) Interpreter() *Interpreter { return evm.interpreter }
