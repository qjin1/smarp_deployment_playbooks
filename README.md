## How to Cross-Compile Go on OS X

`GOOS=linux GOARCH=386 CGO_ENABLED=0 go build -o <OUTPUT> <DEV_FOLDER>/committee/src/committee.go`