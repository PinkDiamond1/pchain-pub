package ethereum

import (
	"github.com/ethereum/go-ethereum/cmd/utils"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/eth"
	"github.com/ethereum/go-ethereum/node"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"gopkg.in/urfave/cli.v1"

	"github.com/tendermint/go-rpc/client"
)

var clientIdentifier = "geth" // Client identifier to advertise over the network

// MakeSystemNode sets up a local node and configures the services to launch
func MakeSystemNode(name, version, rcpuri string, ctx *cli.Context) *node.Node {
	params.TargetGasLimit = common.String2Big(ctx.GlobalString(utils.TargetGasLimitFlag.Name))

	// Configure the node's service container
	stackConf := &node.Config{
		DataDir:     utils.MakeDataDir(ctx),
		PrivateKey:  utils.MakeNodeKey(ctx),
		Name:        clientIdentifier,
		IPCPath:     utils.MakeIPCPath(ctx),
		HTTPHost:    utils.MakeHTTPRpcHost(ctx),
		HTTPPort:    ctx.GlobalInt(utils.RPCPortFlag.Name),
		HTTPCors:    ctx.GlobalString(utils.RPCCORSDomainFlag.Name),
		HTTPModules: utils.MakeRPCModules(ctx.GlobalString(utils.RPCApiFlag.Name)),
		WSHost:      utils.MakeWSRpcHost(ctx),
		WSPort:      ctx.GlobalInt(utils.WSPortFlag.Name),
		WSOrigins:   ctx.GlobalString(utils.WSAllowedOriginsFlag.Name),
		WSModules:   utils.MakeRPCModules(ctx.GlobalString(utils.WSApiFlag.Name)),
		NoDiscovery: true,
		MaxPeers: 0,
	}
	// Assemble and return the protocol stack
	stack, err := node.New(stackConf)
	if err != nil {
		utils.Fatalf("Failed to create the protocol stack: %v", err)
	}

	// Configure the Ethereum service
	ks := stack.AccountManager().Backends(keystore.KeyStoreType)[0].(*keystore.KeyStore)

	// jitEnabled := ctx.GlobalBool(utils.VMEnableJitFlag.Name)
	ethConf := &eth.Config{
		ChainConfig: utils.MakeChainConfig(ctx, stack),
		// BlockChainVersion:       ctx.GlobalInt(utils.BlockchainVersionFlag.Name), TODO
		DatabaseCache:   ctx.GlobalInt(utils.CacheFlag.Name),
		DatabaseHandles: utils.MakeDatabaseHandles(),
		NetworkId:       ctx.GlobalInt(utils.NetworkIdFlag.Name),
		Etherbase:       utils.MakeEtherbase(ks, ctx),
		//EnableJit:               jitEnabled, // TODO
		//ForceJit:                ctx.GlobalBool(utils.VMForceJitFlag.Name),
		GasPrice:                common.String2Big(ctx.GlobalString(utils.GasPriceFlag.Name)),
		GpoMinGasPrice:          common.String2Big(ctx.GlobalString(utils.GpoMinGasPriceFlag.Name)),
		GpoMaxGasPrice:          common.String2Big(ctx.GlobalString(utils.GpoMaxGasPriceFlag.Name)),
		GpoFullBlockRatio:       ctx.GlobalInt(utils.GpoFullBlockRatioFlag.Name),
		GpobaseStepDown:         ctx.GlobalInt(utils.GpobaseStepDownFlag.Name),
		GpobaseStepUp:           ctx.GlobalInt(utils.GpobaseStepUpFlag.Name),
		GpobaseCorrectionFactor: ctx.GlobalInt(utils.GpobaseCorrectionFactorFlag.Name),
		SolcPath:                ctx.GlobalString(utils.SolcPathFlag.Name),
		PowFake:		 true,
	}

	if err := stack.Register(func(nsc *node.ServiceContext) (node.Service, error) {
		return NewBackend(nsc, ethConf, rpcclient.NewURIClient(rcpuri))
	}); err != nil {
		utils.Fatalf("Failed to register the TMSP application service: %v", err)
	}
	return stack
}
