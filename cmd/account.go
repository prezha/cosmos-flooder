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
	"strconv"
	"strings"
	"sync"
	"time"
)

type account struct {
	m        *sync.Mutex // ensure exclusive account access to control sequence in concurrent and sequential runs; use account.lock() and account.unlock()
	id       int
	name     string
	address  string
	mnemonic string
	sequence uint
}

func (a *account) lock() {
	a.m.Lock()
}

func (a *account) unlock() {
	a.m.Unlock()
}

// add adds amount from sponsor referencing note within 10 seconds (timeout)
func (a *account) add(amount string, sponsor *account, note string) error {
	job := job{"topup", flags{sponsor, a.address, amount, fees, gas, note, chainId, "", broadcastMode, keyringBackend, "", sponsor.sequence, ""}, time.Now().Add(10 * time.Second)}
	return job.execute()
}

// load loads up to qty accounts that have prefix in their name
func load(qty int, prefix string) (map[int]*account, error) {
	var out, out2 []byte
	var err error
	if out, _, err = run(fmt.Sprintf("%s keys list --keyring-backend %s --output json", bin, keyringBackend), keyringPassphrase); err != nil {
		return nil, trimerr(err)
	}
	// make sure '.[]|.name,"\t",.address,"\n"' is tied together as one argument!
	if out2, _, err = run(`jq -j .[]|.name,"\t",.address,"\n"`, string(out)); err != nil {
		return nil, fmt.Errorf("parsing %s: %v", out, err)
	}

	accs := strings.Split(string(out2), "\n")

	accounts := map[int]*account{}
	for _, acc := range accs {
		if len(acc) > 1 {
			name := strings.TrimSpace(strings.Split(string(acc), string('\t'))[0])
			if strings.HasPrefix(name, prefix) {
				if n := strings.Split(name, prefix+"-"); len(n) > 1 {
					ord, err := strconv.Atoi(n[1])
					if err != nil {
						return nil, fmt.Errorf("parsing index: %v", err)
					}

					addr := strings.TrimSpace(strings.Split(string(acc), string('\t'))[1])
					seq, err := sequenceFromBC(addr, lcdNode)
					if err != nil {
						fmt.Printf("error getting account sequence (skipping!): %v\n", err)
						continue
					}

					accounts[ord] = &account{&sync.Mutex{}, ord, name, addr, "", seq}
					if len(accounts) == qty {
						return accounts, nil
					}
				}
			}
		}
	}

	return accounts, nil
}

// charge creates qty new accounts each with name prefix and balance from sponsor
func charge(qty int, prefix string, balance string, sponsor *account, endpoints map[int]*account) (map[int]*account, error) {
	for i := 0; i < qty; i++ {
		acc, err := newAccount(prefix, i)
		if err != nil {
			return nil, err
		}
		if err := acc.add(balance, sponsor, "charging"); err != nil {
			return nil, err
		}
		endpoints[i] = acc
	}
	return endpoints, nil
}

// newAccount returns address of newly created account initialised with name ("<prefix>-<index>")
// in case that the account with the same name exists, if will be replaced with new one
func newAccount(prefix string, index int) (acc *account, err error) {
	name := fmt.Sprintf("%s-%03d", prefix, index)

	var out []byte

	// create new account
	if out, _, err = run(fmt.Sprintf("%s keys add %s --keyring-backend %s --output json", bin, name, keyringBackend), []string{keyringPassphrase, "y"}...); err != nil {
		return nil, trimerr(err)
	}
	// make sure '[.address,.mnemonic]|@tsv' is tied together as one argument!
	if out, _, err = run("jq -r [.address,.mnemonic]|@tsv", string(out)); err != nil {
		return nil, fmt.Errorf("%s - %v", out, err)
	}

	if len(out) > 1 {
		address := strings.TrimSpace(strings.Split(string(out), string('\t'))[0])
		mnemonic := strings.TrimSpace(strings.Split(string(out), string('\t'))[1])
		return &account{&sync.Mutex{}, index, name, address, mnemonic, 0}, nil // make sure caller can always call m.Unlock()
	}

	return nil, fmt.Errorf("failed to create new account: %v", err)
}

// sequenceFromBC queries bc via lcdNode for account info and returns next available sequence (nonce)
func sequenceFromBC(address string, lcdNode string) (sequence uint, err error) {
	var seq int
	var out, out2 []byte
	if out, _, err = run(fmt.Sprintf("curl -sS http://%s/cosmos/auth/v1beta1/accounts/%s", lcdNode, address)); err != nil {
		return 0, err
	}
	if out2, _, err = run("jq -r .account.sequence", string(out)); err != nil {
		return 0, fmt.Errorf("parsing sequence %s: %v", out, err)
	}
	if seq, err = strconv.Atoi(strings.TrimSpace(string(out2))); err != nil {
		return 0, fmt.Errorf("parsing sequence %s: %v", out, err)
	}
	return uint(seq), nil
}
