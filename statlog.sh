#!/bin/bash

log=$1

echo "jobs stats from $log:"

requested=$(awk '/test started/ {print $6; exit}' $log | awk -F: '{print $2}') 
echo -e "- requested:\t" $requested

started=$(grep "job started" $log | wc -l)
echo -e "- started:\t" $started

completed=$(grep "job completed" $log | wc -l)
echo -e "- completed:\t" $completed

errored=$(grep "job errored" $log | wc -l)
echo -e "- retried\t" $errored

timeout=$(grep "job timed out" $log | wc -l)
echo -e "- timed out:\t" $timeout

failed=$(grep "job failed" $log | wc -l)
echo -e "- failed jobs:\t" $failed

