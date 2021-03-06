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
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type job struct {
	id       string
	flags    flags
	deadline time.Time
}

// execute will try to run job
// it will lock sender's account address during execution to reduce 'account sequence mismatch' errors rate
// likewise, it will also preemptively update account sequece value for next use
func (j *job) execute() error {
	start := time.Now()

	var err error
	var out []byte

	// handle 'Error: rpc error: code = InvalidArgument desc = account sequence mismatch, expected 157, got 156: incorrect account sequence: invalid request'
	re := regexp.MustCompile(`account sequence mismatch, expected ([0-9]+),`) // extract right sequence from error

	if j.flags.sender == nil && len(endpoints) > 0 {
		i := rand.Intn(len(endpoints))
		j.flags.sender = endpoints[i]
	}
	j.flags.sender.lock()
	defer j.flags.sender.unlock()

	retries := 0
	j.flags.sequence = j.flags.sender.sequence // copy sender's account sequence to flags
	for time.Now().Before(j.deadline) {
		args := j.flags.hydrate()

		cmd := fmt.Sprintf("%s %s", subcmd, args)
		fmt.Printf("%s job started: %s\n", j.id, cmd)
		fmt.Printf("%s sender: %+v\n", j.id, j.flags.sender)
		out, _, err = run(cmd, keyringPassphrase)
		seq := int(j.flags.sender.sequence)
		var err2 error
		if err != nil {
			if r := re.FindStringSubmatch(trimerr(err).Error()); len(r) > 0 {
				if seq, err2 = strconv.Atoi(r[1]); err2 == nil {
					fmt.Printf("%s job errored (will retry): %v\n", j.id, trimerr(err))
					retries++
					j.flags.sequence = uint(seq)        // update job's flags sequence
					j.flags.sender.sequence = uint(seq) // update sender's account sequence
					continue
				}
			}
			return fmt.Errorf("%s job failed (unretryable) after %s: %v", j.id, time.Since(start), trimerr(err))
		}
		fmt.Printf("%s job completed in %s (retries: %d): %s\n", j.id, time.Since(start), retries, out)
		j.flags.sender.sequence = uint(seq) + 1 // automatically set sender's account sequence on successful job completion for next run
		return nil
	}
	// note: presence of "job timed out" substring is checked in spawn()
	return fmt.Errorf("%s job timed out (retries: %d): %v", j.id, retries, trimerr(err))
}

func spawn(flags flags) {
	jobs := make(chan job, requests)

	var wg sync.WaitGroup
	// spawn all workers
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			for j := range jobs {
				j.id = fmt.Sprintf("%s.%d", j.id, i) // amend job id with worker id
				if err := j.execute(); err != nil {
					fmt.Printf("%v\n", err)
					if strings.Contains(err.Error(), "job timed out") {
						return
					}
					continue
				}
				atomic.AddInt64(&finished, 1)
			}
		}(i)
	}

	// send all jobs to channel, then signal the end and wait for completion, limiting overall execution time with deadline based on duration.
	// if spread flag was set - try to also have near-evenly distribution of given number of jobs over requested duration, effectively rate-limiting itself.
	deadline := time.Now().Add(duration)
	delay := time.Duration(duration.Nanoseconds() / int64(requests))
	if !spread || delay < time.Millisecond {
		delay = 0
	}
	for i := 0; i < requests; i++ {
		id := fmt.Sprintf("%s-%d", testId, i)
		flags.note = id
		jobs <- job{id, flags, deadline}
		time.Sleep(delay)
	}
	close(jobs) // signal end of jobs queue to stop workers when done processing
	wg.Wait()
}
