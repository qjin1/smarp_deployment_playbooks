# Comm handler micro-service.

## How to Cross-Compile Script on OS X

`GOOS=linux GOARCH=386 CGO_ENABLED=0 go build -o <OUTPUT> <DEV_FOLDER>/committee/src/committee.go`

## How run docker with persistent files

`docker run -v "<PATH_TO_FILES>:/usr/bin/app/data" -p 2052:2052 -p 2053:2053 -ti committee`