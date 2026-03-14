APP := easybot

.PHONY: build run run-http test clean

build:
	go build -o $(APP) ./cmd/easybot

run: build
	./$(APP)

run-http: build
	./$(APP) --http --listen :8080

test:
	go test ./...

clean:
	rm -f $(APP)
