#!/bin/bash

server="./projectc-wallet"
let item=0
item=`ps -ef | grep $server | grep -v grep | wc -l`

if [ $item -eq 1 ]; then
	echo "The projectc-wallet is running, shut it down..."
	pid=`ps -ef | grep $server | grep -v grep | awk '{print $2}'`
	kill -9 $pid
fi

echo "Start projectc-wallet now ..."
make build
./build/pkg/cmd/projectc-wallet/projectc-wallet  >> projectc-wallet.log 2>&1 &
