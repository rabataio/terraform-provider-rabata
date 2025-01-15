build:
	go build -v ./...

generate:
	go generate ./...

build_docs:
	tfplugindocs generate --provider-dir . -provider-name rabata

install_tools:
	cat tools/tools.go | grep _ | awk -F'"' '{print $$2}' | xargs -tI % go install %

lint:
	golangci-lint run
