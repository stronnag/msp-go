
prefix ?= $$HOME/.local
APP = msptest
all: $(APP)

$(APP):	$(wildcard *.go) go.sum
	-go build -ldflags "-w -s"

go.sum: go.mod
	go mod tidy

clean:
	go clean

install: $(APP)
	-install -d $(prefix)/bin
	-install -s $(APP) $(prefix)/bin/$(APP)
