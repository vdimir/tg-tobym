
.PHONY: all test local verbose


all:

test:
	go test ./... -race

local:
	go run ./app -longpoll -listen '127.0.0.1:8080'

verbose:
	go run ./app -longpoll -listen '127.0.0.1:8080' -debug

