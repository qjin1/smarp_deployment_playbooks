#!/bin/sh
#note for the user of this old revision removal script: COMMIT_HASH, NODE_PORT are the env which need to be set
export NODE_PORT=5936
export COMMIT_HASH=756b0314
#remove the unwanted version
kill `ps -ef|grep $NODE_PORT|grep webserver|awk '{print $3}'` `ps -ef|grep $NODE_PORT|grep webserver|awk '{print $2}'`

#remove the unwanted rp
curl "127.0.0.1:2053/rp?revision=$COMMIT_HASH" -X DELETE -v

#remove the useless folder in /mnt/smarpshare
rm -rf /mnt/smarpshare/$COMMIT_HASH
