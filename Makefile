BINARY := kanban
DB     := $(HOME)/.kanbantui/kanban.db

.PHONY: build run test vet fmt backup clean

build:
	go build -o $(BINARY) ./cmd/kanban
	mkdir -p $(HOME)/.kanbantui
	pwd > $(HOME)/.kanbantui/project_dir

run: build
	./$(BINARY)

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -l -w .

backup:
	mkdir -p $(HOME)/.kanbantui/backups
	sqlite3 $(DB) ".backup '$(HOME)/.kanbantui/backups/kanban-$$(date +%Y%m%d-%H%M%S).db'"

clean:
	rm -f $(BINARY)
