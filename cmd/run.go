/*
Copyright © 2021 prezha

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cmd

import (
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/spf13/cobra"
)

var (
	testId string

	workers  int           // maximum number of concurrent workers
	requests int           // total number of requests
	duration time.Duration // total time over which all jobs should be executed

	bin     string // specific main binary to use
	subcmd  string // specific binary subcommand to execute
	chainId string // --chain-id string: The network chain ID

	// same keyring backend/passphrase combo is used for all accounts (both sponsor and enpoints)
	// currently, only "test" keyring backend is supported
	keyringBackend    = "test" // --keyring-backend string: Select keyring's backend (os|file|kwallet|pass|test|memory) (default "test")
	keyringPassphrase = ""     // keyring passphrase to pipe to cmd

	sponsor   *account             // account used to create endpoints with balance
	endpoints = map[int]*account{} // senders' and receivers' accounts

	numeps  int    // number of endpoints (addresses) to use
	prefix  string // endpoints' account name prefix
	sourcer string // sponsor's address with resources to create endpoints with balance
	balance string // balance for new accounts; 1 CUDOS = "1 000 000 000 000 000 000 acudos"

	amount string // amount to send with each transaction
	fees   string // --fees string: Fees to pay along with transaction; eg: 10uatom
	gas    string // --gas string:  gas limit to set per-transaction; set to "auto" to calculate sufficient gas automatically (default 200000)

	lcdNode string // Light Client Daemon
	nodes   []string

	broadcastMode string // -b, --broadcast-mode string: Transaction broadcasting mode (sync|async|block) (default "sync")
)

// init defines flags and configuration settings.
func init() {
	rootCmd.AddCommand(runCmd)

	// local flags - only run when this command is called directly
	runCmd.Flags().StringVarP(&testId, "title", "t", time.Now().UTC().Format("20060102150405"), "test reference id")

	runCmd.Flags().StringVarP(&chainId, "chain", "c", "cudos-testnet-public", "cosmos network chain id")

	runCmd.Flags().IntVarP(&workers, "workers", "w", 10, "maximum number of concurrent workers")
	runCmd.Flags().IntVarP(&requests, "requests", "r", 1000, "total number of requests")
	runCmd.Flags().DurationVarP(&duration, "duration", "d", 1*time.Hour, "total time over which all jobs should be executed")

	runCmd.Flags().IntVarP(&numeps, "endpoints", "e", 100, "number of endpoints (local addresses) to use or generate")
	runCmd.Flags().StringVarP(&prefix, "prefix", "p", "flooder-", "endpoints' local account name prefix")
	runCmd.Flags().StringVarP(&sourcer, "sourcer", "s", "", "local address with resources to create endpoints with balance")
	runCmd.Flags().StringVarP(&balance, "balance", "b", "1000000000acudos", "balance for new accounts; 1 cudos = 1 000 000 000 000 000 000 acudos")

	runCmd.Flags().StringVarP(&amount, "amount", "a", "1000acudos", "amount to send with each transaction")
	runCmd.Flags().StringVarP(&fees, "fees", "f", "100acudos", "fees to pay with each transaction")
	runCmd.Flags().StringVarP(&gas, "gas", "g", "auto", "gas limit for each transaction")

	runCmd.Flags().StringVarP(&lcdNode, "lcd", "l", "localhost:1317", "<host>:<port> light client daemon node")
	runCmd.Flags().StringSliceVarP(&nodes, "nodes", "n", []string{"tcp://localhost:26657"}, "slice of <host>:<port> tendermint rpc node(s)")

	runCmd.Flags().StringVarP(&broadcastMode, "mode", "m", "async", "transaction broadcasting mode (sync|async|block)")
}

// runCmd represents the run command
var runCmd = &cobra.Command{
	Use:   "run",
	Short: "run cosmos flooder",
	Long: fmt.Sprintf(`cosmos flooder v%s
run cosmos network load and stress test
           - use with care -`, version),
	Run: func(cmd *cobra.Command, args []string) {
		subcmd = bin + " tx bank send"
		rand.Seed(time.Now().UnixNano())

		fmt.Println("loading local accounts...")
		var err error
		endpoints, err = load(numeps, prefix)
		if err != nil {
			fmt.Printf("failed to load accounts: %v\n", err)
			return
		}

		sponsor = &account{&sync.Mutex{}, numeps, "sponsor", sourcer, "", 0}

		if len(endpoints) < numeps {
			fmt.Printf("not enough local accounts found (will [re]generate %d)\n", numeps)

			if sourcer == "" {
				fmt.Println("failed to initialise sponsor: sourcer address flag must be supplied (cannot continue)!")
				return
			}
			seq, err := sequenceFromBC(sponsor.address, lcdNode)
			if err != nil {
				fmt.Printf("failed to initialise sponsor: %v\n", err)
				return
			}
			sponsor.sequence = seq

			if endpoints, err = charge(numeps, prefix, balance, sponsor, endpoints); err != nil {
				fmt.Printf("failed to [re]generate accounts: %v\n", err)
				return
			}
		}
		if sponsor.address == "" {
			sponsor = endpoints[0]
		}
		fmt.Printf("loaded %d endpoints:\n", len(endpoints))
		for i := 0; i < len(endpoints); i++ {
			fmt.Printf("%+v\n", endpoints[i])
		}
		fmt.Printf("sponsor: %v\n", sponsor)

		start := time.Now()
		fmt.Printf("starting test %s...\n", testId)
		spawn(flags{nil, "", amount, fees, gas, testId, chainId, "", broadcastMode, keyringBackend, "", 0, ""})
		fmt.Printf("%s test completed: %d transactions in %s (%d tx/s)\n", testId, requests, time.Since(start), requests/int(time.Since(start).Seconds()))
	},
}
