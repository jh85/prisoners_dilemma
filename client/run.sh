#!/bin/bash
for s in TFT TTT ALC ALD FDM RDM TLK SLV MST GRO JOS DOW GKP MMC; do 
    ./client --addr localhost:19714 --strategy $s >& /dev/null &
done

