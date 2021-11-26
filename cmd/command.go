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
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

type flags struct {
	sender         *account // --from string:               Name or address of private key with which to sign. Note, the'--from' flag is ignored as it is implied from [from_key_or_address]
	receiver       string   // address to send to
	amount         string   // value to send
	fees           string   // --fees string:               Fees to pay along with transaction; eg: 10uatom
	gas            string   // --gas string:                gas limit to set per-transaction; set to "auto" to calculate sufficient gas automatically (default 200000)
	note           string   // --note string:               Note to add a description to the transaction (previously --memo)
	chainId        string   // --chain-id string:           The network chain ID
	node           string   // --node string:               <host>:<port> to tendermint rpc interface for this chain (default "tcp://localhost:26657")
	broadcastMode  string   // -b, --broadcast-mode string: Transaction broadcasting mode (sync|async|block) (default "sync")
	keyringBackend string   // --keyring-backend string:    Select keyring's backend (os|file|kwallet|pass|test|memory) (default "test")
	output         string   // -o, --output string:         Output format (text|json) (default "json")
	sequence       uint     // -s, --sequence uint:         The sequence number of the signing account (offline mode only)
	custom         string   // anythin else
}

// hydrate return flags with appropriate values
func (f *flags) hydrate() string {
	// shuffle senders if not given -- not thread-safe: avoid!
	if f.sender == nil && len(endpoints) > 0 {
		i := rand.Intn(len(endpoints))
		f.sender = endpoints[i]
		f.sequence = endpoints[i].sequence
	}

	// shuffle receivers if not given
	if f.receiver == "" && len(endpoints) > 0 {
		f.receiver = endpoints[rand.Intn(len(endpoints))].address
	}
	h := fmt.Sprintf("%s %s %s", f.sender.address, f.receiver, f.amount)

	if f.fees != "" {
		h = fmt.Sprintf("%s --fees %s", h, f.fees)
	}

	if f.gas != "" {
		h = fmt.Sprintf("%s --gas %s", h, f.gas)
	}

	if f.note != "" {
		h = fmt.Sprintf("%s --note %s", h, f.note)
	}

	if f.chainId != "" {
		h = fmt.Sprintf("%s --chain-id %s", h, f.chainId)
	}

	// shuffle nodes; if nodes set is empty, leaving out node flag completely defaults to localhost
	if f.node == "" && len(nodes) > 0 {
		f.node = nodes[rand.Intn(len(nodes))]
	}
	if f.node != "" {
		h = fmt.Sprintf("%s --node %s", h, f.node)
	}

	if f.broadcastMode != "" {
		h = fmt.Sprintf("%s --broadcast-mode %s", h, f.broadcastMode)
	}

	if f.keyringBackend != "" {
		h = fmt.Sprintf("%s --keyring-backend %s", h, f.keyringBackend)
	}

	if f.output != "" {
		h = fmt.Sprintf("%s --output %s", h, f.output)
	}

	if f.sequence > 0 {
		h = fmt.Sprintf("%s --sequence %d", h, f.sequence)
	}

	h += " --yes"

	return h
}

// run executes cmd (command with flags), piping in if given, and returns output or error
// cmd alias (eg, 'alias simd='docker exec -i simd-binary simd') executes in subshell
// do not quote (single nor double) flags' values! - this is handled by [sub]shell
func run(cmd string, in string) (out []byte, duration time.Duration, err error) {
	start := time.Now()

	bin := strings.Split(cmd, " ")[0]
	args := strings.Split(cmd, " ")[1:]

	var c *exec.Cmd
	// check if bin is executable (preffered) or alias
	if _, err := exec.LookPath(bin); err != nil {
		// use alias (has to be defined in .*rc) in subshell
		c = exec.Command(os.Getenv("SHELL"), "-i", "-c", cmd)
	} else {
		c = exec.Command(bin, args...)
	}

	// pipe in
	if in != "" {
		if !strings.HasSuffix(in, "\n") {
			in += "\n"
		}
		var stdin io.WriteCloser
		if stdin, err = c.StdinPipe(); err != nil {
			return nil, time.Since(start), fmt.Errorf("failed to get StdinPipe for %s: %v", c.String(), err)
		}
		go func() {
			defer stdin.Close()
			io.WriteString(stdin, in)
		}()
	}

	o, err := c.CombinedOutput() // alternatively use 'c.Stderr = nil' then 'o, err = c.Output()', whereas 'err.(*exec.ExitError).Stderr' will contain error
	if err != nil {
		return nil, time.Since(start), fmt.Errorf("failed to execute '%s': %v - %s", c.String(), err, o)
	}

	return o, time.Since(start), nil
}

// trimerr extracts error line from err starting with "Error: " ignoring everything else (eg, command's help output)
func trimerr(err error) error {
	if err != nil {
		re := regexp.MustCompile("Error: (.*)\n")
		if m := re.FindStringSubmatch(err.Error()); len(m) > 0 {
			return errors.New(m[1])
		}
	}
	return err
}
