
prefix ?= $$HOME/.local
APP = mspview
all: $(APP)

$(APP):	$(wildcard *.go) go.sum
	-go build -ldflags "-w -s"

go.sum: go.mod
	go mod tidy

windows:
	GOOS=windows go build -ldflags "-w -s"

freebsd:
	GOOS=freebsd go build -ldflags "-w -s" -o fbsd-mspview

aarch64:
	GOARCH=arm64 go build -ldflags "-w -s" -o arm64-mspview

riscv64:
	GOARCH=riscv64 go build -ldflags "-w -s" -o rv64-mspview

world:	$(APP) windows freebsd aarch64 riscv64

clean:
	go clean
	rm -f *-mspview

install: $(APP)
	-install -d $(prefix)/bin
	-install -s $(APP) $(prefix)/bin/$(APP)
