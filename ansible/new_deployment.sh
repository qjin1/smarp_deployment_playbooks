#!/bin/sh
#note for the user of this deployment script: BUILD_URL, COMMIT_HASH, NODE_PORT are the env which need to have new input in each deployment

#insert the new url of the build download page
export BUILD_URL=https://git.smarpsocial.com/smarpers/smarpshare/builds/40136/artifacts/download

#insert the new commit hash
export COMMIT_HASH=c5a72cfb

#insert the new port
export NODE_PORT=5940

#first delete the old directories if we have deployed the same commit hash 
cd /mnt/smarpshare/
#rm download*
#rm -rf $COMMIT_HASH
#rm -rf build

wget $BUILD_URL --header "private-token: psxKFaxo5of2y4_LCD8Z"
unzip download
mv build $COMMIT_HASH
rm download*
cd /mnt/smarpshare/$COMMIT_HASH/
trap "" HUP
curl "http://localhost:2053/rp?revision=$COMMIT_HASH" -d "p=http://127.0.0.1:$NODE_PORT" -X POST && \
eval `aws dynamodb scan --table-name EnvVars --output json | envvar` && \
./smarpshare.sh $NODE_PORT > nohup.out 2>&1 &
curl "127.0.0.1:2053/vr?version=rc&revision=$COMMIT_HASH" -X POST -v
export LOGENTRIES_TOKEN=97aa8c56-fad4-488e-b3ce-c2204f6dad3b
tail -fn2000 /mnt/smarpshare/$COMMIT_HASH/nohup.out | while read -r line; do echo "$LOGENTRIES_TOKEN $line" > /dev/tcp/data.logentries.com/80 ; done &
