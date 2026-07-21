.PHONY: all build-windows build-linux build-linux-static clean

all: build-windows build-linux

build-windows:
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w -H windowsgui" -o bin/nimbus-windows-amd64.exe .

build-linux:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/nimbus-linux-amd64 .

build-linux-static:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=1 go build -ldflags="-s -w -linkmode external -extldflags '-static'" -o bin/nimbus-linux-amd64-static .

clean:
	rm -rf bin/
