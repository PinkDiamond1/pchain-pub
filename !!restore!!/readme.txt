**********************************************************************************************************
*                                                     README                                             *
**********************************************************************************************************

here are the modifications based on the original version:
----------------------------------------------------------
1, fix the tx gap between ethermintapp and ethereum because of gaslimit check 
2, fix multi-tx with the same nonce in ethereum
3, migrate attach command for ethermint from ethereum
4, support validator adding, with some practice principles
5, supply fake interfaces with eth-netstats monitor webpage, to make it run


here are issues/future tasks:
-----------------------------
1, deploy the multi-node network with scripts
2, fix the bug found in performance test
3, fix the bug found in product test
4, modify the eth-netstats monitor webpage to adapt this ethermint platform


here are the deployment steps:
------------------------------
1, get a ethermint-master-xx.tar.gz source package, such as 'ethermint-master-full.20170525.tar.gz', 
   or get an un-compressed copy, copy the source to a location, ie

	cp ethermint-master /mnt/vdb/ethermint-master

2, step into the directory by 

	cd /mnt/vdb/ethermint-master

3, follow the instructs to make it run in single node,

   3.1) login with user 'ubuntu' under ubuntu 14.0.4; the instructs are linux commands:

	#build the exe file, download necessary software such as golang package
	./build.sh

	#make sure the generated 'ethermint' is the newest; there are some other exe files in the same directory,
	# we will use the directly
	ls ./bin

	#make it simple to run 'ethermint'
	sudo cp ./bin/ethermint /usr/local/bin/

	#package the genesis.json and init chain data
	ethermint -datadir /home/ubuntu/.ethermint init ./src/github.com/tendermint/ethermint/dev/genesis.json

	#create default user
	cp -r ./src/github.com/tendermint/ethermint/dev/keystore /home/ubuntu/.ethermint/keystore

	#start ehtermint node, run_debug.sh with more log output
	./run.sh

    3.2) login to another console and run this cmd to check the log

	tail -f /mnt/vdb/ethermint-master/ethereum.log

4, follow steps 1-3 with the same source code in another machine, build a fresh node('B') but don't start it (!!!!not run the ./run.sh!!!!).

   Here are the steps to make the new node ('B') join existing node ('A'):

   4.1) stop ethermint on A
   4.2) add the peer entry,

		add B's ip:port(say 10.104.107.82:46656) to A's /home/ubuntu/.ethermint/config.toml,
		add A's ip:port(say 10.104.105.106:46656) to B's /home/ubuntu/.ethermint/config.toml.

	 then, A's config.toml should look like:

		# This is a TOML config file.
		# For more information, see https://github.com/toml-lang/toml

		proxy_app = "tcp://127.0.0.1:46658"
		moniker = "anonymous"
		node_laddr = "tcp://0.0.0.0:46656"
		seeds = "10.104.107.82:46656"
		fast_sync = true
		db_backend = "leveldb"
		log_level = "notice"
		rpc_laddr = "tcp://0.0.0.0:46657"

   4.3) copy A's /home/ubuntu/.ethermint/genesis.json to B's /home/ubuntu/.ethermint/, make their genesis.json keep the same,
        therefor the validators are the same between A and B.
   4.4) run ethermint on A and B (order is not relevant). After connected, B should start synchronize blocks from A,
        and they will have the same blockchain behaviors when synchronzation is done

5, here is the step to make a node to be a validator, joining to existing validator(s). 

   we assume there already are/is existing validators. with default single-node deployment, the node itself will be a validator.

   5.1) follow 1-4 with the same souce code, add a new node ('C') to the existing network but don't start it (!!!!not run the ./run.sh!!!!).
   5.2) pick one validator (node 'A'), add C's pub-key (the long hex-string of "pub_key" part in /home/ubuntu/.ethermint/priv_validator.json) 
        to A's "validators" part in A's /home/ubuntu/.ethermint/genesis.json. then A's "validators" part in genesis.json should look like:
        
        "validators": [
                ...
                {
                				"eth_account": "0x7eff122b94897ea5b0e2a9abf47b86337fafebdc"
                        "amount": 10,
                        "name": "",
                        "pub_key": [
                                1,
                                "AE7AF8281D31B9A2CB0A9BC75D925CAD7E8DD6781601EF1519E0B0E01612F1FA"
                        ]
                },
                { #here is C's pub-key part
                        "eth_account": "0x32ef122b94897ea5b0e2a9abf4cda6337fafebdc"
                        "amount": 10,	#vote power
                        "name": "",   #name, could be empty
                        "pub_key": [  #pub-key part
                                1,
                                "E94937710077B38C2A1334B89B394D61CFBFEE732A54F9358A73E36ED0827B9A" #copy from C's priv_validator.json
                        ]
                },
                ...
        ]
        
        make sure C's amount is smaller than 1/2 of the sum of all other validators' amount. (!!! important, this follows the consensus algorithm of tendermint !!!)

   5.3) copy A's genesis.json to all other nodes within the network, including C
   5.4) stop all nodes within the network
   5.5) start all nodes with C the last one to start
   5.6) extension: 5.1-5.5 should work for batch-add of new validators. in the 5.2 step, make sure the sum of the new validators' amount 
        is smaller than 1/2 of the sum of the existing validators' amount. in the 5.5 step, start all nodes with the new validators start lately
        

reference:
---------
https://tendermint.com/intro/getting-started/deploy-testnet
